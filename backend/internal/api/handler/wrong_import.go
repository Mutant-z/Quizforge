package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
	"github.com/quiztrace/quiztrace/internal/service"
)

// WrongImportHandler 错题导入 Agent 接口：
// 图片上传 → 视觉提取草稿（暂存不入库）→ 补答案对话 → 确认入库。
type WrongImportHandler struct {
	repo *sqlite.Repository
	svc  *service.WrongImportService
	cfg  *config.Config
}

var (
	wrongImportAnswerPromptIntent = regexp.MustCompile(`答案|参考答案|答题卡|答案解析`)
	wrongImportAnswerPromptAction = regexp.MustCompile(`识别|提取|读取|回填|匹配|看看|这里`)
)

func isWrongImportAnswerPrompt(instruction string) bool {
	instruction = strings.TrimSpace(instruction)
	return instruction != "" && wrongImportAnswerPromptIntent.MatchString(instruction) && wrongImportAnswerPromptAction.MatchString(instruction)
}

func NewWrongImportHandler(repo *sqlite.Repository, cfg *config.Config) *WrongImportHandler {
	return &WrongImportHandler{repo: repo, svc: service.NewWrongImportService(repo, cfg), cfg: cfg}
}

func (h *WrongImportHandler) ownedSession(c *gin.Context) (*domain.WrongImportSession, bool) {
	session, err := h.repo.GetWrongImportSession(c.Request.Context(), parseID(c.Param("id")))
	if err != nil || session.UserID != middleware.CurrentUserID(c) {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "导入会话不存在")
		return nil, false
	}
	return session, true
}

// ---------- 会话 ----------

func (h *WrongImportHandler) Create(c *gin.Context) {
	var req struct {
		Title        string `json:"title"`
		TargetBankID *int64 `json:"target_bank_id"`
	}
	_ = c.ShouldBindJSON(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "错题导入"
	}
	session, err := h.repo.CreateWrongImportSession(c.Request.Context(), middleware.CurrentUserID(c), &title, req.TargetBankID)
	if err != nil {
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "创建会话失败", err.Error())
		return
	}
	h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventSessionMsg, nil)
	api.Created(c, session)
}

func (h *WrongImportHandler) List(c *gin.Context) {
	items, err := h.repo.ListWrongImportSessions(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询会话失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}

// Get 会话详情（含图片、草稿、消息），侧栏刷新/恢复现场用。
func (h *WrongImportHandler) Get(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if err := h.svc.NormalizeMaterialAnalysisDrafts(c.Request.Context(), session.ID); err != nil {
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "修复历史题型状态失败", err.Error())
		return
	}
	images, _ := h.repo.ListWrongImportImages(c.Request.Context(), session.ID)
	batches, _ := h.repo.ListWrongImportImageBatches(c.Request.Context(), session.ID)
	answerFragments, _ := h.repo.ListWrongImportAnswerFragments(c.Request.Context(), session.ID)
	drafts, _ := h.repo.ListWrongImportDrafts(c.Request.Context(), session.ID)
	messages, _ := h.repo.ListWrongImportMessages(c.Request.Context(), session.ID, 50)
	api.OK(c, gin.H{
		"session":          session,
		"images":           images,
		"batches":          batches,
		"answer_fragments": answerFragments,
		"drafts":           drafts,
		"messages":         messages,
	})
}

func (h *WrongImportHandler) Discard(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if err := h.repo.ArchiveWrongImportSession(c.Request.Context(), session.ID, session.UserID); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "丢弃会话失败")
		return
	}
	c.Status(http.StatusNoContent)
}

