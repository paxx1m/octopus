package keymanager

import (
	"math/rand"
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

// RateLimitCooldownSec resolves 429 cooldown: Key > Channel > Global > 300.
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

// IsAvailable reports whether a key can be used for a new attempt.
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

// ListAvailable returns keys that pass availability checks, excluding tried IDs.
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

// CountAvailable returns the number of currently usable keys.
func CountAvailable(ch *model.Channel) int {
	return len(ListAvailable(ch, nil))
}

// Select picks one key by channel KeyMode from available keys (excluding tried).
func Select(ch *model.Channel, exclude map[int]struct{}) (model.ChannelKey, bool) {
	available := ListAvailable(ch, exclude)
	if len(available) == 0 {
		return model.ChannelKey{}, false
	}
	mode := ch.EffectiveKeyMode()
	switch mode {
	case model.KeyModeRoundRobin:
		return selectRoundRobin(ch.ID, available), true
	case model.KeyModeRandom:
		return available[rand.Intn(len(available))], true
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

func selectRoundRobin(channelID int, keys []model.ChannelKey) model.ChannelKey {
	v, _ := roundRobinCounters.LoadOrStore(channelID, new(uint64))
	counter := v.(*uint64)
	idx := int(atomic.AddUint64(counter, 1)-1) % len(keys)
	return keys[idx]
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
		w := k.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	if total <= 0 {
		return keys[0]
	}
	r := rand.Intn(total)
	for _, k := range keys {
		w := k.Weight
		if w <= 0 {
			w = 1
		}
		r -= w
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

// OnResult updates key state after an attempt.
// - success: apply cost delta and status
// - 429: mark rate-limited (cooldown via LastUseTimeStamp)
// - 401/403: disable key immediately in DB
// - other failures: update status/timestamp only
// Empty key (ID==0) is a no-op for persistence.
func OnResult(ch *model.Channel, key model.ChannelKey, statusCode int, costDelta float64) {
	if key.ID == 0 || ch == nil {
		return
	}
	now := time.Now().Unix()

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		if err := op.ChannelKeySetEnabled(key.ID, ch.ID, false); err != nil {
			log.Warnf("failed to disable channel key %d after %d: %v", key.ID, statusCode, err)
		} else {
			log.Warnf("channel key %d disabled after HTTP %d", key.ID, statusCode)
		}
		_ = op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0)
		return
	case http.StatusTooManyRequests:
		_ = op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0)
		return
	}

	if statusCode >= 200 && statusCode < 300 {
		_ = op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, costDelta)
		return
	}

	_ = op.ChannelKeyApplyUpdate(key.ID, ch.ID, statusCode, now, 0)
}
