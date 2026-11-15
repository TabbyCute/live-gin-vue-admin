package middleware

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOperationLogRedactsSecrets(t *testing.T) {
	input := []byte(`{"password":"old-value","newPassword":"new-value","config":{"jwt":{"signing-key":"jwt-value"},"redis":{"password":"redis-value"}},"items":[{"x-token":"token-value"}],"username":"admin"}`)
	output := redactOperationPayload(input)
	for _, secret := range []string{"old-value", "new-value", "jwt-value", "redis-value", "token-value"} {
		if strings.Contains(output, secret) {
			t.Fatalf("secret was logged: %s", secret)
		}
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["username"] != "admin" {
		t.Fatal("non-sensitive audit data was removed")
	}
	if strings.Contains(redactOperationPayload([]byte("password=plain")), "plain") {
		t.Fatal("raw payload was logged")
	}
}

func TestOperationResponseCaptureIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	writer := responseBodyWriter{ResponseWriter: ctx.Writer, body: new(bytes.Buffer)}
	chunk := []byte(strings.Repeat("a", bufferSize))
	for i := 0; i < 4; i++ {
		if n, err := writer.Write(chunk); err != nil || n != len(chunk) {
			t.Fatalf("response write: %d %v", n, err)
		}
	}
	if writer.body.Len() != bufferSize+1 {
		t.Fatal("log buffer was not capped")
	}
	if recorder.Body.Len() != 4*bufferSize {
		t.Fatal("client response was truncated")
	}
}
