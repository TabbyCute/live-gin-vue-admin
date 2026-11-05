package response

import common "tb_live_module/model/common/response"
import model "tb_live_module/model/guard"

type Membership struct {
	ActivePlan      *model.PlanSnapshot      `json:"activePlan"`
	MembershipNo    string                   `json:"membershipNo"`
	User            common.UserPublicProfile `json:"user"`
	AnchorNo        string                   `json:"anchorNo"`
	ActivePlanNo    string                   `json:"activePlanNo"`
	StartedAt       int64                    `json:"startedAt"`
	ExpiresAt       int64                    `json:"expiresAt"`
	RemainingMillis string                   `json:"remainingMillis"`
	RemainingDays   string                   `json:"remainingDays"`
	Status          string                   `json:"status"`
	GeneratedAt     int64                    `json:"generatedAt"`
}
type Page struct {
	Actions      map[string]bool `json:"actions,omitempty"`
	GuardEnabled bool            `json:"guardEnabled,omitempty"`
	List         any             `json:"list"`
	Total        string          `json:"total"`
	Page         int             `json:"page"`
	PageSize     int             `json:"pageSize"`
}
