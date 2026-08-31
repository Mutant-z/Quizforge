package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// CreateUser 创建用户。
func (r *Repository) CreateUser(ctx context.Context, username, email, passwordHash string) (*domain.User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		username, email, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetUserByID(ctx, id)
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, username, email, password_hash, role, avatar, bio, default_bank_id, study_preferences, status, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (r *Repository) GetUserByAccount(ctx context.Context, account string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, username, email, password_hash, role, avatar, bio, default_bank_id, study_preferences, status, created_at, updated_at FROM users WHERE username = ? OR email = ?`, account, account)
	return scanUser(row)
}

func (r *Repository) UpdateUser(ctx context.Context, id int64, avatar, bio string, defaultBankID *int64) (*domain.User, error) {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE users SET avatar = ?, bio = ?, default_bank_id = ?, updated_at = datetime('now') WHERE id = ?`,
		avatar, bio, defaultBankID, id); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return r.GetUserByID(ctx, id)
}

func (r *Repository) UpdateSettings(ctx context.Context, id int64, prefs string) (*domain.User, error) {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE users SET study_preferences = ?, updated_at = datetime('now') WHERE id = ?`,
		prefs, id); err != nil {
		return nil, fmt.Errorf("update settings: %w", err)
	}
	return r.GetUserByID(ctx, id)
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var u domain.User
	var avatar, bio sql.NullString
	var defaultBankID sql.NullInt64
	var prefs sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &avatar, &bio, &defaultBankID, &prefs, &u.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	u.Avatar = avatar.String
	u.Bio = bio.String
	u.DefaultBankID = defaultBankID.Int64
	u.StudyPreferences = prefs.String
	if t, e := time.Parse("2006-01-02 15:04:05", createdAt); e == nil {
		u.CreatedAt = t
	}
	if t, e := time.Parse("2006-01-02 15:04:05", updatedAt); e == nil {
		u.UpdatedAt = t
	}
	return &u, nil
}

// RefreshToken 相关

func (r *Repository) StoreRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

func (r *Repository) ValidateRefreshToken(ctx context.Context, userID int64, tokenHash string) (bool, error) {
	var c int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ? AND token_hash = ? AND revoked = 0 AND expires_at > datetime('now')`,
		userID, tokenHash).Scan(&c)
	if err != nil {
		return false, fmt.Errorf("validate refresh token: %w", err)
	}
	return c > 0, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, userID int64, tokenHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked = 1 WHERE user_id = ? AND token_hash = ?`, userID, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

var _ = errors.Is
