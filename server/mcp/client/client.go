package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpClient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func NewClient(baseURL, name, version, serverName string, headers ...map[string]string) (*mcpClient.Client, error) {
	httpClient, err := originBoundClient(baseURL)
	if err != nil {
		return nil, err
	}
	options := []transport.StreamableHTTPCOption{transport.WithHTTPBasicClient(httpClient)}
	if len(headers) > 0 && len(headers[0]) > 0 {
		options = append(options, transport.WithHTTPHeaders(headers[0]))
	}

	client, err := mcpClient.NewStreamableHttpClient(baseURL, options...)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    name,
		Version: version,
	}

	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := client.Initialize(initCtx, initRequest)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	if result.ServerInfo.Name != serverName {
		_ = client.Close()
		return nil, errors.New("server name mismatch")
	}

	return client, nil
}

// GO-2026-6911 has no upstream fixed version at review time. Constrain every
// outbound request, including URLs obtained from MCP Location headers, to the
// configured origin before credentials can leave the process.
type originTransport struct {
	scheme, host string
	next         http.RoundTripper
}

func (t originTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r == nil || r.URL == nil || r.URL.Opaque != "" || r.URL.User != nil || r.URL.Scheme != t.scheme || !strings.EqualFold(r.URL.Host, t.host) {
		return nil, errors.New("MCP cross-origin request blocked")
	}
	return t.next.RoundTrip(r)
}

// NewOriginBoundHTTPClient also protects MCP tool-to-backend calls, which carry
// the caller's administrative token but do not use mcp-go's HTTP transport.
func NewOriginBoundHTTPClient(baseURL string) (*http.Client, error) {
	return originBoundClient(baseURL)
}

func originBoundClient(baseURL string) (*http.Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("invalid MCP endpoint")
	}
	return &http.Client{
		Transport:     originTransport{scheme: u.Scheme, host: u.Host, next: http.DefaultTransport},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}
