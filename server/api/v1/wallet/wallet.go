package wallet

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/wallet/request"
	walletResponse "tb_live_module/model/wallet/response"
)

type WalletApi struct{}

var _ = walletResponse.Balance{}
var _ = response.Response{}

// Balance
// @Tags Wallet
// @Summary 查询本人COIN余额
// @Description 仅APP Bearer本人身份；整数金额和版本均为十进制字符串，时间为毫秒。未开户返回零余额。业务错误47001至47012。
// @Security AppBearerAuth
// @Produce application/json
// @Param currencyCode query string false "仅COIN，默认COIN"
// @Success 200 {object} response.Response{data=walletResponse.Balance}
// @Failure 401 {object} response.Response
// @Router /v1/app/wallet/balance [get]
func (*WalletApi) Balance(c *gin.Context) {
	var r struct {
		CurrencyCode string `form:"currencyCode"`
	}
	if !bindQuery(c, &r) {
		return
	}
	id, ok := appID(c)
	if !ok {
		return
	}
	data, err := walletService.Balance(c.Request.Context(), id, r.CurrencyCode)
	respond(c, data, err)
}

// Ledger
// @Tags Wallet
// @Summary 游标查询本人余额流水
// @Description limit=1至100，默认20；cursor使用上页nextCursor。无内部备注、凭证和管理员身份。业务错误47001至47012。
// @Security AppBearerAuth
// @Produce application/json
// @Param currencyCode query string false "仅COIN"
// @Param limit query int false "默认20，最大100"
// @Param cursor query string false "不透明游标"
// @Success 200 {object} response.Response{data=walletResponse.LedgerList}
// @Failure 401 {object} response.Response
// @Router /v1/app/wallet/ledger [get]
func (*WalletApi) Ledger(c *gin.Context) {
	var r struct {
		CurrencyCode string `form:"currencyCode"`
		Limit        int    `form:"limit"`
		Cursor       string `form:"cursor"`
	}
	if !bindQuery(c, &r) {
		return
	}
	id, ok := appID(c)
	if !ok {
		return
	}
	data, err := walletService.Ledger(c.Request.Context(), id, req.LedgerSearch{AccountQuery: req.AccountQuery{CurrencyCode: r.CurrencyCode}, Limit: r.Limit, Cursor: r.Cursor})
	respond(c, data, err)
}
