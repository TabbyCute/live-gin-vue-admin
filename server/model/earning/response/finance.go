package response

import (
	"regexp"
	"strconv"
	model "tb_live_module/model/earning"
	payout "tb_live_module/model/payout/response"
)

type Page struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total,string"`
	List     any   `json:"list"`
}
type AppWithdrawal struct {
	PayeeAccountNo string `json:"payeeAccountNo"`
	PayeeMethod    string `json:"payeeMethod"`
	PayeeVersion   string `json:"payeeVersion"`
	WithdrawalNo   string `json:"withdrawalNo"`
	RequestID      string `json:"requestId"`
	Amount         int64  `json:"amount,string"`
	Currency       string `json:"currency"`
	AccountMasked  string `json:"accountMasked"`
	Status         string `json:"status"`
	CreatedAt      int64  `json:"createdAt"`
	ReviewedAt     int64  `json:"reviewedAt"`
}

var publicPayeeRE = regexp.MustCompile(`^pa_[a-f0-9]{32}$`)

func PublicWithdrawal(w model.Withdrawal) AppWithdrawal {
	if !publicPayeeRE.MatchString(w.PayeeRef) {
		w.PayeeRef = ""
	}
	return AppWithdrawal{w.PayeeRef, w.PayeeMethod, w.PayeeVersion, w.WithdrawalNo, w.RequestID, w.Amount, w.Currency, w.AccountMasked, w.Status, w.CreatedAt, w.ReviewedAt}
}

type AppSettlement struct {
	SettlementNo string `json:"settlementNo"`
	PeriodStart  int64  `json:"periodStart"`
	PeriodEnd    int64  `json:"periodEnd"`
	EarningCoin  int64  `json:"earningCoin,string"`
	Currency     string `json:"currency"`
	Amount       int64  `json:"amount,string"`
	Status       string `json:"status"`
	CreatedAt    int64  `json:"createdAt"`
	ApprovedAt   int64  `json:"approvedAt"`
}

func PublicSettlement(s model.Settlement) AppSettlement {
	return AppSettlement{s.SettlementNo, s.PeriodStart, s.PeriodEnd, s.EarningCoin, s.Currency, s.Amount, s.Status, s.CreatedAt, s.ApprovedAt}
}

type Balance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Debt      string `json:"debt"`
	Paid      string `json:"paid"`
	Version   string `json:"version"`
}

func PublicBalance(a model.Account) Balance {
	return Balance{a.Currency, strconv.FormatInt(a.Available, 10), strconv.FormatInt(a.Frozen, 10), strconv.FormatInt(a.Debt, 10), strconv.FormatInt(a.Paid, 10), strconv.FormatInt(a.Version, 10)}
}

type FinanceOverview struct {
	AnchorNo          string    `json:"anchorNo"`
	PendingCoin       string    `json:"pendingCoin"`
	SettledCoin       string    `json:"settledCoin"`
	Balances          []Balance `json:"balances"`
	WithdrawalEnabled bool      `json:"withdrawalEnabled"`
	PayoutEnabled     bool      `json:"payoutEnabled"`
}
type SettlementDetail struct {
	Settlement model.Settlement       `json:"settlement"`
	Items      []model.SettlementItem `json:"items"`
	Audit      []model.FinanceAudit   `json:"audit"`
	Consistent bool                   `json:"consistent"`
}
type WithdrawalDetail struct {
	CurrentPayee    payout.Current         `json:"currentPayee"`
	Withdrawal      model.Withdrawal       `json:"withdrawal"`
	Reconciliations []model.Reconciliation `json:"reconciliations"`
	Ledger          []model.AccountLedger  `json:"ledger"`
	Audit           []model.FinanceAudit   `json:"audit"`
	PayoutEnabled   bool                   `json:"payoutEnabled"`
}
