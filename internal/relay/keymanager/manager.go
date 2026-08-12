package keymanager

import (
	"math/rand/v2"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const defaultRateLimitCooldownSec = 300

var roundRobinCounters sync.Map // channelID -> *uint64

// CleanupChannel 渠道删除时清理其轮转计数器，防止 sync.Map 无界增长。
func CleanupChannel(channelID int) {
	roundRobinCounters.Delete(channelID)
}

// RateLimitCooldownSec 解析 429 冷却秒数，优先级：Key > 渠道 > 全局设置 > 300。
func RateLimitCooldownSec(ch *model.Channel, key *model.ChannelKey) int {
	if key != nil && key.RateLimitCooldownSec != nil && *key.RateLimitCooldownSec > 0 {
		return *key.RateLimitCooldownSec
	}
	if ch != nil && ch.RateLimitCooldownSec != nil && *ch.RateLimitCooldownSec > 0 {
		return *ch.RateLimitCooldownSec
	}
	if v, err := op.SettingGetInt(model.SettingKeyChannelKeyRateLimitCooldown); err == nil && v > 0 {
		return v
	}
	return defaultRateLimitCooldownSec
}

// IsAvailable 判断 Key 当前是否可被选中。
//
// 429 冷却是“软屏蔽”，不是从 Keys 切片删除：
//   - OnResult 收到 429 时写入 StatusCode=429 与 LastUseTimeStamp
//   - 冷却期内本函数返回 false，Select/ListAvailable 自然跳过
//   - 冷却结束后（now - LastUseTimeStamp >= cooldown）自动重新可用
//     无需后台任务把 Key “补回可用列表”
func IsAvailable(ch *model.Channel, k model.ChannelKey, nowSec int64) bool {
	if !k.Enabled || k.ChannelKey == "" {
		return false
	}
	if k.StatusCode == http.StatusTooManyRequests && k.LastUseTimeStamp > 0 {
		cooldown := int64(RateLimitCooldownSec(ch, &k))
		if nowSec-k.LastUseTimeStamp < cooldown {
			return false
		}
	}
	return true
}

// ListAvailable 返回当前可用 Key（排除本请求已试过的 ID）。
func ListAvailable(ch *model.Channel, exclude map[int]struct{}) []model.ChannelKey {
	if ch == nil {
		return nil
	}
	nowSec := time.Now().Unix()
	out := make([]model.ChannelKey, 0, len(ch.Keys))
	for _, k := range ch.Keys {
		if exclude != nil {
			if _, skip := exclude[k.ID]; skip {
				continue
			}
		}
		if IsAvailable(ch, k, nowSec) {
			out = append(out, k)
		}
	}
	return out
}

// CountAvailable 返回当前可用 Key 数量。
func CountAvailable(ch *model.Channel) int {
	return len(ListAvailable(ch, nil))
}

// Select 按渠道 KeyMode 从可用 Key 中选一个（排除本请求已失败的）。
func Select(ch *model.Channel, exclude map[int]struct{}) (model.ChannelKey, bool) {
	if ch == nil {
		return model.ChannelKey{}, false
	}
	mode := ch.EffectiveKeyMode()
	if mode == model.KeyModeRoundRobin {
		return selectRoundRobin(ch, exclude)
	}

	available := ListAvailable(ch, exclude)
	if len(available) == 0 {
		return model.ChannelKey{}, false
	}
	switch mode {
	case model.KeyModeRandom:
		return available[rand.IntN(len(available))], true
	case model.KeyModeFailover:
		return selectFailover(available), true
	case model.KeyModeWeighted:
		return selectWeighted(available), true
	case model.KeyModeLeastCost:
		return selectLeastCost(available), true
	default:
		return selectLeastCost(available), true
	}
}

// selectRoundRobin 在完整 enabled Key 列表（按 ID 排序）上轮转。
// 若起点 Key 因 429 不可用，则顺延下一个可用，避免只对“当前可用子集”取模导致分布偏斜。
func selectRoundRobin(ch *model.Channel, exclude map[int]struct{}) (model.ChannelKey, bool) {
	candidates := make([]model.ChannelKey, 0, len(ch.Keys))
	for _, k := range ch.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		if exclude != nil {
			if _, skip := exclude[k.ID]; skip {
				continue
			}
		}
		candidates = append(candidates, k)
	}
	if len(candidates) == 0 {
		return model.ChannelKey{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ID < candidates[j].ID
	})

	v, _ := roundRobinCounters.LoadOrStore(ch.ID, new(uint64))
	counter := v.(*uint64)
	start := int(atomic.AddUint64(counter, 1)-1) % len(candidates)
	nowSec := time.Now().Unix()
	for i := 0; i < len(candidates); i++ {
		k := candidates[(start+i)%len(candidates)]
		if IsAvailable(ch, k, nowSec) {
			return k, true
		}
	}
	return model.ChannelKey{}, false
}

