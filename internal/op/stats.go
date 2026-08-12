package op

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const statsDailyRetentionDays = 365

var statsDailyCache model.StatsDaily
var statsDailyCacheLock sync.RWMutex

// statsDailyPending holds previous-day snapshots awaiting successful DB write.
var statsDailyPending []model.StatsDaily
var statsDailyPendingLock sync.Mutex

var statsTotalCache model.StatsTotal
var statsTotalCacheLock sync.RWMutex

var statsHourlyCache [24]model.StatsHourly
var statsHourlyCacheLock sync.RWMutex

var statsChannelCache = cache.New[int, model.StatsChannel](16)
var statsChannelDirty = newDirtySet()
var statsChannelUpdateLock sync.RWMutex

var statsModelCache = cache.New[int, model.StatsModel](16)
var statsModelDirty = newDirtySet()
var statsModelUpdateLock sync.RWMutex

var statsAPIKeyCache = cache.New[int, model.StatsAPIKey](16)
var statsAPIKeyDirty = newDirtySet()
var statsAPIKeyUpdateLock sync.RWMutex

func StatsSaveDBTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log.Debugf("stats save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("stats save db task finished, save time: %s", time.Since(startTime))
	}()
	if err := StatsSaveDB(ctx); err != nil {
		log.Errorf("stats save db error: %v", err)
		return
	}
	if err := statsDailyCleanup(ctx); err != nil {
		log.Warnf("stats daily cleanup error: %v", err)
	}
}

func StatsSaveDB(ctx context.Context) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsDailyCacheLock.RLock()
	dailySnap := statsDailyCache
	statsDailyCacheLock.RUnlock()

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	channelIDs := statsChannelDirty.Snapshot()
	modelIDs := statsModelDirty.Snapshot()
	apiKeyIDs := statsAPIKeyDirty.Snapshot()

	statsDailyPendingLock.Lock()
	pendingDaily := make([]model.StatsDaily, len(statsDailyPending))
	copy(pendingDaily, statsDailyPending)
	statsDailyPendingLock.Unlock()

	statsHourlyPendingLock.Lock()
	pendingHourly := make([]model.StatsHourly, len(statsHourlyPending))
	copy(pendingHourly, statsHourlyPending)
	statsHourlyPendingLock.Unlock()

	if err := persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlyAll, channelIDs, modelIDs, apiKeyIDs, pendingDaily, pendingHourly); err != nil {
		statsChannelDirty.Remerge(channelIDs)
		statsModelDirty.Remerge(modelIDs)
		statsAPIKeyDirty.Remerge(apiKeyIDs)
		return err
	}

	if len(pendingDaily) > 0 {
		statsDailyPendingLock.Lock()
		// 按元素身份剔除本次已落库的快照，避免并发 append 时按长度推断误删
		drop := make(map[model.StatsDaily]struct{}, len(pendingDaily))
		for _, p := range pendingDaily {
			drop[p] = struct{}{}
		}
		remaining := statsDailyPending[:0]
		for _, p := range statsDailyPending {
			if _, ok := drop[p]; !ok {
				remaining = append(remaining, p)
			}
		}
		statsDailyPending = remaining
		statsDailyPendingLock.Unlock()
	}
	if len(pendingHourly) > 0 {
		statsHourlyPendingLock.Lock()
		drop := make(map[model.StatsHourly]struct{}, len(pendingHourly))
		for _, p := range pendingHourly {
			drop[p] = struct{}{}
		}
		remaining := statsHourlyPending[:0]
		for _, p := range statsHourlyPending {
			if _, ok := drop[p]; !ok {
				remaining = append(remaining, p)
			}
		}
		statsHourlyPending = remaining
		statsHourlyPendingLock.Unlock()
	}
	return nil
}

