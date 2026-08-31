package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// EnsureWrongImportImageBatch 创建或读取一个上传批次。客户端重试时保持幂等。
func (r *Repository) EnsureWrongImportImageBatch(ctx context.Context, batchID string, sessionID int64, expectedCount int) (*domain.WrongImportImageBatch, error) {
	if strings.TrimSpace(batchID) == "" || expectedCount < 1 {
		return nil, errors.New("invalid wrong import image batch")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_import_image_batches (id, session_id, expected_count)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET expected_count = CASE
			WHEN wrong_import_image_batches.expected_count = excluded.expected_count
			THEN wrong_import_image_batches.expected_count
			ELSE wrong_import_image_batches.expected_count
		END`, batchID, sessionID, expectedCount)
	if err != nil {
		return nil, fmt.Errorf("ensure wrong import image batch: %w", err)
	}
	batch, err := r.GetWrongImportImageBatch(ctx, batchID, sessionID)
	if err != nil {
		return nil, err
	}
	if batch.ExpectedCount != expectedCount {
		return nil, fmt.Errorf("wrong import image batch expected count mismatch: got %d want %d", batch.ExpectedCount, expectedCount)
	}
	return batch, nil
}

func (r *Repository) GetWrongImportImageBatch(ctx context.Context, batchID string, sessionID int64) (*domain.WrongImportImageBatch, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, expected_count, status, created_at, sealed_at, completed_at
		FROM wrong_import_image_batches WHERE id = ? AND session_id = ?`, batchID, sessionID)
	return scanWrongImportImageBatch(row)
}

func (r *Repository) ListWrongImportImageBatches(ctx context.Context, sessionID int64) ([]*domain.WrongImportImageBatch, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, expected_count, status, created_at, sealed_at, completed_at
		FROM wrong_import_image_batches WHERE session_id = ? ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*domain.WrongImportImageBatch, 0)
	for rows.Next() {
		item, err := scanWrongImportImageBatch(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWrongImportImageBatch(row interface{ Scan(...any) error }) (*domain.WrongImportImageBatch, error) {
	var batch domain.WrongImportImageBatch
	var created, sealed, completed sql.NullString
	if err := row.Scan(&batch.ID, &batch.SessionID, &batch.ExpectedCount, &batch.Status, &created, &sealed, &completed); err != nil {
		return nil, err
	}
	batch.CreatedAt = parseSQLiteTime(created.String)
	if sealed.Valid && sealed.String != "" {
		v := parseSQLiteTime(sealed.String)
		batch.SealedAt = &v
	}
	if completed.Valid && completed.String != "" {
		v := parseSQLiteTime(completed.String)
		batch.CompletedAt = &v
	}
	return &batch, nil
}

// SealWrongImportImageBatch 标记批次上传结束。重复调用不会重复入队。
func (r *Repository) SealWrongImportImageBatch(ctx context.Context, batchID string, sessionID int64, expectedCount int) (*domain.WrongImportImageBatch, error) {
	var actual int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wrong_import_images WHERE session_id = ? AND batch_id = ?`, sessionID, batchID).Scan(&actual); err != nil {
		return nil, err
	}
	if actual != expectedCount {
		return nil, fmt.Errorf("wrong import image batch incomplete: got %d images want %d", actual, expectedCount)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_image_batches
		SET status = CASE WHEN status IN ('reconciling','done') THEN status ELSE 'sealed' END,
			sealed_at = COALESCE(sealed_at, datetime('now'))
		WHERE id = ? AND session_id = ?`, batchID, sessionID)
	if err != nil {
		return nil, err
	}
	return r.GetWrongImportImageBatch(ctx, batchID, sessionID)
}

func (r *Repository) ListWrongImportImagesByBatch(ctx context.Context, batchID string, sessionID int64) ([]*domain.WrongImportImage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), batch_index, COALESCE(extraction_mode,'questions'), COALESCE(original_name,''), file_path, file_hash, mime_type, size_bytes,
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(error_code,''), COALESCE(error_message,''),
		       draft_count, created_at, updated_at
		FROM wrong_import_images
		WHERE session_id = ? AND batch_id = ?
		ORDER BY CASE WHEN batch_index IS NULL THEN 1 ELSE 0 END, batch_index, id`, sessionID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportImage
	for rows.Next() {
		img, err := scanWrongImportImage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, img)
	}
	return items, rows.Err()
}

// TryStartWrongImportBatchReconcile 保证一个批次只有一个整理任务在运行。
func (r *Repository) TryStartWrongImportBatchReconcile(ctx context.Context, batchID string, sessionID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_image_batches SET status = 'reconciling'
		WHERE id = ? AND session_id = ? AND status = 'sealed'`, batchID, sessionID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *Repository) FinishWrongImportBatchReconcile(ctx context.Context, batchID string, sessionID int64, failed bool) error {
	status := domain.WrongImportBatchDone
	if failed {
		status = domain.WrongImportBatchFailed
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_image_batches SET status = ?, completed_at = datetime('now')
		WHERE id = ? AND session_id = ?`, status, batchID, sessionID)
	return err
}

// ResetWrongImportImageBatchForRetry 将已完成的图片批次重新置为 sealed，
// 允许答案识别失败或错误匹配后复用原图重试。
func (r *Repository) ResetWrongImportImageBatchForRetry(ctx context.Context, batchID string, sessionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_image_batches
		SET status = 'sealed', completed_at = NULL, sealed_at = COALESCE(sealed_at, datetime('now'))
		WHERE id = ? AND session_id = ? AND status IN ('done','failed')`, batchID, sessionID)
	return err
}

