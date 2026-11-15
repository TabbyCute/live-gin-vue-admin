package engagementcli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationBackupScopeIsFixedAndComplete(t *testing.T) {
	for _, table := range []string{"wallet_account", "anchor_finance_permission", "payout_attempt", "event_delivery", "event_delivery_retry_audit", "gift_order", "gift_refund_order", "gift_revenue_ledger", "sys_base_menus", "sys_authority_menus", "sys_apis"} {
		require.True(t, migrationBackupTable(table), table)
	}
	for _, table := range []string{"gva_announcements_info", "sys_users", "unrelated", "gift_item", "walletish", "event_delivery_fake_secret"} {
		if table == "event_delivery_fake_secret" {
			// Every future module-prefixed table is included, not silently omitted.
			require.True(t, migrationBackupTable(table))
		} else {
			require.False(t, migrationBackupTable(table), table)
		}
	}
}

func TestScopedTriggerDependenciesFailClosed(t *testing.T) {
	tables := []string{"wallet_account", "anchor_withdrawable_ledger", "payout_order"}
	for _, body := range []string{
		"SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='CALL secret(); UPDATE outside SET x=1'",
		"BEGIN IF NOT (BINARY OLD.`update` <=> BINARY NEW.`update`) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable'; END IF; END",
		"BEGIN IF NOT EXISTS (SELECT 1 FROM `payout_order` o JOIN anchor_withdrawable_ledger l ON l.id=o.id WHERE o.id=NEW.id AND JSON_UNQUOTE(JSON_EXTRACT(o.snapshot,'$.name'))=CAST(COALESCE(NEW.name,'') AS CHAR)) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable'; END IF; END",
		"BEGIN IF (SELECT COUNT(*) FROM wallet_account WHERE id=NEW.id)>0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='quote''s \\\\ text'; END IF; END",
	} {
		require.NoError(t, scopedTriggerDependencies(body, "NO_BACKSLASH_ESCAPES", tables), body)
	}
	for _, body := range []string{
		"CALL external_schema.secret()",
		"SET NEW.id=unknown_function()",
		"SET NEW.id=`COUNT`()",
		"SET NEW.id=other.COUNT()",
		"INSERT INTO unrelated_log(id) VALUES(NEW.id)",
		"SET NEW.id=(SELECT id FROM unrelated)",
		"SET NEW.id=(SELECT id FROM other.wallet_account)",
		"SET NEW.id=(SELECT id FROM wallet_account a, unrelated b)",
		"SET NEW.id=(SELECT a.id FROM wallet_account a JOIN payout_order p ON p.id=a.id, unrelated b)",
		"SET NEW.id=(SELECT id FROM (SELECT id FROM wallet_account) a)",
		"SET NEW.id=1 /*! CALL hidden() */",
		"PREPARE run FROM @sql",
		"SET NEW.id=(TABLE unrelated)",
		"SET NEW.id=私密()",
		"SET NEW.id=secret$COUNT()",
	} {
		require.Error(t, scopedTriggerDependencies(body, "", tables), body)
	}
	require.NoError(t, scopedTriggerDependencies("SET NEW.name=\"CALL external()\"", "", tables))
	require.NoError(t, scopedTriggerDependencies("SET NEW.id=(SELECT id FROM \"wallet_account\" WHERE id=NEW.id)", "ANSI_QUOTES", tables))
}
