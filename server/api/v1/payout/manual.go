package payout

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
)

// CompanyCreate 创建公司人工出款账户.
// @Tags PayoutAdmin
// @Summary 创建公司人工出款账户
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.CreateChannelAccount true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/channel-account/create [post]
func (API) CompanyCreate(c *gin.Context) {
	a := orderActor(c)
	var r req.CreateChannelAccount
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.CreateChannelAccount(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// CompanyEnable 启用公司出款账户.
// @Tags PayoutAdmin
// @Summary 启用公司出款账户
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ChannelAccountAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/channel-account/enable [post]
func (API) CompanyEnable(c *gin.Context) {
	a := orderActor(c)
	var r req.ChannelAccountAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ChangeChannelAccount(c.Request.Context(), a, c.GetString("payoutTrace"), r, true)
	respond(c, out, e)
}

// CompanyDisable 停用公司出款账户.
// @Tags PayoutAdmin
// @Summary 停用公司出款账户
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ChannelAccountAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/channel-account/disable [post]
func (API) CompanyDisable(c *gin.Context) {
	a := orderActor(c)
	var r req.ChannelAccountAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ChangeChannelAccount(c.Request.Context(), a, c.GetString("payoutTrace"), r, false)
	respond(c, out, e)
}

// ManualStart 领取人工出款.
// @Tags PayoutAdmin
// @Summary 领取人工出款
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.StartOrder true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/start [post]
func (API) ManualStart(c *gin.Context) {
	a := orderActor(c)
	var r req.StartOrder
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.StartOrder(c.Request.Context(), a, c.GetString("payoutTrace"), r, false)
	respond(c, out, e)
}

// ManualRetry 明确失败后新建尝试.
// @Tags PayoutAdmin
// @Summary 明确失败后新建尝试
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.StartOrder true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/retry [post]
func (API) ManualRetry(c *gin.Context) {
	a := orderActor(c)
	var r req.StartOrder
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.StartOrder(c.Request.Context(), a, c.GetString("payoutTrace"), r, true)
	respond(c, out, e)
}

// ManualRelease 未转账且无凭证时释放领取.
// @Tags PayoutAdmin
// @Summary 未转账且无凭证时释放领取
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.UnpaidAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/release [post]
func (API) ManualRelease(c *gin.Context) {
	a := orderActor(c)
	var r req.UnpaidAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.UnpaidAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r, false)
	respond(c, out, e)
}

// FailUnpaid 确认未执行转账.
// @Tags PayoutAdmin
// @Summary 确认未执行转账
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.UnpaidAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/fail-unpaid [post]
func (API) FailUnpaid(c *gin.Context) {
	a := orderActor(c)
	var r req.UnpaidAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.UnpaidAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r, true)
	respond(c, out, e)
}

// BankSubmitted 标记已提交银行并禁止释放.
// @Tags PayoutAdmin
// @Summary 标记已提交银行并禁止释放
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.AttemptAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/bank-submitted [post]
func (API) BankSubmitted(c *gin.Context) {
	a := orderActor(c)
	var r req.AttemptAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.MarkBankSubmitted(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// ManualSubmit 提交真实流水及私有凭证待独立复核.
// @Tags PayoutAdmin
// @Summary 提交真实流水及私有凭证待独立复核
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.SubmitAttempt true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/submit [post]
func (API) ManualSubmit(c *gin.Context) {
	a := orderActor(c)
	var r req.SubmitAttempt
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.SubmitAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// ReviewSuccess 独立确认成功并原子入账.
// @Tags PayoutAdmin
// @Summary 独立确认成功并原子入账
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ReviewAttempt true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/review-success [post]
func (API) ReviewSuccess(c *gin.Context) {
	a := orderActor(c)
	var r req.ReviewAttempt
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ReviewAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r, "success")
	respond(c, out, e)
}

// ReviewFailure 独立确认未付款成功.
// @Tags PayoutAdmin
// @Summary 独立确认未付款成功
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ReviewAttempt true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/review-failure [post]
func (API) ReviewFailure(c *gin.Context) {
	a := orderActor(c)
	var r req.ReviewAttempt
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ReviewAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r, "failure")
	respond(c, out, e)
}

// ReviewUnknown 结果未知并继续冻结.
// @Tags PayoutAdmin
// @Summary 结果未知并继续冻结
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ReviewAttempt true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/review-unknown [post]
func (API) ReviewUnknown(c *gin.Context) {
	a := orderActor(c)
	var r req.ReviewAttempt
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ReviewAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r, "unknown")
	respond(c, out, e)
}

