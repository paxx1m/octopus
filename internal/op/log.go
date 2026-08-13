package op

import (
	"context"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/snowflake"
	"gorm.io/gorm"
)

const relayLogMaxSize = 20
const relayLogMaxSizeNoDB = 100 // 当不保存到数据库时，允许更大的缓存用于实时查询

var relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
var relayLogCacheLock sync.Mutex

var relayLogFlushLock sync.Mutex

const relayLogStreamTokenTTL = 5 * time.Minute

// relayLogHub 管理日志 SSE 的 stream-token 与订阅通知（负载为日志条目）。
var relayLogHub = newSSEHub[model.RelayLog](relayLogStreamTokenTTL, 10)

func RelayLogStreamTokenCreate() (string, error) {
	return relayLogHub.TokenCreate()
}

func RelayLogStreamTokenVerify(token string) bool {
	return relayLogHub.TokenVerify(token)
}

func RelayLogStreamTokenRevoke(token string) {
	relayLogHub.TokenRevoke(token)
}

func RelayLogSubscribe() chan model.RelayLog {
	return relayLogHub.Subscribe()
}

func RelayLogUnsubscribe(ch chan model.RelayLog) {
	relayLogHub.Unsubscribe(ch)
}

func truncateLogContent(s string) string {
	if len(s) <= model.RelayLogContentMaxLen {
		return s
	}
	return s[:model.RelayLogContentMaxLen] + "...[truncated]"
}

// trimRelayLogCache 保留最近 keep 条日志，丢弃更旧的。
// 重建底层数组而不是 reslice，避免数组持续引用旧日志的 Request/ResponseContent 导致内存无法回收。
func trimRelayLogCache(keep int) {
	if len(relayLogCache) <= keep {
		return
	}
	newCache := make([]model.RelayLog, keep, relayLogMaxSizeNoDB)
	copy(newCache, relayLogCache[len(relayLogCache)-keep:])
	relayLogCache = newCache
}

func relayLogFlushToDB(ctx context.Context) error {
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()

	relayLogCacheLock.Lock()
	if len(relayLogCache) == 0 {
		relayLogCacheLock.Unlock()
		return nil
	}
	batch := make([]model.RelayLog, len(relayLogCache))
	copy(batch, relayLogCache)
	flushedUpto := len(batch)
	relayLogCacheLock.Unlock()

	result := db.GetDB().WithContext(ctx).Create(&batch)
	if result.Error != nil {
		// 失败时裁剪缓存：保留最近一批，避免 DB 故障期间缓存随请求数无界增长，
		// 也避免每次失败 flush 都全量拷贝 O(n²)。
		relayLogCacheLock.Lock()
		trimRelayLogCache(relayLogMaxSize)
		relayLogCacheLock.Unlock()
		return result.Error
	}

	relayLogCacheLock.Lock()
	if len(relayLogCache) >= flushedUpto {
		relayLogCache = relayLogCache[flushedUpto:]
	} else {
		relayLogCache = relayLogCache[:0]
	}
	if len(relayLogCache) == 0 {
		relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
	}
	relayLogCacheLock.Unlock()

	return nil
}

func RelayLogAdd(ctx context.Context, relayLog model.RelayLog) error {
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}
	maxSize := relayLogMaxSize
	if !enabled {
		maxSize = relayLogMaxSizeNoDB
	}
	relayLog.ID = snowflake.GenerateID()
	relayLog.RequestContent = truncateLogContent(relayLog.RequestContent)
	relayLog.ResponseContent = truncateLogContent(relayLog.ResponseContent)

	// Notify 已是非阻塞（RLock + select default），直接同步调用，避免每请求起 goroutine 的 GC 压力。
	if relayLogHub.HasSubscribers() {
		relayLogHub.Notify(relayLog)
	}

	relayLogCacheLock.Lock()
	relayLogCache = append(relayLogCache, relayLog)
	if len(relayLogCache) >= maxSize {
		if enabled {
			relayLogCacheLock.Unlock()
			// 异步刷库，避免请求路径每攒满一批就阻塞在 DB 写入上；
			// flushToDB 内部先摘出快照再写库，与后台任务并发安全。
			go func() {
				if err := relayLogFlushToDB(context.Background()); err != nil {
					log.Errorf("failed to flush relay logs: %v", err)
				}
			}()
			return nil
		}
		// 如果未启用日志保存，移除最旧的日志，保留最新的日志用于实时查询
		trimRelayLogCache(maxSize / 2)
	}
	relayLogCacheLock.Unlock()
	return nil
}

