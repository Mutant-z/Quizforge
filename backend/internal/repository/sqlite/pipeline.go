package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/quiztrace/quiztrace/internal/parser"
)

// RecordNodeRun 记录 pipeline 节点运行（幂等 + 可观测）。
func (r *Repository) RecordNodeRun(ctx context.Context, candidateID int64, importJobID *int64, nodeName, nodeVersion, provider, model, promptVersion string, inputTokens, outputTokens, latencyMs int, status string) error {
	inputHash := fmt.Sprintf("%s:%s:%d", nodeName, nodeVersion, candidateID)
	// 幂等：相同输入已成功则跳过
	var c int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pipeline_node_runs WHERE input_hash = ? AND node_name = ? AND status = 'success'`,
		inputHash, nodeName).Scan(&c)
	if err == nil && c > 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO pipeline_node_runs (candidate_id, import_job_id, node_name, node_version, input_hash, status, provider, model, prompt_version, latency_ms, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		candidateID, importJobID, nodeName, nodeVersion, inputHash, status, provider, model, promptVersion, latencyMs, inputTokens, outputTokens)
	if err != nil {
		return fmt.Errorf("record node run: %w", err)
	}
	return nil
}

// FindQuestionBySourceHash 按归一化题干 hash 查已有题。
func (r *Repository) FindQuestionBySourceHash(ctx context.Context, hash string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM canonical_questions WHERE source_hash = ? LIMIT 1`, hash).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// CreateConflict 创建题目冲突。
func (r *Repository) CreateConflict(ctx context.Context, questionID int64, field, sourceA, valueA, valueB string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO question_conflicts (canonical_question_id, source_a, field, value_a, value_b, status)
		VALUES (?, ?, ?, ?, ?, 'open')`,
		questionID, truncateStr(sourceA, 500), field, truncateStr(valueA, 500), truncateStr(valueB, 500))
	if err != nil {
		return fmt.Errorf("create conflict: %w", err)
	}
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ListCandidatesAdmin 管理端候选列表。
func (r *Repository) ListCandidatesAdmin(ctx context.Context, status string, page, pageSize int) ([]*parser.Candidate, int64, error) {
	where := "1=1"
	args := []interface{}{}
	if status != "" {
		where = "status = ?"
		args = append(args, status)
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, import_job_id, page_start, page_end, raw_text, normalized_text, input_hash, status, confidence, extracted_json, error_code, retry_count
		FROM candidate_questions WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	cands := []*parser.Candidate{}
	for rows.Next() {
		var c parser.Candidate
		var jobIDNull sql.NullInt64
		var conf sql.NullFloat64
		var extracted, errCode sql.NullString
		if err := rows.Scan(&c.ID, &c.DocumentID, &jobIDNull, &c.PageStart, &c.PageEnd, &c.RawText, &c.NormalizedText,
			&c.InputHash, &c.Status, &conf, &extracted, &errCode, &c.RetryCount); err != nil {
			return nil, 0, err
		}
		if jobIDNull.Valid {
			c.ImportJobID = &jobIDNull.Int64
		}
		c.Confidence = conf.Float64
		c.ExtractedJSON = extracted.String
		c.ErrorCode = errCode.String
		cands = append(cands, &c)
	}
	return cands, total, rows.Err()
}

// ListConflicts 冲突列表。
func (r *Repository) ListConflicts(ctx context.Context, status string, page, pageSize int) ([]map[string]interface{}, int64, error) {
	where := "1=1"
	args := []interface{}{}
	if status != "" {
		where = "status = ?"
		args = append(args, status)
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM question_conflicts WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, canonical_question_id, source_a, field, value_a, value_b, status, created_at
		FROM question_conflicts WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, qid int64
		var sourceA, field, valueA, valueB, status string
		var createdAt string
		if err := rows.Scan(&id, &qid, &sourceA, &field, &valueA, &valueB, &status, &createdAt); err != nil {
			return nil, 0, err
		}
		out = append(out, map[string]interface{}{
			"id": id, "canonical_question_id": qid, "source_a": sourceA, "field": field,
			"value_a": valueA, "value_b": valueB, "status": status, "created_at": createdAt,
		})
	}
	return out, total, rows.Err()
}

// ResolveConflict 解决冲突。
func (r *Repository) ResolveConflict(ctx context.Context, id int64, resolvedBy int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE question_conflicts SET status = 'resolved', resolved_by = ? WHERE id = ?`, resolvedBy, id)
	return err
}

var _ = time.Now
