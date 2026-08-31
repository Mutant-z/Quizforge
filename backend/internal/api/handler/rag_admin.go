package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// RAGHandler 检索接口。
type RAGHandler struct {
	svc *service.RAGService
}

func NewRAGHandler(svc *service.RAGService) *RAGHandler {
	return &RAGHandler{svc: svc}
}

func (h *RAGHandler) Search(c *gin.Context) {
	var req service.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "缺少检索内容")
		return
	}
	res, err := h.svc.Search(c.Request.Context(), req)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "检索失败")
		return
	}
	api.OK(c, res)
}

// AdminHandler 管理端接口。
type AdminHandler struct {
	repo *sqlite.Repository
	cfg  *config.Config
}

func NewAdminHandler(repo *sqlite.Repository, cfg *config.Config) *AdminHandler {
	return &AdminHandler{repo: repo, cfg: cfg}
}

func (h *AdminHandler) ListCandidates(c *gin.Context) {
	status := c.Query("status")
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	cands, total, err := h.repo.ListCandidatesAdmin(c.Request.Context(), status, page, pageSize)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询候选题失败")
		return
	}
	api.PageOK(c, cands, total, page, pageSize)
}

func (h *AdminHandler) GetCandidate(c *gin.Context) {
	id := parseID(c.Param("id"))
	cand, err := h.repo.GetCandidate(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "候选题不存在")
		return
	}
	api.OK(c, cand)
}

// ApproveCandidate 人工确认候选题（低置信度审核）。
func (h *AdminHandler) ApproveCandidate(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.repo.SetCandidateStatus(c.Request.Context(), id, "approved"); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "操作失败")
		return
	}
	api.OK(c, gin.H{"approved": true})
}

func (h *AdminHandler) RejectCandidate(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.repo.SetCandidateStatus(c.Request.Context(), id, "rejected"); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "操作失败")
		return
	}
	api.OK(c, gin.H{"rejected": true})
}

func (h *AdminHandler) ListConflicts(c *gin.Context) {
	status := c.Query("status")
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	conflicts, total, err := h.repo.ListConflicts(c.Request.Context(), status, page, pageSize)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询冲突失败")
		return
	}
	api.PageOK(c, conflicts, total, page, pageSize)
}

func (h *AdminHandler) ResolveConflict(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.repo.ResolveConflict(c.Request.Context(), id, 0); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "解决冲突失败")
		return
	}
	api.OK(c, gin.H{"resolved": true})
}

// ReindexQuestion 重新生成向量（管理端）。
func (h *AdminHandler) ReindexQuestion(c *gin.Context) {
	id := parseID(c.Param("id"))
	if _, err := h.repo.GetQuestion(c.Request.Context(), id); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题目不存在")
		return
	}
	_, err := h.repo.EnqueueJob(c.Request.Context(), "reindex_question", map[string]int64{"question_id": id}, 0)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "入队失败")
		return
	}
	api.OK(c, gin.H{"reindexing": true})
}

func (h *AdminHandler) ListJobs(c *gin.Context) {
	status := c.Query("status")
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	jobs, total, err := h.repo.ListJobs(c.Request.Context(), status, "", page, pageSize)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询任务失败")
		return
	}
	api.PageOK(c, jobs, total, page, pageSize)
}
