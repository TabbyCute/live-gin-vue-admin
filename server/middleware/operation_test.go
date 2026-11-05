package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestOperationRecordRedactsSensitiveJSONWithoutChangingHandlerBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupOperationRecordTest(t)
	requestBody := `{"accountId":7,"phone":"+60123456789","smsCode":"012345","evidenceRef":"private-proof","externalOrderNo":"BANK-PRIVATE-001","nested":{"newPassword":"secret123","super-code":"654321","requestHash":"private-digest"}}`
	var handlerBody string
	router := gin.New()
	router.Use(OperationRecord())
	router.POST("/redacted", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		handlerBody = string(body)
		c.JSON(http.StatusOK, gin.H{"phone": "+60123456789", "result": "ok", "data": gin.H{"evidenceRef": "private-proof", "externalOrderNo": "BANK-PRIVATE-001"}})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/redacted", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, requestBody, handlerBody)
	var record system.SysOperationRecord
	require.NoError(t, db.Order("id DESC").First(&record).Error)
	require.NotContains(t, record.Body, "+60123456789")
	require.NotContains(t, record.Body, "secret123")
	require.NotContains(t, record.Body, "012345")
	require.NotContains(t, record.Body, "654321")
	require.NotContains(t, record.Resp, "+60123456789")
	for _, value := range []string{"private-proof", "BANK-PRIVATE-001", "private-digest"} {
		require.NotContains(t, record.Body, value)
		require.NotContains(t, record.Resp, value)
	}
	require.Contains(t, record.Body, operationRecordRedacted)
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

func TestOperationRecordRedactionFailsClosed(t *testing.T) {
	for _, input := range []string{
		`{"externalOrderNo":"PRIVATE-BANK","amount":`,
		`{"externalOrderNo":"PRIVATE-BANK","x":"` + strings.Repeat("a", 980) + `"}`,
	} {
		require.NotContains(t, string(redactOperationRecordJSON([]byte(input))), "PRIVATE-BANK")
	}
}

func TestOperationRecordHasIndependentBoundedContext(t *testing.T) {
	db := setupOperationRecordTest(t)
	entered := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test_operation_budget", func(tx *gorm.DB) {
		if tx.Statement.Table != "sys_operation_records" {
			return
		}
		entered = true
		require.NoError(t, tx.Statement.Context.Err(), "client cancellation should not discard supplemental log")
		deadline, ok := tx.Statement.Context.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 2*time.Second)
		<-tx.Statement.Context.Done()
		tx.AddError(tx.Statement.Context.Err())
	}))
	t.Cleanup(func() { db.Callback().Create().Remove("test_operation_budget") })
	router := gin.New()
	router.Use(OperationRecord())
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("POST", "/", strings.NewReader("{}")).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	start := time.Now()
	router.ServeHTTP(w, request)
	require.True(t, entered)
	require.Less(t, time.Since(start), 4*time.Second)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestWalletOperationRedactionRegardlessOfContentType(t *testing.T) {
	db := setupOperationRecordTest(t)
	router := gin.New()
	router.Use(OperationRecord())
	router.POST("/wallet/adjustment/create", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	for _, contentType := range []string{"Application/JSON", "text/plain", ""} {
		r := httptest.NewRequest("POST", "/wallet/adjustment/create", strings.NewReader(`{"evidenceRef":"PRIVATE-PROOF","externalOrderNo":"PRIVATE-BANK"}`))
		r.Header.Set("Content-Type", contentType)
		router.ServeHTTP(httptest.NewRecorder(), r)
		var record system.SysOperationRecord
		require.NoError(t, db.Order("id DESC").First(&record).Error)
		require.NotContains(t, record.Body, "PRIVATE-PROOF")
		require.NotContains(t, record.Body, "PRIVATE-BANK")
	}
}
