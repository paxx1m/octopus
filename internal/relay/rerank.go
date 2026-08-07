package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer"
)

// RerankHandler handles POST /v1/rerank (Jina/Cohere-style JSON pass-through).
// Reuses group LB, keymanager, circuit breaker, stats and relay logs.
func RerankHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		startTime := time.Now()
		ctx := c.Request.Context()
		var lastErr error

		for iter.Next() {
			if ctx.Err() != nil {
				log.Infof("rerank request context canceled, stopping retry")
				saveRerankMetrics(ctx, apiKeyID, req.Model, req.Model, startTime, false, context.Canceled, iter.Attempts(), 0)
				return
			}

			ok, written, err := rerankTryChannel(c, iter, stickyKeyID, apiKeyID, req.Model, body)
			if ok {
				item := iter.Item()
				saveRerankMetrics(ctx, apiKeyID, req.Model, item.ModelName, startTime, true, nil, iter.Attempts(), item.ChannelID)
				return
			}
			if written {
				// 已向客户端写出部分内容：不能换渠道，仍记失败（如 copy 中途断开）
				actual := req.Model
				channelID := 0
				if item := iter.Item(); item.ChannelID > 0 {
					channelID = item.ChannelID
					if item.ModelName != "" {
						actual = item.ModelName
					}
				}
				if err == nil {
					err = errors.New("response partially written")
				}
				saveRerankMetrics(ctx, apiKeyID, req.Model, actual, startTime, false, err, iter.Attempts(), channelID)
				return
			}
			if err != nil {
				lastErr = err
			}
		}

		if lastErr == nil {
			lastErr = errors.New("all channels failed")
		}
		saveRerankMetrics(ctx, apiKeyID, req.Model, req.Model, startTime, false, lastErr, iter.Attempts(), 0)
		if !c.Writer.Written() {
			resp.Error(c, http.StatusBadGateway, lastErr.Error())
		}
	}
}

func rerankTryChannel(
	c *gin.Context,
	iter *balancer.Iterator,
	stickyKeyID int,
	apiKeyID int,
	requestModel string,
	body []byte,
) (ok, written bool, err error) {
	item := iter.Item()
	ctx := c.Request.Context()

	channel, err := op.ChannelGet(item.ChannelID, ctx)
	if err != nil {
		iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), fmt.Sprintf("channel not found: %v", err))
		return false, false, err
	}
	if !channel.Enabled {
		iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
		return false, false, nil
	}

	if channel.AllowEmptyKey {
		return rerankTryOneKey(c, iter, channel, dbmodel.ChannelKey{}, item.ModelName, requestModel, body, apiKeyID)
	}

	exclude := map[int]struct{}{}
	if iter.IsSticky() && stickyKeyID > 0 {
		var stickyKey *dbmodel.ChannelKey
		for i := range channel.Keys {
			if channel.Keys[i].ID == stickyKeyID {
				stickyKey = &channel.Keys[i]
				break
			}
		}
		if stickyKey == nil || !stickyKey.Enabled || stickyKey.ChannelKey == "" {
			balancer.ClearSticky(apiKeyID, requestModel)
			stickyKeyID = 0
		} else if keymanager.IsAvailable(channel, *stickyKey, time.Now().Unix()) {
			success, written, err := rerankTryOneKey(c, iter, channel, *stickyKey, item.ModelName, requestModel, body, apiKeyID)
			if success || written {
				return success, written, err
			}
			exclude[stickyKey.ID] = struct{}{}
		} else {
			exclude[stickyKey.ID] = struct{}{}
		}
	}

	available := keymanager.ListAvailable(channel, exclude)
	switch len(available) {
	case 0:
		iter.Skip(channel.ID, 0, channel.Name, "no available key")
		return false, false, nil
	case 1:
		return rerankTryOneKey(c, iter, channel, available[0], item.ModelName, requestModel, body, apiKeyID)
	default:
		var lastErr error
		for {
			if ctx.Err() != nil {
				return false, false, context.Canceled
			}
			if refreshed, getErr := op.ChannelGet(channel.ID, ctx); getErr == nil {
				channel = refreshed
			}
			key, okSel := keymanager.Select(channel, exclude)
			if !okSel {
				break
			}
			success, written, err := rerankTryOneKey(c, iter, channel, key, item.ModelName, requestModel, body, apiKeyID)
			if success || written {
				return success, written, err
			}
			if err != nil {
				lastErr = err
			}
			exclude[key.ID] = struct{}{}
		}
		return false, false, lastErr
	}
}

