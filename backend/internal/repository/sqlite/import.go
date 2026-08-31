package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/parser"
)

// ---- Documents ----

// CreateDocument 写入文档（file_hash 唯一 → 幂等）。
func (r *Repository) CreateDocument(ctx context.Context, d *domain.Document) (*domain.Document, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO documents (user_id, bank_id, original_name, stored_name, file_path, file_size, file_hash, mime_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.UserID, d.BankID, d.OriginalName, d.StoredName, d.FilePath, d.FileSize, d.FileHash, d.MimeType)
	if err != nil {
		return nil, fmt.Errorf("insert document: %w", err)
	}
	d.ID, _ = res.LastInsertId()
	return d, nil
}

func (r *Repository) GetDocumentByHash(ctx context.Context, fileHash string) (*domain.Document, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, bank_id, original_name, stored_name, file_path, file_size, file_hash, mime_type, page_count, is_scanned, status, created_at
		FROM documents WHERE file_hash = ?`, fileHash)
	return scanDocument(row)
}

func (r *Repository) GetDocument(ctx context.Context, id int64) (*domain.Document, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, bank_id, original_name, stored_name, file_path, file_size, file_hash, mime_type, page_count, is_scanned, status, created_at
		FROM documents WHERE id = ?`, id)
	return scanDocument(row)
}

func (r *Repository) ListDocuments(ctx context.Context, userID int64, page, pageSize int) ([]*domain.Document, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, bank_id, original_name, stored_name, file_path, file_size, file_hash, mime_type, page_count, is_scanned, status, created_at
		FROM documents WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	docs := []*domain.Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, 0, err
		}
		docs = append(docs, d)
	}
	return docs, total, rows.Err()
}

func scanDocument(row interface{ Scan(...any) error }) (*domain.Document, error) {
	var d domain.Document
	var bankID sql.NullInt64
	var pageCount sql.NullInt64
	var isScanned int
	var createdAt string
	if err := row.Scan(&d.ID, &d.UserID, &bankID, &d.OriginalName, &d.StoredName, &d.FilePath, &d.FileSize,
		&d.FileHash, &d.MimeType, &pageCount, &isScanned, &d.Status, &createdAt); err != nil {
		return nil, err
	}
	if bankID.Valid {
		d.BankID = &bankID.Int64
	}
	d.PageCount = int(pageCount.Int64)
	d.IsScanned = isScanned == 1
	d.CreatedAt = parseSQLiteTime(createdAt)
	return &d, nil
}

