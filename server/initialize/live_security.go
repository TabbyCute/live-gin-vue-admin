package initialize

import (
	"fmt"
	"os"
	"sync"

	"tb_live_module/global"
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

var (
	liveSecurityOnce     sync.Once
	liveSecurityInitErr  error
	srsHookAuthenticator *middleware.SRSHookAuthenticator
)

// InitializeLiveSecurity 在任何数据库、Redis和定时任务启动前冻结直播关键认证配置。
// SRS_HOOK_TOKEN 一旦显式存在（即使为空）就不会回退到配置文件，避免部署变量拼错时静默降级。
func InitializeLiveSecurity() error {
	liveSecurityOnce.Do(func() {
		token, source, err := resolveSRSHookToken(global.GVA_CONFIG.Live.SRSHookToken, os.LookupEnv)
		if err != nil {
			liveSecurityInitErr = err
			return
		}
		authenticator, err := middleware.NewSRSHookAuthenticator(token)
		if err != nil {
			liveSecurityInitErr = fmt.Errorf("%s中的SRS Hook共享密钥无效: %w", source, err)
			return
		}
		srsHookAuthenticator = authenticator
	})
	return liveSecurityInitErr
}

func resolveSRSHookToken(configured string, lookupEnv func(string) (string, bool)) (token string, source string, err error) {
	if value, exists := lookupEnv(middleware.SRSHookTokenEnv); exists {
		if value == "" {
			return "", "", fmt.Errorf("环境变量%s已设置但为空", middleware.SRSHookTokenEnv)
		}
		return value, "环境变量" + middleware.SRSHookTokenEnv, nil
	}
	if configured == "" {
		return "", "", fmt.Errorf("必须通过环境变量%s或live.srs-hook-token配置SRS Hook共享密钥", middleware.SRSHookTokenEnv)
	}
	return configured, "live.srs-hook-token", nil
}

func requireSRSHookAuth() gin.HandlerFunc {
	if srsHookAuthenticator == nil {
		panic("SRS Hook认证器尚未初始化")
	}
	return srsHookAuthenticator.Handler()
}
