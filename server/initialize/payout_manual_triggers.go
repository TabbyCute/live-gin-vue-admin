package initialize

import (
	"gorm.io/gorm"
	"sort"
	"strings"
	pm "tb_live_module/model/payout"
)

func negateConditions(v []string) []string {
	out := make([]string, len(v))
	for i, c := range v {
		out[i] = "NOT (" + c + ")"
	}
	return out
}
func sameSQL(db *gorm.DB, a, b string) string {
	if db.Dialector.Name() == "mysql" {
		return "(BINARY " + a + " <=> BINARY " + b + ")"
	}
	return "(" + a + " IS " + b + ")"
}
func guardBody(db *gorm.DB, conditions []string) string {
	c := strings.Join(conditions, " OR ")
	if c == "" {
		c = "1=1"
	}
	if db.Dialector.Name() == "mysql" {
		return "BEGIN IF " + c + " THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='manual payout immutable invariant'; END IF; END"
	}
	return "WHEN " + c + " BEGIN SELECT RAISE(ABORT,'manual payout immutable invariant'); END"
}
func immutableColumns(db *gorm.DB, m any, allowed string) ([]string, error) {
	st := &gorm.Statement{DB: db}
	if e := st.Parse(m); e != nil {
		return nil, e
	}
	set := map[string]bool{}
	for _, n := range strings.Fields(allowed) {
		set[n] = true
	}
	var out []string
	for _, f := range st.Schema.Fields {
		if f.DBName != "" && !set[f.DBName] {
			out = append(out, "NOT "+sameSQL(db, "OLD.`"+f.DBName+"`", "NEW.`"+f.DBName+"`"))
		}
	}
	return out, nil
}
func withdrawalTerminalGuard(db *gorm.DB) string {
	parts := []string{"OLD.status='approved'", "NEW.status IN ('paid','cancelled')"}
	for _, c := range []string{"reconciliation_status", "reconciliation_no", "reviewed_by", "reviewed_at", "review_reason"} {
		parts = append(parts, sameSQL(db, "OLD."+c, "NEW."+c))
	}
	parts = append(parts, `EXISTS (SELECT 1 FROM payout_terminal_event e JOIN payout_order o ON o.payout_no=e.payout_no JOIN payout_status_history h ON h.event_no=e.transition_no JOIN anchor_withdrawable_ledger l ON l.ledger_no=e.ledger_no WHERE e.withdrawal_no=OLD.withdrawal_no AND o.business_no=OLD.withdrawal_no AND o.amount=OLD.amount AND o.currency=OLD.currency AND e.amount=OLD.amount AND e.currency=OLD.currency AND l.business_no=OLD.withdrawal_no AND l.change_amount=OLD.amount AND l.version=e.account_version AND h.sequence=o.state_version+1 AND h.from_status=o.status AND h.actor_id=e.actor_id AND ((NEW.status='paid' AND e.kind='payout.succeeded' AND h.to_status='succeeded' AND l.entry_type='withdrawal_paid' AND l.frozen_before-l.frozen_after=OLD.amount AND l.paid_after-l.paid_before=OLD.amount) OR (NEW.status='cancelled' AND e.kind='payout.cancelled' AND h.to_status='cancelled' AND l.entry_type='withdrawal_release' AND l.frozen_before-l.frozen_after=OLD.amount AND l.paid_after=l.paid_before)))`)
	return strings.Join(parts, " AND ")
}
func payoutTriggers(db *gorm.DB) ([]giftTrigger, error) {
	old, e := legacyPayoutTriggers(db)
	if e != nil {
		return nil, e
	}
	var all []giftTrigger
	for _, t := range old {
		if t.name == "trg_payout_order_update" || t.name == "trg_payout_order_insert" {
			continue
		}
		all = append(all, t)
	}
	add := func(table, event string, c []string) {
		all = append(all, giftTrigger{"trg_" + table + "_" + strings.ToLower(event) + "_manual_v3", table, event, guardBody(db, c)})
	}
	add("payout_order", "INSERT", []string{"NEW.status<>'ready'", "NEW.state_version<>1", "NEW.last_transition_no<>''", "NEW.updated_at<>NEW.created_at"})
	c, e := immutableColumns(db, &pm.Order{}, "status state_version last_transition_no updated_at")
	if e != nil {
		return nil, e
	}
	c = append(c, "OLD.status IN ('succeeded','cancelled')", "NEW.state_version<>OLD.state_version+1", "NEW.updated_at<OLD.updated_at", `NOT EXISTS (SELECT 1 FROM payout_status_history h JOIN payout_audit_log a ON a.transition_no=h.event_no WHERE h.event_no=NEW.last_transition_no AND h.payout_no=OLD.payout_no AND h.sequence=NEW.state_version AND h.from_status=OLD.status AND h.to_status=NEW.status AND h.created_at=NEW.updated_at AND h.actor_id=a.actor_id AND a.payout_no=OLD.payout_no AND a.created_at=h.created_at AND h.request_id<>'' AND h.reason<>'')`,
		`NEW.status<>'cancelled' AND NOT EXISTS (SELECT 1 FROM payout_status_history h JOIN payout_attempt t ON t.attempt_no=h.attempt_no WHERE h.event_no=NEW.last_transition_no AND t.payout_no=OLD.payout_no AND t.amount=OLD.amount AND t.currency=OLD.currency AND t.payee_snapshot_hash=OLD.payee_snapshot_hash AND ((NEW.status='processing' AND t.status='processing' AND t.operator_id=h.actor_id) OR (NEW.status='ready' AND t.status='abandoned' AND t.operator_id=h.actor_id) OR (NEW.status='pending_confirmation' AND t.status='submitted' AND t.operator_id=h.actor_id) OR (NEW.status='unknown' AND t.status='unknown' AND t.reviewer_id=h.actor_id) OR (NEW.status='failed' AND t.status='failed') OR (NEW.status='succeeded' AND t.status='succeeded' AND t.reviewer_id=h.actor_id)))`,
		`NEW.status IN ('succeeded','cancelled') AND NOT EXISTS (SELECT 1 FROM payout_terminal_event e JOIN anchor_withdrawal_order w ON w.withdrawal_no=e.withdrawal_no JOIN anchor_withdrawable_ledger l ON l.ledger_no=e.ledger_no WHERE e.payout_no=OLD.payout_no AND e.transition_no=NEW.last_transition_no AND e.withdrawal_no=OLD.business_no AND e.amount=OLD.amount AND e.currency=OLD.currency AND l.business_no=OLD.business_no AND l.change_amount=OLD.amount AND l.version=e.account_version AND ((NEW.status='succeeded' AND e.kind='payout.succeeded' AND w.status='paid' AND l.entry_type='withdrawal_paid') OR (NEW.status='cancelled' AND e.kind='payout.cancelled' AND w.status='cancelled' AND l.entry_type='withdrawal_release')))`,
		`NEW.status='cancelled' AND (OLD.status NOT IN ('ready','failed') OR EXISTS (SELECT 1 FROM payout_attempt t WHERE t.payout_no=OLD.payout_no AND t.status NOT IN ('failed','abandoned')))`)
	add("payout_order", "UPDATE", c)
	add("payout_status_history", "INSERT", []string{`NOT EXISTS (SELECT 1 FROM payout_order o WHERE o.payout_no=NEW.payout_no AND ((NEW.sequence=1 AND NEW.from_status='' AND NEW.to_status='ready' AND o.state_version=1 AND o.status='ready' AND NEW.actor_id=o.created_by AND NEW.created_at=o.created_at) OR (NEW.sequence=o.state_version+1 AND NEW.from_status=o.status AND NEW.actor_id>0 AND NEW.request_id<>'' AND NEW.reason<>'' AND NEW.created_at>=o.updated_at)))`})
	for _, m := range []any{&pm.Channel{}, &pm.ChannelAccount{}, &pm.Attempt{}, &pm.AttemptEvent{}, &pm.ManualRequest{}, &pm.DailyUsage{}, &pm.Evidence{}, &pm.RevealGrant{}, &pm.TerminalEvent{}, &pm.OrderReconciliation{}, &pm.Investigation{}} {
		st := &gorm.Statement{DB: db}
		if e = st.Parse(m); e != nil {
			return nil, e
		}
		add(st.Schema.Table, "DELETE", nil)
		var allowed string
		var conditions []string
		switch st.Schema.Table {
		case "payout_channel_account":
			allowed = "status updated_at disabled_at version last_audit_no"
			conditions = []string{"NEW.version<>OLD.version+1", "NEW.updated_at<OLD.updated_at", "NEW.status=OLD.status", "(NEW.status='active' AND NEW.disabled_at<>0) OR (NEW.status='disabled' AND NEW.disabled_at<>NEW.updated_at)", `NOT EXISTS (SELECT 1 FROM payout_audit_log a WHERE a.event_no=NEW.last_audit_no AND a.object_no=OLD.channel_account_no AND a.created_at=NEW.updated_at AND ((NEW.status='active' AND a.action='channel_account.enable') OR (NEW.status='disabled' AND a.action='channel_account.disable')))`}
		case "payout_attempt":
			allowed = "version status active_slot success_slot reviewer_id payment_reference payment_reference_fingerprint payment_time evidence_reference evidence_hash operator_note review_reason bank_submitted_at submitted_at reviewed_at completed_at failure_code failure_reason updated_at last_event_no"
			conditions = attemptUpdateConditions(db)
		case "payout_channel_daily_usage":
			allowed = "amount version last_attempt_no"
			conditions = []string{"NEW.version<>OLD.version+1", `NOT EXISTS (SELECT 1 FROM payout_attempt a JOIN payout_channel_account c ON c.channel_account_no=a.channel_account_no WHERE a.attempt_no=NEW.last_attempt_no AND a.channel_account_no=OLD.channel_account_no AND a.limit_day=OLD.day AND a.usage_version=NEW.version AND NEW.amount=OLD.amount+a.amount AND NEW.amount<=c.daily_limit)`}
		case "payout_reveal_grant":
			allowed = "consumed_at consumed_request_id"
			conditions = []string{"OLD.consumed_at<>0", "OLD.consumed_request_id<>''", "NEW.consumed_at<OLD.created_at", "NEW.consumed_at>=OLD.expires_at", "NEW.consumed_request_id=''", `NOT EXISTS (SELECT 1 FROM payout_audit_log a WHERE a.object_no=OLD.grant_no AND a.actor_id=OLD.actor_id AND a.created_at=NEW.consumed_at AND a.action IN ('payee.reveal','channel_account.reveal'))`}
		}
		if allowed != "" {
			imm, err := immutableColumns(db, m, allowed)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, imm...)
		}
		add(st.Schema.Table, "UPDATE", conditions)
	}
	add("payout_channel_account", "INSERT", []string{"NEW.status<>'active'", "NEW.version<>1", "NEW.disabled_at<>0", "NEW.updated_at<>NEW.created_at", `NOT EXISTS (SELECT 1 FROM payout_channel c WHERE c.channel_code=NEW.channel_code AND c.channel_type='manual')`, `NOT EXISTS (SELECT 1 FROM payout_audit_log a WHERE a.event_no=NEW.last_audit_no AND a.action='channel_account.create' AND a.object_no=NEW.channel_account_no AND a.actor_id=NEW.created_by)`})
	add("payout_attempt", "INSERT", append(attemptEvidenceConditions(db), "NEW.status<>'processing'", "NEW.version<>1", "NEW.operator_id=0", "NEW.reviewer_id<>0", "NEW.payment_reference<>''", "NEW.bank_submitted_at<>0", `NEW.sequence<>(SELECT COUNT(*)+1 FROM payout_attempt a WHERE a.payout_no=NEW.payout_no)`, `NOT EXISTS (SELECT 1 FROM payout_order o JOIN payout_channel_account c ON c.channel_account_no=NEW.channel_account_no JOIN payout_status_history h ON h.payout_no=o.payout_no WHERE o.payout_no=NEW.payout_no AND o.status IN ('ready','failed') AND h.sequence=o.state_version+1 AND h.to_status='processing' AND h.attempt_no=NEW.attempt_no AND h.actor_id=NEW.operator_id AND o.amount=NEW.amount AND o.currency=NEW.currency AND o.payee_snapshot_hash=NEW.payee_snapshot_hash AND c.status='active' AND c.currency=o.currency AND c.channel_code=NEW.channel_code AND c.masked_account=NEW.company_masked AND NEW.amount<=c.single_limit)`))
	add("payout_channel_daily_usage", "INSERT", []string{"NEW.version<>1", `NOT EXISTS (SELECT 1 FROM payout_attempt a JOIN payout_channel_account c ON c.channel_account_no=a.channel_account_no WHERE a.attempt_no=NEW.last_attempt_no AND a.channel_account_no=NEW.channel_account_no AND a.limit_day=NEW.day AND a.usage_version=1 AND NEW.amount=a.amount AND NEW.amount<=c.daily_limit)`})
	return all, nil
}

