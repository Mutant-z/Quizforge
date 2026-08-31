package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/parser"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// ImportService 导入流水线编排（上传 → 解析 → 提取 → 校验 → 入库 → 向量化）。
type ImportService struct {
	repo *sqlite.Repository
	cfg  *config.Config
}

func NewImportService(repo *sqlite.Repository, cfg *config.Config) *ImportService {
	return &ImportService{repo: repo, cfg: cfg}
}

// ImportPayload job payload。
type ImportPayload struct {
	ImportJobID int64 `json:"import_job_id"`
	// RunID binds a queued import to the session run that created it. It is
	// optional for legacy/non-session imports, but must be preserved for every
	// session run so an old queue item cannot mutate a newer active run.
	RunID *int64 `json:"run_id,omitempty"`
}

// CandidateReviewPayload is intentionally separate from ImportPayload. A
// candidate review must never fall back to the full-PDF import orchestrator.
type CandidateReviewPayload struct {
	SessionID    int64   `json:"session_id"`
	RunID        int64   `json:"run_id"`
	CandidateIDs []int64 `json:"candidate_ids"`
	TargetRole   string  `json:"target_role"` // auto | question | answer | both
}

var errImportSessionCancelled = errors.New("import session cancelled")
var errImportRunSuperseded = errors.New("import run superseded")

func (s *ImportService) ensureImportSessionActive(ctx context.Context, imp *domain.ImportJob) error {
	if imp.SessionID == nil {
		return nil
	}
	if s.repo.IsImportSessionCancelled(ctx, *imp.SessionID) {
		return errImportSessionCancelled
	}
	if imp.RunID != nil {
		session, err := s.repo.GetImportSession(ctx, *imp.SessionID)
		if err != nil {
			return err
		}
		if session.ActiveRunID == nil || *session.ActiveRunID != *imp.RunID {
			return errImportRunSuperseded
		}
	}
	return nil
}

// StartImport 上传完成后创建导入任务并入队。
func (s *ImportService) StartImport(ctx context.Context, importJobID int64) (int64, error) {
	jobID, err := s.repo.EnqueueJob(ctx, "import_pdf", ImportPayload{ImportJobID: importJobID}, 0)
	if err != nil {
		return 0, api.Internal("入队导入任务失败", err)
	}
	return jobID, nil
}

