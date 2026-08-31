package main

import (
	"context"
	"log/slog"

	"github.com/quiztrace/quiztrace/internal/api/router"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// startWorkers 启动后台 Worker 管理器（导入 / 向量化）。
func startWorkers(ctx context.Context, deps *router.Deps, cfg *config.Config, logger *slog.Logger) *WorkerManager {
	return NewWorkerManager(ctx, deps, cfg, logger)
}

// WorkerManager 管理所有后台 worker。
type WorkerManager struct {
	importWorker      *ImportWorker
	wrongImportWorker *WrongImportWorker
	stopCh            chan struct{}
}

func NewWorkerManager(ctx context.Context, deps *router.Deps, cfg *config.Config, logger *slog.Logger) *WorkerManager {
	repo := sqlite.NewRepository(deps.DB)
	m := &WorkerManager{stopCh: make(chan struct{})}
	m.importWorker = NewImportWorker(repo, cfg, logger)
	m.importWorker.Start(ctx)
	m.wrongImportWorker = NewWrongImportWorker(repo, cfg, logger)
	m.wrongImportWorker.Start(ctx)
	return m
}

func (m *WorkerManager) Stop() {
	close(m.stopCh)
	m.importWorker.Stop()
	m.wrongImportWorker.Stop()
}
