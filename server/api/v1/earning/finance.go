package earning

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"tb_live_module/global"
	"tb_live_module/model/common/response"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	res "tb_live_module/model/earning/response"
)

type API struct{}

var _ = response.Response{}
var _ = model.Withdrawal{}
var _ = res.Page{}

// IncomeList
// @Tags Earning
// @Summary 分页查询统一收益事实
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/income/list [get]
func (API) IncomeList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "income", r)
	respond(c, out, e)
}

// ExchangeList
// @Tags Earning
// @Summary 分页查询结算兑换规则
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/exchange/list [get]
func (API) ExchangeList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "exchange", r)
	respond(c, out, e)
}

// SettlementList
// @Tags Earning
// @Summary 分页查询收益结算
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/settlement/list [get]
func (API) SettlementList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "settlement", r)
	respond(c, out, e)
}

// AccountList
// @Tags Earning
// @Summary 分页查询可提现账户
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/account/list [get]
func (API) AccountList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "account", r)
	respond(c, out, e)
}

// WithdrawalList
// @Tags Earning
// @Summary 分页查询提现申请
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/withdrawal/list [get]
func (API) WithdrawalList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "withdrawal", r)
	respond(c, out, e)
}

// ReconciliationList
// @Tags Earning
// @Summary 分页查询不可变对账快照
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/reconciliation/list [get]
func (API) ReconciliationList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.List(c.Request.Context(), 0, &a, "reconciliation", r)
	respond(c, out, e)
}

