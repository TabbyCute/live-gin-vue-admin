package ranking

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPeriodsAndExactNumbers(t *testing.T) {
	at := time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC).UnixMilli()
	for period, want := range map[string]string{"day": "2026-11-02", "week": "2026-11-02", "month": "2026-11", "all": "all"} {
		got, e := PeriodKey(period, at, "Asia/Kuala_Lumpur")
		require.NoError(t, e)
		require.Equal(t, want, got)
	}
	week, e := PeriodKey("week", at-1, "Asia/Kuala_Lumpur")
	require.NoError(t, e)
	require.Equal(t, "2026-10-26", week)
	n, e := addDecimal("9223372036854775807", 9223372036854775807)
	require.NoError(t, e)
	require.Equal(t, "18446744073709551614", n)
	require.Greater(t, sortKey("9007199254740993"), sortKey("9007199254740992"))
	_, e = addDecimal("0", -1)
	require.ErrorIs(t, e, ErrInvariant)
	_, e = addDecimal("01", 1)
	require.ErrorIs(t, e, ErrInvariant)
}
