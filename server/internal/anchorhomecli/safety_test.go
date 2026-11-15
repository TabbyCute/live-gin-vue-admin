package anchorhomecli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	am "tb_live_module/model/anchorhome"
	albums "tb_live_module/service/anchorhome"
)

// Runs only after run_test's endpoint/database guard, in its fresh local fixture.
func testExistingSchemaSafety(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	var ddl []string
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("album_ddl_capture", func(tx *gorm.DB) {
		statement := strings.TrimSpace(strings.ToUpper(tx.Statement.SQL.String()))
		for _, prefix := range []string{"CREATE ", "ALTER ", "DROP ", "TRUNCATE "} {
			if strings.HasPrefix(statement, prefix) {
				ddl = append(ddl, statement)
				break
			}
		}
	}))
	defer db.Callback().Raw().Remove("album_ddl_capture")
	create := func(model any) {
		require.NoError(t, db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin").Migrator().CreateTable(model))
	}
	definition := func(table string) string {
		var name, value string
		require.NoError(t, db.Raw("SHOW CREATE TABLE `"+table+"`").Row().Scan(&name, &value))
		return value
	}
	assertRefused := func(t *testing.T, table string) {
		t.Helper()
		before, metadataBefore, accountsBefore, err := snapshot(db)
		require.NoError(t, err)
		original := definition(table)
		ddl = nil
		report, err := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: filepath.Join(t.TempDir(), "private")})
		require.Error(t, err)
		require.Equal(t, "existing_schema_verification", report.Stage)
		require.False(t, report.Migrated || report.MetadataSynced)
		require.Empty(t, report.BackupFile)
		require.Empty(t, ddl, "CLI preflight must reject before any DDL")
		// Direct service migration has the same guarantee; CLI preflight cannot be
		// the only thing preventing AutoMigrate from changing existing objects.
		require.Error(t, albums.MigrateAlbumSchema(db))
		require.Empty(t, ddl)
		require.Equal(t, original, definition(table))
		after, metadataAfter, accountsAfter, err := snapshot(db)
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.Equal(t, metadataBefore, metadataAfter)
		require.Equal(t, accountsBefore, accountsAfter)
		for _, name := range newTables {
			require.Equal(t, name == table, db.Migrator().HasTable(name), "must not create missing table before rejecting incompatible existing table")
		}
	}
	for _, testcase := range []struct{ name, alter string }{
		{"narrow_column", "ALTER TABLE anchor_home_photo MODIFY reason VARCHAR(799) NOT NULL DEFAULT ''"},
		{"nullable_column", "ALTER TABLE anchor_home_photo MODIFY reason VARCHAR(800) NULL DEFAULT ''"},
		{"changed_default", "ALTER TABLE anchor_home_photo ALTER version SET DEFAULT 4"},
		{"missing_autoincrement", "ALTER TABLE anchor_home_photo MODIFY id BIGINT UNSIGNED NOT NULL"},
		{"missing_index", "ALTER TABLE anchor_home_photo DROP INDEX idx_anchor_home_moderation"},
		{"missing_check", "ALTER TABLE anchor_home_photo DROP CHECK ck_anchor_home_photo_dimensions"},
		{"prefix_unique_index", "ALTER TABLE anchor_home_photo DROP INDEX uk_anchor_home_photo_no, ADD UNIQUE INDEX uk_anchor_home_photo_no(photo_no(8))"},
		{"different_primary_key", "ALTER TABLE anchor_home_photo DROP PRIMARY KEY, ADD PRIMARY KEY(id,owner_id)"},
		{"unknown_column", "ALTER TABLE anchor_home_photo ADD legacy_secret VARCHAR(16) NOT NULL DEFAULT 'retained'"},
		{"wrong_collation", "ALTER TABLE anchor_home_photo DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			create(&am.Photo{})
			require.NoError(t, db.Create(&am.Photo{PhotoNo: "photo_schema_fixture", OwnerID: 7, AnchorNo: "anchor_local", StorageKey: strings.Repeat("c", 32), Status: "pending", Version: 1, Sort: 1, Width: 1, Height: 1, Bytes: 32, Reason: "retained-private", CreatedAt: 1, UpdatedAt: 1}).Error)
			require.NoError(t, db.Exec(testcase.alter).Error)
			assertRefused(t, "anchor_home_photo")
			require.NoError(t, db.Migrator().DropTable(&am.Photo{}))
		})
	}
	t.Run("nonempty_guardless_audit", func(t *testing.T) {
		create(&am.Audit{})
		require.NoError(t, db.Create(&am.Audit{PhotoNo: "photo_schema_fixture", OwnerID: 7, ActorKind: "owner", ActorID: 7, RequestID: "schema_local_request", Action: "upload", ToStatus: "pending", AlbumVersion: 1, CreatedAt: 1}).Error)
		assertRefused(t, "anchor_home_audit")
		require.NoError(t, db.Migrator().DropTable(&am.Audit{}))
	})
	t.Run("tampered_guard_on_empty_table", func(t *testing.T) {
		create(&am.Command{})
		require.NoError(t, db.Exec("CREATE TRIGGER trg_anchor_home_command_update BEFORE UPDATE ON anchor_home_command FOR EACH ROW SET @unexpected_guard=1").Error)
		assertRefused(t, "anchor_home_command")
		require.NoError(t, db.Migrator().DropTable(&am.Command{}))
	})
	t.Run("valid_partial_can_resume_without_alter", func(t *testing.T) {
		create(&am.Album{})
		create(&am.Audit{})
		require.NoError(t, db.Create(&am.Album{OwnerID: 7, Version: 3}).Error)
		before, _, _, err := snapshot(db)
		require.NoError(t, err)
		ddl = nil
		report, err := Execute(ctx, db, Options{Migrate: true, BackupDir: filepath.Join(t.TempDir(), "private")})
		require.NoError(t, err, "stage=%s cause=%v", report.Stage, errors.Unwrap(err))
		require.Len(t, report.NewTables, 2)
		require.NotEmpty(t, ddl)
		for _, query := range ddl {
			require.True(t, strings.HasPrefix(query, "CREATE TABLE ") || strings.HasPrefix(query, "CREATE TRIGGER ") || strings.HasPrefix(query, "CREATE INDEX "), query)
		}
		after, _, _, err := snapshot(db)
		require.NoError(t, err)
		require.True(t, unchangedFinancial(before, after))
		require.NoError(t, albums.VerifyAlbumSchema(db))
		ddl = nil
		report, err = Execute(ctx, db, Options{Migrate: true, BackupDir: filepath.Join(t.TempDir(), "private")})
		require.NoError(t, err)
		require.Empty(t, report.NewTables)
		require.Empty(t, ddl, "valid idempotent rerun performs no DDL")
		for _, model := range albums.AlbumModels() {
			require.NoError(t, db.Migrator().DropTable(model))
		}
	})
}
