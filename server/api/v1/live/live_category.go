package live

import (
	"tb_live_module/global"
	commonRes "tb_live_module/model/common/response"
	liveRes "tb_live_module/model/live/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CategoryApi struct{}

// Keep the response package imported for Swagger schema resolution.
var _ = liveRes.LiveCategoryPublicItem{}

// List
// @Tags        LiveCategoryApp
// @Summary     获取启用的直播分类树
// @Description 公开接口，只返回启用分类；父分类停用后，其子分类不会出现在客户端分类树中。
// @Description 分类 id 是主播 categoryId 的公开合法取值，不属于需要隐藏的主播数据库主键。
// @Produce     application/json
// @Success     200  {object}  commonRes.Response{data=[]liveRes.LiveCategoryPublicItem}  "获取成功"
// @Failure     200  {object}  commonRes.Response                                          "查询失败"
// @Router      /v1/app/live/category/list [get]
func (a *CategoryApi) List(c *gin.Context) {
	result, err := categoryService.GetPublicCategoryTree()
	if err != nil {
		global.GVA_LOG.Error("获取直播分类失败", zap.Error(err))
		commonRes.FailWithMessage("获取直播分类失败", c)
		return
	}
	commonRes.OkWithDetailed(result, "获取成功", c)
}
