package srs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/vhosts":
			require.Equal(t, "100", r.URL.Query().Get("count"))
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-custom","name":"push.example.com"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams/stream-1":
			_, _ = w.Write([]byte(`{"code":0,"stream":{"vhost":"vid-custom","app":"live","name":"room-1","publish":{"active":true,"cid":"8o4s9m8r"}}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/clients/8o4s9m8r":
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

func TestStopPublishersUsesOneSnapshotAndReturnsAlignedResults(t *testing.T) {
	var listCalls, deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-default","name":"__defaultVhost__"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams":
			listCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"streams":[` +
				`{"id":"stream-1","vhost":"vid-default","app":"live","name":"room-1","publish":{"active":true,"cid":"client-1"}},` +
				`{"id":"stream-2","vhost":"vid-default","app":"live","name":"room-2","publish":{"active":true,"cid":"client-2"}}]}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/clients/client-"):
			deleted.Add(1)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	results := client.StopPublishers(context.Background(), []StreamRef{
		{StreamID: "stream-1", Vhost: "__defaultVhost__", App: "live", Stream: "room-1"},
		{StreamID: "stale", Vhost: "__defaultVhost__", App: "live", Stream: "room-2"},
		{StreamID: "missing", Vhost: "__defaultVhost__", App: "live", Stream: "room-3"},
	}, 2)
	require.Len(t, results, 3)
	for _, result := range results {
		require.NoError(t, result)
	}
	require.Equal(t, int32(1), listCalls.Load(), "批量停止不能退化为逐场查询SRS")
	require.Equal(t, int32(2), deleted.Load())
}

func TestListPublishersReturnsLogicalVhostAndMediaInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-default","name":"__defaultVhost__"}]}`))
		case "/api/v1/streams":
			_, _ = w.Write([]byte(`{"code":0,"streams":[{"id":"stream-1","vhost":"vid-default","app":"TB_LIVE","name":"5100008","publish":{"active":true,"cid":"8o4s9m8r"},"video":{"codec":"H264","profile":"High","level":"3.2","width":1280,"height":720},"audio":{"codec":"AAC","sample_rate":48000,"channel":2,"profile":"LC"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	snapshots, err := client.ListPublishers(context.Background())
	require.NoError(t, err)
	require.Equal(t, []PublisherSnapshot{{
		StreamID: "stream-1", Vhost: "__defaultVhost__", App: "TB_LIVE", Stream: "5100008", Active: true,
		VideoCodec: "H264", VideoProfile: "High", VideoLevel: "3.2", Width: 1280, Height: 720,
		AudioCodec: "AAC", AudioProfile: "LC", AudioSampleRate: 48000, AudioChannels: 2,
	}}, snapshots)
}

func TestListPublishersWithRuntimeRejectsRestartDuringSnapshot(t *testing.T) {
	var versionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/versions":
			serverID := "srs-before"
			if versionCalls.Add(1) > 1 {
				serverID = "srs-after"
			}
			_, _ = fmt.Fprintf(w, `{"code":0,"server":%q,"service":"service-1"}`, serverID)
		case "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"server":"srs-before","vhosts":[]}`))
		case "/api/v1/streams":
			_, _ = w.Write([]byte(`{"code":0,"server":"srs-before","streams":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	_, err = client.ListPublishersWithRuntime(context.Background())
	require.ErrorContains(t, err, "快照期间重启")
}

func TestCurrentRuntimeRequiresServerID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"version":"7"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	_, err = client.CurrentRuntime(context.Background())
	require.ErrorContains(t, err, "缺少server")
}

func TestListPublishersUsesConfiguredCursorPages(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[]}`))
		case "/api/v1/streams":
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			count, _ := strconv.Atoi(r.URL.Query().Get("count"))
			starts = append(starts, fmt.Sprintf("%d:%d", start, count))
			items := make([]map[string]interface{}, 0, count)
			for index := start; index < min(start+count, 7); index++ {
				items = append(items, map[string]interface{}{
					"id": fmt.Sprintf("stream-%d", index), "name": fmt.Sprintf("room-%d", index),
					"publish": map[string]interface{}{"active": true},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "streams": items})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL, SnapshotPageSize: 3, SnapshotMaxStreams: 10})
	require.NoError(t, err)
	snapshots, err := client.ListPublishers(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshots, 7)
	require.Equal(t, []string{"0:3", "3:3", "6:3"}, starts)
}

