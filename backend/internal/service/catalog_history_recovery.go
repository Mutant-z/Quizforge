package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// AuditSessionCatalogCoverage audits the current durable directory without a
// model call. It is useful for old sessions created before coverage rows were
// introduced.
func (s *ImportService) AuditSessionCatalogCoverage(ctx context.Context, sessionID int64) ([]pageCoverageResult, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `SELECT document_id FROM import_session_documents WHERE session_id=? ORDER BY document_id`, sessionID)
	if err != nil {
		return nil, err
	}
	documents := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			documents = append(documents, id)
		}
	}
	rows.Close()
	all := []pageCoverageResult{}
	for _, documentID := range documents {
		entryRows, err := s.repo.DB().QueryContext(ctx, `SELECT content_role,section_path_json,set_no,question_type,canonical_question_type,section_scope_id,sequence_in_scope,COALESCE(original_question_label,''),source_question_no,source_page_start,source_page_end,COALESCE(anchor,''),COALESCE(marker_top,0),COALESCE(marker_bottom,0),COALESCE(region_json,''),source_identity FROM import_directory_entries WHERE session_id=? AND document_id=? AND status='active'`, sessionID, documentID)
		if err != nil {
			return nil, err
		}
		entries := map[string]domain.VisionDirectoryEntry{}
		for entryRows.Next() {
			var entry domain.VisionDirectoryEntry
			var sectionJSON, regionJSON string
			if err := entryRows.Scan(&entry.ContentRole, &sectionJSON, &entry.SetNo, &entry.QuestionType, &entry.CanonicalQuestionType, &entry.SectionScopeID, &entry.SequenceInScope, &entry.OriginalQuestionLabel, &entry.SourceQuestionNo, &entry.SourcePageStart, &entry.SourcePageEnd, &entry.Anchor, &entry.MarkerTop, &entry.MarkerBottom, &regionJSON, &entry.SourceIdentity); err != nil {
				entryRows.Close()
				return nil, err
			}
			_ = json.Unmarshal([]byte(sectionJSON), &entry.SectionPath)
			if regionJSON != "" && regionJSON != "null" {
				var region domain.VisionRegion
				if json.Unmarshal([]byte(regionJSON), &region) == nil {
					entry.Region = &region
				}
			}
			entries[entry.SourceIdentity] = entry
		}
		entryRows.Close()
		mismatches, err := s.verifyCatalogCoverage(ctx, sessionID, documentID, 0, entries)
		if err != nil {
			return nil, err
		}
		all = append(all, mismatches...)
	}
	return all, nil
}

