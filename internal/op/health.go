package op

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const healthStreamTokenTTL = 5 * time.Minute

type healthStreamTokenEntry struct {
	expiresAt time.Time
}

var healthStreamTokens = make(map[string]healthStreamTokenEntry)
var healthStreamTokensLock sync.RWMutex

// HealthNotifyChan is a wake signal; payload is unused (rebuild full snapshot on receive).
type HealthNotifyChan chan struct{}

var healthSubscribers = make(map[HealthNotifyChan]struct{})
var healthSubscribersLock sync.RWMutex

func purgeExpiredHealthStreamTokens() {
	now := time.Now()
	for t, e := range healthStreamTokens {
		if now.After(e.expiresAt) {
			delete(healthStreamTokens, t)
		}
	}
}

func HealthStreamTokenCreate() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	healthStreamTokensLock.Lock()
	purgeExpiredHealthStreamTokens()
	healthStreamTokens[token] = healthStreamTokenEntry{expiresAt: time.Now().Add(healthStreamTokenTTL)}
	healthStreamTokensLock.Unlock()

	return token, nil
}

func HealthStreamTokenVerify(token string) bool {
	healthStreamTokensLock.Lock()
	defer healthStreamTokensLock.Unlock()
	e, ok := healthStreamTokens[token]
	if !ok {
		return false
	}
	if time.Now().After(e.expiresAt) {
		delete(healthStreamTokens, token)
		return false
	}
	return true
}

func HealthStreamTokenRevoke(token string) {
	healthStreamTokensLock.Lock()
	delete(healthStreamTokens, token)
	healthStreamTokensLock.Unlock()
}

func HealthSubscribe() HealthNotifyChan {
	ch := make(HealthNotifyChan, 1)
	healthSubscribersLock.Lock()
	healthSubscribers[ch] = struct{}{}
	healthSubscribersLock.Unlock()
	return ch
}

func HealthUnsubscribe(ch HealthNotifyChan) {
	healthSubscribersLock.Lock()
	delete(healthSubscribers, ch)
	healthSubscribersLock.Unlock()
	close(ch)
}

// HealthNotify wakes all health SSE subscribers (coalesced per channel buffer size 1).
func HealthNotify() {
	healthSubscribersLock.RLock()
	defer healthSubscribersLock.RUnlock()
	for ch := range healthSubscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
