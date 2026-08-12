package client

import (
	"context"
	"net/http"
	"time"
)

// GetUrlDelay 对 url 发起 HEAD 探测，返回往返延迟（毫秒）。
func GetUrlDelay(httpClient *http.Client, url string, ctx context.Context) (int, error) {
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return int(time.Since(start).Milliseconds()), nil
}