func TestListPublishersRejectsTruncatedSnapshotAtConfiguredCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[]}`))
		case "/api/v1/streams":
			count, _ := strconv.Atoi(r.URL.Query().Get("count"))
			items := make([]map[string]interface{}, 0, count)
			for index := 0; index < count; index++ {
				items = append(items, map[string]interface{}{"id": fmt.Sprintf("stream-%d", index)})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "streams": items})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL, SnapshotPageSize: 2, SnapshotMaxStreams: 3})
	require.NoError(t, err)
	_, err = client.ListPublishers(context.Background())
	require.ErrorContains(t, err, "超过配置上限3")
}

func TestStopPublisherFallsBackToStableStreamIdentity(t *testing.T) {
	var listCalls, deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-default","name":"__defaultVhost__"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/api/v1/streams/stale-id":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/api/v1/streams":
			listCalls.Add(1)
			require.Equal(t, "100", r.URL.Query().Get("count"))
			_, _ = w.Write([]byte(`{"code":0,"streams":[{"vhost":"vid-default","app":"live","name":"room-1","publish":{"active":true,"cid":"108"}}]}`))
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

func TestStopPublisherFallsBackWhenSRS7ReturnsStreamNotFoundCode(t *testing.T) {
	var listCalls, deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-default","name":"__defaultVhost__"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams/stale-id":
			// SRS 7 uses HTTP 200 plus code 2048 instead of HTTP 404.
			_, _ = w.Write([]byte(`{"code":2048}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams":
			listCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"streams":[{"vhost":"vid-default","app":"live","name":"room-1","publish":{"active":true,"cid":"client-1"}}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/clients/client-1":
			deleted.Add(1)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{
		StreamID: "stale-id", Vhost: "__defaultVhost__", App: "live", Stream: "room-1",
	}))
	require.Equal(t, int32(1), listCalls.Load())
	require.Equal(t, int32(1), deleted.Load())
}

func TestStopPublisherTreatsSRS7MissingClientAsSuccess(t *testing.T) {
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams/stream-1":
			_, _ = w.Write([]byte(`{"code":0,"stream":{"app":"live","name":"room-1","publish":{"active":true,"cid":"gone-client"}}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/clients/gone-client":
			deleted.Add(1)
			// The publisher may disappear between the stream lookup and DELETE.
			_, _ = w.Write([]byte(`{"code":2049}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{
		StreamID: "stream-1", App: "live", Stream: "room-1",
	}))
	require.Equal(t, int32(1), deleted.Load())
}

func TestStopPublisherDoesNotMatchDifferentNamedVhost(t *testing.T) {
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/vhosts":
			_, _ = w.Write([]byte(`{"code":0,"vhosts":[{"id":"vid-one","name":"one.example.com"},{"id":"vid-two","name":"two.example.com"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams":
			_, _ = w.Write([]byte(`{"code":0,"streams":[{"vhost":"vid-two","app":"live","name":"room-1","publish":{"active":true,"cid":109}}]}`))
		case r.Method == http.MethodDelete:
			deleted.Add(1)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(config.SRS{HTTPAPIs: server.URL})
	require.NoError(t, err)
	require.NoError(t, client.StopPublisher(context.Background(), StreamRef{
		Vhost: "one.example.com", App: "live", Stream: "room-1",
	}))
	require.Zero(t, deleted.Load())
}

func TestStreamMatchesLegacyVhostName(t *testing.T) {
	stream := streamItem{VhostID: "push.example.com", App: "live", Name: "room-1"}
	require.True(t, streamMatches(stream, StreamRef{
		Vhost: "PUSH.EXAMPLE.COM", App: "live", Stream: "room-1",
	}, nil))
}

func TestParseCIDSupportsCurrentAndLegacySRSFormats(t *testing.T) {
	tests := []struct {
		name     string
		raw      json.RawMessage
		expected string
		wantErr  bool
	}{
		{name: "SRS 7字符串ID", raw: json.RawMessage(`"8o4s9m8r"`), expected: "8o4s9m8r"},
		{name: "旧版数字ID", raw: json.RawMessage(`107`), expected: "107"},
		{name: "空ID", raw: json.RawMessage(`""`), wantErr: true},
		{name: "非法类型", raw: json.RawMessage(`true`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := parseCID(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
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
