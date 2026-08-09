package health

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
)

// BuildOptions controls snapshot filtering.
type BuildOptions struct {
	GroupID      int  // 0 = all groups
	AbnormalOnly bool // only channels with non-ok status
}

func statusRank(s string) int {
	switch s {
	case model.HealthStatusDisabled:
		return 0
	case model.HealthStatusCircuitOpen:
		return 1
	case model.HealthStatusRateLimited:
		return 2
	case model.HealthStatusCircuitHalfOpen:
		return 3
	case model.HealthStatusDegraded:
		return 4
	default:
		return 5
	}
}

func worseStatus(a, b string) string {
	if statusRank(a) <= statusRank(b) {
		return a
	}
	return b
}

func maskKey(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func buildGroupIndex() map[int][]int {
	// channelID -> groupIDs
	out := make(map[int][]int)
	groups, err := op.GroupList(context.Background())
	if err != nil {
		return out
	}
	seen := make(map[int]map[int]struct{})
	for _, g := range groups {
		for _, item := range g.Items {
			if seen[item.ChannelID] == nil {
				seen[item.ChannelID] = make(map[int]struct{})
			}
			if _, ok := seen[item.ChannelID][g.ID]; ok {
				continue
			}
			seen[item.ChannelID][g.ID] = struct{}{}
			out[item.ChannelID] = append(out[item.ChannelID], g.ID)
		}
	}
	return out
}

func channelInGroup(groupIDs []int, groupID int) bool {
	if groupID == 0 {
		return true
	}
	for _, id := range groupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

// BuildSnapshot assembles the health panel tree from in-memory channel cache + circuit map.
func BuildSnapshot(opts BuildOptions) model.HealthSnapshot {
	now := time.Now()
	nowSec := now.Unix()

	channels, _ := op.ChannelList(context.Background())
	groupIndex := buildGroupIndex()
	// onlyActive: skip fully-closed idle entries; still includes Open/HalfOpen/degraded
	circuits := balancer.ListCircuits(true)

	// index circuits: channelID -> keyID -> []CircuitSnapshot
	type ck struct{ ch, key int }
	byCK := make(map[ck][]balancer.CircuitSnapshot)
	for _, c := range circuits {
		k := ck{c.ChannelID, c.KeyID}
		byCK[k] = append(byCK[k], c)
	}

	result := make([]model.HealthChannel, 0, len(channels))
	var summary model.HealthSummary
	matchedTotal := 0

	for _, ch := range channels {
		gids := groupIndex[ch.ID]
		if !channelInGroup(gids, opts.GroupID) {
			continue
		}
		matchedTotal++

		hc := model.HealthChannel{
			ID:            ch.ID,
			Name:          ch.Name,
			Enabled:       ch.Enabled,
			AllowEmptyKey: ch.AllowEmptyKey,
			GroupIDs:      gids,
			Keys:          make([]model.HealthKey, 0, len(ch.Keys)+1),
		}

		if !ch.Enabled {
			hc.Status = model.HealthStatusDisabled
		} else {
			hc.Status = model.HealthStatusOK
		}

		available := 0
		seenKey := make(map[int]struct{}, len(ch.Keys))
		for _, k := range ch.Keys {
			seenKey[k.ID] = struct{}{}
			hk := buildKeyHealth(&ch, k, byCK[ck{ch.ID, k.ID}], nowSec)
			hc.Keys = append(hc.Keys, hk)
			hc.TotalKeys++
			if keymanager.IsAvailable(&ch, k, nowSec) {
				available++
			}
			hc.Status = worseStatus(hc.Status, hk.Status)
		}

		// allow_empty_key / synthetic keyID=0 circuit entries (no real ChannelKey row)
		if emptyCircuits := byCK[ck{ch.ID, 0}]; len(emptyCircuits) > 0 {
			if _, ok := seenKey[0]; !ok {
				synthetic := model.ChannelKey{
					ID:        0,
					ChannelID: ch.ID,
					Enabled:   true,
				}
				hk := buildKeyHealth(&ch, synthetic, emptyCircuits, nowSec)
				hk.Masked = ""
				hc.Keys = append(hc.Keys, hk)
				hc.Status = worseStatus(hc.Status, hk.Status)
			}
		}

		hc.AvailableKeys = available

		// allow_empty_key with no keys: still a usable channel when enabled
		if ch.AllowEmptyKey && len(ch.Keys) == 0 && ch.Enabled {
			if hc.Status == model.HealthStatusOK {
				hc.AvailableKeys = 1
			}
			hc.TotalKeys = 0
		} else if ch.Enabled && len(ch.Keys) == 0 && !ch.AllowEmptyKey {
			hc.Status = worseStatus(hc.Status, model.HealthStatusDisabled)
		}

		sort.SliceStable(hc.Keys, func(i, j int) bool {
			ri, rj := statusRank(hc.Keys[i].Status), statusRank(hc.Keys[j].Status)
			if ri != rj {
				return ri < rj
			}
			return hc.Keys[i].ID < hc.Keys[j].ID
		})

		if opts.AbnormalOnly && hc.Status == model.HealthStatusOK {
			continue
		}

		result = append(result, hc)
		switch hc.Status {
		case model.HealthStatusDisabled:
			summary.Disabled++
			summary.ChannelsAbnormal++
		case model.HealthStatusCircuitOpen:
			summary.CircuitOpen++
			summary.ChannelsAbnormal++
		case model.HealthStatusRateLimited:
			summary.RateLimited++
			summary.ChannelsAbnormal++
		case model.HealthStatusCircuitHalfOpen:
			summary.HalfOpen++
			summary.ChannelsAbnormal++
		case model.HealthStatusDegraded:
			summary.Degraded++
			summary.ChannelsAbnormal++
		default:
			summary.OK++
		}
	}

	summary.ChannelsTotal = matchedTotal
	if opts.AbnormalOnly {
		summary.OK = matchedTotal - summary.ChannelsAbnormal
	}

	sort.Slice(result, func(i, j int) bool {
		ri, rj := statusRank(result[i].Status), statusRank(result[j].Status)
		if ri != rj {
			return ri < rj
		}
		return result[i].Name < result[j].Name
	})

	return model.HealthSnapshot{
		Summary:  summary,
		Channels: result,
		Ts:       nowSec,
	}
}

func buildKeyHealth(ch *model.Channel, k model.ChannelKey, circuits []balancer.CircuitSnapshot, nowSec int64) model.HealthKey {
	hk := model.HealthKey{
		ID:         k.ID,
		Masked:     maskKey(k.ChannelKey),
		Remark:     k.Remark,
		Enabled:    k.Enabled,
		StatusCode: k.StatusCode,
		Status:     model.HealthStatusOK,
		Models:     make([]model.HealthModel, 0, len(circuits)),
	}

	if !k.Enabled {
		hk.Status = model.HealthStatusDisabled
	}

	// 429 soft cooldown
	if k.StatusCode == http.StatusTooManyRequests && k.LastUseTimeStamp > 0 {
		cooldown := keymanager.RateLimitCooldownSec(ch, &k)
		until := k.LastUseTimeStamp + int64(cooldown)
		rem := int(until - nowSec)
		if rem > 0 {
			hk.RateLimit = &model.HealthRateLimit{
				CooldownSec:   cooldown,
				CooldownUntil: until,
				RemainingSec:  rem,
			}
			hk.Status = worseStatus(hk.Status, model.HealthStatusRateLimited)
		}
	}

	for _, c := range circuits {
		ms := model.HealthStatusOK
		circ := &model.HealthCircuit{
			State:               c.State,
			ConsecutiveFailures: c.ConsecutiveFailures,
			TripCount:           c.TripCount,
			CooldownUntil:       c.CooldownUntil,
			RemainingSec:        c.RemainingSec,
			ProbePending:        c.ProbePending,
		}
		switch c.State {
		case "open":
			ms = model.HealthStatusCircuitOpen
		case "half_open":
			ms = model.HealthStatusCircuitHalfOpen
		default:
			if c.ConsecutiveFailures > 0 {
				ms = model.HealthStatusDegraded
			}
		}
		hk.Status = worseStatus(hk.Status, ms)
		// Only surface non-ok models (or degraded) to keep panel focused
		if ms != model.HealthStatusOK {
			hk.Models = append(hk.Models, model.HealthModel{
				Name:    c.ModelName,
				Status:  ms,
				Circuit: circ,
			})
		}
	}

	sort.Slice(hk.Models, func(i, j int) bool {
		ri, rj := statusRank(hk.Models[i].Status), statusRank(hk.Models[j].Status)
		if ri != rj {
			return ri < rj
		}
		return hk.Models[i].Name < hk.Models[j].Name
	})

	return hk
}
