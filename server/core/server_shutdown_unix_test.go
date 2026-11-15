//go:build !windows

package core

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"tb_live_module/internal/telemetry"
	"testing"
	"time"
)

// Signal only our child test process, never the desktop session or a running app.
func TestSIGTERMDrainsInFlightHTTP(t *testing.T) {
	if os.Getenv("CORE_SHUTDOWN_HELPER") == "1" {
		r := gin.New()
		r.GET("/ready", func(c *gin.Context) { c.Status(204) })
		r.GET("/slow", func(c *gin.Context) {
			c.Status(200)
			c.Writer.Flush()
			time.Sleep(300 * time.Millisecond)
			_, _ = fmt.Fprintf(c.Writer, "finished draining=%t", telemetry.Draining.Load())
		})
		initServer(os.Getenv("CORE_SHUTDOWN_ADDR"), r, time.Second, time.Second)
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	cmd := exec.Command(os.Args[0], "-test.run=^TestSIGTERMDrainsInFlightHTTP$")
	cmd.Env = append(os.Environ(), "CORE_SHUTDOWN_HELPER=1", "CORE_SHUTDOWN_ADDR="+address)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill() }()
	client := &http.Client{Timeout: 2 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	require.Eventually(t, func() bool {
		response, e := client.Get("http://" + address + "/ready")
		if e != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == 204
	}, 5*time.Second, 20*time.Millisecond)
	response, err := client.Get("http://" + address + "/slow")
	require.NoError(t, err)
	defer response.Body.Close()
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "finished draining=true", string(body))
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err = <-finished:
		require.NoError(t, err, output.String())
	case <-time.After(5 * time.Second):
		t.Fatal("child did not finish graceful shutdown")
	}
}
