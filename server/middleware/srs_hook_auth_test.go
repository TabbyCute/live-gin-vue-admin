package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testSRSHookToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newSRSHookAuthTestRouter(t testing.TB, token string, called *atomic.Int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authenticator, err := NewSRSHookAuthenticator(token)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/hook", authenticator.Handler(), func(c *gin.Context) {
		called.Add(1)
		c.Status(http.StatusNoContent)
	})
	return router
}

func TestNewSRSHookAuthenticatorRejectsUnsafeConfiguration(t *testing.T) {
	for name, token := range map[string]string{
		"empty":      "",
		"too short":  strings.Repeat("a", minSRSHookTokenBytes-1),
		"too long":   strings.Repeat("a", maxSRSHookTokenBytes+1),
		"whitespace": strings.Repeat("a", minSRSHookTokenBytes) + " ",
		"query char": strings.Repeat("a", minSRSHookTokenBytes) + "+",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewSRSHookAuthenticator(token)
			require.ErrorIs(t, err, ErrSRSHookTokenInvalid)
		})
	}
}

func TestSRSHookAuthAcceptsExactlyOneValidCredential(t *testing.T) {
	var called atomic.Int64
	router := newSRSHookAuthTestRouter(t, testSRSHookToken, &called)

	headerRecorder := httptest.NewRecorder()
	headerRequest := httptest.NewRequest(http.MethodPost, "/hook", nil)
	headerRequest.Header.Set(SRSHookTokenHeader, testSRSHookToken)
	router.ServeHTTP(headerRecorder, headerRequest)
	require.Equal(t, http.StatusNoContent, headerRecorder.Code)

	queryRecorder := httptest.NewRecorder()
	queryRequest := httptest.NewRequest(http.MethodPost, "/hook?"+SRSHookTokenQuery+"="+testSRSHookToken, nil)
	router.ServeHTTP(queryRecorder, queryRequest)
	require.Equal(t, http.StatusNoContent, queryRecorder.Code)
	require.EqualValues(t, 2, called.Load())
}

func TestSRSHookAuthRemovesCredentialBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator, err := NewSRSHookAuthenticator(testSRSHookToken)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/hook", authenticator.Handler(), func(c *gin.Context) {
		require.Empty(t, c.GetHeader(SRSHookTokenHeader))
		require.NotContains(t, c.Request.URL.RawQuery, testSRSHookToken)
		require.NotContains(t, c.Request.RequestURI, testSRSHookToken)
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hook?hook_token="+testSRSHookToken, nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestSRSHookAuthRejectsMissingWrongAndAmbiguousCredentials(t *testing.T) {
	var called atomic.Int64
	router := newSRSHookAuthTestRouter(t, testSRSHookToken, &called)

	tests := []struct {
		name    string
		target  string
		headers []string
	}{
		{name: "missing", target: "/hook"},
		{name: "wrong", target: "/hook?hook_token=" + strings.Repeat("f", 64)},
		{name: "short", target: "/hook?hook_token=short"},
		{name: "duplicate query", target: "/hook?hook_token=" + testSRSHookToken + "&hook_token=" + testSRSHookToken},
		{name: "header and query", target: "/hook?hook_token=" + testSRSHookToken, headers: []string{testSRSHookToken}},
		{name: "duplicate header", target: "/hook", headers: []string{testSRSHookToken, testSRSHookToken}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tt.target, nil)
			for _, value := range tt.headers {
				request.Header.Add(SRSHookTokenHeader, value)
			}
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.Contains(t, recorder.Body.String(), srsHookAuthFailedError)
		})
	}
	require.Zero(t, called.Load())
}

type countedBody struct {
	reads atomic.Int64
}

func (b *countedBody) Read(_ []byte) (int, error) {
	b.reads.Add(1)
	return 0, io.EOF
}

func (b *countedBody) Close() error { return nil }

func TestSRSHookAuthRejectsBeforeReadingBody(t *testing.T) {
	var called atomic.Int64
	router := newSRSHookAuthTestRouter(t, testSRSHookToken, &called)
	body := &countedBody{}
	request := httptest.NewRequest(http.MethodPost, "/hook", nil)
	request.Body = body
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Zero(t, body.reads.Load())
	require.Zero(t, called.Load())
}

func TestSRSHookAuthIsSafeUnderConcurrentCallbacks(t *testing.T) {
	var called atomic.Int64
	router := newSRSHookAuthTestRouter(t, testSRSHookToken, &called)
	const callbacks = 1000
	var wait sync.WaitGroup
	failures := make(chan int, callbacks)
	wait.Add(callbacks)
	for i := 0; i < callbacks; i++ {
		go func(index int) {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodPost, "/hook?hook_token="+testSRSHookToken, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNoContent {
				failures <- index
			}
		}(i)
	}
	wait.Wait()
	close(failures)
	require.Empty(t, failures)
	require.EqualValues(t, callbacks, called.Load())
}

func BenchmarkSRSHookAuth(b *testing.B) {
	var called atomic.Int64
	router := newSRSHookAuthTestRouter(b, testSRSHookToken, &called)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := httptest.NewRequest(http.MethodPost, "/hook", nil)
		request.Header.Set(SRSHookTokenHeader, testSRSHookToken)
		router.ServeHTTP(httptest.NewRecorder(), request)
	}
}
