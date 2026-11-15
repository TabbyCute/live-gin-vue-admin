package client

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMCPRejectsRedirectAndLocationCredentialExfiltration(t *testing.T) {
	var leaked atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1); w.WriteHeader(204) }))
	defer foreign.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, foreign.URL, 307) }))
	defer origin.Close()
	c, err := originBoundClient(origin.URL)
	require.NoError(t, err)
	r, _ := http.NewRequest("GET", origin.URL, nil)
	r.Header.Set("Authorization", "test-private-token")
	resp, err := c.Do(r)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 307, resp.StatusCode)
	// A transport may consume Location itself, bypassing CheckRedirect.
	r, _ = http.NewRequest("POST", resp.Header.Get("Location"), nil)
	r.Header.Set("Authorization", "test-private-token")
	_, err = c.Do(r)
	require.ErrorContains(t, err, "cross-origin request blocked")
	require.Zero(t, leaked.Load())
	for _, u := range []string{"file:///etc/passwd", "http://user:secret@example.com", "//example.com"} {
		_, err = originBoundClient(u)
		require.Error(t, err)
	}
}
