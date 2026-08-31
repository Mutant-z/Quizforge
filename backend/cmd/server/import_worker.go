package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// sleep 等待 duration 或 ctx 取消，返回 false 表示被取消。
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// ImportWorker 从 SQLite Job Queue 消费导入任务（docs/05）。
type ImportWorker struct {
	repo      *sqlite.Repository
	cfg       *config.Config
	logger    *slog.Logger
	workerID  string
	importSvc *service.ImportService
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewImportWorker(repo *sqlite.Repository, cfg *config.Config, logger *slog.Logger) *ImportWorker {
	return &ImportWorker{
		repo:      repo,
		cfg:       cfg,
		logger:    logger,
		workerID:  "worker_" + uuid.NewString()[:8],
		importSvc: service.NewImportService(repo, cfg),
		done:      make(chan struct{}),
	}
}

func (w *ImportWorker) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go func() {
		defer close(w.done)
		w.logger.Info("import worker started", "worker_id", w.workerID)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			job, err := w.claimNext(ctx)
			if err != nil {
				w.logger.Error("claim job failed", "err", err)
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

func (w *ImportWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	<-w.done
}

// claimNext 抢占下一个 queued 且 lease 过期的 job。
func (w *ImportWorker) claimNext(ctx context.Context) (*domain.Job, error) {
	job, err := w.repo.ClaimJob(ctx, w.workerID, w.cfg.Worker.LeaseSeconds)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (w *ImportWorker) process(ctx context.Context, job *domain.Job) {
	w.logger.Info("process job", "job_id", job.ID, "type", job.Type)
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
	var err error
	switch job.Type {
	case "import_pdf":
		err = w.importSvc.RunImport(jobCtx, job)
	case "import_candidate_review":
		err = w.importSvc.RunCandidateReview(jobCtx, job)
	case "embedding":
		err = w.importSvc.RunEmbedding(jobCtx, job)
	case "reindex_question":
		err = w.importSvc.RunReindexQuestion(jobCtx, job)
	default:
		err = errors.New("unknown job type: " + job.Type)
	}
	cancel()
	<-leaseDone
	// Graceful shutdown cancels the worker context before this point. Use a
	// bounded independent context for the final job transition so a claimed job
	// cannot remain "running" until its lease expires.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if err != nil {
		w.logger.Error("job failed", "job_id", job.ID, "err", err)
		_ = w.repo.CompleteJob(cleanupCtx, job.ID, false, err.Error())
		return
	}
	_ = w.repo.CompleteJob(cleanupCtx, job.ID, true, "")
}
