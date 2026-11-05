package initialize

// MySQL canonical monetary guards for the paid version chain.
func init() {
	payoutMySQLChecks["ck_withdrawable_balances"] = "((`available` >= 0) and (`frozen` >= 0) and (`paid` = 0) and (`debt` >= 0) and ((`debt` = 0) or (`available` = 0)))"
	payoutMySQLChecks["ck_withdrawable_entry"] = "(`entry_type` in ('settlement_credit','negative_adjustment','withdrawal_freeze','withdrawal_release'))"
	payoutMySQLChecks["ck_withdrawal_status"] = "(`status` in ('pending_review','rejected','approved'))"
	payoutMySQLChecks["ck_withdrawable_balances_v3"] = "((`available` >= 0) and (`frozen` >= 0) and (`paid` >= 0) and (`debt` >= 0) and ((`debt` = 0) or (`available` = 0)))"
	payoutMySQLChecks["ck_withdrawable_entry_v3"] = "(`entry_type` in ('settlement_credit','negative_adjustment','withdrawal_freeze','withdrawal_release','withdrawal_paid'))"
	payoutMySQLChecks["ck_withdrawable_paid_chain"] = "((`paid_before` >= 0) and (`paid_after` >= `paid_before`) and (((`entry_type` = 'withdrawal_paid') and (`change_amount` > 0) and ((`paid_after` - `paid_before`) = `change_amount`) and ((`frozen_before` - `frozen_after`) = `change_amount`) and (`available_after` = `available_before`) and (`debt_after` = `debt_before`)) or ((`entry_type` <> 'withdrawal_paid') and (`paid_after` = `paid_before`))))"
	payoutMySQLChecks["ck_withdrawable_version"] = "(`version` >= 0)"
	payoutMySQLChecks["ck_withdrawal_amount"] = "(`amount` > 0)"
	payoutMySQLChecks["ck_withdrawal_recon"] = "(`reconciliation_status` in ('pending','passed','abnormal'))"
	payoutMySQLChecks["ck_withdrawal_status_v3"] = "(`status` in ('pending_review','rejected','approved','paid','cancelled'))"
}
