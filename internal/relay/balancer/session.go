package balancer

import (
	"fmt"
	"sync"
	"time"
)

// SessionEntry 会话保持条目
type SessionEntry struct {
	ChannelID    int
	ChannelKeyID int
	Timestamp    time.Time
}

// 全局会话存储
var globalSession sync.Map // key: string -> value: *SessionEntry

// sessionKey 生成会话键：apiKeyID:requestModel
func sessionKey(apiKeyID int, requestModel string) string {
	return fmt.Sprintf("%d:%s", apiKeyID, requestModel)
}

// GetSticky 获取粘性通道（ttl 内有效）
// ttl 由 Group.SessionKeepTime 决定，返回 nil 表示无有效会话
func GetSticky(apiKeyID int, requestModel string, ttl time.Duration) *SessionEntry {
	key := sessionKey(apiKeyID, requestModel)
	v, ok := globalSession.Load(key)
	if !ok {
		return nil
	}
	entry := v.(*SessionEntry)

	if time.Since(entry.Timestamp) > ttl {
		// 过期，惰性清除
		globalSession.Delete(key)
		return nil
	}

	return entry
}

// SetSticky 写入/更新粘性记录
func SetSticky(apiKeyID int, requestModel string, channelID, keyID int) {
	key := sessionKey(apiKeyID, requestModel)
	globalSession.Store(key, &SessionEntry{
		ChannelID:    channelID,
		ChannelKeyID: keyID,
		Timestamp:    time.Now(),
	})
}

// ClearSticky 清除指定会话的粘性记录
func ClearSticky(apiKeyID int, requestModel string) {
	globalSession.Delete(sessionKey(apiKeyID, requestModel))
}

// ClearStickyByKey 清除所有引用指定 channel key 的粘性记录
func ClearStickyByKey(channelKeyID int) {
	globalSession.Range(func(k, v any) bool {
		entry := v.(*SessionEntry)
		if entry.ChannelKeyID == channelKeyID {
			globalSession.Delete(k)
		}
		return true
	})
}

// ClearStickyByChannel 清除所有引用指定 channel 的粘性记录
func ClearStickyByChannel(channelID int) {
	globalSession.Range(func(k, v any) bool {
		entry := v.(*SessionEntry)
		if entry.ChannelID == channelID {
			globalSession.Delete(k)
		}
		return true
	})
}
