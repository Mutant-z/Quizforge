package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// ImportHandler 导入任务接口（含 SSE 进度）。
type ImportHandler struct {
	repo   *sqlite.Repository
	vision *service.VisionDraftService
}

func NewImportHandler(repo *sqlite.Repository, cfg *config.Config) *ImportHandler {
	return &ImportHandler{repo: repo, vision: service.NewVisionDraftService(repo, cfg)}
}

func (h *ImportHandler) List(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	jobs, total, err := h.repo.ListImportJobs(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询导入任务失败")
		return
	}
	api.PageOK(c, jobs, total, page, pageSize)
}

func (h *ImportHandler) Get(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	job, err := h.repo.GetImportJob(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "导入任务不存在")
		return
	}
	if job.UserID != uid {
		api.Fail(c, http.StatusForbidden, api.ErrUnauthorized, "无权访问")
		return
	}
	doc, _ := h.repo.GetDocument(c.Request.Context(), job.DocumentID)
	logs, _ := h.repo.ListImportLogs(c.Request.Context(), id)
	units, _ := h.repo.ListAgentUnits(c.Request.Context(), id, "")
	preview, _ := h.repo.VisionPreview(c.Request.Context(), id)
	api.OK(c, gin.H{"job": job, "document": doc, "logs": logs, "units": units, "preview": preview})
}

func (h *ImportHandler) ownedJob(c *gin.Context) (*domain.ImportJob, bool) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	job, err := h.repo.GetImportJob(c.Request.Context(), id)
	if err != nil || job.UserID != uid {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "导入任务不存在")
		return nil, false
	}
	return job, true
}

func (h *ImportHandler) Draft(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	page, pageSize := parseInt(c.Query("page"), 1), parseInt(c.Query("page_size"), 50)
	// Match ListVisionDraft's cap so page_size in the response describes the
	// actual query. Otherwise a stale/invalid client value can make pagination
	// skip pages and expose only the first batch of a large PDF.
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	items, total, err := h.repo.ListVisionDraft(c.Request.Context(), job.ID, page, pageSize)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询草稿失败")
		return
	}
	api.PageOK(c, items, total, page, pageSize)
}

