package relay

import (
	"context"
	"fmt"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
)

type channelSkipKey struct {
	channelID int
	modelName string
}

// channelSelector caches per-request channel/key selection work.
// It keeps resource loading separate from Iterator, which remains responsible
// for candidate order and attempt tracing.
type channelSelector struct {
	ctx context.Context

	channels    map[int]*dbmodel.Channel
	keys        map[int][]dbmodel.ChannelKey
	modelLists  map[int][]string
	unavailable map[channelSkipKey]string
}

func newChannelSelector(ctx context.Context) *channelSelector {
	return &channelSelector{
		ctx:         ctx,
		channels:    make(map[int]*dbmodel.Channel),
		keys:        make(map[int][]dbmodel.ChannelKey),
		modelLists:  make(map[int][]string),
		unavailable: make(map[channelSkipKey]string),
	}
}

func (s *channelSelector) skipIfUnavailable(iter *balancer.Iterator, item dbmodel.GroupItem) bool {
	if reason, ok := s.unavailable[channelSkipKey{channelID: item.ChannelID}]; ok {
		iter.Skip(item.ChannelID, 0, s.channelName(item.ChannelID), reason)
		return true
	}
	if reason, ok := s.unavailable[channelSkipKey{channelID: item.ChannelID, modelName: item.ModelName}]; ok {
		iter.Skip(item.ChannelID, 0, s.channelName(item.ChannelID), reason)
		return true
	}
	return false
}

func (s *channelSelector) markChannelUnavailable(channelID int, reason string) {
	s.unavailable[channelSkipKey{channelID: channelID}] = reason
}

func (s *channelSelector) markModelUnavailable(channelID int, modelName, reason string) {
	s.unavailable[channelSkipKey{channelID: channelID, modelName: modelName}] = reason
}

func (s *channelSelector) channelFor(channelID int) (*dbmodel.Channel, error) {
	if channel, ok := s.channels[channelID]; ok {
		return channel, nil
	}
	channel, err := op.ChannelGet(channelID, s.ctx)
	if err != nil {
		return nil, err
	}
	s.channels[channelID] = channel
	return channel, nil
}

func (s *channelSelector) channelName(channelID int) string {
	if channel, ok := s.channels[channelID]; ok && channel.Name != "" {
		return channel.Name
	}
	return fmt.Sprintf("channel_%d", channelID)
}

func (s *channelSelector) keysFor(channel *dbmodel.Channel) []dbmodel.ChannelKey {
	if keys, ok := s.keys[channel.ID]; ok {
		return keys
	}
	var keys []dbmodel.ChannelKey
	if channel.NoKey {
		// ID 0 represents this channel's unauthenticated path. It participates in
		// circuit breaking but is never persisted as a ChannelKey.
		keys = []dbmodel.ChannelKey{{ChannelID: channel.ID}}
	} else {
		keys = channel.GetChannelKeys()
	}
	s.keys[channel.ID] = keys
	return keys
}

func (s *channelSelector) availableKeys(channel *dbmodel.Channel, iter *balancer.Iterator) []dbmodel.ChannelKey {
	keys := s.keysFor(channel)
	if len(keys) == 0 {
		return nil
	}

	available := make([]dbmodel.ChannelKey, 0, len(keys))
	for _, key := range keys {
		if iter.SkipCircuitBreak(channel.ID, key.ID, channel.Name) {
			continue
		}
		available = append(available, key)
	}
	return available
}

func (s *channelSelector) channelModels(channel *dbmodel.Channel) []string {
	if models, ok := s.modelLists[channel.ID]; ok {
		return models
	}
	models := channelModelList(channel.Model)
	s.modelLists[channel.ID] = models
	return models
}

func (s *channelSelector) invalidateKeys(channelID int) {
	delete(s.keys, channelID)
	delete(s.channels, channelID)
}