// RecoverCatalogGapsFromIdenticalDocumentSessions reuses only question
// extraction evidence from an earlier validated session over the exact same
// documents table row/file hash. Answers and review decisions are not copied;
// the current session relinks against its own answer records afterward.
func (s *ImportService) RecoverCatalogGapsFromIdenticalDocumentSessions(ctx context.Context, sessionID int64, mismatches []pageCoverageResult) (int, error) {
	if len(mismatches) == 0 {
		return 0, nil
	}
	db := s.repo.DB()
	recovered := 0
	for _, mismatch := range mismatches {
		if mismatch.Role != "question" {
			continue
		}
		var documentID, jobID, sourceSession int64
		if err := db.QueryRowContext(ctx, `SELECT sd.document_id,COALESCE(sd.import_job_id,0) FROM import_session_documents sd WHERE sd.session_id=? AND EXISTS(SELECT 1 FROM import_directory_entries de WHERE de.session_id=sd.session_id AND de.document_id=sd.document_id AND de.source_page_start=?) ORDER BY sd.id LIMIT 1`, sessionID, mismatch.Page).Scan(&documentID, &jobID); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return recovered, err
		}
		if jobID == 0 {
			continue
		}
		if err := db.QueryRowContext(ctx, `SELECT c.import_session_id FROM candidate_questions c JOIN import_sessions s ON s.id=c.import_session_id WHERE c.document_id=? AND c.import_session_id<>? AND c.question_page_start=? AND c.status='validated' GROUP BY c.import_session_id ORDER BY COUNT(*) DESC,s.updated_at DESC LIMIT 1`, documentID, sessionID, mismatch.Page).Scan(&sourceSession); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return recovered, err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return recovered, err
		}
		candidateRows, err := tx.QueryContext(ctx, `SELECT c.id,c.source_question_no FROM candidate_questions c WHERE c.import_session_id=? AND c.document_id=? AND c.question_page_start=? AND c.status='validated' AND NOT EXISTS(SELECT 1 FROM candidate_questions current WHERE current.import_session_id=? AND current.document_id=c.document_id AND (current.source_identity=c.source_identity OR (current.match_scope_key=c.match_scope_key AND current.canonical_question_type=c.canonical_question_type AND current.source_question_no=c.source_question_no)))`, sourceSession, documentID, mismatch.Page, sessionID)
		if err != nil {
			tx.Rollback()
			return recovered, err
		}
		ids := []int64{}
		for candidateRows.Next() {
			var id int64
			var number int
			if candidateRows.Scan(&id, &number) == nil && containsCoverageNumber(mismatch.Missing, number) {
				ids = append(ids, id)
			}
		}
		candidateRows.Close()
		for _, sourceID := range ids {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO import_directory_entries(session_id,document_id,unit_id,content_role,document_role,section_path_json,normalized_section_path,set_no,question_type,canonical_question_type,section_scope_id,sequence_in_scope,original_question_label,match_scope_key,source_question_no,source_page_start,source_page_end,anchor,marker_top,marker_bottom,region_json,source_identity,status)
				SELECT ?,de.document_id,NULL,de.content_role,de.document_role,de.section_path_json,de.normalized_section_path,de.set_no,de.question_type,de.canonical_question_type,de.section_scope_id,de.sequence_in_scope,de.original_question_label,de.match_scope_key,de.source_question_no,de.source_page_start,de.source_page_end,de.anchor,de.marker_top,de.marker_bottom,de.region_json,de.source_identity,'active' FROM import_directory_entries de JOIN candidate_questions c ON c.import_session_id=de.session_id AND c.source_identity=de.source_identity WHERE c.id=?`, sessionID, sourceID); err != nil {
				tx.Rollback()
				return recovered, err
			}
			res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_questions(document_id,import_job_id,page_start,page_end,raw_text,normalized_text,input_hash,status,confidence,extracted_json,source_identity,section_path_json,set_no,source_question_no,question_type,question_json,answer_json,merged_json,question_page_start,question_page_end,visual_confidence,validation_issues_json,draft_version,pipeline_version,import_session_id,canonical_question_type,section_scope_id,match_scope_key,answer_link_status,review_decision)
				SELECT document_id,?,page_start,page_end,raw_text,normalized_text,input_hash,'needs_review',confidence,question_json,source_identity,section_path_json,set_no,source_question_no,question_type,question_json,'[]',question_json,question_page_start,question_page_end,visual_confidence,'[]',1,pipeline_version,?,canonical_question_type,section_scope_id,match_scope_key,'unresolved','pending' FROM candidate_questions WHERE id=?`, jobID, sessionID, sourceID)
			if err != nil {
				tx.Rollback()
				return recovered, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				recovered++
			}
		}
		if err := tx.Commit(); err != nil {
			return recovered, err
		}
	}
	if recovered > 0 {
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO source_question_origins(origin_uid,session_id,candidate_id,question_document_id,document_file_hash,source_identity,original_question_label,original_question_no,sequence_in_scope,section_path_json,section_scope_id,set_no,raw_question_type,canonical_question_type,identity_confidence,status)
			SELECT 'qf-origin:'||?||':'||c.id,?,c.id,c.document_id,COALESCE(d.file_hash,''),c.source_identity,COALESCE(de.original_question_label,CAST(c.source_question_no AS TEXT)),c.source_question_no,COALESCE(de.sequence_in_scope,c.source_question_no),c.section_path_json,c.section_scope_id,c.set_no,c.question_type,c.canonical_question_type,'verified_history','active' FROM candidate_questions c JOIN documents d ON d.id=c.document_id LEFT JOIN import_directory_entries de ON de.session_id=c.import_session_id AND de.source_identity=c.source_identity WHERE c.import_session_id=?`, sessionID, sessionID, sessionID); err != nil {
			return recovered, err
		}
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,document_file_hash,page_start,page_end,regions_json,marker_top,marker_bottom,anchor_text,directory_entry_id,content_hash,extraction_confidence,status)
			SELECT o.id,c.id,'question',c.document_id,o.document_file_hash,c.question_page_start,c.question_page_end,CASE WHEN COALESCE(de.region_json,'')='' THEN '[]' ELSE '['||de.region_json||']' END,de.marker_top,de.marker_bottom,COALESCE(de.anchor,''),de.id,c.source_identity||':question:'||c.question_page_start||':'||c.question_page_end,COALESCE(c.visual_confidence,0.8),'active' FROM candidate_questions c JOIN source_question_origins o ON o.session_id=c.import_session_id AND o.candidate_id=c.id LEFT JOIN import_directory_entries de ON de.session_id=c.import_session_id AND de.source_identity=c.source_identity WHERE c.import_session_id=?`, sessionID); err != nil {
			return recovered, err
		}
	}
	return recovered, nil
}

