package testpay

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSimulatorBoundary(t *testing.T) {
	for _, dsn := range []string{"root@tcp(8.8.8.8:3306)/bank_test_x", "root@tcp(127.0.0.1:3306)/production", "root@unix(/tmp/mysql.sock)/bank_test_x"} {
		_, e := New(dsn)
		require.Error(t, e)
	}
	s, e := New("root@tcp(127.0.0.1:13329)/bank_test_reconcile")
	require.NoError(t, e)
	order := Order{No: "test-order-001", AccountNo: "user001", Amount: 100, Currency: "CNY"}
	_, e = s.Submit(context.Background(), order, true)
	require.ErrorIs(t, e, ErrUnknown)
	pending, e := s.Query(context.Background(), order.No)
	require.NoError(t, e)
	success, e := s.Complete(order.No)
	require.NoError(t, e)
	calls := 0
	apply := func(Order) error { calls++; return nil }
	require.NoError(t, Deliver(s, pending, apply))
	require.Zero(t, calls)
	bad := success
	bad.Signature = "invalid"
	require.ErrorIs(t, Deliver(s, bad, apply), ErrSignature)
	require.Zero(t, calls)
	require.NoError(t, Deliver(s, success, apply))
	require.Equal(t, 1, calls)
	require.NoError(t, Deliver(s, pending, apply))
	require.Equal(t, 1, calls)
	order.Amount++
	_, e = s.Submit(context.Background(), order, false)
	require.ErrorIs(t, e, ErrConflict)
}
