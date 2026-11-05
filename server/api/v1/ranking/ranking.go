package ranking

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"tb_live_module/internal/httpkit"
	req "tb_live_module/model/ranking/request"
	"tb_live_module/service"
)

// AdminBoard
// @Tags ranking
// @Summary 管理员查询净礼物消费榜
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "榜单类型、范围、周期和分页"
// @Success 200 {object} response.Response
// @Router /v1/admin/ranking/admin/board [get]
func (API) AdminBoard(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.AdminBoard(c.Request.Context(), httpkit.Actor(c), q)
	httpkit.Respond(c, out, e)
}

// Boardgift
// @Tags ranking
// @Summary 净礼物消费榜
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/ranking/gift [get]
func (API) Boardgift(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.Board(c.Request.Context(), user, "gift", q)
	httpkit.Respond(c, out, e)
}

// Boardwealth
// @Tags ranking
// @Summary 净礼物消费榜
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/ranking/wealth [get]
func (API) Boardwealth(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.Board(c.Request.Context(), user, "wealth", q)
	httpkit.Respond(c, out, e)
}

// Boardcharm
// @Tags ranking
// @Summary 净礼物消费榜
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/ranking/charm [get]
func (API) Boardcharm(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.Board(c.Request.Context(), user, "charm", q)
	httpkit.Respond(c, out, e)
}

// Status
// @Tags ranking
// @Summary 榜单进度与积压
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/ranking/admin/status [get]
func (API) Status(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.Status(c.Request.Context(), httpkit.Actor(c))
	httpkit.Respond(c, out, e)
}

// Jobs
// @Tags ranking
// @Summary 榜单重建任务
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/ranking/admin/job/list [get]
func (API) Jobs(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.Jobs(c.Request.Context(), httpkit.Actor(c), q)
	httpkit.Respond(c, out, e)
}

// Actionrebuild
// @Tags ranking
// @Summary 重建/核验礼物榜单
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Action true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/ranking/admin/rebuild [post]
func (API) Actionrebuild(c *gin.Context) {
	var r req.Action
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.AdminAction(c.Request.Context(), httpkit.Actor(c), "rebuild", r)
	httpkit.Respond(c, json.RawMessage(out), e)
}

// Actionreconcile
// @Tags ranking
// @Summary 重建/核验礼物榜单
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Action true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/ranking/admin/reconcile [post]
func (API) Actionreconcile(c *gin.Context) {
	var r req.Action
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.RankingServiceGroup.AdminAction(c.Request.Context(), httpkit.Actor(c), "reconcile", r)
	httpkit.Respond(c, json.RawMessage(out), e)
}
