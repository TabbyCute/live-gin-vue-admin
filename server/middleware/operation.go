package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tb_live_module/global"
	"tb_live_module/model/system"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const operationRecordCaptureLimit = 1024

const (
	operationRecordTooLarge = "[超出记录长度]"
	operationRecordFileBody = "[文件]"
)

func OperationRecord() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body []byte
		var bodyTruncated bool
		var userID int
		contentType := c.GetHeader("Content-Type")

		switch {
		case c.Request.Method == http.MethodGet:
			body, bodyTruncated = operationRecordQuery(c.Request.URL.RawQuery)
		case strings.Contains(contentType, "multipart/form-data"):
			// 文件请求完全不预读，避免日志中间件把整个上传内容复制进内存。
			body = []byte(operationRecordFileBody)
		default:
			captured, truncated, replacement, err := captureOperationRequestBody(c.Request.Body, operationRecordCaptureLimit)
			c.Request.Body = replacement
			body, bodyTruncated = captured, truncated
			if err != nil && global.GVA_LOG != nil {
				global.GVA_LOG.Error("read body from request error:", zap.Error(err))
			}
		}

		claims, _ := utils.GetClaims(c)
		if claims != nil && claims.BaseClaims.ID != 0 {
			userID = int(claims.BaseClaims.ID)
		} else {
			id, err := strconv.Atoi(c.Request.Header.Get("x-user-id"))
			if err == nil {
				userID = id
			}
		}
		record := system.SysOperationRecord{
			Ip:     c.ClientIP(),
			Method: c.Request.Method,
			Path:   boundedOperationRecordText(c.Request.URL.Path),
			Agent:  boundedOperationRecordText(c.Request.UserAgent()),
			Body:   operationRecordText(body, bodyTruncated),
			UserID: userID,
		}

		writer := &responseBodyWriter{ResponseWriter: c.Writer, limit: operationRecordCaptureLimit}
		c.Writer = writer
		now := time.Now()

		c.Next()

		record.ErrorMessage = boundedOperationRecordText(c.Errors.ByType(gin.ErrorTypePrivate).String())
		record.Status = c.Writer.Status()
		record.Latency = time.Since(now)
		if isDownloadResponse(c.Writer.Header()) {
			record.Resp = operationRecordFileBody
		} else {
			record.Resp = operationRecordText(writer.body.Bytes(), writer.truncated)
		}

		if global.GVA_DB == nil {
			if global.GVA_LOG != nil {
				global.GVA_LOG.Error("create operation record error: database is nil")
			}
			return
		}
		if err := global.GVA_DB.Create(&record).Error; err != nil && global.GVA_LOG != nil {
			global.GVA_LOG.Error("create operation record error:", zap.Error(err))
		}
	}
}

// captureOperationRequestBody 最多读取 limit+1 字节用于判断是否截断，再把已读取前缀与原流拼回去。
// 因此日志内存有界，同时后续 Handler 仍能按原顺序读取完整请求体。
func captureOperationRequestBody(body io.ReadCloser, limit int) ([]byte, bool, io.ReadCloser, error) {
	if body == nil {
		return nil, false, http.NoBody, nil
	}
	prefix, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	replacement := &operationReplayBody{
		Reader: io.MultiReader(bytes.NewReader(prefix), body),
		closer: body,
	}
	if len(prefix) <= limit {
		return prefix, false, replacement, err
	}
	return prefix[:limit], true, replacement, err
}

type operationReplayBody struct {
	io.Reader
	closer io.Closer
}

func (b *operationReplayBody) Close() error {
	return b.closer.Close()
}

func operationRecordQuery(rawQuery string) ([]byte, bool) {
	if len(rawQuery) > operationRecordCaptureLimit {
		return nil, true
	}
	query, _ := url.QueryUnescape(rawQuery)
	parts := strings.Split(query, "&")
	values := make(map[string]string, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) == 2 {
			values[pair[0]] = pair[1]
		}
	}
	body, _ := json.Marshal(&values)
	if len(body) > operationRecordCaptureLimit {
		return body[:operationRecordCaptureLimit], true
	}
	return body, false
}

func operationRecordText(value []byte, truncated bool) string {
	if truncated {
		return operationRecordTooLarge
	}
	return string(value)
}

func boundedOperationRecordText(value string) string {
	if len(value) > operationRecordCaptureLimit {
		return operationRecordTooLarge
	}
	return value
}

func isDownloadResponse(header http.Header) bool {
	return strings.Contains(header.Get("Pragma"), "public") ||
		strings.Contains(header.Get("Expires"), "0") ||
		strings.Contains(header.Get("Cache-Control"), "must-revalidate, post-check=0, pre-check=0") ||
		strings.Contains(header.Get("Content-Type"), "application/force-download") ||
		strings.Contains(header.Get("Content-Type"), "application/octet-stream") ||
		strings.Contains(header.Get("Content-Type"), "application/vnd.ms-excel") ||
		strings.Contains(header.Get("Content-Type"), "application/download") ||
		strings.Contains(header.Get("Content-Disposition"), "attachment") ||
		strings.Contains(header.Get("Content-Transfer-Encoding"), "binary")
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	limit     int
	truncated bool
}

func (w *responseBodyWriter) Write(value []byte) (int, error) {
	w.captureBytes(value)
	return w.ResponseWriter.Write(value)
}

func (w *responseBodyWriter) WriteString(value string) (int, error) {
	w.captureString(value)
	return w.ResponseWriter.WriteString(value)
}

func (w *responseBodyWriter) captureBytes(value []byte) {
	remaining := w.limit - w.body.Len()
	if remaining > 0 {
		written := len(value)
		if written > remaining {
			written = remaining
		}
		_, _ = w.body.Write(value[:written])
	}
	if len(value) > remaining {
		w.truncated = true
	}
}

func (w *responseBodyWriter) captureString(value string) {
	remaining := w.limit - w.body.Len()
	if remaining > 0 {
		written := len(value)
		if written > remaining {
			written = remaining
		}
		_, _ = w.body.WriteString(value[:written])
	}
	if len(value) > remaining {
		w.truncated = true
	}
}
