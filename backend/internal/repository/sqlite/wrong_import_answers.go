package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

const wrongImportAnswerFragmentColumns = `
	id, session_id, image_id, COALESCE(batch_id,''), batch_index, fragment_index,
	COALESCE(section_title,''), seq, COALESCE(question_hint,''), COALESCE(answer_json,'[]'),
	COALESCE(analysis,''), confidence, starts_mid_answer, ends_mid_answer, fragment_role,
	status, matched_draft_id, COALESCE(payload_json,'{}'), created_at, updated_at`

func (r *Repository) CreateWrongImportAnswerFragment(ctx context.Context, fragment *domain.WrongImportAnswerFragment) (*domain.WrongImportAnswerFragment, error) {
	payload := fragment.PayloadJSON
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_import_answer_fragments
			(session_id, image_id, batch_id, batch_index, fragment_index, section_title, seq,
			 question_hint, answer_json, analysis, confidence, starts_mid_answer, ends_mid_answer,
			 fragment_role, status, matched_draft_id, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fragment.SessionID, fragment.ImageID, nullableString(fragment.BatchID), nullableIntPtr(fragment.BatchIndex),
		fragment.FragmentIndex, fragment.SectionTitle, fragment.Seq, fragment.QuestionHint,
		marshalJSON(fragment.Answer), fragment.Analysis, fragment.Confidence, boolToInt(fragment.StartsMidAnswer),
		boolToInt(fragment.EndsMidAnswer), fragment.FragmentRole, fragment.Status, nullableInt64Ptr(fragment.MatchedDraftID), payload)
	if err != nil {
		return nil, fmt.Errorf("insert wrong import answer fragment: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetWrongImportAnswerFragment(ctx, id)
}

func (r *Repository) DeleteWrongImportAnswerFragmentsByImage(ctx context.Context, sessionID, imageID int64) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM wrong_import_answer_fragments WHERE session_id = ? AND image_id = ?`, sessionID, imageID)
	return err
}

func (r *Repository) GetWrongImportAnswerFragment(ctx context.Context, id int64) (*domain.WrongImportAnswerFragment, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+wrongImportAnswerFragmentColumns+` FROM wrong_import_answer_fragments WHERE id = ?`, id)
	return scanWrongImportAnswerFragment(row)
}

func (r *Repository) ListWrongImportAnswerFragmentsByBatch(ctx context.Context, sessionID int64, batchID string) ([]*domain.WrongImportAnswerFragment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+wrongImportAnswerFragmentColumns+`
		FROM wrong_import_answer_fragments
		WHERE session_id = ? AND batch_id = ?
		ORDER BY COALESCE(batch_index, 2147483647), image_id, fragment_index, id`, sessionID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWrongImportAnswerFragments(rows)
}

func (r *Repository) ListWrongImportAnswerFragments(ctx context.Context, sessionID int64) ([]*domain.WrongImportAnswerFragment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+wrongImportAnswerFragmentColumns+`
		FROM wrong_import_answer_fragments
		WHERE session_id = ?
		ORDER BY COALESCE(batch_id,''), COALESCE(batch_index, 2147483647), image_id, fragment_index, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWrongImportAnswerFragments(rows)
}

func (r *Repository) UpdateWrongImportAnswerFragmentMatch(ctx context.Context, id int64, status string, draftID *int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_answer_fragments
		SET status = ?, matched_draft_id = ?, updated_at = datetime('now')
		WHERE id = ?`, status, nullableInt64Ptr(draftID), id)
	return err
}

func (r *Repository) RefreshWrongImportAnswerImageDraftCount(ctx context.Context, imageID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_images SET draft_count = (
			SELECT COUNT(DISTINCT matched_draft_id)
			FROM wrong_import_answer_fragments
			WHERE image_id = ? AND status = 'matched' AND matched_draft_id IS NOT NULL
		), updated_at = datetime('now') WHERE id = ?`, imageID, imageID)
	return err
}

func scanWrongImportAnswerFragments(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*domain.WrongImportAnswerFragment, error) {
	var items []*domain.WrongImportAnswerFragment
	for rows.Next() {
		item, err := scanWrongImportAnswerFragment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWrongImportAnswerFragment(row interface{ Scan(...any) error }) (*domain.WrongImportAnswerFragment, error) {
	var item domain.WrongImportAnswerFragment
	var batchIndex, matchedDraft sql.NullInt64
	var answerJSON string
	var starts, ends int
	var payload, created, updated string
	if err := row.Scan(
		&item.ID, &item.SessionID, &item.ImageID, &item.BatchID, &batchIndex, &item.FragmentIndex,
		&item.SectionTitle, &item.Seq, &item.QuestionHint, &answerJSON, &item.Analysis, &item.Confidence,
		&starts, &ends, &item.FragmentRole, &item.Status, &matchedDraft, &payload, &created, &updated,
	); err != nil {
		return nil, err
	}
	if batchIndex.Valid {
		v := int(batchIndex.Int64)
		item.BatchIndex = &v
	}
	if matchedDraft.Valid {
		v := matchedDraft.Int64
		item.MatchedDraftID = &v
	}
	item.StartsMidAnswer = starts != 0
	item.EndsMidAnswer = ends != 0
	item.Answer = parseStringArray(answerJSON)
	item.PayloadJSON = payload
	item.CreatedAt, item.UpdatedAt = parseSQLiteTime(created), parseSQLiteTime(updated)
	return &item, nil
}

// MarshalWrongImportAnswerFragmentPayload keeps callers from duplicating the
// storage representation when the raw answer model output is persisted.
func MarshalWrongImportAnswerFragmentPayload(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
