package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

// UploadHandler 分片上传（docs/05 §2-3）。
type UploadHandler struct {
	repo *sqlite.Repository
	cfg  *config.Config
}

func NewUploadHandler(repo *sqlite.Repository, cfg *config.Config) *UploadHandler {
	return &UploadHandler{repo: repo, cfg: cfg}
}

type uploadInitRequest struct {
	Filename  string `json:"filename" binding:"required"`
	Size      int64  `json:"size" binding:"required"`
	MimeType  string `json:"mime_type"`
	BankID    *int64 `json:"bank_id"`
	SessionID int64  `json:"session_id"`
}

// Init 初始化上传。
func (h *UploadHandler) Init(c *gin.Context) {
	var req uploadInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.SessionID == 0 {
		req.SessionID = parseID(c.Param("session_id"))
		if req.SessionID == 0 {
			req.SessionID = parseID(c.Param("id"))
		}
	}
	if req.Size <= 0 || req.Size > int64(h.cfg.Upload.MaxFileSizeMB)*1024*1024 {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, fmt.Sprintf("文件大小超出限制（最大 %d MB）", h.cfg.Upload.MaxFileSizeMB))
		return
	}
	// MIME 校验
	mime := req.MimeType
	if mime == "" {
		mime = detectMime(req.Filename)
	}
	if !allowedMime(h.cfg.Upload.AllowedTypes, mime) {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "不支持的文件类型")
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
	api.Created(c, gin.H{
		"upload_id":   uploadID,
		"chunk_size":  chunkSize,
		"chunk_count": chunkCount,
		"filename":    security.SanitizeFileName(req.Filename),
		"mime_type":   mime,
		"size":        req.Size,
		"bank_id":     req.BankID,
		"session_id":  req.SessionID,
	})
}

// UploadChunk 上传分片。
func (h *UploadHandler) UploadChunk(c *gin.Context) {
	uploadID := c.Param("upload_id")
	if uploadID == "" {
		uploadID = c.Param("id")
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "分片序号不合法")
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

type uploadCompleteRequest struct {
	Filename        string `json:"filename" binding:"required"`
	MimeType        string `json:"mime_type"`
	BankID          *int64 `json:"bank_id"`
	TotalSize       int64  `json:"total_size"`
	ChunkCount      int    `json:"chunk_count"`
	FileHash        string `json:"file_hash"`
	SessionID       int64  `json:"session_id"`
	DeferProcessing bool   `json:"defer_processing"`
}

// Complete 合并分片、计算 hash、创建 document + import job。
func (h *UploadHandler) Complete(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	uploadID := c.Param("upload_id")
	if uploadID == "" {
		uploadID = c.Param("id")
	}
	var req uploadCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.SessionID == 0 {
		req.SessionID = parseID(c.Param("session_id"))
		if req.SessionID == 0 {
			req.SessionID = parseID(c.Param("id"))
		}
	}
	dir := filepath.Join(h.cfg.Upload.Root, "uploads", uploadID)
	mergedPath := filepath.Join(dir, "merged")
	if err := mergeChunks(dir, mergedPath, req.ChunkCount); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "分片不完整: "+err.Error())
		return
	}

	// 计算文件 hash
	hash, size, err := fileSHA256(mergedPath)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "计算 hash 失败")
		return
	}
	if size != req.TotalSize {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "文件大小不一致")
		return
	}

	// 幂等：相同 hash 已存在
	existing, err := h.repo.GetDocumentByHash(c.Request.Context(), hash)
	if err == nil && existing != nil {
		// 清理临时文件
		_ = os.RemoveAll(dir)
		if req.SessionID > 0 && existing.UserID == uid {
			link, imp, run, attachErr := h.attachDocument(c, req.SessionID, uid, existing.ID, req.DeferProcessing)
			if attachErr != nil {
				api.FailDetail(c, http.StatusConflict, api.ErrConflict, "文档无法加入当前会话", attachErr.Error())
				return
			}
			var jobID int64
			if !req.DeferProcessing || run != nil {
				queuedRunID := int64(0)
				if run != nil {
					queuedRunID = run.ID
				}
				jobID, err = h.enqueueImport(c.Request.Context(), imp.ID, queuedRunID)
				if err != nil {
					api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "入队失败")
					return
				}
			}
			api.Created(c, gin.H{"session_id": req.SessionID, "session_document_id": link.ID, "document_id": existing.ID, "import_job_id": imp.ID, "run_id": importRunID(run), "job_id": jobID, "file_hash": hash, "reused": true})
			return
		}
		api.Fail(c, http.StatusConflict, api.ErrConflict, "该文件已导入过（ID: "+strconv.FormatInt(existing.ID, 10)+"），请勿重复上传")
		return
	}

	mime := req.MimeType
	if mime == "" {
		mime = detectMime(req.Filename)
	}
	// bank_id 未指定时 fallback 到用户默认题库或第一个题库
	bankID := req.BankID
	if bankID == nil {
		bankID = h.resolveDefaultBank(c.Request.Context(), uid)
	}
	storedName := uuid.NewString() + filepath.Ext(security.SanitizeFileName(req.Filename))
	origDir := filepath.Join(h.cfg.Upload.Root, "originals")
	_ = os.MkdirAll(origDir, 0o755)
	finalPath := filepath.Join(origDir, storedName)
	if err := os.Rename(mergedPath, finalPath); err != nil {
		// 跨设备回退：复制
		if err := copyFile(mergedPath, finalPath); err != nil {
			api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存文件失败")
			return
		}
		_ = os.Remove(mergedPath)
	}

	doc := &domain.Document{
		UserID:       uid,
		BankID:       bankID,
		OriginalName: security.SanitizeFileName(req.Filename),
		StoredName:   storedName,
		FilePath:     finalPath,
		FileSize:     size,
		FileHash:     hash,
		MimeType:     mime,
		Status:       "uploaded",
	}
	created, err := h.repo.CreateDocument(c.Request.Context(), doc)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建文档记录失败")
		return
	}
	// 创建导入任务
	var imp *domain.ImportJob
	var sessionDocumentID, runID int64
	if req.SessionID > 0 {
		link, sessionJob, run, attachErr := h.attachDocument(c, req.SessionID, uid, created.ID, req.DeferProcessing)
		if attachErr != nil {
			api.FailDetail(c, http.StatusInternalServerError, api.ErrInternal, "文档加入会话失败", attachErr.Error())
			return
		}
		imp, sessionDocumentID, runID = sessionJob, link.ID, importRunID(run)
	} else {
		imp, err = h.repo.CreateImportJob(c.Request.Context(), uid, created.ID, bankID)
	}
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建导入任务失败")
		return
	}
	// 入队 worker
	var jobID int64
	if !req.DeferProcessing || runID > 0 {
		jobID, err = h.enqueueImport(c.Request.Context(), imp.ID, runID)
		if err != nil {
			api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "入队失败")
			return
		}
	}
	// 清理分片
	_ = os.RemoveAll(filepath.Join(h.cfg.Upload.Root, "uploads", uploadID))

	api.Created(c, gin.H{
		"document_id":         created.ID,
		"import_job_id":       imp.ID,
		"job_id":              jobID,
		"file_hash":           hash,
		"session_id":          req.SessionID,
		"session_document_id": sessionDocumentID,
		"run_id":              runID,
	})
}