func RelayLogSaveDBTask(ctx context.Context) error {
	log.Debugf("relay log save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("relay log save db task finished, save time: %s", time.Since(startTime))
	}()
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}

	if enabled {
		if err := relayLogFlushToDB(ctx); err != nil {
			return err
		}
		return relayLogCleanup(ctx)
	}

	// 如果未启用日志保存，检查缓存大小，如果超过限制则清理旧日志
	relayLogCacheLock.Lock()
	trimRelayLogCache(relayLogMaxSizeNoDB / 2)
	relayLogCacheLock.Unlock()

	return nil
}

func relayLogCleanup(ctx context.Context) error {
	keepPeriod, err := SettingGetInt(model.SettingKeyRelayLogKeepPeriod)
	if err != nil {
		return err
	}

	if keepPeriod <= 0 {
		return nil
	}

	cutoffTime := time.Now().Add(-time.Duration(keepPeriod) * 24 * time.Hour).Unix()
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()
	return db.GetDB().WithContext(ctx).Where("time < ?", cutoffTime).Delete(&model.RelayLog{}).Error
}

// RelayLogList 查询日志列表，支持可选的时间范围过滤
// startTime 和 endTime 为 nil 时表示不限制时间范围
func RelayLogList(ctx context.Context, startTime, endTime *int, page, pageSize int) ([]model.RelayLog, error) {
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return nil, err
	}
	hasTimeFilter := startTime != nil && endTime != nil

	// 获取缓存中符合条件的日志
	relayLogCacheLock.Lock()
	var cachedLogs []model.RelayLog
	for _, log := range relayLogCache {
		if hasTimeFilter {
			if log.Time >= int64(*startTime) && log.Time <= int64(*endTime) {
				cachedLogs = append(cachedLogs, log)
			}
		} else {
			cachedLogs = append(cachedLogs, log)
		}
	}
	relayLogCacheLock.Unlock()

	// 反转缓存日志顺序（原本新的在末尾，反转后新的在前面，方便分页）
	for i, j := 0, len(cachedLogs)-1; i < j; i, j = i+1, j-1 {
		cachedLogs[i], cachedLogs[j] = cachedLogs[j], cachedLogs[i]
	}

	cacheCount := len(cachedLogs)
	offset := (page - 1) * pageSize

	var result []model.RelayLog

	// 先从缓存中取（缓存是最新的日志）
	if offset < cacheCount {
		cacheEnd := offset + pageSize
		if cacheEnd > cacheCount {
			cacheEnd = cacheCount
		}
		result = append(result, cachedLogs[offset:cacheEnd]...)
	}

	// 如果启用了日志保存，缓存不够时从数据库补充
	if enabled {
		remaining := pageSize - len(result)
		if remaining > 0 {
			dbOffset := 0
			if offset > cacheCount {
				dbOffset = offset - cacheCount
			}

			query := db.GetDB().WithContext(ctx)
			if hasTimeFilter {
				query = query.Where("time >= ? AND time <= ?", *startTime, *endTime)
			}
			// 排除仍在缓存中的日志：缓存 flush 后这些日志也进了 DB，
			// 不排除会与已返回的缓存页重复。
			if len(cachedLogs) > 0 {
				cachedIDs := make([]int64, 0, len(cachedLogs))
				for _, l := range cachedLogs {
					cachedIDs = append(cachedIDs, l.ID)
				}
				query = query.Where("id NOT IN ?", cachedIDs)
			}

			var dbLogs []model.RelayLog
			if err := query.Order("id DESC").Offset(dbOffset).Limit(remaining).Find(&dbLogs).Error; err != nil {
				return nil, err
			}
			result = append(result, dbLogs...)
		}
	}

	return result, nil
}

func RelayLogClear(ctx context.Context) error {
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()

	if err := db.GetDB().WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.RelayLog{}).Error; err != nil {
		return err
	}

	relayLogCacheLock.Lock()
	relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
	relayLogCacheLock.Unlock()
	return nil
}