// CreateWrongImportMergeCandidate 写入一次边界判断结果，重复 pair 不重复创建 pending 记录。
func (r *Repository) CreateWrongImportMergeCandidate(ctx context.Context, candidate *domain.WrongImportMergeCandidate) (*domain.WrongImportMergeCandidate, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_import_merge_candidates
			(session_id, batch_id, left_draft_id, right_draft_id, score, reason_code, decision, status, before_json, after_json, target_version_after)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		candidate.SessionID, nullableString(candidate.BatchID), candidate.LeftDraftID, candidate.RightDraftID,
		candidate.Score, candidate.ReasonCode, candidate.Decision, candidate.Status,
		nullableString(candidate.BeforeJSON), nullableString(candidate.AfterJSON), nullableInt(candidate.TargetVersionAfter))
	if err != nil {
		// A previous run may have already created the same state. Return it so
		// reconciliation remains idempotent.
		if existing, getErr := r.GetLatestWrongImportMergeCandidate(ctx, candidate.SessionID, candidate.LeftDraftID, candidate.RightDraftID); getErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create wrong import merge candidate: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetWrongImportMergeCandidate(ctx, id, candidate.SessionID)
}

func (r *Repository) GetWrongImportMergeCandidate(ctx context.Context, id, sessionID int64) (*domain.WrongImportMergeCandidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), left_draft_id, right_draft_id, score, reason_code,
		       decision, status, COALESCE(before_json,''), COALESCE(after_json,''), COALESCE(target_version_after,0),
		       created_at, updated_at
		FROM wrong_import_merge_candidates WHERE id = ? AND session_id = ?`, id, sessionID)
	return scanWrongImportMergeCandidate(row)
}

func (r *Repository) GetLatestWrongImportMergeCandidate(ctx context.Context, sessionID, leftDraftID, rightDraftID int64) (*domain.WrongImportMergeCandidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), left_draft_id, right_draft_id, score, reason_code,
		       decision, status, COALESCE(before_json,''), COALESCE(after_json,''), COALESCE(target_version_after,0),
		       created_at, updated_at
		FROM wrong_import_merge_candidates
		WHERE session_id = ? AND left_draft_id = ? AND right_draft_id = ?
		ORDER BY id DESC LIMIT 1`, sessionID, leftDraftID, rightDraftID)
	return scanWrongImportMergeCandidate(row)
}

func scanWrongImportMergeCandidate(row interface{ Scan(...any) error }) (*domain.WrongImportMergeCandidate, error) {
	var candidate domain.WrongImportMergeCandidate
	var batchID, beforeJSON, afterJSON sql.NullString
	var created, updated string
	if err := row.Scan(&candidate.ID, &candidate.SessionID, &batchID, &candidate.LeftDraftID, &candidate.RightDraftID,
		&candidate.Score, &candidate.ReasonCode, &candidate.Decision, &candidate.Status, &beforeJSON, &afterJSON,
		&candidate.TargetVersionAfter, &created, &updated); err != nil {
		return nil, err
	}
	candidate.BatchID = batchID.String
	candidate.BeforeJSON = beforeJSON.String
	candidate.AfterJSON = afterJSON.String
	candidate.CreatedAt = parseSQLiteTime(created)
	candidate.UpdatedAt = parseSQLiteTime(updated)
	return &candidate, nil
}

