package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// WrongImportWorker 错题导入专用 worker：消费图片提取和跨图整理任务，
// 与 PDF 导入 Pipeline 的 ImportWorker 互不抢占。
type WrongImportWorker struct {
	repo      *sqlite.Repository
	cfg       *config.Config
	logger    *slog.Logger
	workerID  string
	importSvc *service.WrongImportService
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewWrongImportWorker(repo *sqlite.Repository, cfg *config.Config, logger *slog.Logger) *WrongImportWorker {
	return &WrongImportWorker{
		repo:      repo,
		cfg:       cfg,
		logger:    logger,
		workerID:  "wrong_import_worker_" + uuid.NewString()[:8],
		importSvc: service.NewWrongImportService(repo, cfg),
		done:      make(chan struct{}),
	}
}

func (w *WrongImportWorker) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go func() {
		defer close(w.done)
		w.logger.Info("wrong import worker started", "worker_id", w.workerID)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			job, err := w.repo.ClaimJobOfTypes(ctx, w.workerID, w.cfg.Worker.LeaseSeconds, []string{"wrong_import_extract", "wrong_import_reconcile"})
			if err != nil {
				w.logger.Error("claim wrong import job failed", "err", err)
				if !sleep(ctx, 2*time.Second) {
					return
				}
				continue
			}
			if job == nil {
				if !sleep(ctx, time.Duration(w.cfg.Worker.PollIntervalMs)*time.Millisecond) {
					return
				}
				continue
			}
			w.process(ctx, job)
		}
	}()
}

func (w *WrongImportWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	<-w.done
}

func (w *WrongImportWorker) process(ctx context.Context, job *domain.Job) {
	w.logger.Info("process wrong import job", "job_id", job.ID, "type", job.Type)
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		interval := time.Duration(w.cfg.Worker.LeaseSeconds/3) * time.Second
		if interval < time.Second {
			interval = time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if err := w.repo.RenewJobLease(jobCtx, job.ID, w.workerID, w.cfg.Worker.LeaseSeconds); err != nil {
					w.logger.Error("renew job lease failed", "job_id", job.ID, "err", err)
					cancel()
					return
				}
			}
		}
	}()
	err := w.runExtract(jobCtx, job)
	cancel()
	<-leaseDone
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if err != nil {
		w.logger.Error("wrong import job failed", "job_id", job.ID, "err", err)
		_ = w.repo.CompleteJob(cleanupCtx, job.ID, false, err.Error())
		return
	}
	_ = w.repo.CompleteJob(cleanupCtx, job.ID, true, "")
}

// runExtract 解析 job payload 并执行错题图片提取或跨图整理。
func (w *WrongImportWorker) runExtract(ctx context.Context, job *domain.Job) error {
	var payload struct {
		ImageID   int64  `json:"image_id"`
		SessionID int64  `json:"session_id"`
		BatchID   string `json:"batch_id"`
		Scope     string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("decode wrong import payload: %w", err)
	}
	switch job.Type {
	case "wrong_import_extract":
		if payload.ImageID <= 0 {
			return errors.New("wrong import payload missing image_id")
		}
		return w.importSvc.ExtractImage(ctx, payload.ImageID)
	case "wrong_import_reconcile":
		if payload.SessionID <= 0 {
			return errors.New("wrong import reconcile payload missing session_id")
		}
		if payload.Scope == "batch" {
			if payload.BatchID == "" {
				return errors.New("wrong import reconcile payload missing batch_id")
			}
			return w.importSvc.ReconcileBatch(ctx, payload.SessionID, payload.BatchID)
		}
		if payload.Scope == "legacy_session" {
			return w.importSvc.ReconcileLegacySession(ctx, payload.SessionID)
		}
		return fmt.Errorf("unknown wrong import reconcile scope: %s", payload.Scope)
	default:
		return fmt.Errorf("unsupported wrong import job type: %s", job.Type)
	}
}
