package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// ---------- 会话 ----------

func (r *Repository) CreateWrongImportSession(ctx context.Context, userID int64, title *string, targetBankID *int64) (*domain.WrongImportSession, error) {
	if strings.TrimSpace(*title) == "" {
		*title = "错题导入"
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_import_sessions (user_id, title, target_bank_id) VALUES (?, ?, ?)`,
		userID, *title, targetBankID)
	if err != nil {
		return nil, fmt.Errorf("insert wrong import session: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetWrongImportSession(ctx, id)
}

func (r *Repository) GetWrongImportSession(ctx context.Context, id int64) (*domain.WrongImportSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, status, target_bank_id, draft_count, ready_count, missing_answer_count,
		       created_at, updated_at, committed_at
		FROM wrong_import_sessions WHERE id = ?`, id)
	return scanWrongImportSession(row)
}

// GetLatestActiveWrongImportSession 取用户最近一个进行中的会话（侧栏恢复现场）。
func (r *Repository) GetLatestActiveWrongImportSession(ctx context.Context, userID int64) (*domain.WrongImportSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, status, target_bank_id, draft_count, ready_count, missing_answer_count,
		       created_at, updated_at, committed_at
		FROM wrong_import_sessions WHERE user_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1`, userID)
	s, err := scanWrongImportSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *Repository) ListWrongImportSessions(ctx context.Context, userID int64) ([]*domain.WrongImportSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, status, target_bank_id, draft_count, ready_count, missing_answer_count,
		       created_at, updated_at, committed_at
		FROM wrong_import_sessions WHERE user_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportSession
	for rows.Next() {
		s, err := scanWrongImportSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func scanWrongImportSession(row interface{ Scan(...any) error }) (*domain.WrongImportSession, error) {
	var s domain.WrongImportSession
	var bank sql.NullInt64
	var committed sql.NullString
	var created, updated string
	if err := row.Scan(&s.ID, &s.UserID, &s.Title, &s.Status, &bank, &s.DraftCount, &s.ReadyCount,
		&s.MissingAnswerCount, &created, &updated, &committed); err != nil {
		return nil, err
	}
	if bank.Valid {
		v := bank.Int64
		s.TargetBankID = &v
	}
	if committed.Valid {
		t := parseSQLiteTime(committed.String)
		s.CommittedAt = &t
	}
	s.CreatedAt, s.UpdatedAt = parseSQLiteTime(created), parseSQLiteTime(updated)
	return &s, nil
}

func (r *Repository) ArchiveWrongImportSession(ctx context.Context, id, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_sessions SET status = 'archived', updated_at = datetime('now') WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// RefreshWrongImportCounters 重算会话草稿计数（触发点：草稿增删改、图片完成、入库）。
func (r *Repository) RefreshWrongImportCounters(ctx context.Context, sessionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_sessions SET
			draft_count = (SELECT COUNT(*) FROM wrong_import_drafts WHERE session_id = ? AND review_status != 'excluded' AND merge_status = 'active'),
			ready_count = (SELECT COUNT(*) FROM wrong_import_drafts WHERE session_id = ? AND review_status = 'ready' AND merge_status = 'active'),
			missing_answer_count = (SELECT COUNT(*) FROM wrong_import_drafts WHERE session_id = ? AND review_status = 'missing_answer' AND merge_status = 'active'),
			updated_at = datetime('now')
		WHERE id = ?`, sessionID, sessionID, sessionID, sessionID)
	return err
}

func (r *Repository) MarkWrongImportSessionCommitted(ctx context.Context, sessionID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_sessions SET status = 'committed', committed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, sessionID)
	return err
}

