package initialize

// Canonical MySQL 8 CHECK definitions captured from the isolated migration
// fixture, including boolean grouping. Startup rejects altered same-name checks.
func init() {
	payoutMySQLChecks["ck_attempt_active"] = "(((`status` in ('processing','submitted','unknown')) and (`active_slot` is not null) and (`active_slot` = 1)) or ((`status` in ('succeeded','failed','abandoned')) and (`active_slot` is null)))"
	payoutMySQLChecks["ck_attempt_amount"] = "(`amount` > 0)"
	payoutMySQLChecks["ck_attempt_sequence"] = "((`sequence` > 0) and (`version` > 0) and (`usage_version` > 0))"
	payoutMySQLChecks["ck_attempt_status"] = "(`status` in ('processing','submitted','succeeded','failed','unknown','abandoned'))"
	payoutMySQLChecks["ck_attempt_success"] = "(((`status` = 'succeeded') and (`success_slot` is not null) and (`success_slot` = 1)) or ((`status` <> 'succeeded') and (`success_slot` is null)))"
	payoutMySQLChecks["ck_company_limits"] = "((`single_limit` > 0) and (`daily_limit` >= `single_limit`))"
	payoutMySQLChecks["ck_company_status"] = "(`status` in ('active','disabled'))"
	payoutMySQLChecks["ck_company_version"] = "(`version` > 0)"
	payoutMySQLChecks["ck_payout_actor"] = "(`created_by` > 0)"
	payoutMySQLChecks["ck_payout_amount"] = "(`amount` > 0)"
	payoutMySQLChecks["ck_payout_audit_v3"] = "(`action` in ('order.create','order.start','order.retry','order.release','attempt.bank_submitted','attempt.submit','attempt.review_success','attempt.review_failure','attempt.review_unknown','attempt.resolve_unknown','attempt.fail_unpaid','order.cancel','order.investigate','order.reconcile','channel_account.create','channel_account.enable','channel_account.disable','channel_account.reveal','payee.reveal_grant','payee.reveal','evidence.upload','evidence.read'))"
	payoutMySQLChecks["ck_payout_business"] = "(`business_type` = 'anchor_withdrawal')"
	payoutMySQLChecks["ck_payout_channel"] = "((`channel_code` = 'manual_bank') and (`channel_type` = 'manual'))"
	payoutMySQLChecks["ck_payout_history_sequence"] = "(`sequence` > 0)"
	payoutMySQLChecks["ck_payout_history_v3"] = "(((`from_status` = '') and (`to_status` = 'ready')) or ((`from_status` = 'ready') and (`to_status` in ('processing','cancelled'))) or ((`from_status` = 'processing') and (`to_status` in ('ready','pending_confirmation','failed'))) or ((`from_status` = 'pending_confirmation') and (`to_status` in ('succeeded','failed','unknown'))) or ((`from_status` = 'unknown') and (`to_status` in ('succeeded','failed'))) or ((`from_status` = 'failed') and (`to_status` in ('processing','cancelled'))))"
	payoutMySQLChecks["ck_payout_method"] = "(`payee_method` in ('bank_card','alipay','wechat'))"
	payoutMySQLChecks["ck_payout_payload"] = "(`payload_version` = 1)"
	payoutMySQLChecks["ck_payout_reconciliation"] = "(`status` in ('passed','abnormal'))"
	payoutMySQLChecks["ck_payout_state_version"] = "(`state_version` > 0)"
	payoutMySQLChecks["ck_payout_status_v3"] = "(`status` in ('ready','processing','pending_confirmation','succeeded','failed','unknown','cancelled'))"
	payoutMySQLChecks["ck_payout_terminal"] = "(`kind` in ('payout.succeeded','payout.cancelled'))"
	payoutMySQLChecks["ck_payout_time_v3"] = "((`created_at` > 0) and (`updated_at` >= `created_at`))"
	payoutMySQLChecks["ck_payout_usage"] = "((`amount` > 0) and (`version` > 0))"
}
