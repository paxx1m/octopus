package client

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
)

// ChannelHttpClient 按渠道代理配置返回 HTTP 客户端：
//   - 未开启代理：直连
//   - 开启代理但未指定 channel_proxy：使用系统/应用代理设置
//   - 指定 channel_proxy：使用自定义代理
func ChannelHttpClient(channel *model.Channel) (*http.Client, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if !channel.Proxy {
		return GetHTTPClientSystemProxy(false)
	} else if channel.ChannelProxy == nil || strings.TrimSpace(*channel.ChannelProxy) == "" {
		return GetHTTPClientSystemProxy(true)
	} else {
		return GetHTTPClientCustomProxy(strings.TrimSpace(*channel.ChannelProxy))
	}
}
