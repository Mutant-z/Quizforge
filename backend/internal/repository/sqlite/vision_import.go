package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func (r *Repository) UpsertPageAsset(ctx context.Context, asset *domain.DocumentPageAsset) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO document_page_assets
		(document_id,page_no,render_version,purpose,file_path,mime_type,width,height,sha256,status,error)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(document_id,page_no,render_version,purpose) DO UPDATE SET
		file_path=excluded.file_path,mime_type=excluded.mime_type,width=excluded.width,height=excluded.height,
		sha256=excluded.sha256,status=excluded.status,error=excluded.error,updated_at=datetime('now')`,
		asset.DocumentID, asset.PageNo, asset.RenderVersion, asset.Purpose, asset.FilePath, asset.MIMEType,
		asset.Width, asset.Height, asset.SHA256, asset.Status, nullableString(asset.Error))
	return err
}

func (r *Repository) ListPageAssets(ctx context.Context, documentID int64, purpose string, start, end int) ([]*domain.DocumentPageAsset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,document_id,page_no,render_version,purpose,file_path,mime_type,width,height,sha256,status,COALESCE(error,'')
		FROM document_page_assets WHERE document_id=? AND purpose=? AND page_no BETWEEN ? AND ? ORDER BY page_no`, documentID, purpose, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.DocumentPageAsset{}
	for rows.Next() {
		a := new(domain.DocumentPageAsset)
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.PageNo, &a.RenderVersion, &a.Purpose, &a.FilePath, &a.MIMEType, &a.Width, &a.Height, &a.SHA256, &a.Status, &a.Error); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) ListPageAssetsVersion(ctx context.Context, documentID int64, purpose, renderVersion string, start, end int) ([]*domain.DocumentPageAsset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,document_id,page_no,render_version,purpose,file_path,mime_type,width,height,sha256,status,COALESCE(error,'')
		FROM document_page_assets WHERE document_id=? AND purpose=? AND render_version=? AND page_no BETWEEN ? AND ? ORDER BY page_no`, documentID, purpose, renderVersion, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.DocumentPageAsset{}
	for rows.Next() {
		a := new(domain.DocumentPageAsset)
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.PageNo, &a.RenderVersion, &a.Purpose, &a.FilePath, &a.MIMEType, &a.Width, &a.Height, &a.SHA256, &a.Status, &a.Error); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) GetPageAsset(ctx context.Context, documentID int64, purpose string, page int) (*domain.DocumentPageAsset, error) {
	a := new(domain.DocumentPageAsset)
	err := r.db.QueryRowContext(ctx, `SELECT id,document_id,page_no,render_version,purpose,file_path,mime_type,width,height,sha256,status,COALESCE(error,'')
		FROM document_page_assets WHERE document_id=? AND purpose=? AND page_no=? AND status='ready'
		ORDER BY CASE WHEN render_version LIKE '%v2' THEN 0 ELSE 1 END,id DESC LIMIT 1`, documentID, purpose, page).Scan(&a.ID, &a.DocumentID, &a.PageNo, &a.RenderVersion, &a.Purpose, &a.FilePath, &a.MIMEType, &a.Width, &a.Height, &a.SHA256, &a.Status, &a.Error)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *Repository) UpsertAgentUnit(ctx context.Context, unit *domain.ImportAgentUnit) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO import_agent_units
		(import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,input_hash,pipeline_version,run_id,document_id,queued_at,image_profile)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,COALESCE(?,datetime('now')),?)
		ON CONFLICT(import_job_id,unit_type,unit_key,pipeline_version) DO UPDATE SET
		owned_page_start=excluded.owned_page_start,owned_page_end=excluded.owned_page_end,
		context_page_start=excluded.context_page_start,context_page_end=excluded.context_page_end,input_hash=excluded.input_hash,
		run_id=COALESCE(excluded.run_id,import_agent_units.run_id),document_id=COALESCE(excluded.document_id,import_agent_units.document_id)`,
		unit.ImportJobID, unit.UnitType, unit.UnitKey, unit.OwnedPageStart, unit.OwnedPageEnd,
		unit.ContextPageStart, unit.ContextPageEnd, unit.Status, unit.InputHash, unit.PipelineVersion, unit.RunID, unit.DocumentID, unit.QueuedAt, nullableString(unit.ImageProfile))
	if err != nil {
		return err
	}
	return r.db.QueryRowContext(ctx, `SELECT id,status,attempt,COALESCE(output_json,''),COALESCE(validation_json,''),COALESCE(error_code,''),COALESCE(error_message,'')
		FROM import_agent_units WHERE import_job_id=? AND unit_type=? AND unit_key=? AND pipeline_version=?`,
		unit.ImportJobID, unit.UnitType, unit.UnitKey, unit.PipelineVersion).Scan(&unit.ID, &unit.Status, &unit.Attempt, &unit.OutputJSON, &unit.ValidationJSON, &unit.ErrorCode, &unit.ErrorMessage)
}

