package live

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/global"
	"tb_live_module/service"
	gift "tb_live_module/service/gift"
	"tb_live_module/utils"
)

func canReadGiftRevenue(c *gin.Context) bool {
	if !global.GiftFactsReady.Load() {
		return false
	}
	return service.ServiceGroupApp.GiftServiceGroup.CanReadRevenue(c.Request.Context(), gift.Actor{ID: uint64(utils.GetUserID(c)), AuthorityID: utils.GetUserAuthorityId(c)})
}
