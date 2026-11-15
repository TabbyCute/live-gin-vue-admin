package live

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHookURLSecretsAreOmitted(t *testing.T) {
	input := []byte(`{"action":"on_publish","stream":"123","param":"?pt=secret-param","tcUrl":"rtmp://secret-user:secret-pass@host/live?pt=secret-query#secret-fragment","stream_url":"/secret-path/live?token=secret-token","extra":{"credential":"secret-extra"}}`)
	out := string(sanitizeSRSHookLogBody(input))
	require.NotContains(t, out, "secret-")
	require.Contains(t, out, "on_publish")
	require.Contains(t, out, "123")
}