// FinalizeBatch 封存一次前端批量上传，并在所有图片结束后触发跨图整理。
func (h *WrongImportHandler) FinalizeBatch(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法封存图片批次")
		return
	}
	var req struct {
		ExpectedCount int `json:"expected_count"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedCount < 1 {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "批次数量参数不合法")
		return
	}
	batch, err := h.repo.SealWrongImportImageBatch(c.Request.Context(), c.Param("batch_id"), session.ID, req.ExpectedCount)
	if err != nil {
		api.FailDetail(c, http.StatusConflict, api.ErrConflict, "封存图片批次失败", err.Error())
		return
	}
	if err := h.svc.QueueBatchReconcileIfReady(c.Request.Context(), session.ID, batch.ID); err != nil {
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "跨图整理任务入队失败", err.Error())
		return
	}
	batch, _ = h.repo.GetWrongImportImageBatch(c.Request.Context(), batch.ID, session.ID)
	api.OK(c, gin.H{"batch": batch})
}

// Reconcile 手动触发当前会话或指定批次的跨图检查。
func (h *WrongImportHandler) Reconcile(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法检查跨图题目")
		return
	}
	var req struct {
		Scope   string `json:"scope"`
		BatchID string `json:"batch_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.Scope == "" || req.Scope == "legacy_session" {
		if _, err := h.svc.QueueLegacyReconcile(c.Request.Context(), session.ID); err != nil {
			api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "跨图检查入队失败", err.Error())
			return
		}
		api.OK(c, gin.H{"queued": true, "scope": "legacy_session"})
		return
	}
	if req.Scope != "batch" || strings.TrimSpace(req.BatchID) == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "跨图检查范围不合法")
		return
	}
	if err := h.svc.QueueBatchReconcileIfReady(c.Request.Context(), session.ID, req.BatchID); err != nil {
		api.FailDetail(c, http.StatusConflict, api.ErrConflict, "批次尚未具备整理条件", err.Error())
		return
	}
	api.OK(c, gin.H{"queued": true, "scope": "batch", "batch_id": req.BatchID})
}

// RetryAnswerExtraction 重新使用当前会话已有的答案参考图进行识别。
func (h *WrongImportHandler) RetryAnswerExtraction(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法重新识别答案")
		return
	}
	count, err := h.svc.RetryAnswerExtraction(c.Request.Context(), session.ID)
	if err != nil {
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "重新识别答案失败", err.Error())
		return
	}
	api.OK(c, gin.H{"queued": count})
}

func (h *WrongImportHandler) ListMergeCandidates(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	items, err := h.repo.ListWrongImportMergeCandidates(c.Request.Context(), session.ID, c.Query("status"))
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询跨图候选失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}

func (h *WrongImportHandler) ResolveMergeCandidate(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法处理跨图候选")
		return
	}
	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Action != "accept" && req.Action != "reject") {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "处理动作不合法")
		return
	}
	draft, err := h.svc.ResolveMergeCandidate(c.Request.Context(), session.ID, parseID(c.Param("candidate_id")), req.Action)
	if err != nil {
		if strings.Contains(err.Error(), "not pending") || strings.Contains(err.Error(), "changed") {
			api.Fail(c, http.StatusConflict, api.ErrConflict, "跨图候选已变化，请刷新后重试")
			return
		}
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "处理跨图候选失败", err.Error())
		return
	}
	api.OK(c, gin.H{"action": req.Action, "draft": draft})
}

func (h *WrongImportHandler) UndoMerge(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法撤销合并")
		return
	}
	if err := h.svc.UndoMerge(c.Request.Context(), session.ID, parseID(c.Param("merge_id"))); err != nil {
		if strings.Contains(err.Error(), "edited after merge") {
			api.Fail(c, http.StatusConflict, api.ErrConflict, "合并后的题目已被编辑，无法自动撤销")
			return
		}
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "撤销合并失败", err.Error())
		return
	}
	api.OK(c, gin.H{"undone": true})
}

// ---------- 图片上传（复用分片上传工具函数） ----------

func (h *WrongImportHandler) detectImageMime(filename, declared string) string {
	mime := strings.ToLower(strings.TrimSpace(declared))
	for _, allowed := range h.cfg.WrongImport.AllowedImageTypes {
		if mime == allowed {
			return mime
		}
	}
	ext := strings.ToLower(filepath.Ext(security.SanitizeFileName(filename)))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	return ""
}

func normalizeWrongImportExtractionMode(raw string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return domain.WrongImportExtractionQuestions, true
	}
	if mode == domain.WrongImportExtractionQuestions || mode == domain.WrongImportExtractionAnswerKey || mode == domain.WrongImportExtractionAuto {
		return mode, true
	}
	return "", false
}