func rerankTryOneKey(
	c *gin.Context,
	iter *balancer.Iterator,
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

	log.Infof("rerank model %s forwarding to channel: %s model: %s key: %d (sticky=%t)",
		requestModel, channel.Name, upstreamModel, usedKey.ID, iter.IsSticky())

	span := iter.StartAttempt(channel.ID, usedKey.ID, channel.Name)
	statusCode, fwdErr := rerankForward(c.Request.Context(), c, channel, usedKey, upstreamModel, requestModel, body)
	if fwdErr == nil && statusCode == 0 {
		statusCode = http.StatusOK
	}

	if fwdErr == nil {
		keymanager.OnResult(channel, usedKey, statusCode, 0)
		span.End(dbmodel.AttemptSuccess, "")
		op.StatsChannelUpdate(channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})
		balancer.RecordSuccess(channel.ID, usedKey.ID, circuitModel)
		balancer.SetSticky(apiKeyID, requestModel, channel.ID, usedKey.ID)
		probeDone = true
		return true, false, nil
	}

	keymanager.OnResult(channel, usedKey, statusCode, 0)
	span.End(dbmodel.AttemptFailed, fwdErr.Error())
	op.StatsChannelUpdate(channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})
	balancer.RecordFailure(channel.ID, usedKey.ID, circuitModel)
	probeDone = true
	written = c.Writer.Written()
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
	httpClient, err := helper.ChannelHttpClient(channel)
	if err != nil {
		return 0, err
	}

	outBody := body
	if upstreamModel != requestModel {
		var bodyMap map[string]any
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			bodyMap["model"] = upstreamModel
			if modified, err := json.Marshal(bodyMap); err == nil {
				outBody = modified
			}
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
	for _, h := range channel.CustomHeader {
		if h.HeaderKey == "" {
			continue
		}
		if httpReq.Header.Get(h.HeaderKey) != "" && httpclient.IsSensitiveHeader(h.HeaderKey) {
			continue
		}
		httpReq.Header.Set(h.HeaderKey, h.HeaderValue)
	}

	upstream, err := httpClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer upstream.Body.Close()

	if upstream.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(upstream.Body, 64*1024))
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

func saveRerankMetrics(
	ctx context.Context,
	apiKeyID int,
	requestModel, actualModel string,
	startTime time.Time,
	success bool,
	err error,
	attempts []dbmodel.ChannelAttempt,
	channelID int,
) {
	duration := time.Since(startTime)
	globalStats := dbmodel.StatsMetrics{
		WaitTime: duration.Milliseconds(),
	}
	if success {
		globalStats.RequestSuccess = 1
	} else {
		globalStats.RequestFailed = 1
	}

	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	_ = op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(apiKeyID, globalStats)

	log.Infof("rerank complete: model=%s actual=%s success=%t duration=%dms attempts=%d",
		requestModel, actualModel, success, duration.Milliseconds(), len(attempts))

	relayLog := dbmodel.RelayLog{
		Time:             startTime.Unix(),
		RequestModelName: requestModel,
		ActualModelName:  actualModel,
		UseTime:          int(duration.Milliseconds()),
		Attempts:         attempts,
		TotalAttempts:    len(attempts),
	}
	if apiKey, getErr := op.APIKeyGet(apiKeyID, ctx); getErr == nil {
		relayLog.RequestAPIKeyName = apiKey.Name
	}
	if channelID > 0 {
		if ch, chErr := op.ChannelGet(channelID, ctx); chErr == nil {
			relayLog.ChannelName = ch.Name
			relayLog.ChannelId = channelID
		}
	}
	if err != nil {
		relayLog.Error = err.Error()
	}
	if logErr := op.RelayLogAdd(context.WithoutCancel(ctx), relayLog); logErr != nil {
		log.Warnf("failed to save rerank relay log: %v", logErr)
	}
}
