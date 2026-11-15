package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/utils"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/system"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var respPool sync.Pool
var bufferSize = 1024

func init() {
	respPool.New = func() interface{} {
		return make([]byte, bufferSize)
	}
}

func OperationRecord() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body []byte
		var userId int
		if c.Request.Method != http.MethodGet {
			var err error
			body, err = io.ReadAll(c.Request.Body)
			if err != nil {
				global.GVA_LOG.Error("read body from request error:", zap.Error(err))
			} else {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
			}
		} else {
			query := c.Request.URL.RawQuery
			query, _ = url.QueryUnescape(query)
			split := strings.Split(query, "&")
			m := make(map[string]string)
			for _, v := range split {
				kv := strings.Split(v, "=")
				if len(kv) == 2 {
					m[kv[0]] = kv[1]
				}
			}
			body, _ = json.Marshal(&m)
		}
		claims, _ := utils.GetClaims(c)
		if claims != nil && claims.BaseClaims.ID != 0 {
			userId = int(claims.BaseClaims.ID)
		} else {
			id, err := strconv.Atoi(c.Request.Header.Get("x-user-id"))
			if err != nil {
				userId = 0
			}
			userId = id
		}
		record := system.SysOperationRecord{
			Ip:     c.ClientIP(),
			Method: c.Request.Method,
			Path:   c.Request.URL.Path,
			Agent:  c.Request.UserAgent(),
			Body:   "",
			UserID: userId,
		}

		// 上传文件时候 中间件日志进行裁断操作
		if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
			record.Body = "[文件]"
		} else {
			if len(body) > bufferSize {
				record.Body = "[超出记录长度]"
			} else {
				record.Body = redactOperationPayload(body)
			}
		}

		writer := responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer
		now := time.Now()

		c.Next()

		latency := time.Since(now)
		record.ErrorMessage = c.Errors.ByType(gin.ErrorTypePrivate).String()
		record.Status = c.Writer.Status()
		record.Latency = latency
		if writer.body.Len() > bufferSize {
			record.Resp = "[超出记录长度]"
		} else {
			record.Resp = redactOperationPayload(writer.body.Bytes())
		}

		if strings.Contains(c.Writer.Header().Get("Pragma"), "public") ||
			strings.Contains(c.Writer.Header().Get("Expires"), "0") ||
			strings.Contains(c.Writer.Header().Get("Cache-Control"), "must-revalidate, post-check=0, pre-check=0") ||
			strings.Contains(c.Writer.Header().Get("Content-Type"), "application/force-download") ||
			strings.Contains(c.Writer.Header().Get("Content-Type"), "application/octet-stream") ||
			strings.Contains(c.Writer.Header().Get("Content-Type"), "application/vnd.ms-excel") ||
			strings.Contains(c.Writer.Header().Get("Content-Type"), "application/download") ||
			strings.Contains(c.Writer.Header().Get("Content-Disposition"), "attachment") ||
			strings.Contains(c.Writer.Header().Get("Content-Transfer-Encoding"), "binary") {
			if len(record.Resp) > bufferSize {
				// 截断
				record.Body = "超出记录长度"
			}
		}
		if err := global.GVA_DB.Create(&record).Error; err != nil {
			global.GVA_LOG.Error("create operation record error:", zap.Error(err))
		}
	}
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r responseBodyWriter) Write(b []byte) (int, error) {
	// 仅缓存日志所需内容，响应仍完整发送给客户端。
	if remaining := bufferSize + 1 - r.body.Len(); remaining > 0 {
		if remaining > len(b) {
			remaining = len(b)
		}
		r.body.Write(b[:remaining])
	}
	return r.ResponseWriter.Write(b)
}

func redactOperationPayload(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "[非 JSON 内容已省略]"
	}
	redactOperationValue(payload)
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > bufferSize {
		return "[超出记录长度]"
	}
	return string(encoded)
}

func redactOperationValue(value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
			sensitive := false
			for _, name := range []string{"password", "passwd", "secret", "token", "signingkey", "authorization", "cookie", "privatekey", "credential"} {
				if strings.Contains(normalized, name) {
					sensitive = true
					break
				}
			}
			if sensitive {
				v[key] = "[已隐藏]"
			} else {
				redactOperationValue(child)
			}
		}
	case []interface{}:
		for _, child := range v {
			redactOperationValue(child)
		}
	}
}
