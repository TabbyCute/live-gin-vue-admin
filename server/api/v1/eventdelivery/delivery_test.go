package eventdelivery

import (
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/eventdelivery"
	"testing"
)

func TestAdminBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, svc.Migrate(db))
	require.NoError(t, db.Exec("CREATE TABLE sys_users (id INTEGER PRIMARY KEY, enable INTEGER, deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_users (id,enable) VALUES (1,1),(2,1)").Error)
	old := deliveryService
	deliveryService = svc.ServiceGroup{Service: &svc.Service{DB: db}}
	defer func() { deliveryService = old }()
	for _, tc := range []struct {
		name, path, body string
		actor            uint
		want             string
	}{
		{"non_root_list", "/list", "", 2, "EVENT_DELIVERY_FORBIDDEN"},
		{"non_root_detail", "/detail?id=1", "", 2, "EVENT_DELIVERY_FORBIDDEN"},
		{"forged_role", "/retry", `{"requestId":"request_123","deliveryId":"1","generation":0,"reason":"approved restore"}`, 2, "EVENT_DELIVERY_FORBIDDEN"},
		{"unknown_key", "/retry", `{"requestId":"request_123","deliveryId":"1","generation":0,"reason":"approved restore","adapterUrl":"https://example.test"}`, 1, "EVENT_DELIVERY_INVALID"},
		{"duplicate_query", "/list?page=1&page=2", "", 1, "EVENT_DELIVERY_INVALID"},
		{"invalid_percent", "/list?page=%xx", "", 1, "EVENT_DELIVERY_INVALID"},
		{"semicolon", "/list?page=1;garbage=2", "", 1, "EVENT_DELIVERY_INVALID"},
		{"duplicate_key", "/retry", `{ "requestId":"request_123","requestId":"request_456" }`, 1, "EVENT_DELIVERY_INVALID"},
		{"case_alias", "/retry", `{ "RequestId":"request_123" }`, 1, "EVENT_DELIVERY_INVALID"},
		{"null_json", "/retry", `null`, 1, "EVENT_DELIVERY_INVALID"},
		{"null_field", "/retry", `{ "generation":null }`, 1, "EVENT_DELIVERY_INVALID"},
		{"query_on_mutation", "/retry?extra=1", `{}`, 1, "EVENT_DELIVERY_INVALID"},
		{"trailing_json", "/retry", `{} {}`, 1, "EVENT_DELIVERY_INVALID"},
		{"oversized_body", "/retry", `{"reason":"` + strings.Repeat("a", 3000) + `"}`, 1, "EVENT_DELIVERY_INVALID"},
		{"anonymous", "/list", "", 0, "EVENT_DELIVERY_FORBIDDEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tc.actor > 0 {
					claims := &systemReq.CustomClaims{}
					claims.BaseClaims.ID = tc.actor
					claims.AuthorityId = 888
					c.Set("claims", claims)
				}
			})
			api := API{}
			r.GET("/list", api.List)
			r.GET("/detail", api.Detail)
			r.POST("/retry", api.Retry)
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}
			request := httptest.NewRequest(method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			r.ServeHTTP(result, request)
			require.Contains(t, result.Body.String(), tc.want)
			require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
			require.NotContains(t, result.Body.String(), "SELECT")
		})
	}
}

func FuzzRetryBody(f *testing.F) {
	gin.SetMode(gin.TestMode)
	for _, seed := range []string{`{}`, `null`, `{"requestId":"one","requestId":"two"}`, `{"RequestId":"alias"}`, `{"reason":"valid text"}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var input svc.RetryRequest
		accepted := retryBody(c, &input)
		if len(body) > 2048 {
			require.False(t, accepted)
		}
	})
}