func persistStatsSnapshots(
	ctx context.Context,
	totalSnap model.StatsTotal,
	dailySnap model.StatsDaily,
	hourlyAll [24]model.StatsHourly,
	channelIDs []int,
	modelIDs []int,
	apiKeyIDs []int,
	pendingDaily []model.StatsDaily,
	pendingHourly []model.StatsHourly,
) error {
	dbConn := db.GetDB().WithContext(ctx)

	return dbConn.Transaction(func(tx *gorm.DB) error {
		if result := tx.Save(&totalSnap); result.Error != nil {
			return result.Error
		}

		for i := range pendingDaily {
			if pendingDaily[i].Date == "" {
				continue
			}
			if result := tx.Save(&pendingDaily[i]); result.Error != nil {
				return result.Error
			}
		}

		if dailySnap.Date != "" {
			if result := tx.Save(&dailySnap); result.Error != nil {
				return result.Error
			}
		}

		// StatsHourly 主键仅 hour：跨日 pending 与今日 slot 同 hour 时，
		// 先写 pending 再写今日，最终落库为今日；pending 主要用于进程内不丢累计路径上的 best-effort。
		// 若需完整历史小时，应改为 (date,hour) 复合主键（后续迁移）。
		todayDate := time.Now().Format("20060102")
		hourlyStats := make([]model.StatsHourly, 0, 24+len(pendingHourly))
		// 今日 slot 优先；pending 仅当该 hour 今日尚无数据时写入（避免覆盖今日）
		todayHours := make(map[int]struct{}, 24)
		for hour := 0; hour < 24; hour++ {
			if hourlyAll[hour].Date == todayDate {
				hourlyStats = append(hourlyStats, hourlyAll[hour])
				todayHours[hour] = struct{}{}
			}
		}
		for i := range pendingHourly {
			h := pendingHourly[i]
			if h.Date == "" || h.Hour < 0 || h.Hour >= 24 {
				continue
			}
			if _, hasToday := todayHours[h.Hour]; hasToday {
				continue
			}
			hourlyStats = append(hourlyStats, h)
		}
		if len(hourlyStats) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "hour"}},
				UpdateAll: true,
			}).Create(&hourlyStats); result.Error != nil {
				return result.Error
			}
		}

		for _, id := range channelIDs {
			ch, ok := statsChannelCache.Get(id)
			if !ok {
				continue
			}
			if result := tx.Save(&ch); result.Error != nil {
				return result.Error
			}
		}

		for _, id := range modelIDs {
			m, ok := statsModelCache.Get(id)
			if !ok {
				continue
			}
			if result := tx.Save(&m); result.Error != nil {
				return result.Error
			}
		}

		for _, id := range apiKeyIDs {
			ak, ok := statsAPIKeyCache.Get(id)
			if !ok {
				continue
			}
			if result := tx.Save(&ak); result.Error != nil {
				return result.Error
			}
		}

		return nil
	})
}

func StatsDailyUpdate(metrics model.StatsMetrics) error {
	today := time.Now().Format("20060102")

	statsDailyCacheLock.Lock()
	if statsDailyCache.Date == today {
		statsDailyCache.StatsMetrics.Add(metrics)
		statsDailyCacheLock.Unlock()
		return nil
	}

	prevDaily := statsDailyCache
	statsDailyCache = model.StatsDaily{Date: today}
	statsDailyCache.StatsMetrics.Add(metrics)
	statsDailyCacheLock.Unlock()

	if prevDaily.Date != "" {
		statsDailyPendingLock.Lock()
		statsDailyPending = append(statsDailyPending, prevDaily)
		statsDailyPendingLock.Unlock()
	}

	// 跨日落库改为异步，避免新一天第一个请求承担全量持久化的延迟尖峰；
	// 失败时数据仍留在 pending，由后台 StatsSaveDBTask 兜底重试。
	go func() {
		if err := StatsSaveDB(context.Background()); err != nil {
			log.Warnf("failed to persist daily stats on day rollover: %v", err)
		}
	}()
	return nil
}

func StatsTotalUpdate(metrics model.StatsMetrics) error {
	statsTotalCacheLock.Lock()
	defer statsTotalCacheLock.Unlock()
	if statsTotalCache.ID == 0 {
		statsTotalCache.ID = 1
	}
	statsTotalCache.StatsMetrics.Add(metrics)
	return nil
}

func StatsChannelUpdate(channelID int, metrics model.StatsMetrics) error {
	statsChannelUpdateLock.Lock()
	defer statsChannelUpdateLock.Unlock()

	channelCache, ok := statsChannelCache.Get(channelID)
	if !ok {
		channelCache = model.StatsChannel{
			ChannelID: channelID,
		}
	}
	channelCache.StatsMetrics.Add(metrics)
	statsChannelCache.Set(channelID, channelCache)
	statsChannelDirty.Mark(channelID)
	return nil
}

var statsHourlyPending []model.StatsHourly
var statsHourlyPendingLock sync.Mutex

