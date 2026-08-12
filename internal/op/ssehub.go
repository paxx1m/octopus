package op

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// sseHub 统一 SSE 订阅的 stream-token 管理与广播通知，
// 消除 health 与 relay log 两套此前几乎逐行重复的实现。
// T 为订阅通道的负载类型。
type sseHub[T any] struct {
	tokenTTL time.Duration
	buffer   int

	tokensLock sync.RWMutex
	tokens     map[string]struct{ expiresAt time.Time }

	subscribersLock sync.RWMutex
	subscribers     map[chan T]struct{}
}

func newSSEHub[T any](tokenTTL time.Duration, buffer int) *sseHub[T] {
	return &sseHub[T]{
		tokenTTL:    tokenTTL,
		buffer:      buffer,
		tokens:      make(map[string]struct{ expiresAt time.Time }),
		subscribers: make(map[chan T]struct{}),
	}
}

func (h *sseHub[T]) purgeExpiredTokens() {
	now := time.Now()
	for t, e := range h.tokens {
		if now.After(e.expiresAt) {
			delete(h.tokens, t)
		}
	}
}

// TokenCreate 生成一次性 stream-token（带 TTL）。
func (h *sseHub[T]) TokenCreate() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	h.tokensLock.Lock()
	h.purgeExpiredTokens()
	h.tokens[token] = struct{ expiresAt time.Time }{expiresAt: time.Now().Add(h.tokenTTL)}
	h.tokensLock.Unlock()

	return token, nil
}

func (h *sseHub[T]) TokenVerify(token string) bool {
	h.tokensLock.Lock()
	defer h.tokensLock.Unlock()
	e, ok := h.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(e.expiresAt) {
		delete(h.tokens, token)
		return false
	}
	return true
}

func (h *sseHub[T]) TokenRevoke(token string) {
	h.tokensLock.Lock()
	delete(h.tokens, token)
	h.tokensLock.Unlock()
}

// Subscribe 注册一个订阅通道（缓冲 buffer，通知自动合并）。
func (h *sseHub[T]) Subscribe() chan T {
	ch := make(chan T, h.buffer)
	h.subscribersLock.Lock()
	h.subscribers[ch] = struct{}{}
	h.subscribersLock.Unlock()
	return ch
}

func (h *sseHub[T]) Unsubscribe(ch chan T) {
	h.subscribersLock.Lock()
	delete(h.subscribers, ch)
	h.subscribersLock.Unlock()
	close(ch)
}

// Notify 唤醒所有订阅者（非阻塞，缓冲 1 自动合并）。
func (h *sseHub[T]) Notify(v T) {
	h.subscribersLock.RLock()
	defer h.subscribersLock.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- v:
		default:
		}
	}
}

// HasSubscribers 是否有活跃订阅者（用于跳过无订阅时的 goroutine 开销）。
func (h *sseHub[T]) HasSubscribers() bool {
	h.subscribersLock.RLock()
	defer h.subscribersLock.RUnlock()
	return len(h.subscribers) > 0
}