func (s *ImportService) RecoverAnswerGapsFromIdenticalDocumentSessions(ctx context.Context, sessionID int64, mismatches []pageCoverageResult) (int, error) {
	db := s.repo.DB()
	recovered := 0
	for _, mismatch := range mismatches {
		if mismatch.Role == "question" {
			continue
		}
		var documentID, sourceSession int64
		if err := db.QueryRowContext(ctx, `SELECT document_id FROM import_session_documents WHERE session_id=? ORDER BY id LIMIT 1`, sessionID).Scan(&documentID); err != nil {
			return recovered, err
		}
		if err := db.QueryRowContext(ctx, `SELECT session_id FROM import_answer_records WHERE document_id=? AND session_id<>? AND page_start=? AND trim(COALESCE(source_identity,''))<>'' AND canonical_question_type IN ('single_choice','multiple_choice','true_false','fill_in_blank','subjective') GROUP BY session_id ORDER BY session_id DESC LIMIT 1`, documentID, sessionID, mismatch.Page).Scan(&sourceSession); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return recovered, err
		}
		for _, number := range mismatch.Missing {
			rows, err := db.QueryContext(ctx, `SELECT id FROM import_answer_records old WHERE old.session_id=? AND old.document_id=? AND old.page_start=? AND old.source_question_no=? AND NOT EXISTS(SELECT 1 FROM import_answer_records current WHERE current.session_id=? AND current.document_id=old.document_id AND current.match_scope_key=old.match_scope_key AND current.canonical_question_type=old.canonical_question_type AND current.source_question_no=old.source_question_no)`, sourceSession, documentID, mismatch.Page, number, sessionID)
			if err != nil {
				return recovered, err
			}
			ids := []int64{}
			for rows.Next() {
				var id int64
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			for _, id := range ids {
				res, err := db.ExecContext(ctx, `INSERT INTO import_answer_records(session_id,document_id,run_id,normalized_section_path,section_path_json,set_no,question_type,source_question_no,answer_json,analysis,page_start,page_end,source_kind,canonical_question_type,section_scope_id,link_status,source_identity,match_scope_key)
					SELECT ?,document_id,NULL,normalized_section_path,section_path_json,set_no,question_type,source_question_no,answer_json,analysis,page_start,page_end,source_kind,canonical_question_type,section_scope_id,'unresolved',source_identity,match_scope_key FROM import_answer_records WHERE id=?`, sessionID, id)
				if err != nil {
					return recovered, err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					recovered++
				}
				_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO import_directory_entries(session_id,document_id,content_role,document_role,section_path_json,normalized_section_path,set_no,question_type,canonical_question_type,section_scope_id,sequence_in_scope,original_question_label,match_scope_key,source_question_no,source_page_start,source_page_end,anchor,marker_top,marker_bottom,region_json,source_identity,status)
					SELECT ?,de.document_id,de.content_role,de.document_role,de.section_path_json,de.normalized_section_path,de.set_no,de.question_type,de.canonical_question_type,de.section_scope_id,de.sequence_in_scope,de.original_question_label,de.match_scope_key,de.source_question_no,de.source_page_start,de.source_page_end,de.anchor,de.marker_top,de.marker_bottom,de.region_json,de.source_identity,'active' FROM import_directory_entries de JOIN import_answer_records ar ON ar.session_id=de.session_id AND ar.source_identity=de.source_identity WHERE ar.id=?`, sessionID, id)
			}
		}
	}
	return recovered, nil
}

func (s *ImportService) RepairInvalidCandidateBodiesFromIdenticalDocumentSessions(ctx context.Context, sessionID int64) (int, error) {
	db := s.repo.DB()
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT c.id,c.document_id,c.question_page_start,c.source_question_no,c.canonical_question_type,c.section_scope_id,COALESCE(c.question_json,'{}'),COALESCE(c.validation_issues_json,'[]') FROM candidate_questions c JOIN import_problems p ON p.candidate_id=c.id AND p.session_id=c.import_session_id WHERE c.import_session_id=? AND p.status='open' AND p.problem_type='QUESTION_INVALID'`, sessionID)
	if err != nil {
		return 0, err
	}
	type target struct {
		id, document                         int64
		page, number                         int
		canonical, scope, before, validation string
	}
	targets := []target{}
	for rows.Next() {
		var v target
		if rows.Scan(&v.id, &v.document, &v.page, &v.number, &v.canonical, &v.scope, &v.before, &v.validation) == nil {
			targets = append(targets, v)
		}
	}
	rows.Close()
	repaired := 0
	for _, v := range targets {
		var sourceJSON string
		err := db.QueryRowContext(ctx, `SELECT c.question_json FROM candidate_questions c WHERE c.document_id=? AND c.import_session_id<>? AND c.question_page_start=? AND c.source_question_no=? AND c.canonical_question_type=? AND c.section_scope_id=? AND c.status='validated' AND json_array_length(json_extract(c.question_json,'$.options'))>=2 GROUP BY c.question_json HAVING COUNT(*)>=1 ORDER BY MAX(c.import_session_id) DESC LIMIT 1`, v.document, sessionID, v.page, v.number, v.canonical, v.scope).Scan(&sourceJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return repaired, err
		}
		var q domain.VisionQuestion
		if json.Unmarshal([]byte(sourceJSON), &q) != nil {
			continue
		}
		var current domain.VisionQuestion
		_ = json.Unmarshal([]byte(v.before), &current)
		q.SourceIdentity = current.SourceIdentity
		q.Answer = nil
		q.Analysis = ""
		after, _ := json.Marshal(q)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return repaired, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_revision_history(candidate_id,target_role,before_json,after_json,change_status,validation_before_json,validation_after_json,problems_before_json,problems_after_json) VALUES(?,'question',?,?,'updated',?,'[]',(SELECT COALESCE(json_group_array(json_object('problem_type',problem_type,'detail_json',detail_json)),'[]') FROM import_problems WHERE candidate_id=? AND status='open'),'[]')`, v.id, v.before, string(after), v.validation, v.id); err != nil {
			tx.Rollback()
			return repaired, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE candidate_questions SET question_json=?,extracted_json=?,merged_json=?,answer_json='[]',status='needs_review',validation_issues_json='[]',answer_link_status='unresolved',review_decision='pending',draft_version=draft_version+1,updated_at=datetime('now') WHERE id=?`, string(after), string(after), string(after), v.id); err != nil {
			tx.Rollback()
			return repaired, err
		}
		if err = tx.Commit(); err != nil {
			return repaired, err
		}
		repaired++
	}
	return repaired, nil
}

func containsCoverageNumber(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func formatCoverageMismatches(items []pageCoverageResult) string {
	encoded, _ := json.Marshal(items)
	return fmt.Sprintf("%s", encoded)
}