func (h *WrongImportHandler) InitImage(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法继续上传")
		return
	}
	var req struct {
		Filename       string `json:"filename" binding:"required"`
		Size           int64  `json:"size" binding:"required"`
		MimeType       string `json:"mime_type"`
		BatchID        string `json:"batch_id"`
		BatchIndex     *int   `json:"batch_index"`
		BatchSize      int    `json:"batch_size"`
		ExtractionMode string `json:"extraction_mode"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Size <= 0 {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if _, valid := normalizeWrongImportExtractionMode(req.ExtractionMode); !valid {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "图片提取模式不合法")
		return
	}
	maxBytes := int64(h.cfg.WrongImport.MaxImageMB) * 1024 * 1024
	if req.Size > maxBytes {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "单张图片大小超出限制")
		return
	}
	if req.BatchID != "" {
		if req.BatchSize < 1 || req.BatchIndex == nil || *req.BatchIndex < 0 || *req.BatchIndex >= req.BatchSize {
			api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "图片批次参数不合法")
			return
		}
		if _, err := h.repo.EnsureWrongImportImageBatch(c.Request.Context(), req.BatchID, session.ID, req.BatchSize); err != nil {
			api.FailDetail(c, http.StatusBadRequest, api.ErrInvalidRequest, "创建图片批次失败", err.Error())
			return
		}
	}
	mime := h.detectImageMime(req.Filename, req.MimeType)
	if mime == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "仅支持 PNG / JPG 图片")
		return
	}
	uploadID := uuid.NewString()
	dir := filepath.Join(h.cfg.Upload.Root, "uploads", uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建上传目录失败")
		return
	}
	chunkSize := h.cfg.Upload.ChunkSizeMB * 1024 * 1024
	chunkCount := int(req.Size / int64(chunkSize))
	if req.Size%int64(chunkSize) != 0 {
		chunkCount++
	}
	if chunkCount < 1 {
		chunkCount = 1
	}
	api.Created(c, gin.H{
		"upload_id":   uploadID,
		"chunk_size":  chunkSize,
		"chunk_count": chunkCount,
		"filename":    security.SanitizeFileName(req.Filename),
		"mime_type":   mime,
		"size":        req.Size,
		"session_id":  session.ID,
	})
}

// UploadImageChunk 保存图片分片（参数名统一为 image_id 以满足 gin 路由前缀约束，
// 该位置的值实为 upload_id）。
func (h *WrongImportHandler) UploadImageChunk(c *gin.Context) {
	uploadID := c.Param("image_id")
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || uploadID == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "分片参数不合法")
		return
	}
	dir := filepath.Join(h.cfg.Upload.Root, "uploads", uploadID)
	if _, err := os.Stat(dir); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "上传任务不存在")
		return
	}
	file, err := c.FormFile("chunk")
	if err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "缺少分片文件")
		return
	}
	src, err := file.Open()
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "读取分片失败")
		return
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(dir, fmt.Sprintf("chunk_%05d", index)))
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存分片失败")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "写入分片失败")
		return
	}
	api.OK(c, gin.H{"upload_id": uploadID, "index": index, "received": true})
}

// CompleteImage 合并分片、校验真实图片格式、登记并入队提取。
func (h *WrongImportHandler) CompleteImage(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法继续上传")
		return
	}
	uploadID := c.Param("image_id")
	var req struct {
		Filename       string `json:"filename" binding:"required"`
		MimeType       string `json:"mime_type"`
		TotalSize      int64  `json:"total_size"`
		ChunkCount     int    `json:"chunk_count"`
		BatchID        string `json:"batch_id"`
		BatchIndex     *int   `json:"batch_index"`
		BatchSize      int    `json:"batch_size"`
		ExtractionMode string `json:"extraction_mode"`
		Instruction    string `json:"instruction"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ChunkCount < 1 {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	extractionMode, validMode := normalizeWrongImportExtractionMode(req.ExtractionMode)
	if !validMode {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "图片提取模式不合法")
		return
	}
	// 前端会根据 prompt 选择模式，但服务端也做一次兜底判断，避免
	// 用户明确要求“提取答案”时被客户端状态或旧版本页面误判成题目提取。
	if extractionMode == domain.WrongImportExtractionQuestions && isWrongImportAnswerPrompt(req.Instruction) {
		extractionMode = domain.WrongImportExtractionAnswerKey
	}
	mime := h.detectImageMime(req.Filename, req.MimeType)
	if mime == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "仅支持 PNG / JPG 图片")
		return
	}
	if req.BatchID != "" {
		if req.BatchSize < 1 || req.BatchIndex == nil || *req.BatchIndex < 0 || *req.BatchIndex >= req.BatchSize {
			api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "图片批次参数不合法")
			return
		}
		if _, err := h.repo.EnsureWrongImportImageBatch(c.Request.Context(), req.BatchID, session.ID, req.BatchSize); err != nil {
			api.FailDetail(c, http.StatusBadRequest, api.ErrInvalidRequest, "图片批次不存在或参数不一致", err.Error())
			return
		}
	}
	dir := filepath.Join(h.cfg.Upload.Root, "uploads", uploadID)
	mergedPath := filepath.Join(dir, "merged")
	if err := mergeChunks(dir, mergedPath, req.ChunkCount); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "分片不完整: "+err.Error())
		return
	}
	hash, size, err := fileSHA256(mergedPath)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "计算 hash 失败")
		return
	}
	if size != req.TotalSize {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "文件大小不一致")
		return
	}
	// 用真实解码校验内容是图片（防 MIME 伪装）
	data, err := os.ReadFile(mergedPath)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "读取上传文件失败")
		return
	}
	imgCfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "文件不是有效的 PNG/JPG 图片")
		return
	}
	// 会话内相同图片去重：失败或 0 条结果允许重试；如果旧记录是按题目模式
	// 误处理的，用户改以 answer_key 上传时复用旧记录，避免残留错误草稿。
	existing, hashErr := h.repo.GetWrongImportImageByHash(c.Request.Context(), session.ID, hash)
	if hashErr != nil {
		_ = os.RemoveAll(dir)
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "校验图片重复状态失败", hashErr.Error())
		return
	}
	if existing != nil {
		inProgress := existing.Status == domain.WrongImportImagePending || existing.Status == domain.WrongImportImageProcessing
		retryAsAnswer := extractionMode == domain.WrongImportExtractionAnswerKey && existing.ExtractionMode != domain.WrongImportExtractionAnswerKey
		answerRetry := extractionMode == domain.WrongImportExtractionAnswerKey && existing.ExtractionMode == domain.WrongImportExtractionAnswerKey && existing.Status == domain.WrongImportImageDone
		emptyResult := existing.Status == domain.WrongImportImageFailed || (existing.Status == domain.WrongImportImageDone && existing.DraftCount == 0)
		if inProgress {
			_ = os.RemoveAll(dir)
			api.Fail(c, http.StatusConflict, api.ErrConflict, "这张图片正在识别中，请等待当前任务完成")
			return
		}
		if retryAsAnswer || answerRetry || (emptyResult && extractionMode == domain.WrongImportExtractionAnswerKey) {
			// 带 batch_id 的新上传需要在新批次中登记一张图片，否则 finalize
			// 会因批次图片数量为 0 而失败。因此只有老式无批次上传复用旧记录。
			if req.BatchID != "" {
				if retryAsAnswer {
					if err := h.repo.DeleteUneditedWrongImportDraftsByImage(c.Request.Context(), session.ID, existing.ID); err != nil {
						api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "清理旧答案草稿失败", err.Error())
						return
					}
					_ = h.repo.RefreshWrongImportImageDraftCount(c.Request.Context(), existing.ID)
					existing, _ = h.repo.GetWrongImportImageByHash(c.Request.Context(), session.ID, hash)
				}
			} else {
				_ = os.RemoveAll(dir)
				if err := h.repo.PrepareWrongImportImageForAnswerExtraction(c.Request.Context(), session.ID, existing.ID); err != nil {
					api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "准备答案图片重试失败", err.Error())
					return
				}
				img, err := h.repo.GetWrongImportImage(c.Request.Context(), existing.ID)
				if err != nil {
					api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "读取重试图片失败", err.Error())
					return
				}
				h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventImageQueued, map[string]any{"image_id": img.ID, "retry": true, "answer_mode": true})
				instruction := strings.TrimSpace(req.Instruction)
				if len([]rune(instruction)) > 2000 {
					instruction = string([]rune(instruction)[:2000])
				}
				message := instruction
				if message == "" {
					message = fmt.Sprintf("重新上传答案参考图「%s」，请识别并回填已有题目的答案", img.OriginalName)
				}
				var metaJSON string
				if req.BatchID != "" {
					metaMap := map[string]interface{}{"batch_id": req.BatchID, "batch_size": req.BatchSize}
					if b, err := json.Marshal(metaMap); err == nil {
						metaJSON = string(b)
					}
				}
				_, _ = h.repo.AddWrongImportMessage(c.Request.Context(), session.ID, "user", message, metaJSON)
				h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventSessionMsg, nil)
				if h.cfg.WrongImport.AutoExtract {
					if err := h.svc.QueueExtraction(c.Request.Context(), img.ID); err != nil {
						api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "图片已准备重试，但提取任务入队失败", err.Error())
						return
					}
				}
				api.OK(c, img)
				return
			}
		}
		if existing.Status == domain.WrongImportImageDone && existing.DraftCount > 0 && !answerRetry {
			_ = os.RemoveAll(dir)
			api.Fail(c, http.StatusConflict, api.ErrConflict, "这张图片已在本会话中成功处理过，无需重复上传")
			return
		}
		if existing.Status != domain.WrongImportImageFailed && !(existing.Status == domain.WrongImportImageDone && existing.DraftCount == 0) && !answerRetry {
			_ = os.RemoveAll(dir)
			api.Fail(c, http.StatusConflict, api.ErrConflict, "这张图片已在本会话中上传过")
			return
		}
	}
	finalDir := filepath.Join(h.cfg.Upload.Root, "wrong-import", strconv.FormatInt(session.ID, 10))
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存文件失败")
		return
	}
	finalPath := filepath.Join(finalDir, uuid.NewString()+filepath.Ext(security.SanitizeFileName(req.Filename)))
	if err := os.Rename(mergedPath, finalPath); err != nil {
		if err := copyFile(mergedPath, finalPath); err != nil {
			api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存文件失败")
			return
		}
		_ = os.Remove(mergedPath)
	}
	_ = os.RemoveAll(dir)

	img, err := h.registerImageWithRetry(c.Request.Context(), &domain.WrongImportImage{
		SessionID:      session.ID,
		BatchID:        req.BatchID,
		BatchIndex:     req.BatchIndex,
		OriginalName:   security.SanitizeFileName(req.Filename),
		FilePath:       finalPath,
		FileHash:       hash,
		MIMEType:       mime,
		ExtractionMode: extractionMode,
		SizeBytes:      size,
		Width:          imgCfg.Width,
		Height:         imgCfg.Height,
		Status:         domain.WrongImportImagePending,
	})
	if err != nil {
		slog.Error("wrong import image register failed", "session_id", session.ID, "filename", req.Filename, "err", err)
		api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "登记图片失败", err.Error())
		return
	}
	h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventImageQueued, map[string]any{"image_id": img.ID, "batch_id": img.BatchID, "batch_index": img.BatchIndex})
	// 对话流：上传动作以用户消息呈现，与 PDF 导入 Pipeline 的交互形式一致
	shouldAddMsg := req.BatchID == "" || (req.BatchIndex != nil && *req.BatchIndex == 0)
	if shouldAddMsg {
		instruction := strings.TrimSpace(req.Instruction)
		if len([]rune(instruction)) > 2000 {
			instruction = string([]rune(instruction)[:2000])
		}
		message := instruction
		if message == "" {
			message = fmt.Sprintf("上传了错题图片「%s」，请提取其中的题目", img.OriginalName)
			if req.BatchID != "" && req.BatchSize > 1 {
				message = fmt.Sprintf("上传了 %d 张错题图片，请提取其中的题目", req.BatchSize)
			}
			if extractionMode == domain.WrongImportExtractionAnswerKey {
				message = fmt.Sprintf("上传了答案参考图「%s」，请识别并回填已有题目的答案", img.OriginalName)
				if req.BatchID != "" && req.BatchSize > 1 {
					message = fmt.Sprintf("上传了 %d 张答案参考图，请识别并回填已有题目的答案", req.BatchSize)
				}
			} else if extractionMode == domain.WrongImportExtractionAuto {
				message = fmt.Sprintf("上传了 %d 张题目和答案图片，请自动识别并整理", req.BatchSize)
			}
		}
		var metaJSON string
		if req.BatchID != "" {
			metaMap := map[string]interface{}{"batch_id": req.BatchID, "batch_size": req.BatchSize}
			if b, err := json.Marshal(metaMap); err == nil {
				metaJSON = string(b)
			}
		}
		_, _ = h.repo.AddWrongImportMessage(c.Request.Context(), session.ID, "user", message, metaJSON)
		h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventSessionMsg, nil)
	}
	if h.cfg.WrongImport.AutoExtract {
		if err := h.svc.QueueExtraction(c.Request.Context(), img.ID); err != nil {
			api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "图片已上传，但提取任务入队失败", err.Error())
			return
		}
	}
	api.Created(c, img)
}

