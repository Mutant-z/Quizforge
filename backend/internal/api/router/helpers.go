package router

import (
	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
)

func newProviderRegistry(deps *Deps, cfg *config.Config) *provider.Registry {
	return provider.NewRegistry(deps.DB, deps.Crypt)
}
