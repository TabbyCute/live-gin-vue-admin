package request

type ProfileQuery struct {
	AnchorNo string `form:"anchorNo"`
}

type SupportersQuery struct {
	ProfileQuery
	Period   string `form:"period"`
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
}

type GuardsQuery struct {
	ProfileQuery
	Page     int `form:"page"`
	PageSize int `form:"pageSize"`
}

type SaveSettings struct {
	Signature  string `json:"signature"`
	CategoryID uint64 `json:"categoryId"`
	Version    string `json:"version"`
}
