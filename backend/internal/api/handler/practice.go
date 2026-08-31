package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// PracticeHandler 刷题接口。
type PracticeHandler struct {
	svc  *service.PracticeService
	repo *sqlite.Repository
}

func NewPracticeHandler(svc *service.PracticeService, repo *sqlite.Repository) *PracticeHandler {
	return &PracticeHandler{svc: svc, repo: repo}
}

func (h *PracticeHandler) CreateSession(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req service.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	session, err := h.svc.CreateSession(c.Request.Context(), uid, req)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建会话失败")
		return
	}
	api.Created(c, session)
}

func (h *PracticeHandler) Preview(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req service.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	preview, err := h.svc.Preview(c.Request.Context(), uid, req)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取刷题预览失败")
		return
	}
	api.OK(c, preview)
}

func (h *PracticeHandler) GetSession(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	session, err := h.svc.GetSession(c.Request.Context(), uid, id)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "会话不存在")
		return
	}
	api.OK(c, session)
}

func (h *PracticeHandler) GetCurrentQuestion(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	q, session, err := h.svc.GetCurrentQuestion(c.Request.Context(), uid, id)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取题目失败")
		return
	}
	api.OK(c, gin.H{"question": q, "session": session})
}

func (h *PracticeHandler) Answer(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	var req service.AnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	result, err := h.svc.Answer(c.Request.Context(), uid, &id, req)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "判题失败")
		return
	}
	api.OK(c, result)
}

// AnswerWithoutSession 无会话直接答题（错题复习等场景）。
func (h *PracticeHandler) AnswerStandalone(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req service.AnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	result, err := h.svc.Answer(c.Request.Context(), uid, nil, req)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "判题失败")
		return
	}
	api.OK(c, result)
}

func (h *PracticeHandler) Next(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	session, err := h.svc.Next(c.Request.Context(), uid, id)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "切换题目失败")
		return
	}
	api.OK(c, session)
}

func (h *PracticeHandler) Finish(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	session, err := h.svc.Finish(c.Request.Context(), uid, id)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "结束会话失败")
		return
	}
	api.OK(c, session)
}
