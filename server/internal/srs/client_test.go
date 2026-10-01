package srs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"tb_live_module/config"

	"github.com/stretchr/testify/require"
)

func TestStopPublisherByStreamID(t *testing.T) {
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "admin", username)
		require.Equal(t, "secret", password)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams/stream-1":
			_, _ = w.Write([]byte(`{"code":0,"stream":{"vhost":"push.example.com","app":"live","name":"room-1","publish":{"active":true,"cid":107}}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/clients/107":
			deleted.Add(1)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{
		HTTPAPIs: server.URL, APIUsername: "admin", APIPassword: "secret", APIRequestTimeoutSeconds: 1,
	})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{
		StreamID: "stream-1", Vhost: "push.example.com", App: "live", Stream: "room-1",
	}))
	require.Equal(t, int32(1), deleted.Load())
}

func TestStopPublisherFallsBackToStableStreamIdentity(t *testing.T) {
	var listCalls, deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/api/v1/streams/stale-id":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/api/v1/streams":
			listCalls.Add(1)
			require.Equal(t, "100", r.URL.Query().Get("count"))
			_, _ = w.Write([]byte(`{"code":0,"streams":[{"vhost":"__defaultVhost__","app":"live","name":"room-1","publish":{"active":true,"cid":"108"}}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/proxy/api/v1/clients/108":
			deleted.Add(1)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL + "/proxy/api/v1"})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{
		StreamID: "stale-id", Vhost: "push.example.com", App: "live", Stream: "room-1",
	}))
	require.Equal(t, int32(1), listCalls.Load())
	require.Equal(t, int32(1), deleted.Load())
}

func TestStopPublisherTreatsMissingStreamAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"streams":[]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{App: "live", Stream: "missing"}))
}

func TestStopPublisherRejectsSRSBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1061}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	err = client.StopPublisher(context.Background(), StreamRef{App: "live", Stream: "room-1"})
	require.ErrorContains(t, err, "1061")
}

func TestStopPublisherHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":0,"streams":[]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL, APIRequestTimeoutSeconds: 1})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = client.StopPublisher(ctx, StreamRef{App: "live", Stream: "room-1"})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestStopPublisherRejectsMalformedSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"streams":[]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	err = client.StopPublisher(context.Background(), StreamRef{App: "live", Stream: "room-1"})
	require.ErrorContains(t, err, "缺少code")
}
