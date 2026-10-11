package middleware

import (
	"net/http"
	"strings"

	"tb_live_module/config"
	"tb_live_module/global"

	"github.com/gin-gonic/gin"
)

const corsDefaultAllowHeaders = "Content-Type, AccessToken, X-CSRF-Token, Authorization, Token, X-Token, X-User-Id, Idempotency-Key"

// Cors 直接放行所有跨域请求并放行所有 OPTIONS 方法
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		addCorsVaryHeaders(c)
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Headers", corsDefaultAllowHeaders)
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE, PUT, PATCH, HEAD")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type, New-Token, New-Expires-At")
		c.Header("Access-Control-Allow-Credentials", "true")

		// 放行所有OPTIONS方法
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// CorsByRules 按照配置处理跨域请求
func CorsByRules() gin.HandlerFunc {
	// 放行全部
	if global.GVA_CONFIG.Cors.Mode == "allow-all" {
		return Cors()
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// CORS 是浏览器安全策略。SRS hook、健康检查和原生客户端通常没有 Origin，
		// 不能因启用严格白名单而误伤这些服务端请求。
		if origin == "" {
			c.Next()
			return
		}
		addCorsVaryHeaders(c)
		whitelist := checkCors(origin)

		// 通过检查, 添加请求头
		if whitelist != nil {
			c.Header("Access-Control-Allow-Origin", whitelist.AllowOrigin)
			c.Header("Access-Control-Allow-Headers", mergeCorsAllowHeaders(whitelist.AllowHeaders))
			c.Header("Access-Control-Allow-Methods", whitelist.AllowMethods)
			c.Header("Access-Control-Expose-Headers", whitelist.ExposeHeaders)
			if whitelist.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		// 严格白名单模式且未通过检查，直接拒绝处理请求
		if whitelist == nil && global.GVA_CONFIG.Cors.Mode == "strict-whitelist" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		// 预检请求到此已经完成，不能继续进入鉴权或业务处理器。
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func addCorsVaryHeaders(c *gin.Context) {
	c.Header("Vary", "Origin")
	c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
	c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
}

func mergeCorsAllowHeaders(configured string) string {
	required := strings.Split(corsDefaultAllowHeaders, ",")
	configuredHeaders := strings.Split(configured, ",")
	merged := make([]string, 0, len(configuredHeaders)+len(required))
	seen := make(map[string]struct{}, len(configuredHeaders)+len(required))
	appendHeader := func(header string) {
		header = strings.TrimSpace(header)
		if header == "" {
			return
		}
		key := strings.ToLower(header)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, header)
	}
	for _, header := range configuredHeaders {
		appendHeader(header)
	}
	for _, header := range required {
		appendHeader(header)
	}
	return strings.Join(merged, ", ")
}

func checkCors(currentOrigin string) *config.CORSWhitelist {
	for _, whitelist := range global.GVA_CONFIG.Cors.Whitelist {
		// 遍历配置中的跨域头，寻找匹配项
		if currentOrigin == whitelist.AllowOrigin {
			return &whitelist
		}
	}
	return nil
}