func (r *Repository) ListWrongImportMergeCandidates(ctx context.Context, sessionID int64, status string) ([]*domain.WrongImportMergeCandidate, error) {
	query := `
		SELECT id, session_id, COALESCE(batch_id,''), left_draft_id, right_draft_id, score, reason_code,
		       decision, status, COALESCE(before_json,''), COALESCE(after_json,''), COALESCE(target_version_after,0),
		       created_at, updated_at
		FROM wrong_import_merge_candidates WHERE session_id = ?`
	args := []interface{}{sessionID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportMergeCandidate
	for rows.Next() {
		item, err := scanWrongImportMergeCandidate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) UpdateWrongImportMergeCandidateStatus(ctx context.Context, id, sessionID int64, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_merge_candidates SET status = ?, updated_at = datetime('now')
		WHERE id = ? AND session_id = ?`, status, id, sessionID)
	return err
}

// ApplyWrongImportMerge 在一个事务中更新目标草稿、隐藏来源草稿并保留来源关系。
func (r *Repository) ApplyWrongImportMerge(ctx context.Context, candidateID int64, apply domain.WrongImportMergeApply) (*domain.WrongImportDraft, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var sessionID int64
	var leftID, rightID int64
	var status string
	if err := tx.QueryRowContext(ctx, `
		SELECT session_id, left_draft_id, right_draft_id, status
		FROM wrong_import_merge_candidates WHERE id = ?`, candidateID).
		Scan(&sessionID, &leftID, &rightID, &status); err != nil {
		return nil, err
	}
	if status != domain.WrongImportMergePending {
		return nil, fmt.Errorf("merge candidate is not pending: %s", status)
	}
	if leftID != apply.TargetID || rightID != apply.SourceID {
		return nil, errors.New("merge candidate pair mismatch")
	}

	var currentVersion int
	if err := tx.QueryRowContext(ctx, `SELECT content_version FROM wrong_import_drafts WHERE id = ? AND merge_status = 'active'`, apply.TargetID).Scan(&currentVersion); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_drafts SET question_type = ?, stem = ?, options_json = ?, answer_json = ?,
			answer_source = ?, answer_status = ?, user_marked_no_answer = ?, analysis = ?, boundary_json = ?, difficulty = ?, knowledge_points_json = ?,
			confidence = ?, warnings_json = ?, merge_status = 'active', merged_into_draft_id = NULL,
			updated_at = datetime('now'), content_version = content_version + 1
		WHERE id = ? AND merge_status = 'active'`,
		apply.Target.QuestionType, apply.Target.Stem, marshalJSON(apply.Target.Options), marshalJSON(apply.Target.Answer),
		apply.Target.AnswerSource, apply.Target.AnswerStatus, boolToInt(apply.Target.UserMarkedNoAnswer), apply.Target.Analysis, marshalJSON(apply.Target.Boundary), apply.Target.Difficulty,
		marshalJSON(apply.Target.KnowledgePoints), apply.Target.Confidence, marshalJSON(apply.Target.Warnings), apply.TargetID)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, errors.New("merge target is no longer active")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM wrong_import_draft_sources WHERE draft_id = ?`, apply.TargetID); err != nil {
		return nil, err
	}
	for _, source := range apply.Target.Sources {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wrong_import_draft_sources (draft_id, image_id, source_order, fragment_role, region_json)
			VALUES (?, ?, ?, ?, ?)`, apply.TargetID, source.ImageID, source.SourceOrder, source.FragmentRole, marshalJSON(source.Region)); err != nil {
			return nil, err
		}
	}
	res, err = tx.ExecContext(ctx, `
		UPDATE wrong_import_drafts SET merge_status = 'merged', merged_into_draft_id = ?,
			updated_at = datetime('now'), content_version = content_version + 1
		WHERE id = ? AND merge_status = 'active'`, apply.TargetID, apply.SourceID)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, errors.New("merge source is no longer active")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_merge_candidates SET status = 'accepted', updated_at = datetime('now')
		WHERE id = ? AND status = 'pending'`, candidateID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_merge_candidates SET target_version_after = ?, after_json = ?
		WHERE id = ?`, currentVersion+1, marshalJSON(apply), candidateID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	_ = sessionID
	return r.GetWrongImportDraft(ctx, apply.TargetID)
}

