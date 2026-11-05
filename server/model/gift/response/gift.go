package response

import (
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/gift"
)

type Page struct {
	List     any    `json:"list"`
	Total    int64  `json:"total,string"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Version  string `json:"version,omitempty"`
}
type Detail struct {
	Order         model.Order                 `json:"order"`
	Refunds       []model.Refund              `json:"refunds"`
	Revenue       []model.Revenue             `json:"revenue"`
	Earnings      []earning.Ledger            `json:"earnings"`
	Benefits      []model.BenefitGrant        `json:"benefits"`
	Compensations []model.BenefitCompensation `json:"compensations"`
	Events        []model.Event               `json:"events"`
	Consistent    bool                        `json:"consistent"`
	Issues        []string                    `json:"issues"`
}
type Ranking struct {
	No       string `json:"no"`
	Gross    string `json:"gross"`
	Refund   string `json:"refund"`
	Net      string `json:"net"`
	Earning  string `json:"earning"`
	Quantity string `json:"quantity"`
	Orders   string `json:"orders"`
	Users    string `json:"users"`
}

type Rule struct {
	earning.RevenueRule
	Termination *earning.RuleTermination `json:"termination"`
}
