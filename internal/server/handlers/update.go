package handlers

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/update"
	"github.com/gin-gonic/gin"
)

func RegisterUpdateRoutes() []*router.GroupRouter {
	return []*router.GroupRouter{
		router.NewGroupRouter("/api/v1/update").
			Use(middleware.Auth()).
			AddRoute(
				router.NewRoute("", http.MethodGet).
					Handle(latest),
			).
			AddRoute(
				router.NewRoute("/now-version", http.MethodGet).
					Handle(getNowVersion),
			).
			AddRoute(
				router.NewRoute("", http.MethodPost).
					Handle(updateFunc),
			),
	}
}

func latest(c *gin.Context) {
	latestInfo, err := update.GetLatestInfo()
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, *latestInfo)
}

func getNowVersion(c *gin.Context) {
	resp.Success(c, conf.Version)
}

func updateFunc(c *gin.Context) {
	if err := update.UpdateCore(); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, "update success")
}