func (h *ImportHandler) UpdateCandidate(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	candidateID := parseID(c.Param("candidate_id"))
	var req struct {
		ExpectedVersion int                    `json:"expected_version"`
		Question        map[string]interface{} `json:"question"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 || req.Question == nil {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	updated, err := h.vision.UpdateCandidate(c.Request.Context(), job.ID, candidateID, req.ExpectedVersion, req.Question)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "更新草稿失败")
		return
	}
	if !updated {
		api.Fail(c, 409, api.ErrConflict, "草稿已被更新，请刷新后重试")
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *ImportHandler) Messages(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	items, err := h.repo.ListImportMessages(c.Request.Context(), job.ID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询任务对话失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}

func (h *ImportHandler) CreateMessage(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	var req struct {
		Content         string `json:"content"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 || len(req.Content) == 0 {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	action, err := h.vision.HandleMessage(c.Request.Context(), job, req.Content, req.ExpectedVersion)
	if err != nil {
		if err.Error() == "DRAFT_VERSION_CONFLICT" {
			api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
			return
		}
		api.FailDetail(c, 500, api.ErrInternal, "创建修复任务失败", err.Error())
		return
	}
	c.JSON(http.StatusAccepted, api.Response{Data: gin.H{"accepted": true, "action": action}})
}

func (h *ImportHandler) Retry(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	var req struct {
		UnitIDs []int64 `json:"unit_ids"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.UnitIDs) == 0 {
		api.Fail(c, 400, api.ErrInvalidRequest, "请选择要重试的 Agent 单元")
		return
	}
	count, err := h.repo.RetryAgentUnits(c.Request.Context(), job.ID, req.UnitIDs)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "重试失败")
		return
	}
	if _, err = h.repo.EnqueueJob(c.Request.Context(), "import_pdf", service.ImportPayload{ImportJobID: job.ID}, 10); err != nil {
		api.Fail(c, 500, api.ErrInternal, "重试任务入队失败")
		return
	}
	api.OK(c, gin.H{"retried_units": count})
}

// ReprocessVision creates a new auditable run for a historical import. The old
// job, candidates and agent records remain untouched.
func (h *ImportHandler) ReprocessVision(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	doc, err := h.repo.GetDocument(c.Request.Context(), job.DocumentID)
	if err != nil || doc.MimeType != "application/pdf" {
		api.Fail(c, 422, api.ErrValidationFailed, "只有 PDF 任务可以使用 vision-agent.v1.1 重新识别")
		return
	}
	created, err := h.repo.CreateImportJob(c.Request.Context(), job.UserID, job.DocumentID, job.BankID)
	if err != nil {
		api.FailDetail(c, 500, api.ErrInternal, "创建视觉重识别任务失败", err.Error())
		return
	}
	if _, err := h.repo.EnqueueJob(c.Request.Context(), "import_pdf", service.ImportPayload{ImportJobID: created.ID}, 10); err != nil {
		api.FailDetail(c, 500, api.ErrInternal, "视觉重识别任务入队失败", err.Error())
		return
	}
	api.Created(c, created)
}

func (h *ImportHandler) Preview(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	preview, err := h.vision.Preview(c.Request.Context(), job.ID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "预检失败")
		return
	}
	api.OK(c, preview)
}

func (h *ImportHandler) Commit(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	result, err := h.vision.Commit(c.Request.Context(), job.ID, req.ExpectedVersion)
	if err != nil {
		switch err.Error() {
		case "DRAFT_VERSION_CONFLICT":
			api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
		case "IMPORT_PREVIEW_BLOCKED", "IMPORT_VALIDATED_COUNT_MISMATCH":
			api.Fail(c, 422, api.ErrValidationFailed, "草稿仍有阻断问题，无法提交")
		default:
			api.FailDetail(c, 500, api.ErrInternal, "提交失败", err.Error())
		}
		return
	}
	api.OK(c, result)
}

func (h *ImportHandler) PageImage(c *gin.Context) {
	job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	page := parseInt(c.Param("page"), 0)
	purpose := c.DefaultQuery("purpose", "extraction")
	asset, err := h.repo.GetPageAsset(c.Request.Context(), job.DocumentID, purpose, page)
	if err != nil {
		api.Fail(c, 404, api.ErrNotFound, "页面图像不存在")
		return
	}
	if _, err := os.Stat(asset.FilePath); err != nil {
		api.Fail(c, 404, api.ErrNotFound, "页面图像不存在")
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.File(asset.FilePath)
}

// Events SSE 实时进度。
func (h *ImportHandler) Events(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	job, err := h.repo.GetImportJob(c.Request.Context(), id)
	if err != nil || job.UserID != uid {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "导入任务不存在")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 先推送当前状态
	h.pushEvent(c, "import_progress", map[string]interface{}{
		"import_job_id": job.ID, "status": job.Status, "stage": job.Stage,
		"progress": job.Progress, "candidate_count": job.CandidateCount,
		"new_count": job.NewCount, "duplicate_count": job.DuplicateCount,
		"conflict_count": job.ConflictCount, "failed_count": job.FailedCount,
		"review_count":       job.ReviewCount,
		"directory_complete": job.DirectoryComplete, "expected_count": job.ExpectedCount,
		"extracted_count": job.ExtractedCount, "complete_count": job.CompleteCount,
		"total_units": job.TotalUnits, "completed_units": job.CompletedUnits,
		"failed_units": job.FailedUnits, "blocking_issue_count": job.BlockingIssueCount,
		"draft_version": job.DraftVersion,
	})

	lastStatus := job.Status
	lastProgress := job.Progress
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			current, err := h.repo.GetImportJob(c.Request.Context(), id)
			if err != nil {
				return
			}
			if current.Status != lastStatus || current.Progress != lastProgress {
				h.pushEvent(c, "import_progress", map[string]interface{}{
					"import_job_id": current.ID, "status": current.Status, "stage": current.Stage,
					"progress": current.Progress, "candidate_count": current.CandidateCount,
					"new_count": current.NewCount, "duplicate_count": current.DuplicateCount,
					"conflict_count": current.ConflictCount, "failed_count": current.FailedCount,
					"review_count":       current.ReviewCount,
					"directory_complete": current.DirectoryComplete, "expected_count": current.ExpectedCount,
					"extracted_count": current.ExtractedCount, "complete_count": current.CompleteCount,
					"total_units": current.TotalUnits, "completed_units": current.CompletedUnits,
					"failed_units": current.FailedUnits, "blocking_issue_count": current.BlockingIssueCount,
					"draft_version": current.DraftVersion,
					"error_code":    current.ErrorCode, "error_message": current.ErrorMessage,
				})
				lastStatus = current.Status
				lastProgress = current.Progress
			}
			// 终止状态结束 SSE
			if current.Status == "completed" || current.Status == "failed" || current.Status == "cancelled" || current.Status == "draft_ready" || current.Status == "needs_attention" || current.Status == "needs_model_configuration" {
				h.pushEvent(c, "import_done", map[string]interface{}{"status": current.Status})
				return
			}
		}
	}
}

func (h *ImportHandler) pushEvent(c *gin.Context, event string, data interface{}) {
	payload := fmt.Sprintf("event: %s\ndata: {\"type\":\"%s\",\"timestamp\":\"%s\",\"payload\":%s}\n\n",
		event, event, time.Now().UTC().Format(time.RFC3339), mustJSON(data))
	_, _ = c.Writer.WriteString(payload)
	flushSSE(c)
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

var _ = context.Background
