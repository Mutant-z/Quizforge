package handler

import (
	"github.com/gin-gonic/gin"
)

// flushSSE 安全 Flush（连接已关闭时 panic，用 recover 兜底）。
func flushSSE(c *gin.Context) {
	defer func() {
		_ = recover()
	}()
	c.Writer.Flush()
}
