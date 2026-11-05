package response

import (
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/payout"
)

type Order struct {
	PayoutNo          string `json:"payoutNo"`
	BusinessType      string `json:"businessType"`
	BusinessNo        string `json:"businessNo"`
	AnchorNo          string `json:"anchorNo"`
	Amount            int64  `json:"amount,string"`
	Currency          string `json:"currency"`
	PayeeAccountNo    string `json:"payeeAccountNo"`
	PayeeVersion      string `json:"payeeVersion"`
	PayeeMethod       string `json:"payeeMethod"`
	AccountMasked     string `json:"accountMasked"`
	PayeeSnapshotHash string `json:"payeeSnapshotHash"`
	Status            string `json:"status"`
	CreatedBy         uint64 `json:"createdBy,string"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
}

func PublicOrder(o model.Order) Order {
	return Order{o.PayoutNo, o.BusinessType, o.BusinessNo, o.AnchorNo, o.Amount, o.Currency, o.PayeeAccountNo, o.PayeeVersion, o.PayeeMethod, o.AccountMasked, o.PayeeSnapshotHash, o.Status, o.CreatedBy, o.CreatedAt, o.UpdatedAt}
}

type OrderPage struct {
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int64   `json:"total,string"`
	List     []Order `json:"list"`
}
type OrderDetail struct {
	Evidence                []EvidenceInfo              `json:"evidence"`
	Attempts                []Attempt                   `json:"attempts"`
	AttemptEvents           []model.AttemptEvent        `json:"attemptEvents"`
	Investigations          []model.Investigation       `json:"investigations"`
	Reconciliations         []model.OrderReconciliation `json:"reconciliations"`
	Check                   Reconciliation              `json:"check"`
	AllowedActions          map[string]bool             `json:"allowedActions"`
	AuditTruncated          bool                        `json:"auditTruncated"`
	InvestigationsTruncated bool                        `json:"investigationsTruncated"`
	Order                   Order                       `json:"order"`
	Withdrawal              earning.Withdrawal          `json:"withdrawal"`
	CurrentPayee            Current                     `json:"currentPayee"`
	History                 []model.StatusHistory       `json:"history"`
	Audit                   []model.OrderAudit          `json:"audit"`
	Consistent              bool                        `json:"consistent"`
	CheckedAt               int64                       `json:"checkedAt"`
	CreationReconciliation  string                      `json:"creationReconciliation"`
}
