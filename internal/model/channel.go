package model

import (
	"github.com/looplj/axonhub/llm"
)

type AutoGroupType int

const (
	AutoGroupTypeNone  AutoGroupType = 0 //不自动分组
	AutoGroupTypeFuzzy AutoGroupType = 1 //模糊匹配
	AutoGroupTypeExact AutoGroupType = 2 //准确匹配
	AutoGroupTypeRegex AutoGroupType = 3 //正则匹配
)

// KeyMode 渠道内 Key 选择策略
type KeyMode int

const (
	KeyModeRoundRobin KeyMode = 1 // 轮询
	KeyModeRandom     KeyMode = 2 // 随机
	KeyModeFailover   KeyMode = 3 // 故障转移（固定顺序）
	KeyModeWeighted   KeyMode = 4 // 加权
	KeyModeLeastCost  KeyMode = 5 // 最少费用（默认，兼容旧行为）
)

const ChannelTypeDoubao llm.APIFormat = "doubao"

type Channel struct {
	ID                   int            `json:"id" gorm:"primaryKey"`
	Name                 string         `json:"name" gorm:"unique;not null"`
	Type                 llm.APIFormat  `json:"type"`
	Enabled              bool           `json:"enabled" gorm:"default:true"`
	BaseUrls             []BaseUrl      `json:"base_urls" gorm:"serializer:json"`
	Keys                 []ChannelKey   `json:"keys" gorm:"foreignKey:ChannelID"`
	Model                string         `json:"model"`
	CustomModel          string         `json:"custom_model"`
	Proxy                bool           `json:"proxy" gorm:"default:false"`
	AutoSync             bool           `json:"auto_sync" gorm:"default:false"`
	AutoGroup            AutoGroupType  `json:"auto_group" gorm:"default:0"`
	CustomHeader         []CustomHeader `json:"custom_header" gorm:"serializer:json"`
	ParamOverride        *string        `json:"param_override"`
	ChannelProxy         *string        `json:"channel_proxy"`
	Stats                *StatsChannel  `json:"stats,omitempty" gorm:"foreignKey:ChannelID"`
	MatchRegex           *string        `json:"match_regex"`
	KeyMode              KeyMode        `json:"key_mode" gorm:"default:5"`                 // 渠道内 Key 策略，默认最少费用
	RateLimitCooldownSec *int           `json:"rate_limit_cooldown_sec"`                  // 渠道级 429 冷却秒，nil 用全局
	AllowEmptyKey        bool           `json:"allow_empty_key" gorm:"default:false"`      // 无需 API Key
}

type BaseUrl struct {
	URL   string `json:"url"`
	Delay int    `json:"delay"`
}

type CustomHeader struct {
	HeaderKey   string `json:"header_key"`
	HeaderValue string `json:"header_value"`
}

type ChannelKey struct {
	ID                   int     `json:"id" gorm:"primaryKey"`
	ChannelID            int     `json:"channel_id" gorm:"index"`
	Enabled              bool    `json:"enabled" gorm:"default:true"`
	ChannelKey           string  `json:"channel_key"`
	StatusCode           int     `json:"status_code"`
	LastUseTimeStamp     int64   `json:"last_use_time_stamp"`
	TotalCost            float64 `json:"total_cost"`
	Remark               string  `json:"remark"`
	Weight               int     `json:"weight" gorm:"default:1"`
	RateLimitCooldownSec *int    `json:"rate_limit_cooldown_sec"` // Key 级 429 冷却秒，nil 用渠道/全局
}

// ChannelUpdateRequest 渠道更新请求 - 仅包含变更的数据
type ChannelUpdateRequest struct {
	ID                   int             `json:"id" binding:"required"`
	Name                 *string         `json:"name,omitempty"`
	Type                 *llm.APIFormat  `json:"type,omitempty"`
	Enabled              *bool           `json:"enabled,omitempty"`
	BaseUrls             *[]BaseUrl      `json:"base_urls,omitempty"`
	Model                *string         `json:"model,omitempty"`
	CustomModel          *string         `json:"custom_model,omitempty"`
	Proxy                *bool           `json:"proxy,omitempty"`
	AutoSync             *bool           `json:"auto_sync,omitempty"`
	AutoGroup            *AutoGroupType  `json:"auto_group,omitempty"`
	CustomHeader         *[]CustomHeader `json:"custom_header,omitempty"`
	ChannelProxy         *string         `json:"channel_proxy,omitempty"`
	ParamOverride        *string         `json:"param_override,omitempty"`
	MatchRegex           *string         `json:"match_regex,omitempty"`
	KeyMode              *KeyMode        `json:"key_mode,omitempty"`
	RateLimitCooldownSec *int            `json:"rate_limit_cooldown_sec,omitempty"`
	AllowEmptyKey        *bool           `json:"allow_empty_key,omitempty"`

	KeysToAdd    []ChannelKeyAddRequest    `json:"keys_to_add,omitempty"`
	KeysToUpdate []ChannelKeyUpdateRequest `json:"keys_to_update,omitempty"`
	KeysToDelete []int                     `json:"keys_to_delete,omitempty"`
}

type ChannelKeyAddRequest struct {
	Enabled              bool   `json:"enabled"`
	ChannelKey           string `json:"channel_key"`
	Remark               string `json:"remark"`
	Weight               int    `json:"weight"`
	RateLimitCooldownSec *int   `json:"rate_limit_cooldown_sec,omitempty"`
}

type ChannelKeyUpdateRequest struct {
	ID                   int     `json:"id" binding:"required"`
	Enabled              *bool   `json:"enabled,omitempty"`
	ChannelKey           *string `json:"channel_key,omitempty"`
	Remark               *string `json:"remark,omitempty"`
	Weight               *int    `json:"weight,omitempty"`
	RateLimitCooldownSec *int    `json:"rate_limit_cooldown_sec,omitempty"`
}

func (c *Channel) GetBaseUrl() string {
	if c == nil || len(c.BaseUrls) == 0 {
		return ""
	}

	bestURL := ""
	bestDelay := 0
	bestSet := false

	for _, bu := range c.BaseUrls {
		if bu.URL == "" {
			continue
		}
		if !bestSet || bu.Delay < bestDelay {
			bestURL = bu.URL
			bestDelay = bu.Delay
			bestSet = true
		}
	}

	return bestURL
}

// EffectiveKeyMode returns KeyMode with default LeastCost for zero/unknown values.
func (c *Channel) EffectiveKeyMode() KeyMode {
	if c == nil {
		return KeyModeLeastCost
	}
	switch c.KeyMode {
	case KeyModeRoundRobin, KeyModeRandom, KeyModeFailover, KeyModeWeighted, KeyModeLeastCost:
		return c.KeyMode
	default:
		return KeyModeLeastCost
	}
}

// GetChannelKey returns one non-empty enabled key for admin/fetch helpers.
// Relay hot path uses keymanager instead; this does not apply rate-limit cooldown.
func (c *Channel) GetChannelKey() ChannelKey {
	if c == nil || len(c.Keys) == 0 {
		return ChannelKey{}
	}
	best := ChannelKey{}
	bestSet := false
	for _, k := range c.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		if !bestSet || k.TotalCost < best.TotalCost {
			best = k
			bestSet = true
		}
	}
	return best
}
