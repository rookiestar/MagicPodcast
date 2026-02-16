package middleware

import (
	"magicpodcast/internal/config"
	"magicpodcast/internal/logger"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 中间件 - 安全的跨域资源共享配置
func CORS() gin.HandlerFunc {
	log := logger.GetLogger()

	return func(c *gin.Context) {
		// 每次请求时获取配置（确保使用最新配置）
		cfg := config.Get().Server.CORS
		origin := c.Request.Header.Get("Origin")

		// 调试日志：打印请求信息和配置状态
		log.Debugf("[CORS] method=%s, path=%s, origin=%s, enabled=%v, allowOrigins=%v",
			c.Request.Method, c.Request.URL.Path, origin, cfg.Enabled, cfg.AllowOrigins)

		// 如果CORS未启用，跳过
		if !cfg.Enabled {
			log.Debug("[CORS] disabled, skipping")
			c.Next()
			return
		}

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
			// 支持 https://*.example.com 格式
			if strings.HasPrefix(allowedOrigin, "https://*.") || strings.HasPrefix(allowedOrigin, "http://*.") {
				protocol := "https://"
				if strings.HasPrefix(allowedOrigin, "http://*.") {
					protocol = "http://"
				}
				domain := strings.TrimPrefix(allowedOrigin, protocol+"*.")
				if strings.HasPrefix(origin, protocol) && strings.HasSuffix(origin, "."+domain) {
					allowed = true
					break
				}
				// 也支持直接匹配
				if origin == strings.Replace(allowedOrigin, "*.", "", 1) {
					allowed = true
					break
				}
			}
		}

		if !allowed && origin != "" {
			log.Infof("CORS: origin %s not in allow list %v", origin, cfg.AllowOrigins)
		}

		if allowed {
			log.Debugf("[CORS] origin %s is allowed, setting headers", origin)
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
