package live

import "github.com/gin-gonic/gin"

type CategoryRouter struct{}

func (r *CategoryRouter) InitCategoryRouter(publicGroup *gin.RouterGroup) {
	categoryRouter := publicGroup.Group("category")
	{
		categoryRouter.GET("list", categoryApi.List)
	}
}
