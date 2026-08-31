package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func (r *Repository) CreateImportSession(ctx context.Context, userID int64, bankID *int64, title string) (*domain.ImportSession, error) {
	if strings.TrimSpace(title) == "" {
		title = "新建导入会话"
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_sessions(user_id,bank_id,title) VALUES(?,?,?)`, userID, bankID, strings.TrimSpace(title))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	_, _ = r.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: id, Role: "assistant", MessageType: "text", Content: "会话已创建。上传题本、答案册或解析册后，我会自动开始识别并形成统一草稿。", Status: "completed"})
	return r.GetImportSession(ctx, id)
}

func (r *Repository) GetImportSession(ctx context.Context, id int64) (*domain.ImportSession, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,user_id,bank_id,title,status,pipeline_version,attachment_revision,draft_version,active_run_id,expected_count,unique_count,complete_count,blocking_issue_count,COALESCE(last_preview_hash,''),last_preview_version,created_at,updated_at,archived_at FROM import_sessions WHERE id=?`, id)
	return scanImportSession(row)
}

func scanImportSession(row interface{ Scan(...any) error }) (*domain.ImportSession, error) {
	var s domain.ImportSession
	var bank, activeRun, expected, previewVersion sql.NullInt64
	var created, updated string
	var archived sql.NullString
	if err := row.Scan(&s.ID, &s.UserID, &bank, &s.Title, &s.Status, &s.PipelineVersion, &s.AttachmentRevision, &s.DraftVersion, &activeRun, &expected, &s.UniqueCount, &s.CompleteCount, &s.BlockingIssueCount, &s.LastPreviewHash, &previewVersion, &created, &updated, &archived); err != nil {
		return nil, err
	}
	if bank.Valid {
		v := bank.Int64
		s.BankID = &v
	}
	if activeRun.Valid {
		v := activeRun.Int64
		s.ActiveRunID = &v
	}
	if expected.Valid {
		v := int(expected.Int64)
		s.ExpectedCount = &v
	}
	if previewVersion.Valid {
		v := int(previewVersion.Int64)
		s.LastPreviewVersion = &v
	}
	s.CreatedAt, s.UpdatedAt = parseSQLiteTime(created), parseSQLiteTime(updated)
	if archived.Valid {
		t := parseSQLiteTime(archived.String)
		s.ArchivedAt = &t
	}
	return &s, nil
}

