package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// AgentHandler AI 侧栏接口。
type AgentHandler struct {
	svc *service.AgentService
	repo *sqlite.Repository
}

func NewAgentHandler(svc *service.AgentService, repo *sqlite.Repository) *AgentHandler {
	return &AgentHandler{svc: svc, repo: repo}
}

// CreateSession 新建会话。
func (h *AgentHandler) CreateSession(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req struct {
		Title string `json:"title"`
		Context *service.AgentContext `json:"context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	var agctx service.AgentContext
	if req.Context != nil {
		agctx = *req.Context
	}
	session, err := h.svc.CreateSession(c.Request.Context(), uid, req.Title, agctx)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "创建会话失败")
		return
	}
	api.Created(c, session)
}

func (h *AgentHandler) ListSessions(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	sessions, err := h.svc.ListSessions(c.Request.Context(), uid)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询会话失败")
		return
	}
	api.OK(c, sessions)
}

func (h *AgentHandler) GetSession(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	session, messages, err := h.svc.GetSession(c.Request.Context(), uid, id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "会话不存在")
		return
	}
	api.OK(c, gin.H{"session": session, "messages": messages})
}

func (h *AgentHandler) DeleteSession(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	if err := h.svc.DeleteSession(c.Request.Context(), uid, id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除失败")
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

// Stream 消息 + SSE 流式回复。
func (h *AgentHandler) Stream(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	id := parseID(c.Param("id"))
	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	emit := func(evt service.AgentEvent) error {
		data, _ := json.Marshal(evt)
		_, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", evt.Type, data)
		flushSSE(c)
		return err
	}

	// 先发 heartbeat 防超时
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_, _ = fmt.Fprintf(c.Writer, ": heartbeat\n\n")
			flushSSE(c)
		}
	}()

	if err := h.svc.StreamChat(c.Request.Context(), uid, id, req.Message, emit); err != nil {
		_ = emit(service.AgentEvent{Type: "agent.failed", Message: "AI 服务暂时不可用"})
	}
}
