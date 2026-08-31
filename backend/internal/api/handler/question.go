package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// QuestionHandler 题目接口（查询、题库创建者编辑、收藏 + 笔记）。
type QuestionHandler struct {
	repo *sqlite.Repository
	svc  *service.QuestionService
}

func NewQuestionHandler(repo *sqlite.Repository, svc *service.QuestionService) *QuestionHandler {
	return &QuestionHandler{repo: repo, svc: svc}
}

func (h *QuestionHandler) Get(c *gin.Context) {
	id := parseID(c.Param("id"))
	q, err := h.repo.GetQuestion(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题目不存在")
		return
	}
	api.OK(c, q)
}

// Update 允许题库创建者编辑自己的题目，管理员也可以编辑全部题目。
// 题库详情页面向普通学习账号，因此不能要求调用管理端接口。
func (h *QuestionHandler) Update(c *gin.Context) {
	id := parseID(c.Param("id"))
	existing, err := h.repo.GetQuestion(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题目不存在")
		return
	}

	bank, err := h.repo.GetBank(c.Request.Context(), existing.BankID)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "题库不存在")
		return
	}
	role, _ := c.Get("role")
	if role != "admin" && bank.CreatedBy != middleware.CurrentUserID(c) {
		api.Fail(c, http.StatusForbidden, api.ErrUnauthorized, "只有题库创建者可以修改题目")
		return
	}

	var q domain.Question
	if err := c.ShouldBindJSON(&q); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	// 不允许通过编辑请求把题目挪到其他题库；前端也无需传递内部 source_hash。
	q.ID = id
	q.BankID = existing.BankID
	q.SourceHash = existing.SourceHash
	if q.Status == "" {
		q.Status = existing.Status
	}
	if err := h.svc.UpdateQuestion(c.Request.Context(), id, &q); err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新题目失败")
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *QuestionHandler) Search(c *gin.Context) {
	q := c.Query("q")
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	status := c.Query("status")
	filter := sqlite.QuestionFilter{
		Keyword:  q,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	}
	if bankID := c.Query("bank_id"); bankID != "" {
		id := parseID(bankID)
		filter.BankID = &id
	}
	if subjectID := c.Query("subject_id"); subjectID != "" {
		id := parseID(subjectID)
		filter.SubjectID = &id
	}
	if chapterID := c.Query("chapter_id"); chapterID != "" {
		id := parseID(chapterID)
		filter.ChapterID = &id
	}
	if qType := c.Query("type"); qType != "" {
		filter.Type = qType
	}
	questions, total, err := h.repo.ListQuestions(c.Request.Context(), filter)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "搜索失败")
		return
	}
	api.PageOK(c, questions, total, page, pageSize)
}

func (h *QuestionHandler) ToggleFavorite(c *gin.Context) {
	id := parseID(c.Param("id"))
	uid := middleware.CurrentUserID(c)
	var req struct {
		Favorite *bool `json:"favorite"`
	}
	_ = c.ShouldBindJSON(&req)
	var err error
	if req.Favorite != nil && !*req.Favorite {
		err = h.repo.RemoveFavorite(c.Request.Context(), uid, id)
	} else {
		err = h.repo.AddFavorite(c.Request.Context(), uid, id)
	}
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "操作失败")
		return
	}
	api.OK(c, gin.H{"favorite": true})
}

// Note 相关

func (h *QuestionHandler) GetNote(c *gin.Context) {
	id := parseID(c.Param("id"))
	uid := middleware.CurrentUserID(c)
	note, err := h.repo.GetNote(c.Request.Context(), uid, id)
	if err != nil {
		api.OK(c, gin.H{"content": "", "question_id": id})
		return
	}
	api.OK(c, note)
}

func (h *QuestionHandler) PutNote(c *gin.Context) {
	id := parseID(c.Param("id"))
	uid := middleware.CurrentUserID(c)
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	note, err := h.repo.UpsertNote(c.Request.Context(), uid, id, req.Content)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存笔记失败")
		return
	}
	api.OK(c, note)
}

func (h *QuestionHandler) DeleteNote(c *gin.Context) {
	id := parseID(c.Param("id"))
	uid := middleware.CurrentUserID(c)
	if err := h.repo.DeleteNote(c.Request.Context(), uid, id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除笔记失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

// AdminQuestionHandler 管理端题目 CRUD。
type AdminQuestionHandler struct {
	svc  *service.QuestionService
	repo *sqlite.Repository
}

func NewAdminQuestionHandler(svc *service.QuestionService, repo *sqlite.Repository) *AdminQuestionHandler {
	return &AdminQuestionHandler{svc: svc, repo: repo}
}

func (h *AdminQuestionHandler) List(c *gin.Context) {
	// 复用 question search
	q := c.Query("q")
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	filter := sqlite.QuestionFilter{Keyword: q, Page: page, PageSize: pageSize}
	if bankID := c.Query("bank_id"); bankID != "" {
		id := parseID(bankID)
		filter.BankID = &id
	}
	if status := c.Query("status"); status != "" {
		filter.Status = status
	}
	questions, total, err := h.repo.ListQuestions(c.Request.Context(), filter)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询题目失败")
		return
	}
	api.PageOK(c, questions, total, page, pageSize)
}

func (h *AdminQuestionHandler) Create(c *gin.Context) {
	var q domain.Question
	if err := c.ShouldBindJSON(&q); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	created, err := h.svc.CreateQuestion(c.Request.Context(), &q)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建题目失败")
		return
	}
	api.Created(c, created)
}

func (h *AdminQuestionHandler) Update(c *gin.Context) {
	id := parseID(c.Param("id"))
	var q domain.Question
	if err := c.ShouldBindJSON(&q); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	q.ID = id
	if err := h.svc.UpdateQuestion(c.Request.Context(), id, &q); err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新题目失败")
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *AdminQuestionHandler) Delete(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.svc.DeleteQuestion(c.Request.Context(), id); err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除题目失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}