func (r *Repository) ListAgentUnits(ctx context.Context, jobID int64, unitType string) ([]*domain.ImportAgentUnit, error) {
	where := "import_job_id=?"
	args := []interface{}{jobID}
	if unitType != "" {
		where += " AND unit_type=?"
		args = append(args, unitType)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,attempt,input_hash,COALESCE(output_json,''),COALESCE(validation_json,''),COALESCE(error_code,''),COALESCE(error_message,''),pipeline_version,run_id,document_id,queued_at,started_at,finished_at,queue_latency_ms,payload_bytes,result_count,COALESCE(image_profile,'') FROM import_agent_units WHERE `+where+` ORDER BY context_page_start,id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportAgentUnit{}
	for rows.Next() {
		u := new(domain.ImportAgentUnit)
		var runID, documentID sql.NullInt64
		var queued, started, finished sql.NullString
		if err := rows.Scan(&u.ID, &u.ImportJobID, &u.UnitType, &u.UnitKey, &u.OwnedPageStart, &u.OwnedPageEnd, &u.ContextPageStart, &u.ContextPageEnd, &u.Status, &u.Attempt, &u.InputHash, &u.OutputJSON, &u.ValidationJSON, &u.ErrorCode, &u.ErrorMessage, &u.PipelineVersion, &runID, &documentID, &queued, &started, &finished, &u.QueueLatencyMS, &u.PayloadBytes, &u.ResultCount, &u.ImageProfile); err != nil {
			return nil, err
		}
		if runID.Valid {
			v := runID.Int64
			u.RunID = &v
		}
		if documentID.Valid {
			v := documentID.Int64
			u.DocumentID = &v
		}
		u.QueuedAt, u.StartedAt, u.FinishedAt = parseTime(queued), parseTime(started), parseTime(finished)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) StartAgentUnit(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='running',attempt=attempt+1,error_code=NULL,error_message=NULL,started_at=datetime('now'),finished_at=NULL,queue_latency_ms=CAST((julianday('now')-julianday(COALESCE(queued_at,'now')))*86400000 AS INTEGER),updated_at=datetime('now') WHERE id=? AND status<>'cancelled'`, id)
	return err
}

func (r *Repository) CompleteAgentUnit(ctx context.Context, id int64, outputJSON, validationJSON string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='completed',output_json=?,validation_json=?,error_code=NULL,error_message=NULL,finished_at=datetime('now'),updated_at=datetime('now') WHERE id=? AND status<>'cancelled'`, outputJSON, validationJSON, id)
	return err
}

func (r *Repository) FailAgentUnit(ctx context.Context, id int64, code, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='failed',error_code=?,error_message=?,finished_at=datetime('now'),updated_at=datetime('now') WHERE id=? AND status<>'cancelled'`, code, message, id)
	return err
}

// FailRunningAgentUnits marks only units that were actively executing when a
// worker was interrupted. Pending units remain resumable on the next worker;
// running units must not remain stuck forever or be shown as live progress.
func (r *Repository) FailRunningAgentUnits(ctx context.Context, jobID int64, code, message string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='failed',error_code=?,error_message=?,lease_until=NULL,finished_at=datetime('now'),updated_at=datetime('now') WHERE import_job_id=? AND status='running'`, code, message, jobID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) SetAgentUnitTelemetry(ctx context.Context, id int64, payloadBytes, resultCount int, imageProfile string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET payload_bytes=?,result_count=?,image_profile=?,updated_at=datetime('now') WHERE id=? AND status<>'cancelled'`, payloadBytes, resultCount, nullableString(imageProfile), id)
	return err
}

func (r *Repository) UpdateImportRunProgress(ctx context.Context, runID int64, stage string, completed, total, failed, eta, concurrency int, degradedReason string) error {
	var etaValue interface{}
	if eta >= 0 {
		etaValue = eta
	}
	_, err := r.db.ExecContext(ctx, `UPDATE import_runs SET current_stage=?,completed_units=?,total_units=?,failed_units=?,last_progress_at=datetime('now'),eta_seconds=?,adaptive_concurrency=?,degraded_reason=?,updated_at=datetime('now') WHERE id=?`, stage, completed, total, failed, etaValue, concurrency, nullableString(degradedReason), runID)
	return err
}

func (r *Repository) RecentSuccessfulUnitMedianMS(ctx context.Context, jobID int64, unitType string, limit int) (int64, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT ar.latency_ms FROM import_agent_runs ar JOIN import_agent_units u ON u.id=ar.unit_id WHERE u.import_job_id=? AND u.unit_type=? AND ar.status='completed' ORDER BY ar.id DESC LIMIT ?`, jobID, unitType, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	values := []int64{}
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return 0, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return 0, nil
	}
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
	return values[len(values)/2], nil
}

func (r *Repository) RetryAgentUnits(ctx context.Context, jobID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := []interface{}{jobID}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='pending',error_code=NULL,error_message=NULL,updated_at=datetime('now') WHERE import_job_id=? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) RetrySessionAgentUnits(ctx context.Context, sessionID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := []interface{}{sessionID}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE import_agent_units SET status='pending',error_code=NULL,error_message=NULL,run_id=(SELECT active_run_id FROM import_sessions WHERE id=?),updated_at=datetime('now') WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) RecordAgentRun(ctx context.Context, unitID int64, role, promptVersion, providerName, model, status string, retry, inputTokens, outputTokens, latencyMs int, rawOutput, parsedOutput, validationError string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO import_agent_runs(unit_id,agent_role,prompt_version,provider_name,model,status,retry_index,input_tokens,output_tokens,latency_ms,raw_output,parsed_output,validation_error) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, unitID, role, promptVersion, providerName, model, status, retry, inputTokens, outputTokens, latencyMs, nullableString(rawOutput), nullableString(parsedOutput), nullableString(validationError))
	return err
}

