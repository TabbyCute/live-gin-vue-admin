package payout

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestPaymentReferenceCanonicalization(t *testing.T) {
	a, e := normalizeReference(" bank-0001 \u00a0")
	require.NoError(t, e)
	require.Equal(t, "BANK-0001", a)
	for _, v := range []string{"", "tiny", "550e8400-e29b-41d4-a716-446655440000", "550e8400e29b41d4a716446655440000", "BANK\nREF", "BANK<script>", strings.Repeat("A", 129)} {
		_, e = normalizeReference(v)
		require.Error(t, e)
	}
}
func FuzzManualEvidenceAndReference(f *testing.F) {
	f.Add([]byte("not an image"), "BANK-EXAMPLE")
	f.Fuzz(func(t *testing.T, b []byte, ref string) {
		if len(b) > 3<<20 {
			t.Skip()
		}
		_, _ = validateEvidence("image/png", b)
		v, e := normalizeReference(ref)
		if e == nil {
			again, e := normalizeReference(v)
			require.NoError(t, e)
			require.Equal(t, v, again)
			require.LessOrEqual(t, len(v), 128)
		}
	})
}
