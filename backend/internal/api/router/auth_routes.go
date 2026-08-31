package router

import (
	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api/handler"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

func newAuthHandler(deps *Deps) *handler.AuthHandler {
	repo := sqlite.NewRepository(deps.DB)
	return handler.NewAuthHandler(repo, deps.Token, deps.Crypt)
}

func newUserHandler(deps *Deps) *handler.UserHandler {
	repo := sqlite.NewRepository(deps.DB)
	return handler.NewUserHandler(repo)
}

func registerAuthRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	h := newAuthHandler(deps)
	auth := g.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)
}

func registerUserRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	h := newUserHandler(deps)
	u := g.Group("/users")
	u.GET("/me", h.Me)
	u.PUT("/me", h.UpdateMe)
	u.PUT("/me/settings", h.UpdateSettings)
}