func (r *Repository) InsertVisionCandidate(ctx context.Context, jobID, documentID int64, entry domain.VisionDirectoryEntry, question domain.VisionQuestion) (int64, error) {
	canonical := domain.CanonicalQuestionType(question.QuestionType)
	if entry.CanonicalQuestionType != "" {
		canonical = entry.CanonicalQuestionType
	}
	question.CanonicalQuestionType = canonical
	if entry.SectionScopeID == "" {
		entry.SectionScopeID = normalizeSectionPath(entry.SectionPath)
	}
	sectionJSON, _ := json.Marshal(entry.SectionPath)
	questionJSON, _ := json.Marshal(question)
	inputHash := hashText(entry.SourceIdentity + string(questionJSON))
	var sessionID, runID sql.NullInt64
	_ = r.db.QueryRowContext(ctx, `SELECT session_id FROM import_jobs WHERE id=?`, jobID).Scan(&sessionID)
	if sessionID.Valid {
		_ = r.db.QueryRowContext(ctx, `SELECT active_run_id FROM import_sessions WHERE id=?`, sessionID.Int64).Scan(&runID)
	}
	// Retries may normalize the same section path differently or return minor
	// OCR edits in the stem. Prefer the immutable directory identity. A physical
	// fallback is allowed only when it is unique; otherwise two question blocks
	// sharing a page and number (for example 简答题/论述题 q1) must remain
	// separate candidates.
	if sessionID.Valid {
		var existingID int64
		existingErr := r.db.QueryRowContext(ctx, `SELECT id FROM candidate_questions WHERE import_session_id=? AND document_id=? AND source_identity=? AND status NOT IN ('persisted','duplicate') ORDER BY id LIMIT 1`, sessionID.Int64, documentID, entry.SourceIdentity).Scan(&existingID)
		if existingErr == nil && existingID > 0 {
			_, _ = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID.Int64, existingID, documentID, "question", question.SourcePageStart, question.SourcePageEnd, string(questionJSON))
			if err := r.updateExistingVisionCandidate(ctx, sessionID.Int64, existingID, runID, entry, question, inputHash); err != nil {
				return 0, err
			}
			if err := r.UpsertQuestionProvenance(ctx, sessionID.Int64, existingID, documentID, entry, question, nullIntPointer(runID)); err != nil {
				return 0, err
			}
			return existingID, nil
		}
		var physicalCount, directoryPhysicalCount int
		_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND document_id=? AND source_question_no=? AND question_page_start=? AND question_page_end=? AND canonical_question_type=? AND section_scope_id=? AND status NOT IN ('persisted','duplicate')`, sessionID.Int64, documentID, entry.SourceQuestionNo, question.SourcePageStart, question.SourcePageEnd, canonical, entry.SectionScopeID).Scan(&physicalCount)
		// A single old candidate is safe to reuse only when the directory itself
		// has one identity at that physical key. If the directory has two
		// subjective blocks with the same qno/page, a physical fallback would
		// steal the first block's row and permanently lose the second one.
		_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_directory_entries WHERE session_id=? AND document_id=? AND status='active' AND content_role='question' AND source_question_no=? AND source_page_start=? AND source_page_end=? AND canonical_question_type=? AND section_scope_id=?`, sessionID.Int64, documentID, entry.SourceQuestionNo, question.SourcePageStart, question.SourcePageEnd, canonical, entry.SectionScopeID).Scan(&directoryPhysicalCount)
		if physicalCount == 1 && directoryPhysicalCount <= 1 {
			existingErr = r.db.QueryRowContext(ctx, `SELECT id FROM candidate_questions WHERE import_session_id=? AND document_id=? AND source_question_no=? AND question_page_start=? AND question_page_end=? AND canonical_question_type=? AND section_scope_id=? AND status NOT IN ('persisted','duplicate') ORDER BY id LIMIT 1`, sessionID.Int64, documentID, entry.SourceQuestionNo, question.SourcePageStart, question.SourcePageEnd, canonical, entry.SectionScopeID).Scan(&existingID)
			if existingErr == nil && existingID > 0 {
				_, _ = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID.Int64, existingID, documentID, "question", question.SourcePageStart, question.SourcePageEnd, string(questionJSON))
				if err := r.updateExistingVisionCandidate(ctx, sessionID.Int64, existingID, runID, entry, question, inputHash); err != nil {
					return 0, err
				}
				if err := r.UpsertQuestionProvenance(ctx, sessionID.Int64, existingID, documentID, entry, question, nullIntPointer(runID)); err != nil {
					return 0, err
				}
				return existingID, nil
			}
		}
	}
	var pipelineVersion string
	_ = r.db.QueryRowContext(ctx, `SELECT pipeline_version FROM import_jobs WHERE id=?`, jobID).Scan(&pipelineVersion)
	if pipelineVersion == "" {
		pipelineVersion = domain.VisionPipelineVersion
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO candidate_questions
		(document_id,import_job_id,page_start,page_end,raw_text,normalized_text,input_hash,status,confidence,extracted_json,
		source_identity,section_path_json,set_no,source_question_no,question_type,canonical_question_type,section_scope_id,question_json,merged_json,
		question_page_start,question_page_end,visual_confidence,validation_issues_json,draft_version,pipeline_version,import_session_id,import_run_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT DO NOTHING`, documentID, jobID, question.SourcePageStart, question.SourcePageEnd, string(questionJSON), question.Stem, inputHash, "extracted", question.VisualConfidence, string(questionJSON), entry.SourceIdentity, string(sectionJSON), entry.SetNo, entry.SourceQuestionNo, question.QuestionType, canonical, entry.SectionScopeID, string(questionJSON), string(questionJSON), question.SourcePageStart, question.SourcePageEnd, question.VisualConfidence, "[]", 1, pipelineVersion, nullableNullInt(sessionID), nullableNullInt(runID))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		_ = r.db.QueryRowContext(ctx, `SELECT id FROM candidate_questions WHERE import_job_id=? AND input_hash=?`, jobID, inputHash).Scan(&id)
	}
	if sessionID.Valid && id > 0 {
		_, _ = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID.Int64, id, documentID, "question", question.SourcePageStart, question.SourcePageEnd, string(questionJSON))
		if err := r.UpsertQuestionProvenance(ctx, sessionID.Int64, id, documentID, entry, question, nullIntPointer(runID)); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func nullIntPointer(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// updateExistingVisionCandidate applies fresh extraction output to the
// existing source identity. Manual review decisions and answer linkage fields
// are deliberately excluded so a retry cannot approve, exclude, or unlink a
// question by accident.
func (r *Repository) updateExistingVisionCandidate(ctx context.Context, sessionID, candidateID int64, runID sql.NullInt64, entry domain.VisionDirectoryEntry, question domain.VisionQuestion, inputHash string) error {
	var before string
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(merged_json,question_json,extracted_json,'{}') FROM candidate_questions WHERE id=? AND import_session_id=?`, candidateID, sessionID).Scan(&before); err != nil {
		return err
	}
	var previous domain.VisionQuestion
	if json.Unmarshal([]byte(before), &previous) == nil {
		if len(question.Answer) == 0 {
			question.Answer = previous.Answer
		}
		if strings.TrimSpace(question.Analysis) == "" {
			question.Analysis = previous.Analysis
		}
	}
	question.SourceIdentity = entry.SourceIdentity
	question.CanonicalQuestionType = domain.CanonicalQuestionType(question.QuestionType)
	afterJSON, _ := json.Marshal(question)
	if _, err := r.db.ExecContext(ctx, `UPDATE candidate_questions SET page_start=?,page_end=?,raw_text=?,normalized_text=?,input_hash=?,status='extracted',confidence=?,extracted_json=?,question_json=?,merged_json=?,question_page_start=?,question_page_end=?,question_type=?,canonical_question_type=?,visual_confidence=?,import_run_id=?,updated_at=datetime('now') WHERE id=? AND import_session_id=?`, question.SourcePageStart, question.SourcePageEnd, string(afterJSON), question.Stem, inputHash, question.VisualConfidence, string(afterJSON), string(afterJSON), string(afterJSON), question.SourcePageStart, question.SourcePageEnd, question.QuestionType, question.CanonicalQuestionType, question.VisualConfidence, nullableNullInt(runID), candidateID, sessionID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE import_sessions SET draft_version=draft_version+1,last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=?`, sessionID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,run_id,target_role,before_json,after_json,change_status,created_at) VALUES(?,?, 'question_extract',?,?, 'applied',datetime('now'))`, candidateID, nullableNullInt(runID), before, string(afterJSON))
	return err
}

