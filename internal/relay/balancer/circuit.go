package balancer

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// CircuitState 熔断器状态
type CircuitState int

const (
	StateClosed   CircuitState = iota // 正常通行
	StateOpen                         // 熔断中，拒绝所有请求
	StateHalfOpen                     // 半开，仅允许单个试探请求
)

// circuitEntry 单个熔断器条目
type circuitEntry struct {
	State               CircuitState
	ConsecutiveFailures int64
	LastFailureTime     time.Time
	TripCount           int // 累计熔断触发次数（用于指数退避）
	ProbePending        bool
	mu                  sync.Mutex
}

// 全局熔断器存储
var globalBreaker sync.Map // key: string -> value: *circuitEntry

// circuitKey 生成熔断器键：channelID:channelKeyID:modelName
func circuitKey(channelID, keyID int, modelName string) string {
	return fmt.Sprintf("%d:%d:%s", channelID, keyID, modelName)
}

// getOrCreateEntry 获取或创建熔断器条目
func getOrCreateEntry(key string) *circuitEntry {
	if v, ok := globalBreaker.Load(key); ok {
		return v.(*circuitEntry)
	}
	entry := &circuitEntry{State: StateClosed}
	actual, _ := globalBreaker.LoadOrStore(key, entry)
	return actual.(*circuitEntry)
}

// 熔断配置带 TTL 缓存：失败/检查路径在持锁状态下频繁读取设置，
// 用短 TTL 缓存避免每次失败都做 SettingGetInt + strconv.Atoi。
const circuitSettingsTTL = 10 * time.Second

var (
	circuitSettingsMu    sync.Mutex
	circuitSettingsCache = struct {
		threshold   int64
		cooldown    int64
		maxCooldown int64
		refreshedAt time.Time
	}{threshold: 5, cooldown: 60, maxCooldown: 600}
)

func circuitSettings() (threshold, cooldown, maxCooldown int64) {
	circuitSettingsMu.Lock()
	defer circuitSettingsMu.Unlock()

	if time.Since(circuitSettingsCache.refreshedAt) < circuitSettingsTTL {
		return circuitSettingsCache.threshold, circuitSettingsCache.cooldown, circuitSettingsCache.maxCooldown
	}

	cache := struct {
		threshold   int64
		cooldown    int64
		maxCooldown int64
		refreshedAt time.Time
	}{refreshedAt: time.Now()}

	if v, err := op.SettingGetInt(model.SettingKeyCircuitBreakerThreshold); err == nil && v > 0 {
		cache.threshold = int64(v)
	} else {
		cache.threshold = 5
	}
	if v, err := op.SettingGetInt(model.SettingKeyCircuitBreakerCooldown); err == nil && v > 0 {
		cache.cooldown = int64(v)
	} else {
		cache.cooldown = 60
	}
	if v, err := op.SettingGetInt(model.SettingKeyCircuitBreakerMaxCooldown); err == nil && v > 0 {
		cache.maxCooldown = int64(v)
	} else {
		cache.maxCooldown = 600
	}

	circuitSettingsCache = cache
	return cache.threshold, cache.cooldown, cache.maxCooldown
}

// getThreshold 获取熔断阈值配置
func getThreshold() int64 {
	threshold, _, _ := circuitSettings()
	return threshold
}

// GetCooldown 获取当前冷却时间（带指数退避）
func GetCooldown(tripCount int) time.Duration {
	_, base, maxCooldown := circuitSettings()

	// 指数退避：baseCooldown * 2^(tripCount-1)
	cooldown := base
	if tripCount > 1 {
		shift := tripCount - 1
		if shift > 20 { // 防止溢出
			shift = 20
		}
		cooldown = base << shift
	}
	if cooldown > maxCooldown {
		cooldown = maxCooldown
	}

	return time.Duration(cooldown) * time.Second
}

// IsTripped 检查通道是否处于熔断状态
// 返回 tripped=true 表示该通道应被跳过，remaining 为剩余冷却时间
// 返回 probe=true 表示本次请求是 HalfOpen 探测，调用方在未完成转发时必须调用 RecordProbeAborted
func IsTripped(channelID, keyID int, modelName string) (tripped bool, remaining time.Duration, probe bool) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return false, 0, false // 无记录，视为 Closed
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	var (
		outTripped bool
		outRem     time.Duration
		outProbe   bool
		notify     bool
	)
	switch entry.State {
	case StateClosed:
		// defaults: not tripped
	case StateOpen:
		cooldown := GetCooldown(entry.TripCount)
		elapsed := time.Since(entry.LastFailureTime)
		if elapsed >= cooldown {
			entry.State = StateHalfOpen
			entry.ProbePending = true
			log.Infof("circuit breaker [%s] Open -> HalfOpen (cooldown %v elapsed)", key, cooldown)
			outProbe = true
			notify = true
		} else {
			outTripped = true
			outRem = cooldown - elapsed
		}
	case StateHalfOpen:
		// 已有试探请求在进行中，拒绝其他请求
		outTripped = true
	}
	entry.mu.Unlock()
	if notify {
		op.HealthNotify()
	}
	return outTripped, outRem, outProbe
}

// RecordProbeAborted 探测请求在真正转发前被跳过（无 key、disabled、adapter 失败等），
// 将 HalfOpen 恢复为 Open，避免熔断器永久卡在 HalfOpen。
func RecordProbeAborted(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	changed := false
	if entry.State == StateHalfOpen && entry.ProbePending {
		entry.State = StateOpen
		entry.ProbePending = false
		entry.LastFailureTime = time.Now()
		log.Warnf("circuit breaker [%s] HalfOpen -> Open (probe aborted before forward)", key)
		changed = true
	}
	entry.mu.Unlock()
	if changed {
		op.HealthNotify()
	}
}

