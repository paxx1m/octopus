package op

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
	"gorm.io/gorm"
)

var channelCache = cache.New[int, model.Channel](16)
var channelKeyCache = cache.New[int, model.ChannelKey](16)
var channelKeyDirty = newDirtySet()
var channelUpdateLocks sync.Map // channelID -> *sync.Mutex

func channelLock(channelID int) *sync.Mutex {
	v, _ := channelUpdateLocks.LoadOrStore(channelID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func ChannelList(ctx context.Context) ([]model.Channel, error) {
	channels := make([]model.Channel, 0, channelCache.Len())
	for _, channel := range channelCache.GetAll() {
		// 深拷贝，避免调用方修改 Keys/BaseUrls 等切片污染缓存
		channels = append(channels, cloneChannel(channel))
	}
	return channels, nil
}

func ChannelCreate(channel *model.Channel, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Create(channel).Error; err != nil {
		return err
	}
	channelCache.Set(channel.ID, *channel)
	for _, k := range channel.Keys {
		if k.ID != 0 {
			channelKeyCache.Set(k.ID, k)
		}
	}
	HealthNotify()
	return nil
}

// ChannelKeySetEnabled 立即将 Key 的 Enabled 写入缓存与数据库（用于 401/403 自动禁用）。
func ChannelKeySetEnabled(keyID, channelID int, enabled bool) error {
	if keyID == 0 || channelID == 0 {
		return fmt.Errorf("invalid channel key")
	}

	mu := channelLock(channelID)
	mu.Lock()
	defer mu.Unlock()

	ch, ok := channelCache.Get(channelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}

	if err := db.GetDB().Model(&model.ChannelKey{}).
		Where("id = ? AND channel_id = ?", keyID, channelID).
		Update("enabled", enabled).Error; err != nil {
		return err
	}

	found := false
	if len(ch.Keys) > 0 {
		keys := make([]model.ChannelKey, len(ch.Keys))
		copy(keys, ch.Keys)
		for i := range keys {
			if keys[i].ID == keyID {
				keys[i].Enabled = enabled
				channelKeyCache.Set(keyID, keys[i])
				found = true
				break
			}
		}
		ch.Keys = keys
		channelCache.Set(channelID, ch)
	}
	if !found {
		if k, ok := channelKeyCache.Get(keyID); ok {
			k.Enabled = enabled
			channelKeyCache.Set(keyID, k)
			// 渠道缓存中的 Keys 切片未同步该 key 的状态：后台整体刷新，
			// 避免 relay 仍按旧 Enabled 选中已禁用的 key。
			// 异步执行：本函数持有 channelLock，同步 refresh 会死锁。
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := channelRefreshCacheByID(channelID, ctx); err != nil {
					log.Warnf("failed to refresh channel %d after key %d disable: %v", channelID, keyID, err)
				}
			}()
		}
	}
	HealthNotify()
	return nil
}

// ChannelKeyApplyUpdate 原子地应用运行时 key 状态与费用增量（不落库，标记 dirty）。
func ChannelKeyApplyUpdate(keyID, channelID, statusCode int, lastUseTimeStamp int64, costDelta float64) error {
	if keyID == 0 || channelID == 0 {
		return fmt.Errorf("invalid channel key")
	}

	mu := channelLock(channelID)
	mu.Lock()
	defer mu.Unlock()

	ch, ok := channelCache.Get(channelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}

	var updated model.ChannelKey
	found := false
	if len(ch.Keys) > 0 {
		keys := make([]model.ChannelKey, len(ch.Keys))
		copy(keys, ch.Keys)
		for i := range keys {
			if keys[i].ID == keyID {
				keys[i].StatusCode = statusCode
				keys[i].LastUseTimeStamp = lastUseTimeStamp
				keys[i].TotalCost += costDelta
				updated = keys[i]
				found = true
				break
			}
		}
		if found {
			ch.Keys = keys
		}
	}
	if !found {
		k, ok := channelKeyCache.Get(keyID)
		if !ok {
			return fmt.Errorf("channel key not found")
		}
		k.StatusCode = statusCode
		k.LastUseTimeStamp = lastUseTimeStamp
		k.TotalCost += costDelta
		updated = k
	}

	channelCache.Set(channelID, ch)
	channelKeyCache.Set(keyID, updated)
	channelKeyDirty.Mark(keyID)
	return nil
}

func ChannelBaseUrlUpdate(channelID int, baseUrl []model.BaseUrl) error {
	mu := channelLock(channelID)
	mu.Lock()
	defer mu.Unlock()

	ch, ok := channelCache.Get(channelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	if baseUrl == nil {
		ch.BaseUrls = nil
	} else {
		cp := make([]model.BaseUrl, len(baseUrl))
		copy(cp, baseUrl)
		ch.BaseUrls = cp
	}
	channelCache.Set(channelID, ch)

	if err := db.GetDB().Model(&model.Channel{}).Where("id = ?", channelID).
		Select("base_urls").Updates(&model.Channel{BaseUrls: ch.BaseUrls}).Error; err != nil {
		return fmt.Errorf("failed to persist base urls: %w", err)
	}
	return nil
}

// ChannelKeySaveDB 将运行时更新过的 ChannelKey 缓存写入数据库。
func ChannelKeySaveDB(ctx context.Context) error {
	return channelKeyPersist(ctx, channelKeyDirty.Snapshot())
}

// channelKeySaveDBByChannel 将指定渠道下 dirty 的 key 落库。
func channelKeySaveDBByChannel(ctx context.Context, channelID int) error {
	keyIDs := channelKeyDirty.SnapshotFiltered(func(id int) bool {
		k, ok := channelKeyCache.Get(id)
		return ok && k.ChannelID == channelID
	})
	return channelKeyPersist(ctx, keyIDs)
}

func channelKeyPersist(ctx context.Context, keyIDs []int) error {
	if len(keyIDs) == 0 {
		return nil
	}
	err := db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range keyIDs {
			k, ok := channelKeyCache.Get(id)
			if !ok {
				continue
			}
			if err := tx.Save(&k).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		channelKeyDirty.Remerge(keyIDs)
		return err
	}
	return nil
}

func ChannelUpdate(req *model.ChannelUpdateRequest, ctx context.Context) (channel *model.Channel, err error) {
	_, ok := channelCache.Get(req.ID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			// 不静默吞掉 panic：回滚并返回显式错误，避免调用方拿到 (nil, nil)
			tx.Rollback()
			channel = nil
			err = fmt.Errorf("channel update panicked: %v", r)
		}
	}()

	var updates = newPartialUpdate()
	updates.set(&model.Channel{}, "name", req.Name)
	updates.set(&model.Channel{}, "type", req.Type)
	updates.set(&model.Channel{}, "enabled", req.Enabled)
	updates.set(&model.Channel{}, "base_urls", req.BaseUrls)
	updates.set(&model.Channel{}, "model", req.Model)
	updates.set(&model.Channel{}, "custom_model", req.CustomModel)
	updates.set(&model.Channel{}, "proxy", req.Proxy)
	updates.set(&model.Channel{}, "auto_sync", req.AutoSync)
	updates.set(&model.Channel{}, "auto_group", req.AutoGroup)
	updates.set(&model.Channel{}, "custom_header", req.CustomHeader)
	updates.set(&model.Channel{}, "channel_proxy", req.ChannelProxy)
	updates.set(&model.Channel{}, "param_override", req.ParamOverride)
	updates.set(&model.Channel{}, "match_regex", req.MatchRegex)
	updates.set(&model.Channel{}, "key_mode", req.KeyMode)
	updates.set(&model.Channel{}, "allow_empty_key", req.AllowEmptyKey)

	// 只有当有字段需要更新时才执行 UPDATE
	if len(updates) > 0 {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).Updates(updates.gormMap()).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update channel: %w", err)
		}
	}

	// rate_limit_cooldown_sec：用 ClearRateLimitCooldown 区分「未传」与「清空继承全局」
	if req.ClearRateLimitCooldown {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).
			Update("rate_limit_cooldown_sec", nil).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to clear channel rate limit cooldown: %w", err)
		}
	} else if req.RateLimitCooldownSec != nil {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).
			Update("rate_limit_cooldown_sec", *req.RateLimitCooldownSec).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update channel rate limit cooldown: %w", err)
		}
	}

	// 删除 keys
	if len(req.KeysToDelete) > 0 {
		if err := tx.Where("id IN ? AND channel_id = ?", req.KeysToDelete, req.ID).Delete(&model.ChannelKey{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete channel keys: %w", err)
		}
	}

	// 更新 keys（逐条，只更新提供的字段）
	if len(req.KeysToUpdate) > 0 {
		for _, ku := range req.KeysToUpdate {
			updates := newPartialUpdate()
			updates.set(&model.ChannelKey{}, "enabled", ku.Enabled)
			updates.set(&model.ChannelKey{}, "channel_key", ku.ChannelKey)
			updates.set(&model.ChannelKey{}, "remark", ku.Remark)
			if ku.Weight != nil {
				w := *ku.Weight
				if w <= 0 {
					w = 1
				}
				updates["weight"] = w
			}
			if ku.ClearRateLimitCooldown {
				updates["rate_limit_cooldown_sec"] = nil
			} else {
				updates.set(&model.ChannelKey{}, "rate_limit_cooldown_sec", ku.RateLimitCooldownSec)
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.ChannelKey{}).
				Where("id = ? AND channel_id = ?", ku.ID, req.ID).
				Updates(updates.gormMap()).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to update channel key %d: %w", ku.ID, err)
			}
		}
	}

	// 新增 keys
	if len(req.KeysToAdd) > 0 {
		newKeys := make([]model.ChannelKey, 0, len(req.KeysToAdd))
		for _, ka := range req.KeysToAdd {
			newKeys = append(newKeys, model.ChannelKey{
				ChannelID:            req.ID,
				Enabled:              ka.Enabled,
				ChannelKey:           ka.ChannelKey,
				Remark:               ka.Remark,
				Weight:               model.KeyWeight(ka.Weight),
				RateLimitCooldownSec: ka.RateLimitCooldownSec,
			})
		}
		if err := tx.Create(&newKeys).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create channel keys: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 刷新缓存并返回最新数据；脱离请求取消信号——事务已提交，客户端断开不应导致
	// "DB 已更新但缓存停留在旧值、接口返回 500"。
	if err := channelRefreshCacheByID(req.ID, context.WithoutCancel(ctx)); err != nil {
		return nil, err
	}

	cached, _ := channelCache.Get(req.ID)
	channel = &cached
	HealthNotify()
	return channel, nil
}