// registerImageWithRetry 对 SQLite 写锁冲突做有限退避重试。后台任务
// （候选题复核、目录重建等）可能连续持有写连接超过 busy_timeout，
// 此时登记 INSERT 会以 database is locked 失败，不应直接让上传报错。
func (h *WrongImportHandler) registerImageWithRetry(ctx context.Context, img *domain.WrongImportImage) (*domain.WrongImportImage, error) {
	var lastErr error
	for _, delay := range []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second} {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		created, err := h.repo.CreateWrongImportImage(ctx, img)
		if err == nil {
			return created, nil
		}
		lastErr = err
		if !isSQLiteBusyError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}

// ExtractImage 手动重新提取（失败重试入口）。
func (h *WrongImportHandler) ExtractImage(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	imageID := parseID(c.Param("image_id"))
	img, err := h.repo.GetWrongImportImage(c.Request.Context(), imageID)
	if err != nil || img.SessionID != session.ID {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "图片不存在")
		return
	}
	if img.Status == domain.WrongImportImageProcessing {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "图片正在识别中")
		return
	}
	if err := h.svc.QueueExtraction(c.Request.Context(), imageID); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "入队失败")
		return
	}
	api.OK(c, gin.H{"queued": true})
}

// ImageFile 图片预览；?crop=left,top,right,bottom（0..1000 归一化）返回草稿区域。
func (h *WrongImportHandler) ImageFile(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	imageID := parseID(c.Param("image_id"))
	img, err := h.repo.GetWrongImportImage(c.Request.Context(), imageID)
	if err != nil || img.SessionID != session.ID {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "图片不存在")
		return
	}
	data, err := os.ReadFile(img.FilePath)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "图片文件不存在")
		return
	}
	contentType := img.MIMEType
	if crop := c.Query("crop"); crop != "" {
		region, valid := parseCropRegion(crop)
		if valid {
			if cropped, err := cropWrongImage(data, region); err == nil && len(cropped) > 0 {
				data = cropped
				contentType = "image/jpeg"
			}
		}
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, contentType, data)
}

