package handlers

import (
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func RegisterModelRoutes() []*router.GroupRouter {
	return []*router.GroupRouter{
		router.NewGroupRouter("/api/v1/model").
			Use(middleware.Auth()).
			Use(middleware.RequireJSON()).
			AddRoute(
				router.NewRoute("/list", http.MethodGet).
					Handle(listLLM),
			).
			AddRoute(
				router.NewRoute("/create", http.MethodPost).
					Handle(createLLM),
			).
			AddRoute(
				router.NewRoute("/channel", http.MethodGet).
					Handle(listLLMByChannel),
			).
			AddRoute(
				router.NewRoute("/update", http.MethodPost).
					Handle(updateLLM),
			).
			AddRoute(
				router.NewRoute("/delete", http.MethodPost).
					Handle(deleteLLM),
			).
			AddRoute(
				router.NewRoute("/update-price", http.MethodPost).
					Handle(updateLLMPrice),
			).
			AddRoute(
				router.NewRoute("/last-update-time", http.MethodGet).
					Handle(getLastUpdateTime),
			),
		router.NewGroupRouter("/v1").
			Use(middleware.APIKeyAuth()).
			AddRoute(
				router.NewRoute("/models", http.MethodGet).
					Handle(getModelList),
			),
	}
}

func getModelList(c *gin.Context) {
	models, err := op.GroupListModel(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	apiKeyId := c.GetInt("api_key_id")
	apiKey, err := op.APIKeyGet(apiKeyId, c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	if apiKey.SupportedModels != "" {
		supportedModels := lo.Map(strings.Split(apiKey.SupportedModels, ","), func(s string, _ int) string {
			return strings.TrimSpace(s)
		})
		models = lo.Filter(models, func(m string, _ int) bool {
			return lo.Contains(supportedModels, m)
		})
	}

	if c.GetString("request_type") == "anthropic" {
		var anthropicModels []model.AnthropicModel
		for _, m := range models {
			anthropicModels = append(anthropicModels, model.AnthropicModel{
				ID:          m,
				CreatedAt:   "2024-01-01T00:00:00Z",
				DisplayName: m,
				Type:        "model",
			})
		}
		response := gin.H{
			"data":     anthropicModels,
			"has_more": false,
		}
		if len(anthropicModels) > 0 {
			response["first_id"] = anthropicModels[0].ID
			response["last_id"] = anthropicModels[len(anthropicModels)-1].ID
		}
		c.JSON(200, response)
	} else {
		var openAIModels []model.OpenAIModel
		for _, m := range models {
			openAIModels = append(openAIModels, model.OpenAIModel{
				ID:      m,
				Object:  "model",
				Created: 1763395200,
				OwnedBy: "octopus",
			})
		}
		c.JSON(200, gin.H{
			"success": true,
			"data":    openAIModels,
			"object":  "list",
		})
	}
}

func listLLM(c *gin.Context) {
	models, err := op.LLMList(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, models)
}

func listLLMByChannel(c *gin.Context) {
	channels, err := op.ChannelLLMList(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, channels)
}

func createLLM(c *gin.Context) {
	var info model.LLMInfo
	if !bindJSON(c, &info) {
		return
	}
	if err := op.LLMCreate(info, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, info)
}

func updateLLM(c *gin.Context) {
	var info model.LLMInfo
	if !bindJSON(c, &info) {
		return
	}
	if err := op.LLMUpdate(info, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, info)
}

func deleteLLM(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := op.LLMDelete(req.Name, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, nil)
}

func updateLLMPrice(c *gin.Context) {
	if err := price.UpdateLLMPrice(c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, nil)
}

func getLastUpdateTime(c *gin.Context) {
	time := price.GetLastUpdateTime()
	resp.Success(c, time)
}