func StatsHourlyUpdate(metrics model.StatsMetrics) error {
	now := time.Now()
	nowHour := now.Hour()
	todayDate := now.Format("20060102")

	statsHourlyCacheLock.Lock()
	slot := statsHourlyCache[nowHour]
	var prev *model.StatsHourly
	if slot.Date != "" && slot.Date != todayDate {
		// 跨日：旧小时快照进入 pending，与 daily 对称，避免未落库数据丢失
		cp := slot
		prev = &cp
		statsHourlyCache[nowHour] = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	} else if slot.Date == "" {
		statsHourlyCache[nowHour] = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	}
	statsHourlyCache[nowHour].StatsMetrics.Add(metrics)
	statsHourlyCacheLock.Unlock()

	if prev != nil {
		statsHourlyPendingLock.Lock()
		statsHourlyPending = append(statsHourlyPending, *prev)
		statsHourlyPendingLock.Unlock()
	}
	return nil
}

func StatsModelUpdate(stats model.StatsModel) error {
	statsModelUpdateLock.Lock()
	defer statsModelUpdateLock.Unlock()

	modelCache, ok := statsModelCache.Get(stats.ID)
	if !ok {
		modelCache = model.StatsModel{
			ID: stats.ID,
		}
	}
	modelCache.StatsMetrics.Add(stats.StatsMetrics)
	statsModelCache.Set(stats.ID, modelCache)
	statsModelDirty.Mark(stats.ID)
	return nil
}

func StatsAPIKeyUpdate(apiKeyID int, metrics model.StatsMetrics) error {
	statsAPIKeyUpdateLock.Lock()
	defer statsAPIKeyUpdateLock.Unlock()

	apiKeyCache, ok := statsAPIKeyCache.Get(apiKeyID)
	if !ok {
		apiKeyCache = model.StatsAPIKey{
			APIKeyID: apiKeyID,
		}
	}
	apiKeyCache.StatsMetrics.Add(metrics)
	statsAPIKeyCache.Set(apiKeyID, apiKeyCache)
	statsAPIKeyDirty.Mark(apiKeyID)
	return nil
}

func StatsChannelDel(id int) error {
	statsChannelUpdateLock.Lock()
	defer statsChannelUpdateLock.Unlock()

	if _, ok := statsChannelCache.Get(id); !ok {
		return nil
	}
	statsChannelCache.Del(id)
	statsChannelDirty.Remove(id)
	return db.GetDB().Delete(&model.StatsChannel{}, id).Error
}

// StatsChannelCacheRemove 仅清理缓存与脏标记；DB 删除已由调用方在事务内完成。
func StatsChannelCacheRemove(id int) {
	statsChannelUpdateLock.Lock()
	defer statsChannelUpdateLock.Unlock()
	statsChannelCache.Del(id)
	statsChannelDirty.Remove(id)
}

func StatsAPIKeyDel(id int) error {
	statsAPIKeyUpdateLock.Lock()
	defer statsAPIKeyUpdateLock.Unlock()

	if _, ok := statsAPIKeyCache.Get(id); !ok {
		return nil
	}
	statsAPIKeyCache.Del(id)
	statsAPIKeyDirty.Remove(id)
	return db.GetDB().Delete(&model.StatsAPIKey{}, id).Error
}

func StatsTotalGet() model.StatsTotal {
	statsTotalCacheLock.RLock()
	defer statsTotalCacheLock.RUnlock()
	return statsTotalCache
}

func StatsTodayGet() model.StatsDaily {
	statsDailyCacheLock.RLock()
	defer statsDailyCacheLock.RUnlock()
	return statsDailyCache
}

func StatsChannelGet(id int) model.StatsChannel {
	// 快速路径：只读命中无需独占锁（每请求热路径，auth 中间件与 channel 列表调用）
	statsChannelUpdateLock.RLock()
	stats, ok := statsChannelCache.Get(id)
	statsChannelUpdateLock.RUnlock()
	if ok {
		return stats
	}

	statsChannelUpdateLock.Lock()
	defer statsChannelUpdateLock.Unlock()
	if stats, ok := statsChannelCache.Get(id); ok {
		return stats
	}
	tmp := model.StatsChannel{
		ChannelID: id,
	}
	statsChannelCache.Set(id, tmp)
	statsChannelDirty.Mark(id)
	return tmp
}

