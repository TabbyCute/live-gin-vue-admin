package response

import model "tb_live_module/model/payout"

type Payee struct {
	PayeeAccountNo string `json:"payeeAccountNo"`
	Method         string `json:"method"`
	Currency       string `json:"currency"`
	MaskedAccount  string `json:"maskedAccount"`
	Status         string `json:"status"`
	Version        int64  `json:"version,string"`
	IsDefault      bool   `json:"isDefault"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

func Public(p model.PayeeAccount) Payee {
	return Payee{p.PayeeAccountNo, p.Method, p.Currency, p.MaskedAccount, p.Status, p.Version, p.DefaultSlot != nil, p.CreatedAt, p.UpdatedAt}
}

type Page struct {
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int64   `json:"total,string"`
	List     []Payee `json:"list"`
}

// Current is informational. Approval always rechecks inside the write transaction.
type Current struct {
	Account         *Payee `json:"account"`
	State           string `json:"state"`
	MatchesSnapshot bool   `json:"matchesSnapshot"`
	Usable          bool   `json:"usable"`
}
