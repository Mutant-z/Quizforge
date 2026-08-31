package sqlite

import (
	"context"
	"fmt"
)

// ReviewConfig 复习配置。
type ReviewConfig struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Scheduler string `json:"scheduler"`
	Params    string `json:"params"`
}

func (r *Repository) GetReviewConfig(ctx context.Context, userID int64) (*ReviewConfig, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, scheduler, params FROM review_configs WHERE user_id = ?`, userID)
	var c ReviewConfig
	if err := row.Scan(&c.ID, &c.UserID, &c.Scheduler, &c.Params); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) UpsertReviewConfig(ctx context.Context, userID int64, scheduler, params string) (*ReviewConfig, error) {
	if params == "" {
		params = "{}"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO review_configs (user_id, scheduler, params) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET scheduler = excluded.scheduler, params = excluded.params, updated_at = datetime('now')`,
		userID, scheduler, params)
	if err != nil {
		return nil, fmt.Errorf("upsert review config: %w", err)
	}
	return r.GetReviewConfig(ctx, userID)
}
