package earning

import (
	"github.com/stretchr/testify/require"
	"math"
	"math/big"
	"testing"
)

func TestShareOverflowAndRounding(t *testing.T) {
	for _, n := range []int64{1, 2, 3, 9999, 10000, 10001, math.MaxInt64} {
		for _, bps := range []int64{0, 1, 3333, 4000, 9999, 10000} {
			want := new(big.Int).Quo(new(big.Int).Mul(big.NewInt(n), big.NewInt(bps)), big.NewInt(10000))
			require.Equal(t, want.Int64(), Share(n, bps))
		}
	}
}
