package task

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/modelfetch"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/utils/diff"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
)

// lastSyncModelsTime 以 UnixNano 存储，避免定时任务与 handler 触发并发读写竞争。
var lastSyncModelsTime atomic.Int64

// syncModelsRunning 防止定时任务与手动触发并发执行同一同步。
var syncModelsRunning atomic.Bool

// SyncModelsTask 同步模型任务
func SyncModelsTask() {
	if !syncModelsRunning.CompareAndSwap(false, true) {
		log.Debugf("sync models task skipped: previous run still in progress")
		return
	}
	defer syncModelsRunning.Store(false)
	runSyncModels()
}

// TriggerSyncModels 异步触发一次模型同步（handler 使用，避免请求内阻塞最长 30 分钟）。
// 返回 false 表示已有同步在进行中。
func TriggerSyncModels() bool {
	if !syncModelsRunning.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer syncModelsRunning.Store(false)
		runSyncModels()
	}()
	return true
}

func runSyncModels() {
	log.Debugf("sync models task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("sync models task finished, sync time: %s", time.Since(startTime))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	channels, err := op.ChannelList(ctx)
	if err != nil {
		log.Errorf("failed to list channels: %v", err)
		return
	}
	totalNewModels := make([]string, 0, 128)
	seenTotalNewModels := make(map[string]struct{}, 128)
	for _, channel := range channels {
		if !channel.AutoSync {
			continue
		}
		fetchModels, err := modelfetch.FetchModels(ctx, channel)
		if err != nil {
			log.Warnf("failed to fetch models for channel %s: %v", channel.Name, err)
			continue
		}
		oldModels := xstrings.SplitTrimCompact(",", channel.Model)
		newModels := xstrings.TrimCompact(fetchModels)
		for _, m := range newModels {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			m = strings.ToLower(m)
			if _, ok := seenTotalNewModels[m]; ok {
				continue
			}
			seenTotalNewModels[m] = struct{}{}
			totalNewModels = append(totalNewModels, m)
		}
		deletedModels, addedModels := diff.Diff(oldModels, newModels)
		if len(deletedModels) > 0 || len(addedModels) > 0 {
			fetchModelStr := strings.Join(newModels, ",")
			if _, err := op.ChannelUpdate(&model.ChannelUpdateRequest{
				ID:    channel.ID,
				Model: &fetchModelStr,
			}, ctx); err != nil {
				log.Errorf("failed to update channel %s: %v", channel.Name, err)
				continue
			}
		}
		// 批量删除消失的模型对应的 GroupItem
		if len(deletedModels) > 0 {
			log.Infof("deleted channel %s models: %v", channel.Name, deletedModels)
			keys := make([]model.GroupIDAndLLMName, len(deletedModels))
			for i, m := range deletedModels {
				keys[i] = model.GroupIDAndLLMName{ChannelID: channel.ID, ModelName: m}
			}
			if err := op.GroupItemBatchDelByChannelAndModels(keys, ctx); err != nil {
				log.Errorf("failed to batch delete group items for channel %s: %v", channel.Name, err)
			}
		}

		// 自动分组
		if len(newModels) > 0 {
			op.ChannelAutoGroup(&channel, ctx)
		}
	}
	llmPrice, err := op.LLMList(ctx)
	if err != nil {
		log.Errorf("failed to list models price: %v", err)
		return
	}
	llmPriceNames := make([]string, 0, len(llmPrice))
	for _, price := range llmPrice {
		llmPriceNames = append(llmPriceNames, price.Name)
	}

	deletedNorm, addedNorm := diff.Diff(llmPriceNames, totalNewModels)
	if len(deletedNorm) > 0 {
		if err := price.DeleteModelsWithNoPrice(deletedNorm, ctx); err != nil {
			log.Errorf("failed to batch delete models price: %v", err)
		}
	}
	if len(addedNorm) > 0 {
		if err := price.AddModelsToDB(addedNorm, ctx); err != nil {
			log.Errorf("failed to add models price: %v", err)
		}
	}
	lastSyncModelsTime.Store(time.Now().UnixNano())
}

func GetLastSyncModelsTime() time.Time {
	return time.Unix(0, lastSyncModelsTime.Load())
}