func attemptEvidenceConditions(db *gorm.DB) []string {
	keys := map[string]string{"version": "version", "status": "status", "reviewerId": "reviewer_id", "paymentReference": "payment_reference", "paymentFingerprint": "COALESCE(NEW.payment_reference_fingerprint,'')", "paymentTime": "payment_time", "evidenceReference": "evidence_reference", "evidenceHash": "evidence_hash", "operatorNote": "operator_note", "reviewReason": "review_reason", "bankSubmittedAt": "bank_submitted_at", "submittedAt": "submitted_at", "reviewedAt": "reviewed_at", "completedAt": "completed_at", "failureCode": "failure_code", "failureReason": "failure_reason", "updatedAt": "updated_at"}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := []string{"e.event_no=NEW.last_event_no", "e.attempt_no=NEW.attempt_no", "e.payout_no=NEW.payout_no", "e.version=NEW.version", "e.to_status=NEW.status", "e.created_at=NEW.updated_at"}
	for _, k := range names {
		col := keys[k]
		if !strings.HasPrefix(col, "COALESCE") {
			col = "NEW." + col
		}
		x := "json_extract(e.snapshot,'$." + k + "')"
		typ := "TEXT"
		if db.Dialector.Name() == "mysql" {
			x = "JSON_UNQUOTE(" + x + ")"
			typ = "CHAR"
		}
		parts = append(parts, sameSQL(db, x, "CAST("+col+" AS "+typ+")"))
	}
	parts = append(parts, `EXISTS (SELECT 1 FROM payout_audit_log au WHERE au.payout_no=NEW.payout_no AND au.object_no=NEW.attempt_no AND au.actor_id=e.actor_id AND au.action=e.action AND au.created_at=e.created_at AND au.transition_no=e.transition_no)`)
	return []string{"NOT EXISTS (SELECT 1 FROM payout_attempt_event e WHERE " + strings.Join(parts, " AND ") + ")"}
}
func attemptUpdateConditions(db *gorm.DB) []string {
	c := append(attemptEvidenceConditions(db), "NEW.version<>OLD.version+1", "NEW.updated_at<OLD.updated_at", "OLD.status IN ('succeeded','failed','abandoned')", `NOT ((OLD.status='processing' AND NEW.status IN ('processing','submitted','failed','abandoned')) OR (OLD.status='submitted' AND NEW.status IN ('succeeded','failed','unknown')) OR (OLD.status='unknown' AND NEW.status IN ('succeeded','failed')))`,
		`NOT EXISTS (SELECT 1 FROM payout_attempt_event e WHERE e.event_no=NEW.last_event_no AND e.from_status=OLD.status AND ((OLD.status='processing' AND e.actor_id=OLD.operator_id) OR (OLD.status IN ('submitted','unknown') AND e.actor_id=NEW.reviewer_id AND (NEW.reviewer_id<>OLD.operator_id OR NEW.reviewer_id=1))))`,
		`OLD.status='processing' AND NEW.status IN ('failed','abandoned') AND (OLD.bank_submitted_at<>0 OR OLD.payment_reference<>'' OR EXISTS (SELECT 1 FROM payout_evidence ev WHERE ev.attempt_no=OLD.attempt_no))`,
		`NEW.status IN ('submitted','unknown','succeeded') AND (NEW.payment_reference='' OR NEW.payment_reference_fingerprint IS NULL OR NEW.bank_submitted_at=0 OR NEW.submitted_at<OLD.created_at OR NEW.payment_time<OLD.created_at OR NOT EXISTS (SELECT 1 FROM payout_evidence ev WHERE ev.evidence_no=NEW.evidence_reference AND ev.attempt_no=OLD.attempt_no AND ev.payout_no=OLD.payout_no AND ev.sha256=NEW.evidence_hash AND ev.created_by=OLD.operator_id))`,
		`OLD.status IN ('submitted','unknown') AND (NEW.reviewer_id=0 OR NEW.review_reason='' OR NEW.reviewed_at<OLD.submitted_at)`,
		`OLD.status='unknown' AND NOT EXISTS (SELECT 1 FROM payout_attempt_event e JOIN payout_investigation i ON i.attempt_no=e.attempt_no JOIN payout_evidence ev ON ev.evidence_no=i.evidence_no WHERE e.event_no=NEW.last_event_no AND e.action='attempt.resolve_unknown' AND i.actor_id=NEW.reviewer_id AND i.created_at>=OLD.reviewed_at AND ev.created_by=NEW.reviewer_id AND ev.attempt_no=OLD.attempt_no AND ev.evidence_no<>OLD.evidence_reference AND ev.created_at>=OLD.reviewed_at)`,
		`NEW.status='processing' AND (OLD.bank_submitted_at<>0 OR NEW.bank_submitted_at=0 OR NEW.status<>OLD.status)`)
	for _, col := range []string{"payment_reference", "payment_reference_fingerprint", "payment_time", "evidence_reference", "evidence_hash", "operator_note", "submitted_at", "bank_submitted_at"} {
		c = append(c, "OLD.status<>'processing' AND NOT "+sameSQL(db, "NEW."+col, "OLD."+col))
	}
	// Every state change must be supported by the matching order transition intent.
	c = append(c, `NEW.status<>OLD.status AND NOT EXISTS (SELECT 1 FROM payout_attempt_event e JOIN payout_status_history h ON h.event_no=e.transition_no JOIN payout_order o ON o.payout_no=h.payout_no WHERE e.event_no=NEW.last_event_no AND h.attempt_no=OLD.attempt_no AND h.actor_id=e.actor_id AND h.sequence=o.state_version+1 AND h.from_status=o.status AND ((NEW.status='submitted' AND h.to_status='pending_confirmation') OR (NEW.status='abandoned' AND h.to_status='ready') OR (NEW.status=h.to_status)))`)
	return c
}
