package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/observability"
)

// RequestID 为每个请求注入 request_id。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Request = c.Request.WithContext(observability.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// Logger 结构化访问日志（request_id / status / duration）。
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http_request",
			"request_id", c.GetString("request_id"),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

var counter uint64

func newRequestID() string {
	counter++
	t := time.Now().UnixNano()
	return "req_" + itoa(t) + "_" + itoa(int64(counter))
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
