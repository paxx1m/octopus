package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm/httpclient"
)

// tryOneKeyFn 在选定渠道与 Key 上执行一次真实转发。
// 返回值语义与 tryChannel 一致：ok 成功结束；written 已写出不可再试。
type tryOneKeyFn func(channel *dbmodel.Channel, key dbmodel.ChannelKey, upstreamModel string) (ok, written bool, err error)

// tryChannelWithKeys 统一渠道内 Key 选择 / 粘性 / 故障转移逻辑（主 relay 与 rerank 共用）。
func tryChannelWithKeys(
	c *gin.Context,
	iter *balancer.Iterator,
	stickyKeyID *int,
	apiKeyID int,
	requestModel string,
	tryOne tryOneKeyFn,
) (ok, written bool, err error) {
	item := iter.Item()
	ctx := c.Request.Context()

	channel, err := op.ChannelGet(item.ChannelID, ctx)
	if err != nil {
		log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
		iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), fmt.Sprintf("channel not found: %v", err))
		return false, false, err
	}
	if !channel.Enabled {
		iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
		return false, false, nil
	}

	if channel.AllowEmptyKey {
		return tryOne(&channel, dbmodel.ChannelKey{}, item.ModelName)
	}

	exclude := map[int]struct{}{}
	if iter.IsSticky() && stickyKeyID != nil && *stickyKeyID > 0 {
		var stickyKey *dbmodel.ChannelKey
		for i := range channel.Keys {
			if channel.Keys[i].ID == *stickyKeyID {
				stickyKey = &channel.Keys[i]
				break
			}
		}
		if stickyKey == nil || !stickyKey.Enabled || stickyKey.ChannelKey == "" {
			balancer.ClearSticky(apiKeyID, requestModel)
			*stickyKeyID = 0
		} else if keymanager.IsAvailable(&channel, *stickyKey, time.Now().Unix()) {
			success, written, err := tryOne(&channel, *stickyKey, item.ModelName)
			if success || written {
				return success, written, err
			}
			exclude[stickyKey.ID] = struct{}{}
		} else {
			exclude[stickyKey.ID] = struct{}{}
		}
	}

	available := keymanager.ListAvailable(&channel, exclude)
	switch len(available) {
	case 0:
		iter.Skip(channel.ID, 0, channel.Name, "no available key")
		return false, false, nil
	case 1:
		return tryOne(&channel, available[0], item.ModelName)
	default:
		var lastErr error
		attempts := 0
		for {
			if ctx.Err() != nil {
				return false, false, context.Canceled
			}
			if refreshed, getErr := op.ChannelGet(channel.ID, ctx); getErr == nil {
				channel = refreshed
			} else {
				log.Warnf("failed to refresh channel %d state: %v", channel.ID, getErr)
			}
			key, okSel := keymanager.Select(&channel, exclude)
			if !okSel {
				// 所有 Key 已试完或均不可用；保留具体失败原因，避免客户端只见泛化错误。
				if lastErr == nil {
					lastErr = fmt.Errorf("no available key for channel %d after %d attempt(s)", channel.ID, attempts)
				}
				break
			}
			attempts++
			success, written, err := tryOne(&channel, key, item.ModelName)
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

// completeAttempt 统一单次 attempt 的记账：key 冷却/费用、熔断、统计与粘性更新（主 relay 与 rerank 共用）。
// 返回 written：响应是否已写出（写出后不可再换渠道/Key）。
func completeAttempt(
	c *gin.Context,
	channel *dbmodel.Channel,
	usedKey dbmodel.ChannelKey,
	circuitModel string,
	apiKeyID int,
	requestModel string,
	span *balancer.AttemptSpan,
	statusCode int,
	fwdErr error,
	costDelta float64,
) bool {
	if fwdErr == nil {
		keymanager.OnResult(channel, usedKey, statusCode, costDelta)
		span.End(dbmodel.AttemptSuccess, "")
		op.StatsChannelUpdate(channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})
		balancer.RecordSuccess(channel.ID, usedKey.ID, circuitModel)
		balancer.SetSticky(apiKeyID, requestModel, channel.ID, usedKey.ID)
		return false
	}

	keymanager.OnResult(channel, usedKey, statusCode, 0)
	span.End(dbmodel.AttemptFailed, fwdErr.Error())
	op.StatsChannelUpdate(channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})
	balancer.RecordFailure(channel.ID, usedKey.ID, circuitModel)
	return c.Writer.Written()
}

// applyParamOverride 将 param_override 合并到 JSON 请求体，返回合并后的 body 与是否成功。
func applyParamOverride(body []byte, overrideJSON string) ([]byte, bool) {
	if overrideJSON == "" {
		return body, true
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return body, false
	}
	var override map[string]any
	if err := json.Unmarshal([]byte(overrideJSON), &override); err != nil {
		return body, false
	}
	maps.Copy(bodyMap, override)
	modified, err := json.Marshal(bodyMap)
	if err != nil {
		return body, false
	}
	return modified, true
}

// applyCustomHeaders 应用渠道自定义 header；同名敏感头保持认证配置优先。
func applyCustomHeaders(headers http.Header, custom []dbmodel.CustomHeader) {
	for _, header := range custom {
		if header.HeaderKey == "" {
			continue
		}
		if headers.Get(header.HeaderKey) != "" && httpclient.IsSensitiveHeader(header.HeaderKey) {
			continue
		}
		headers.Set(header.HeaderKey, header.HeaderValue)
	}
}
