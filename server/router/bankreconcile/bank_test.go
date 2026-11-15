package bankreconcile_test

import (
	"bufio"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/songzhibin97/gkit/cache/local_cache"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"tb_live_module/config"
	"tb_live_module/global"
	sr "tb_live_module/model/system/request"
	router "tb_live_module/router/bankreconcile"
	"tb_live_module/utils"
	"testing"
	"time"
)

type observedBody struct {
	io.ReadCloser
	once    sync.Once
	started chan struct{}
}

func (b *observedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { b.started <- struct{}{} })
	return b.ReadCloser.Read(p)
}

func TestBankAdminBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldConfig, oldCache, oldLog := global.GVA_CONFIG, global.BlackCache, global.GVA_LOG
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "local-admin-signing", Issuer: "local-admin", ExpiresTime: "1h", BufferTime: "1m"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "local-app-signing", Issuer: "local-app", ExpiresTime: "1h"}
	global.BlackCache = local_cache.NewCache(local_cache.SetDefaultExpire(time.Hour))
	global.GVA_LOG = zap.NewNop()
	t.Cleanup(func() { global.GVA_CONFIG = oldConfig; global.BlackCache = oldCache; global.GVA_LOG = oldLog })
	app, _, e := utils.NewAppJWT().CreateToken(1, "test", 1)
	require.NoError(t, e)
	jwt := utils.NewJWT()
	admin, e := jwt.CreateToken(jwt.CreateClaims(sr.BaseClaims{ID: 1, AuthorityId: 888}))
	require.NoError(t, e)
	engine := gin.New()
	router.RouterGroup{}.InitBankReconcileRouter(engine.Group("/v1/admin"))
	require.Len(t, engine.Routes(), 5)
	for _, route := range engine.Routes() {
		for _, token := range []string{"", app} {
			request := httptest.NewRequest(route.Method, route.Path, strings.NewReader("{}"))
			request.Header.Set("x-token", token)
			writer := httptest.NewRecorder()
			engine.ServeHTTP(writer, request)
			require.Equal(t, 401, writer.Code)
		}
	}
	request := httptest.NewRequest("POST", "/v1/admin/payout/bank-reconciliation/import", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	request.Header.Set("x-token", admin)
	writer := httptest.NewRecorder()
	engine.ServeHTTP(writer, request)
	require.Equal(t, 413, writer.Code)
	require.Equal(t, "no-store", writer.Header().Get("Cache-Control"))
	// Real TCP is required to exercise net/http read deadlines. Two slow bodies
	// exhaust bounded capacity; a third request must reject without allocating CSV.
	slow := gin.New()
	started := make(chan struct{}, 4)
	slow.Use(func(c *gin.Context) {
		c.Request.Body = &observedBody{ReadCloser: c.Request.Body, started: started}
		c.Next()
	})
	router.RouterGroup{}.InitBankReconcileRouter(slow.Group("/v1/admin"))
	server := httptest.NewServer(slow)
	defer server.Close()
	var conns []net.Conn
	for i := 0; i < 2; i++ {
		conn, e := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		require.NoError(t, e)
		conns = append(conns, conn)
		defer conn.Close()
		_, e = fmt.Fprintf(conn, "POST /v1/admin/payout/bank-reconciliation/import HTTP/1.1\r\nHost: localhost\r\nx-token: %s\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{", admin)
		require.NoError(t, e)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("slow body did not reach bounded reader")
		}
	}
	third, e := http.NewRequest("POST", server.URL+"/v1/admin/payout/bank-reconciliation/import", strings.NewReader("{}"))
	require.NoError(t, e)
	third.Header.Set("x-token", admin)
	third.Header.Set("Content-Type", "application/json")
	response, e := server.Client().Do(third)
	require.NoError(t, e)
	require.Equal(t, 429, response.StatusCode)
	response.Body.Close()
	require.NoError(t, conns[0].SetReadDeadline(time.Now().Add(7*time.Second)))
	line, e := bufio.NewReader(conns[0]).ReadString('\n')
	require.NoError(t, e)
	require.Contains(t, line, "HTTP/1.1")

}
