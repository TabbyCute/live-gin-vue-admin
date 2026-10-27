package social

import (
	"github.com/gin-gonic/gin"
	api "tb_live_module/api/v1"
	socialAPI "tb_live_module/api/v1/social"
	"tb_live_module/middleware"
)

type RouterGroup struct {
	SocialRouter
	SocialAdminRouter
}
type SocialRouter struct{}

func (r *SocialRouter) InitSocialRouter(root *gin.RouterGroup) {
	group := root.Group("social").Use(middleware.JWTAppAuth(), socialAPI.Guard())
	a := api.ApiGroupApp.SocialApiGroup.SocialApi
	group.POST("follow", a.Follow)
	group.POST("unfollow", a.Unfollow)
	group.POST("block", a.Block)
	group.POST("unblock", a.Unblock)
	group.POST("live/subscribe", a.Subscribe)
	group.POST("live/unsubscribe", a.Unsubscribe)
	group.POST("follow/batch-status", a.FollowBatch)
	group.POST("block/batch-status", a.BlockBatch)
	group.POST("mute/batch-status", a.MuteBatch)
	group.POST("live/subscription/batch-status", a.SubscriptionBatch)
	group.GET("following/list", a.FollowingList)
	group.GET("followers/list", a.FollowersList)
	group.GET("follow/mutual", a.MutualList)
	group.GET("block/list", a.BlockList)
	group.GET("mute/list", a.MuteList)
	group.GET("live/subscription/list", a.SubscriptionList)
	group.GET("follow/status", a.Status)
	group.GET("follow/counters", a.Counters)
	group.GET("follow/common", a.Common)
	group.POST("mute", a.Mute)
	group.POST("unmute", a.Unmute)
	group.POST("favorite", a.Favorite)
	group.POST("unfavorite", a.Unfavorite)
	group.POST("favorite/batch-status", a.FavoriteBatch)
	group.POST("history/open", a.HistoryOpen)
	group.POST("history/report", a.HistoryReport)
	group.GET("history/list", a.HistoryList)
	group.POST("history/progress/batch", a.HistoryProgress)
	group.POST("history/delete", a.HistoryDelete)
	group.POST("history/clear", a.HistoryClear)
	group.GET("live/following-online", a.FollowingOnline)
}
