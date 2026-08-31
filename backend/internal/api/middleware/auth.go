package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/security"
)

// Auth 从 Authorization: Bearer <token> 解析用户。
func Auth(tm *security.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			api.Fail(c, http.StatusUnauthorized, api.ErrUnauthorized, "未登录")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := tm.Parse(tokenStr)
		if err != nil || claims.TokenType != "access" {
			api.Fail(c, http.StatusUnauthorized, api.ErrUnauthorized, "登录已过期，请重新登录")
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Request = c.Request.WithContext(observability.WithUserID(c.Request.Context(), claims.UserID))
		c.Next()
	}
}

// RequireRole 后台接口权限（admin）。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)
		for _, r := range roles {
			if roleStr == r {
				c.Next()
				return
			}
		}
		api.Fail(c, http.StatusForbidden, api.ErrUnauthorized, "没有权限")
		c.Abort()
	}
}

// CurrentUserID 从 context 取 user_id。
func CurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get("user_id"); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}
