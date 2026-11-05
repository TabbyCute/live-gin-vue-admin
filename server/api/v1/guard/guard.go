package guard

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"tb_live_module/internal/httpkit"
	req "tb_live_module/model/guard/request"
	"tb_live_module/service"
)

// Plans
// @Tags guard
// @Summary 可用守护方案
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/guard/plan/list [get]
func (API) Plans(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.Plans(c.Request.Context(), nil, q)
	httpkit.Respond(c, out, e)
}

// Status
// @Tags guard
// @Summary 当前用户守护/公开成员列表
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/guard/status [get]
func (API) Status(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.Members(c.Request.Context(), user, "status", nil, q)
	httpkit.Respond(c, out, e)
}

// Mine
// @Tags guard
// @Summary 当前用户守护/公开成员列表
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/guard/mine [get]
func (API) Mine(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.Members(c.Request.Context(), user, "mine", nil, q)
	httpkit.Respond(c, out, e)
}

// Members
// @Tags guard
// @Summary 当前用户守护/公开成员列表
// @Security AppBearerAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/app/guard/anchor/members [get]
func (API) Members(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.Members(c.Request.Context(), user, "members", nil, q)
	httpkit.Respond(c, out, e)
}

// AdminPlans
// @Tags guard
// @Summary 管理守护方案
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/plan/list [get]
func (API) AdminPlans(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	a := httpkit.Actor(c)
	out, e := service.ServiceGroupApp.GuardServiceGroup.Plans(c.Request.Context(), &a, q)
	httpkit.Respond(c, out, e)
}

// SavePlan
// @Tags guard
// @Summary 保存/启停/排序守护方案
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Plan true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/plan/save [post]
func (API) SavePlan(c *gin.Context) {
	var r req.Plan
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.SavePlan(c.Request.Context(), httpkit.Actor(c), r)
	httpkit.Respond(c, json.RawMessage(out), e)
}

// AdminMemberlist
// @Tags guard
// @Summary 管理守护会员
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/member/list [get]
func (API) AdminMemberlist(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	a := httpkit.Actor(c)
	out, e := service.ServiceGroupApp.GuardServiceGroup.Members(c.Request.Context(), 0, "list", &a, q)
	httpkit.Respond(c, out, e)
}

// AdminMemberdetail
// @Tags guard
// @Summary 管理守护会员
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/member/detail [get]
func (API) AdminMemberdetail(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	a := httpkit.Actor(c)
	out, e := service.ServiceGroupApp.GuardServiceGroup.Members(c.Request.Context(), 0, "detail", &a, q)
	httpkit.Respond(c, out, e)
}

// Admingrant
// @Tags guard
// @Summary 守护来源/补偿/流水/任务
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/grant/list [get]
func (API) Admingrant(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminList(c.Request.Context(), httpkit.Actor(c), "grant", q)
	httpkit.Respond(c, out, e)
}

// Admincompensation
// @Tags guard
// @Summary 守护来源/补偿/流水/任务
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/compensation/list [get]
func (API) Admincompensation(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminList(c.Request.Context(), httpkit.Actor(c), "compensation", q)
	httpkit.Respond(c, out, e)
}

// Adminledger
// @Tags guard
// @Summary 守护来源/补偿/流水/任务
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/ledger/list [get]
func (API) Adminledger(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminList(c.Request.Context(), httpkit.Actor(c), "ledger", q)
	httpkit.Respond(c, out, e)
}

// Admintask
// @Tags guard
// @Summary 守护来源/补偿/流水/任务
// @Security ApiKeyAuth
// @Produce json
// @Param query query req.Query false "分页与筛选，拒绝未知或重复参数"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/task/list [get]
func (API) Admintask(c *gin.Context) {
	var q req.Query
	if !httpkit.Query(c, &q) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminList(c.Request.Context(), httpkit.Actor(c), "task", q)
	httpkit.Respond(c, out, e)
}

// Actiontaskretry
// @Tags guard
// @Summary 守护重试/重建/核验
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Action true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/task/retry [post]
func (API) Actiontaskretry(c *gin.Context) {
	var r req.Action
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminAction(c.Request.Context(), httpkit.Actor(c), "task/retry", r)
	httpkit.Respond(c, json.RawMessage(out), e)
}

// Actionrebuild
// @Tags guard
// @Summary 守护重试/重建/核验
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Action true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/rebuild [post]
func (API) Actionrebuild(c *gin.Context) {
	var r req.Action
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminAction(c.Request.Context(), httpkit.Actor(c), "rebuild", r)
	httpkit.Respond(c, json.RawMessage(out), e)
}

// Actionreconcile
// @Tags guard
// @Summary 守护重试/重建/核验
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param data body req.Action true "永久幂等请求"
// @Success 200 {object} response.Response
// @Router /v1/admin/guard/reconcile [post]
func (API) Actionreconcile(c *gin.Context) {
	var r req.Action
	if !httpkit.Body(c, &r) {
		return
	}
	out, e := service.ServiceGroupApp.GuardServiceGroup.AdminAction(c.Request.Context(), httpkit.Actor(c), "reconcile", r)
	httpkit.Respond(c, json.RawMessage(out), e)
}
