package client

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm/httpclient"
)

// ApplyCustomHeaders 应用渠道自定义 header。
// 同名敏感头保持认证配置优先（Authorization / X-Api-Key 等由渠道 key 写入，
// 自定义 header 不得覆盖），relay 转发与模型列表拉取共用同一实现。
func ApplyCustomHeaders(headers http.Header, custom []model.CustomHeader) {
	for _, header := range custom {
		if header.HeaderKey == "" {
			continue
		}
		if headers.Get(header.HeaderKey) != "" && httpclient.IsSensitiveHeader(header.HeaderKey) {
			continue
		}
		headers.Set(header.HeaderKey, header.HeaderValue)
	}
}
