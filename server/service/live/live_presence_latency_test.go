package live

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"net"
	"sync"
	req "tb_live_module/model/live/request"
	"testing"
	"time"
)

func TestPresenceRedisBlackholeHasDeadlineAndFailsClosed(t *testing.T) {
	s, _, _ := presenceFixture(t)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	stop := make(chan struct{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	go func() {
		defer close(done)
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() { defer wg.Done(); defer c.Close(); <-stop }()
		}
	}()
	defer func() { close(stop); listener.Close(); <-done; wg.Wait() }()
	client := redis.NewClient(&redis.Options{Addr: listener.Addr().String(), ContextTimeoutEnabled: true, MaxRetries: -1, DialTimeout: 200 * time.Millisecond, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer client.Close()
	s.Redis = client
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	e = s.Rate(ctx, "test", "actor", 10)
	require.ErrorIs(t, e, ErrPresenceUnavailable)
	require.Less(t, time.Since(start), 1500*time.Millisecond)
	_, e = s.Like(ctx, presenceViewer(), req.PresenceLike{PresenceToken: "invalid", RequestID: "request_01", Sequence: "1", Count: "1"})
	require.Error(t, e)
}
