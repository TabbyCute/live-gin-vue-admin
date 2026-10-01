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
		public.GET("detail", roomApi.PublicDetail) // [不鉴权] 按房间号获取公开直播间。返回：直播间信息 + 场次号；
		public.GET("list", roomApi.PublicList)     // [不鉴权] 分页获取公开直播列表。
	}
	{
		private.GET("info", roomApi.Info)                        // [主播自己鉴权] 主播查询自己的直播间，返回：直播间+当前场次。
		private.POST("save", roomApi.Save)                       // [主播自己鉴权] 主播修改自己的直播间信息
		private.POST("status/update", roomApi.UpdateOwnerStatus) // [主播自己鉴权] 主播关闭或重新开启直播间。主播只允许在正常(1)和关闭(2)之间切换；活动场次存在时不能关闭。
		private.POST("session/prepare", roomApi.Prepare)         // [主播自己鉴权] 准备开播。返回：roomNo，sessionNo，streamName，publishToken，pushUrl，playUrl。
		private.GET("session/current", roomApi.Current)          // [主播自己鉴权] 返回当前场次。
		private.POST("session/end", roomApi.End)                 // [主播自己鉴权] 结束当前活动场次，无请求体。
		private.GET("session/history", roomApi.History)          // [主播自己鉴权] 分页查询主播的历史场次。
	}
}
