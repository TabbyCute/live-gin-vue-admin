package earning_test

import (
	"github.com/stretchr/testify/require"
	"math"
	"math/big"
	es "tb_live_module/service/earning"
	"testing"
)

func TestRoundingIntegerExtremes(t *testing.T) {
	for _, amount := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		delta, rest, e := es.ConvertWithRemainder(amount, 0, math.MaxInt64, math.MaxInt64)
		require.NoError(t, e)
		require.Equal(t, amount, delta)
		require.Zero(t, rest)
	}
	_, _, e := es.ConvertWithRemainder(math.MaxInt64, 0, 1, 2)
	require.Error(t, e)
	_, _, e = es.ConvertWithRemainder(1, 10, 10, 1)
	require.Error(t, e)
	delta, rest, e := es.ConvertWithRemainder(-1, 0, 10, 1)
	require.NoError(t, e)
	require.Equal(t, int64(-1), delta)
	require.Equal(t, int64(9), rest)
}
func FuzzRoundingConservation(f *testing.F) {
	f.Add(int64(4), int64(-4), int64(10), int64(1), int64(0))
	f.Add(int64(math.MaxInt64), int64(-1), int64(math.MaxInt64), int64(math.MaxInt64), int64(0))
	f.Fuzz(func(t *testing.T, a, b, coin, minor, rest int64) {
		if coin <= 0 || minor <= 0 || rest < 0 || rest >= coin {
			t.Skip()
		}
		x, r, e := es.ConvertWithRemainder(a, rest, coin, minor)
		if e != nil {
			return
		}
		y, s, e := es.ConvertWithRemainder(b, r, coin, minor)
		if e != nil {
			return
		}
		left := new(big.Int).Add(big.NewInt(x), big.NewInt(y))
		left.Mul(left, big.NewInt(coin))
		left.Add(left, big.NewInt(s))
		right := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
		right.Mul(right, big.NewInt(minor))
		right.Add(right, big.NewInt(rest))
		if left.Cmp(right) != 0 || s < 0 || s >= coin {
			t.Fatalf("fractional value changed: %s != %s", left, right)
		}
	})
}
