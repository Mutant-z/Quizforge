package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimiter 简单的内存滑动窗口限流（按 IP + 路径分组）。
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limitPerMin int) *rateLimiter {
	return &rateLimiter{
		requests: map[string][]time.Time{},
		limit:    limitPerMin,
		window:   time.Minute,
	}
}

func (r *rateLimiter) allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.requests[key]
	cutoff := now.Add(-r.window)
	kept := list[:0]
	for _, t := range list {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.requests[key] = kept
		return false
	}
	r.requests[key] = append(kept, now)
	return true
}

// RateLimit 按 IP+路径限流。
func RateLimit(limitPerMin int) gin.HandlerFunc {
	rl := newRateLimiter(limitPerMin)
	return func(c *gin.Context) {
		if !rl.allow(c.ClientIP() + "|" + c.FullPath()) {
			c.AbortWithStatusJSON(429, gin.H{
				"data": nil, "error": gin.H{"code": "RATE_LIMITED", "message": "请求过于频繁，请稍后再试"},
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.Next()
	}
}