func (r *Repository) LinkVisionAnswer(ctx context.Context, candidateID int64, answer domain.VisionAnswer, merged domain.VisionQuestion) error {
	answerJSON, _ := json.Marshal(answer)
	mergedJSON, _ := json.Marshal(merged)
	_, err := r.db.ExecContext(ctx, `UPDATE candidate_questions SET answer_json=?,merged_json=?,extracted_json=?,answer_page_start=?,answer_page_end=?,canonical_question_type=?,answer_link_status='confirmed',answer_link_method='canonical',answer_link_confidence=1,draft_version=draft_version+1,updated_at=datetime('now') WHERE id=?`, string(answerJSON), string(mergedJSON), string(mergedJSON), answer.SourcePageStart, answer.SourcePageEnd, domain.CanonicalQuestionType(merged.QuestionType), candidateID)
	if err != nil {
		return err
	}
	var sessionID, documentID int64
	if err = r.db.QueryRowContext(ctx, `SELECT import_session_id,COALESCE((SELECT ar.document_id FROM candidate_answer_links l JOIN import_answer_records ar ON ar.id=l.answer_record_id WHERE l.candidate_id=candidate_questions.id AND l.status='confirmed' ORDER BY l.id DESC LIMIT 1),document_id) FROM candidate_questions WHERE id=?`, candidateID).Scan(&sessionID, &documentID); err == nil {
		_ = r.UpsertCandidateEvidence(ctx, sessionID, candidateID, documentID, "answer", answer.SourcePageStart, answer.SourcePageEnd, string(answerJSON), 1, nil)
		if strings.TrimSpace(answer.Analysis) != "" {
			_ = r.UpsertCandidateEvidence(ctx, sessionID, candidateID, documentID, "explanation", answer.SourcePageStart, answer.SourcePageEnd, answer.Analysis, 1, nil)
		}
	}
	return nil
}

