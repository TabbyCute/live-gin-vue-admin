package earning

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	res "tb_live_module/model/earning/response"
)

var _ = response.Response{}
var _ = model.AuditJob{}
var _ = res.Page{}

// StartAudit
// @Tags Earning
// @Summary 排队后台深度核验；报告不能作为放款凭据，失败重试须新幂等键
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.StartAudit true "主播、币种、幂等键和可选失败原作业号"
// @Success 200 {object} response.Response{data=model.AuditJob}
// @Router /v1/admin/earning/audit/start [post]
func (API) StartAudit(c *gin.Context) {
	var r req.StartAudit
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.StartAudit(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// AuditList
// @Tags Earning
// @Summary 分页查看后台核验状态、进度及历史快照
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "主播、状态、作业号与分页参数"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/audit/list [get]
func (API) AuditList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	out, e := earningService.ListAudits(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// CancelAudit
// @Tags Earning
// @Summary 取消排队或执行中核验，不改变资金及终态报告
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.CancelAudit true "作业号"
// @Success 200 {object} response.Response{data=model.AuditJob}
// @Router /v1/admin/earning/audit/cancel [post]
func (API) CancelAudit(c *gin.Context) {
	var r req.CancelAudit
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.CancelAudit(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}
