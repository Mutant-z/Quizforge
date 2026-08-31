package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
)

// New 构建路由。
func New(cfg *config.Config, logger *slog.Logger, deps *Deps) *gin.Engine {
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Logger(logger))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) {
		if err := deps.DB.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	apiGroup := r.Group("/api/v1")
	registerAuthRoutes(apiGroup, deps, cfg)
	registerUserRoutes(apiGroup, deps, cfg)
	registerBankRoutes(apiGroup, deps, cfg)
	registerQuestionRoutes(apiGroup, deps, cfg)
	registerPracticeRoutes(apiGroup, deps, cfg)
	registerWrongRoutes(apiGroup, deps, cfg)
	registerReviewRoutes(apiGroup, deps, cfg)
	registerUploadRoutes(apiGroup, deps, cfg)
	registerImportRoutes(apiGroup, deps, cfg)
	if cfg.Agent.ImportAgentV2 {
		registerImportAgentRoutes(apiGroup, deps, cfg)
	}
	registerWrongImportRoutes(apiGroup, deps, cfg)
	registerRAGRoutes(apiGroup, deps, cfg)
	registerAgentRoutes(apiGroup, deps, cfg)
	registerStatRoutes(apiGroup, deps, cfg)
	registerProviderRoutes(apiGroup, deps, cfg)
	registerAdminRoutes(apiGroup, deps, cfg)

	return r
}