func (r *Repository) SetVisionCandidateValidation(ctx context.Context, candidateID int64, status string, issues []map[string]interface{}) error {
	payload, _ := json.Marshal(issues)
	_, err := r.db.ExecContext(ctx, `UPDATE candidate_questions SET status=?,validation_issues_json=?,updated_at=datetime('now') WHERE id=?`, status, string(payload), candidateID)
	return err
}

// UpdateVisionCandidateQuestionFromReview applies a validated patch to one
// pending candidate. It intentionally does not touch review_decision or the
// persisted answer link: model re-recognition is not an approval action.
func (r *Repository) UpdateVisionCandidateQuestionFromReview(ctx context.Context, documentID, candidateID, runID int64, question domain.VisionQuestion, issues []map[string]interface{}) error {
	questionJSON, err := json.Marshal(question)
	if err != nil {
		return err
	}
	issuesJSON, _ := json.Marshal(issues)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var before string
	var reviewDecision string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(merged_json,question_json,extracted_json,'{}'),COALESCE(review_decision,'pending') FROM candidate_questions WHERE id=? AND document_id=? AND import_session_id=(SELECT session_id FROM import_runs WHERE id=?) AND EXISTS (SELECT 1 FROM import_sessions s WHERE s.id=candidate_questions.import_session_id AND s.active_run_id=?)`, candidateID, documentID, runID, runID).Scan(&before, &reviewDecision); err != nil {
		return err
	}
	if reviewDecision != domain.ReviewPending {
		return fmt.Errorf("CANDIDATE_REVIEW_DECISION_CHANGED")
	}
	if before == string(questionJSON) {
		_, _ = tx.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,run_id,target_role,before_json,after_json,change_status,created_at) VALUES(?,?, 'question',?,?, 'unchanged',datetime('now'))`, candidateID, runID, before, string(questionJSON))
		if err = tx.Commit(); err != nil {
			return err
		}
		return fmt.Errorf("CANDIDATE_REVIEW_UNCHANGED")
	}
	status := "validated"
	if len(issues) > 0 {
		status = "needs_review"
	}
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET page_start=?,page_end=?,raw_text=?,normalized_text=?,question_type=?,canonical_question_type=?,question_json=?,merged_json=?,extracted_json=?,question_page_start=?,question_page_end=?,visual_confidence=?,validation_issues_json=?,status=?,import_run_id=?,draft_version=draft_version+1,updated_at=datetime('now') WHERE id=? AND document_id=? AND import_session_id=(SELECT session_id FROM import_runs WHERE id=?) AND COALESCE(review_decision,'pending')='pending' AND EXISTS (SELECT 1 FROM import_sessions s WHERE s.id=candidate_questions.import_session_id AND s.active_run_id=?)`, question.SourcePageStart, question.SourcePageEnd, string(questionJSON), question.Stem, question.QuestionType, domain.CanonicalQuestionType(question.QuestionType), string(questionJSON), string(questionJSON), string(questionJSON), question.SourcePageStart, question.SourcePageEnd, question.VisualConfidence, string(issuesJSON), status, runID, candidateID, documentID, runID, runID)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return fmt.Errorf("CANDIDATE_REVIEW_NOT_APPLIED")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET status='needs_attention',last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=(SELECT import_session_id FROM candidate_questions WHERE id=?) AND active_run_id=?`, candidateID, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,run_id,target_role,before_json,after_json,change_status,created_at) VALUES(?,?, 'question',?,?, 'applied',datetime('now'))`, candidateID, runID, before, string(questionJSON)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var sessionID int64
	if err = r.db.QueryRowContext(ctx, `SELECT import_session_id FROM candidate_questions WHERE id=?`, candidateID).Scan(&sessionID); err == nil {
		_ = r.UpsertCandidateEvidence(ctx, sessionID, candidateID, documentID, "question", question.SourcePageStart, question.SourcePageEnd, string(questionJSON), question.VisualConfidence, &runID)
	}
	return nil
}

// UpdateVisionCandidateAnswerFromReview updates only answer evidence. The
// question JSON and manual review decision remain untouched.
func (r *Repository) UpdateVisionCandidateAnswerFromReview(ctx context.Context, documentID, candidateID, runID int64, merged domain.VisionQuestion, pageStart, pageEnd int) error {
	answer := domain.VisionAnswer{SourceIdentity: merged.SourceIdentity, Answer: merged.Answer, Analysis: merged.Analysis, SourcePageStart: pageStart, SourcePageEnd: pageEnd}
	answerJSON, _ := json.Marshal(answer)
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var before, reviewDecision string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(merged_json,question_json,extracted_json,'{}'),COALESCE(review_decision,'pending') FROM candidate_questions WHERE id=? AND document_id=? AND import_session_id=(SELECT session_id FROM import_runs WHERE id=?) AND EXISTS (SELECT 1 FROM import_sessions s WHERE s.id=candidate_questions.import_session_id AND s.active_run_id=?)`, candidateID, documentID, runID, runID).Scan(&before, &reviewDecision); err != nil {
		return err
	}
	if reviewDecision != domain.ReviewPending {
		return fmt.Errorf("CANDIDATE_REVIEW_DECISION_CHANGED")
	}
	if before == string(mergedJSON) {
		_, _ = tx.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,run_id,target_role,before_json,after_json,change_status,created_at) VALUES(?,?, 'answer',?,?, 'unchanged',datetime('now'))`, candidateID, runID, before, string(mergedJSON))
		if err = tx.Commit(); err != nil {
			return err
		}
		return fmt.Errorf("CANDIDATE_REVIEW_UNCHANGED")
	}
	linkStatus := domain.AnswerLinkUnresolved
	var linkConfidence interface{}
	if len(merged.Answer) > 0 {
		linkStatus = domain.AnswerLinkConfirmed
		linkConfidence = 1.0
	}
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET answer_json=?,merged_json=?,extracted_json=?,answer_page_start=?,answer_page_end=?,answer_link_status=?,answer_link_method='candidate_review',answer_link_confidence=?,import_run_id=?,draft_version=draft_version+1,updated_at=datetime('now') WHERE id=? AND document_id=? AND import_session_id=(SELECT session_id FROM import_runs WHERE id=?) AND COALESCE(review_decision,'pending')='pending' AND EXISTS (SELECT 1 FROM import_sessions s WHERE s.id=candidate_questions.import_session_id AND s.active_run_id=?)`, string(answerJSON), string(mergedJSON), string(mergedJSON), pageStart, pageEnd, linkStatus, linkConfidence, runID, candidateID, documentID, runID, runID)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return fmt.Errorf("CANDIDATE_REVIEW_NOT_APPLIED")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET status='needs_attention',last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=(SELECT import_session_id FROM candidate_questions WHERE id=?) AND active_run_id=?`, candidateID, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,run_id,target_role,before_json,after_json,change_status,created_at) VALUES(?,?, 'answer',?,?, 'applied',datetime('now'))`, candidateID, runID, before, string(mergedJSON)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var sessionID int64
	if err = r.db.QueryRowContext(ctx, `SELECT import_session_id FROM candidate_questions WHERE id=?`, candidateID).Scan(&sessionID); err == nil {
		_ = r.UpsertCandidateEvidence(ctx, sessionID, candidateID, documentID, "answer", pageStart, pageEnd, string(answerJSON), 1, &runID)
		if strings.TrimSpace(merged.Analysis) != "" {
			_ = r.UpsertCandidateEvidence(ctx, sessionID, candidateID, documentID, "explanation", pageStart, pageEnd, merged.Analysis, 1, &runID)
		}
	}
	return nil
}

