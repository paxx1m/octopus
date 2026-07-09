package balancer

import (
	"fmt"
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
	mu                  sync.Mutex
}

const (
	defaultCircuitBreakerThreshold   int64 = 5
	defaultCircuitBreakerCooldown          = 60
	defaultCircuitBreakerMaxCooldown       = 600
	circuitSettingsTTL                     = 5 * time.Second
)

type circuitSettings struct {
	Threshold   int64
	Cooldown    int
	MaxCooldown int
}

var (
	// 全局熔断器存储
	globalBreaker sync.Map // key: string -> value: *circuitEntry

	circuitSettingsMu       sync.Mutex
	cachedCircuitSettings   circuitSettings
	cachedCircuitSettingsAt time.Time
)

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

func getCircuitSettings() circuitSettings {
	now := time.Now()
	circuitSettingsMu.Lock()
	defer circuitSettingsMu.Unlock()

	if !cachedCircuitSettingsAt.IsZero() && now.Sub(cachedCircuitSettingsAt) < circuitSettingsTTL {
		return cachedCircuitSettings
	}

	settings := circuitSettings{
		Threshold:   defaultCircuitBreakerThreshold,
		Cooldown:    defaultCircuitBreakerCooldown,
		MaxCooldown: defaultCircuitBreakerMaxCooldown,
	}
	if threshold, err := op.SettingGetInt(model.SettingKeyCircuitBreakerThreshold); err == nil && threshold > 0 {
		settings.Threshold = int64(threshold)
	}
	if cooldown, err := op.SettingGetInt(model.SettingKeyCircuitBreakerCooldown); err == nil && cooldown > 0 {
		settings.Cooldown = cooldown
	}
	if maxCooldown, err := op.SettingGetInt(model.SettingKeyCircuitBreakerMaxCooldown); err == nil && maxCooldown > 0 {
		settings.MaxCooldown = maxCooldown
	}

	cachedCircuitSettings = settings
	cachedCircuitSettingsAt = now
	return settings
}

// getThreshold 获取熔断阈值配置
func getThreshold() int64 {
	return getCircuitSettings().Threshold
}

// GetCooldown 获取当前冷却时间（带指数退避）
func GetCooldown(tripCount int) time.Duration {
	settings := getCircuitSettings()

	// 指数退避：baseCooldown * 2^(tripCount-1)
	cooldown := settings.Cooldown
	if tripCount > 1 {
		shift := tripCount - 1
		if shift > 20 { // 防止溢出
			shift = 20
		}
		cooldown = settings.Cooldown << shift
	}
	if cooldown > settings.MaxCooldown {
		cooldown = settings.MaxCooldown
	}

	return time.Duration(cooldown) * time.Second
}

// IsTripped 检查通道是否处于熔断状态
// 返回 tripped=true 表示该通道应被跳过，remaining 为剩余冷却时间
func IsTripped(channelID, keyID int, modelName string) (tripped bool, remaining time.Duration) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return false, 0 // 无记录，视为 Closed
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	switch entry.State {
	case StateClosed:
		return false, 0

	case StateOpen:
		cooldown := GetCooldown(entry.TripCount)
		elapsed := time.Since(entry.LastFailureTime)
		if elapsed >= cooldown {
			entry.State = StateHalfOpen
			log.Infof("circuit breaker [%s] Open -> HalfOpen (cooldown %v elapsed)", key, cooldown)
			return false, 0
		}
		// 仍在冷却中
		return true, cooldown - elapsed

	case StateHalfOpen:
		// 已有试探请求在进行中，拒绝其他请求
		return true, 0

	default:
		return false, 0
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
	defer entry.mu.Unlock()

	if entry.State == StateHalfOpen {
		log.Infof("circuit breaker [%s] HalfOpen -> Closed (probe succeeded)", key)
	}

	// 重置全部状态
	entry.State = StateClosed
	entry.ConsecutiveFailures = 0
	entry.TripCount = 0
}

// RecordFailure 记录失败，可能触发熔断
func RecordFailure(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	entry := getOrCreateEntry(key)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.LastFailureTime = time.Now()

	switch entry.State {
	case StateClosed:
		entry.ConsecutiveFailures++
		threshold := getThreshold()
		if entry.ConsecutiveFailures >= threshold {
			entry.State = StateOpen
			entry.TripCount++
			log.Warnf("circuit breaker [%s] Closed -> Open (failures=%d >= threshold=%d, tripCount=%d, cooldown=%v)",
				key, entry.ConsecutiveFailures, threshold, entry.TripCount, GetCooldown(entry.TripCount))
		}

	case StateHalfOpen:
		// 试探失败，重新进入 Open 状态，TripCount 递增（冷却时间翻倍）
		entry.State = StateOpen
		entry.TripCount++
		entry.ConsecutiveFailures = 0 // 重新开始计数
		log.Warnf("circuit breaker [%s] HalfOpen -> Open (probe failed, tripCount=%d, cooldown=%v)",
			key, entry.TripCount, GetCooldown(entry.TripCount))

	case StateOpen:
		// 理论上不应该在 Open 状态下接收到失败记录（请求应被拒绝），
		// 但为安全起见仍更新失败时间
	}
}
