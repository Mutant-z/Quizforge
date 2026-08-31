package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// UpsertQuestionProvenance creates the immutable origin once and refreshes the
// active question evidence projection. Recognition retries reuse origin_uid.
func (r *Repository) UpsertQuestionProvenance(ctx context.Context, sessionID, candidateID, documentID int64, entry domain.VisionDirectoryEntry, question domain.VisionQuestion, runID *int64) error {
	if sessionID == 0 || candidateID == 0 || question.SourcePageStart <= 0 || question.SourcePageEnd < question.SourcePageStart {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var revision int
	var fileHash string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,'') FROM documents d LEFT JOIN import_session_documents sd ON sd.document_id=d.id AND sd.session_id=? WHERE d.id=?`, sessionID, documentID).Scan(&revision, &fileHash)
	label := entry.OriginalQuestionLabel
	if label == "" && entry.SourceQuestionNo > 0 {
		label = fmt.Sprint(entry.SourceQuestionNo)
	}
	sectionJSON, _ := json.Marshal(entry.SectionPath)
	originUID := fmt.Sprintf("qf-origin:%d:%d", sessionID, candidateID)
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_question_origins(origin_uid,session_id,candidate_id,question_document_id,attachment_revision,document_file_hash,source_identity,original_question_label,original_question_no,sequence_in_scope,section_path_json,section_scope_id,set_no,raw_question_type,canonical_question_type,identity_confidence,status)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'active')`,
		originUID, sessionID, candidateID, documentID, revision, fileHash, entry.SourceIdentity, label, entry.SourceQuestionNo, entry.SequenceInScope, string(sectionJSON), entry.SectionScopeID, entry.SetNo, entry.QuestionType, domain.CanonicalQuestionType(entry.QuestionType), confidenceLabel(entry.OriginalQuestionLabel))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE source_question_origins SET question_document_id=?,attachment_revision=?,document_file_hash=?,source_identity=?,original_question_label=CASE WHEN original_question_label='' THEN ? ELSE original_question_label END,original_question_no=?,sequence_in_scope=?,section_path_json=?,section_scope_id=?,set_no=?,raw_question_type=?,canonical_question_type=?,updated_at=datetime('now') WHERE session_id=? AND candidate_id=?`, documentID, revision, fileHash, entry.SourceIdentity, label, entry.SourceQuestionNo, entry.SequenceInScope, string(sectionJSON), entry.SectionScopeID, entry.SetNo, entry.QuestionType, domain.CanonicalQuestionType(entry.QuestionType), sessionID, candidateID)
	if err != nil {
		return err
	}
	var originID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM source_question_origins WHERE session_id=? AND candidate_id=?`, sessionID, candidateID).Scan(&originID); err != nil {
		return err
	}
	var directoryID, unitID sql.NullInt64
	var markerTop, markerBottom sql.NullInt64
	var anchor, regionJSON string
	_ = tx.QueryRowContext(ctx, `SELECT id,unit_id,marker_top,marker_bottom,COALESCE(anchor,''),COALESCE(region_json,'') FROM import_directory_entries WHERE session_id=? AND document_id=? AND source_identity=? AND content_role='question' AND status='active' ORDER BY id LIMIT 1`, sessionID, documentID, entry.SourceIdentity).Scan(&directoryID, &unitID, &markerTop, &markerBottom, &anchor, &regionJSON)
	regions := "[]"
	if regionJSON != "" {
		regions = "[" + regionJSON + "]"
	}
	contentHash := hashText(fmt.Sprintf("%s|question|%d|%d|%s", entry.SourceIdentity, question.SourcePageStart, question.SourcePageEnd, question.Stem))
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,page_start,page_end,regions_json,marker_top,marker_bottom,anchor_text,directory_entry_id,agent_unit_id,agent_run_id,content_hash,extraction_confidence,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'active')`, originID, candidateID, "question", documentID, revision, fileHash, question.SourcePageStart, question.SourcePageEnd, regions, nullableNullInt(markerTop), nullableNullInt(markerBottom), anchor, nullableNullInt(directoryID), nullableNullInt(unitID), nullableInt64Ptr(runID), contentHash, question.VisualConfidence)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func confidenceLabel(original string) string {
	if original != "" {
		return "explicit"
	}
	return "derived"
}

func nullableInt64Ptr(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// UpsertCandidateEvidence records answer/explanation evidence without changing
// the immutable question origin.
func (r *Repository) UpsertCandidateEvidence(ctx context.Context, sessionID, candidateID, documentID int64, role string, pageStart, pageEnd int, content string, confidence float64, runID *int64) error {
	if pageStart <= 0 || pageEnd < pageStart || (role != "question" && role != "answer" && role != "explanation") {
		return nil
	}
	var originID int64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM source_question_origins WHERE session_id=? AND candidate_id=?`, sessionID, candidateID).Scan(&originID); err != nil {
		return err
	}
	var revision int
	var fileHash string
	_ = r.db.QueryRowContext(ctx, `SELECT COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,'') FROM documents d LEFT JOIN import_session_documents sd ON sd.document_id=d.id AND sd.session_id=? WHERE d.id=?`, sessionID, documentID).Scan(&revision, &fileHash)
	contentHash := hashText(fmt.Sprintf("%s|%d|%d|%s", role, pageStart, pageEnd, content))
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,page_start,page_end,regions_json,agent_run_id,content_hash,extraction_confidence,status) VALUES(?,?,?,?,?,?,?,?, '[]',?,?,?,'active')`, originID, candidateID, role, documentID, revision, fileHash, pageStart, pageEnd, nullableInt64Ptr(runID), contentHash, confidence)
	return err
}

func (r *Repository) GetCandidateProvenance(ctx context.Context, sessionID, candidateID int64) (*domain.SourceQuestionOrigin, error) {
	o := new(domain.SourceQuestionOrigin)
	var candidate, canonical, document sql.NullInt64
	var sectionJSON string
	err := r.db.QueryRowContext(ctx, `SELECT id,origin_uid,session_id,candidate_id,canonical_question_id,question_document_id,attachment_revision,document_file_hash,source_identity,identity_version,original_question_label,original_question_no,sequence_in_scope,section_path_json,section_scope_id,set_no,raw_question_type,canonical_question_type,identity_confidence,status FROM source_question_origins WHERE session_id=? AND candidate_id=?`, sessionID, candidateID).Scan(&o.ID, &o.OriginUID, &o.SessionID, &candidate, &canonical, &document, &o.AttachmentRevision, &o.DocumentFileHash, &o.SourceIdentity, &o.IdentityVersion, &o.OriginalQuestionLabel, &o.OriginalQuestionNo, &o.SequenceInScope, &sectionJSON, &o.SectionScopeID, &o.SetNo, &o.RawQuestionType, &o.CanonicalQuestionType, &o.IdentityConfidence, &o.Status)
	if err != nil {
		return nil, err
	}
	if candidate.Valid {
		v := candidate.Int64
		o.CandidateID = &v
	}
	if canonical.Valid {
		v := canonical.Int64
		o.CanonicalQuestionID = &v
	}
	if document.Valid {
		v := document.Int64
		o.QuestionDocumentID = &v
	}
	_ = json.Unmarshal([]byte(sectionJSON), &o.SectionPath)
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.origin_id,s.candidate_id,s.canonical_question_id,s.source_role,s.document_id,COALESCE(d.original_name,''),s.attachment_revision,s.document_file_hash,s.page_start,s.page_end,s.regions_json,s.marker_top,s.marker_bottom,s.anchor_text,s.page_asset_render_version,s.page_asset_sha256,s.content_hash,s.extraction_confidence,s.status FROM source_evidence_spans s JOIN documents d ON d.id=s.document_id WHERE s.origin_id=? ORDER BY CASE s.source_role WHEN 'question' THEN 1 WHEN 'answer' THEN 2 ELSE 3 END,s.page_start,s.id`, o.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.SourceEvidenceSpan
		var c, q, mt, mb sql.NullInt64
		var regions string
		if err = rows.Scan(&s.ID, &s.OriginID, &c, &q, &s.SourceRole, &s.DocumentID, &s.DocumentName, &s.AttachmentRevision, &s.DocumentFileHash, &s.PageStart, &s.PageEnd, &regions, &mt, &mb, &s.AnchorText, &s.PageAssetRenderVersion, &s.PageAssetSHA256, &s.ContentHash, &s.ExtractionConfidence, &s.Status); err != nil {
			return nil, err
		}
		if c.Valid {
			v := c.Int64
			s.CandidateID = &v
		}
		if q.Valid {
			v := q.Int64
			s.CanonicalQuestionID = &v
		}
		if mt.Valid {
			v := int(mt.Int64)
			s.MarkerTop = &v
		}
		if mb.Valid {
			v := int(mb.Int64)
			s.MarkerBottom = &v
		}
		_ = json.Unmarshal([]byte(regions), &s.Regions)
		o.SourceSpans = append(o.SourceSpans, s)
	}
	return o, rows.Err()
}

func (r *Repository) UpdateImportRunResult(ctx context.Context, runID int64, resultStatus string, modelSuccess, updated, resolved, remaining int, summary interface{}) error {
	payload, _ := json.Marshal(summary)
	_, err := r.db.ExecContext(ctx, `UPDATE import_runs SET result_status=?,model_success_count=?,candidate_updated_count=?,resolved_candidate_count=?,remaining_blocked_count=?,result_summary_json=?,updated_at=datetime('now') WHERE id=?`, resultStatus, modelSuccess, updated, resolved, remaining, string(payload), runID)
	return err
}