func (r *Repository) SetWrongImportSessionBank(ctx context.Context, sessionID int64, bankID *int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_sessions SET target_bank_id = ?, updated_at = datetime('now') WHERE id = ?`, bankID, sessionID)
	return err
}

// ---------- 图片 ----------

func (r *Repository) CreateWrongImportImage(ctx context.Context, img *domain.WrongImportImage) (*domain.WrongImportImage, error) {
	mode := img.ExtractionMode
	if mode != domain.WrongImportExtractionAnswerKey && mode != domain.WrongImportExtractionAuto {
		mode = domain.WrongImportExtractionQuestions
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_import_images
			(session_id, batch_id, batch_index, extraction_mode, original_name, file_path, file_hash, mime_type, size_bytes, width, height, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		img.SessionID, nullableString(img.BatchID), nullableIntPtr(img.BatchIndex), mode, img.OriginalName, img.FilePath, img.FileHash, img.MIMEType, img.SizeBytes, img.Width, img.Height, img.Status)
	if err != nil {
		return nil, fmt.Errorf("insert wrong import image: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetWrongImportImage(ctx, id)
}

func (r *Repository) GetWrongImportImage(ctx context.Context, id int64) (*domain.WrongImportImage, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), batch_index, COALESCE(extraction_mode,'questions'), COALESCE(original_name,''), file_path, file_hash, mime_type, size_bytes,
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(error_code,''), COALESCE(error_message,''),
		       draft_count, created_at, updated_at
		FROM wrong_import_images WHERE id = ?`, id)
	return scanWrongImportImage(row)
}

func scanWrongImportImage(row interface{ Scan(...any) error }) (*domain.WrongImportImage, error) {
	var img domain.WrongImportImage
	var batchIndex sql.NullInt64
	var extractionMode string
	var created, updated string
	if err := row.Scan(&img.ID, &img.SessionID, &img.BatchID, &batchIndex, &extractionMode, &img.OriginalName, &img.FilePath, &img.FileHash, &img.MIMEType,
		&img.SizeBytes, &img.Width, &img.Height, &img.Status, &img.ErrorCode, &img.ErrorMessage,
		&img.DraftCount, &created, &updated); err != nil {
		return nil, err
	}
	if batchIndex.Valid {
		v := int(batchIndex.Int64)
		img.BatchIndex = &v
	}
	img.ExtractionMode = extractionMode
	img.CreatedAt, img.UpdatedAt = parseSQLiteTime(created), parseSQLiteTime(updated)
	return &img, nil
}

func (r *Repository) ListWrongImportImages(ctx context.Context, sessionID int64) ([]*domain.WrongImportImage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), batch_index, COALESCE(extraction_mode,'questions'), COALESCE(original_name,''), file_path, file_hash, mime_type, size_bytes,
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(error_code,''), COALESCE(error_message,''),
		       draft_count, created_at, updated_at
		FROM wrong_import_images WHERE session_id = ? ORDER BY id`, sessionID)
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

func (r *Repository) CountWrongImportImages(ctx context.Context, sessionID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wrong_import_images WHERE session_id = ?`, sessionID).Scan(&n)
	return n, err
}

// GetWrongImportImageByHash 获取会话内相同文件的最近一次记录。
func (r *Repository) GetWrongImportImageByHash(ctx context.Context, sessionID int64, hash string) (*domain.WrongImportImage, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, COALESCE(batch_id,''), batch_index, COALESCE(extraction_mode,'questions'), COALESCE(original_name,''), file_path, file_hash, mime_type, size_bytes,
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(error_code,''), COALESCE(error_message,''),
		       draft_count, created_at, updated_at
		FROM wrong_import_images
		WHERE session_id = ? AND file_hash = ?
		ORDER BY id DESC LIMIT 1`, sessionID, hash)
	img, err := scanWrongImportImage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return img, err
}

// WrongImportImageExistsByHash 判断是否应拦截同一图片的再次上传。
// 失败或完成但没有任何识别/回填结果的图片允许重试；处理中和已产生结果的图片仍去重。
func (r *Repository) WrongImportImageExistsByHash(ctx context.Context, sessionID int64, hash string) (bool, error) {
	img, err := r.GetWrongImportImageByHash(ctx, sessionID, hash)
	if err != nil || img == nil {
		return false, err
	}
	if img.Status == domain.WrongImportImageFailed || (img.Status == domain.WrongImportImageDone && img.DraftCount == 0) {
		return false, nil
	}
	return true, nil
}

func (r *Repository) UpdateWrongImportImageStatus(ctx context.Context, id int64, status, errCode, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_images SET status = ?, error_code = ?, error_message = ?, updated_at = datetime('now') WHERE id = ?`,
		status, errCode, errMsg, id)
	return err
}

