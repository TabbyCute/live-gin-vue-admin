package live

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLiveAccountModelsDoNotMarshalSensitiveFields(t *testing.T) {
	phone := "+60123456789"
	accountJSON, err := json.Marshal(LiveAccount{
		Username: "demo", Password: "bcrypt-hash", Phone: &phone, PhoneVerifiedAt: 123,
		StatusReason: "internal reason", TokenVersion: 2,
	})
	require.NoError(t, err)
	require.NotContains(t, string(accountJSON), phone)
	require.NotContains(t, string(accountJSON), "bcrypt-hash")
	require.NotContains(t, string(accountJSON), "internal reason")
	require.NotContains(t, string(accountJSON), "tokenVersion")

	realNameJSON, err := json.Marshal(LiveAccountRealName{
		LegalNameCiphertext: "encrypted-name", DocumentNoCiphertext: "encrypted-document",
		DocumentNoHash: []byte("secret-hmac"), ProviderReference: &phone,
	})
	require.NoError(t, err)
	require.NotContains(t, string(realNameJSON), "encrypted-name")
	require.NotContains(t, string(realNameJSON), "encrypted-document")
	require.NotContains(t, string(realNameJSON), "documentNoHash")
	require.NotContains(t, string(realNameJSON), phone)
}
