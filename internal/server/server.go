package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/handlers"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/static"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
)

var httpSrv http.Server

const maxRelayBodyBytes = 32 << 20 // 32 MiB

func Start() error {
	if conf.IsDebug() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 默认不信任任何代理，防止伪造 X-Forwarded-For 绕过基于 ClientIP 的限流；
	// 部署在反向代理后时通过 server.trusted_proxies 显式信任代理网段。
	if err := r.SetTrustedProxies(conf.AppConfig.Server.TrustedProxies); err != nil {
		log.Warnf("invalid server.trusted_proxies, ignoring client IP headers: %v", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
		c.Abort()
	}))

	if conf.IsDebug() {
		r.Use(gin.Logger())
	}
	r.Use(middleware.Cors())
	r.Use(middleware.StaticEmbed("/", static.StaticFS))

	registerRelayRoutes(r)
	if err := router.RegisterAll(r, adminRoutes()); err != nil {
		return err
	}

	httpSrv.Addr = fmt.Sprintf("%s:%d", conf.AppConfig.Server.Host, conf.AppConfig.Server.Port)
	httpSrv.Handler = r
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("http server listen and serve error: %v", err)
		}
	}()
	return nil
}

// Close 优雅关闭 HTTP 服务，等待进行中的请求结束（最长 30s）。
func Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

// adminRoutes 显式装配各 handler 模块的路由（不再依赖 init() 全局副作用）。
func adminRoutes() []*router.GroupRouter {
	var groups []*router.GroupRouter
	groups = append(groups, handlers.RegisterUserRoutes()...)
	groups = append(groups, handlers.RegisterChannelRoutes()...)
	groups = append(groups, handlers.RegisterGroupRoutes()...)
	groups = append(groups, handlers.RegisterAPIKeyRoutes()...)
	groups = append(groups, handlers.RegisterModelRoutes()...)
	groups = append(groups, handlers.RegisterSettingRoutes()...)
	groups = append(groups, handlers.RegisterStatsRoutes()...)
	groups = append(groups, handlers.RegisterHealthRoutes()...)
	groups = append(groups, handlers.RegisterLogRoutes()...)
	groups = append(groups, handlers.RegisterUpdateRoutes()...)
	return groups
}

func registerRelayRoutes(r *gin.Engine) {
	v1 := r.Group("/v1", middleware.APIKeyAuth(), middleware.MaxBodyBytes(maxRelayBodyBytes))
	v1.POST("/chat/completions", middleware.RequireJSON(), relay.Handler(llm.APIFormatOpenAIChatCompletion))
	v1.POST("/responses", middleware.RequireJSON(), relay.Handler(llm.APIFormatOpenAIResponse))
	v1.POST("/messages", middleware.RequireJSON(), relay.Handler(llm.APIFormatAnthropicMessage))
	v1.POST("/embeddings", middleware.RequireJSON(), relay.Handler(llm.APIFormatOpenAIEmbedding))
	v1.POST("/rerank", middleware.RequireJSON(), relay.RerankHandler())
	v1.POST("/images/generations", middleware.RequireJSON(), relay.Handler(llm.APIFormatOpenAIImageGeneration))
	v1.POST("/images/edits", relay.Handler(llm.APIFormatOpenAIImageEdit))
	v1.POST("/images/variations", relay.Handler(llm.APIFormatOpenAIImageVariation))
}
