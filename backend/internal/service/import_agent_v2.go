package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

type sessionCandidate struct {
	id, documentID       int64
	identity             string
	section              []string
	sectionScope         string
	setNo, questionNo    int
	questionType, merged string
	pageStart, pageEnd   int
}

func sessionIdentity(section []string, setNo int, questionType string, questionNo int) string {
	// canonicalSectionScope intentionally removes the trailing presentation
	// heading (for example "四、辨析题").  That is correct for linking an
	// answer to a question, but it is too broad for duplicate detection: a
	// single topic can legitimately contain q1 in both a short-answer block
	// and a case-analysis block. Preserve the narrow subjective subtype in the
	// reconciliation identity so those distinct questions are not reported as
	// QUESTION_SOURCE_CONFLICT.
	scope := canonicalSectionScope(section)
	if subtype := sectionSubtypeKey(section, questionType); subtype != "" {
		scope += "|subtype:" + subtype
	}
	return fmt.Sprintf("%s|%d|%s|%d", scope, setNo, domain.CanonicalQuestionType(questionType), questionNo)
}

func sessionIdentityFromScope(scope string, section []string, setNo int, questionType string, questionNo int) string {
	if strings.TrimSpace(scope) == "" {
		return sessionIdentity(section, setNo, questionType, questionNo)
	}
	if subtype := sectionSubtypeKey(section, questionType); subtype != "" {
		scope += "|subtype:" + subtype
	}
	return fmt.Sprintf("%s|%d|%s|%d", scope, setNo, domain.CanonicalQuestionType(questionType), questionNo)
}

