package response

import common "tb_live_module/model/common/response"

type Entry struct {
	Rank        string                   `json:"rank"`
	Score       string                   `json:"score"`
	Quantity    string                   `json:"quantity"`
	SubjectNo   string                   `json:"subjectNo"`
	SubjectType string                   `json:"subjectType"`
	User        common.UserPublicProfile `json:"user"`
}
type Board struct {
	List        []Entry `json:"list"`
	Total       string  `json:"total"`
	Page        int     `json:"page"`
	PageSize    int     `json:"pageSize"`
	RankType    string  `json:"rankType"`
	Period      string  `json:"period"`
	PeriodKey   string  `json:"periodKey"`
	Timezone    string  `json:"timezone"`
	ScopeType   string  `json:"scopeType"`
	ScopeNo     string  `json:"scopeNo"`
	GiftNo      string  `json:"giftNo"`
	MyRank      *Entry  `json:"myRank"`
	GeneratedAt int64   `json:"generatedAt"`
	DataAsOf    int64   `json:"dataAsOf"`
	LagCount    string  `json:"lagCount"`
}