func parseCropRegion(raw string) (domain.VisionRegion, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return domain.VisionRegion{}, false
	}
	nums := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return domain.VisionRegion{}, false
		}
		nums[i] = v
	}
	r := domain.VisionRegion{Left: nums[0], Top: nums[1], Right: nums[2], Bottom: nums[3]}
	if r.Left < 0 || r.Top < 0 || r.Right > 1000 || r.Bottom > 1000 || r.Right <= r.Left || r.Bottom <= r.Top {
		return domain.VisionRegion{}, false
	}
	return r, true
}

func cropWrongImage(data []byte, region domain.VisionRegion) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	left := bounds.Min.X + bounds.Dx()*region.Left/1000
	top := bounds.Min.Y + bounds.Dy()*region.Top/1000
	right := bounds.Min.X + bounds.Dx()*region.Right/1000
	bottom := bounds.Min.Y + bounds.Dy()*region.Bottom/1000
	if right <= left || bottom <= top {
		return data, nil
	}
	cropped := image.NewRGBA(image.Rect(0, 0, right-left, bottom-top))
	draw.Draw(cropped, cropped.Bounds(), img, image.Pt(left, top), draw.Src)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, cropped, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---------- 草稿 ----------

func (h *WrongImportHandler) ListDrafts(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	drafts, err := h.repo.ListWrongImportDrafts(c.Request.Context(), session.ID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询草稿失败")
		return
	}
	api.OK(c, gin.H{"items": drafts})
}

