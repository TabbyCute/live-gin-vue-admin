package payout

import (
	"gorm.io/gorm"
	"strconv"
	pm "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"tb_live_module/model/system"
)

func validateAttemptFilters(q req.OrderSearch) error {
	if q.ChannelAccountNo != "" && !companyNoRE.MatchString(q.ChannelAccountNo) {
		return ErrOrderInvalid
	}
	for _, v := range []string{q.OperatorID, q.ReviewerID} {
		if v != "" {
			n, e := strconv.ParseUint(v, 10, 64)
			if e != nil || n == 0 || strconv.FormatUint(n, 10) != v {
				return ErrOrderInvalid
			}
		}
	}
	if q.PaymentReference != "" {
		if q.ChannelAccountNo == "" {
			return ErrOrderInvalid
		}
		if _, e := normalizeReference(q.PaymentReference); e != nil {
			return e
		}
	}
	return nil
}
func filterOrderAttempts(db, tx *gorm.DB, c *Crypto, q req.OrderSearch) *gorm.DB {
	sub := tx.Model(&pm.Attempt{}).Select("payout_no")
	for _, f := range []struct{ k, v string }{{"channel_account_no", q.ChannelAccountNo}, {"operator_id", q.OperatorID}, {"reviewer_id", q.ReviewerID}} {
		if f.v != "" {
			sub = sub.Where(f.k+"=?", f.v)
		}
	}
	if q.PaymentReference != "" {
		r, _ := normalizeReference(q.PaymentReference)
		if c == nil {
			return db.Where("1=0")
		}
		sub = sub.Where("payment_reference_fingerprint=?", c.digest("payout.payment.reference.v1", []string{q.ChannelAccountNo, r}))
	}
	return db.Where("payout_no IN (?)", sub)
}
func (s *Service) manualDetailInTx(tx *gorm.DB, a Actor, o pm.Order, out *res.OrderDetail) error {
	out.Evidence = []res.EvidenceInfo{}
	if e := tx.Model(&pm.Evidence{}).Select("evidence_no,attempt_no,sha256,mime,size,created_by,created_at").Where("payout_no=?", o.PayoutNo).Order("created_at DESC,evidence_no DESC").Limit(1000).Scan(&out.Evidence).Error; e != nil {
		return e
	}
	out.Attempts = []res.Attempt{}
	out.AttemptEvents = []pm.AttemptEvent{}
	out.Investigations = []pm.Investigation{}
	out.Reconciliations = []pm.OrderReconciliation{}
	out.Audit = []pm.OrderAudit{}
	var attempts []pm.Attempt
	if e := tx.Where("payout_no=?", o.PayoutNo).Order("sequence ASC").Limit(100).Find(&attempts).Error; e != nil {
		return e
	}
	for _, at := range attempts {
		out.Attempts = append(out.Attempts, res.PublicAttempt(at))
	}
	for _, q := range []struct {
		dest  any
		order string
		limit int
	}{{&out.AttemptEvents, "created_at ASC,event_no ASC", 600}, {&out.Investigations, "created_at DESC,investigation_no DESC", 201}, {&out.Reconciliations, "created_at DESC,reconciliation_no DESC", 20}, {&out.Audit, "id DESC", 201}} {
		if e := tx.Where("payout_no=?", o.PayoutNo).Order(q.order).Limit(q.limit).Find(q.dest).Error; e != nil {
			return e
		}
	}
	if len(out.Audit) > 200 {
		out.AuditTruncated = true
		out.Audit = out.Audit[:200]
	}
	if len(out.Investigations) > 200 {
		out.InvestigationsTruncated = true
		out.Investigations = out.Investigations[:200]
	}
	caps, e := s.capabilitiesInTx(tx, a)
	if e != nil {
		return e
	}
	allowed := map[string]bool{}
	add := func(path string, ok bool) { allowed[path] = caps[path] && ok }
	add("order/start", o.Status == "ready" && out.Consistent && out.CurrentPayee.MatchesSnapshot && out.CurrentPayee.State == "active" && s.EvidenceStore != nil)
	add("order/retry", o.Status == "failed" && out.Consistent && out.CurrentPayee.MatchesSnapshot && out.CurrentPayee.State == "active" && s.EvidenceStore != nil)
	add("order/cancel", o.Status == "ready" || o.Status == "failed")
	add("order/reconcile", true)
	add("order/investigate", o.Status == "unknown")
	if len(attempts) > 0 {
		at := attempts[len(attempts)-1]
		owner := at.OperatorID == a.ID
		processing := o.Status == "processing" && at.Status == "processing"
		independent := !owner || system.IsSuperAdmin(a.ID)
		var evidenceCount int64
		if e = tx.Model(&pm.Evidence{}).Where("attempt_no=?", at.AttemptNo).Count(&evidenceCount).Error; e != nil {
			return e
		}
		add("order/release", owner && processing && at.BankSubmittedAt == 0 && evidenceCount == 0)
		add("attempt/fail-unpaid", owner && processing && at.BankSubmittedAt == 0 && evidenceCount == 0)
		add("attempt/bank-submitted", owner && processing && at.BankSubmittedAt == 0)
		add("order/reveal-payee", owner && processing && caps["order/reveal-payee-grant"])
		add("attempt/submit", owner && processing)
		add("evidence/upload", s.EvidenceStore != nil && ((owner && processing) || (o.Status == "unknown" && independent && caps["attempt/resolve-unknown"])))
		add("evidence/read", true)
		for _, decision := range []string{"success", "failure", "unknown"} {
			add("attempt/review-"+decision, independent && o.Status == "pending_confirmation")
		}
		add("attempt/resolve-unknown", independent && o.Status == "unknown")
	}
	out.AllowedActions = allowed
	return nil
}