func (r *Repository) UpdateWrongImportImageExtractionMode(ctx context.Context, imageID int64, mode string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_images SET extraction_mode = ?, updated_at = datetime('now') WHERE id = ?`, mode, imageID)
	return err
}

func (r *Repository) UpdateWrongImportImageDims(ctx context.Context, id int64, width, height int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_images SET width = ?, height = ?, updated_at = datetime('now') WHERE id = ?`, width, height, id)
	return err
}

func (r *Repository) RefreshWrongImportImageDraftCount(ctx context.Context, imageID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_images SET draft_count = (SELECT COUNT(DISTINCT ds.draft_id)
			FROM wrong_import_draft_sources ds
			JOIN wrong_import_drafts d ON d.id = ds.draft_id
			WHERE ds.image_id = ? AND d.merge_status = 'active' AND d.review_status != 'excluded'),
			updated_at = datetime('now') WHERE id = ?`, imageID, imageID)
	return err
}

// SetWrongImportImageDraftCount 记录答案参考图实际成功回填的题目数量。
// 答案图不是题目来源，不写入 wrong_import_draft_sources，因此不能使用刷新来源计数。
func (r *Repository) SetWrongImportImageDraftCount(ctx context.Context, imageID int64, count int) error {
	if count < 0 {
		count = 0
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_images SET draft_count = ?, updated_at = datetime('now') WHERE id = ?`, count, imageID)
	return err
}

