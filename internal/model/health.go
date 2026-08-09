package model

// Health status constants (worst-first for aggregation).
const (
	HealthStatusDisabled        = "disabled"
	HealthStatusCircuitOpen     = "circuit_open"
	HealthStatusRateLimited     = "rate_limited"
	HealthStatusCircuitHalfOpen = "circuit_half_open"
	HealthStatusDegraded        = "degraded"
	HealthStatusOK              = "ok"
)

// HealthRateLimit is 429 soft-cooldown info for a key.
type HealthRateLimit struct {
	CooldownSec   int   `json:"cooldown_sec"`
	CooldownUntil int64 `json:"cooldown_until"`
	RemainingSec  int   `json:"remaining_sec"`
}

// HealthCircuit is circuit-breaker info for a channel+key+model.
type HealthCircuit struct {
	State               string `json:"state"` // closed | open | half_open
	ConsecutiveFailures int64  `json:"consecutive_failures"`
	TripCount           int    `json:"trip_count"`
	CooldownUntil       int64  `json:"cooldown_until,omitempty"`
	RemainingSec        int    `json:"remaining_sec,omitempty"`
	ProbePending        bool   `json:"probe_pending,omitempty"`
}

// HealthModel is per-model circuit status under a key.
type HealthModel struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Circuit *HealthCircuit `json:"circuit,omitempty"`
}

// HealthKey is per-key health under a channel.
type HealthKey struct {
	ID         int              `json:"id"`
	Masked     string           `json:"masked"`
	Remark     string           `json:"remark,omitempty"`
	Enabled    bool             `json:"enabled"`
	Status     string           `json:"status"`
	StatusCode int              `json:"status_code"`
	RateLimit  *HealthRateLimit `json:"rate_limit,omitempty"`
	Models     []HealthModel    `json:"models,omitempty"`
}

// HealthChannel is top-level channel health row.
type HealthChannel struct {
	ID            int         `json:"id"`
	Name          string      `json:"name"`
	Enabled       bool        `json:"enabled"`
	AllowEmptyKey bool        `json:"allow_empty_key"`
	Status        string      `json:"status"`
	AvailableKeys int         `json:"available_keys"`
	TotalKeys     int         `json:"total_keys"`
	GroupIDs      []int       `json:"group_ids,omitempty"`
	Keys          []HealthKey `json:"keys,omitempty"`
}

// HealthSummary aggregates counts for the health panel header.
type HealthSummary struct {
	ChannelsTotal    int `json:"channels_total"`
	ChannelsAbnormal int `json:"channels_abnormal"`
	Disabled         int `json:"disabled"`
	CircuitOpen      int `json:"circuit_open"`
	RateLimited      int `json:"rate_limited"`
	HalfOpen         int `json:"half_open"`
	Degraded         int `json:"degraded"`
	OK               int `json:"ok"`
}

// HealthSnapshot is the full health panel payload.
type HealthSnapshot struct {
	Summary  HealthSummary   `json:"summary"`
	Channels []HealthChannel `json:"channels"`
	Ts       int64           `json:"ts"`
}
