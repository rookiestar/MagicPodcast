package middleware

import (
	"magicpodcast/internal/config"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 中间件 - 安全的跨域资源共享配置
func CORS() gin.HandlerFunc {
	cfg := config.Get().Server.CORS

	// 如果CORS未启用，返回空中间件
	if !cfg.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 检查origin是否在允许列表中
		allowed := false
		for _, allowedOrigin := range cfg.AllowOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
			// 支持通配符域名匹配，如 *.example.com
			if strings.HasPrefix(allowedOrigin, "*.") {
				domain := strings.TrimPrefix(allowedOrigin, "*.")
				if strings.HasSuffix(origin, "."+domain) {
					allowed = true
					break
				}
			}
		}

		if allowed {
			if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
				// 只有在明确配置允许所有域名时才使用 *
				// 注意：当AllowCredentials为true时，不能使用 *
				if !cfg.AllowCredentials {
					c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					// 当需要credentials时，必须使用具体的origin
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				}
			} else {
				// 使用请求的origin
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}

			// 设置其他CORS头
			if cfg.AllowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(cfg.AllowHeaders) > 0 {
				c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			}

			if len(cfg.AllowMethods) > 0 {
				c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
			}

			if len(cfg.ExposeHeaders) > 0 {
				c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
			}

			if cfg.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
		}

		// 处理OPTIONS预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