func ChannelEnabled(id int, enabled bool, ctx context.Context) error {
	mu := channelLock(id)
	mu.Lock()
	defer mu.Unlock()

	oldChannel, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	if err := db.GetDB().WithContext(ctx).Model(&model.Channel{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}
	oldChannel.Enabled = enabled
	channelCache.Set(id, oldChannel)
	HealthNotify()
	return nil
}

func ChannelDel(id int, ctx context.Context) (err error) {
	ch, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			err = fmt.Errorf("channel delete panicked: %v", r)
		}
	}()

	// 获取所有受影响的 GroupID，用于刷新缓存
	var affectedGroupIDs []int
	if err := tx.Model(&model.GroupItem{}).
		Where("channel_id = ?", id).
		Pluck("group_id", &affectedGroupIDs).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get affected groups: %w", err)
	}

	// 删除所有引用该渠道的 GroupItem
	if err := tx.Where("channel_id = ?", id).Delete(&model.GroupItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group items: %w", err)
	}

	// 删除渠道 keys
	if err := tx.Where("channel_id = ?", id).Delete(&model.ChannelKey{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel keys: %w", err)
	}

	// 删除统计数据
	if err := tx.Where("channel_id = ?", id).Delete(&model.StatsChannel{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel stats: %w", err)
	}

	// 删除渠道
	if err := tx.Delete(&model.Channel{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 与 channelRefreshCacheByID 共享同一把锁：防止并发 refresh（读 DB → Set 缓存）
	// 在删除提交后把旧数据写回缓存，导致已删渠道"幽灵复活"。
	mu := channelLock(id)
	mu.Lock()
	defer mu.Unlock()

	// 删除缓存
	channelCache.Del(id)
	keyIDs := make([]int, 0, len(ch.Keys))
	for _, k := range ch.Keys {
		if k.ID != 0 {
			channelKeyCache.Del(k.ID)
			keyIDs = append(keyIDs, k.ID)
		}
	}
	channelKeyDirty.Remove(keyIDs...)
	channelUpdateLocks.Delete(id)
	// DB 删除已在事务内完成，这里只清缓存，避免重复 DELETE
	StatsChannelCacheRemove(id)

	HealthNotify()

	// 刷新受影响的分组缓存
	for _, groupID := range affectedGroupIDs {
		if err := groupRefreshCacheByID(groupID, ctx); err != nil {
			log.Warnf("failed to refresh group cache for group %d: %v", groupID, err)
		}
	}

	return nil
}

func ChannelLLMList(ctx context.Context) ([]model.LLMChannel, error) {
	models := []model.LLMChannel{}
	for _, channel := range channelCache.GetAll() {
		modelNames := xstrings.SplitTrimCompact(",", channel.Model, channel.CustomModel)
		for _, modelName := range modelNames {
			if modelName == "" {
				continue
			}
			models = append(models, model.LLMChannel{
				Name:        modelName,
				Enabled:     channel.Enabled,
				ChannelID:   channel.ID,
				ChannelName: channel.Name,
			})
		}
	}
	return models, nil
}

// ChannelGet 返回渠道的独立快照（值拷贝 + 切片深拷贝）。
// 调用方对返回值及其 Keys/BaseUrls 等切片的修改不会影响缓存。
func ChannelGet(id int, ctx context.Context) (model.Channel, error) {
	channel, ok := channelCache.Get(id)
	if !ok {
		return model.Channel{}, fmt.Errorf("channel not found")
	}
	return cloneChannel(channel), nil
}

// cloneChannel 深拷贝渠道中可能被共享的切片字段。
func cloneChannel(ch model.Channel) model.Channel {
	if len(ch.BaseUrls) > 0 {
		cp := make([]model.BaseUrl, len(ch.BaseUrls))
		copy(cp, ch.BaseUrls)
		ch.BaseUrls = cp
	}
	if len(ch.Keys) > 0 {
		cp := make([]model.ChannelKey, len(ch.Keys))
		copy(cp, ch.Keys)
		ch.Keys = cp
	}
	if len(ch.CustomHeader) > 0 {
		cp := make([]model.CustomHeader, len(ch.CustomHeader))
		copy(cp, ch.CustomHeader)
		ch.CustomHeader = cp
	}
	if ch.ParamOverride != nil {
		v := *ch.ParamOverride
		ch.ParamOverride = &v
	}
	if ch.ChannelProxy != nil {
		v := *ch.ChannelProxy
		ch.ChannelProxy = &v
	}
	if ch.MatchRegex != nil {
		v := *ch.MatchRegex
		ch.MatchRegex = &v
	}
	if ch.Stats != nil {
		s := *ch.Stats
		ch.Stats = &s
	}
	return ch
}

func channelRefreshCache(ctx context.Context) error {
	channels := []model.Channel{}
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		Find(&channels).Error; err != nil {
		log.Warnf("failed to get channels: %v", err)
		return err
	}
	channelCache.Clear()
	channelKeyCache.Clear()
	channelKeyDirty.Clear()
	for _, channel := range channels {
		channelCache.Set(channel.ID, channel)
		for _, k := range channel.Keys {
			if k.ID != 0 {
				channelKeyCache.Set(k.ID, k)
			}
		}
	}
	return nil
}

func channelRefreshCacheByID(id int, ctx context.Context) error {
	// Persist runtime key costs/status before reload so they are not wiped.
	if err := channelKeySaveDBByChannel(ctx, id); err != nil {
		log.Warnf("failed to flush channel %d keys before refresh: %v", id, err)
	}

	mu := channelLock(id)
	mu.Lock()
	defer mu.Unlock()

	// Capture runtime fields that may still be dirty or just flushed but
	// also re-applied concurrently; merge into DB snapshot.
	runtimeByID := map[int]model.ChannelKey{}
	if old, ok := channelCache.Get(id); ok {
		for _, k := range old.Keys {
			if k.ID != 0 {
				runtimeByID[k.ID] = k
			}
		}
	}

	var channel model.Channel
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		First(&channel, id).Error; err != nil {
		return err
	}

	for i := range channel.Keys {
		k := &channel.Keys[i]
		if rt, ok := runtimeByID[k.ID]; ok {
			// Keep live accounting / rate-limit state over DB snapshot when dirty
			// was just flushed OR concurrent ApplyUpdate raced the flush.
			if channelKeyDirty.IsDirty(k.ID) {
				k.TotalCost = rt.TotalCost
				k.StatusCode = rt.StatusCode
				k.LastUseTimeStamp = rt.LastUseTimeStamp
			} else if rt.TotalCost > k.TotalCost || rt.LastUseTimeStamp > k.LastUseTimeStamp {
				// Prefer higher cost / newer use time if flush and concurrent update raced.
				if rt.TotalCost > k.TotalCost {
					k.TotalCost = rt.TotalCost
				}
				if rt.LastUseTimeStamp >= k.LastUseTimeStamp {
					k.LastUseTimeStamp = rt.LastUseTimeStamp
					k.StatusCode = rt.StatusCode
				}
			}
		}
	}

	// Drop old key cache entries for this channel
	for id := range runtimeByID {
		channelKeyCache.Del(id)
	}
	channelCache.Set(channel.ID, channel)
	for _, k := range channel.Keys {
		if k.ID != 0 {
			channelKeyCache.Set(k.ID, k)
		}
	}
	return nil
}
