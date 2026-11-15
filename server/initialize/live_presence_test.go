package initialize

import (
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	model "tb_live_module/model/live"
	"testing"
)

func TestPresenceMigrationRepeatableAndPreservesSession(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, e)
	p, _ := db.DB()
	defer p.Close()
	p.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.LiveSession{}))
	ls := model.LiveSession{SessionNo: "migration_session", RoomId: 1, AnchorId: 1, Status: model.LiveSessionEnded, ViewCount: 123, LikeCount: 456, GiftCoinAmount: 789}
	require.NoError(t, db.Create(&ls).Error)
	require.NoError(t, MigratePresence(db))
	require.NoError(t, MigratePresence(db))
	require.NoError(t, ValidatePresenceSchema(db))
	var after model.LiveSession
	require.NoError(t, db.First(&after, ls.ID).Error)
	require.Equal(t, ls.ViewCount, after.ViewCount)
	require.Equal(t, ls.GiftCoinAmount, after.GiftCoinAmount)
	visit := model.LiveViewerVisit{VisitNo: "unique_visit", SessionNo: ls.SessionNo, RoomNo: "room", VisitorKey: "u:1", Status: "online"}
	require.NoError(t, db.Create(&visit).Error)
	require.Error(t, db.Create(&visit).Error)
}
