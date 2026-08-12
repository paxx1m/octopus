package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/health"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func RegisterHealthRoutes() []*router.GroupRouter {
	return []*router.GroupRouter{
		router.NewGroupRouter("/api/v1/health").
			Use(middleware.Auth()).
			AddRoute(
				router.NewRoute("/list", http.MethodGet).
					Handle(listHealth),
			).
			AddRoute(
				router.NewRoute("/stream-token", http.MethodGet).
					Handle(getHealthStreamToken),
			),
		router.NewGroupRouter("/api/v1/health").
			AddRoute(
				router.NewRoute("/stream", http.MethodGet).
					Handle(streamHealth),
			),
	}
}

func parseHealthBuildOpts(c *gin.Context) health.BuildOptions {
	opts := health.BuildOptions{}
	if g := c.Query("group_id"); g != "" {
		if id, err := strconv.Atoi(g); err == nil {
			opts.GroupID = id
		}
	}
	abnormal := c.DefaultQuery("abnormal_only", "1")
	opts.AbnormalOnly = abnormal == "1" || abnormal == "true"
	return opts
}

func listHealth(c *gin.Context) {
	opts := parseHealthBuildOpts(c)
	snap := health.BuildSnapshot(opts)
	resp.Success(c, snap)
}

func getHealthStreamToken(c *gin.Context) {
	token, err := op.HealthStreamTokenCreate()
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, gin.H{"token": token})
}

func streamHealth(c *gin.Context) {
	token := c.Query("token")
	if token == "" || !op.HealthStreamTokenVerify(token) {
		resp.Error(c, http.StatusUnauthorized, "invalid stream token")
		return
	}
	op.HealthStreamTokenRevoke(token)

	opts := parseHealthBuildOpts(c)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	notify := op.HealthSubscribe()
	defer op.HealthUnsubscribe(notify)

	ctx := c.Request.Context()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	// periodic full snapshot to correct any missed coalesced events / countdown drift
	resync := time.NewTicker(30 * time.Second)
	defer resync.Stop()

	writeSnapshot := func(event string) bool {
		snap := health.BuildSnapshot(opts)
		data, err := json.Marshal(snap)
		if err != nil {
			return true
		}
		if _, err := c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}

	if !writeSnapshot("snapshot") {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-notify:
			if !ok {
				return
			}
			if !writeSnapshot("snapshot") {
				return
			}
		case <-resync.C:
			if !writeSnapshot("snapshot") {
				return
			}
		case <-ping.C:
			if _, err := c.Writer.Write([]byte("event: ping\ndata: {}\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
