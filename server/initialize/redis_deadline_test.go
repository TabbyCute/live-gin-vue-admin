package initialize

import (
	"context"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"tb_live_module/config"
	"tb_live_module/global"
	"testing"
	"time"
)

// 真实TCP代理吞掉响应，验证deadline确实进入socket层，而不只是在hook里检查ctx。
func TestRedisClientHonorsSocketDeadline(t *testing.T) {
	addr := os.Getenv("SOCIAL_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("requires isolated Redis")
	}
	require.Equal(t, "127.0.0.1:16389", addr)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var stalled atomic.Bool
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			client, e := listener.Accept()
			if e != nil {
				return
			}
			upstream, e := net.DialTimeout("tcp", addr, time.Second)
			if e != nil {
				client.Close()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer client.Close()
				defer upstream.Close()
				copied := make(chan struct{})
				go func() { defer close(copied); io.Copy(upstream, client); upstream.Close() }()
				buf := make([]byte, 8192)
				for {
					n, e := upstream.Read(buf)
					if e != nil {
						break
					}
					if stalled.Load() {
						<-done
						break
					}
					if _, e = client.Write(buf[:n]); e != nil {
						break
					}
				}
				client.Close()
				upstream.Close()
				<-copied
			}()
		}
	}()
	t.Cleanup(func() { close(done); listener.Close(); wg.Wait() })
	previous := global.GVA_LOG
	global.GVA_LOG = zap.NewNop()
	t.Cleanup(func() { global.GVA_LOG = previous })
	client, err := initRedisClient(config.Redis{Addr: listener.Addr().String()})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	stalled.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = client.Ping(ctx).Err()
	elapsed := time.Since(start)
	require.Error(t, err)
	require.Less(t, elapsed, 500*time.Millisecond, "不能退回驱动默认3秒socket超时")
	t.Logf("stalled TCP response: deadline=80ms elapsed=%s", elapsed)
}