func selectFailover(keys []model.ChannelKey) model.ChannelKey {
	sorted := make([]model.ChannelKey, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	return sorted[0]
}

func selectWeighted(keys []model.ChannelKey) model.ChannelKey {
	total := 0
	for _, k := range keys {
		total += model.KeyWeight(k.Weight)
	}
	if total <= 0 {
		return keys[0]
	}
	r := rand.IntN(total)
	for _, k := range keys {
		r -= model.KeyWeight(k.Weight)
		if r < 0 {
			return k
		}
	}
	return keys[len(keys)-1]
}

func selectLeastCost(keys []model.ChannelKey) model.ChannelKey {
	best := keys[0]
	for i := 1; i < len(keys); i++ {
		if keys[i].TotalCost < best.TotalCost {
			best = keys[i]
		}
	}
	return best
}

// OnResult 根据上游结果更新 Key 运行时状态（费用 / 冷却 / 禁用）。
//
//   - 2xx：累加 costDelta，刷新状态与最后使用时间
//   - 429：写入 StatusCode+LastUseTimeStamp，进入冷却；到期后 IsAvailable 自动放行
//   - 401/403：立即 Enabled=false 落库，需用户手动重新启用
//   - 其他失败：只更新状态码与时间，不永久禁用
//   - key.ID==0（allow_empty_key）：不落库
func OnResult(ch *model.Channel, key model.ChannelKey, statusCode int, costDelta float64) {
	if key.ID == 0 || ch == nil {
		return
	}
	now := time.Now().Unix()
	notify := false

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		if err := op.ChannelKeySetEnabled(key.ID, ch.ID, false); err != nil {
			log.Warnf("failed to disable channel key %d after %d: %v", key.ID, statusCode, err)
			// SetEnabled failed — still try status update + notify
			if err := op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0); err != nil {
				log.Warnf("failed to record status for channel key %d: %v", key.ID, err)
			}
			notify = true
		} else {
			log.Warnf("channel key %d disabled after HTTP %d", key.ID, statusCode)
			if err := op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0); err != nil {
				log.Warnf("failed to record status for channel key %d: %v", key.ID, err)
			}
			// ChannelKeySetEnabled already HealthNotify'd
		}
	case http.StatusTooManyRequests:
		// 软冷却：不删 Key、不改 Enabled；仅靠时间窗屏蔽，到期自动恢复可选。
		if err := op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0); err != nil {
			// 冷却信息写失败时限流防护会静默失效，必须显式告警。
			log.Warnf("failed to record 429 cooldown for channel key %d: %v", key.ID, err)
		}
		notify = true
	default:
		if statusCode >= 200 && statusCode < 300 {
			prev := key.StatusCode
			if err := op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, costDelta); err != nil {
				log.Warnf("failed to record success for channel key %d: %v", key.ID, err)
			}
			// recovery from 429 / error codes should refresh health panel
			if prev == http.StatusTooManyRequests || (prev >= 400 && prev != statusCode) {
				notify = true
			}
		} else {
			if err := op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0); err != nil {
				log.Warnf("failed to record failure for channel key %d: %v", key.ID, err)
			}
			// surface non-2xx last status without necessarily changing availability
		}
	}

	if notify {
		op.HealthNotify()
	}
}