// PatchDraft 手动编辑草稿；答案变化会联动 review_status 与 answer_source。
func (h *WrongImportHandler) PatchDraft(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	draft, err := h.repo.GetWrongImportDraft(c.Request.Context(), parseID(c.Param("draft_id")))
	if err != nil || draft.SessionID != session.ID {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题目不存在")
		return
	}
	var req struct {
		Stem         *string                  `json:"stem"`
		Options      *[]domain.QuestionOption `json:"options"`
		Answer       *[]string                `json:"answer"`
		Analysis     *string                  `json:"analysis"`
		QuestionType *string                  `json:"question_type"`
		MarkNoAnswer bool                     `json:"mark_no_answer"`
		Excluded     *bool                    `json:"excluded"`
	}
	if c.ShouldBindJSON(&req) != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	upd := &sqlite.WrongImportDraftUpdate{}
	if req.Stem != nil && strings.TrimSpace(*req.Stem) != "" {
		v := strings.TrimSpace(*req.Stem)
		upd.Stem = &v
	}
	if req.Options != nil {
		upd.Options = req.Options
	}
	if req.QuestionType != nil {
		t := domain.CanonicalWrongImportQuestionType(*req.QuestionType)
		if t == "fill_in_blank" {
			t = domain.QuestionTypeFillBlank
		}
		upd.QuestionType = &t
		if t == domain.QuestionTypeMaterialAnalysis && len(draft.Answer) == 0 && !draft.UserMarkedNoAnswer {
			answerSource := domain.WrongImportAnswerSourcePending
			answerStatus := domain.WrongImportAnswerStatusMissing
			reviewStatus := domain.WrongImportDraftReviewMissingAnswer
			if draft.ReviewStatus == domain.WrongImportDraftReviewCommitted {
				reviewStatus = domain.WrongImportDraftReviewCommitted
			}
			upd.AnswerSource = &answerSource
			upd.AnswerStatus = &answerStatus
			upd.ReviewStatus = &reviewStatus
		}
	}
	if req.Analysis != nil {
		upd.Analysis = req.Analysis
	}
	if req.MarkNoAnswer {
		source := domain.WrongImportAnswerSourceNone
		answerStatus := domain.WrongImportAnswerStatusExplicitNone
		marked := true
		empty := []string{}
		status := domain.WrongImportDraftReviewReady
		if draft.ReviewStatus == domain.WrongImportDraftReviewCommitted {
			status = domain.WrongImportDraftReviewCommitted
		}
		upd.Answer, upd.AnswerSource, upd.AnswerStatus, upd.UserMarkedNoAnswer, upd.ReviewStatus = &empty, &source, &answerStatus, &marked, &status
	} else if req.Answer != nil {
		answer := *req.Answer
		source := domain.WrongImportAnswerSourceUser
		answerStatus := domain.WrongImportAnswerStatusProvided
		marked := false
		status := domain.WrongImportDraftReviewReady
		if draft.ReviewStatus == domain.WrongImportDraftReviewCommitted {
			status = domain.WrongImportDraftReviewCommitted
		} else if len(answer) == 0 {
			source = domain.WrongImportAnswerSourcePending
			answerStatus = domain.WrongImportAnswerStatusMissing
			probe := &domain.WrongImportDraft{QuestionType: draft.QuestionType, AnswerStatus: answerStatus}
			if upd.QuestionType != nil {
				probe.QuestionType = *upd.QuestionType
			}
			if probe.NeedsAnswer() {
				status = domain.WrongImportDraftReviewMissingAnswer
			}
		}
		upd.Answer, upd.AnswerSource, upd.AnswerStatus, upd.UserMarkedNoAnswer, upd.ReviewStatus = &answer, &source, &answerStatus, &marked, &status
	}
	if req.Excluded != nil {
		status := domain.WrongImportDraftReviewReady
		if *req.Excluded {
			status = domain.WrongImportDraftReviewExcluded
		} else if draft.ReviewStatus == domain.WrongImportDraftReviewExcluded {
			// 恢复时回到提取态或缺答案态
			probe := &domain.WrongImportDraft{QuestionType: draft.QuestionType}
			if len(draft.Answer) > 0 {
				probe.ReviewStatus = domain.WrongImportDraftReviewReady
				status = domain.WrongImportDraftReviewReady
			} else if probe.NeedsAnswer() {
				status = domain.WrongImportDraftReviewMissingAnswer
			} else {
				status = domain.WrongImportDraftReviewReady
			}
		}
		upd.ReviewStatus = &status
	}
	updated, err := h.repo.UpdateWrongImportDraft(c.Request.Context(), draft.ID, upd)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新失败")
		return
	}
	_ = h.repo.RefreshWrongImportCounters(c.Request.Context(), session.ID)
	h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventDraftUpdated, map[string]any{"draft_id": updated.ID})
	api.OK(c, updated)
}