// SaveDocumentPages 保存分页文本。
func (r *Repository) SaveDocumentPages(ctx context.Context, documentID int64, pages []parser.Page) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pages {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_pages (document_id, page_no, text, text_len, needs_ocr, text_source, ocr_status, ocr_confidence, ocr_boxes_json)
			VALUES (?, ?, ?, ?, ?, COALESCE(NULLIF(?,''),'text_layer'), COALESCE(NULLIF(?,''),CASE WHEN ?=1 THEN 'required' ELSE 'not_required' END), NULLIF(?,0), NULLIF(?,''))
			ON CONFLICT(document_id,page_no) DO UPDATE SET text=excluded.text,text_len=excluded.text_len,needs_ocr=excluded.needs_ocr,text_source=excluded.text_source,ocr_status=excluded.ocr_status,ocr_confidence=excluded.ocr_confidence,ocr_boxes_json=excluded.ocr_boxes_json`,
			documentID, p.PageNo, p.Text, len(p.Text), boolToInt(p.NeedsOCR), p.TextSource, p.OCRStatus, boolToInt(p.NeedsOCR), p.OCRConfidence, p.OCRBoxesJSON); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET is_scanned=CASE WHEN (SELECT COUNT(*) FROM document_pages WHERE document_id=? AND (needs_ocr=1 OR text_source IN ('ocr','none') OR ocr_status IN ('required','failed'))) >= MAX(1, (SELECT COUNT(*) FROM document_pages WHERE document_id=?)*0.2) THEN 1 ELSE 0 END WHERE id=?`, documentID, documentID, documentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpdateDocumentPages(ctx context.Context, documentID int64, pageCount int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE documents SET page_count = ?, status = 'parsed' WHERE id = ?`, pageCount, documentID)
	return err
}

// GetDocumentPages 取分页。
func (r *Repository) GetDocumentPages(ctx context.Context, documentID int64) ([]parser.Page, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT page_no, COALESCE(text,''), needs_ocr, COALESCE(text_source,''), COALESCE(ocr_status,''), COALESCE(ocr_confidence,0), COALESCE(ocr_boxes_json,'') FROM document_pages WHERE document_id = ? ORDER BY page_no`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []parser.Page{}
	for rows.Next() {
		var p parser.Page
		var needsOCR int
		if err := rows.Scan(&p.PageNo, &p.Text, &needsOCR, &p.TextSource, &p.OCRStatus, &p.OCRConfidence, &p.OCRBoxesJSON); err != nil {
			return nil, err
		}
		p.NeedsOCR = needsOCR == 1
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func (r *Repository) UpdateDocumentPageOCR(ctx context.Context, documentID, pageNo int64, text, status, providerName, version string, confidence float64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE document_pages SET text=?,text_len=?,needs_ocr=CASE WHEN ?='success' THEN 0 ELSE 1 END,text_source=CASE WHEN ?='success' THEN 'ocr' ELSE 'none' END,ocr_status=?,ocr_confidence=?,ocr_provider=?,ocr_version=? WHERE document_id=? AND page_no=?`, text, len(text), status, status, status, confidence, nullableString(providerName), nullableString(version), documentID, pageNo)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE documents SET is_scanned=CASE WHEN (SELECT COUNT(*) FROM document_pages WHERE document_id=? AND (needs_ocr=1 OR text_source IN ('ocr','none') OR ocr_status IN ('required','failed'))) >= MAX(1, (SELECT COUNT(*) FROM document_pages WHERE document_id=?)*0.2) THEN 1 ELSE 0 END WHERE id=?`, documentID, documentID, documentID)
	return err
}

// ---- Import Jobs ----

// CreateImportJob 创建导入任务。
func (r *Repository) CreateImportJob(ctx context.Context, userID, documentID int64, bankID *int64) (*domain.ImportJob, error) {
	return r.CreateImportJobForSession(ctx, userID, documentID, bankID, nil)
}

func (r *Repository) CreateImportJobForSession(ctx context.Context, userID, documentID int64, bankID *int64, sessionID *int64) (*domain.ImportJob, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO import_jobs (user_id, bank_id, document_id, status, stage, pipeline_version, session_id) VALUES (?, ?, ?, 'uploaded', 'uploaded', ?, ?)`,
		userID, bankID, documentID, domain.VisionPipelineVersionV11, sessionID)
	if err != nil {
		return nil, fmt.Errorf("insert import job: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetImportJob(ctx, id)
}

func (r *Repository) GetImportJob(ctx context.Context, id int64) (*domain.ImportJob, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, bank_id, document_id, status, stage, total_pages, processed_pages, candidate_count,
		       parsed_count, new_count, duplicate_count, conflict_count, failed_count, review_count, progress,
		       error_code, error_message, created_at, updated_at,
		       pipeline_version, draft_version, directory_complete, expected_count, extracted_count, complete_count,
		       total_units, completed_units, failed_units, blocking_issue_count, provider_id_snapshot, model_snapshot, session_id
		FROM import_jobs WHERE id = ?`, id)
	return scanImportJob(row)
}

func (r *Repository) ListImportJobs(ctx context.Context, userID int64, page, pageSize int) ([]*domain.ImportJob, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_jobs WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, bank_id, document_id, status, stage, total_pages, processed_pages, candidate_count,
		       parsed_count, new_count, duplicate_count, conflict_count, failed_count, review_count, progress,
		       error_code, error_message, created_at, updated_at,
		       pipeline_version, draft_version, directory_complete, expected_count, extracted_count, complete_count,
		       total_units, completed_units, failed_units, blocking_issue_count, provider_id_snapshot, model_snapshot, session_id
		FROM import_jobs WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	jobs := []*domain.ImportJob{}
	for rows.Next() {
		j, err := scanImportJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}

func scanImportJob(row interface{ Scan(...any) error }) (*domain.ImportJob, error) {
	var j domain.ImportJob
	var bankID sql.NullInt64
	var stage, errCode, errMsg sql.NullString
	var createdAt, updatedAt string
	var directoryComplete int
	var expectedCount, providerID, sessionID sql.NullInt64
	var modelSnapshot sql.NullString
	if err := row.Scan(&j.ID, &j.UserID, &bankID, &j.DocumentID, &j.Status, &stage, &j.TotalPages, &j.ProcessedPages,
		&j.CandidateCount, &j.ParsedCount, &j.NewCount, &j.DuplicateCount, &j.ConflictCount, &j.FailedCount, &j.ReviewCount,
		&j.Progress, &errCode, &errMsg, &createdAt, &updatedAt,
		&j.PipelineVersion, &j.DraftVersion, &directoryComplete, &expectedCount, &j.ExtractedCount, &j.CompleteCount,
		&j.TotalUnits, &j.CompletedUnits, &j.FailedUnits, &j.BlockingIssueCount, &providerID, &modelSnapshot, &sessionID); err != nil {
		return nil, err
	}
	if bankID.Valid {
		j.BankID = &bankID.Int64
	}
	j.Stage = stage.String
	j.ErrorCode = errCode.String
	j.ErrorMessage = errMsg.String
	j.CreatedAt = parseSQLiteTime(createdAt)
	j.UpdatedAt = parseSQLiteTime(updatedAt)
	j.DirectoryComplete = directoryComplete == 1
	if expectedCount.Valid {
		v := int(expectedCount.Int64)
		j.ExpectedCount = &v
	}
	if providerID.Valid {
		v := providerID.Int64
		j.ProviderIDSnapshot = &v
	}
	j.ModelSnapshot = modelSnapshot.String
	if sessionID.Valid {
		v := sessionID.Int64
		j.SessionID = &v
	}
	return &j, nil
}

func (r *Repository) UpdateImportStatus(ctx context.Context, id int64, status, stage string, progress float64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_jobs SET status = ?, stage = ?, progress = ?, updated_at = datetime('now') WHERE id = ? AND status <> 'cancelled'`,
		status, stage, progress, id)
	return err
}

func (r *Repository) UpdateImportStage(ctx context.Context, id int64, stage, message string, progress float64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_jobs SET status = ?, stage = ?, progress = ?, updated_at = datetime('now') WHERE id = ? AND status <> 'cancelled'`,
		stage, stage, progress, id)
	if err != nil {
		return err
	}
	return r.AddImportLog(ctx, id, stage, message)
}

func (r *Repository) UpdateImportProgress(ctx context.Context, id int64, progress float64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_jobs SET progress = ?, updated_at = datetime('now') WHERE id = ? AND status <> 'cancelled'`, progress, id)
	return err
}

func (r *Repository) UpdateImportError(ctx context.Context, id int64, status, code, msg string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_jobs SET status = ?, error_code = ?, error_message = ?, updated_at = datetime('now') WHERE id = ? AND status <> 'cancelled'`,
		status, code, msg, id)
	if err != nil {
		return err
	}
	return r.AddImportLog(ctx, id, status, msg)
}

