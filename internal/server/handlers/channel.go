package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/modelfetch"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/gin-gonic/gin"
)

func RegisterChannelRoutes() []*router.GroupRouter {
	return []*router.GroupRouter{
		router.NewGroupRouter("/api/v1/channel").
			Use(middleware.Auth()).
			Use(middleware.RequireJSON()).
			AddRoute(
				router.NewRoute("/list", http.MethodGet).
					Handle(listChannel),
			).
			AddRoute(
				router.NewRoute("/create", http.MethodPost).
					Handle(createChannel),
			).
			AddRoute(
				router.NewRoute("/update", http.MethodPost).
					Handle(updateChannel),
			).
			AddRoute(
				router.NewRoute("/enable", http.MethodPost).
					Handle(enableChannel),
			).
			AddRoute(
				router.NewRoute("/delete/:id", http.MethodDelete).
					Handle(deleteChannel),
			).
			AddRoute(
				router.NewRoute("/fetch-model", http.MethodPost).
					Handle(fetchModel),
			),
		router.NewGroupRouter("/api/v1/channel").
			Use(middleware.Auth()).
			AddRoute(
				router.NewRoute("/sync", http.MethodPost).
					Handle(syncChannel),
			).
			AddRoute(
				router.NewRoute("/last-sync-time", http.MethodGet).
					Handle(getLastSyncTime),
			),
	}
}

func listChannel(c *gin.Context) {
	channels, err := op.ChannelList(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	for i, channel := range channels {
		stats := op.StatsChannelGet(channel.ID)
		channels[i].Stats = &stats
	}
	resp.Success(c, channels)
}

func createChannel(c *gin.Context) {
	var channel model.Channel
	if !bindJSON(c, &channel) {
		return
	}
	if err := op.ChannelCreate(&channel, c.Request.Context()); err != nil {
		handleWriteError(c, err)
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	channelPostProcess(&channel)
	resp.Success(c, channel)
}

func updateChannel(c *gin.Context) {
	var req model.ChannelUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	channel, err := op.ChannelUpdate(&req, c.Request.Context())
	if err != nil {
		handleWriteError(c, err)
		return
	}
	for _, keyID := range req.KeysToDelete {
		balancer.ClearStickyByKey(keyID)
	}
	for _, ku := range req.KeysToUpdate {
		if ku.Enabled != nil && !*ku.Enabled {
			balancer.ClearStickyByKey(ku.ID)
		}
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	channelPostProcess(channel)
	resp.Success(c, channel)
}

// channelPostProcess 渠道创建/更新后的异步后处理：价格入库、延迟探测、自动分组。
func channelPostProcess(channel *model.Channel) {
	go func(ch *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := ch.Model + "," + ch.CustomModel
		modelArray := strings.Split(modelStr, ",")
		price.AddModelsToDB(modelArray, ctx)
		task.ChannelBaseUrlDelayUpdate(ch, ctx)
		op.ChannelAutoGroup(ch, ctx)
	}(channel)
}

func enableChannel(c *gin.Context) {
	var request struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if !bindJSON(c, &request) {
		return
	}
	if err := op.ChannelEnabled(request.ID, request.Enabled, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	if !request.Enabled {
		balancer.ClearStickyByChannel(request.ID)
	}
	resp.Success(c, nil)
}

func deleteChannel(c *gin.Context) {
	idNum, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := op.ChannelDel(idNum, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	balancer.ClearStickyByChannel(idNum)
	keymanager.CleanupChannel(idNum)
	resp.Success(c, nil)
}
func fetchModel(c *gin.Context) {
	var request model.Channel
	if !bindJSON(c, &request) {
		return
	}
	models, err := modelfetch.FetchModels(c.Request.Context(), request)
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, models)
}

func syncChannel(c *gin.Context) {
	// 异步触发，避免请求阻塞最长 30 分钟；前端通过 last-sync-time 轮询完成状态。
	if !task.TriggerSyncModels() {
		resp.Error(c, http.StatusConflict, "model sync already in progress")
		return
	}
	resp.Success(c, nil)
}

func getLastSyncTime(c *gin.Context) {
	time := task.GetLastSyncModelsTime()
	resp.Success(c, time)
}