func (h *UploadHandler) attachDocument(c *gin.Context, sessionID, userID, documentID int64, deferProcessing bool) (*domain.ImportSessionDocument, *domain.ImportJob, *domain.ImportRun, error) {
	if deferProcessing {
		return h.repo.AttachDocumentToSessionDeferred(c.Request.Context(), sessionID, userID, documentID)
	}
	return h.repo.AttachDocumentToSession(c.Request.Context(), sessionID, userID, documentID)
}

func importRunID(run *domain.ImportRun) int64 {
	if run == nil {
		return 0
	}
	return run.ID
}

func (h *UploadHandler) enqueueImport(ctx context.Context, importJobID int64, runIDs ...int64) (int64, error) {
	payload := map[string]interface{}{"import_job_id": importJobID}
	if len(runIDs) > 0 && runIDs[0] > 0 {
		payload["run_id"] = runIDs[0]
	}
	jobID, err := h.repo.EnqueueJob(ctx, "import_pdf", payload, 0)
	if err != nil {
		return 0, err
	}
	return jobID, nil
}

// resolveDefaultBank 未指定题库时 fallback：用户默认题库 → 第一个题库。
func (h *UploadHandler) resolveDefaultBank(ctx context.Context, userID int64) *int64 {
	user, err := h.repo.GetUserByID(ctx, userID)
	if err == nil && user.DefaultBankID > 0 {
		return &user.DefaultBankID
	}
	banks, _, err := h.repo.ListBanks(ctx, 1, 1)
	if err == nil && len(banks) > 0 {
		id := banks[0].ID
		return &id
	}
	return nil
}

func mergeChunks(dir, mergedPath string, chunkCount int) error {
	dst, err := os.Create(mergedPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	for i := 0; i < chunkCount; i++ {
		chunkPath := filepath.Join(dir, fmt.Sprintf("chunk_%05d", i))
		src, err := os.Open(chunkPath)
		if err != nil {
			return fmt.Errorf("缺少分片 %d", i)
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			return err
		}
		src.Close()
	}
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func detectMime(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

func allowedMime(allowed []string, mime string) bool {
	for _, a := range allowed {
		if a == mime {
			return true
		}
	}
	return false
}
