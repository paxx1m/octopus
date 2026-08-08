package middleware

import (
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Cors() gin.HandlerFunc {
	config := cors.DefaultConfig()
	// 管理端使用 Authorization Bearer，不依赖 cookie；关闭 credentials
	// 避免与 "*" 白名单组合时违反浏览器规范。
	config.AllowCredentials = false
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"*"}
	config.ExposeHeaders = []string{"Content-Disposition"}
	// CORS 白名单:
	// - 为空: 不允许跨域
	// - "*": 允许所有来源
	// - 逗号分隔的域名列表: 只允许指定的域名
	config.AllowOriginFunc = func(origin string) bool {
		allowed, err := op.SettingGetString(model.SettingKeyCORSAllowOrigins)
		if err != nil {
			return false
		}
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			return false
		}
		if allowed == "*" {
			return true
		}

		origin = strings.TrimSpace(origin)
		if origin == "" {
			return false
		}

		originHost := origin
		if idx := strings.Index(origin, "://"); idx != -1 {
			originHost = origin[idx+3:]
		}
		originHost = strings.TrimRight(originHost, "/")

		for _, item := range strings.Split(allowed, ",") {
			item = strings.TrimSpace(item)
			item = strings.TrimRight(item, "/")
			if item == "" {
				continue
			}
			if item == origin || item == originHost {
				return true
			}
		}
		return false
	}
	return cors.New(config)
}
