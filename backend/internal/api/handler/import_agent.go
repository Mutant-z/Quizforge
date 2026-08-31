package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

type ImportAgentHandler struct {
	repo         *sqlite.Repository
	conversation *service.ImportConversationService
}

func NewImportAgentHandler(repo *sqlite.Repository, cfg *config.Config) *ImportAgentHandler {
	return &ImportAgentHandler{repo: repo, conversation: service.NewImportConversationService(repo, cfg)}
}
func (h *ImportAgentHandler) owned(c *gin.Context) (*domain.ImportSession, bool) {
	session, err := h.repo.GetImportSession(c.Request.Context(), parseID(c.Param("id")))
	if err != nil || session.UserID != middleware.CurrentUserID(c) {
		api.Fail(c, 404, api.ErrNotFound, "导入会话不存在")
		return nil, false
	}
	return session, true
}

func (h *ImportAgentHandler) Create(c *gin.Context) {
	var req struct {
		Title  string `json:"title"`
		BankID *int64 `json:"bank_id"`
	}
	_ = c.ShouldBindJSON(&req)
	userID := middleware.CurrentUserID(c)
	if req.BankID == nil {
		if user, getErr := h.repo.GetUserByID(c.Request.Context(), userID); getErr == nil && user.DefaultBankID > 0 {
			req.BankID = &user.DefaultBankID
		} else if banks, _, listErr := h.repo.ListBanks(c.Request.Context(), 1, 1); listErr == nil && len(banks) > 0 {
			id := banks[0].ID
			req.BankID = &id
		}
	}
	session, err := h.repo.CreateImportSession(c.Request.Context(), userID, req.BankID, req.Title)
	if err != nil {
		api.FailDetail(c, 500, api.ErrInternal, "创建导入会话失败", err.Error())
		return
	}
	api.Created(c, session)
}
func (h *ImportAgentHandler) List(c *gin.Context) {
	items, err := h.repo.ListImportSessions(c.Request.Context(), middleware.CurrentUserID(c), c.Query("scope") == "all")
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询会话失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}
func (h *ImportAgentHandler) Get(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	summary, err := h.repo.SessionSummary(c.Request.Context(), session.ID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询会话失败")
		return
	}
	api.OK(c, summary)
}
func (h *ImportAgentHandler) Patch(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Title) == "" {
		api.Fail(c, 400, api.ErrInvalidRequest, "标题不能为空")
		return
	}
	if err := h.repo.UpdateImportSession(c.Request.Context(), session.ID, session.UserID, req.Title, false); err != nil {
		api.Fail(c, 500, api.ErrInternal, "更新失败")
		return
	}
	api.OK(c, gin.H{"updated": true})
}
func (h *ImportAgentHandler) Archive(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	if err := h.repo.ArchiveImportSession(c.Request.Context(), session.ID, session.UserID); err != nil {
		api.Fail(c, 500, api.ErrInternal, "归档失败")
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *ImportAgentHandler) Messages(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	items, err := h.repo.ListSessionMessages(c.Request.Context(), session.ID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询消息失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}

func (h *ImportAgentHandler) MessageStream(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		Content              string `json:"content"`
		ExpectedDraftVersion int    `json:"expected_draft_version"`
		LastEventID          int64  `json:"last_event_id"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Content) == "" {
		api.Fail(c, 400, api.ErrInvalidRequest, "消息不能为空")
		return
	}
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	writeNDJSON(c, gin.H{"type": "message_started", "session_id": session.ID})
	action, err := h.conversation.HandleMessage(c.Request.Context(), session, req.Content, req.ExpectedDraftVersion)
	if err != nil {
		slog.Error("import conversation failed", "session_id", session.ID, "err", err)
		message := importConversationErrorMessage(err)
		_, _ = h.repo.AddSessionMessage(c.Request.Context(), &domain.ImportSessionMessage{
			SessionID:   session.ID,
			Role:        "assistant",
			MessageType: "error",
			Content:     message,
			Status:      "failed",
		})
		writeNDJSON(c, gin.H{"type": "error", "error": message})
		writeNDJSON(c, gin.H{"type": "done", "ok": false})
		return
	}
	writeNDJSON(c, gin.H{"type": "tool_result", "action": action})
	events, _ := h.repo.ListImportEvents(c.Request.Context(), session.ID, req.LastEventID, 200)
	for _, event := range events {
		writeNDJSON(c, event)
	}
	writeNDJSON(c, gin.H{"type": "done", "ok": true})
}

func importConversationErrorMessage(err error) string {
	switch err.Error() {
	case "NO_TARGET_DOCUMENTS":
		return "当前会话还没有可处理的文档，请先上传 PDF 后再发送识别指令。"
	case "NO_TARGET_UNITS":
		return "当前没有需要重新识别的题目页窗。"
	case "NO_TARGET_CANDIDATES":
		return "当前没有处于待审核状态的候选题。"
	case "CANDIDATE_REVIEW_SCOPE_INVALID", "CANDIDATE_REVIEW_DECISION_CHANGED":
		return "待审核题目集合已发生变化，请刷新后重新选择。"
	case "INVALID_REVIEW_SCOPE", "INVALID_TARGET_ROLE":
		return "候选题复核范围或目标字段不合法。"
	case "DRAFT_VERSION_CONFLICT":
		return "当前草稿已更新，请刷新会话后重试。"
	case "ACTIVE_RUN_EXISTS":
		return "当前会话已有识别任务正在执行，请等待本轮完成后再继续。"
	case "SESSION_ARCHIVED":
		return "当前导入会话已归档，无法继续执行操作。"
	case "DOCUMENT_SCOPE_VIOLATION":
		return "请求中的文档不属于当前会话，请重新选择文档。"
	case "CANDIDATE_SCOPE_VIOLATION":
		return "请求中的题目不属于当前会话，请刷新后重试。"
	default:
		return "处理这条消息时遇到问题，请稍后重试。"
	}
}

func writeNDJSON(c *gin.Context, value interface{}) {
	raw, _ := json.Marshal(value)
	_, _ = c.Writer.Write(append(raw, '\n'))
	c.Writer.Flush()
}

func (h *ImportAgentHandler) Events(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(c.Query("after_event_id"), 10, 64)
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	// A page refresh needs a finite history snapshot before it attaches to the
	// long-lived stream. Keeping this as an opt-in mode preserves the existing
	// streaming contract for older clients while preventing them from replaying
	// the whole event log as if those events were a new run.
	if c.Query("snapshot") == "1" || strings.EqualFold(c.Query("snapshot"), "true") {
		events, err := h.repo.ListImportEvents(c.Request.Context(), session.ID, after, 500)
		if err != nil {
			return
		}
		for _, event := range events {
			writeNDJSON(c, event)
		}
		return
	}
	ticker := time.NewTicker(time.Second)
	heartbeat := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			events, _ := h.repo.ListImportEvents(c.Request.Context(), session.ID, after, 200)
			for _, event := range events {
				writeNDJSON(c, event)
				after = event.ID
			}
		case <-heartbeat.C:
			writeNDJSON(c, gin.H{"type": "heartbeat", "session_id": session.ID, "after_event_id": after})
		}
	}
}

func (h *ImportAgentHandler) Draft(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	page, pageSize := parseInt(c.Query("page"), 1), parseInt(c.Query("page_size"), 50)
	// Keep the response metadata in sync with SessionDraft's hard cap. Without
	// normalizing here, an invalid client request such as page_size=1000 would
	// return 50 rows but advertise a page size of 1000, causing the UI to skip
	// most pages and look like the PDF contained only a small subset.
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	items, total, err := h.repo.SessionDraft(c.Request.Context(), session.ID, page, pageSize)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询统一草稿失败")
		return
	}
	api.PageOK(c, items, total, page, pageSize)
}
func (h *ImportAgentHandler) UpdateCandidate(c *gin.Context) {
	session, ok := h.owned(c)
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
	raw, _ := json.Marshal(req.Question)
	updated, err := h.repo.UpdateSessionCandidate(c.Request.Context(), session.ID, candidateID, req.ExpectedVersion, string(raw))
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "更新草稿失败")
		return
	}
	if !updated {
		api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *ImportAgentHandler) ReviewCandidate(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int    `json:"expected_draft_version"`
		Decision        string `json:"review_decision"`
		Reason          string `json:"exclusion_reason"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	updated, err := h.repo.UpdateCandidateReview(c.Request.Context(), session.ID, parseID(c.Param("candidate_id")), req.ExpectedVersion, req.Decision, req.Reason, middleware.CurrentUserID(c))
	if err != nil {
		if err.Error() == "EXCLUSION_REASON_REQUIRED" {
			api.Fail(c, 422, api.ErrValidationFailed, "排除题目必须填写原因")
			return
		}
		if err.Error() == "INVALID_REVIEW_DECISION" {
			api.Fail(c, 400, api.ErrInvalidRequest, "审核状态不合法")
			return
		}
		api.FailDetail(c, 500, api.ErrInternal, "更新审核状态失败", err.Error())
		return
	}
	if !updated {
		api.Fail(c, 409, api.ErrConflict, "草稿版本冲突或题目不存在")
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *ImportAgentHandler) ReviewCandidatesBatch(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int     `json:"expected_draft_version"`
		CandidateIDs    []int64 `json:"candidate_ids"`
		Decision        string  `json:"review_decision"`
		Reason          string  `json:"exclusion_reason"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 || len(req.CandidateIDs) == 0 {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	for _, id := range req.CandidateIDs {
		if !h.repo.SessionOwnsCandidate(c.Request.Context(), session.ID, id) {
			api.Fail(c, 404, api.ErrNotFound, "题目不属于当前会话")
			return
		}
	}
	updated, err := h.repo.BulkUpdateCandidateReview(c.Request.Context(), session.ID, req.CandidateIDs, req.ExpectedVersion, req.Decision, req.Reason, middleware.CurrentUserID(c))
	if err != nil {
		api.FailDetail(c, 422, api.ErrValidationFailed, "批量审核失败", err.Error())
		return
	}
	if !updated {
		api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
		return
	}
	api.OK(c, gin.H{"updated": true, "count": len(req.CandidateIDs)})
}

func (h *ImportAgentHandler) ReextractCandidates(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int     `json:"expected_draft_version"`
		CandidateIDs    []int64 `json:"candidate_ids"`
		Scope           string  `json:"scope"`
		Target          string  `json:"target"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 1 {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if session.DraftVersion != req.ExpectedVersion {
		api.Fail(c, 409, api.ErrConflict, "草稿版本冲突，请刷新后重试")
		return
	}
	if req.Scope == "" {
		req.Scope = "pending"
	}
	if req.Target == "" {
		req.Target = "auto"
	}
	if req.Scope != "pending" || (req.Target != "auto" && req.Target != "question" && req.Target != "answer" && req.Target != "both") {
		api.Fail(c, 400, api.ErrInvalidRequest, "候选题复核范围或目标不合法")
		return
	}
	instruction := "按问题类型和来源证据智能精准修复当前待审核题目"
	if req.Target == "answer" {
		instruction = "从 PDF 重识别当前待审核题目的答案"
	} else if req.Target == "both" {
		instruction = "从 PDF 重识别当前待审核题目及答案"
	}
	messageID, err := h.repo.AddSessionMessage(c.Request.Context(), &domain.ImportSessionMessage{SessionID: session.ID, Role: "user", MessageType: "text", Content: instruction, Status: "completed"})
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "记录复核指令失败")
		return
	}
	action := &domain.ConversationAction{Intent: "review_pending_candidates", ReviewScope: req.Scope, TargetRole: req.Target, CandidateIDs: req.CandidateIDs, Instruction: instruction}
	plan, _, err := h.conversation.StartCandidateReview(c.Request.Context(), session, &messageID, action)
	if err != nil {
		status := 500
		code := api.ErrInternal
		if err.Error() == "NO_TARGET_CANDIDATES" || err.Error() == "CANDIDATE_REVIEW_SCOPE_INVALID" || err.Error() == "CANDIDATE_REVIEW_DECISION_CHANGED" || err.Error() == "ACTIVE_RUN_EXISTS" {
			status, code = 409, api.ErrConflict
		}
		api.FailDetail(c, status, code, importConversationErrorMessage(err), err.Error())
		return
	}
	api.OK(c, plan)
}

func (h *ImportAgentHandler) CandidateProvenance(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	candidateID := parseID(c.Param("candidate_id"))
	if !h.repo.SessionOwnsCandidate(c.Request.Context(), session.ID, candidateID) {
		api.Fail(c, 404, api.ErrNotFound, "题目不存在")
		return
	}
	provenance, err := h.repo.GetCandidateProvenance(c.Request.Context(), session.ID, candidateID)
	if err != nil {
		api.FailDetail(c, 404, api.ErrNotFound, "题目来源尚未建立", err.Error())
		return
	}
	api.OK(c, provenance)
}

func (h *ImportAgentHandler) RelinkAnswers(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int `json:"expected_draft_version"`
	}
	if c.Request.ContentLength != 0 && c.ShouldBindJSON(&req) != nil {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if err := h.conversation.RelinkAnswers(c.Request.Context(), session.ID, req.ExpectedVersion); err != nil {
		if err.Error() == "DRAFT_VERSION_CONFLICT" {
			api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
			return
		}
		api.FailDetail(c, 500, api.ErrInternal, "答案重新对齐失败", err.Error())
		return
	}
	api.OK(c, gin.H{"relinked": true})
}

func (h *ImportAgentHandler) RetryAnswerPages(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int `json:"expected_draft_version"`
		PageStart       int `json:"page_start"`
		PageEnd         int `json:"page_end"`
	}
	if c.ShouldBindJSON(&req) != nil || req.PageStart <= 0 || req.PageEnd < req.PageStart {
		api.Fail(c, 400, api.ErrInvalidRequest, "页码范围不合法")
		return
	}
	err := h.conversation.RetryAnswerPages(c.Request.Context(), session.ID, req.ExpectedVersion, req.PageStart, req.PageEnd)
	if err != nil {
		if err.Error() == "DRAFT_VERSION_CONFLICT" {
			api.Fail(c, 409, api.ErrConflict, "草稿版本冲突")
			return
		}
		api.FailDetail(c, 500, api.ErrInternal, "答案页重试失败", err.Error())
		return
	}
	api.OK(c, gin.H{"queued": true, "page_start": req.PageStart, "page_end": req.PageEnd})
}
func (h *ImportAgentHandler) Preview(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	preview, err := h.conversation.Preview(c.Request.Context(), session.ID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "预检失败")
		return
	}
	api.OK(c, preview)
}
func (h *ImportAgentHandler) Commit(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int    `json:"expected_version"`
		PreviewHash     string `json:"preview_hash"`
	}
	if c.ShouldBindJSON(&req) != nil {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	result, err := h.conversation.Commit(c.Request.Context(), session.ID, req.ExpectedVersion, req.PreviewHash)
	if err != nil {
		status := 422
		if err.Error() == "DRAFT_VERSION_CONFLICT" {
			status = 409
		}
		api.FailDetail(c, status, api.ErrValidationFailed, "当前草稿不能提交", err.Error())
		return
	}
	api.OK(c, result)
}
func (h *ImportAgentHandler) ConfirmAction(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		api.Fail(c, 400, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if err := h.conversation.ConfirmAction(c.Request.Context(), session, parseID(c.Param("action_id")), req.ExpectedVersion); err != nil {
		api.FailDetail(c, 409, api.ErrConflict, "操作确认失败", err.Error())
		return
	}
	api.OK(c, gin.H{"confirmed": true})
}
func (h *ImportAgentHandler) CancelRun(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	if err := h.repo.CancelImportRun(c.Request.Context(), session.ID, parseID(c.Param("run_id"))); err != nil {
		api.Fail(c, 500, api.ErrInternal, "取消失败")
		return
	}
	api.OK(c, gin.H{"cancel_requested": true})
}
func (h *ImportAgentHandler) Audit(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	runID := parseID(c.Param("run_id"))
	var exists int
	if h.repo.DB().QueryRowContext(c.Request.Context(), `SELECT 1 FROM import_runs WHERE id=? AND session_id=?`, runID, session.ID).Scan(&exists) != nil {
		api.Fail(c, 404, api.ErrNotFound, "运行不存在")
		return
	}
	rows, err := h.repo.DB().QueryContext(c.Request.Context(), `SELECT ar.id,ar.unit_id,ar.agent_role,ar.prompt_version,COALESCE(ar.provider_name,''),COALESCE(ar.model,''),ar.status,ar.retry_index,ar.input_tokens,ar.output_tokens,ar.latency_ms,COALESCE(ar.validation_error,''),ar.created_at FROM import_agent_runs ar JOIN import_agent_units u ON u.id=ar.unit_id WHERE u.run_id=? ORDER BY ar.id`, runID)
	if err != nil {
		api.Fail(c, 500, api.ErrInternal, "查询审计失败")
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var id, unitID int64
		var role, prompt, providerName, model, status, validation, created string
		var retry, inputTokens, outputTokens, latency int
		if rows.Scan(&id, &unitID, &role, &prompt, &providerName, &model, &status, &retry, &inputTokens, &outputTokens, &latency, &validation, &created) == nil {
			items = append(items, gin.H{"id": id, "unit_id": unitID, "agent_role": role, "prompt_version": prompt, "provider_name": providerName, "model": model, "status": status, "retry_index": retry, "input_tokens": inputTokens, "output_tokens": outputTokens, "latency_ms": latency, "validation_error": validation, "created_at": created})
		}
	}
	api.OK(c, gin.H{"items": items})
}
func (h *ImportAgentHandler) PageImage(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	documentID := parseID(c.Param("document_id"))
	if !h.repo.SessionOwnsDocument(c.Request.Context(), session.ID, documentID) {
		api.Fail(c, 404, api.ErrNotFound, "文档不存在")
		return
	}
	page := parseInt(c.Param("page"), 0)
	if spanID := parseID(c.Query("source_span_id")); spanID > 0 {
		var spanDocumentID int64
		var spanStart, spanEnd int
		err := h.repo.DB().QueryRowContext(c.Request.Context(), `SELECT s.document_id,s.page_start,s.page_end FROM source_evidence_spans s JOIN source_question_origins o ON o.id=s.origin_id WHERE s.id=? AND o.session_id=? AND s.status='active'`, spanID, session.ID).Scan(&spanDocumentID, &spanStart, &spanEnd)
		if err != nil || spanDocumentID != documentID || page < spanStart || page > spanEnd {
			api.Fail(c, 403, api.ErrInvalidRequest, "来源区域不属于当前页面")
			return
		}
	}
	asset, err := h.repo.GetPageAsset(c.Request.Context(), documentID, c.DefaultQuery("purpose", "extraction"), page)
	if err != nil {
		api.Fail(c, 404, api.ErrNotFound, "页面图像不存在")
		return
	}
	if _, err = os.Stat(asset.FilePath); err != nil {
		api.Fail(c, 404, api.ErrNotFound, "页面图像不存在")
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.File(asset.FilePath)
}
func (h *ImportAgentHandler) UpdateDocumentRole(c *gin.Context) {
	session, ok := h.owned(c)
	if !ok {
		return
	}
	documentID := parseID(c.Param("document_id"))
	var req struct {
		Role string `json:"document_role"`
	}
	if c.ShouldBindJSON(&req) != nil || !validDocumentRole(req.Role) {
		api.Fail(c, 400, api.ErrInvalidRequest, "文档角色不合法")
		return
	}
	if !h.repo.SessionOwnsDocument(c.Request.Context(), session.ID, documentID) {
		api.Fail(c, 404, api.ErrNotFound, "文档不存在")
		return
	}
	if err := h.repo.UpdateSessionDocumentRole(c.Request.Context(), session.ID, documentID, req.Role); err != nil {
		api.Fail(c, 500, api.ErrInternal, "更新角色失败")
		return
	}
	if err := h.conversation.ReconcileSession(c.Request.Context(), session.ID); err != nil {
		api.FailDetail(c, 500, api.ErrInternal, "角色已更新，但重新协调草稿失败", err.Error())
		return
	}
	api.OK(c, gin.H{"updated": true})
}
func validDocumentRole(role string) bool {
	switch role {
	case "question_book", "answer_book", "explanation_book", "mixed", "supplement", "unknown":
		return true
	}
	return false
}

var _ = fmt.Sprintf
