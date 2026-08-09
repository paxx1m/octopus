package op

import (
	"context"
	"fmt"
	"sync"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
	"gorm.io/gorm"
)

var channelCache = cache.New[int, model.Channel](16)
var channelKeyCache = cache.New[int, model.ChannelKey](16)
var channelKeyCacheNeedUpdate = make(map[int]struct{})
var channelKeyCacheNeedUpdateLock sync.Mutex
var channelUpdateLocks sync.Map // channelID -> *sync.Mutex

func channelLock(channelID int) *sync.Mutex {
	v, _ := channelUpdateLocks.LoadOrStore(channelID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func ChannelList(ctx context.Context) ([]model.Channel, error) {
	channels := make([]model.Channel, 0, channelCache.Len())
	for _, channel := range channelCache.GetAll() {
		channels = append(channels, channel)
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

	if len(ch.Keys) > 0 {
		keys := make([]model.ChannelKey, len(ch.Keys))
		copy(keys, ch.Keys)
		for i := range keys {
			if keys[i].ID == keyID {
				keys[i].Enabled = enabled
				channelKeyCache.Set(keyID, keys[i])
				break
			}
		}
		ch.Keys = keys
		channelCache.Set(channelID, ch)
	} else if k, ok := channelKeyCache.Get(keyID); ok {
		k.Enabled = enabled
		channelKeyCache.Set(keyID, k)
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
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate[keyID] = struct{}{}
	channelKeyCacheNeedUpdateLock.Unlock()
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
	return channelKeyPersist(ctx, snapshotDirtyIDs(&channelKeyCacheNeedUpdate, &channelKeyCacheNeedUpdateLock))
}

// channelKeySaveDBByChannel 将指定渠道下 dirty 的 key 落库。
func channelKeySaveDBByChannel(ctx context.Context, channelID int) error {
	channelKeyCacheNeedUpdateLock.Lock()
	keyIDs := make([]int, 0)
	for id := range channelKeyCacheNeedUpdate {
		if k, ok := channelKeyCache.Get(id); ok && k.ChannelID == channelID {
			keyIDs = append(keyIDs, id)
			delete(channelKeyCacheNeedUpdate, id)
		}
	}
	channelKeyCacheNeedUpdateLock.Unlock()
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
		remergeDirtyIDs(&channelKeyCacheNeedUpdate, &channelKeyCacheNeedUpdateLock, keyIDs)
		return err
	}
	return nil
}

func ChannelUpdate(req *model.ChannelUpdateRequest, ctx context.Context) (*model.Channel, error) {
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
			tx.Rollback()
		}
	}()

	var selectFields []string
	updates := model.Channel{ID: req.ID}

	if req.Name != nil {
		selectFields = append(selectFields, "name")
		updates.Name = *req.Name
	}
	if req.Type != nil {
		selectFields = append(selectFields, "type")
		updates.Type = *req.Type
	}
	if req.Enabled != nil {
		selectFields = append(selectFields, "enabled")
		updates.Enabled = *req.Enabled
	}
	if req.BaseUrls != nil {
		selectFields = append(selectFields, "base_urls")
		updates.BaseUrls = *req.BaseUrls
	}
	if req.Model != nil {
		selectFields = append(selectFields, "model")
		updates.Model = *req.Model
	}
	if req.CustomModel != nil {
		selectFields = append(selectFields, "custom_model")
		updates.CustomModel = *req.CustomModel
	}
	if req.Proxy != nil {
		selectFields = append(selectFields, "proxy")
		updates.Proxy = *req.Proxy
	}
	if req.AutoSync != nil {
		selectFields = append(selectFields, "auto_sync")
		updates.AutoSync = *req.AutoSync
	}
	if req.AutoGroup != nil {
		selectFields = append(selectFields, "auto_group")
		updates.AutoGroup = *req.AutoGroup
	}
	if req.CustomHeader != nil {
		selectFields = append(selectFields, "custom_header")
		updates.CustomHeader = *req.CustomHeader
	}
	if req.ChannelProxy != nil {
		selectFields = append(selectFields, "channel_proxy")
		updates.ChannelProxy = req.ChannelProxy
	}
	if req.ParamOverride != nil {
		selectFields = append(selectFields, "param_override")
		updates.ParamOverride = req.ParamOverride
	}
	if req.MatchRegex != nil {
		selectFields = append(selectFields, "match_regex")
		updates.MatchRegex = req.MatchRegex
	}
	if req.KeyMode != nil {
		selectFields = append(selectFields, "key_mode")
		updates.KeyMode = *req.KeyMode
	}
	if req.AllowEmptyKey != nil {
		selectFields = append(selectFields, "allow_empty_key")
		updates.AllowEmptyKey = *req.AllowEmptyKey
	}

	// 只有当有字段需要更新时才执行 UPDATE
	if len(selectFields) > 0 {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).Select(selectFields).Updates(&updates).Error; err != nil {
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
			updates := map[string]interface{}{}
			if ku.Enabled != nil {
				updates["enabled"] = *ku.Enabled
			}
			if ku.ChannelKey != nil {
				updates["channel_key"] = *ku.ChannelKey
			}
			if ku.Remark != nil {
				updates["remark"] = *ku.Remark
			}
			if ku.Weight != nil {
				w := *ku.Weight
				if w <= 0 {
					w = 1
				}
				updates["weight"] = w
			}
			if ku.ClearRateLimitCooldown {
				updates["rate_limit_cooldown_sec"] = nil
			} else if ku.RateLimitCooldownSec != nil {
				updates["rate_limit_cooldown_sec"] = *ku.RateLimitCooldownSec
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.ChannelKey{}).
				Where("id = ? AND channel_id = ?", ku.ID, req.ID).
				Updates(updates).Error; err != nil {
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

	// 刷新缓存并返回最新数据
	if err := channelRefreshCacheByID(req.ID, ctx); err != nil {
		return nil, err
	}

	channel, _ := channelCache.Get(req.ID)
	HealthNotify()
	return &channel, nil
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

func ChannelDel(id int, ctx context.Context) error {
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

	// 删除缓存
	channelCache.Del(id)
	for _, k := range ch.Keys {
		if k.ID != 0 {
			channelKeyCache.Del(k.ID)
		}
	}
	StatsChannelDel(id)

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
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate = make(map[int]struct{})
	channelKeyCacheNeedUpdateLock.Unlock()
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
			channelKeyCacheNeedUpdateLock.Lock()
			_, stillDirty := channelKeyCacheNeedUpdate[k.ID]
			channelKeyCacheNeedUpdateLock.Unlock()
			if stillDirty {
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
