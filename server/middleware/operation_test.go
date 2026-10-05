package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tb_live_module/global"
	"tb_live_module/model/system"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setupOperationRecordTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&system.SysOperationRecord{}))
	previousDB, previousLog := global.GVA_DB, global.GVA_LOG
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
	})
	return db
}

func TestOperationRecordBoundsRequestAndResponseButPreservesHandlerBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupOperationRecordTest(t)
	requestBody := strings.Repeat("request-", 4096)
	responseBody := strings.Repeat("response-", 4096)

	var handlerBody string
	router := gin.New()
	router.Use(OperationRecord())
	router.POST("/bounded", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		handlerBody = string(body)
		c.String(http.StatusOK, responseBody)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/bounded", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, requestBody, handlerBody, "日志截断不能截断业务Handler实际收到的请求体")
	require.Equal(t, responseBody, recorder.Body.String(), "日志截断不能截断客户端实际收到的响应")
	var record system.SysOperationRecord
	require.NoError(t, db.Order("id DESC").First(&record).Error)
	require.Equal(t, operationRecordTooLarge, record.Body)
	require.Equal(t, operationRecordTooLarge, record.Resp)
}

func TestOperationRecordDoesNotDrainUnreadLargeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupOperationRecordTest(t)
	reader := &countingReadCloser{Reader: strings.NewReader(strings.Repeat("x", 1<<20))}

	router := gin.New()
	router.Use(OperationRecord())
	router.POST("/ignore-body", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignore-body", nil)
	request.Body = reader
	router.ServeHTTP(recorder, request)

	require.LessOrEqual(t, reader.readBytes, operationRecordCaptureLimit+1)
	var record system.SysOperationRecord
	require.NoError(t, db.Order("id DESC").First(&record).Error)
	require.Equal(t, operationRecordTooLarge, record.Body)
}

type countingReadCloser struct {
	io.Reader
	readBytes int
}

func (r *countingReadCloser) Read(value []byte) (int, error) {
	n, err := r.Reader.Read(value)
	r.readBytes += n
	return n, err
}

func (r *countingReadCloser) Close() error { return nil }