// CreateExchange
// @Tags Earning
// @Summary 创建兑换规则新版本
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ExchangeRule true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.SettlementRule}
// @Router /v1/admin/earning/exchange/create [post]
func (API) CreateExchange(c *gin.Context) {
	var r req.ExchangeRule
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.CreateExchangeRule(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// ReviewExchange
// @Tags Earning
// @Summary 复核兑换规则（超级 root ID 1 可自审）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.FinanceReview true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.SettlementRule}
// @Router /v1/admin/earning/exchange/review [post]
func (API) ReviewExchange(c *gin.Context) {
	var r req.FinanceReview
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.ReviewExchangeRule(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// CreateSettlement
// @Tags Earning
// @Summary 生成结算申请和不可变来源明细
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.CreateSettlement true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.Settlement}
// @Router /v1/admin/earning/settlement/create [post]
func (API) CreateSettlement(c *gin.Context) {
	var r req.CreateSettlement
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.CreateSettlement(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// ReviewSettlement
// @Tags Earning
// @Summary 结算复核，超级 root ID 1 可自审并原子记入独立账户
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.FinanceReview true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.Settlement}
// @Router /v1/admin/earning/settlement/review [post]
func (API) ReviewSettlement(c *gin.Context) {
	var r req.FinanceReview
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.ReviewSettlement(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// ReviewWithdrawal
// @Tags Earning
// @Summary 自动对账后审核；通过保持冻结，驳回释放
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.FinanceReview true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.Withdrawal}
// @Router /v1/admin/earning/withdrawal/review [post]
func (API) ReviewWithdrawal(c *gin.Context) {
	var r req.FinanceReview
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.ReviewWithdrawal(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// RunReconciliation
// @Tags Earning
// @Summary 生成不可变提现对账快照
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Reconcile true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=model.Reconciliation}
// @Router /v1/admin/earning/reconciliation/run [post]
func (API) RunReconciliation(c *gin.Context) {
	var r req.Reconcile
	if !bindBody(c, &r) {
		return
	}
	out, e := earningService.ReconcileWithdrawal(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// Overview
// @Tags Earning
// @Summary 收益与各币种可提现余额概览
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.FinanceOverview}
// @Router /v1/admin/earning/overview [get]
func (API) Overview(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.Overview(c.Request.Context(), 0, &a, r)
	respond(c, out, e)
}

// SettlementDetail
// @Tags Earning
// @Summary 结算明细、审计及来源核验
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.SettlementDetail}
// @Router /v1/admin/earning/settlement/detail [get]
func (API) SettlementDetail(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	out, e := earningService.SettlementDetail(c.Request.Context(), actor(c), r.No)
	respond(c, out, e)
}

// WithdrawalDetail
// @Tags Earning
// @Summary 提现详情、冻结流水、审计与对账快照
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.WithdrawalDetail}
// @Router /v1/admin/earning/withdrawal/detail [get]
func (API) WithdrawalDetail(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := earningService.WithdrawalDetail(c.Request.Context(), 0, &a, r.No)
	respond(c, out, e)
}

// AccountLedger
// @Tags Earning
// @Summary 可提现账户不可变流水
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/admin/earning/account/ledger [get]
func (API) AccountLedger(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	out, e := earningService.AccountLedger(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// AccountCheck
// @Tags Earning
// @Summary 只读核查独立账户全链路账务
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=object}
// @Router /v1/admin/earning/account/check [get]
func (API) AccountCheck(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	out, e := earningService.CheckAccount(c.Request.Context(), actor(c), r)
	respond(c, out, e)
}

// AppOverview
// @Tags Earning
// @Summary 本人主播收益与分币种余额
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.FinanceOverview}
// @Router /v1/app/earning/overview [get]
func (API) AppOverview(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.Overview(c.Request.Context(), user, nil, r)
	respond(c, out, e)
}

// AppBalance
// @Tags Earning
// @Summary 本人主播收益与分币种余额
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.FinanceOverview}
// @Router /v1/app/earning/balance [get]
func (API) AppBalance(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.Overview(c.Request.Context(), user, nil, r)
	respond(c, out, e)
}

// AppSettlementList
// @Tags Earning
// @Summary 本人主播结算记录
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/app/earning/settlement/list [get]
func (API) AppSettlementList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.List(c.Request.Context(), user, nil, "settlement", r)
	respond(c, out, e)
}

// AppWithdrawalList
// @Tags Earning
// @Summary 本人主播提现记录
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.FinanceQuery true "参数，金额与版本使用十进制字符串"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/app/earning/withdrawal/list [get]
func (API) AppWithdrawalList(c *gin.Context) {
	var r req.FinanceQuery
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.List(c.Request.Context(), user, nil, "withdrawal", r)
	respond(c, out, e)
}

// AppWithdrawalDetail
// @Tags Earning
// @Summary 本人提现详情，隐藏内部审计及财务快照
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.AppWithdrawalResult true "requestId/currency/amount/payeeAccountNo；金额为最小单位字符串，禁止提交姓名、账号、版本或风险结果"
// @Success 200 {object} response.Response{data=res.AppWithdrawal}
// @Router /v1/app/earning/withdrawal/detail [get]
func (API) AppWithdrawalDetail(c *gin.Context) {
	var r req.AppWithdrawalResult
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.AppWithdrawalResult(c.Request.Context(), user, r)
	respond(c, out, e)
}

// AppApplyWithdrawal
// @Tags Earning
// @Summary 本人选择有效收款账户申请提现，原子保存快照并冻结余额
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.ApplyWithdrawal true "requestId/currency/amount/payeeAccountNo；金额为最小单位字符串，禁止提交姓名、账号、版本或风险结果"
// @Success 200 {object} response.Response{data=res.AppWithdrawal}
// @Router /v1/app/earning/withdrawal/apply [post]
func (API) AppApplyWithdrawal(c *gin.Context) {
	var r req.ApplyWithdrawal
	if !bindBody(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := earningService.ApplyWithdrawal(c.Request.Context(), user, r)
	if e == nil && global.GVA_LOG != nil {
		traceID := trace()
		c.Set("earningTrace", traceID)
		global.GVA_LOG.Info("withdrawal request resolved", zap.String("traceId", traceID), zap.String("withdrawalNo", out.WithdrawalNo), zap.String("requestId", out.RequestID), zap.String("payeeAccountNo", out.PayeeRef), zap.String("payeeVersion", out.PayeeVersion), zap.String("status", out.Status))
	}
	respond(c, res.PublicWithdrawal(out), e)
}