func (s *ImportService) reconcileImportSession(ctx context.Context, sessionID int64, requestedRunID ...int64) error {
	session, err := s.repo.GetImportSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == "cancelled" || session.ArchivedAt != nil {
		return nil
	}
	runID := session.ActiveRunID
	if len(requestedRunID) > 0 {
		requested := requestedRunID[0]
		if runID == nil || *runID != requested {
			// A stale worker may finish after a newer run has claimed the
			// session. It can be audited as superseded, but it must not rebuild
			// counters or clear the newer active_run_id.
			_, _ = s.repo.DB().ExecContext(ctx, `UPDATE import_runs SET status='superseded',error_code='STALE_RUN',error_message='新运行已接管会话',finished_at=COALESCE(finished_at,datetime('now')) WHERE id=? AND session_id=? AND status IN ('queued','running','merging')`, requested, sessionID)
			return nil
		}
		runID = &requested
	}
	docs, err := s.repo.ListSessionDocuments(ctx, sessionID)
	if err != nil {
		return err
	}
	allFinished := len(docs) > 0
	directoryComplete := true
	for _, doc := range docs {
		if doc.ImportJobID == nil {
			allFinished = false
			continue
		}
		job, getErr := s.repo.GetImportJob(ctx, *doc.ImportJobID)
		if getErr != nil {
			return getErr
		}
		switch job.Status {
		case domain.ImportStatusDraftReady, domain.ImportStatusNeedsAttention, domain.ImportStatusNeedsModelConfiguration, domain.ImportStatusFailed, domain.ImportStatusCancelled:
		default:
			allFinished = false
		}
		if (doc.DocumentRole == "question_book" || doc.DocumentRole == "mixed" || doc.DocumentRole == "supplement") && !job.DirectoryComplete {
			directoryComplete = false
		}
	}
	if !allFinished {
		if runID != nil {
			_, _ = s.repo.DB().ExecContext(ctx, `UPDATE import_sessions SET status='extracting',updated_at=datetime('now') WHERE id=? AND active_run_id=?`, sessionID, *runID)
		} else {
			_, _ = s.repo.DB().ExecContext(ctx, `UPDATE import_sessions SET status='extracting',updated_at=datetime('now') WHERE id=?`, sessionID)
		}
		return nil
	}
	if runID != nil {
		res, claimErr := s.repo.DB().ExecContext(ctx, `UPDATE import_runs SET status='merging' WHERE id=? AND session_id=? AND status='running'`, *runID, sessionID)
		if claimErr != nil {
			return claimErr
		}
		claimed, _ := res.RowsAffected()
		if claimed == 0 {
			return nil
		}
	}

	db := s.repo.DB()
	// These problem rows are derived projections. Clear the previous run's
	// source conflicts as well as answer/validation issues before rebuilding
	// them; otherwise a corrected scoped identity would remain blocked by stale
	// QUESTION_SOURCE_CONFLICT rows forever.
	_, _ = db.ExecContext(ctx, `DELETE FROM import_problems WHERE session_id=? AND status='open' AND problem_type IN ('DOCUMENT_ROLE_UNCERTAIN','QUESTION_SOURCE_CONFLICT','ANSWER_CONFLICT','ANSWER_REQUIRED','ANSWER_MISSING','ANSWER_SOURCE_IDENTITY_INVALID','ANSWER_UNRESOLVED','QUESTION_INVALID','DIRECTORY_INCOMPLETE')`, sessionID)
	for _, doc := range docs {
		if doc.DocumentRole == "unknown" || doc.RoleConfidence < .45 {
			detail, _ := json.Marshal(map[string]interface{}{"message": "无法可靠判断文档用途，请确认它是题本、答案册、解析册还是补充卷", "document_name": doc.OriginalName})
			_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,detail_json) VALUES(?,?,?,'DOCUMENT_ROLE_UNCERTAIN','blocking',?)`, sessionID, runID, doc.DocumentID, string(detail))
			directoryComplete = false
		}
	}
	if !directoryComplete {
		_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,problem_type,severity,detail_json) VALUES(?,?,'DIRECTORY_INCOMPLETE','blocking','{"message":"一个或多个题本文档目录不完整"}')`, sessionID, runID)
	}

	rows, err := db.QueryContext(ctx, `SELECT id,document_id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),COALESCE(section_scope_id,''),set_no,source_question_no,COALESCE(question_type,''),COALESCE(merged_json,question_json,extracted_json,'{}'),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('persisted','duplicate','session_duplicate') ORDER BY id`, sessionID)
	if err != nil {
		return err
	}
	groups := map[string][]sessionCandidate{}
	for rows.Next() {
		var c sessionCandidate
		var sectionRaw string
		if err := rows.Scan(&c.id, &c.documentID, &c.identity, &sectionRaw, &c.sectionScope, &c.setNo, &c.questionNo, &c.questionType, &c.merged, &c.pageStart, &c.pageEnd); err != nil {
			rows.Close()
			return err
		}
		_ = json.Unmarshal([]byte(sectionRaw), &c.section)
		// A logical scope/number is not a physical source identity. The PDF can
		// legitimately repeat q35 in a single-choice block and a multiple-choice
		// block under the same topic, and overlap windows can expose both. Group
		// by the repaired immutable identity (plus its physical span) so those
		// rows are not turned into QUESTION_SOURCE_CONFLICT.
		key := c.identity
		if strings.TrimSpace(key) == "" {
			key = sessionIdentityFromScope(c.sectionScope, c.section, c.setNo, c.questionType, c.questionNo)
		}
		key = fmt.Sprintf("%s|pages:%d-%d", key, c.pageStart, c.pageEnd)
		groups[key] = append(groups[key], c)
	}
	rows.Close()

	answerRows, err := db.QueryContext(ctx, `SELECT document_id,normalized_section_path,COALESCE(section_scope_id,''),COALESCE(section_path_json,'[]'),set_no,question_type,source_question_no,answer_json,COALESCE(analysis,''),page_start,page_end,source_kind FROM import_answer_records WHERE session_id=? ORDER BY source_kind='explanation' DESC,id`, sessionID)
	if err != nil {
		return err
	}
	type answerRecord struct {
		documentID                 int64
		sectionScope               string
		section                    []string
		answerJSON, analysis, kind string
		start, end                 int
	}
	answers := map[string][]answerRecord{}
	for answerRows.Next() {
		var d int64
		var section, scope, sectionJSON, qtype, answer, analysis, kind string
		var setNo, qno, start, end int
		if err := answerRows.Scan(&d, &section, &scope, &sectionJSON, &setNo, &qtype, &qno, &answer, &analysis, &start, &end, &kind); err != nil {
			answerRows.Close()
			return err
		}
		if strings.TrimSpace(scope) == "" {
			scope = section
		}
		var sectionParts []string
		_ = json.Unmarshal([]byte(sectionJSON), &sectionParts)
		key := sessionIdentityFromScope(scope, sectionParts, setNo, qtype, qno)
		answers[key] = append(answers[key], answerRecord{documentID: d, sectionScope: scope, section: sectionParts, answerJSON: answer, analysis: analysis, kind: kind, start: start, end: end})
	}
	answerRows.Close()

	uniqueCount, completeCount := 0, 0
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		items := groups[key]
		primary := items[0]
		uniqueCount++
		var q domain.VisionQuestion
		if json.Unmarshal([]byte(primary.merged), &q) != nil {
			continue
		}
		for _, other := range items[1:] {
			var oq domain.VisionQuestion
			_ = json.Unmarshal([]byte(other.merged), &oq)
			// A retry of the same physical source can produce harmless OCR
			// variation (考入/考人, punctuation, spacing). It is still the same
			// source identity and must not become a new blocking conflict. Keep
			// conflicts for distinct documents or distinct page spans, where two
			// different source texts genuinely need human review.
			samePhysicalSource := primary.documentID == other.documentID && primary.pageStart == other.pageStart && primary.pageEnd == other.pageEnd
			if samePhysicalSource || comparableQuestionStem(q.Stem) == comparableQuestionStem(oq.Stem) {
				_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET status='session_duplicate',updated_at=datetime('now') WHERE id=?`, other.id)
				_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID, primary.id, other.documentID, "question", other.pageStart, other.pageEnd, other.merged)
			} else {
				detail, _ := json.Marshal(map[string]interface{}{"identity": key, "candidate_ids": []int64{primary.id, other.id}})
				_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,candidate_id,source_identity,detail_json) VALUES(?,?,?,'QUESTION_SOURCE_CONFLICT','blocking',?,?,?)`, sessionID, runID, other.documentID, primary.id, key, string(detail))
			}
		}
		records := answers[key]
		distinct := map[string]answerRecord{}
		for _, record := range records {
			var values []string
			_ = json.Unmarshal([]byte(record.answerJSON), &values)
			canonical := strings.Join(values, ",")
			existing, ok := distinct[canonical]
			if !ok || len(record.analysis) > len(existing.analysis) {
				distinct[canonical] = record
			}
		}
		if len(distinct) == 1 {
			for _, record := range distinct {
				_ = json.Unmarshal([]byte(record.answerJSON), &q.Answer)
				if record.analysis != "" {
					q.Analysis = record.analysis
				}
				merged, _ := json.Marshal(q)
				_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,answer_json=?,answer_page_start=?,answer_page_end=?,updated_at=datetime('now') WHERE id=?`, string(merged), string(merged), record.answerJSON, record.start, record.end, primary.id)
				_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID, primary.id, record.documentID, record.kind, record.start, record.end, record.answerJSON)
			}
		} else if len(distinct) > 1 {
			detail, _ := json.Marshal(map[string]interface{}{"identity": key, "answers": distinct})
			_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,problem_type,severity,candidate_id,source_identity,detail_json) VALUES(?,?,'ANSWER_CONFLICT','blocking',?,?,?)`, sessionID, runID, primary.id, key, string(detail))
		}
		issues := validateVisionQuestion(q)
		if requiresAnswer(q.QuestionType) && len(q.Answer) == 0 {
			issues = append(issues, issue("ANSWER_REQUIRED", q.SourcePageStart, q.SourcePageEnd, "选择题或判断题缺少印刷答案"))
		}
		if len(issues) == 0 {
			completeCount++
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET status='validated',validation_issues_json='[]' WHERE id=?`, primary.id)
		} else {
			raw, _ := json.Marshal(issues)
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET status='needs_review',validation_issues_json=? WHERE id=?`, string(raw), primary.id)
			for _, item := range issues {
				detail, _ := json.Marshal(item)
				_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,problem_type,severity,candidate_id,source_identity,detail_json) VALUES(?,?,'QUESTION_INVALID','blocking',?,?,?)`, sessionID, runID, primary.id, key, string(detail))
			}
		}
	}
	expected, err := countSessionDirectoryIdentities(ctx, db, sessionID)
	if err != nil {
		return err
	}
	var blocking int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND status='open' AND severity='blocking'`, sessionID).Scan(&blocking)
	status := "draft_ready"
	var expectedValue interface{} = expected
	if !directoryComplete || blocking > 0 || expected == 0 || expected != uniqueCount || uniqueCount != completeCount {
		status = "needs_attention"
	}
	if !directoryComplete {
		expectedValue = nil
	}
	newVersion := session.DraftVersion + 1
	if runID != nil {
		if run, getErr := s.repo.GetImportRun(ctx, *runID); getErr == nil && run.RunType == "candidate_review" {
			// CandidateReview finalizes the session version once after the batch
			// patches are applied. Do not increment it again during projection
			// reconciliation.
			newVersion = session.DraftVersion
		}
	}
	_, err = db.ExecContext(ctx, `UPDATE import_sessions SET status=?,draft_version=?,expected_count=?,unique_count=?,complete_count=?,blocking_issue_count=?,active_run_id=NULL,last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=?`, status, newVersion, expectedValue, uniqueCount, completeCount, blocking, sessionID)
	if err != nil {
		return err
	}
	if runID != nil {
		_ = s.repo.FinishSessionRun(ctx, sessionID, *runID, "completed", "", "", newVersion)
	}
	summary := fmt.Sprintf("本轮识别完成：预期 %v 题，唯一 %d 题，完整 %d 题，阻断 %d 项。", expectedValue, uniqueCount, completeCount, blocking)
	messageID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: sessionID, RunID: runID, Role: "assistant", MessageType: "agent_run", Content: summary, MetadataJSON: fmt.Sprintf(`{"status":%q,"expected_count":%d,"unique_count":%d,"complete_count":%d,"blocking":%d}`, status, expected, uniqueCount, completeCount, blocking), Status: "completed"})
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: sessionID, RunID: runID, MessageID: &messageID, Type: "run_completed", Stage: status, AgentRole: "ValidationAgent", Summary: summary})
	// The legacy reconciliation pass is retained for backwards-compatible
	// duplicate/source bookkeeping. Canonical answer identity and review state
	// are then rebuilt by the auditable relinker, which never uses suffix paths.
	var relinkRunID int64
	if runID != nil {
		relinkRunID = *runID
	}
	return s.RelinkSessionAnswers(ctx, sessionID, relinkRunID)
}

