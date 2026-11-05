package initialize

import (
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
)

func TestManualMigrationRejectsWeakenedLegacyCheck(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	require.NoError(t, db.Exec("CREATE TABLE anchor_withdrawable_account (id INTEGER PRIMARY KEY, available BIGINT, frozen BIGINT, paid BIGINT, debt BIGINT, CONSTRAINT `ck_withdrawable_balances` CHECK (available>=0 AND frozen>=0 AND paid=0 AND debt>=0 AND debt=0 OR available=0))").Error)
	require.ErrorContains(t, prepareManualChecks(db, true), "refuse unknown legacy constraint")
	require.True(t, db.Migrator().HasConstraint("anchor_withdrawable_account", "ck_withdrawable_balances"), "unrecognized protection must not be dropped")
	require.ErrorContains(t, validateNoLegacyManualGuards(db), "legacy financial CHECK")
	require.NotEqual(t, normalizeLegacyCheck("a AND (b OR c)"), normalizeLegacyCheck("a AND b OR c"))
}
