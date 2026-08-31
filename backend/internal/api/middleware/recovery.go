package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/observability"
)

// Recovery 统一处理 panic 与业务错误（返回统一错误格式）。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					"request_id", c.GetString("request_id"),
					"panic", rec,
				)
				api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "服务器内部错误")
				c.Abort()
			}
		}()
		c.Next()

		// 若 handler 中使用了 c.Error 传播业务错误，在此统一响应
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			if ae := api.AsAppError(err); ae != nil {
				rid := observability.RequestID(c.Request.Context())
				_ = rid
				logger.Warn("app_error",
					"request_id", c.GetString("request_id"),
					"code", ae.Code,
					"message", ae.Message,
					"detail", ae.Detail,
					"err", ae.Err,
				)
				if !c.Writer.Written() {
					api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
				}
				return
			}
			if !c.Writer.Written() {
				api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "服务器内部错误")
			}
		}
	}
}
