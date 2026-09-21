package middleware

import (
	"errors"
	"strings"

	"tb_live_module/global"
	"tb_live_module/model/common/response"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const appClaimsContextKey = "app-claims"

// JWTAppAuth 校验客户端 Authorization Bearer token。
// 客户端 JWT 使用独立配置和 Claims，不读取管理后台的 x-token 或 Cookie。
func JWTAppAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := appBearerToken(c.GetHeader("Authorization"))
		if !ok {
			response.NoAuth("未登录或客户端 token 格式错误", c)
			c.Abort()
			return
		}

		claims, err := utils.NewAppJWT().ParseToken(token)
		if err != nil {
			if errors.Is(err, utils.ErrAppTokenExpired) {
				response.NoAuth("登录已过期，请重新登录", c)
			} else {
				response.NoAuth("客户端 token 无效", c)
			}
			c.Abort()
			return
		}

		if global.GVA_DB == nil {
			response.NoAuth("客户端鉴权服务不可用", c)
			c.Abort()
			return
		}
		var account liveModel.LiveAccount
		err = global.GVA_DB.Select("id", "status").First(&account, claims.AccountID).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				global.GVA_LOG.Error("客户端鉴权查询账号失败", zap.Error(err))
			}
			response.NoAuth("客户端账号不存在或已失效", c)
			c.Abort()
			return
		}
		if account.Status != liveModel.LiveAccountStatusNormal {
			response.NoAuth("客户端账号已被禁用", c)
			c.Abort()
			return
		}

		c.Set(appClaimsContextKey, claims)
		c.Next()
	}
}

func GetAppClaims(c *gin.Context) (*liveReq.AppClaims, bool) {
	value, exists := c.Get(appClaimsContextKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*liveReq.AppClaims)
	return claims, ok
}

func GetAppAccountID(c *gin.Context) (uint, bool) {
	claims, ok := GetAppClaims(c)
	if !ok || claims.AccountID == 0 {
		return 0, false
	}
	return claims.AccountID, true
}

func appBearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
