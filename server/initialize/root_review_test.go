package initialize

import (
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestRootReviewTriggerUpgrade(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/upgrade.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	for _, module := range []struct {
		migrate  func(*gorm.DB) error
		triggers func(*gorm.DB) ([]giftTrigger, error)
	}{{MigrateGift, giftTriggers}, {MigrateEarning, earningTriggers}} {
		require.NoError(t, module.migrate(db))
		triggers, err := module.triggers(db)
		require.NoError(t, err)
		upgraded := 0
		for _, tr := range triggers {
			if !strings.HasSuffix(tr.name, "_root_v2") {
				continue
			}
			upgraded++
			column, legacyCondition := "approved_by", "NEW.approved_by = OLD.created_by"
			if tr.table == "anchor_rule_termination" {
				column, legacyCondition = "reviewed_by", "NEW.reviewed_by = OLD.created_by"
			}
			if strings.HasPrefix(tr.table, "anchor_settlement") {
				legacyCondition = "NEW.approved_by=OLD.created_by"
			}
			legacyBody := strings.ReplaceAll(tr.body, rootSelfReviewCondition(column), legacyCondition)
			legacyName := strings.TrimSuffix(tr.name, "_root_v2")
			require.NoError(t, db.Exec("DROP TRIGGER "+tr.name).Error)
			require.NoError(t, db.Exec("CREATE TRIGGER "+legacyName+" BEFORE UPDATE ON "+tr.table+" "+legacyBody).Error)
		}
		require.Equal(t, 2, upgraded)
		require.NoError(t, module.migrate(db))
		require.NoError(t, module.migrate(db))
		for _, tr := range triggers {
			if !strings.HasSuffix(tr.name, "_root_v2") {
				continue
			}
			var count int64
			require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?", strings.TrimSuffix(tr.name, "_root_v2")).Scan(&count).Error)
			require.Zero(t, count)
		}
	}
}