func comparableQuestionStem(value string) string {
	value = compactText(value)
	// Extraction retries sometimes include the printed number in the stem and
	// sometimes omit it. Remove only a leading numeric marker; the identity
	// group has already established that both rows refer to the same question
	// number, so this cannot merge different numbered questions.
	runes := []rune(value)
	i := 0
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	if i > 0 && i < len(runes) {
		switch runes[i] {
		case '.', ')', '、':
			runes = runes[i+1:]
		}
	}
	value = string(runes)
	return value
}

// countSessionDirectoryIdentities counts the durable physical directory
// identities. A logical topic/number key is insufficient because the source
// PDF can repeat the same printed number in different question blocks.
func countSessionDirectoryIdentities(ctx context.Context, db *sql.DB, sessionID int64) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT document_id,COALESCE(source_identity,''),source_page_start,source_page_end FROM import_directory_entries WHERE session_id=? AND content_role='question' AND status='active'`, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var documentID, pageStart, pageEnd int
		var identity string
		if err := rows.Scan(&documentID, &identity, &pageStart, &pageEnd); err != nil {
			return 0, err
		}
		seen[fmt.Sprintf("%d|%s|%d-%d", documentID, strings.TrimSpace(identity), pageStart, pageEnd)] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return len(seen), nil
}

func compactText(value string) string {
	// Normalize the punctuation variants commonly produced by different PDF
	// fonts. In particular, full-width parentheses in the question book and
	// ASCII parentheses in the answer book must share one section identity.
	value = strings.NewReplacer(
		"（", "(", "）", ")", "【", "[", "】", "]", "：", ":",
		"·", "", "•", "",
		"，", ",", "。", ".", "；", ";", "／", "/",
	).Replace(value)
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

// matchScopeKey is a derived comparison key. Raw section paths and immutable
// source identities remain untouched for provenance and display.
func matchScopeKey(value string) string {
	value = compactText(value)
	return strings.NewReplacer("《", "", "》", "", "〈", "", "〉", "", "(", "", ")", "", "[", "", "]", "", ":", "", ",", "", ".", "", ";", "").Replace(value)
}

var _ = sql.ErrNoRows
