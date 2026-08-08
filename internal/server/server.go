package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/relay"
	_ "github.com/bestruirui/octopus/internal/server/handlers"
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
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
		c.Abort()
	}))

	if conf.IsDebug() {
		r.Use(middleware.Logger())
	}
	r.Use(middleware.Cors())
	r.Use(middleware.StaticEmbed("/", static.StaticFS))

	registerRelayRoutes(r)
	router.RegisterAll(r)

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
