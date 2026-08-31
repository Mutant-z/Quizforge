package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// StatHandler 统计接口。
type StatHandler struct {
	svc *service.StatService
}

func NewStatHandler(svc *service.StatService) *StatHandler {
	return &StatHandler{svc: svc}
}

func (h *StatHandler) Overview(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	data, err := h.svc.Overview(c.Request.Context(), uid)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取统计失败")
		return
	}
	api.OK(c, data)
}

func (h *StatHandler) Trend(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	days := parseInt(c.Query("days"), 30)
	data, err := h.svc.Trend(c.Request.Context(), uid, days)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取趋势失败")
		return
	}
	api.OK(c, data)
}

func (h *StatHandler) ChapterMastery(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	data, err := h.svc.ChapterMastery(c.Request.Context(), uid)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取章节掌握度失败")
		return
	}
	api.OK(c, data)
}

func (h *StatHandler) SubjectMastery(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	data, err := h.svc.SubjectMastery(c.Request.Context(), uid)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取科目掌握度失败")
		return
	}
	api.OK(c, data)
}

func (h *StatHandler) WrongTrend(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	days := parseInt(c.Query("days"), 30)
	data, err := h.svc.WrongTrend(c.Request.Context(), uid, days)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取错题趋势失败")
		return
	}
	api.OK(c, data)
}

func (h *StatHandler) WeakKnowledgePoints(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	data, err := h.svc.WeakKnowledgePoints(c.Request.Context(), uid, 10)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "获取薄弱知识点失败")
		return
	}
	api.OK(c, data)
}

// ReviewConfigHandler 复习配置。
type ReviewConfigHandler struct {
	repo *sqlite.Repository
}

func NewReviewConfigHandler(repo *sqlite.Repository) *ReviewConfigHandler {
	return &ReviewConfigHandler{repo: repo}
}

func (h *ReviewConfigHandler) GetConfig(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	cfg, err := h.repo.GetReviewConfig(c.Request.Context(), uid)
	if err != nil {
		api.OK(c, gin.H{"scheduler": "simple_v1", "params": map[string]interface{}{}})
		return
	}
	api.OK(c, cfg)
}

func (h *ReviewConfigHandler) UpdateConfig(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req struct {
		Scheduler string `json:"scheduler"`
		Params    string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.Scheduler == "" {
		req.Scheduler = "simple_v1"
	}
	cfg, err := h.repo.UpsertReviewConfig(c.Request.Context(), uid, req.Scheduler, req.Params)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存配置失败")
		return
	}
	api.OK(c, cfg)
}

// RespondJSON 便捷响应（用于 router 中内联处理）。
func RespondJSON(c *gin.Context, data interface{}) {
	api.OK(c, data)
}