func StatsAPIKeyGet(id int) model.StatsAPIKey {
	statsAPIKeyUpdateLock.RLock()
	stats, ok := statsAPIKeyCache.Get(id)
	statsAPIKeyUpdateLock.RUnlock()
	if ok {
		return stats
	}

	statsAPIKeyUpdateLock.Lock()
	defer statsAPIKeyUpdateLock.Unlock()
	if stats, ok := statsAPIKeyCache.Get(id); ok {
		return stats
	}
	tmp := model.StatsAPIKey{
		APIKeyID: id,
	}
	statsAPIKeyCache.Set(id, tmp)
	statsAPIKeyDirty.Mark(id)
	return tmp
}

func StatsAPIKeyList() []model.StatsAPIKey {
	apiKeys := make([]model.StatsAPIKey, 0, statsAPIKeyCache.Len())
	for _, v := range statsAPIKeyCache.GetAll() {
		apiKeys = append(apiKeys, v)
	}
	return apiKeys
}

func StatsHourlyGet() []model.StatsHourly {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.RLock()
	defer statsHourlyCacheLock.RUnlock()

	result := make([]model.StatsHourly, 0, currentHour+1)

	for hour := 0; hour <= currentHour; hour++ {
		if statsHourlyCache[hour].Date == todayDate {
			result = append(result, statsHourlyCache[hour])
		} else {
			result = append(result, model.StatsHourly{
				Hour: hour,
				Date: todayDate,
			})
		}
	}

	return result
}

func StatsGetDaily(ctx context.Context) ([]model.StatsDaily, error) {
	var statsDaily []model.StatsDaily
	cutoff := time.Now().AddDate(0, 0, -statsDailyRetentionDays).Format("20060102")
	result := db.GetDB().WithContext(ctx).
		Where("date >= ?", cutoff).
		Order("date ASC").
		Find(&statsDaily)
	if result.Error != nil {
		return nil, result.Error
	}
	return statsDaily, nil
}

func statsDailyCleanup(ctx context.Context) error {
	cutoff := time.Now().AddDate(0, 0, -statsDailyRetentionDays).Format("20060102")
	return db.GetDB().WithContext(ctx).Where("date < ?", cutoff).Delete(&model.StatsDaily{}).Error
}

func statsRefreshCache(ctx context.Context) error {
	dbConn := db.GetDB().WithContext(ctx)
	today := time.Now().Format("20060102")

	var loadedDaily model.StatsDaily
	result := dbConn.Last(&loadedDaily)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get daily stats: %v", result.Error)
	}
	if result.RowsAffected == 0 || loadedDaily.Date != today {
		loadedDaily = model.StatsDaily{Date: today}
	}

	var loadedTotal model.StatsTotal
	result = dbConn.First(&loadedTotal)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get total stats: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		loadedTotal = model.StatsTotal{ID: 1}
	} else if loadedTotal.ID == 0 {
		loadedTotal.ID = 1
	}

	var loadedChannels []model.StatsChannel
	result = dbConn.Find(&loadedChannels)
	if result.Error != nil {
		return fmt.Errorf("failed to get channels: %v", result.Error)
	}

	var loadedHourly []model.StatsHourly
	result = dbConn.Find(&loadedHourly)
	if result.Error != nil {
		return fmt.Errorf("failed to get hourly stats: %v", result.Error)
	}

	statsDailyCacheLock.Lock()
	statsDailyCache = loadedDaily
	statsDailyCacheLock.Unlock()

	statsDailyPendingLock.Lock()
	statsDailyPending = nil
	statsDailyPendingLock.Unlock()

	statsTotalCacheLock.Lock()
	statsTotalCache = loadedTotal
	statsTotalCacheLock.Unlock()

	statsChannelCache.Clear()
	statsChannelDirty.Clear()
	for _, v := range loadedChannels {
		statsChannelCache.Set(v.ChannelID, v)
	}

	var loadedAPIKeys []model.StatsAPIKey
	result = dbConn.Find(&loadedAPIKeys)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key stats: %v", result.Error)
	}

	statsAPIKeyCache.Clear()
	statsAPIKeyDirty.Clear()
	for _, v := range loadedAPIKeys {
		statsAPIKeyCache.Set(v.APIKeyID, v)
	}

	statsHourlyCacheLock.Lock()
	statsHourlyCache = [24]model.StatsHourly{}
	for _, v := range loadedHourly {
		if v.Hour >= 0 && v.Hour < 24 {
			statsHourlyCache[v.Hour] = v
		}
	}
	statsHourlyCacheLock.Unlock()

	return nil
}
