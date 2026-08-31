package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
	"github.com/quiztrace/quiztrace/internal/service"
)

// AuthHandler 认证接口。
type AuthHandler struct {
	svc   *service.AuthService
	token *security.TokenManager
}

func NewAuthHandler(repo *sqlite.Repository, token *security.TokenManager, crypt *security.AESCrypt) *AuthHandler {
	return &AuthHandler{svc: service.NewAuthService(repo, token, crypt), token: token}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=2,max=32"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法: "+err.Error())
		return
	}
	user, err := h.svc.Register(c.Request.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "注册失败")
		return
	}
	api.Created(c, user)
}

type loginRequest struct {
	Account  string `json:"account" binding:"required"` // username 或 email
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "登录失败")
		return
	}
	api.OK(c, resp)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if ae := api.AsAppError(err); ae != nil {
			api.FailDetail(c, ae.Status, ae.Code, ae.Message, ae.Detail)
			return
		}
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "刷新失败")
		return
	}
	api.OK(c, resp)
}

// UserHandler 用户接口。
type UserHandler struct {
	repo *sqlite.Repository
}

func NewUserHandler(repo *sqlite.Repository) *UserHandler {
	return &UserHandler{repo: repo}
}

func (h *UserHandler) Me(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	user, err := h.repo.GetUserByID(c.Request.Context(), uid)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "用户不存在")
		return
	}
	api.OK(c, toPublicUser(user))
}

type updateMeRequest struct {
	Avatar        string `json:"avatar"`
	Bio           string `json:"bio"`
	DefaultBankID *int64 `json:"default_bank_id"`
}

func (h *UserHandler) UpdateMe(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req updateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.DefaultBankID != nil && !isAdmin(c) {
		if _, err := h.repo.GetBankForUser(c.Request.Context(), *req.DefaultBankID, uid); err != nil {
			api.Fail(c, http.StatusNotFound, api.ErrNotFound, "默认题库不存在")
			return
		}
	}
	user, err := h.repo.UpdateUser(c.Request.Context(), uid, req.Avatar, req.Bio, req.DefaultBankID)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新失败")
		return
	}
	api.OK(c, toPublicUser(user))
}

type updateSettingsRequest struct {
	StudyPreferences string `json:"study_preferences"`
}

func (h *UserHandler) UpdateSettings(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var req updateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	user, err := h.repo.UpdateSettings(c.Request.Context(), uid, req.StudyPreferences)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新失败")
		return
	}
	api.OK(c, toPublicUser(user))
}

func toPublicUser(u *domain.User) gin.H {
	return gin.H{
		"id":                u.ID,
		"username":          u.Username,
		"email":             maskEmail(u.Email),
		"role":              u.Role,
		"avatar":            u.Avatar,
		"bio":               u.Bio,
		"default_bank_id":   u.DefaultBankID,
		"study_preferences": u.StudyPreferences,
		"created_at":        u.CreatedAt,
	}
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	name := parts[0]
	if len(name) > 2 {
		name = name[:2] + "***"
	}
	return name + "@" + parts[1]
}