// RunImport 执行导入（幂等：已完成的 candidate 阶段跳过）。
func (s *ImportService) RunImport(ctx context.Context, job *domain.Job) error {
	var payload ImportPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("parse import payload: %w", err)
	}

	imp, err := s.repo.GetImportJob(ctx, payload.ImportJobID)
	if err != nil {
		return fmt.Errorf("import job not found: %w", err)
	}
	// Queue workers run outside the authenticated HTTP request, so the user
	// identity must be restored from the durable import job before any
	// user-scoped provider or other service is accessed.
	ctx = observability.WithUserID(ctx, imp.UserID)
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		if errors.Is(err, errImportSessionCancelled) || errors.Is(err, errImportRunSuperseded) {
			return nil
		}
		return err
	}
	if imp.SessionID != nil {
		if session, sessionErr := s.repo.GetImportSession(ctx, *imp.SessionID); sessionErr == nil {
			if payload.RunID != nil {
				// A queue item from a superseded run must not be allowed to
				// resume against the current session projection.
				if session.ActiveRunID == nil || *session.ActiveRunID != *payload.RunID {
					return nil
				}
				imp.RunID = payload.RunID
			} else {
				// Keep compatibility with queue rows created before run_id was
				// added to the payload.
				imp.RunID = session.ActiveRunID
			}
		}
	}
	doc, err := s.repo.GetDocument(ctx, imp.DocumentID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}
	if domain.IsVisionPipeline(imp.PipelineVersion) && doc.MimeType == "application/pdf" {
		if err := s.runVisionImport(ctx, imp, doc); err != nil {
			if errors.Is(err, errImportSessionCancelled) || errors.Is(err, errImportRunSuperseded) {
				return nil
			}
			// The worker may already be shutting down, so cleanup must not use its
			// cancelled request context. Persist running-unit failures and the
			// session error with a short independent context for safe recovery.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = s.repo.FailRunningAgentUnits(cleanupCtx, imp.ID, "WORKER_INTERRUPTED", err.Error())
			s.failVisionImport(cleanupCtx, imp, doc.ID, "VISION_PIPELINE_FAILED", err.Error())
			return err
		}
		return nil
	}

	// 1. 解析文档（分页提取文本 / 检测扫描页）
	s.updateStage(ctx, imp.ID, domain.ImportStatusExtracting, "正在解析文档", 0.05)
	pages, err := parser.ParseDocument(ctx, doc.FilePath, doc.MimeType)
	if err != nil {
		s.failImport(ctx, imp.ID, domain.ImportStatusFailed, "PDF_EXTRACT_FAILED", err.Error())
		return err
	}
	// 保存分页
	if err := s.repo.SaveDocumentPages(ctx, doc.ID, pages); err != nil {
		s.failImport(ctx, imp.ID, domain.ImportStatusFailed, "PIPELINE_FAILED", err.Error())
		return err
	}
	_ = s.repo.UpdateDocumentPages(ctx, doc.ID, len(pages))

	// 2. 清洗 + 题目边界检测 → Candidate
	s.updateStage(ctx, imp.ID, domain.ImportStatusCleaning, "正在清洗文本", 0.15)
	candidates, err := parser.BuildCandidates(ctx, pages)
	if err != nil {
		s.failImport(ctx, imp.ID, domain.ImportStatusFailed, "BOUNDARY_UNCERTAIN", err.Error())
		return err
	}
	// 写入 candidate（幂等：按 input_hash 跳过已存在）
	newCount := 0
	for _, cand := range candidates {
		cand.DocumentID = doc.ID
		jobID := imp.ID
		cand.ImportJobID = &jobID
		created, err := s.repo.InsertCandidateIfAbsent(ctx, cand)
		if err != nil {
			continue
		}
		if created {
			newCount++
		}
	}
	_ = s.repo.UpdateImportCounts(ctx, imp.ID, len(pages), len(candidates), newCount)

	// 3. 多 Agent 提取（candidate → 题目）
	s.updateStage(ctx, imp.ID, domain.ImportStatusParsing, "正在提取题目", 0.3)
	if err := s.runExtraction(ctx, imp, doc); err != nil {
		s.failImport(ctx, imp.ID, domain.ImportStatusFailed, "PIPELINE_FAILED", err.Error())
		return err
	}

	// 4. 校验 + 入库
	s.updateStage(ctx, imp.ID, domain.ImportStatusValidating, "正在校验题目", 0.7)
	if err := s.runValidationAndPersist(ctx, imp, doc); err != nil {
		s.failImport(ctx, imp.ID, domain.ImportStatusFailed, "PIPELINE_FAILED", err.Error())
		return err
	}

	// 5. 向量化入队
	s.updateStage(ctx, imp.ID, domain.ImportStatusEmbedding, "正在生成向量索引", 0.9)
	if _, err := s.repo.EnqueueJob(ctx, "embedding", ImportPayload{ImportJobID: imp.ID}, 0); err == nil {
	}

	_ = s.repo.UpdateImportStatus(ctx, imp.ID, domain.ImportStatusCompleted, "completed", 1.0)
	_ = s.repo.AddImportLog(ctx, imp.ID, "completed", "任务完成")
	return nil
}

// runExtraction 对每个 candidate 执行提取（LLM），按配置并发。
func (s *ImportService) runExtraction(ctx context.Context, imp *domain.ImportJob, doc *domain.Document) error {
	candidates, err := s.repo.ListCandidatesByJob(ctx, imp.ID, "pending")
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	extractor := s.extractor()
	// 顺序处理（保证简单可靠；并发由 worker 配置控制）
	for i, cand := range candidates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		progress := 0.3 + 0.4*float64(i)/float64(len(candidates))
		_ = s.repo.UpdateImportProgress(ctx, imp.ID, progress)
		if err := extractor.Extract(ctx, cand); err != nil {
			_ = s.repo.SetCandidateError(ctx, cand.ID, "MODEL_ERROR", err.Error())
		}
	}
	return nil
}

