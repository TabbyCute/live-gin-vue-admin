package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type RoomAdminRouter struct{}

func (r *RoomAdminRouter) InitRoomAdminRouter(privateGroup *gin.RouterGroup) {
	roomRead := privateGroup.Group("live/room")
	roomWrite := privateGroup.Group("live/room").Use(middleware.OperationRecord())
	sessionRead := privateGroup.Group("live/session")
	sessionWrite := privateGroup.Group("live/session").Use(middleware.OperationRecord())
	{
		roomRead.GET("list", roomAdminApi.RoomList)
		roomRead.GET("detail", roomAdminApi.RoomDetail)
		roomWrite.POST("update", roomAdminApi.UpdateRoom)
		roomWrite.POST("status/update", roomAdminApi.UpdateRoomStatus)
	}
	{
		sessionRead.GET("list", roomAdminApi.SessionList)
		sessionRead.GET("detail", roomAdminApi.SessionDetail)
		sessionWrite.POST("end", roomAdminApi.EndSession)
	}
}
