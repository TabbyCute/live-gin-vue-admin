package social

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUTCTimeDriverLocationDoesNotChangeInstant(t *testing.T) {
	shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
	// 数据库保存UTC墙钟2026-10-25 01:02:03，驱动附加Shanghai不能把它误认成前一天。
	var value UTCTime
	require.NoError(t, value.Scan(time.Date(2026, 10, 25, 1, 2, 3, 123000000, shanghai)))
	require.Equal(t, time.Date(2026, 10, 25, 1, 2, 3, 123000000, time.UTC), value.Time)
	sqlValue, err := NewUTCTime(time.Date(2026, 10, 25, 9, 2, 3, 123000000, shanghai)).Value()
	require.NoError(t, err)
	require.Equal(t, "2026-10-25 01:02:03.123", sqlValue)
	require.NoError(t, value.Scan([]byte(sqlValue.(string))))
	require.Equal(t, time.Date(2026, 10, 25, 1, 2, 3, 123000000, time.UTC), value.Time)
}