// ResolveUnknown 通过新调查和新凭证解决未知结果.
// @Tags PayoutAdmin
// @Summary 通过新调查和新凭证解决未知结果
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ResolveAttempt true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/attempt/resolve-unknown [post]
func (API) ResolveUnknown(c *gin.Context) {
	a := orderActor(c)
	var r req.ResolveAttempt
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ResolveAttempt(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// ManualCancel 确认无付款并取消提现释放冻结.
// @Tags PayoutAdmin
// @Summary 确认无付款并取消提现释放冻结
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.CancelOrder true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/cancel [post]
func (API) ManualCancel(c *gin.Context) {
	a := orderActor(c)
	var r req.CancelOrder
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.CancelOrder(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// ManualInvestigate 追加未知结果调查记录.
// @Tags PayoutAdmin
// @Summary 追加未知结果调查记录
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Investigate true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/investigate [post]
func (API) ManualInvestigate(c *gin.Context) {
	a := orderActor(c)
	var r req.Investigate
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.InvestigateOrder(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// ManualReconcile 执行内部账务对账（非银行对账）.
// @Tags PayoutAdmin
// @Summary 执行内部账务对账（非银行对账）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.OrderAction true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/order/reconcile [post]
func (API) ManualReconcile(c *gin.Context) {
	a := orderActor(c)
	var r req.OrderAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.RunOrderReconciliation(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// PayeeGrant 申请45秒单次收款资料查看凭证.
// @Tags PayoutAdmin
// @Summary 申请45秒单次收款资料查看凭证
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.RevealGrant true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.RevealTicket}
// @Router /v1/admin/payout/order/reveal-payee-grant [post]
func (API) PayeeGrant(c *gin.Context) {
	a := orderActor(c)
	var r req.RevealGrant
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.GrantReveal(c.Request.Context(), a, c.GetString("payoutTrace"), r, false)
	respond(c, out, e)
}

// PayeeReveal 消耗单次凭证查看当前领取订单的收款资料.
// @Tags PayoutAdmin
// @Summary 消耗单次凭证查看当前领取订单的收款资料
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.RevealConsume true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.RevealedAccount}
// @Router /v1/admin/payout/order/reveal-payee [post]
func (API) PayeeReveal(c *gin.Context) {
	a := orderActor(c)
	var r req.RevealConsume
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ConsumeReveal(c.Request.Context(), a, c.GetString("payoutTrace"), r, false)
	respond(c, out, e)
}

// CompanyGrant 申请45秒单次公司账户查看凭证.
// @Tags PayoutAdmin
// @Summary 申请45秒单次公司账户查看凭证
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.RevealGrant true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.RevealTicket}
// @Router /v1/admin/payout/channel-account/reveal-grant [post]
func (API) CompanyGrant(c *gin.Context) {
	a := orderActor(c)
	var r req.RevealGrant
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.GrantReveal(c.Request.Context(), a, c.GetString("payoutTrace"), r, true)
	respond(c, out, e)
}

// CompanyReveal 消耗单次凭证查看公司完整账号.
// @Tags PayoutAdmin
// @Summary 消耗单次凭证查看公司完整账号
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.RevealConsume true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.RevealedAccount}
// @Router /v1/admin/payout/channel-account/reveal [post]
func (API) CompanyReveal(c *gin.Context) {
	a := orderActor(c)
	var r req.RevealConsume
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.ConsumeReveal(c.Request.Context(), a, c.GetString("payoutTrace"), r, true)
	respond(c, out, e)
}

// EvidenceUpload 上传私有加密PNG或JPEG（原图最多2MiB）.
// @Tags PayoutAdmin
// @Summary 上传私有加密PNG或JPEG（原图最多2MiB）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.UploadEvidence true "固定requestId；参数不可改变"
// @Success 200 {object} response.Response{data=res.ActionResult}
// @Router /v1/admin/payout/evidence/upload [post]
func (API) EvidenceUpload(c *gin.Context) {
	a := orderActor(c)
	var r req.UploadEvidence
	if !bindBodySized(c, &r, 3<<20) {
		return
	}
	out, e := payoutService.UploadEvidence(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// CompanyList returns masked company accounts.
// @Tags PayoutAdmin
// @Summary 公司出款账户列表（限额为平台风控限制）
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "默认1，最多100"
// @Param pageSize query int false "默认20，最多100"
// @Param currency query string false "币种"
// @Param status query string false "active/disabled"
// @Success 200 {object} response.Response{data=res.ChannelAccountPage}
// @Router /v1/admin/payout/channel-account/list [get]
func (API) CompanyList(c *gin.Context) {
	a := orderActor(c)
	var q req.ChannelAccountSearch
	if !orderQuery(c, &q) {
		return
	}
	out, e := payoutService.ListChannelAccounts(c.Request.Context(), a, q)
	respond(c, out, e)
}

// Capabilities returns current exact permissions for this administrator.
// @Tags PayoutAdmin
// @Summary 当前启用管理员的出款权限
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=map[string]bool}
// @Router /v1/admin/payout/capabilities [get]
func (API) Capabilities(c *gin.Context) {
	a := orderActor(c)
	if c.Request.URL.RawQuery != "" {
		invalid(c)
		return
	}
	out, e := payoutService.Capabilities(c.Request.Context(), a)
	respond(c, out, e)
}

// EvidenceRead is authenticated and audited; it never returns a public URL.
// @Tags PayoutAdmin
// @Summary 审计读取私有凭证（成功返回图片，失败返回JSON）
// @Security ApiKeyAuth
// @Accept json
// @Produce image/png,image/jpeg,application/json
// @Param data body req.ReadEvidence true "凭证编号与查看原因"
// @Success 200 {file} binary
// @Router /v1/admin/payout/evidence/read [post]
func (API) EvidenceRead(c *gin.Context) {
	a := orderActor(c)
	var r req.ReadEvidence
	if !bindBody(c, &r) {
		return
	}
	b, mime, e := payoutService.ReadEvidence(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	if e != nil {
		respond(c, nil, e)
		return
	}
	defer clear(b)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Content-Disposition", "attachment; filename=payment-evidence")
	c.Data(200, mime, b)
}

var _ response.Response
var _ res.ActionResult
