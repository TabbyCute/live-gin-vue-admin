package system

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"tb_live_module/model/common/response"
	systemReq "tb_live_module/model/system/request"
	systemRes "tb_live_module/model/system/response"
	svc "tb_live_module/service/system"
)

type operationsResult = systemRes.Operations

var operationsSlots = make(chan struct{}, 2)

// Operations
// @Tags System
// @Summary 单机运行指标与出款积压（仅当前有效root）
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=operationsResult}
// @Failure 403 {object} response.Response
// @Failure 503 {object} response.Response
// @Router /v1/admin/system/operations [get]
func (s *SystemApi) Operations(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims, ok := c.Value("claims").(*systemReq.CustomClaims)
	if !ok || claims == nil || claims.BaseClaims.ID != 1 {
		c.JSON(http.StatusForbidden, response.Response{Code: 403, Msg: "无运行指标访问权限", Data: gin.H{}})
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, response.Response{Code: 400, Msg: "此接口不接收参数", Data: gin.H{}})
		return
	}
	select {
	case operationsSlots <- struct{}{}:
		defer func() { <-operationsSlots }()
	default:
		c.Header("Retry-After", "3")
		c.JSON(503, response.Response{Code: 503, Msg: "运行指标查询繁忙", Data: gin.H{}})
		return
	}
	out, e := systemConfigService.Operations(c.Request.Context(), claims.BaseClaims.ID)
	if e != nil {
		status, msg := 503, "运行指标暂不可用"
		if errors.Is(e, svc.ErrOperationsForbidden) {
			status, msg = 403, "无运行指标访问权限"
		}
		c.JSON(status, response.Response{Code: status, Msg: msg, Data: gin.H{}})
		return
	}
	response.OkWithDetailed(out, "查询成功", c)
}
