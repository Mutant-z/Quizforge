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

func isAdmin(c *gin.Context) bool {
	role, exists := c.Get("role")
	return exists && role == "admin"
}

// accessibleBank 统一收口题库访问范围：管理员可以管理全部题库，学习者只能访问自己创建的题库。
func (h *BankHandler) accessibleBank(c *gin.Context, id int64) (*domain.QuestionBank, error) {
	if isAdmin(c) {
		return h.repo.GetBank(c.Request.Context(), id)
	}
	return h.repo.GetBankForUser(c.Request.Context(), id, middleware.CurrentUserID(c))
}

func (h *BankHandler) accessibleSubject(c *gin.Context, id int64) (*domain.Subject, error) {
	if isAdmin(c) {
		return h.repo.GetSubject(c.Request.Context(), id)
	}
	return h.repo.GetSubjectForUser(c.Request.Context(), id, middleware.CurrentUserID(c))
}

func (h *BankHandler) accessibleChapter(c *gin.Context, id int64) (*domain.Chapter, error) {
	if isAdmin(c) {
		return h.repo.GetChapter(c.Request.Context(), id)
	}
	return h.repo.GetChapterForUser(c.Request.Context(), id, middleware.CurrentUserID(c))
}

func (h *BankHandler) List(c *gin.Context) {
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	var banks []*domain.QuestionBank
	var total int64
	var err error
	if isAdmin(c) {
		banks, total, err = h.repo.ListBanksFiltered(c.Request.Context(), page, pageSize, c.Query("q"))
	} else {
		banks, total, err = h.repo.ListBanksForUser(c.Request.Context(), middleware.CurrentUserID(c), page, pageSize, c.Query("q"))
	}
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询题库失败")
		return
	}
	api.PageOK(c, banks, total, page, pageSize)
}

func (h *BankHandler) Get(c *gin.Context) {
	id := parseID(c.Param("id"))
	bank, err := h.accessibleBank(c, id)
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
	// 普通学习者的题库始终是个人空间，不接受客户端把它创建成全站可见。
	if !isAdmin(c) {
		req.Visibility = "private"
	}
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
	var err error
	if isAdmin(c) {
		err = h.svc.DeleteBank(c.Request.Context(), id)
	} else {
		err = h.svc.DeleteBankForUser(c.Request.Context(), id, middleware.CurrentUserID(c))
	}
	if err != nil {
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
	if _, err := h.accessibleBank(c, bankID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题库不存在")
		return
	}
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
	if _, err := h.accessibleBank(c, bankID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题库不存在")
		return
	}
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
	if _, err := h.accessibleSubject(c, subjectID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "科目不存在")
		return
	}
	tree, err := h.repo.ChapterTree(c.Request.Context(), subjectID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询章节失败")
		return
	}
	api.OK(c, tree)
}

func (h *BankHandler) GetChapterQuestions(c *gin.Context) {
	chapterID := parseID(c.Param("chapter_id"))
	if _, err := h.accessibleChapter(c, chapterID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "章节不存在")
		return
	}
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
	if _, err := h.accessibleSubject(c, subjectID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "科目不存在")
		return
	}
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
	if req.ParentID != nil {
		parent, err := h.accessibleChapter(c, *req.ParentID)
		if err != nil || parent.SubjectID != subjectID {
			api.Fail(c, http.StatusNotFound, api.ErrNotFound, "父章节不存在")
			return
		}
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
	if _, err := h.accessibleChapter(c, id); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "章节不存在")
		return
	}
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
	if _, err := h.accessibleChapter(c, id); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "章节不存在")
		return
	}
	if err := h.repo.DeleteChapter(c.Request.Context(), id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除章节失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

func (h *BankHandler) GetSubject(c *gin.Context) {
	id := parseID(c.Param("subject_id"))
	sub, err := h.accessibleSubject(c, id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "科目不存在")
		return
	}
	api.OK(c, sub)
}

// ListBankChapters 兼容旧版按题库 ID 请求章节的接口。
func (h *BankHandler) ListBankChapters(c *gin.Context) {
	bankID := parseID(c.Param("id"))
	if _, err := h.accessibleBank(c, bankID); err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题库不存在")
		return
	}
	subjects, err := h.repo.ListSubjects(c.Request.Context(), bankID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询科目失败")
		return
	}
	if len(subjects) == 0 {
		api.OK(c, []*domain.Chapter{})
		return
	}
	tree, err := h.repo.ChapterTree(c.Request.Context(), subjects[0].ID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询章节失败")
		return
	}
	api.OK(c, tree)
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