func (r *Repository) ListImportSessions(ctx context.Context, userID int64, includeArchived bool) ([]*domain.ImportSession, error) {
	where := "user_id=? AND archived_at IS NULL"
	if includeArchived {
		where = "user_id=?"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,bank_id,title,status,pipeline_version,attachment_revision,draft_version,active_run_id,expected_count,unique_count,complete_count,blocking_issue_count,COALESCE(last_preview_hash,''),last_preview_version,created_at,updated_at,archived_at FROM import_sessions WHERE `+where+` ORDER BY updated_at DESC,id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportSession{}
	for rows.Next() {
		item, err := scanImportSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateImportSession(ctx context.Context, id, userID int64, title string, archive bool) error {
	if archive {
		_, err := r.db.ExecContext(ctx, `UPDATE import_sessions SET archived_at=datetime('now'),updated_at=datetime('now') WHERE id=? AND user_id=?`, id, userID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE import_sessions SET title=?,updated_at=datetime('now') WHERE id=? AND user_id=?`, strings.TrimSpace(title), id, userID)
	return err
}

// ArchiveImportSession cancels every piece of work owned by a session before
// hiding it from the active list.  Archiving is intentionally still a soft
// delete so the conversation and its imported data remain recoverable/auditable.
func (r *Repository) ArchiveImportSession(ctx context.Context, id, userID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM import_sessions WHERE id=? AND user_id=?`, id, userID).Scan(&exists); err != nil {
		return err
	}

	// jobs is a generic queue and stores import_job_id in its JSON payload, so
	// decode only the currently claimable jobs instead of relying on SQLite JSON
	// extensions. This also covers both import_pdf and embedding jobs.
	rows, err := tx.QueryContext(ctx, `SELECT id,payload FROM jobs WHERE status IN ('queued','running')`)
	if err != nil {
		return err
	}
	jobImportIDs := map[int64]int64{}
	for rows.Next() {
		var jobID int64
		var payload string
		if err := rows.Scan(&jobID, &payload); err != nil {
			rows.Close()
			return err
		}
		var importPayload struct {
			ImportJobID int64 `json:"import_job_id"`
		}
		if json.Unmarshal([]byte(payload), &importPayload) == nil && importPayload.ImportJobID > 0 {
			jobImportIDs[jobID] = importPayload.ImportJobID
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// Narrow the generic queue matches to import jobs belonging to this session.
	for jobID, importJobID := range jobImportIDs {
		var belongs int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM import_jobs WHERE id=? AND session_id=?`, importJobID, id).Scan(&belongs); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status='cancelled',error_code='SESSION_ARCHIVED',error_message='会话已删除',lease_until=NULL,updated_at=datetime('now') WHERE id=? AND status IN ('queued','running')`, jobID); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE import_agent_units SET status='cancelled',error_code='SESSION_ARCHIVED',error_message='会话已删除',lease_until=NULL,finished_at=datetime('now'),updated_at=datetime('now') WHERE import_job_id IN (SELECT id FROM import_jobs WHERE session_id=?) AND status IN ('pending','running')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='cancelled',stage='cancelled',error_code='SESSION_ARCHIVED',error_message='会话已删除',updated_at=datetime('now') WHERE session_id=? AND status NOT IN ('completed','failed','cancelled')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_runs SET cancel_requested=1,status='cancelled',error_code='SESSION_ARCHIVED',error_message='会话已删除',finished_at=COALESCE(finished_at,datetime('now')),updated_at=datetime('now') WHERE session_id=? AND status IN ('queued','running','merging')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_sessions SET archived_at=datetime('now'),active_run_id=NULL,status='cancelled',updated_at=datetime('now') WHERE id=? AND user_id=?`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) AttachDocumentToSession(ctx context.Context, sessionID, userID, documentID int64) (*domain.ImportSessionDocument, *domain.ImportJob, *domain.ImportRun, error) {
	return r.attachDocumentToSession(ctx, sessionID, userID, documentID, false)
}

// AttachDocumentToSessionDeferred links an upload without starting the import
// run. The conversation service can then create one run for the attachment and
// the user's prompt together.
func (r *Repository) AttachDocumentToSessionDeferred(ctx context.Context, sessionID, userID, documentID int64) (*domain.ImportSessionDocument, *domain.ImportJob, *domain.ImportRun, error) {
	return r.attachDocumentToSession(ctx, sessionID, userID, documentID, true)
}

func (r *Repository) attachDocumentToSession(ctx context.Context, sessionID, userID, documentID int64, deferProcessing bool) (*domain.ImportSessionDocument, *domain.ImportJob, *domain.ImportRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()
	var bank sql.NullInt64
	var revision, draftVersion int
	var activeRun sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT bank_id,attachment_revision,draft_version,active_run_id FROM import_sessions WHERE id=? AND user_id=? AND archived_at IS NULL`, sessionID, userID).Scan(&bank, &revision, &draftVersion, &activeRun); err != nil {
		return nil, nil, nil, err
	}
	revision++
	res, err := tx.ExecContext(ctx, `INSERT INTO import_jobs(user_id,bank_id,document_id,status,stage,pipeline_version,session_id) VALUES(?,?,?,'uploaded','uploaded',?,?)`, userID, nullableNullInt(bank), documentID, domain.VisionPipelineVersionV11, sessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	jobID, _ := res.LastInsertId()
	res, err = tx.ExecContext(ctx, `INSERT INTO import_session_documents(session_id,document_id,import_job_id,attachment_revision) VALUES(?,?,?,?)`, sessionID, documentID, jobID, revision)
	if err != nil {
		return nil, nil, nil, err
	}
	sessionDocumentID, _ := res.LastInsertId()
	runID := activeRun.Int64
	if !activeRun.Valid && !deferProcessing {
		runType := "incremental_document"
		if revision == 1 {
			runType = "initial_extract"
		}
		res, err = tx.ExecContext(ctx, `INSERT INTO import_runs(session_id,run_type,status,base_attachment_revision,base_draft_version,started_at,strategy_version) VALUES(?,?,'running',?,?,datetime('now'),?)`, sessionID, runType, revision, draftVersion, domain.VisionPipelineVersionV11)
		if err != nil {
			return nil, nil, nil, err
		}
		runID, _ = res.LastInsertId()
	} else {
		_, _ = tx.ExecContext(ctx, `UPDATE import_runs SET base_attachment_revision=? WHERE id=? AND status='running'`, revision, runID)
	}
	sessionStatus := "receiving_files"
	if runID > 0 {
		sessionStatus = "analyzing"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET attachment_revision=?,active_run_id=?,status=?,last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=?`, revision, nullableInt64(runID), sessionStatus, sessionID); err != nil {
		return nil, nil, nil, err
	}
	attachmentMessage := "已上传文档，Agent 将自动分析并合并到当前草稿。"
	if runID == 0 {
		attachmentMessage = "已上传文档，等待你的导入指令。"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO import_session_messages(session_id,run_id,role,message_type,content,status) VALUES(?,?, 'user','attachment',?,'completed')`, sessionID, nullableInt64(runID), attachmentMessage)
	if err != nil {
		return nil, nil, nil, err
	}
	var agentMessageID int64
	if runID > 0 {
		err = tx.QueryRowContext(ctx, `SELECT id FROM import_session_messages WHERE session_id=? AND run_id=? AND role='assistant' AND message_type='agent_run' ORDER BY id LIMIT 1`, sessionID, runID).Scan(&agentMessageID)
		if errors.Is(err, sql.ErrNoRows) {
			messageRes, insertErr := tx.ExecContext(ctx, `INSERT INTO import_session_messages(session_id,run_id,role,message_type,content,status) VALUES(?,?,'assistant','agent_run','已接收附件，正在等待后台 Agent 开始分析。','running')`, sessionID, runID)
			if insertErr != nil {
				return nil, nil, nil, insertErr
			}
			agentMessageID, _ = messageRes.LastInsertId()
		} else if err != nil {
			return nil, nil, nil, err
		}
	}
	_, _ = tx.ExecContext(ctx, `INSERT INTO import_events(session_id,run_id,message_id,event_type,stage,document_id,summary) VALUES(?,?,?,'attachment_received','receiving_files',?,'文档已上传，等待导入指令')`, sessionID, nullableInt64(runID), nullableInt64(agentMessageID), documentID)
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}
	docLink := &domain.ImportSessionDocument{ID: sessionDocumentID, SessionID: sessionID, DocumentID: documentID, ImportJobID: &jobID, AttachmentRevision: revision, DocumentRole: "unknown", RoleStatus: "inferred", Status: "uploaded"}
	job, err := r.GetImportJob(ctx, jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	if runID == 0 {
		return docLink, job, nil, nil
	}
	run, err := r.GetImportRun(ctx, runID)
	return docLink, job, run, err
}

func nullableNullInt(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func (r *Repository) GetImportRun(ctx context.Context, id int64) (*domain.ImportRun, error) {
	var run domain.ImportRun
	var trigger, result sql.NullInt64
	var cancel int
	var started, finished, lastProgress sql.NullString
	var eta sql.NullInt64
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT id,session_id,trigger_message_id,run_type,status,base_attachment_revision,base_draft_version,result_draft_version,COALESCE(action_json,''),cancel_requested,COALESCE(error_code,''),COALESCE(error_message,''),started_at,finished_at,created_at,COALESCE(strategy_version,'vision-agent.v1'),COALESCE(current_stage,''),completed_units,total_units,failed_units,last_progress_at,eta_seconds,adaptive_concurrency,COALESCE(degraded_reason,''),COALESCE(result_status,'unchanged'),COALESCE(model_success_count,0),COALESCE(candidate_updated_count,0),COALESCE(resolved_candidate_count,0),COALESCE(remaining_blocked_count,0),COALESCE(result_summary_json,'{}') FROM import_runs WHERE id=?`, id).Scan(&run.ID, &run.SessionID, &trigger, &run.RunType, &run.Status, &run.BaseAttachmentRevision, &run.BaseDraftVersion, &result, &run.ActionJSON, &cancel, &run.ErrorCode, &run.ErrorMessage, &started, &finished, &created, &run.StrategyVersion, &run.CurrentStage, &run.CompletedUnits, &run.TotalUnits, &run.FailedUnits, &lastProgress, &eta, &run.AdaptiveConcurrency, &run.DegradedReason, &run.ResultStatus, &run.ModelSuccessCount, &run.CandidateUpdatedCount, &run.ResolvedCandidateCount, &run.RemainingBlockedCount, &run.ResultSummaryJSON)
	if err != nil {
		return nil, err
	}
	if trigger.Valid {
		v := trigger.Int64
		run.TriggerMessageID = &v
	}
	if result.Valid {
		v := int(result.Int64)
		run.ResultDraftVersion = &v
	}
	run.CancelRequested = cancel == 1
	run.StartedAt = parseTime(started)
	run.FinishedAt = parseTime(finished)
	run.LastProgressAt = parseTime(lastProgress)
	if eta.Valid {
		v := int(eta.Int64)
		run.ETASeconds = &v
	}
	run.CreatedAt = parseSQLiteTime(created)
	if run.TotalUnits > 0 {
		run.StageProgress = float64(run.CompletedUnits) / float64(run.TotalUnits)
		if run.StageProgress > 1 {
			run.StageProgress = 1
		}
	}
	base, weight := 0.0, 0.0
	weightedOverall := false
	switch run.CurrentStage {
	case "rendering":
		base, weight = 0, .05
	case "catalog":
		base, weight = .05, .30
	case "candidate_review":
		// Candidate-level review is already scoped to the requested units, so
		// its persisted unit progress is the complete progress signal.
		base, weight = 0, 1
	case "parallel_extraction", "extract_question", "extract_answer":
		base, weight = .35, .55
		var questionDone, questionTotal, answerDone, answerTotal int
		_ = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN u.unit_type='extract_question' AND u.status='completed' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN u.unit_type='extract_question' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN u.unit_type='extract_answer' AND u.status='completed' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN u.unit_type='extract_answer' THEN 1 ELSE 0 END),0) FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=? AND u.run_id=?`, run.SessionID, run.ID).Scan(&questionDone, &questionTotal, &answerDone, &answerTotal)
		questionProgress, answerProgress := 0.0, 0.0
		if questionTotal > 0 {
			questionProgress = float64(questionDone) / float64(questionTotal)
		}
		if answerTotal > 0 {
			answerProgress = float64(answerDone) / float64(answerTotal)
		}
		run.OverallProgress = .35 + .30*questionProgress + .25*answerProgress
		weightedOverall = true
	case "linking":
		base, weight = .90, .05
	case "validating":
		base, weight = .95, .05
	default:
		if run.Status == "completed" {
			base = 1
		}
	}
	if !weightedOverall {
		run.OverallProgress = base + weight*run.StageProgress
	}
	if run.OverallProgress > 1 {
		run.OverallProgress = 1
	}
	return &run, nil
}

func (r *Repository) ListSessionDocuments(ctx context.Context, sessionID int64) ([]*domain.ImportSessionDocument, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT sd.id,sd.session_id,sd.document_id,sd.import_job_id,d.original_name,COALESCE(d.page_count,0),COALESCE(d.is_scanned,0),COALESCE((SELECT COUNT(*) FROM document_pages p WHERE p.document_id=d.id AND (p.ocr_status IN ('required','failed') OR p.needs_ocr=1)),0),COALESCE((SELECT COUNT(*) FROM document_pages p WHERE p.document_id=d.id AND p.ocr_status='success'),0),COALESCE((SELECT COUNT(*) FROM document_pages p WHERE p.document_id=d.id AND p.ocr_status='failed'),0),sd.document_role,sd.role_confidence,sd.role_status,COALESCE(sd.profile_json,''),sd.attachment_revision,sd.status FROM import_session_documents sd JOIN documents d ON d.id=sd.document_id WHERE sd.session_id=? ORDER BY sd.id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportSessionDocument{}
	for rows.Next() {
		d := new(domain.ImportSessionDocument)
		var job sql.NullInt64
		var scanned, required, completed, failed int
		if err := rows.Scan(&d.ID, &d.SessionID, &d.DocumentID, &job, &d.OriginalName, &d.PageCount, &scanned, &required, &completed, &failed, &d.DocumentRole, &d.RoleConfidence, &d.RoleStatus, &d.ProfileJSON, &d.AttachmentRevision, &d.Status); err != nil {
			return nil, err
		}
		d.IsScanned = scanned == 1
		d.OCRRequiredCount, d.OCRCompletedCount, d.OCRFailedCount = required, completed, failed
		if job.Valid {
			v := job.Int64
			d.ImportJobID = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateSessionDocumentProfile(ctx context.Context, sessionID, documentID int64, role string, confidence float64, profileJSON string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE import_session_documents SET document_role=?,role_confidence=?,profile_json=?,status='profiled',updated_at=datetime('now') WHERE session_id=? AND document_id=?`, role, confidence, profileJSON, sessionID, documentID)
	return err
}

func (r *Repository) AddSessionMessage(ctx context.Context, m *domain.ImportSessionMessage) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_session_messages(session_id,run_id,parent_message_id,role,message_type,content,metadata_json,status) VALUES(?,?,?,?,?,?,?,?)`, m.SessionID, m.RunID, m.ParentMessageID, m.Role, m.MessageType, m.Content, nullableString(m.MetadataJSON), m.Status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) ListSessionMessages(ctx context.Context, sessionID int64) ([]*domain.ImportSessionMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,session_id,run_id,parent_message_id,role,message_type,content,COALESCE(metadata_json,''),status,created_at FROM import_session_messages WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportSessionMessage{}
	for rows.Next() {
		m := new(domain.ImportSessionMessage)
		var run, parent sql.NullInt64
		var created string
		if err := rows.Scan(&m.ID, &m.SessionID, &run, &parent, &m.Role, &m.MessageType, &m.Content, &m.MetadataJSON, &m.Status, &created); err != nil {
			return nil, err
		}
		if run.Valid {
			v := run.Int64
			m.RunID = &v
		}
		if parent.Valid {
			v := parent.Int64
			m.ParentMessageID = &v
		}
		m.CreatedAt = parseSQLiteTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) AddImportEvent(ctx context.Context, e *domain.ImportEvent) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_events(session_id,run_id,message_id,event_type,stage,agent_role,document_id,owned_page_start,owned_page_end,context_page_start,context_page_end,current_count,total_count,summary,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.SessionID, e.RunID, e.MessageID, e.Type, nullableString(e.Stage), nullableString(e.AgentRole), e.DocumentID, nullableInt(e.OwnedPageStart), nullableInt(e.OwnedPageEnd), nullableInt(e.ContextPageStart), nullableInt(e.ContextPageEnd), nullableInt(e.Current), nullableInt(e.Total), nullableString(e.Summary), nullableString(e.PayloadJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) ListImportEvents(ctx context.Context, sessionID, afterID int64, limit int) ([]*domain.ImportEvent, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,session_id,run_id,message_id,event_type,COALESCE(stage,''),COALESCE(agent_role,''),document_id,COALESCE(owned_page_start,0),COALESCE(owned_page_end,0),COALESCE(context_page_start,0),COALESCE(context_page_end,0),COALESCE(current_count,0),COALESCE(total_count,0),COALESCE(summary,''),COALESCE(payload_json,''),created_at FROM import_events WHERE session_id=? AND id>? ORDER BY id LIMIT ?`, sessionID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportEvent{}
	for rows.Next() {
		e := new(domain.ImportEvent)
		var run, msg, doc sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &e.SessionID, &run, &msg, &e.Type, &e.Stage, &e.AgentRole, &doc, &e.OwnedPageStart, &e.OwnedPageEnd, &e.ContextPageStart, &e.ContextPageEnd, &e.Current, &e.Total, &e.Summary, &e.PayloadJSON, &created); err != nil {
			return nil, err
		}
		if run.Valid {
			v := run.Int64
			e.RunID = &v
		}
		if msg.Valid {
			v := msg.Int64
			e.MessageID = &v
		}
		if doc.Valid {
			v := doc.Int64
			e.DocumentID = &v
		}
		e.CreatedAt = parseSQLiteTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ListImportProblems(ctx context.Context, sessionID int64) ([]*domain.ImportProblem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,session_id,run_id,document_id,problem_type,severity,COALESCE(page_start,0),COALESCE(page_end,0),candidate_id,COALESCE(source_identity,''),COALESCE(detail_json,''),status,origin_id,source_span_id,COALESCE(resolution_hint,'manual_review'),resolved_by_run_id,resolved_at FROM import_problems WHERE session_id=? ORDER BY status='open' DESC,id DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ImportProblem{}
	for rows.Next() {
		p := new(domain.ImportProblem)
		var run, doc, candidate, origin, span, resolvedRun sql.NullInt64
		var resolvedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.SessionID, &run, &doc, &p.ProblemType, &p.Severity, &p.PageStart, &p.PageEnd, &candidate, &p.SourceIdentity, &p.DetailJSON, &p.Status, &origin, &span, &p.ResolutionHint, &resolvedRun, &resolvedAt); err != nil {
			return nil, err
		}
		if run.Valid {
			v := run.Int64
			p.RunID = &v
		}
		if doc.Valid {
			v := doc.Int64
			p.DocumentID = &v
		}
		if candidate.Valid {
			v := candidate.Int64
			p.CandidateID = &v
		}
		if origin.Valid {
			v := origin.Int64
			p.OriginID = &v
		}
		if span.Valid {
			v := span.Int64
			p.SourceSpanID = &v
		}
		if resolvedRun.Valid {
			v := resolvedRun.Int64
			p.ResolvedByRunID = &v
		}
		p.ResolvedAt = parseTime(resolvedAt)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertDirectoryEntries(ctx context.Context, sessionID, documentID int64, unitID int64, documentRole string, entries []domain.VisionDirectoryEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range entries {
		canonical := domain.CanonicalQuestionType(e.QuestionType)
		if e.CanonicalQuestionType != "" {
			canonical = e.CanonicalQuestionType
		}
		if e.SourceIdentity == "" {
			e.SourceIdentity = fmt.Sprintf("%s|%d|%s|%d|%d", normalizeSectionPath(e.SectionPath), e.SetNo, canonical, e.SourceQuestionNo, e.SourcePageStart)
		}
		if e.SectionScopeID == "" {
			e.SectionScopeID = normalizeSectionPath(e.SectionPath)
		}
		section, _ := json.Marshal(e.SectionPath)
		region, _ := json.Marshal(e.Region)
		normalized := normalizeSectionPath(e.SectionPath)
		matchScope := normalizeMatchScope(e.SectionScopeID)
		_, err = tx.ExecContext(ctx, `INSERT INTO import_directory_entries(session_id,document_id,unit_id,content_role,document_role,section_path_json,normalized_section_path,set_no,question_type,canonical_question_type,section_scope_id,sequence_in_scope,original_question_label,match_scope_key,source_question_no,source_page_start,source_page_end,anchor,marker_top,marker_bottom,region_json,source_identity) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(session_id,document_id,source_identity) DO UPDATE SET unit_id=excluded.unit_id,document_role=excluded.document_role,section_path_json=excluded.section_path_json,normalized_section_path=excluded.normalized_section_path,canonical_question_type=excluded.canonical_question_type,section_scope_id=excluded.section_scope_id,sequence_in_scope=excluded.sequence_in_scope,original_question_label=excluded.original_question_label,match_scope_key=excluded.match_scope_key,source_page_start=excluded.source_page_start,source_page_end=excluded.source_page_end,anchor=excluded.anchor,marker_top=excluded.marker_top,marker_bottom=excluded.marker_bottom,region_json=excluded.region_json,status='active',updated_at=datetime('now')`, sessionID, documentID, nullableInt64(unitID), e.ContentRole, documentRole, string(section), normalized, e.SetNo, e.QuestionType, canonical, e.SectionScopeID, e.SequenceInScope, e.OriginalQuestionLabel, matchScope, e.SourceQuestionNo, e.SourcePageStart, e.SourcePageEnd, e.Anchor, e.MarkerTop, e.MarkerBottom, nullableString(string(region)), e.SourceIdentity)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeMatchScope(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
	return strings.NewReplacer("《", "", "》", "", "〈", "", "〉", "", "（", "", "）", "", "(", "", ")", "", "【", "", "】", "", "[", "", "]", "", "·", "", "，", "", ",", "", "。", "", ".", "", "：", "", ":", "").Replace(value)
}

// ReplaceDirectoryEntries atomically replaces the derived directory projection
// for one document. Raw Agent unit output remains untouched, so a corrected
// coordinator can rebuild identities after a model omitted a topic heading or
// caused a cross-topic duplicate.
func (r *Repository) ReplaceDirectoryEntries(ctx context.Context, sessionID, documentID int64, unitID int64, documentRole string, entries []domain.VisionDirectoryEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM import_directory_entries WHERE session_id=? AND document_id=?`, sessionID, documentID); err != nil {
		return err
	}
	for _, e := range entries {
		canonical := domain.CanonicalQuestionType(e.QuestionType)
		if e.CanonicalQuestionType != "" {
			canonical = e.CanonicalQuestionType
		}
		if e.SourceIdentity == "" {
			e.SourceIdentity = fmt.Sprintf("%s|%d|%s|%d|%d", normalizeSectionPath(e.SectionPath), e.SetNo, canonical, e.SourceQuestionNo, e.SourcePageStart)
		}
		if e.SectionScopeID == "" {
			e.SectionScopeID = normalizeSectionPath(e.SectionPath)
		}
		section, _ := json.Marshal(e.SectionPath)
		region, _ := json.Marshal(e.Region)
		normalized := normalizeSectionPath(e.SectionPath)
		_, err = tx.ExecContext(ctx, `INSERT INTO import_directory_entries(session_id,document_id,unit_id,content_role,document_role,section_path_json,normalized_section_path,set_no,question_type,canonical_question_type,section_scope_id,sequence_in_scope,source_question_no,source_page_start,source_page_end,anchor,marker_top,marker_bottom,region_json,source_identity) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(session_id,document_id,source_identity) DO UPDATE SET unit_id=excluded.unit_id,document_role=excluded.document_role,section_path_json=excluded.section_path_json,normalized_section_path=excluded.normalized_section_path,canonical_question_type=excluded.canonical_question_type,section_scope_id=excluded.section_scope_id,sequence_in_scope=excluded.sequence_in_scope,source_page_start=excluded.source_page_start,source_page_end=excluded.source_page_end,anchor=excluded.anchor,marker_top=excluded.marker_top,marker_bottom=excluded.marker_bottom,region_json=excluded.region_json,status='active',updated_at=datetime('now')`, sessionID, documentID, nullableInt64(unitID), e.ContentRole, documentRole, string(section), normalized, e.SetNo, e.QuestionType, canonical, e.SectionScopeID, e.SequenceInScope, e.SourceQuestionNo, e.SourcePageStart, e.SourcePageEnd, e.Anchor, e.MarkerTop, e.MarkerBottom, nullableString(string(region)), e.SourceIdentity)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeSectionPath(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(p)), ""))
		if v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, "/")
}
func nullableInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func (r *Repository) UpsertAnswerRecords(ctx context.Context, sessionID, documentID, runID int64, answers []domain.VisionAnswer) error {
	for _, a := range answers {
		canonical := domain.CanonicalQuestionType(a.QuestionType)
		if a.CanonicalQuestionType != "" {
			canonical = a.CanonicalQuestionType
		}
		if a.QuestionType == "" {
			a.QuestionType = canonical
		}
		sectionPath := append([]string(nil), a.SectionPath...)
		if len(sectionPath) == 0 && strings.TrimSpace(a.SectionScopeID) != "" {
			sectionPath = strings.Split(strings.TrimSpace(a.SectionScopeID), "/")
		}
		if a.SectionScopeID == "" {
			a.SectionScopeID = normalizeSectionPath(sectionPath)
		}
		storageScope := strings.TrimSpace(a.SectionScopeID)
		if storageScope == "" {
			storageScope = normalizeSectionPath(sectionPath)
		}
		matchScope := normalizeMatchScope(storageScope)
		if storageScope == "" {
			// Keep the storage projection non-empty even when a model omitted
			// every section field. The immutable source identity (when present)
			// remains the only value that can later repair this record.
			storageScope = "__unresolved__"
		}
		section, _ := json.Marshal(sectionPath)
		answer, _ := json.Marshal(a.Answer)
		kind := "answer"
		if strings.TrimSpace(a.Analysis) != "" {
			kind = "explanation"
		}
		identity := strings.TrimSpace(a.SourceIdentity)
		if identity == "" && len(sectionPath) > 0 && a.SourceQuestionNo > 0 {
			identity = fmt.Sprintf("%s|%s|%d|%s|%d", string(section), kind, a.SetNo, a.QuestionType, a.SourceQuestionNo)
		}
		// source_identity is the durable primary identity for answer records.
		// Do the lookup explicitly instead of relying on the old normalized-path
		// UNIQUE constraint: two subjective blocks can legitimately share the
		// same topic, type, number and physical page while having different
		// printed headings (for example 辨析题 1 and 简答题 1).
		var existingID int64
		if identity != "" {
			_ = r.db.QueryRowContext(ctx, `SELECT id FROM import_answer_records WHERE session_id=? AND document_id=? AND source_identity=? AND source_kind=? ORDER BY id DESC LIMIT 1`, sessionID, documentID, identity, kind).Scan(&existingID)
		} else {
			_ = r.db.QueryRowContext(ctx, `SELECT id FROM import_answer_records WHERE session_id=? AND document_id=? AND normalized_section_path=? AND set_no=? AND question_type=? AND source_question_no=? AND source_kind=? AND page_start=? ORDER BY id DESC LIMIT 1`, sessionID, documentID, storageScope, a.SetNo, a.QuestionType, a.SourceQuestionNo, kind, a.SourcePageStart).Scan(&existingID)
		}
		if existingID > 0 {
			_, err := r.db.ExecContext(ctx, `UPDATE import_answer_records SET run_id=COALESCE(?,run_id),normalized_section_path=?,section_path_json=?,set_no=?,question_type=?,canonical_question_type=?,section_scope_id=?,match_scope_key=?,source_question_no=?,answer_json=?,analysis=?,page_end=?,source_identity=CASE WHEN ?<>'' THEN ? ELSE source_identity END,link_status='unresolved' WHERE id=?`, nullableInt64(runID), storageScope, string(section), a.SetNo, a.QuestionType, canonical, storageScope, matchScope, a.SourceQuestionNo, string(answer), a.Analysis, a.SourcePageEnd, identity, identity, existingID)
			if err != nil {
				return err
			}
			continue
		}
		_, err := r.db.ExecContext(ctx, `INSERT INTO import_answer_records(session_id,document_id,run_id,normalized_section_path,section_path_json,set_no,question_type,canonical_question_type,section_scope_id,match_scope_key,source_question_no,answer_json,analysis,page_start,page_end,source_kind,source_identity) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, sessionID, documentID, nullableInt64(runID), storageScope, string(section), a.SetNo, a.QuestionType, canonical, storageScope, matchScope, a.SourceQuestionNo, string(answer), a.Analysis, a.SourcePageStart, a.SourcePageEnd, kind, identity)
		if err != nil {
			// Keep compatibility with databases created before migration 0014.
			// Those databases still have the legacy UNIQUE constraint; a record
			// without an immutable identity can safely fall back to that key.
			if identity == "" {
				var legacyID int64
				if lookupErr := r.db.QueryRowContext(ctx, `SELECT id FROM import_answer_records WHERE session_id=? AND document_id=? AND normalized_section_path=? AND set_no=? AND question_type=? AND source_question_no=? AND source_kind=? AND page_start=? ORDER BY id DESC LIMIT 1`, sessionID, documentID, storageScope, a.SetNo, a.QuestionType, a.SourceQuestionNo, kind, a.SourcePageStart).Scan(&legacyID); lookupErr == nil {
					_, updateErr := r.db.ExecContext(ctx, `UPDATE import_answer_records SET run_id=COALESCE(?,run_id),section_path_json=?,canonical_question_type=?,section_scope_id=?,answer_json=?,analysis=?,page_end=?,link_status='unresolved' WHERE id=?`, nullableInt64(runID), string(section), canonical, storageScope, string(answer), a.Analysis, a.SourcePageEnd, legacyID)
					if updateErr == nil {
						continue
					}
				}
			}
			return err
		}
	}
	return nil
}

func (r *Repository) CancelImportRun(ctx context.Context, sessionID, runID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE import_runs SET cancel_requested=1,status='cancelled',finished_at=datetime('now') WHERE id=? AND session_id=? AND status IN ('queued','running','merging')`, runID, sessionID); err != nil {
		return err
	}
	// A run owns the retry state of its Agent units. Leaving those rows as
	// pending/running makes the activity card continue to look alive after the
	// run itself was cancelled, and a later worker can accidentally resume
	// them. Completed/failed outputs remain untouched for audit and reuse.
	if _, err = tx.ExecContext(ctx, `UPDATE import_agent_units SET status='cancelled',error_code='RUN_CANCELLED',error_message='任务已由用户取消',lease_until=NULL,finished_at=COALESCE(finished_at,datetime('now')),updated_at=datetime('now') WHERE run_id=? AND status IN ('pending','running')`, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_session_messages SET status='cancelled' WHERE session_id=? AND run_id=? AND message_type='agent_run' AND status='running'`, sessionID, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET active_run_id=NULL,status='cancelled',updated_at=datetime('now') WHERE id=? AND active_run_id=?`, sessionID, runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) IsImportRunCancelled(ctx context.Context, runID int64) bool {
	var cancelled int
	err := r.db.QueryRowContext(ctx, `SELECT cancel_requested FROM import_runs WHERE id=?`, runID).Scan(&cancelled)
	return err == nil && cancelled == 1
}

func (r *Repository) IsImportSessionCancelled(ctx context.Context, sessionID int64) bool {
	var cancelled int
	err := r.db.QueryRowContext(ctx, `SELECT CASE WHEN archived_at IS NOT NULL OR status='cancelled' THEN 1 ELSE 0 END FROM import_sessions WHERE id=?`, sessionID).Scan(&cancelled)
	return err == nil && cancelled == 1
}

func (r *Repository) CreateSessionRun(ctx context.Context, sessionID int64, messageID *int64, runType string, action interface{}) (*domain.ImportRun, error) {
	raw, _ := json.Marshal(action)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var attachmentRevision, draftVersion int
	var activeRun sql.NullInt64
	var archived sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT attachment_revision,draft_version,active_run_id,archived_at FROM import_sessions WHERE id=?`, sessionID).Scan(&attachmentRevision, &draftVersion, &activeRun, &archived); err != nil {
		return nil, err
	}
	if archived.Valid && archived.String != "" {
		return nil, fmt.Errorf("SESSION_ARCHIVED")
	}
	if activeRun.Valid {
		var activeStatus string
		if err = tx.QueryRowContext(ctx, `SELECT status FROM import_runs WHERE id=? AND session_id=?`, activeRun.Int64, sessionID).Scan(&activeStatus); err == nil {
			if activeStatus == "queued" || activeStatus == "running" || activeStatus == "merging" {
				return nil, fmt.Errorf("ACTIVE_RUN_EXISTS")
			}
		}
	}
	strategyVersion := domain.VisionPipelineVersionV11
	_ = tx.QueryRowContext(ctx, `SELECT j.pipeline_version FROM import_jobs j JOIN import_session_documents sd ON sd.import_job_id=j.id WHERE sd.session_id=? ORDER BY sd.id DESC LIMIT 1`, sessionID).Scan(&strategyVersion)
	res, err := tx.ExecContext(ctx, `INSERT INTO import_runs(session_id,trigger_message_id,run_type,status,base_attachment_revision,base_draft_version,action_json,started_at,strategy_version) VALUES(?,?,?,'running',?,?,?,datetime('now'),?)`, sessionID, messageID, runType, attachmentRevision, draftVersion, string(raw), strategyVersion)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "uq_import_runs_active_mutation") || strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return nil, fmt.Errorf("ACTIVE_RUN_EXISTS")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET active_run_id=?,status='analyzing',updated_at=datetime('now') WHERE id=?`, id, sessionID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetImportRun(ctx, id)
}

func (r *Repository) FinishSessionRun(ctx context.Context, sessionID, runID int64, status, code, message string, draftVersion int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingStatus string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM import_runs WHERE id=? AND session_id=?`, runID, sessionID).Scan(&existingStatus); err != nil {
		return err
	}
	if existingStatus == "completed" || existingStatus == "failed" || existingStatus == "cancelled" || existingStatus == "superseded" {
		// A late worker callback must not overwrite a terminal result (especially
		// cancelled/superseded) with a misleading failure or completion.
		return nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_runs SET status=?,error_code=?,error_message=?,result_draft_version=?,finished_at=datetime('now') WHERE id=? AND session_id=?`, status, nullableString(code), nullableString(message), draftVersion, runID, sessionID); err != nil {
		return err
	}
	messageStatus := "completed"
	if status == "failed" || status == "cancelled" {
		messageStatus = status
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_session_messages SET status=? WHERE session_id=? AND run_id=? AND message_type='agent_run' AND status='running'`, messageStatus, sessionID, runID); err != nil {
		return err
	}
	if strings.TrimSpace(message) != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE import_session_messages SET content=? WHERE session_id=? AND run_id=? AND message_type='agent_run'`, message, sessionID, runID); err != nil {
			return err
		}
	}
	if status == "completed" || status == "failed" || status == "cancelled" || status == "superseded" {
		sessionStatus := ""
		if status == "failed" {
			sessionStatus = "needs_attention"
		} else if status == "cancelled" {
			sessionStatus = "cancelled"
		}
		if sessionStatus == "" {
			_, err = tx.ExecContext(ctx, `UPDATE import_sessions SET active_run_id=NULL,updated_at=datetime('now') WHERE id=? AND active_run_id=?`, sessionID, runID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE import_sessions SET active_run_id=NULL,status=?,updated_at=datetime('now') WHERE id=? AND active_run_id=?`, sessionStatus, sessionID, runID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EnsureRunAgentMessage returns the persistent assistant card that owns a run's
// live tool trace. It also repairs sessions created before live run cards were
// introduced.
func (r *Repository) EnsureRunAgentMessage(ctx context.Context, sessionID, runID int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM import_session_messages WHERE session_id=? AND run_id=? AND role='assistant' AND message_type='agent_run' ORDER BY id LIMIT 1`, sessionID, runID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_session_messages(session_id,run_id,role,message_type,content,status) VALUES(?,?,'assistant','agent_run','Agent 已开始分析文档，执行进度会持续更新。','running')`, sessionID, runID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) CreateActionRequest(ctx context.Context, sessionID int64, messageID *int64, actionType string, action interface{}, expectedVersion int) (int64, error) {
	raw, _ := json.Marshal(action)
	res, err := r.db.ExecContext(ctx, `INSERT INTO import_action_requests(session_id,message_id,action_type,action_json,expected_draft_version) VALUES(?,?,?,?,?)`, sessionID, messageID, actionType, string(raw), expectedVersion)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) ConfirmActionRequest(ctx context.Context, sessionID, actionID int64, expectedVersion int) (string, string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var actionType, raw, status string
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT action_type,action_json,expected_draft_version,status FROM import_action_requests WHERE id=? AND session_id=?`, actionID, sessionID).Scan(&actionType, &raw, &version, &status); err != nil {
		return "", "", err
	}
	if status != "pending" {
		return "", "", fmt.Errorf("ACTION_ALREADY_HANDLED")
	}
	if version != expectedVersion {
		return "", "", fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_action_requests SET status='confirmed',confirmed_at=datetime('now') WHERE id=?`, actionID); err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return actionType, raw, nil
}

func (r *Repository) SessionOwnsDocument(ctx context.Context, sessionID, documentID int64) bool {
	var one int
	return r.db.QueryRowContext(ctx, `SELECT 1 FROM import_session_documents WHERE session_id=? AND document_id=?`, sessionID, documentID).Scan(&one) == nil
}
func (r *Repository) SessionOwnsCandidate(ctx context.Context, sessionID, candidateID int64) bool {
	var one int
	return r.db.QueryRowContext(ctx, `SELECT 1 FROM candidate_questions WHERE import_session_id=? AND id=?`, sessionID, candidateID).Scan(&one) == nil
}

func (r *Repository) UpdateSessionCandidate(ctx context.Context, sessionID, candidateID int64, expectedVersion int, questionJSON string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT draft_version FROM import_sessions WHERE id=?`, sessionID).Scan(&version); err != nil {
		return false, err
	}
	if version != expectedVersion {
		return false, nil
	}
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,draft_version=draft_version+1,status='needs_review',review_decision='pending',answer_link_status='unresolved',answer_link_method=NULL,answer_link_confidence=NULL,updated_at=datetime('now') WHERE id=? AND import_session_id=?`, questionJSON, questionJSON, candidateID, sessionID)
	if err != nil {
		return false, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET draft_version=draft_version+1,status='needs_attention',last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=?`, sessionID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) UpdateCandidateReview(ctx context.Context, sessionID, candidateID int64, expectedVersion int, decision, reason string, reviewerID int64) (bool, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != domain.ReviewInclude && decision != domain.ReviewExclude && decision != domain.ReviewPending {
		return false, fmt.Errorf("INVALID_REVIEW_DECISION")
	}
	if decision == domain.ReviewExclude && strings.TrimSpace(reason) == "" {
		return false, fmt.Errorf("EXCLUSION_REASON_REQUIRED")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT draft_version FROM import_sessions WHERE id=?`, sessionID).Scan(&version); err != nil {
		return false, err
	}
	if version != expectedVersion {
		return false, nil
	}
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET review_decision=?,exclusion_reason=?,reviewed_by=?,reviewed_at=datetime('now'),updated_at=datetime('now') WHERE id=? AND import_session_id=? AND status NOT IN ('persisted','duplicate')`, decision, nullableString(reason), reviewerID, candidateID, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET draft_version=draft_version+1,last_preview_hash=NULL,last_preview_version=NULL,status='needs_attention',updated_at=datetime('now') WHERE id=?`, sessionID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) BulkUpdateCandidateReview(ctx context.Context, sessionID int64, candidateIDs []int64, expectedVersion int, decision, reason string, reviewerID int64) (bool, error) {
	if len(candidateIDs) == 0 {
		return false, fmt.Errorf("CANDIDATES_REQUIRED")
	}
	if decision == domain.ReviewExclude && strings.TrimSpace(reason) == "" {
		return false, fmt.Errorf("EXCLUSION_REASON_REQUIRED")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT draft_version FROM import_sessions WHERE id=?`, sessionID).Scan(&version); err != nil {
		return false, err
	}
	if version != expectedVersion {
		return false, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(candidateIDs)), ",")
	args := []interface{}{decision, nullableString(reason), reviewerID}
	for _, id := range candidateIDs {
		args = append(args, id)
	}
	args = append(args, sessionID)
	res, err := tx.ExecContext(ctx, `UPDATE candidate_questions SET review_decision=?,exclusion_reason=?,reviewed_by=?,reviewed_at=datetime('now'),updated_at=datetime('now') WHERE id IN (`+placeholders+`) AND import_session_id=?`, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n != int64(len(candidateIDs)) {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET draft_version=draft_version+1,last_preview_hash=NULL,last_preview_version=NULL,status='needs_attention',updated_at=datetime('now') WHERE id=?`, sessionID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) UpdateSessionDocumentRole(ctx context.Context, sessionID, documentID int64, role string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE import_session_documents SET document_role=?,role_status='confirmed',role_confidence=1,updated_at=datetime('now') WHERE session_id=? AND document_id=?`, role, sessionID, documentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_directory_entries SET document_role=?,updated_at=datetime('now') WHERE session_id=? AND document_id=?`, role, sessionID, documentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SessionDraft(ctx context.Context, sessionID int64, page, pageSize int) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	} else if pageSize > 200 {
		// Keep the repository contract aligned with the HTTP handler: callers
		// may request a larger page, but the server must cap it rather than
		// silently falling back to the small legacy default (which can make a
		// complete draft look like it only contains the first 50/100 rows).
		pageSize = 200
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate'`, sessionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	// Keep cards in the same reading order as the PDF.  Sorting only by
	// physical page and printed question number is incorrect when a page
	// contains the tail of one block followed by a new block whose numbers
	// restart at 1 (for example single-choice 33--38 followed by multiple-
	// choice 1--4).  Directory entries carry the normalized vertical anchor;
	// use it as the primary intra-page position and fall back to the question
	// number for legacy rows without a directory anchor.
	rows, err := r.db.QueryContext(ctx, `WITH directory_positions AS (
		SELECT session_id,document_id,source_page_start,source_page_end,source_question_no,canonical_question_type,MIN(marker_top) AS marker_top
		FROM import_directory_entries
		WHERE session_id=? AND content_role='question' AND status='active' AND marker_top>0
		GROUP BY session_id,document_id,source_page_start,source_page_end,source_question_no,canonical_question_type
	) SELECT c.id,c.status,c.document_id,COALESCE(c.source_identity,''),COALESCE(c.section_path_json,'[]'),c.set_no,c.source_question_no,COALESCE(c.question_type,''),COALESCE(c.canonical_question_type,''),COALESCE(c.section_scope_id,''),COALESCE(c.merged_json,c.question_json,c.extracted_json,'{}'),COALESCE(c.validation_issues_json,'[]'),COALESCE(c.visual_confidence,c.confidence,0),COALESCE(c.question_page_start,c.page_start,0),COALESCE(c.question_page_end,c.page_end,0),COALESCE(c.answer_page_start,0),COALESCE(c.answer_page_end,0),c.draft_version,COALESCE(c.review_decision,'pending'),COALESCE(c.exclusion_reason,''),COALESCE(c.answer_link_status,'unresolved'),COALESCE(c.answer_link_method,''),COALESCE(c.answer_link_confidence,0),COALESCE(d.marker_top,0) AS source_marker_top,
		COALESCE((SELECT ip.problem_type FROM import_problems ip WHERE ip.session_id=c.import_session_id AND ip.candidate_id=c.id AND ip.status='open' AND ip.problem_type IN ('ANSWER_SOURCE_IDENTITY_INVALID','ANSWER_MISSING','ANSWER_CONFLICT','ANSWER_UNRESOLVED') ORDER BY CASE ip.problem_type WHEN 'ANSWER_CONFLICT' THEN 1 WHEN 'ANSWER_SOURCE_IDENTITY_INVALID' THEN 2 WHEN 'ANSWER_MISSING' THEN 3 ELSE 4 END,ip.id DESC LIMIT 1),'') AS answer_resolution
		FROM candidate_questions c
		LEFT JOIN directory_positions d ON d.session_id=c.import_session_id AND d.document_id=c.document_id AND d.source_page_start=COALESCE(c.question_page_start,c.page_start,0) AND d.source_page_end=COALESCE(c.question_page_end,c.page_end,0) AND d.source_question_no=c.source_question_no AND d.canonical_question_type=COALESCE(NULLIF(c.canonical_question_type,''),c.question_type)
		WHERE c.import_session_id=? AND c.status!='session_duplicate'
		ORDER BY c.question_page_start,CASE WHEN d.marker_top IS NULL THEN 1 ELSE 0 END,d.marker_top,c.source_question_no,c.id LIMIT ? OFFSET ?`, sessionID, sessionID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type draftRow struct {
		id, doc                                                            int64
		status, identity, section, qtype, canonical, scope, merged, issues string
		review, reason, linkStatus, linkMethod, answerResolution           string
		setNo, qno, qs, qe, as, ae, version, markerTop                     int
		conf, linkConfidence                                               float64
	}
	rowsBuffer := make([]draftRow, 0, pageSize)
	for rows.Next() {
		var row draftRow
		if err := rows.Scan(&row.id, &row.status, &row.doc, &row.identity, &row.section, &row.setNo, &row.qno, &row.qtype, &row.canonical, &row.scope, &row.merged, &row.issues, &row.conf, &row.qs, &row.qe, &row.as, &row.ae, &row.version, &row.review, &row.reason, &row.linkStatus, &row.linkMethod, &row.linkConfidence, &row.markerTop, &row.answerResolution); err != nil {
			return nil, 0, err
		}
		rowsBuffer = append(rowsBuffer, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// Do not query candidate_sources while rows is still open. Each active
	// *sql.Rows holds a connection from the pool; doing a nested query here
	// deadlocks when the UI loads four draft pages concurrently and all four
	// connections are waiting for another connection. Buffering the page first
	// also makes the connection lifetime independent of the per-candidate
	// source enrichment below.
	rows.Close()

	out := make([]map[string]interface{}, 0, len(rowsBuffer))
	for _, row := range rowsBuffer {
		var sv, mv, iv interface{}
		_ = json.Unmarshal([]byte(row.section), &sv)
		_ = json.Unmarshal([]byte(row.merged), &mv)
		_ = json.Unmarshal([]byte(row.issues), &iv)
		sources := []map[string]interface{}{}
		var provenance *domain.SourceQuestionOrigin
		provenance, _ = r.GetCandidateProvenance(ctx, sessionID, row.id)
		var answerDocumentID int64
		sourceRows, _ := r.db.QueryContext(ctx, `SELECT document_id,source_role,page_start,page_end,COALESCE(source_json,'') FROM candidate_sources WHERE session_id=? AND candidate_id=? AND source_role IN ('answer','explanation') ORDER BY id`, sessionID, row.id)
		if sourceRows != nil {
			for sourceRows.Next() {
				var sourceDoc int64
				var role, sourceJSON string
				var start, end int
				if sourceRows.Scan(&sourceDoc, &role, &start, &end, &sourceJSON) == nil {
					if answerDocumentID == 0 {
						answerDocumentID = sourceDoc
					}
					sources = append(sources, map[string]interface{}{"document_id": sourceDoc, "source_kind": role, "page_start": start, "page_end": end, "source_json": sourceJSON})
				}
			}
			sourceRows.Close()
		}
		originUID, originalLabel, sequence := "", "", 0
		spans := []domain.SourceEvidenceSpan{}
		hasQuestion, hasAnswer := false, false
		if provenance != nil {
			originUID = provenance.OriginUID
			originalLabel = provenance.OriginalQuestionLabel
			sequence = provenance.SequenceInScope
			spans = provenance.SourceSpans
			for _, span := range spans {
				if span.Status != "active" {
					continue
				}
				if span.SourceRole == "question" {
					hasQuestion = true
				}
				if span.SourceRole == "answer" || span.SourceRole == "explanation" {
					hasAnswer = true
				}
			}
		}
		provenanceComplete := hasQuestion && (hasAnswer || row.linkStatus == "confirmed")
		remainingProblems := []map[string]interface{}{}
		problemRows, _ := r.db.QueryContext(ctx, `SELECT id,problem_type,severity,COALESCE(resolution_hint,'manual_review'),COALESCE(page_start,0),COALESCE(page_end,0),COALESCE(detail_json,'') FROM import_problems WHERE session_id=? AND candidate_id=? AND status='open' ORDER BY id`, sessionID, row.id)
		if problemRows != nil {
			for problemRows.Next() {
				var id int64
				var kind, severity, hint, detail string
				var ps, pe int
				if problemRows.Scan(&id, &kind, &severity, &hint, &ps, &pe, &detail) == nil {
					remainingProblems = append(remainingProblems, map[string]interface{}{"id": id, "problem_type": kind, "severity": severity, "resolution_hint": hint, "page_start": ps, "page_end": pe, "detail_json": detail})
				}
			}
			problemRows.Close()
		}
		out = append(out, map[string]interface{}{"id": row.id, "document_id": row.doc, "answer_document_id": answerDocumentID, "status": row.status, "source_identity": row.identity, "origin_uid": originUID, "original_question_label": originalLabel, "sequence_in_scope": sequence, "provenance_complete": provenanceComplete, "source_spans": spans, "remaining_problems": remainingProblems, "section_path": sv, "set_no": row.setNo, "source_question_no": row.qno, "question_type": row.qtype, "canonical_question_type": row.canonical, "section_scope_id": row.scope, "question": mv, "validation_issues": iv, "visual_confidence": row.conf, "question_page_start": row.qs, "question_page_end": row.qe, "source_marker_top": row.markerTop, "answer_page_start": row.as, "answer_page_end": row.ae, "draft_version": row.version, "review_decision": row.review, "exclusion_reason": row.reason, "answer_link_status": row.linkStatus, "answer_link_method": row.linkMethod, "answer_link_confidence": row.linkConfidence, "answer_resolution": row.answerResolution, "answer_sources": sources})
	}
	return out, total, nil
}

func (r *Repository) SessionSummary(ctx context.Context, sessionID int64) (map[string]interface{}, error) {
	s, err := r.GetImportSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	docs, err := r.ListSessionDocuments(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	problems, err := r.ListImportProblems(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var activeRun *domain.ImportRun
	if s.ActiveRunID != nil {
		activeRun, _ = r.GetImportRun(ctx, *s.ActiveRunID)
	}
	// These counts are read from the candidate rows instead of being inferred
	// from the currently loaded draft page. The frontend refreshes a 1,000+
	// question draft in several requests, so counting the first page briefly
	// displayed a plausible but incorrect number (for example 3 instead of 48).
	// Keep review decisions and problem counts separate: one candidate can have
	// several blocking problems, and a pending manual review need not have one.
	var total, included, excluded, pending, unresolved, needsReview int
	if err := r.db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN review_decision='include' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN review_decision='exclude' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN COALESCE(review_decision,'pending')='pending' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN COALESCE(answer_link_status,'unresolved')!='confirmed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status!='validated' THEN 1 ELSE 0 END),0)
		FROM candidate_questions
		WHERE import_session_id=? AND status!='session_duplicate'`, sessionID).Scan(&total, &included, &excluded, &pending, &unresolved, &needsReview); err != nil {
		return nil, err
	}
	var openProblems, openBlockingProblems int
	if err := r.db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN severity='blocking' THEN 1 ELSE 0 END),0)
		FROM import_problems WHERE session_id=? AND status='open'`, sessionID).Scan(&openProblems, &openBlockingProblems); err != nil {
		return nil, err
	}
	draftStats := map[string]int{
		"total_count":                 total,
		"included_count":              included,
		"excluded_count":              excluded,
		"pending_review_count":        pending,
		"unresolved_answer_count":     unresolved,
		"needs_review_count":          needsReview,
		"open_problem_count":          openProblems,
		"open_blocking_problem_count": openBlockingProblems,
	}
	return map[string]interface{}{"session": s, "documents": docs, "problems": problems, "active_run": activeRun, "draft_stats": draftStats}, nil
}

func (r *Repository) PreviewImportSession(ctx context.Context, sessionID int64) (map[string]interface{}, error) {
	s, err := r.GetImportSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var activeRuns, failedUnits, verifiedPages, mismatchPages int
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_runs WHERE session_id=? AND status IN ('queued','running','merging')`, sessionID).Scan(&activeRuns)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=? AND u.status='failed'`, sessionID).Scan(&failedUnits)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(CASE WHEN status IN ('verified','repaired') THEN 1 END),COUNT(CASE WHEN status='mismatch' THEN 1 END) FROM import_page_coverage WHERE session_id=?`, sessionID).Scan(&verifiedPages, &mismatchPages)
	var included, excluded, pending, includedComplete, unlinked, conflicts int
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' AND review_decision='include'`, sessionID).Scan(&included)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' AND review_decision='exclude'`, sessionID).Scan(&excluded)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' AND COALESCE(review_decision,'pending')='pending'`, sessionID).Scan(&pending)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' AND review_decision='include' AND status='validated' AND answer_link_status='confirmed'`, sessionID).Scan(&includedComplete)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='include' AND COALESCE(answer_link_status,'unresolved')!='confirmed'`, sessionID).Scan(&unlinked)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND status='open' AND problem_type='ANSWER_CONFLICT'`, sessionID).Scan(&conflicts)
	canCommit := activeRuns == 0 && failedUnits == 0 && mismatchPages == 0 && s.ExpectedCount != nil && *s.ExpectedCount == included+excluded && *s.ExpectedCount > 0 && pending == 0 && included == includedComplete && unlinked == 0 && conflicts == 0 && s.BlockingIssueCount == 0
	rows, err := r.db.QueryContext(ctx, `SELECT id,draft_version,status,COALESCE(review_decision,'pending'),COALESCE(exclusion_reason,''),COALESCE(answer_link_status,'unresolved'),COALESCE(merged_json,'') FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for rows.Next() {
		var id int64
		var version int
		var status, decision, reason, linkStatus, raw string
		if err := rows.Scan(&id, &version, &status, &decision, &reason, &linkStatus, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		_, _ = fmt.Fprintf(h, "%d:%d:%s:%s:%s:%s:%s|", id, version, status, decision, reason, linkStatus, raw)
	}
	rows.Close()
	previewHash := fmt.Sprintf("%x", h.Sum(nil))
	_, err = r.db.ExecContext(ctx, `UPDATE import_sessions SET last_preview_hash=?,last_preview_version=?,updated_at=datetime('now') WHERE id=?`, previewHash, s.DraftVersion, sessionID)
	if err != nil {
		return nil, err
	}
	problems, _ := r.ListImportProblems(ctx, sessionID)
	coverageStatus := "unverifiable"
	if mismatchPages > 0 {
		coverageStatus = "mismatch"
	} else if verifiedPages > 0 {
		coverageStatus = "verified"
	}
	return map[string]interface{}{"directory_complete": s.ExpectedCount != nil && mismatchPages == 0, "directory_coverage_status": coverageStatus, "verified_page_count": verifiedPages, "mismatch_page_count": mismatchPages, "expected_count_ready": s.ExpectedCount != nil && mismatchPages == 0, "expected_count": s.ExpectedCount, "unique_count": s.UniqueCount, "complete_count": s.CompleteCount, "blocking_issue_count": s.BlockingIssueCount, "failed_units": failedUnits, "active_runs": activeRuns, "included_count": included, "excluded_count": excluded, "pending_review_count": pending, "included_complete_count": includedComplete, "unlinked_answer_count": unlinked, "answer_conflict_count": conflicts, "can_commit": canCommit, "preview_hash": previewHash, "preview_version": s.DraftVersion, "problems": problems}, nil
}

func (r *Repository) CommitImportSession(ctx context.Context, sessionID int64, expectedVersion int, expectedPreviewHash string) (*VisionCommitResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var bank sql.NullInt64
	var draftVersion, attachmentRevision, unique, complete, blocking, included, excluded, pending, includedComplete, unlinked int
	var expected sql.NullInt64
	var previewHash sql.NullString
	var previewVersion sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT bank_id,draft_version,attachment_revision,expected_count,unique_count,complete_count,blocking_issue_count,last_preview_hash,last_preview_version FROM import_sessions WHERE id=?`, sessionID).Scan(&bank, &draftVersion, &attachmentRevision, &expected, &unique, &complete, &blocking, &previewHash, &previewVersion); err != nil {
		return nil, err
	}
	if draftVersion != expectedVersion {
		return nil, fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='include'`, sessionID).Scan(&included)
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='exclude'`, sessionID).Scan(&excluded)
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND COALESCE(review_decision,'pending')='pending'`, sessionID).Scan(&pending)
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='include' AND status='validated' AND answer_link_status='confirmed'`, sessionID).Scan(&includedComplete)
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='include' AND COALESCE(answer_link_status,'unresolved')!='confirmed'`, sessionID).Scan(&unlinked)
	if !previewVersion.Valid || int(previewVersion.Int64) != draftVersion || !previewHash.Valid || previewHash.String == "" || previewHash.String != expectedPreviewHash {
		return nil, fmt.Errorf("PREVIEW_STALE")
	}
	var activeRuns, failedUnits int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_runs WHERE session_id=? AND status IN ('queued','running','merging')`, sessionID).Scan(&activeRuns)
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=? AND u.status='failed'`, sessionID).Scan(&failedUnits)
	if !bank.Valid || !expected.Valid || expected.Int64 <= 0 || int(expected.Int64) != included+excluded || pending != 0 || included != includedComplete || unlinked != 0 || blocking != 0 || activeRuns != 0 || failedUnits != 0 {
		return nil, fmt.Errorf("IMPORT_PREVIEW_BLOCKED")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,document_id,COALESCE(merged_json,''),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),input_hash FROM candidate_questions WHERE import_session_id=? AND review_decision='include' AND status='validated' AND answer_link_status='confirmed' ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	type item struct {
		id, documentID int64
		raw, inputHash string
		start, end     int
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.id, &v.documentID, &v.raw, &v.start, &v.end, &v.inputHash); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, v)
	}
	rows.Close()
	if len(items) != unique {
		return nil, fmt.Errorf("IMPORT_VALIDATED_COUNT_MISMATCH")
	}
	result := &VisionCommitResult{}
	for _, candidate := range items {
		var q domain.VisionQuestion
		if err := json.Unmarshal([]byte(candidate.raw), &q); err != nil {
			return nil, err
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(q.Stem), ""))
		sum := sha256.Sum256([]byte(normalized))
		sourceHash := fmt.Sprintf("%x", sum[:16])
		var qid int64
		insertedThis := false
		if err := tx.QueryRowContext(ctx, `SELECT id FROM canonical_questions WHERE source_hash=? LIMIT 1`, sourceHash).Scan(&qid); err == nil {
			result.Duplicates++
		} else if err == sql.ErrNoRows {
			options, _ := json.Marshal(q.Options)
			answers, _ := json.Marshal(q.Answer)
			res, err := tx.ExecContext(ctx, `INSERT INTO canonical_questions(bank_id,type,stem,options_json,answer_json,original_analysis,difficulty,knowledge_points_json,quality_score,status,source_hash,version) VALUES(?,?,?,?,?,?,3,'[]',?,'published',?,1)`, bank.Int64, q.QuestionType, q.Stem, string(options), string(answers), q.Analysis, q.VisualConfidence, sourceHash)
			if err != nil {
				return nil, err
			}
			qid, _ = res.LastInsertId()
			insertedThis = true
			result.Inserted++
			result.QuestionIDs = append(result.QuestionIDs, qid)
		} else {
			return nil, err
		}
		sourceRows, err := tx.QueryContext(ctx, `SELECT document_id,page_start,page_end,COALESCE(source_json,'') FROM candidate_sources WHERE candidate_id=? ORDER BY id`, candidate.id)
		if err != nil {
			return nil, err
		}
		sourceCount := 0
		for sourceRows.Next() {
			var docID int64
			var start, end int
			var raw string
			if err := sourceRows.Scan(&docID, &start, &end, &raw); err != nil {
				sourceRows.Close()
				return nil, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO question_sources(canonical_question_id,document_id,candidate_id,page_start,page_end,raw_text,original_answer,original_analysis,source_hash) VALUES(?,?,?,?,?,?,?,?,?)`, qid, docID, candidate.id, start, end, raw, strings.Join(q.Answer, ","), q.Analysis, candidate.inputHash)
			if err != nil {
				sourceRows.Close()
				return nil, err
			}
			sourceCount++
		}
		sourceRows.Close()
		if sourceCount == 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO question_sources(canonical_question_id,document_id,candidate_id,page_start,page_end,raw_text,original_answer,original_analysis,source_hash) VALUES(?,?,?,?,?,?,?,?,?)`, qid, candidate.documentID, candidate.id, candidate.start, candidate.end, candidate.raw, strings.Join(q.Answer, ","), q.Analysis, candidate.inputHash)
			if err != nil {
				return nil, err
			}
		}
		status := "duplicate"
		if insertedThis {
			status = "persisted"
		}
		_, _ = tx.ExecContext(ctx, `UPDATE candidate_questions SET status=? WHERE id=?`, status, candidate.id)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE question_banks SET question_count=(SELECT COUNT(*) FROM canonical_questions WHERE bank_id=?),updated_at=datetime('now') WHERE id=?`, bank.Int64, bank.Int64); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET status='embedding',draft_version=draft_version+1,active_run_id=NULL,updated_at=datetime('now') WHERE id=?`, sessionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) debugSession(_ context.Context, id int64) string {
	return fmt.Sprintf("session:%d", id)
}
