package op

import (
	"context"
	"fmt"
	"time"

	"github.com/bestruirui/octopus/internal/utils/log"
)

func InitCache() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return loadCache(ctx)
}

// ReloadCache 先落盘内存运行态再从 DB 重载。
// 导入后调用，避免未保存的 key 费用与统计被丢弃。
func ReloadCache() error {
	if err := SaveCache(); err != nil {
		log.Warnf("save cache before reload failed (continuing reload): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return loadCache(ctx)
}

func loadCache(ctx context.Context) error {
	if err := settingRefreshCache(ctx); err != nil {
		return fmt.Errorf("setting refresh cache error: %v", err)
	}
	if err := channelRefreshCache(ctx); err != nil {
		return fmt.Errorf("channel refresh cache error: %v", err)
	}
	if err := groupRefreshCache(ctx); err != nil {
		return fmt.Errorf("group refresh cache error: %v", err)
	}
	if err := apiKeyRefreshCache(ctx); err != nil {
		return fmt.Errorf("api key refresh cache error: %v", err)
	}
	if err := llmRefreshCache(ctx); err != nil {
		return fmt.Errorf("llm refresh cache error: %v", err)
	}
	if err := statsRefreshCache(ctx); err != nil {
		return fmt.Errorf("stats refresh cache error: %v", err)
	}
	return nil
}

func SaveCache() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := StatsSaveDB(ctx); err != nil {
		return err
	}
	if err := ChannelKeySaveDB(ctx); err != nil {
		return err
	}
	if err := RelayLogSaveDBTask(ctx); err != nil {
		return err
	}
	return nil
}
