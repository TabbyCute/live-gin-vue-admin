package live

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tb_live_module/config"
	liveModel "tb_live_module/model/live"

	"github.com/stretchr/testify/require"
)

func TestGenerateLivePublishTokenV1(t *testing.T) {
	resolved, err := resolveLivePublishTokenConfig(config.Live{
		PushTokenSeconds: 180,
		PublishTokenKey:  "publish-token-secret",
		SRS: config.SRS{
			App:         "tb_live",
			PushBaseURL: "rtmp://push.xxx.com:1935",
			PlayBaseURL: "https://play.xxx.com",
		},
	})
	require.NoError(t, err)

	now := time.Unix(1790204400, 987_000_000)
	token, err := generateLivePublishToken(resolved, "LS_20260923_00001", "A_1000009", 1, now)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, livePublishTokenPrefix))
	require.Equal(t,
		"rtmp://push.xxx.com:1935/tb_live/A_1000009?pt="+token,
		resolved.streamURL("A_1000009", token),
	)
	require.Equal(t, "https://play.xxx.com/tb_live/A_1000009", resolved.playURL("A_1000009"))

	claims := decryptLivePublishTokenForTest(t, token, "publish-token-secret")
	require.Equal(t, uint8(1), claims.Version)
	require.Equal(t, "live:publish", claims.Scope)
	require.Regexp(t, `^[0-9a-f]{16}$`, claims.TokenID)
	require.Equal(t, "LS_20260923_00001", claims.SessionNo)
	require.Equal(t, "tb_live", claims.App)
	require.Equal(t, "push.xxx.com", claims.Vhost)
	require.Equal(t, "A_1000009", claims.Stream)
	require.Equal(t, uint32(1), claims.CredentialVersion)
	require.Equal(t, int64(1790204400), claims.IssuedAt)
	require.Equal(t, int64(1790204340), claims.NotBefore)
	require.Equal(t, int64(1790204580), claims.ExpiresAt)

	decrypted, err := decryptLivePublishToken(resolved, token)
	require.NoError(t, err)
	require.Equal(t, claims, decrypted)
	require.NoError(t, validateLivePublishTokenClaims(
		resolved, decrypted, "tb_live", "push.xxx.com", "A_1000009",
		"LS_20260923_00001", 1, now,
	))
	require.ErrorIs(t, validateLivePublishTokenClaims(
		resolved, decrypted, "wrong-app", "push.xxx.com", "A_1000009",
		"LS_20260923_00001", 1, now,
	), ErrLivePublishTokenInvalid)
	require.ErrorIs(t, validateLivePublishTokenClaims(
		resolved, decrypted, "tb_live", "push.xxx.com", "A_1000009",
		"LS_20260923_00001", 1, time.Unix(decrypted.ExpiresAt, 0),
	), ErrLivePublishTokenInvalid)

	tamperedLastByte := "A"
	if strings.HasSuffix(token, tamperedLastByte) {
		tamperedLastByte = "B"
	}
	_, err = decryptLivePublishToken(resolved, token[:len(token)-1]+tamperedLastByte)
	require.ErrorIs(t, err, ErrLivePublishTokenInvalid)
}

func TestConfiguredLiveStreamURLDoesNotDuplicateApp(t *testing.T) {
	resolved, err := resolveLivePublishTokenConfig(config.Live{
		PushTokenSeconds: 180,
		PublishTokenKey:  "publish-token-secret",
		SRS: config.SRS{
			App:         "tb_live",
			PushBaseURL: "rtmp://push.xxx.com:1935/tb_live/",
		},
	})
	require.NoError(t, err)
	require.Equal(t,
		"rtmp://push.xxx.com:1935/tb_live/A_1000009?pt=abc",
		resolved.streamURL("A_1000009", "abc"),
	)
}

func TestNextLiveCredentialVersion(t *testing.T) {
	version, err := nextLiveCredentialVersion(liveModel.LiveRoom{StreamKeyVersion: 1})
	require.NoError(t, err)
	require.Equal(t, uint32(1), version)

	version, err = nextLiveCredentialVersion(liveModel.LiveRoom{
		StreamKeyVersion: 1, PublishSecretHash: "already-issued",
	})
	require.NoError(t, err)
	require.Equal(t, uint32(2), version)
}

func decryptLivePublishTokenForTest(t *testing.T, token, publishTokenKey string) livePublishTokenClaims {
	t.Helper()
	require.True(t, strings.HasPrefix(token, livePublishTokenPrefix))
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, livePublishTokenPrefix))
	require.NoError(t, err)
	key := sha256.Sum256([]byte(publishTokenKey))
	block, err := aes.NewCipher(key[:])
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	require.Greater(t, len(sealed), gcm.NonceSize()+gcm.Overhead())
	plainText, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	require.NoError(t, err)
	var claims livePublishTokenClaims
	require.NoError(t, json.Unmarshal(plainText, &claims))
	return claims
}
