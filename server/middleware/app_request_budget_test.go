package middleware

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAppRequestBudgetCapsChunkedBodyAndPropagatesDeadline(t *testing.T) {
	r := gin.New()
	r.Use(AppRequestBudget())
	r.POST("/", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
		_, err := io.ReadAll(c.Request.Body)
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			c.Status(413)
			return
		}
		require.NoError(t, err)
		c.Status(204)
	})
	for _, length := range []int{64 << 10, (64 << 10) + 1} {
		for _, chunked := range []bool{false, true} {
			q := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", length)))
			if chunked {
				q.ContentLength = -1
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if length == 64<<10 {
				require.Equal(t, 204, w.Code)
			} else {
				require.Equal(t, 413, w.Code)
			}
		}
	}
}

// A recorder cannot reproduce net/http draining an unread request body after
// the handler has returned. Use TCP and leave the declared body unfinished.
func TestAppRequestBudgetSlowBodyAndEarlyRejectionReleaseConnection(t *testing.T) {
	r := gin.New()
	r.Use(AppRequestBudget())
	r.POST("/", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestTimeout)
			return
		}
		c.Status(http.StatusNoContent)
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	for _, scenario := range []struct {
		size, status int
		budget       time.Duration
	}{{65537, 413, 2 * time.Second}, {100, 408, 7 * time.Second}} {
		conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(scenario.budget)))
		_, err = fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n{", scenario.size)
		require.NoError(t, err)
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		require.NoError(t, err)
		require.Equal(t, scenario.status, response.StatusCode)
		response.Body.Close()
		conn.Close()
	}
	response, err := server.Client().Post(server.URL, "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, 204, response.StatusCode)
	response.Body.Close()
}
