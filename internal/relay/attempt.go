package relay

import (
	"context"
	"fmt"
	"time"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
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
		for {
			if ctx.Err() != nil {
				return false, false, context.Canceled
			}
			if refreshed, getErr := op.ChannelGet(channel.ID, ctx); getErr == nil {
				channel = refreshed
			}
			key, okSel := keymanager.Select(&channel, exclude)
			if !okSel {
				break
			}
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