func (r *Repository) UpdateImportCounts(ctx context.Context, id int64, totalPages, candidateCount, newCount int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_jobs SET total_pages = ?, processed_pages = ?, candidate_count = ?, new_count = ?, updated_at = datetime('now') WHERE id = ?`,
		totalPages, totalPages, candidateCount, newCount, id)
	return err
}

func (r *Repository) UpdateImportCountsFull(ctx context.Context, id int64, newCount, dupCount, conflictCount, reviewCount, failedCount int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE import_jobs SET new_count = ?, duplicate_count = ?, conflict_count = ?, review_count = ?, failed_count = ?, updated_at = datetime('now') WHERE id = ?`,
		newCount, dupCount, conflictCount, reviewCount, failedCount, id)
	return err
}

// GetImportStats 统计导入结果。
type ImportStats struct {
	NewCount       int
	DuplicateCount int
	ConflictCount  int
	ReviewCount    int
	FailedCount    int
}

func (r *Repository) GetImportStats(ctx context.Context, jobID int64) (*ImportStats, error) {
	var s ImportStats
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM candidate_questions WHERE import_job_id = ? GROUP BY status`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var c int
		if err := rows.Scan(&status, &c); err != nil {
			return nil, err
		}
		switch status {
		case "persisted":
			s.NewCount += c
		case "duplicate":
			s.DuplicateCount += c
		case "conflict":
			s.ConflictCount += c
		case "needs_review":
			s.ReviewCount += c
		case "failed":
			s.FailedCount += c
		}
	}
	return &s, rows.Err()
}

func (r *Repository) AddImportLog(ctx context.Context, jobID int64, stage, message string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO import_task_logs (import_job_id, stage, message) VALUES (?, ?, ?)`, jobID, stage, message)
	return err
}

