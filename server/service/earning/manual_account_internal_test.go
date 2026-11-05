package earning

import (
	"github.com/stretchr/testify/require"
	"math"
	"math/big"
	em "tb_live_module/model/earning"
	"testing"
)

func TestManualPaidOverflowAndDebtRelease(t *testing.T) {
	for _, a := range []em.Account{{Frozen: 1, Paid: math.MaxInt64}, {Frozen: 0}, {Frozen: 5, Version: math.MaxInt64}, {Frozen: 5, Paid: -1}} {
		_, e := move(a, "withdrawal_paid", 1)
		require.Error(t, e)
	}
	after, e := move(em.Account{Frozen: 10, Debt: 7, Paid: 23}, "withdrawal_release", 10)
	require.NoError(t, e)
	require.EqualValues(t, 3, after.Available)
	require.Zero(t, after.Debt)
	require.Zero(t, after.Frozen)
	require.EqualValues(t, 23, after.Paid)
	after, e = move(em.Account{Frozen: 10, Debt: 13, Paid: 23}, "withdrawal_release", 10)
	require.NoError(t, e)
	require.Zero(t, after.Available)
	require.EqualValues(t, 3, after.Debt)
	require.EqualValues(t, 23, after.Paid)
	after, e = move(em.Account{Available: 2000, Frozen: 5000, Paid: 0}, "withdrawal_paid", 5000)
	require.NoError(t, e)
	require.EqualValues(t, 2000, after.Available)
	require.Zero(t, after.Frozen)
	require.EqualValues(t, 5000, after.Paid)
}
func FuzzManualPaidConservation(f *testing.F) {
	f.Add(int64(2000), int64(5000), int64(0), int64(0), int64(5000), true)
	f.Add(int64(0), int64(1), int64(math.MaxInt64), int64(0), int64(1), true)
	f.Add(int64(0), int64(10), int64(4), int64(7), int64(10), false)
	f.Fuzz(func(t *testing.T, available, frozen, paid, debt, amount int64, pay bool) {
		before := em.Account{Available: available, Frozen: frozen, Paid: paid, Debt: debt}
		kind := "withdrawal_release"
		if pay {
			kind = "withdrawal_paid"
		}
		after, e := move(before, kind, amount)
		if e != nil {
			return
		}
		if after.Available < 0 || after.Frozen < 0 || after.Paid < 0 || after.Debt < 0 || after.Debt > 0 && after.Available > 0 {
			t.Fatal("invalid balance")
		}
		total := func(a em.Account) *big.Int {
			n := big.NewInt(a.Available)
			n.Add(n, big.NewInt(a.Frozen))
			n.Add(n, big.NewInt(a.Paid))
			return n.Sub(n, big.NewInt(a.Debt))
		}
		if total(before).Cmp(total(after)) != 0 {
			t.Fatal("money conservation failed")
		}
		if pay && (after.Paid-before.Paid != amount || before.Frozen-after.Frozen != amount || after.Available != before.Available || after.Debt != before.Debt) {
			t.Fatal("paid posting mismatch")
		}
		if !pay && after.Paid != before.Paid {
			t.Fatal("release changed paid")
		}
	})
}
