package response

import common "tb_live_module/model/common/response"

type PublicPlan struct {
	PlanNo   string   `json:"planNo"`
	Name     string   `json:"name"`
	Level    int      `json:"level"`
	Priority int      `json:"priority"`
	Benefits []string `json:"benefits"`
}
type PublicMember struct {
	User common.UserPublicProfile `json:"user"`
	Plan PublicPlan               `json:"plan"`
}
type PublicMembers struct {
	List        []PublicMember `json:"list"`
	Total       string         `json:"total"`
	Page        int            `json:"page"`
	PageSize    int            `json:"pageSize"`
	Enabled     bool           `json:"enabled"`
	Plans       []PublicPlan   `json:"plans"`
	GeneratedAt int64          `json:"generatedAt"`
}