func (h *WrongImportHandler) DeleteDraft(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	draftID := parseID(c.Param("draft_id"))
	draft, err := h.repo.GetWrongImportDraft(c.Request.Context(), draftID)
	if err != nil || draft.SessionID != session.ID {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题目不存在")
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法删除")
		return
	}
	if err := h.repo.DeleteWrongImportDraft(c.Request.Context(), session.ID, draftID); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除失败")
		return
	}
	_ = h.repo.RefreshWrongImportImageDraftCount(c.Request.Context(), draft.ImageID)
	_ = h.repo.RefreshWrongImportCounters(c.Request.Context(), session.ID)
	h.repo.AddWrongImportEvent(c.Request.Context(), session.ID, domain.WrongImportEventDraftDeleted, map[string]any{"draft_id": draftID})
	api.OK(c, gin.H{"deleted": true})
}

// ---------- 消息 ----------

func (h *WrongImportHandler) ListMessages(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	items, err := h.repo.ListWrongImportMessages(c.Request.Context(), session.ID, parseInt(c.Query("limit"), 50))
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询消息失败")
		return
	}
	api.OK(c, gin.H{"items": items})
}

// SendMessage 补答案对话；agent 回复由 service 结构化解析后落库。
func (h *WrongImportHandler) SendMessage(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	if session.Status != domain.WrongImportSessionStatusActive {
		api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束")
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Content) == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "消息不能为空")
		return
	}
	agentMsg, err := h.svc.HandleUserMessage(c.Request.Context(), session, req.Content)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "处理消息失败")
		return
	}
	api.OK(c, agentMsg)
}

