package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/gin-gonic/gin"
)

// LoginRateLimit 对登录接口按客户端 IP 限速。
// maxAttempts 次失败窗口 window 内触发；成功登录后清零。
func LoginRateLimit(maxAttempts int, window time.Duration) gin.HandlerFunc {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	type bucket struct {
		count    int
		windowAt time.Time
	}
	var (
		mu        sync.Mutex
		byIP      = make(map[string]*bucket)
		lastPurge time.Time
	)

	purgeExpired := func(now time.Time) {
		// 避免每次请求全表扫描；至少间隔 1 分钟
		if now.Sub(lastPurge) < time.Minute && len(byIP) < 1024 {
			return
		}
		lastPurge = now
		for ip, b := range byIP {
			if now.Sub(b.windowAt) > window {
				delete(byIP, ip)
			}
		}
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		mu.Lock()
		purgeExpired(now)
		b, ok := byIP[ip]
		if !ok || now.Sub(b.windowAt) > window {
			b = &bucket{count: 0, windowAt: now}
			byIP[ip] = b
		}
		if b.count >= maxAttempts {
			mu.Unlock()
			resp.Error(c, http.StatusTooManyRequests, "too many login attempts, try again later")
			c.Abort()
			return
		}
		mu.Unlock()

		c.Next()

		// 仅在登录失败时累加
		if c.Writer.Status() == http.StatusUnauthorized || c.Writer.Status() == http.StatusBadRequest {
			mu.Lock()
			if b2, ok := byIP[ip]; ok {
				if time.Since(b2.windowAt) > window {
					b2.count = 1
					b2.windowAt = time.Now()
				} else {
					b2.count++
				}
			} else {
				byIP[ip] = &bucket{count: 1, windowAt: time.Now()}
			}
			mu.Unlock()
			return
		}
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			mu.Lock()
			delete(byIP, ip)
			mu.Unlock()
		}
	}
}

// MaxBodyBytes 限制请求体大小，防止超大 body 导致 OOM。
func MaxBodyBytes(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && max > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}