// RecordSuccess 记录成功，重置熔断器状态
func RecordSuccess(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	prev := entry.State
	prevFailures := entry.ConsecutiveFailures
	if entry.State == StateHalfOpen {
		log.Infof("circuit breaker [%s] HalfOpen -> Closed (probe succeeded)", key)
	}

	// 重置全部状态
	entry.State = StateClosed
	entry.ConsecutiveFailures = 0
	entry.TripCount = 0
	entry.ProbePending = false
	changed := prev != StateClosed || prevFailures > 0
	entry.mu.Unlock()
	if changed {
		op.HealthNotify()
	}
}

// RecordFailure 记录失败，可能触发熔断
func RecordFailure(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	entry := getOrCreateEntry(key)

	entry.mu.Lock()
	entry.LastFailureTime = time.Now()
	entry.ProbePending = false
	notify := false

	switch entry.State {
	case StateClosed:
		entry.ConsecutiveFailures++
		threshold := getThreshold()
		if entry.ConsecutiveFailures >= threshold {
			entry.State = StateOpen
			entry.TripCount++
			log.Warnf("circuit breaker [%s] Closed -> Open (failures=%d >= threshold=%d, tripCount=%d, cooldown=%v)",
				key, entry.ConsecutiveFailures, threshold, entry.TripCount, GetCooldown(entry.TripCount))
			notify = true
		}
	// degraded (failures < threshold): no notify — 30s SSE resync is enough

	case StateHalfOpen:
		// 试探失败，重新进入 Open 状态，TripCount 递增（冷却时间翻倍）
		entry.State = StateOpen
		entry.TripCount++
		entry.ConsecutiveFailures = 0 // 重新开始计数
		log.Warnf("circuit breaker [%s] HalfOpen -> Open (probe failed, tripCount=%d, cooldown=%v)",
			key, entry.TripCount, GetCooldown(entry.TripCount))
		notify = true
	}
	entry.mu.Unlock()
	if notify {
		op.HealthNotify()
	}
}

// CircuitSnapshot 熔断器只读快照（不迁移状态）。
type CircuitSnapshot struct {
	ChannelID           int    `json:"channel_id"`
	KeyID               int    `json:"key_id"`
	ModelName           string `json:"model_name"`
	State               string `json:"state"` // closed | open | half_open
	ConsecutiveFailures int64  `json:"consecutive_failures"`
	TripCount           int    `json:"trip_count"`
	LastFailureUnix     int64  `json:"last_failure_unix"`
	CooldownUntil       int64  `json:"cooldown_until"`
	RemainingSec        int    `json:"remaining_sec"`
	ProbePending        bool   `json:"probe_pending"`
}

func circuitStateString(s CircuitState) string {
	switch s {
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}

// ListCircuits 返回熔断条目只读快照。
// onlyActive=true 时仅返回 Open / HalfOpen，或 Closed 但仍有连续失败计数的条目。
// 本函数不修改状态（不会触发 Open→HalfOpen）。
func ListCircuits(onlyActive bool) []CircuitSnapshot {
	now := time.Now()
	out := make([]CircuitSnapshot, 0, 16)
	globalBreaker.Range(func(k, v any) bool {
		key, _ := k.(string)
		entry := v.(*circuitEntry)
		entry.mu.Lock()
		state := entry.State
		failures := entry.ConsecutiveFailures
		tripCount := entry.TripCount
		lastFail := entry.LastFailureTime
		probePending := entry.ProbePending
		entry.mu.Unlock()

		if onlyActive {
			if state == StateClosed && failures == 0 {
				return true
			}
		}

		channelID, keyID, modelName := parseCircuitKey(key)

		snap := CircuitSnapshot{
			ChannelID:           channelID,
			KeyID:               keyID,
			ModelName:           modelName,
			State:               circuitStateString(state),
			ConsecutiveFailures: failures,
			TripCount:           tripCount,
			ProbePending:        probePending,
		}
		if !lastFail.IsZero() {
			snap.LastFailureUnix = lastFail.Unix()
		}
		if state == StateOpen {
			cooldown := GetCooldown(tripCount)
			until := lastFail.Add(cooldown)
			snap.CooldownUntil = until.Unix()
			rem := int(until.Sub(now).Seconds())
			if rem < 0 {
				rem = 0
			}
			snap.RemainingSec = rem
		}
		out = append(out, snap)
		return true
	})
	return out
}

func parseCircuitKey(key string) (channelID, keyID int, modelName string) {
	// key format: "{channelID}:{keyID}:{modelName}"；modelName 可含冒号
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 3 {
		return 0, 0, key
	}
	channelID, _ = strconv.Atoi(parts[0])
	keyID, _ = strconv.Atoi(parts[1])
	modelName = parts[2]
	return channelID, keyID, modelName
}

// cleanupCircuitEntries 清理长期 Closed 且无活动的熔断条目，防止 map 无界增长。
func cleanupCircuitEntries(maxIdle time.Duration) int {
	removed := 0
	now := time.Now()
	globalBreaker.Range(func(k, v any) bool {
		entry := v.(*circuitEntry)
		entry.mu.Lock()
		idle := entry.State == StateClosed &&
			entry.ConsecutiveFailures == 0 &&
			!entry.ProbePending &&
			(entry.LastFailureTime.IsZero() || now.Sub(entry.LastFailureTime) > maxIdle)
		entry.mu.Unlock()
		if idle {
			globalBreaker.Delete(k)
			removed++
		}
		return true
	})
	return removed
}
