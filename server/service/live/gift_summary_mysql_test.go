package live

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	model "tb_live_module/model/gift"
)

func TestMySQLGiftSummaryExactAmountsAndScope(t *testing.T) {
	raw := os.Getenv("GIFT_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("dedicated local MySQL not configured")
	}
	cfg, err := driver.ParseDSN(raw)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:13327", cfg.Addr)
	require.True(t, strings.HasPrefix(cfg.DBName, "gift_test_"))
	control, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := control.DB()
	require.NoError(t, err)
	defer pool.Close()
	// Separate database: the gift package rebuilds its own fixture concurrently.
	name := fmt.Sprintf("gift_test_summary_%d", time.Now().UnixNano())
	require.NoError(t, control.Exec("CREATE DATABASE `"+name+"`").Error)
	defer func() { require.NoError(t, control.Exec("DROP DATABASE `"+name+"`").Error) }()
	cfg.DBName = name
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	p, err := db.DB()
	require.NoError(t, err)
	defer p.Close()
	require.NoError(t, db.AutoMigrate(&model.Order{}))
	old := global.GiftFactsReady.Swap(true)
	defer global.GiftFactsReady.Store(old)
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("summary_%d", i)
		o := model.Order{OrderNo: key, RequestID: key, SenderUserID: uint64(i), SessionID: 1, UnitPrice: math.MaxInt64, Quantity: 1, GrossCoinAmount: math.MaxInt64, AnchorShareBPS: 10000, WalletTransactionNo: key, WalletLedgerNo: key, Status: "succeeded"}
		if i == 3 {
			o.SessionID = 2
			o.UnitPrice = 7
			o.Quantity = 3
			o.GrossCoinAmount = 21
			o.RefundedCoinAmount = 7
			o.Status = "partial_refund"
		}
		require.NoError(t, db.Create(&o).Error)
	}
	rows, err := giftSummaries(db, "session_id", []uint{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, "18446744073709551614", rows[1].GrossCoin)
	require.Equal(t, rows[1].GrossCoin, rows[1].NetCoin)
	require.Equal(t, rows[1].GrossCoin, rows[1].PendingEarningCoin)
	require.Equal(t, "2", rows[1].GrossQuantity)
	require.Equal(t, "3", rows[2].GrossQuantity)
	require.Equal(t, "1", rows[2].RefundQuantity)
	require.Equal(t, "2", rows[2].Quantity)
	require.Equal(t, "14", rows[2].NetCoin)
	require.Equal(t, "0", rows[3].GrossQuantity)
	var plan []struct {
		PossibleKeys string `gorm:"column:possible_keys"`
	}
	require.NoError(t, db.Raw("EXPLAIN SELECT session_id, SUM(quantity), SUM(gross_coin_amount) FROM gift_order WHERE session_id IN (1,2) GROUP BY session_id").Scan(&plan).Error)
	require.NotEmpty(t, plan)
	require.Contains(t, plan[0].PossibleKeys, "session_id")
}
