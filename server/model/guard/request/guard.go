package request

type Query struct {
	Page         int    `form:"page" json:"page"`
	PageSize     int    `form:"pageSize" json:"pageSize"`
	AnchorNo     string `form:"anchorNo" json:"anchorNo"`
	AccountNo    string `form:"accountNo" json:"accountNo"`
	MembershipNo string `form:"membershipNo" json:"membershipNo"`
	Status       string `form:"status" json:"status"`
}
type Plan struct {
	RequestID           string `json:"requestId"`
	PlanNo              string `json:"planNo"`
	Name                string `json:"name"`
	Level               int    `json:"level"`
	Priority            int    `json:"priority"`
	DefaultDurationDays string `json:"defaultDurationDays"`
	BenefitConfig       string `json:"benefitConfig"`
	Status              string `json:"status"`
	SortOrder           int    `json:"sortOrder"`
	Version             string `json:"version"`
}
type Action struct {
	RequestID string `json:"requestId"`
	Number    string `json:"number"`
}
