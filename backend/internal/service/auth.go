package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

// AuthService 注册 / 登录 / 刷新。
type AuthService struct {
	repo  *sqlite.Repository
	token *security.TokenManager
	crypt *security.AESCrypt
}

func NewAuthService(repo *sqlite.Repository, token *security.TokenManager, crypt *security.AESCrypt) *AuthService {
	return &AuthService{repo: repo, token: token, crypt: crypt}
}

func (s *AuthService) Register(ctx context.Context, username, email, password string) (*domain.User, error) {
	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, api.Internal("密码加密失败", err)
	}
	user, err := s.repo.CreateUser(ctx, username, email, hash)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, api.Conflict("用户名或邮箱已存在")
		}
		return nil, api.Internal("创建用户失败", err)
	}
	return user, nil
}

type AuthResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int64       `json:"expires_in"` // 秒
	User         *domain.User `json:"user"`
}

func (s *AuthService) Login(ctx context.Context, account, password string) (*AuthResponse, error) {
	user, err := s.repo.GetUserByAccount(ctx, account)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, api.Unauthorized("用户名或密码错误")
		}
		return nil, api.Internal("查询用户失败", err)
	}
	if user.Status != "active" {
		return nil, api.Unauthorized("账号已被禁用")
	}
	if !security.CheckPassword(user.PasswordHash, password) {
		return nil, api.Unauthorized("用户名或密码错误")
	}
	return s.issueTokens(ctx, user)
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	claims, err := s.token.Parse(refreshToken)
	if err != nil || claims.TokenType != "refresh" {
		return nil, api.Unauthorized("刷新令牌无效")
	}
	// 校验库中令牌是否存在且未吊销
	hash := sha256.Sum256([]byte(refreshToken))
	ok, err := s.repo.ValidateRefreshToken(ctx, claims.UserID, hex.EncodeToString(hash[:]))
	if err != nil || !ok {
		return nil, api.Unauthorized("刷新令牌已失效，请重新登录")
	}
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, api.Unauthorized("用户不存在")
	}
	// 吊销旧令牌，签发新令牌
	_ = s.repo.RevokeRefreshToken(ctx, claims.UserID, hex.EncodeToString(hash[:]))
	return s.issueTokens(ctx, user)
}

func (s *AuthService) issueTokens(ctx context.Context, user *domain.User) (*AuthResponse, error) {
	access, _, err := s.token.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, api.Internal("签发令牌失败", err)
	}
	refresh, rExp, err := s.token.GenerateRefreshToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, api.Internal("签发刷新令牌失败", err)
	}
	h := sha256.Sum256([]byte(refresh))
	if err := s.repo.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(h[:]), rExp); err != nil {
		return nil, api.Internal("保存刷新令牌失败", err)
	}
	return &AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.token.AccessTTL().Seconds()),
		User:         user,
	}, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