// runValidationAndPersist 校验 + 去重 + 入库。
func (s *ImportService) runValidationAndPersist(ctx context.Context, imp *domain.ImportJob, doc *domain.Document) error {
	candidates, err := s.repo.ListCandidatesByJob(ctx, imp.ID, "extracted")
	if err != nil {
		return err
	}
	persister := s.persister()
	bankID := imp.BankID
	subjectID := subjectIDFromBank(ctx, s.repo, bankID)
	for _, cand := range candidates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := persister.Persist(ctx, cand, doc, bankID, subjectID); err != nil {
			_ = s.repo.SetCandidateError(ctx, cand.ID, "PIPELINE_FAILED", err.Error())
		}
	}
	// 更新导入结果统计
	stats, _ := s.repo.GetImportStats(ctx, imp.ID)
	_ = s.repo.UpdateImportCountsFull(ctx, imp.ID, stats.NewCount, stats.DuplicateCount, stats.ConflictCount, stats.ReviewCount, stats.FailedCount)
	return nil
}

// RunEmbedding 对未向量化文档生成 embedding（幂等）。
func (s *ImportService) RunEmbedding(ctx context.Context, job *domain.Job) error {
	var payload ImportPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return err
	}
	imp, err := s.repo.GetImportJob(ctx, payload.ImportJobID)
	if err != nil {
		return err
	}
	ctx = observability.WithUserID(ctx, imp.UserID)
	if imp.SessionID != nil && s.repo.IsImportSessionCancelled(ctx, *imp.SessionID) {
		return nil
	}
	if imp.BankID != nil {
		_ = s.embedder().EmbedPending(ctx, *imp.BankID)
	}
	if domain.IsVisionPipeline(imp.PipelineVersion) {
		s.updateStage(ctx, imp.ID, domain.ImportStatusCompleted, "视觉草稿已完成事务入库与检索索引", 1)
	}
	if imp.SessionID != nil {
		_, _ = s.repo.DB().ExecContext(ctx, `UPDATE import_sessions SET status='completed',updated_at=datetime('now') WHERE id=? AND status='embedding'`, *imp.SessionID)
		content := "统一草稿已完成事务入库与检索索引。"
		messageID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: *imp.SessionID, Role: "assistant", MessageType: "tool_result", Content: content, Status: "completed"})
		_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: *imp.SessionID, MessageID: &messageID, Type: "run_completed", Stage: "completed", AgentRole: "EmbeddingAgent", Summary: content})
	}
	return nil
}

// RunReindexQuestion 单题重建向量（管理端触发）。
func (s *ImportService) RunReindexQuestion(ctx context.Context, job *domain.Job) error {
	var payload struct {
		QuestionID int64 `json:"question_id"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return err
	}
	q, err := s.repo.GetQuestion(ctx, payload.QuestionID)
	if err != nil {
		return err
	}
	if err := s.repo.BuildQuestionFTS(ctx, q); err != nil {
		return err
	}
	return s.embedder().EmbedOne(ctx, q)
}

// ---- 内部辅助 ----

func (s *ImportService) extractor() *Extractor {
	return NewExtractor(s.repo, s.cfg)
}

func (s *ImportService) persister() *Persister {
	return NewPersister(s.repo, s.cfg)
}

func (s *ImportService) embedder() *Embedder {
	return NewEmbedder(s.repo, s.cfg)
}

func (s *ImportService) updateStage(ctx context.Context, jobID int64, stage, message string, progress float64) {
	_ = s.repo.UpdateImportStage(ctx, jobID, stage, message, progress)
}

func (s *ImportService) failImport(ctx context.Context, jobID int64, status, code, msg string) {
	_ = s.repo.UpdateImportError(ctx, jobID, status, code, msg)
}

// subjectIDFromBank 取题库第一个科目作为默认分类。
func subjectIDFromBank(ctx context.Context, repo *sqlite.Repository, bankID *int64) *int64 {
	if bankID == nil {
		return nil
	}
	subjects, err := repo.ListSubjects(ctx, *bankID)
	if err != nil || len(subjects) == 0 {
		return nil
	}
	return &subjects[0].ID
}

var _ = sql.ErrNoRows
