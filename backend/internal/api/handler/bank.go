package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// BankHandler 题库 / 科目 / 章节接口。
type BankHandler struct {
	svc  *service.QuestionService
	repo *sqlite.Repository
}

func NewBankHandler(svc *service.QuestionService, repo *sqlite.Repository) *BankHandler {
	return &BankHandler{svc: svc, repo: repo}
}

func (h *BankHandler) List(c *gin.Context) {
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	banks, total, err := h.repo.ListBanksFiltered(c.Request.Context(), page, pageSize, c.Query("q"))
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询题库失败")
		return
	}
	api.PageOK(c, banks, total, page, pageSize)
}

func (h *BankHandler) Get(c *gin.Context) {
	id := parseID(c.Param("id"))
	bank, err := h.repo.GetBank(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题库不存在")
		return
	}
	api.OK(c, bank)
}

type createBankRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

func (h *BankHandler) Create(c *gin.Context) {
	var req createBankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	uid := middleware.CurrentUserID(c)
	bank, err := h.svc.CreateBank(c.Request.Context(), req.Name, req.Description, req.Visibility, uid)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建题库失败")
		return
	}
	api.Created(c, bank)
}

func (h *BankHandler) Delete(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.svc.DeleteBank(c.Request.Context(), id); err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除题库失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

func (h *BankHandler) ListSubjects(c *gin.Context) {
	bankID := parseID(c.Param("id"))
	subjects, err := h.repo.ListSubjects(c.Request.Context(), bankID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询科目失败")
		return
	}
	api.OK(c, subjects)
}

type createSubjectRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *BankHandler) CreateSubject(c *gin.Context) {
	bankID := parseID(c.Param("id"))
	var req createSubjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	sub, err := h.svc.CreateSubject(c.Request.Context(), bankID, req.Name)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建科目失败")
		return
	}
	api.Created(c, sub)
}

func (h *BankHandler) ListChapters(c *gin.Context) {
	subjectID := parseID(c.Param("subject_id"))
	tree, err := h.repo.ChapterTree(c.Request.Context(), subjectID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询章节失败")
		return
	}
	api.OK(c, tree)
}

func (h *BankHandler) GetChapterQuestions(c *gin.Context) {
	chapterID := parseID(c.Param("chapter_id"))
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	questions, total, err := h.repo.ListQuestions(c.Request.Context(), sqlite.QuestionFilter{ChapterID: &chapterID, Status: domain.QuestionStatusPublished, Page: page, PageSize: pageSize})
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询题目失败")
		return
	}
	api.PageOK(c, questions, total, page, pageSize)
}

func (h *BankHandler) CreateChapter(c *gin.Context) {
	subjectID := parseID(c.Param("subject_id"))
	var req struct {
		Name      string `json:"name" binding:"required"`
		ParentID  *int64 `json:"parent_id"`
		Level     int    `json:"level"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	ch, err := h.svc.CreateChapter(c.Request.Context(), subjectID, req.ParentID, req.Name, req.Level, req.SortOrder)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建章节失败")
		return
	}
	api.Created(c, ch)
}

func (h *BankHandler) UpdateChapter(c *gin.Context) {
	id := parseID(c.Param("id"))
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	ch, err := h.repo.UpdateChapter(c.Request.Context(), id, req.Name)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新章节失败")
		return
	}
	api.OK(c, ch)
}

func (h *BankHandler) DeleteChapter(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.repo.DeleteChapter(c.Request.Context(), id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除章节失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

func (h *BankHandler) GetSubject(c *gin.Context) {
	id := parseID(c.Param("subject_id"))
	sub, err := h.repo.GetSubject(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "科目不存在")
		return
	}
	api.OK(c, sub)
}

func parseID(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

func parseInt(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
