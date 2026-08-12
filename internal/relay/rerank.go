package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/client"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm/transformer"
)

const maxRerankBodyBytes = 16 << 20 // 16 MiB

// RerankHandler 处理 POST /v1/rerank（Jina/Cohere 风格 JSON 透传）。
// 复用分组 LB、keymanager、熔断、统计与中继日志（与主 relay 共享 runState / completeAttempt）。
func RerankHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRerankBodyBytes)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, "failed to read request body")
			return
		}

		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			resp.Error(c, http.StatusBadRequest, "missing or invalid model field")
			return
		}

		// 与主 relay 一致：中间件写入的是逗号分隔 string，不是 []string
		if supportedModels := c.GetString("supported_models"); supportedModels != "" {
			if !slices.Contains(strings.Split(supportedModels, ","), req.Model) {
				resp.Error(c, http.StatusBadRequest, "model not supported")
				return
			}
		}

		group, err := op.GroupGetEnabledMap(req.Model, c.Request.Context())
		if err != nil {
			resp.Error(c, http.StatusNotFound, "model not found")
			return
		}

		apiKeyID := c.GetInt("api_key_id")
		iter, stickyKeyID := balancer.NewIterator(group, apiKeyID, req.Model)
		if iter.Len() == 0 {
			resp.Error(c, http.StatusServiceUnavailable, "no available channel")
			return
		}

		metrics := &RelayMetrics{
			APIKeyID:     apiKeyID,
			RequestModel: req.Model,
			ActualModel:  req.Model,
			StartTime:    time.Now(),
			RawBody:      body,
		}

		(&runState{
			c:            c,
			iter:         iter,
			apiKeyID:     apiKeyID,
			requestModel: req.Model,
			stickyKeyID:  &stickyKeyID,
			record:       metrics,
			tryOne: func(channel *dbmodel.Channel, key dbmodel.ChannelKey, upstreamModel string) (bool, bool, error) {
				return rerankTryOneKey(c, iter, metrics, channel, key, upstreamModel, req.Model, body, apiKeyID)
			},
		}).run()
	}
}

func rerankTryOneKey(
	c *gin.Context,
	iter *balancer.Iterator,
	metrics *RelayMetrics,
	channel *dbmodel.Channel,
	usedKey dbmodel.ChannelKey,
	upstreamModel, requestModel string,
	body []byte,
	apiKeyID int,
) (ok, written bool, err error) {
	// 熔断键与 SkipCircuitBreak 一致，使用上游候选模型名（item.ModelName）
	circuitModel := upstreamModel
	if circuitModel == "" {
		circuitModel = requestModel
	}

	skipped, isProbe := iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name)
	if skipped {
		return false, false, nil
	}
	probeDone := false
	defer func() {
		if isProbe && !probeDone {
			balancer.RecordProbeAborted(channel.ID, usedKey.ID, circuitModel)
		}
	}()

	metrics.ActualModel = upstreamModel
	log.Infof("rerank model %s forwarding to channel: %s model: %s key: %d (sticky=%t)",
		requestModel, channel.Name, upstreamModel, usedKey.ID, iter.IsSticky())

	span := iter.StartAttempt(channel.ID, usedKey.ID, channel.Name)
	statusCode, fwdErr := rerankForward(c.Request.Context(), c, channel, usedKey, upstreamModel, requestModel, body)
	if fwdErr == nil && statusCode == 0 {
		statusCode = http.StatusOK
	}

	written = completeAttempt(c, channel, usedKey, circuitModel, apiKeyID, requestModel, span, statusCode, fwdErr, 0)
	probeDone = true
	if fwdErr == nil {
		return true, false, nil
	}
	return false, written, fmt.Errorf("channel %s failed: %v", channel.Name, fwdErr)
}

func rerankForward(
	ctx context.Context,
	c *gin.Context,
	channel *dbmodel.Channel,
	usedKey dbmodel.ChannelKey,
	upstreamModel, requestModel string,
	body []byte,
) (int, error) {
	httpClient, err := client.ChannelHttpClient(channel)
	if err != nil {
		return 0, err
	}

	// 模型映射失败必须返回错误计入失败/熔断，而不是把未映射的请求透传给上游。
	outBody := body
	if upstreamModel != requestModel {
		var bodyMap map[string]any
		if err := json.Unmarshal(body, &bodyMap); err != nil {
			return 0, fmt.Errorf("failed to parse request body for model mapping: %w", err)
		}
		bodyMap["model"] = upstreamModel
		modified, err := json.Marshal(bodyMap)
		if err != nil {
			return 0, fmt.Errorf("failed to re-encode request body: %w", err)
		}
		outBody = modified
	}
	// 与主 relay 一致的渠道参数覆盖（rerank 为纯 JSON 透传，总是可应用）。
	if channel.ParamOverride != nil && *channel.ParamOverride != "" {
		if modified, ok := applyParamOverride(outBody, *channel.ParamOverride); ok {
			outBody = modified
		} else {
			log.Warnf("failed to apply param_override for channel %s, skipping", channel.Name)
		}
	}

	baseURL := transformer.NormalizeBaseURL(channel.GetBaseUrl(), "v1")
	upstreamURL := baseURL + "/rerank"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(outBody))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if usedKey.ChannelKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+usedKey.ChannelKey)
	}
	applyCustomHeaders(httpReq.Header, channel.CustomHeader)

	upstream, err := httpClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer upstream.Body.Close()

	if upstream.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(io.LimitReader(upstream.Body, 64*1024))
		if readErr != nil {
			return upstream.StatusCode, fmt.Errorf("upstream returned %d (failed to read body: %v)", upstream.StatusCode, readErr)
		}
		return upstream.StatusCode, fmt.Errorf("upstream returned %d: %s", upstream.StatusCode, string(respBody))
	}

	for key, values := range upstream.Header {
		if key == "Content-Length" {
			continue
		}
		for _, v := range values {
			c.Header(key, v)
		}
	}
	c.Status(upstream.StatusCode)
	_, copyErr := io.Copy(c.Writer, upstream.Body)
	if copyErr != nil {
		return upstream.StatusCode, copyErr
	}
	return upstream.StatusCode, nil
}
