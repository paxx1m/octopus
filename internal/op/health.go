package op

import "time"

const healthStreamTokenTTL = 5 * time.Minute

// healthHub 管理健康面板 SSE 的 stream-token 与订阅通知。
// 负载为 struct{}{}（唤醒信号，payload 未使用——收到后重建全量快照）。
var healthHub = newSSEHub[struct{}](healthStreamTokenTTL, 1)

func HealthStreamTokenCreate() (string, error) {
	return healthHub.TokenCreate()
}

func HealthStreamTokenVerify(token string) bool {
	return healthHub.TokenVerify(token)
}

func HealthStreamTokenRevoke(token string) {
	healthHub.TokenRevoke(token)
}

func HealthSubscribe() chan struct{} {
	return healthHub.Subscribe()
}

func HealthUnsubscribe(ch chan struct{}) {
	healthHub.Unsubscribe(ch)
}

// HealthNotify wakes all health SSE subscribers (coalesced per channel buffer size 1).
func HealthNotify() {
	healthHub.Notify(struct{}{})
}
