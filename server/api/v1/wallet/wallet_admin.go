package wallet

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"tb_live_module/global"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/wallet/request"
	walletResponse "tb_live_module/model/wallet/response"
)

type WalletAdminApi struct{}

var _ = response.Response{}
var _ = walletResponse.Balance{}

// Create
// @Tags WalletAdmin
// @Summary 创建系统人工充值申请
// @Description 需create-authorities权限；requestId为8至64位字母数字下划线横线，同管理员重复提交相同内容幂等。amount必须为正整数字符串；sourceType仅manual（可省略）；externalOrderNo及evidenceRef必填，资金凭证号全局防重、统一大写，驳回或冲正不释放。 默认授权角色888，业务错误47001至47015；金额字符串，时间毫秒。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.CreateAdjustment true "请求参数"
// @Success 200 {object} response.Response{data=walletResponse.Order}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/create [post]
func (*WalletAdminApi) Create(c *gin.Context) {
	var r req.CreateAdjustment
	if !bindBody(c, &r) {
		return
	}
	data, err := walletService.Create(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Approve
// @Tags WalletAdmin
// @Summary 双人审核通过并原子入账
// @Description 普通管理员需review-authorities权限且申请人与审核人必须不同；固定顶级超级管理员userId=1不受钱包角色列表和自审限制。相同审核人、同原因重试幂等；余额、流水、额度、审计及订单原子提交。 默认授权角色888，业务错误47001至47015；金额字符串，时间毫秒。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.ReviewAdjustment true "请求参数"
// @Success 200 {object} response.Response{data=walletResponse.Order}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/approve [post]
func (*WalletAdminApi) Approve(c *gin.Context) {
	var r req.ReviewAdjustment
	if !bindBody(c, &r) {
		return
	}
	data, err := walletService.Approve(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Reject
// @Tags WalletAdmin
// @Summary 双人审核驳回申请
// @Description 普通管理员需review-authorities权限且不能自审；固定顶级超级管理员userId=1不受钱包角色列表和自审限制。冲正申请被驳回后可另建申请。 默认授权角色888，业务错误47001至47015；金额字符串，时间毫秒。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.ReviewAdjustment true "请求参数"
// @Success 200 {object} response.Response{data=walletResponse.Order}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/reject [post]
func (*WalletAdminApi) Reject(c *gin.Context) {
	var r req.ReviewAdjustment
	if !bindBody(c, &r) {
		return
	}
	data, err := walletService.Reject(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Reverse
// @Tags WalletAdmin
// @Summary 创建全额冲正申请
// @Description 需create-authorities权限；此接口不立即扣减余额。另一管理员approve后全额冲正，不支持部分冲正或再次冲正。 默认授权角色888，业务错误47001至47015；金额字符串，时间毫秒。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.ReverseAdjustment true "请求参数"
// @Success 200 {object} response.Response{data=walletResponse.Order}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/reverse [post]
func (*WalletAdminApi) Reverse(c *gin.Context) {
	var r req.ReverseAdjustment
	if !bindBody(c, &r) {
		return
	}
	data, err := walletService.Reverse(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// List
// @Tags WalletAdmin
// @Summary 分页查询人工入账和冲正单
// @Description 校验当前管理员及钱包读权限，默认888；accountNo和phone可选并可同时交叉确认用户。金额和版本字符串，时间毫秒。业务错误47001至47015。
// @Security ApiKeyAuth
// @Produce application/json
// @Param page query int false "默认1，最大偏移10000"
// @Param pageSize query int false "默认20，最大100"
// @Param accountNo query string false "公开用户编号"
// @Param phone query string false "E.164完整手机号；可替代accountNo，两者同时提供时必须属于同一用户"
// @Param orderNo query string false "精确单号"
// @Param status query string false "pending/credited/rejected/reversed"
// @Param kind query string false "grant/reverse"
// @Param sourceType query string false "manual/legacy/reversal"
// @Param externalOrderNo query string false "精确凭证号，省略sourceType时默认manual"
// @Success 200 {object} response.Response{data=response.PageResult}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/list [get]
func (*WalletAdminApi) List(c *gin.Context) {
	var r req.OrderSearch
	if !bindQuery(c, &r) {
		return
	}
	data, err := walletService.Orders(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Detail
// @Tags WalletAdmin
// @Summary 查询入账单及流水和事务审计
// @Description 校验当前管理员及钱包读权限，默认888；金额和版本字符串，时间毫秒。业务错误47001至47015。
// @Security ApiKeyAuth
// @Produce application/json
// @Param orderNo query string true "入账单号"
// @Success 200 {object} response.Response{data=walletResponse.OrderDetail}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/adjustment/detail [get]
func (*WalletAdminApi) Detail(c *gin.Context) {
	var r req.OrderQuery
	if !bindQuery(c, &r) {
		return
	}
	data, err := walletService.Detail(c.Request.Context(), actor(c), r.OrderNo)
	respond(c, data, err)
}

// Balance
// @Tags WalletAdmin
// @Summary 查询指定用户钱包
// @Description 校验当前管理员及钱包读权限，默认888；accountNo和phone至少提供一个，两者同时提供时必须属于同一用户。金额和版本字符串，时间毫秒。业务错误47001至47015。
// @Security ApiKeyAuth
// @Produce application/json
// @Param accountNo query string false "公开用户编号"
// @Param phone query string false "E.164完整手机号"
// @Param currencyCode query string false "仅COIN"
// @Success 200 {object} response.Response{data=walletResponse.Balance}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/account/detail [get]
func (*WalletAdminApi) Balance(c *gin.Context) {
	var r req.AccountQuery
	if !bindQuery(c, &r) {
		return
	}
	data, err := walletService.AdminBalance(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Ledger
// @Tags WalletAdmin
// @Summary 游标查询指定用户流水
// @Description 校验当前管理员及钱包读权限，默认888；accountNo和phone至少提供一个，两者同时提供时必须属于同一用户。金额和版本字符串，时间毫秒。业务错误47001至47015。
// @Security ApiKeyAuth
// @Produce application/json
// @Param accountNo query string false "公开用户编号"
// @Param phone query string false "E.164完整手机号"
// @Param currencyCode query string false "仅COIN"
// @Param limit query int false "默认20，最大100"
// @Param cursor query string false "不透明游标"
// @Success 200 {object} response.Response{data=walletResponse.LedgerList}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/ledger/list [get]
func (*WalletAdminApi) Ledger(c *gin.Context) {
	var r req.LedgerSearch
	if !bindQuery(c, &r) {
		return
	}
	data, err := walletService.AdminLedger(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// Check
// @Tags WalletAdmin
// @Summary 分批只读对账
// @Description 校验当前管理员及钱包读权限，默认888；accountNo和phone至少提供一个，两者同时提供时必须属于同一用户。金额和版本字符串，时间毫秒。业务错误47001至47015。
// @Security ApiKeyAuth
// @Produce application/json
// @Param accountNo query string false "公开用户编号"
// @Param phone query string false "E.164完整手机号"
// @Param currencyCode query string false "仅COIN"
// @Param afterOrderId query string false "起始订单内部游标，首次0；后续使用nextOrderId"
// @Param afterVersion query string false "起始版本，首次0；后续使用nextVersion"
// @Param limit query int false "默认100，最大1000"
// @Success 200 {object} response.Response{data=walletResponse.Check}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/account/check [get]
func (*WalletAdminApi) Check(c *gin.Context) {
	var r req.CheckSearch
	if !bindQuery(c, &r) {
		return
	}
	data, err := walletService.Check(c.Request.Context(), actor(c), r)
	if err == nil && !data.Consistent && global.GVA_LOG != nil {
		global.GVA_LOG.Error("wallet reconciliation inconsistent", zap.String("accountNo", data.AccountNo), zap.Int("issueCount", len(data.Issues)), zap.Uint64("operatorId", actor(c).ID))
	}
	respond(c, data, err)
}

// SetStatus
// @Tags WalletAdmin
// @Summary 启用或停用钱包并记录事务审计
// @Description 需review-authorities权限；normal或disabled，停用阻止入账和冲正；仅操作已开户钱包，无钱包返回47014；注销/软删除账号不能启用。不会修改余额。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.AccountStatus true "状态和原因"
// @Success 200 {object} response.Response{data=walletResponse.Balance}
// @Failure 401 {object} response.Response
// @Router /v1/admin/wallet/account/status [post]
func (*WalletAdminApi) SetStatus(c *gin.Context) {
	var r req.AccountStatus
	if !bindBody(c, &r) {
		return
	}
	data, err := walletService.SetStatus(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}