// ---------- 确认入库 ----------

func (h *WrongImportHandler) Confirm(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	var req struct {
		DraftIDs     []int64 `json:"draft_ids"`
		TargetBankID *int64  `json:"target_bank_id"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.Confirm(c.Request.Context(), session, req.DraftIDs, req.TargetBankID)
	if err != nil {
		switch err.Error() {
		case "SESSION_NOT_ACTIVE":
			api.Fail(c, http.StatusConflict, api.ErrConflict, "会话已结束，无法入库")
		case "NO_READY_DRAFTS":
			api.Fail(c, http.StatusUnprocessableEntity, api.ErrValidationFailed, "没有可入库的题目：请先补充答案或勾选就绪题目")
		case "TARGET_BANK_NOT_FOUND":
			api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "目标题库不存在")
		default:
			api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "入库失败", err.Error())
		}
		return
	}
	api.OK(c, result)
}

// ---------- 事件流（NDJSON，同 import-agent Events 模式） ----------

func (h *WrongImportHandler) Events(c *gin.Context) {
	session, ok := h.ownedSession(c)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(c.Query("after_event_id"), 10, 64)
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	if c.Query("snapshot") == "1" || strings.EqualFold(c.Query("snapshot"), "true") {
		events, err := h.repo.ListWrongImportEvents(c.Request.Context(), session.ID, after, 500)
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
			events, _ := h.repo.ListWrongImportEvents(c.Request.Context(), session.ID, after, 200)
			for _, event := range events {
				writeNDJSON(c, event)
				after = event.ID
			}
		case <-heartbeat.C:
			writeNDJSON(c, gin.H{"type": "heartbeat", "session_id": session.ID, "after_event_id": after})
		}
	}
}
