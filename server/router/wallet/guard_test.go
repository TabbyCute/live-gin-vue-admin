package wallet

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWalletGuardBoundedConcurrencyAndRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	engine.GET("/", guard(2, time.Second), func(c *gin.Context) { entered <- struct{}{}; <-release; c.Status(204) })
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		}()
	}
	<-entered
	<-entered
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, 503, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.Contains(t, w.Body.String(), `"code":47012`)
	close(release)
	wg.Wait()
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, 204, w.Code)
}

func TestWalletGuardRejectsOversizeBeforeAuth(t *testing.T) {
	engine := gin.New()
	engine.Use(guard(1, time.Second))
	engine.Any("/", func(c *gin.Context) { t.Error("oversize reached authentication/handler") })
	for _, kind := range []string{"query", "body", "app_token", "admin_token"} {
		r := httptest.NewRequest("POST", "/", nil)
		switch kind {
		case "query":
			r.URL.RawQuery = strings.Repeat("x", 2049)
		case "body":
			r.ContentLength = 16385
		case "app_token":
			r.Header.Set("Authorization", strings.Repeat("x", 8193))
		case "admin_token":
			r.Header.Set("x-token", strings.Repeat("x", 8193))
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		require.Equal(t, 400, w.Code, kind)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

func TestWalletGuardSlowBodyReleasesConnection(t *testing.T) {
	// 真实TCP连接，不以context取消代替验证Body.Read真的被打断。
	engine := gin.New()
	engine.POST("/", guard(1, 100*time.Millisecond), func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestTimeout)
			return
		}
		c.Status(204)
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = fmt.Fprint(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 100\r\n\r\n{")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestTimeout, response.StatusCode)
	response.Body.Close()
	// 超时后的槽位被释放，后续正常请求可进入。
	response, err = server.Client().Post(server.URL, "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, 204, response.StatusCode)
	response.Body.Close()
}

func TestWalletGuardRejectsIncompleteOversizeBodyPromptly(t *testing.T) {
	engine := gin.New()
	engine.POST("/", guard(1, 8*time.Second), func(c *gin.Context) { t.Error("oversize reached handler") })
	server := httptest.NewServer(engine)
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	_, err = fmt.Fprint(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 20000\r\n\r\n{")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	response.Body.Close()
}
