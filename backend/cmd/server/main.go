package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/quiztrace/quiztrace/internal/api/router"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	logger := observability.NewLogger(cfg.Server.Env)
	slog.SetDefault(logger)

	// 数据目录
	if err := os.MkdirAll(cfg.Upload.Root, 0o755); err != nil {
		logger.Error("create data dir failed", "err", err)
		os.Exit(1)
	}
	for _, sub := range []string{"uploads", "originals", "processed", "temp"} {
		_ = os.MkdirAll(filepath.Join(cfg.Upload.Root, sub), 0o755)
	}
	dbPath := cfg.DB.Path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfg.Upload.Root, "..", dbPath)
		dbPath = filepath.Clean(dbPath)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logger.Error("create db dir failed", "err", err)
		os.Exit(1)
	}

	db, err := database.Open(dbPath, cfg.DB.BusyTimeoutMs)
	if err != nil {
		logger.Error("open db failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); err != nil {
		migrationsDir = filepath.Join("..", "migrations")
	}
	if err := database.Migrate(ctx, db, migrationsDir); err != nil {
		logger.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	tokenMgr := security.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
	crypt := security.NewAESCrypt(cfg.Auth.MasterKey)
	repo := sqlite.NewRepository(db)

	// Seed 演示数据（无数据时）
	if err := seedIfEmpty(ctx, repo, logger); err != nil {
		logger.Error("seed failed", "err", err)
	}
	stateRepair := strings.EqualFold(os.Getenv("QT_IMPORT_STATE_REPAIR"), "true") || os.Getenv("QT_IMPORT_STATE_REPAIR") == "1"
	if report, auditErr := repo.AuditImportState(ctx, stateRepair); auditErr != nil {
		logger.Warn("import state audit failed", "err", auditErr)
	} else {
		logger.Info("import state audit complete", "apply", stateRepair, "sessions", report.SessionsChecked, "stale_active_runs", report.StaleActiveRuns, "counter_mismatches", report.CounterMismatches, "stale_running_messages", report.StaleRunningMessages, "scanned_flags_rechecked", report.ScannedFlagsRechecked, "repairs_applied", report.RepairsApplied)
	}

	deps := &router.Deps{DB: db, Token: tokenMgr, Crypt: crypt}

	// 启动 Worker（后台任务）
	workerMgr := startWorkers(ctx, deps, cfg, logger)
	defer workerMgr.Stop()

	r := router.New(cfg, logger, deps)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("server started", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("server stopped")
}
