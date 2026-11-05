package payout

import (
	"github.com/gin-gonic/gin"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	"time"
)

func initManualRoutes(root *gin.RouterGroup) {
	a := api.ApiGroupApp.PayoutApiGroup.API
	g := root.Group("payout").Use(guard(8, 8*time.Second), middleware.JWTAuth(), adminRateLimit())
	g.GET("capabilities", a.Capabilities)
	g.GET("channel-account/list", a.CompanyList)
	g.POST("channel-account/create", a.CompanyCreate)
	g.POST("channel-account/enable", a.CompanyEnable)
	g.POST("channel-account/disable", a.CompanyDisable)
	g.POST("order/start", a.ManualStart)
	g.POST("order/retry", a.ManualRetry)
	g.POST("order/release", a.ManualRelease)
	g.POST("attempt/fail-unpaid", a.FailUnpaid)
	g.POST("attempt/bank-submitted", a.BankSubmitted)
	g.POST("attempt/submit", a.ManualSubmit)
	g.POST("attempt/review-success", a.ReviewSuccess)
	g.POST("attempt/review-failure", a.ReviewFailure)
	g.POST("attempt/review-unknown", a.ReviewUnknown)
	g.POST("attempt/resolve-unknown", a.ResolveUnknown)
	g.POST("order/cancel", a.ManualCancel)
	g.POST("order/investigate", a.ManualInvestigate)
	g.POST("order/reconcile", a.ManualReconcile)
	g.POST("order/reveal-payee-grant", sensitiveRateLimit(), a.PayeeGrant)
	g.POST("order/reveal-payee", sensitiveRateLimit(), a.PayeeReveal)
	g.POST("channel-account/reveal-grant", sensitiveRateLimit(), a.CompanyGrant)
	g.POST("channel-account/reveal", sensitiveRateLimit(), a.CompanyReveal)
	g.POST("evidence/read", a.EvidenceRead)
	upload := root.Group("payout/evidence").Use(guardSized(2, 15*time.Second, 3<<20), middleware.JWTAuth(), sensitiveRateLimit())
	upload.POST("upload", a.EvidenceUpload)
}
