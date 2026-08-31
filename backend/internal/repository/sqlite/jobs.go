package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// EnqueueJob 入队任务。
func (r *Repository) EnqueueJob(ctx context.Context, jobType string, payload interface{}, priority int) (int64, error) {
	payloadJSON, _ := json.Marshal(payload)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs (type, payload, priority) VALUES (?, ?, ?)`,
		jobType, string(payloadJSON), priority)
	if err != nil {
		return 0, fmt.Errorf("enqueue job: %w", err)
	}
	return res.LastInsertId()
}

// reservedJobTypes 由各自专用 worker 消费，通用 ClaimJob（PDF 导入 worker）不领取。
var reservedJobTypes = []string{"wrong_import_extract", "wrong_import_reconcile"}

// ClaimJob 抢占任务（带 lease）。
func (r *Repository) ClaimJob(ctx context.Context, workerID string, leaseSeconds int) (*domain.Job, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 抢占：queued 或 lease 已过期的 running
	var id int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM jobs
		WHERE status IN ('queued', 'running')
		  AND (lease_until IS NULL OR lease_until < datetime('now'))
		  AND type NOT IN (`+placeholders(len(reservedJobTypes))+`)
		ORDER BY priority DESC, id ASC
		LIMIT 1`, typeArgs(reservedJobTypes)...).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE jobs SET status = 'running', attempt = attempt + 1, lease_until = datetime('now', '+' || ? || ' seconds'),
			worker_id = ?, updated_at = datetime('now')
		WHERE id = ?`, leaseSeconds, workerID, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetJob(ctx, id)
}

// ClaimJobOfType 由专用 worker 按类型抢占任务（与其他 pipeline 互不影响）。
func (r *Repository) ClaimJobOfType(ctx context.Context, workerID string, leaseSeconds int, jobType string) (*domain.Job, error) {
	return r.ClaimJobOfTypes(ctx, workerID, leaseSeconds, []string{jobType})
}

// ClaimJobOfTypes 由专用 worker 按多个相关类型抢占任务。
func (r *Repository) ClaimJobOfTypes(ctx context.Context, workerID string, leaseSeconds int, jobTypes []string) (*domain.Job, error) {
	if len(jobTypes) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM jobs
		WHERE status IN ('queued', 'running')
		  AND (lease_until IS NULL OR lease_until < datetime('now'))
		  AND type IN (`+placeholders(len(jobTypes))+`)
		ORDER BY priority DESC, id ASC
		LIMIT 1`, typeArgs(jobTypes)...).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE jobs SET status = 'running', attempt = attempt + 1, lease_until = datetime('now', '+' || ? || ' seconds'),
			worker_id = ?, updated_at = datetime('now')
		WHERE id = ?`, leaseSeconds, workerID, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetJob(ctx, id)
}

func placeholders(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ", "
		}
		out += "?"
	}
	return out
}

func typeArgs(types []string) []interface{} {
	args := make([]interface{}, len(types))
	for i, t := range types {
		args[i] = t
	}
	return args
}

func (r *Repository) GetJob(ctx context.Context, id int64) (*domain.Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, payload, status, attempt, max_attempts, lease_until, worker_id, error_code, error_message, priority, created_at, updated_at
		FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (r *Repository) CompleteJob(ctx context.Context, id int64, success bool, errMsg string) error {
	status := "done"
	if !success {
		status = "failed"
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, error_message = ?, lease_until = NULL, updated_at = datetime('now')
		WHERE id = ? AND status <> 'cancelled'`, status, errMsg, id)
	return err
}

func (r *Repository) RenewJobLease(ctx context.Context, id int64, workerID string, leaseSeconds int) error {
	res, err := r.db.ExecContext(ctx, `UPDATE jobs SET lease_until=datetime('now','+' || ? || ' seconds'),updated_at=datetime('now') WHERE id=? AND worker_id=? AND status='running'`, leaseSeconds, id, workerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("job lease lost")
	}
	return nil
}

// ListJobs 管理端查看任务。
func (r *Repository) ListJobs(ctx context.Context, status, jobType string, page, pageSize int) ([]*domain.Job, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if jobType != "" {
		where = append(where, "type = ?")
		args = append(args, jobType)
	}
	whereSQL := joinWhere(where)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, type, payload, status, attempt, max_attempts, lease_until, worker_id, error_code, error_message, priority, created_at, updated_at
		FROM jobs WHERE `+whereSQL+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	jobs := []*domain.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}

func scanJob(row interface{ Scan(...any) error }) (*domain.Job, error) {
	var j domain.Job
	var leaseUntil sql.NullString
	var workerID, errCode, errMsg sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Attempt, &j.MaxAttempts, &leaseUntil, &workerID, &errCode, &errMsg, &j.Priority, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if leaseUntil.Valid && leaseUntil.String != "" {
		t := parseSQLiteTime(leaseUntil.String)
		j.LeaseUntil = &t
	}
	j.WorkerID = workerID.String
	j.ErrorCode = errCode.String
	j.ErrorMessage = errMsg.String
	j.CreatedAt = parseSQLiteTime(createdAt)
	j.UpdatedAt = parseSQLiteTime(updatedAt)
	return &j, nil
}

func joinWhere(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}

var _ = time.Now