// PrepareWrongImportImageForAnswerExtraction 将一张已经按“题目图片”处理过的图片
// 切换为答案参考图并重新排队。只删除尚未被用户编辑的机器草稿，避免覆盖用户修改。
func (r *Repository) PrepareWrongImportImageForAnswerExtraction(ctx context.Context, sessionID, imageID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE wrong_import_images
		SET extraction_mode = 'answer_key', status = 'pending', error_code = NULL,
			error_message = NULL, draft_count = 0, updated_at = datetime('now')
		WHERE id = ? AND session_id = ?`, imageID, sessionID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM wrong_import_drafts
		WHERE session_id = ? AND image_id = ?
		  AND merge_status = 'active'
		  AND content_version = 1
		  AND answer_source <> 'user'`, sessionID, imageID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// DeleteUneditedWrongImportDraftsByImage 清理答案模式重试期间可能由旧任务产生的
// 未编辑机器草稿。它是幂等的，并保留用户编辑过的草稿和已合并草稿。
func (r *Repository) DeleteUneditedWrongImportDraftsByImage(ctx context.Context, sessionID, imageID int64) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM wrong_import_drafts
		WHERE session_id = ? AND image_id = ?
		  AND merge_status = 'active'
		  AND content_version = 1
		  AND answer_source <> 'user'`, sessionID, imageID)
	return err
}

// ---------- 草稿 ----------

func (r *Repository) CreateWrongImportDraft(ctx context.Context, d *domain.WrongImportDraft) (*domain.WrongImportDraft, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO wrong_import_drafts
			(session_id, image_id, seq_no, question_type, stem, options_json, answer_json, answer_source,
			 answer_status, user_marked_no_answer, analysis, difficulty, knowledge_points_json, confidence, warnings_json, region_json, boundary_json, review_status)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM wrong_import_images
			WHERE id = ? AND session_id = ? AND extraction_mode = 'questions'
		)`,
		d.SessionID, d.ImageID, d.SeqNo, d.QuestionType, d.Stem, marshalJSON(d.Options), marshalJSON(d.Answer),
		d.AnswerSource, answerStatusOrMissing(d), boolToInt(d.UserMarkedNoAnswer), d.Analysis, d.Difficulty, marshalJSON(d.KnowledgePoints), d.Confidence,
		marshalJSON(d.Warnings), marshalJSON(d.Region), marshalJSON(d.Boundary), d.ReviewStatus,
		d.ImageID, d.SessionID)
	if err != nil {
		return nil, fmt.Errorf("insert wrong import draft: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, fmt.Errorf("image %d is not in questions extraction mode", d.ImageID)
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO wrong_import_draft_sources
			(draft_id, image_id, source_order, fragment_role, region_json)
		VALUES (?, ?, 0, 'source', ?)`, id, d.ImageID, marshalJSON(d.Region)); err != nil {
		return nil, fmt.Errorf("create wrong import draft source: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetWrongImportDraft(ctx, id)
}

func scanWrongImportDraft(row interface{ Scan(...any) error }) (*domain.WrongImportDraft, error) {
	var d domain.WrongImportDraft
	var optionsJSON, answerJSON, kpJSON, warnJSON, regionJSON, boundaryJSON sql.NullString
	var analysis sql.NullString
	var answerStatus string
	var userMarkedNoAnswer int
	var difficulty sql.NullFloat64
	var duplicate, committedQ, committedW, mergedInto sql.NullInt64
	var contentVersion sql.NullInt64
	var created, updated string
	if err := row.Scan(&d.ID, &d.SessionID, &d.ImageID, &d.SeqNo, &d.QuestionType, &d.Stem, &optionsJSON,
		&answerJSON, &d.AnswerSource, &answerStatus, &userMarkedNoAnswer, &analysis, &difficulty, &kpJSON, &d.Confidence, &warnJSON, &regionJSON,
		&boundaryJSON, &duplicate, &d.ReviewStatus, &d.MergeStatus, &mergedInto, &contentVersion, &committedQ, &committedW, &created, &updated); err != nil {
		return nil, err
	}
	d.Options = parseQuestionOptions(optionsJSON.String)
	d.Answer = parseStringArray(answerJSON.String)
	d.AnswerStatus = answerStatus
	d.UserMarkedNoAnswer = userMarkedNoAnswer != 0
	d.NormalizeWrongImportAnswerState()
	d.KnowledgePoints = parseStringArray(kpJSON.String)
	d.Warnings = parseStringArray(warnJSON.String)
	if analysis.Valid {
		d.Analysis = analysis.String
	}
	if difficulty.Valid {
		v := difficulty.Float64
		d.Difficulty = &v
	}
	if regionJSON.Valid && regionJSON.String != "" {
		var region domain.VisionRegion
		if json.Unmarshal([]byte(regionJSON.String), &region) == nil {
			d.Region = &region
		}
	}
	if boundaryJSON.Valid && boundaryJSON.String != "" {
		var boundary domain.WrongImportBoundary
		if json.Unmarshal([]byte(boundaryJSON.String), &boundary) == nil {
			d.Boundary = &boundary
		}
	}
	if mergedInto.Valid {
		v := mergedInto.Int64
		d.MergedIntoDraftID = &v
	}
	if contentVersion.Valid {
		d.ContentVersion = int(contentVersion.Int64)
	}
	if d.MergeStatus == "" {
		d.MergeStatus = domain.WrongImportMergeStatusActive
	}
	if duplicate.Valid {
		v := duplicate.Int64
		d.DuplicateOfDraftID = &v
	}
	if committedQ.Valid {
		v := committedQ.Int64
		d.CommittedQuestionID = &v
	}
	if committedW.Valid {
		v := committedW.Int64
		d.CommittedWrongID = &v
	}
	d.CreatedAt, d.UpdatedAt = parseSQLiteTime(created), parseSQLiteTime(updated)
	return &d, nil
}

const wrongImportDraftColumns = `
		id, session_id, image_id, seq_no, question_type, stem, options_json, answer_json, answer_source,
		answer_status, user_marked_no_answer, analysis, difficulty, knowledge_points_json, confidence, warnings_json, region_json, boundary_json,
		duplicate_of_draft_id, review_status, merge_status, merged_into_draft_id, content_version,
		committed_question_id, committed_wrong_id, created_at, updated_at`

func (r *Repository) GetWrongImportDraft(ctx context.Context, id int64) (*domain.WrongImportDraft, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+wrongImportDraftColumns+` FROM wrong_import_drafts WHERE id = ?`, id)
	d, err := scanWrongImportDraft(row)
	if err != nil {
		return nil, err
	}
	d.Sources, err = r.listWrongImportDraftSources(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *Repository) ListWrongImportDrafts(ctx context.Context, sessionID int64) ([]*domain.WrongImportDraft, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wrongImportDraftColumns+` FROM wrong_import_drafts d WHERE d.session_id = ? AND d.merge_status = 'active'
		 ORDER BY
		   (SELECT COALESCE(b.created_at, i.created_at) FROM wrong_import_images i
		      LEFT JOIN wrong_import_image_batches b ON b.id = i.batch_id AND b.session_id = i.session_id
		      WHERE i.id = d.image_id),
		   (SELECT COALESCE(i.batch_id, '') FROM wrong_import_images i WHERE i.id = d.image_id),
		   (SELECT COALESCE(i.batch_index, 2147483647) FROM wrong_import_images i WHERE i.id = d.image_id),
		   d.seq_no, d.id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportDraft
	for rows.Next() {
		d, err := scanWrongImportDraft(rows)
		if err != nil {
			return nil, err
		}
		d.Sources, err = r.listWrongImportDraftSources(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// RenumberWrongImportDrafts 按批次创建时间、批次内图片顺序、图片内提取顺序
// 重新编号。没有批次的历史图片退化为 image_id 顺序，保证旧客户端仍可用。
func (r *Repository) RenumberWrongImportDrafts(ctx context.Context, sessionID int64) error {
	images, err := r.ListWrongImportImages(ctx, sessionID)
	if err != nil {
		return err
	}
	batchCreated := map[string]time.Time{}
	rows, err := r.db.QueryContext(ctx, `SELECT id, created_at FROM wrong_import_image_batches WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, created string
		if err := rows.Scan(&id, &created); err != nil {
			rows.Close()
			return err
		}
		batchCreated[id] = parseSQLiteTime(created)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	imageOrder := map[int64]int{}
	sort.SliceStable(images, func(i, j int) bool {
		left, right := images[i], images[j]
		leftBatch, leftBatchOK := batchCreated[left.BatchID]
		rightBatch, rightBatchOK := batchCreated[right.BatchID]
		leftCreated, rightCreated := left.CreatedAt, right.CreatedAt
		if leftBatchOK {
			leftCreated = leftBatch
		}
		if rightBatchOK {
			rightCreated = rightBatch
		}
		if !leftCreated.Equal(rightCreated) {
			return leftCreated.Before(rightCreated)
		}
		if left.BatchID != right.BatchID {
			return left.ID < right.ID
		}
		if left.BatchIndex != nil && right.BatchIndex != nil && *left.BatchIndex != *right.BatchIndex {
			return *left.BatchIndex < *right.BatchIndex
		}
		if left.BatchIndex != nil && right.BatchIndex == nil {
			return true
		}
		if left.BatchIndex == nil && right.BatchIndex != nil {
			return false
		}
		return left.ID < right.ID
	})
	for index, image := range images {
		imageOrder[image.ID] = index
	}
	drafts, err := r.ListWrongImportDrafts(ctx, sessionID)
	if err != nil {
		return err
	}
	sort.SliceStable(drafts, func(i, j int) bool {
		left, right := drafts[i], drafts[j]
		if imageOrder[left.ImageID] != imageOrder[right.ImageID] {
			return imageOrder[left.ImageID] < imageOrder[right.ImageID]
		}
		if left.SeqNo != right.SeqNo {
			return left.SeqNo < right.SeqNo
		}
		return left.ID < right.ID
	})
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, draft := range drafts {
		seq := index + 1
		if draft.SeqNo == seq {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE wrong_import_drafts SET seq_no = ?, updated_at = datetime('now') WHERE id = ? AND session_id = ?`, seq, draft.ID, sessionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) listWrongImportDraftSources(ctx context.Context, draftID int64) ([]domain.WrongImportDraftSource, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, draft_id, image_id, source_order, fragment_role, COALESCE(region_json,'')
		FROM wrong_import_draft_sources WHERE draft_id = ? ORDER BY source_order, id`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.WrongImportDraftSource
	for rows.Next() {
		var source domain.WrongImportDraftSource
		var regionJSON string
		if err := rows.Scan(&source.ID, &source.DraftID, &source.ImageID, &source.SourceOrder, &source.FragmentRole, &regionJSON); err != nil {
			return nil, err
		}
		if regionJSON != "" {
			var region domain.VisionRegion
			if json.Unmarshal([]byte(regionJSON), &region) == nil {
				source.Region = &region
			}
		}
		items = append(items, source)
	}
	return items, rows.Err()
}

// UpdateWrongImportDraft 手动编辑草稿字段；nil 表示不修改。
func (r *Repository) UpdateWrongImportDraft(ctx context.Context, id int64, upd *WrongImportDraftUpdate) (*domain.WrongImportDraft, error) {
	sets := []string{"updated_at = datetime('now')", "content_version = content_version + 1"}
	args := []interface{}{}
	if upd.QuestionType != nil {
		sets = append(sets, "question_type = ?")
		args = append(args, *upd.QuestionType)
	}
	if upd.Stem != nil {
		sets = append(sets, "stem = ?")
		args = append(args, *upd.Stem)
	}
	if upd.Options != nil {
		sets = append(sets, "options_json = ?")
		args = append(args, marshalJSON(upd.Options))
	}
	if upd.Answer != nil {
		sets = append(sets, "answer_json = ?")
		args = append(args, marshalJSON(upd.Answer))
	}
	if upd.AnswerSource != nil {
		sets = append(sets, "answer_source = ?")
		args = append(args, *upd.AnswerSource)
	}
	if upd.AnswerStatus != nil {
		sets = append(sets, "answer_status = ?")
		args = append(args, *upd.AnswerStatus)
	}
	if upd.UserMarkedNoAnswer != nil {
		sets = append(sets, "user_marked_no_answer = ?")
		args = append(args, boolToInt(*upd.UserMarkedNoAnswer))
	}
	if upd.Analysis != nil {
		sets = append(sets, "analysis = ?")
		args = append(args, *upd.Analysis)
	}
	if upd.Boundary != nil {
		sets = append(sets, "boundary_json = ?")
		args = append(args, marshalJSON(upd.Boundary))
	}
	if upd.ReviewStatus != nil {
		sets = append(sets, "review_status = ?")
		args = append(args, *upd.ReviewStatus)
	}
	if upd.DuplicateOf != nil {
		if *upd.DuplicateOf > 0 {
			sets = append(sets, "duplicate_of_draft_id = ?")
			args = append(args, *upd.DuplicateOf)
		} else {
			sets = append(sets, "duplicate_of_draft_id = NULL")
		}
	}
	args = append(args, id)
	if _, err := r.db.ExecContext(ctx,
		`UPDATE wrong_import_drafts SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	return r.GetWrongImportDraft(ctx, id)
}

// WrongImportDraftUpdate PATCH 草稿的可选字段。
type WrongImportDraftUpdate struct {
	QuestionType       *string
	Stem               *string
	Options            *[]domain.QuestionOption
	Answer             *[]string
	AnswerSource       *string
	AnswerStatus       *string
	UserMarkedNoAnswer *bool
	Analysis           *string
	Boundary           *domain.WrongImportBoundary
	ReviewStatus       *string
	// DuplicateOf：>0 设置疑似重复指向，0 清除，nil 不动。
	DuplicateOf *int64
}

func answerStatusOrMissing(d *domain.WrongImportDraft) string {
	if d.AnswerStatus != "" {
		return d.AnswerStatus
	}
	if len(d.Answer) > 0 {
		return domain.WrongImportAnswerStatusProvided
	}
	return domain.WrongImportAnswerStatusMissing
}

func (r *Repository) DeleteWrongImportDraft(ctx context.Context, sessionID, draftID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM wrong_import_drafts WHERE id = ? AND session_id = ?`, draftID, sessionID)
	return err
}

func (r *Repository) MarkWrongImportDraftCommitted(ctx context.Context, draftID, questionID, wrongID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_import_drafts SET review_status = 'committed', committed_question_id = ?, committed_wrong_id = ?,
			updated_at = datetime('now') WHERE id = ?`, questionID, wrongID, draftID)
	return err
}

// ---------- 消息 / 事件 ----------

func (r *Repository) AddWrongImportMessage(ctx context.Context, sessionID int64, role, content, metadataJSON string) (*domain.WrongImportMessage, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO wrong_import_messages (session_id, role, content, metadata_json) VALUES (?, ?, ?, ?)`,
		sessionID, role, content, metadataJSON)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	var m domain.WrongImportMessage
	var created string
	if err := r.db.QueryRowContext(ctx,
		`SELECT id, session_id, role, content, COALESCE(metadata_json,''), created_at FROM wrong_import_messages WHERE id = ?`, id).
		Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MetadataJSON, &created); err != nil {
		return nil, err
	}
	m.CreatedAt = parseSQLiteTime(created)
	return &m, nil
}

func (r *Repository) ListWrongImportMessages(ctx context.Context, sessionID int64, limit int) ([]*domain.WrongImportMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, role, content, COALESCE(metadata_json,''), created_at
		FROM (SELECT * FROM wrong_import_messages WHERE session_id = ? ORDER BY id DESC LIMIT ?)
		ORDER BY id ASC`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportMessage
	for rows.Next() {
		var m domain.WrongImportMessage
		var created string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MetadataJSON, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseSQLiteTime(created)
		items = append(items, &m)
	}
	return items, rows.Err()
}

func (r *Repository) AddWrongImportEvent(ctx context.Context, sessionID int64, eventType string, payload interface{}) {
	payloadJSON := ""
	if payload != nil {
		if raw, err := json.Marshal(payload); err == nil {
			payloadJSON = string(raw)
		}
	}
	_, _ = r.db.ExecContext(ctx,
		`INSERT INTO wrong_import_events (session_id, type, payload_json) VALUES (?, ?, ?)`, sessionID, eventType, payloadJSON)
}

func (r *Repository) ListWrongImportEvents(ctx context.Context, sessionID, afterID int64, limit int) ([]*domain.WrongImportEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, type, COALESCE(payload_json,''), created_at
		FROM wrong_import_events WHERE session_id = ? AND id > ? ORDER BY id ASC LIMIT ?`, sessionID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.WrongImportEvent
	for rows.Next() {
		var e domain.WrongImportEvent
		var created string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Type, &e.PayloadJSON, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseSQLiteTime(created)
		items = append(items, &e)
	}
	return items, rows.Err()
}

// ---------- 个人错题本题库 / 查重 ----------

// EnsureWrongBookBank 取或建用户专属「我的错题本」题库（0019 部分唯一索引保证唯一）。
func (r *Repository) EnsureWrongBookBank(ctx context.Context, userID int64) (*domain.QuestionBank, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM question_banks WHERE kind = 'wrong_book' AND created_by = ? LIMIT 1`, userID).Scan(&id)
	if err == nil {
		return r.GetBank(ctx, id)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO question_banks (name, description, visibility, status, kind, created_by)
		VALUES ('我的错题本', '由错题导入 Agent 自动创建的个人错题题库', 'private', 'active', 'wrong_book', ?)`, userID)
	if err != nil {
		return nil, fmt.Errorf("create wrong book bank: %w", err)
	}
	id, _ = res.LastInsertId()
	return r.GetBank(ctx, id)
}

// FindQuestionByStemHash 按归一化指纹精确查重（导入去重）。
func (r *Repository) FindQuestionByStemHash(ctx context.Context, hash string) (*domain.Question, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM canonical_questions WHERE source_hash = ? ORDER BY id LIMIT 1`, hash).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetQuestion(ctx, id)
}

// SetWrongQuestionSource 标记错题来源（导入后调用）。
func (r *Repository) SetWrongQuestionSource(ctx context.Context, userID, questionID int64, source string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wrong_questions SET source = ? WHERE user_id = ? AND question_id = ?`, source, userID, questionID)
	return err
}

// ScheduleFirstWrongReview 新导入错题按 MVP 策略安排首次复习（10 分钟后到期）。
func (r *Repository) ScheduleFirstWrongReview(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_questions SET next_review_at = datetime('now', '+10 minutes')
		WHERE user_id = ? AND question_id = ? AND next_review_at IS NULL`, userID, questionID)
	return err
}

// parseQuestionOptions 解析选项 JSON（容错）。
func parseQuestionOptions(raw string) []domain.QuestionOption {
	if raw == "" {
		return nil
	}
	var opts []domain.QuestionOption
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return nil
	}
	return opts
}

// parseStringArray 解析字符串数组 JSON（容错）。
func parseStringArray(raw string) []string {
	if raw == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	return arr
}
