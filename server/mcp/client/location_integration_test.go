package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type quietMCPLogger struct{}

func (quietMCPLogger) Infof(string, ...any)  {}
func (quietMCPLogger) Errorf(string, ...any) {}

// Exercise the real installed mcp-go transport, not a manually imitated Do call.
// TLS trust is injected only into this local httptest transport; globals and
// production certificate validation are never changed.
func TestRealMCPTransportRejectsMaliciousLocation(t *testing.T) {
	for _, targetKind := range []string{"other_host", "same_host_other_port", "https_to_http"} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			for _, operation := range []string{"request", "notification", "close"} {
				t.Run(fmt.Sprintf("%s/%d/%s", targetKind, status, operation), func(t *testing.T) {
					var foreignRequests, foreignCredentials, originRequests atomic.Int32
					foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						foreignRequests.Add(1)
						if r.Header.Get("Authorization") != "" || r.Header.Get("X-Token") != "" || r.Header.Get("Mcp-Session-Id") != "" {
							foreignCredentials.Add(1)
						}
						w.WriteHeader(http.StatusNoContent)
					}))
					defer foreign.Close()
					location := foreign.URL + "/stolen"
					if targetKind == "other_host" {
						location = strings.Replace(location, "127.0.0.1", "localhost", 1)
					}
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						originRequests.Add(1)
						if r.Method == http.MethodPost {
							require.Equal(t, "Bearer local-only-authorization", r.Header.Get("Authorization"))
							require.Equal(t, "local-only-admin-token", r.Header.Get("X-Token"))
						}
						require.Equal(t, "local-only-session", r.Header.Get("Mcp-Session-Id"))
						http.Redirect(w, r, location, status)
					})
					var origin *httptest.Server
					if targetKind == "https_to_http" {
						origin = httptest.NewTLSServer(handler)
					} else {
						origin = httptest.NewServer(handler)
					}
					defer origin.Close()
					bound, err := originBoundClient(origin.URL)
					require.NoError(t, err)
					if targetKind == "https_to_http" {
						guard := bound.Transport.(originTransport)
						guard.next = origin.Client().Transport
						bound.Transport = guard
					}
					tr, err := transport.NewStreamableHTTP(origin.URL, transport.WithHTTPBasicClient(bound), transport.WithHTTPHeaders(map[string]string{"Authorization": "Bearer local-only-authorization", "X-Token": "local-only-admin-token"}), transport.WithSession("local-only-session"), transport.WithHTTPLogger(quietMCPLogger{}))
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					require.NoError(t, tr.Start(ctx))
					switch operation {
					case "request":
						_, err = tr.SendRequest(ctx, transport.JSONRPCRequest{JSONRPC: "2.0", ID: mcp.NewRequestId(1), Method: "ping"})
						require.Error(t, err)
					case "notification":
						err = tr.SendNotification(ctx, mcp.JSONRPCNotification{JSONRPC: "2.0", Notification: mcp.Notification{Method: "notifications/initialized"}})
						require.Error(t, err)
					case "close":
						require.NoError(t, tr.Close())
					}
					require.NoError(t, tr.Close())
					require.Positive(t, originRequests.Load())
					require.Zero(t, foreignRequests.Load())
					require.Zero(t, foreignCredentials.Load())
				})
			}
		}
	}
}

func TestMCPConstructorRejectsRedirectButNormalRPCStillWorks(t *testing.T) {
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { foreignRequests.Add(1); w.WriteHeader(204) }))
	defer foreign.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, foreign.URL, 307) }))
	defer redirect.Close()
	c, err := NewClient(redirect.URL, "fixture-client", "1", "fixture-server", map[string]string{"X-Token": "local-only-token"})
	require.Error(t, err)
	require.Nil(t, c)
	require.Zero(t, foreignRequests.Load())
	var initialized, notifications, tools, closed atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A Location header on a normal response does not retarget the next RPC.
		w.Header().Set("Location", foreign.URL)
		if r.Method == http.MethodDelete {
			closed.Add(1)
			w.WriteHeader(204)
			return
		}
		require.Equal(t, "local-only-token", r.Header.Get("X-Token"))
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch request.Method {
		case "initialize":
			initialized.Add(1)
			w.Header().Set("Mcp-Session-Id", "local-only-session")
			result = map[string]any{"protocolVersion": mcp.LATEST_PROTOCOL_VERSION, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "fixture-server", "version": "1"}}
		case "notifications/initialized":
			notifications.Add(1)
			w.WriteHeader(202)
			return
		case "tools/list":
			tools.Add(1)
			result = map[string]any{"tools": []any{}}
		default:
			t.Errorf("unexpected method %s", request.Method)
			w.WriteHeader(400)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}))
	}))
	defer origin.Close()
	c, err = NewClient(origin.URL, "fixture-client", "1", "fixture-server", map[string]string{"X-Token": "local-only-token"})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.Empty(t, list.Tools)
	require.NoError(t, c.Close())
	require.EqualValues(t, 1, initialized.Load())
	require.EqualValues(t, 1, notifications.Load())
	require.EqualValues(t, 1, tools.Load())
	require.EqualValues(t, 1, closed.Load())
	require.Zero(t, foreignRequests.Load())
}

// Defense in depth: even if a future library replaces the redirect policy or
// consumes Location itself, the final RoundTripper rejects before network I/O.
func TestMCPOriginGuardWorksWithoutRedirectPolicy(t *testing.T) {
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { foreignRequests.Add(1); w.WriteHeader(204) }))
	defer foreign.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, foreign.URL, 307) }))
	defer origin.Close()
	bound, err := originBoundClient(origin.URL)
	require.NoError(t, err)
	bound.CheckRedirect = nil
	request, err := http.NewRequest(http.MethodPost, origin.URL, nil)
	require.NoError(t, err)
	request.Header.Set("X-Token", "local-only-token")
	_, err = bound.Do(request)
	require.ErrorContains(t, err, "cross-origin request blocked")
	require.Zero(t, foreignRequests.Load())
	var dispatched atomic.Int32
	guard := originTransport{scheme: "https", host: "localhost:12345", next: testRoundTripper(func(*http.Request) (*http.Response, error) { dispatched.Add(1); return nil, nil })}
	for _, endpoint := range []string{"http://localhost:12345/mcp", "https://localhost:12346/mcp", "https://other.invalid:12345/mcp", "https://user:password@localhost:12345/mcp"} {
		u, err := url.Parse(endpoint)
		require.NoError(t, err)
		_, err = guard.RoundTrip(&http.Request{URL: u})
		require.Error(t, err)
	}
	_, err = guard.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "localhost:12345", Opaque: "//other.invalid/mcp"}})
	require.Error(t, err)
	require.Zero(t, dispatched.Load())
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
