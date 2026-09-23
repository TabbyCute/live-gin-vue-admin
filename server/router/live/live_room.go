package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type RoomRouter struct{}

func (r *RoomRouter) InitRoomRouter(publicGroup, privateGroup *gin.RouterGroup) {
	public := publicGroup.Group("room")
	private := privateGroup.Group("room").Use(middleware.JWTAppAuth())
	{
		public.GET("detail", roomApi.PublicDetail)
		public.GET("list", roomApi.PublicList)
	}
	{
		private.GET("info", roomApi.Info)
		private.POST("save", roomApi.Save)
		private.POST("status/update", roomApi.UpdateOwnerStatus)
		private.POST("session/prepare", roomApi.Prepare)
		private.GET("session/current", roomApi.Current)
		private.POST("session/end", roomApi.End)
		private.GET("session/history", roomApi.History)
	}
}
