package social

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
)

type SocialAdminApi struct{}

var _ = response.Response{}
var _ = res.CounterAudit{}

// Check
// @Tags SocialAdmin
// @Summary 核对指定账号的关系事实与计数
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param accountNo query string false "用户公开编号，与anchorNo二选一"
// @Param anchorNo query string false "主播公开编号"
// @Success 200 {object} response.Response{data=res.CounterAudit}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/counter/check [get]
func (a *SocialAdminApi) Check(c *gin.Context) {
	var r req.Target
	if !bind(c, &r) {
		return
	}
	data, err := socialService.AuditCounters(c.Request.Context(), r, false)
	respond(c, data, err)
}

// Repair
// @Tags SocialAdmin
// @Summary 持锁重算计数，需REPAIR确认及审计原因
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.CounterRepair true "参数"
// @Success 200 {object} response.Response{data=res.CounterAudit}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/counter/repair [post]
func (a *SocialAdminApi) Repair(c *gin.Context) {
	var r req.CounterRepair
	if !bind(c, &r) {
		return
	}
	data, err := socialService.AuditCounters(c.Request.Context(), r.Target, true)
	respond(c, data, err)
}

// Events
// @Tags SocialAdmin
// @Summary 有界查询事件积压和失败记录
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "不透明游标"
// @Param limit query int false "1至100，默认20"
// @Success 200 {object} response.Response{data=res.EventList}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/event/list [get]
func (a *SocialAdminApi) Events(c *gin.Context) {
	var r req.CursorSearch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.EventList(c.Request.Context(), r)
	respond(c, data, err)
}

// Replay
// @Tags SocialAdmin
// @Summary 重放失败事件，保留投递去重记录
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.EventReplay true "参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/event/replay [post]
func (a *SocialAdminApi) Replay(c *gin.Context) {
	var r req.EventReplay
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ReplayEvent(c.Request.Context(), r.EventID)
	respond(c, data, err)
}

// Blocks
// @Tags SocialAdmin
// @Summary 授权管理员审计指定用户黑名单
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param accountNo query string false "用户公开编号，与anchorNo二选一"
// @Param anchorNo query string false "主播公开编号"
// @Param cursor query string false "不透明游标"
// @Param limit query int false "1至100，默认20"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/block/list [get]
func (a *SocialAdminApi) Blocks(c *gin.Context) {
	var r req.CommonSearch
	if !bind(c, &r) {
		return
	}
	id, err := socialService.Resolve(c.Request.Context(), r.Target)
	if err != nil {
		respond(c, nil, err)
		return
	}
	data, err := socialService.ListUsers(c.Request.Context(), id, "block", r.CursorSearch, 0)
	respond(c, data, err)
}

// Anomalies
// @Tags SocialAdmin
// @Summary 扫描拉黑与关注并存的异常关系
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "不透明游标"
// @Param limit query int false "1至100，默认20"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/relationship/check [get]
func (a *SocialAdminApi) Anomalies(c *gin.Context) {
	var r req.CursorSearch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.Anomalies(c.Request.Context(), r)
	respond(c, data, err)
}

// Health
// @Tags SocialAdmin
// @Summary 查询事件积压、投递渠道状态和实例指标
// @Description 每种状态最多统计1000条；countIsLowerBound=true表示总数大于1000，sampleOldestCreatedAt仅为样本最早毫秒时间，不是全局最早时间。
// @Description 仅高级管理员角色888，后台JWT、服务端权限、操作审计同时生效。错误码46001至46010与APP一致；时间戳毫秒。示例accountNo=10000001。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Success 200 {object} response.Response{data=object}
// @Failure 401 {object} response.Response "未登录或角色无权限"
// @Router /v1/admin/social/health [get]
func (a *SocialAdminApi) Health(c *gin.Context) {
	data, err := socialService.Health(c.Request.Context())
	respond(c, data, err)
}