// RepairWrongImportMerge 重新写入已经接受的合并结果，但不改变来源草稿
// 的隐藏状态。只有目标版本仍等于上次合并版本时才允许执行，避免覆盖用户编辑。
func (r *Repository) RepairWrongImportMerge(ctx context.Context, candidateID int64, expectedVersion int, apply domain.WrongImportMergeApply) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var targetID, currentVersion int64
	var status string
	if err := tx.QueryRowContext(ctx, `
		SELECT left_draft_id, status, target_version_after
		FROM wrong_import_merge_candidates WHERE id = ?`, candidateID).
		Scan(&targetID, &status, &currentVersion); err != nil {
		return err
	}
	if status != domain.WrongImportMergeAccepted || targetID != apply.TargetID || int(currentVersion) != expectedVersion {
		return errors.New("merge target was edited after merge")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_drafts SET question_type = ?, stem = ?, options_json = ?, answer_json = ?,
			answer_source = ?, answer_status = ?, user_marked_no_answer = ?, analysis = ?, boundary_json = ?, difficulty = ?, knowledge_points_json = ?,
			confidence = ?, warnings_json = ?, review_status = ?, updated_at = datetime('now'),
			content_version = content_version + 1
		WHERE id = ? AND merge_status = 'active' AND content_version = ?`,
		apply.Target.QuestionType, apply.Target.Stem, marshalJSON(apply.Target.Options), marshalJSON(apply.Target.Answer),
		apply.Target.AnswerSource, apply.Target.AnswerStatus, boolToInt(apply.Target.UserMarkedNoAnswer), apply.Target.Analysis, marshalJSON(apply.Target.Boundary), apply.Target.Difficulty,
		marshalJSON(apply.Target.KnowledgePoints), apply.Target.Confidence, marshalJSON(apply.Target.Warnings),
		apply.Target.ReviewStatus, apply.TargetID, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New("merge target was edited after merge")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wrong_import_draft_sources WHERE draft_id = ?`, apply.TargetID); err != nil {
		return err
	}
	for _, source := range apply.Target.Sources {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wrong_import_draft_sources (draft_id, image_id, source_order, fragment_role, region_json)
			VALUES (?, ?, ?, ?, ?)`, apply.TargetID, source.ImageID, source.SourceOrder, source.FragmentRole, marshalJSON(source.Region)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_merge_candidates SET after_json = ?, target_version_after = ?, updated_at = datetime('now')
		WHERE id = ? AND status = 'accepted'`, marshalJSON(apply), expectedVersion+1, candidateID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// UndoWrongImportMerge 只在目标草稿版本未被后续修改时恢复快照。
func (r *Repository) UndoWrongImportMerge(ctx context.Context, candidateID int64) error {
	candidate, err := r.getMergeCandidateByID(ctx, candidateID)
	if err != nil {
		return err
	}
	if candidate.Status != domain.WrongImportMergeAccepted {
		return fmt.Errorf("merge candidate is not accepted: %s", candidate.Status)
	}
	var before domain.WrongImportMergeBefore
	if err := json.Unmarshal([]byte(candidate.BeforeJSON), &before); err != nil {
		return fmt.Errorf("decode merge snapshot: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentVersion int
	if err := tx.QueryRowContext(ctx, `SELECT content_version FROM wrong_import_drafts WHERE id = ?`, before.TargetID).Scan(&currentVersion); err != nil {
		return err
	}
	if currentVersion != candidate.TargetVersionAfter {
		return fmt.Errorf("merge target was edited after merge")
	}
	if err := restoreDraftSnapshot(ctx, tx, before.TargetID, before.Target); err != nil {
		return err
	}
	if err := restoreDraftSnapshot(ctx, tx, before.SourceID, before.Source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wrong_import_merge_candidates SET status = 'undone', updated_at = datetime('now') WHERE id = ? AND status = 'accepted'`, candidateID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) getMergeCandidateByID(ctx context.Context, id int64) (*domain.WrongImportMergeCandidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), left_draft_id, right_draft_id, score, reason_code,
		       decision, status, COALESCE(before_json,''), COALESCE(after_json,''), COALESCE(target_version_after,0),
		       created_at, updated_at
		FROM wrong_import_merge_candidates WHERE id = ?`, id)
	return scanWrongImportMergeCandidate(row)
}

func restoreDraftSnapshot(ctx context.Context, tx *sql.Tx, id int64, snapshot domain.WrongImportDraftSnapshot) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_drafts SET question_type = ?, stem = ?, options_json = ?, answer_json = ?,
			answer_source = ?, answer_status = ?, user_marked_no_answer = ?, analysis = ?, boundary_json = ?, difficulty = ?, knowledge_points_json = ?,
			confidence = ?, warnings_json = ?, review_status = ?, merge_status = ?, merged_into_draft_id = ?,
			updated_at = datetime('now'), content_version = content_version + 1
		WHERE id = ?`, snapshot.QuestionType, snapshot.Stem, marshalJSON(snapshot.Options), marshalJSON(snapshot.Answer),
		snapshot.AnswerSource, snapshot.AnswerStatus, boolToInt(snapshot.UserMarkedNoAnswer), snapshot.Analysis, marshalJSON(snapshot.Boundary), snapshot.Difficulty,
		marshalJSON(snapshot.KnowledgePoints), snapshot.Confidence, marshalJSON(snapshot.Warnings), snapshot.ReviewStatus,
		snapshot.MergeStatus, snapshot.MergedIntoDraftID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wrong_import_draft_sources WHERE draft_id = ?`, id); err != nil {
		return err
	}
	for _, source := range snapshot.Sources {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wrong_import_draft_sources (draft_id, image_id, source_order, fragment_role, region_json)
			VALUES (?, ?, ?, ?, ?)`, id, source.ImageID, source.SourceOrder, source.FragmentRole, marshalJSON(source.Region)); err != nil {
			return err
		}
	}
	return nil
}