func (r *Repository) ListImportLogs(ctx context.Context, jobID int64) ([]*domain.ImportTaskLog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, import_job_id, stage, message, created_at FROM import_task_logs WHERE import_job_id = ? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []*domain.ImportTaskLog{}
	for rows.Next() {
		var l domain.ImportTaskLog
		var stage sql.NullString
		var createdAt string
		if err := rows.Scan(&l.ID, &l.ImportJobID, &stage, &l.Message, &createdAt); err != nil {
			return nil, err
		}
		l.Stage = stage.String
		l.CreatedAt = parseSQLiteTime(createdAt)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// ---- Candidates ----

// InsertCandidateIfAbsent 幂等写入候选（input_hash 唯一）。返回是否新插入。
func (r *Repository) InsertCandidateIfAbsent(ctx context.Context, c *parser.Candidate) (bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM candidate_questions WHERE document_id = ? AND input_hash = ?`, c.DocumentID, c.InputHash).Scan(&id)
	if err == nil {
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO candidate_questions (document_id, import_job_id, page_start, page_end, raw_text, normalized_text, input_hash, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
		c.DocumentID, c.ImportJobID, c.PageStart, c.PageEnd, c.RawText, c.NormalizedText, c.InputHash)
	if err != nil {
		return false, fmt.Errorf("insert candidate: %w", err)
	}
	c.ID, _ = res.LastInsertId()
	return true, nil
}

func (r *Repository) ListCandidatesByJob(ctx context.Context, jobID int64, status string) ([]*parser.Candidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, import_job_id, page_start, page_end, raw_text, normalized_text, input_hash, status, confidence, extracted_json, error_code, retry_count
		FROM candidate_questions WHERE import_job_id = ? AND status = ? ORDER BY page_start, id`, jobID, status)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		if jobIDNull.Valid {
			c.ImportJobID = &jobIDNull.Int64
		}
		c.Confidence = conf.Float64
		c.ExtractedJSON = extracted.String
		c.ErrorCode = errCode.String
		cands = append(cands, &c)
	}
	return cands, rows.Err()
}

func (r *Repository) GetCandidate(ctx context.Context, id int64) (*parser.Candidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, document_id, import_job_id, page_start, page_end, raw_text, normalized_text, input_hash, status, confidence, extracted_json, error_code, retry_count
		FROM candidate_questions WHERE id = ?`, id)
	var c parser.Candidate
	var jobIDNull sql.NullInt64
	var conf sql.NullFloat64
	var extracted, errCode sql.NullString
	if err := row.Scan(&c.ID, &c.DocumentID, &jobIDNull, &c.PageStart, &c.PageEnd, &c.RawText, &c.NormalizedText,
		&c.InputHash, &c.Status, &conf, &extracted, &errCode, &c.RetryCount); err != nil {
		return nil, err
	}
	if jobIDNull.Valid {
		c.ImportJobID = &jobIDNull.Int64
	}
	c.Confidence = conf.Float64
	c.ExtractedJSON = extracted.String
	c.ErrorCode = errCode.String
	return &c, nil
}

func (r *Repository) SetCandidateExtracted(ctx context.Context, id int64, extractedJSON string, confidence float64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE candidate_questions SET status = 'extracted', extracted_json = ?, confidence = ?, updated_at = datetime('now') WHERE id = ?`,
		extractedJSON, confidence, id)
	return err
}

func (r *Repository) SetCandidateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE candidate_questions SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, id)
	return err
}

func (r *Repository) SetCandidateError(ctx context.Context, id int64, code, msg string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE candidate_questions SET status = 'failed', error_code = ?, error_message = ?, retry_count = retry_count + 1, updated_at = datetime('now') WHERE id = ?`,
		code, msg, id)
	return err
}

// CreateQuestionSource 创建题目来源。
func (r *Repository) CreateQuestionSource(ctx context.Context, qid int64, cand *parser.Candidate, doc *domain.Document, originalAnswer, originalAnalysis string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO question_sources (canonical_question_id, document_id, candidate_id, page_start, page_end, raw_text, original_answer, original_analysis, source_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		qid, doc.ID, cand.ID, cand.PageStart, cand.PageEnd, cand.RawText, originalAnswer, originalAnalysis, cand.InputHash)
	return err
}