// FinalizeCandidateReview recalculates only the session counters after a
// candidate-level review. It deliberately does not rebuild directory/answer
// projections, so candidates outside the target set keep their exact content,
// answer evidence, and review decision. Individual candidate rows receive a
// revision; the session version advances once for the whole review round.
func (r *Repository) FinalizeCandidateReview(ctx context.Context, sessionID, runID int64, changed bool) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var draftVersion int
	var expected sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT draft_version,expected_count FROM import_sessions WHERE id=? AND active_run_id=? AND archived_at IS NULL`, sessionID, runID).Scan(&draftVersion, &expected); err != nil {
		return 0, err
	}
	var uniqueCount, completeCount, blocking int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate')`, sessionID).Scan(&uniqueCount); err != nil {
		return 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status='validated'`, sessionID).Scan(&completeCount); err != nil {
		return 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND status='open' AND severity='blocking'`, sessionID).Scan(&blocking); err != nil {
		return 0, err
	}
	status := domain.ImportStatusDraftReady
	if !expected.Valid || expected.Int64 != int64(uniqueCount) || uniqueCount != completeCount || blocking > 0 {
		status = domain.ImportStatusNeedsAttention
	}
	nextVersion := draftVersion
	if changed {
		nextVersion++
	}
	res, err := tx.ExecContext(ctx, `UPDATE import_sessions SET status=?,draft_version=?,unique_count=?,complete_count=?,blocking_issue_count=?,last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=? AND active_run_id=? AND archived_at IS NULL`, status, nextVersion, uniqueCount, completeCount, blocking, sessionID, runID)
	if err != nil {
		return 0, err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return 0, fmt.Errorf("CANDIDATE_REVIEW_NOT_CURRENT")
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return nextVersion, nil
}

func (r *Repository) ListVisionDraft(ctx context.Context, jobID int64, page, pageSize int) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_job_id=?`, jobID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,status,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),set_no,source_question_no,COALESCE(question_type,''),COALESCE(merged_json,question_json,extracted_json,'{}'),COALESCE(validation_issues_json,'[]'),COALESCE(visual_confidence,confidence,0),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),COALESCE(answer_page_start,0),COALESCE(answer_page_end,0),draft_version FROM candidate_questions WHERE import_job_id=? ORDER BY question_page_start,source_question_no,id LIMIT ? OFFSET ?`, jobID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var status, identity, section, qtype, merged, issues string
		var setNo, qno, qStart, qEnd, aStart, aEnd, version int
		var conf float64
		if err := rows.Scan(&id, &status, &identity, &section, &setNo, &qno, &qtype, &merged, &issues, &conf, &qStart, &qEnd, &aStart, &aEnd, &version); err != nil {
			return nil, 0, err
		}
		var sectionValue, mergedValue, issuesValue interface{}
		_ = json.Unmarshal([]byte(section), &sectionValue)
		_ = json.Unmarshal([]byte(merged), &mergedValue)
		_ = json.Unmarshal([]byte(issues), &issuesValue)
		out = append(out, map[string]interface{}{"id": id, "status": status, "source_identity": identity, "section_path": sectionValue, "set_no": setNo, "source_question_no": qno, "question_type": qtype, "question": mergedValue, "validation_issues": issuesValue, "visual_confidence": conf, "question_page_start": qStart, "question_page_end": qEnd, "answer_page_start": aStart, "answer_page_end": aEnd, "draft_version": version})
	}
	return out, total, rows.Err()
}

func (r *Repository) UpdateVisionCandidate(ctx context.Context, jobID, candidateID int64, expectedVersion int, mergedJSON string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT draft_version FROM import_jobs WHERE id=?`, jobID).Scan(&current); err != nil {
		return false, err
	}
	if current != expectedVersion {
		return false, nil
	}
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,draft_version=draft_version+1,updated_at=datetime('now') WHERE id=? AND import_job_id=?`, mergedJSON, mergedJSON, candidateID, jobID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET draft_version=draft_version+1,blocking_issue_count=blocking_issue_count+1,status='needs_attention',stage='needs_attention',updated_at=datetime('now') WHERE id=?`, jobID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) AddImportMessage(ctx context.Context, m *domain.ImportMessage) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_messages(import_job_id,role,content,action_json,status,draft_version_before,draft_version_after) VALUES(?,?,?,?,?,?,?)`, m.ImportJobID, m.Role, m.Content, nullableString(m.ActionJSON), m.Status, nullableInt(m.DraftVersionBefore), nullableInt(m.DraftVersionAfter))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) ListImportMessages(ctx context.Context, jobID int64) ([]*domain.ImportMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,import_job_id,role,content,COALESCE(action_json,''),status,COALESCE(draft_version_before,0),COALESCE(draft_version_after,0),created_at FROM import_messages WHERE import_job_id=? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportMessage{}
	for rows.Next() {
		m := new(domain.ImportMessage)
		var created string
		if err := rows.Scan(&m.ID, &m.ImportJobID, &m.Role, &m.Content, &m.ActionJSON, &m.Status, &m.DraftVersionBefore, &m.DraftVersionAfter, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseSQLiteTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateVisionImportMetrics(ctx context.Context, jobID int64, status, stage string, directoryComplete bool, expected *int, extracted, complete, totalUnits, completedUnits, failedUnits, blocking int, model string, progress float64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_jobs SET status=?,stage=?,directory_complete=?,expected_count=?,extracted_count=?,complete_count=?,candidate_count=?,total_units=?,completed_units=?,failed_units=?,blocking_issue_count=?,model_snapshot=?,progress=?,updated_at=datetime('now') WHERE id=? AND status<>'cancelled'`, status, stage, boolToInt(directoryComplete), nullableIntPtr(expected), extracted, complete, extracted, totalUnits, completedUnits, failedUnits, blocking, nullableString(model), progress, jobID)
	return err
}

func (r *Repository) SnapshotVisionProvider(ctx context.Context, jobID, providerID int64, model string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_jobs SET provider_id_snapshot=?,model_snapshot=?,updated_at=datetime('now') WHERE id=?`, providerID, model, jobID)
	return err
}

func (r *Repository) VisionPreview(ctx context.Context, jobID int64) (*domain.ImportPreview, error) {
	job, err := r.GetImportJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	preview := &domain.ImportPreview{DirectoryComplete: job.DirectoryComplete, ExpectedCount: job.ExpectedCount, ExpectedCountReady: job.DirectoryComplete && job.ExpectedCount != nil, ExtractedCount: job.ExtractedCount, UniqueCount: job.CandidateCount, CompleteCount: job.CompleteCount, FailedUnits: job.FailedUnits, BlockingIssueCount: job.BlockingIssueCount, Issues: []map[string]interface{}{}}
	preview.DirectoryCoverageStatus = "unverifiable"
	if job.SessionID != nil {
		_ = r.db.QueryRowContext(ctx, `SELECT COUNT(CASE WHEN status IN ('verified','repaired') THEN 1 END),COUNT(CASE WHEN status='mismatch' THEN 1 END) FROM import_page_coverage WHERE session_id=?`, *job.SessionID).Scan(&preview.VerifiedPageCount, &preview.MismatchPageCount)
		if preview.MismatchPageCount > 0 {
			preview.DirectoryCoverageStatus = "mismatch"
		} else if preview.VerifiedPageCount > 0 {
			preview.DirectoryCoverageStatus = "verified"
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,COALESCE(validation_issues_json,'[]') FROM candidate_questions WHERE import_job_id=? AND validation_issues_json IS NOT NULL AND validation_issues_json!='[]'`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var issues []map[string]interface{}
		if json.Unmarshal([]byte(raw), &issues) == nil {
			for _, item := range issues {
				item["candidate_id"] = id
				preview.Issues = append(preview.Issues, item)
			}
		}
	}
	preview.CanCommit = preview.DirectoryComplete && preview.ExpectedCount != nil && *preview.ExpectedCount > 0 && *preview.ExpectedCount == preview.UniqueCount && preview.UniqueCount == preview.CompleteCount && preview.FailedUnits == 0 && preview.BlockingIssueCount == 0
	return preview, rows.Err()
}

type VisionCommitResult struct {
	Inserted    int     `json:"inserted"`
	Duplicates  int     `json:"duplicates"`
	QuestionIDs []int64 `json:"-"`
}

func (r *Repository) CommitVisionDraft(ctx context.Context, jobID int64, expectedVersion int) (*VisionCommitResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var bankID sql.NullInt64
	var documentID int64
	var currentVersion, dirComplete, expected, unique, complete, failed, blocking int
	if err := tx.QueryRowContext(ctx, `SELECT bank_id,document_id,draft_version,directory_complete,COALESCE(expected_count,0),candidate_count,complete_count,failed_units,blocking_issue_count FROM import_jobs WHERE id=?`, jobID).Scan(&bankID, &documentID, &currentVersion, &dirComplete, &expected, &unique, &complete, &failed, &blocking); err != nil {
		return nil, err
	}
	if currentVersion != expectedVersion {
		return nil, fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	if !bankID.Valid {
		return nil, fmt.Errorf("IMPORT_BANK_REQUIRED")
	}
	if dirComplete != 1 || expected <= 0 || expected != unique || unique != complete || failed != 0 || blocking != 0 {
		return nil, fmt.Errorf("IMPORT_PREVIEW_BLOCKED")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(merged_json,''),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),input_hash FROM candidate_questions WHERE import_job_id=? AND status='validated' ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id         int64
		raw        string
		start, end int
		inputHash  string
	}
	items := []candidate{}
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.raw, &c.start, &c.end, &c.inputHash); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, c)
	}
	rows.Close()
	if len(items) != expected {
		return nil, fmt.Errorf("IMPORT_VALIDATED_COUNT_MISMATCH")
	}
	result := &VisionCommitResult{}
	for _, item := range items {
		var q domain.VisionQuestion
		if err := json.Unmarshal([]byte(item.raw), &q); err != nil {
			return nil, err
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(q.Stem), ""))
		sum := sha256.Sum256([]byte(normalized))
		sourceHash := fmt.Sprintf("%x", sum[:16])
		var existing int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM canonical_questions WHERE source_hash=? LIMIT 1`, sourceHash).Scan(&existing); err == nil {
			if _, err := tx.ExecContext(ctx, `INSERT INTO question_sources(canonical_question_id,document_id,candidate_id,page_start,page_end,raw_text,original_answer,original_analysis,source_hash) VALUES(?,?,?,?,?,?,?,?,?)`, existing, documentID, item.id, item.start, item.end, item.raw, strings.Join(q.Answer, ","), q.Analysis, item.inputHash); err != nil {
				return nil, err
			}
			_, _ = tx.ExecContext(ctx, `UPDATE candidate_questions SET status='duplicate',updated_at=datetime('now') WHERE id=?`, item.id)
			result.Duplicates++
			continue
		} else if err != sql.ErrNoRows {
			return nil, err
		}
		optionsJSON, _ := json.Marshal(q.Options)
		answerJSON, _ := json.Marshal(q.Answer)
		res, err := tx.ExecContext(ctx, `INSERT INTO canonical_questions(bank_id,type,stem,options_json,answer_json,original_analysis,difficulty,knowledge_points_json,quality_score,status,source_hash,version) VALUES(?,?,?,?,?,?,3,'[]',?,'published',?,1)`, bankID.Int64, q.QuestionType, q.Stem, string(optionsJSON), string(answerJSON), q.Analysis, q.VisualConfidence, sourceHash)
		if err != nil {
			return nil, err
		}
		qid, _ := res.LastInsertId()
		result.QuestionIDs = append(result.QuestionIDs, qid)
		if _, err := tx.ExecContext(ctx, `INSERT INTO question_sources(canonical_question_id,document_id,candidate_id,page_start,page_end,raw_text,original_answer,original_analysis,source_hash) VALUES(?,?,?,?,?,?,?,?,?)`, qid, documentID, item.id, item.start, item.end, item.raw, strings.Join(q.Answer, ","), q.Analysis, item.inputHash); err != nil {
			return nil, err
		}
		_, _ = tx.ExecContext(ctx, `UPDATE candidate_questions SET status='persisted',updated_at=datetime('now') WHERE id=?`, item.id)
		result.Inserted++
	}
	if _, err := tx.ExecContext(ctx, `UPDATE question_banks SET question_count=(SELECT COUNT(*) FROM canonical_questions WHERE bank_id=?),updated_at=datetime('now') WHERE id=?`, bankID.Int64, bankID.Int64); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='embedding',stage='embedding',progress=.95,new_count=?,duplicate_count=?,draft_version=draft_version+1,updated_at=datetime('now') WHERE id=?`, result.Inserted, result.Duplicates, jobID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
func nullableInt(value int) interface{} {
	if value == 0 {
		return nil
	}
	return value
}
func nullableIntPtr(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
func hashText(value string) string {
	// input_hash participates in a UNIQUE constraint. The old FNV-like 64-bit
	// hash collided on real PDF questions (for example q1 true/false and q1
	// multiple-choice on the same page), so a valid extraction was silently
	// discarded. Use the existing cryptographic hash convention used by the
	// canonical importer instead.
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

var _ = sql.ErrNoRows
