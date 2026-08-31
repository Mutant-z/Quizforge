package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// Regression coverage for the pending-candidate supplement flow: answer-side
// blockers must be repairable instead of being re-recognized as question
// content and silently left blocked.

func seedReviewFixture(t *testing.T, sessionName string) (*importReviewFixture, error) {
	t.Helper()
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, sessionName)
	if err != nil {
		return nil, err
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "book.pdf", FilePath: "/tmp/review-fixture.pdf", FileSize: 1, FileHash: sessionName, MimeType: "application/pdf"})
	if err != nil {
		return nil, err
	}
	_, job, _, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		return nil, err
	}
	return &importReviewFixture{repo: repo, ctx: ctx, sessionID: session.ID, documentID: doc.ID, jobID: job.ID}, nil
}

type importReviewFixture struct {
	repo       *sqlite.Repository
	ctx        context.Context
	sessionID  int64
	documentID int64
	jobID      int64
}

func nullableInt(value int64) interface{} {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableText(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (f *importReviewFixture) insertCandidate(t *testing.T, inputHash, stem string, qno, setNo int64, merged, answerJSON string, linkMethod, linkStatus string, answerStart, answerEnd int64) int64 {
	t.Helper()
	questionJSON := `{"source_identity":"专题|question|` + itoa(qno) + `","stem":"` + stem + `","options":[],"question_type":"true_false","answer":[],"source_page_start":50,"source_page_end":50}`
	identity := `["专题一","判断题"]|question|` + itoa(setNo) + `|true_false|` + itoa(qno)
	_, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO candidate_questions(document_id,import_job_id,page_start,page_end,raw_text,input_hash,status,source_identity,section_path_json,set_no,source_question_no,question_type,canonical_question_type,section_scope_id,question_json,merged_json,answer_json,question_page_start,question_page_end,answer_page_start,answer_page_end,answer_link_method,answer_link_status,import_session_id,review_decision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.documentID, f.jobID, 50, 50, stem, inputHash, "extracted", identity, `["专题一","判断题"]`, setNo, qno, "true_false", "true_false", "专题一", questionJSON, merged, answerJSON, 50, 50, nullableInt(answerStart), nullableInt(answerEnd), nullableText(linkMethod), linkStatus, f.sessionID, "pending")
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT id FROM candidate_questions WHERE input_hash=?`, inputHash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO source_question_origins(origin_uid,session_id,candidate_id,question_document_id,source_identity) VALUES(?,?,?,?,?)`, "uid-"+inputHash, f.sessionID, id, f.documentID, identity); err != nil {
		t.Fatal(err)
	}
	var originID int64
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT id FROM source_question_origins WHERE session_id=? AND candidate_id=?`, f.sessionID, id).Scan(&originID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,page_start,page_end,content_hash,status) VALUES(?,?,?,?,?,?,?,'active')`, originID, id, "question", f.documentID, 50, 50, "q-"+inputHash); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLoadCandidateReviewTargetsResolvesAnswerPagesFromOrphanRecords(t *testing.T) {
	f, err := seedReviewFixture(t, "答案页解析")
	if err != nil {
		t.Fatal(err)
	}
	blocked := f.insertCandidate(t, "orphan-16", "判断题16", 16, 3, `{"source_identity":"x","stem":"判断题16","question_type":"true_false","answer":[]}`, `[]`, "", "unresolved", 0, 0)
	unmatched := f.insertCandidate(t, "orphan-30", "判断题30", 30, 3, `{"source_identity":"x","stem":"判断题30","question_type":"true_false","answer":[]}`, `[]`, "", "unresolved", 0, 0)
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO import_answer_records(session_id,document_id,source_question_no,question_type,canonical_question_type,set_no,answer_json,page_start,page_end,source_kind,link_status,source_identity,section_scope_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.sessionID, f.documentID, 16, "判断题", "true_false", 3, `["正确"]`, 133, 133, "answer", "unresolved", `["上一专题","三、判断题"]|answer|3|true_false|16`, "上一专题"); err != nil {
		t.Fatal(err)
	}
	// A linked record for the same printed number belongs to another topic and
	// must not widen the review window.
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO import_answer_records(session_id,document_id,source_question_no,question_type,canonical_question_type,set_no,answer_json,page_start,page_end,source_kind,link_status,source_identity,section_scope_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.sessionID, f.documentID, 16, "判断题", "true_false", 3, `["错误"]`, 145, 145, "answer", "linked", `["专题三"]|answer|3|true_false|16`, "专题三"); err != nil {
		t.Fatal(err)
	}
	targets, err := loadCandidateReviewTargets(f.ctx, f.repo.DB(), f.sessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]candidateReviewTarget{}
	for _, target := range targets {
		byID[target.ID] = target
	}
	resolved, ok := byID[blocked]
	if !ok {
		t.Fatalf("blocked candidate missing from targets: %#v", targets)
	}
	if resolved.AnswerPageStart != 133 || resolved.AnswerPageEnd != 133 || resolved.AnswerDocumentID != f.documentID {
		t.Fatalf("orphan answer pages were not resolved: start=%d end=%d doc=%d", resolved.AnswerPageStart, resolved.AnswerPageEnd, resolved.AnswerDocumentID)
	}
	remaining, ok := byID[unmatched]
	if !ok {
		t.Fatalf("unmatched candidate missing from targets: %#v", targets)
	}
	if remaining.AnswerPageStart != 0 {
		t.Fatalf("candidate without answer records must stay unresolved, got start=%d", remaining.AnswerPageStart)
	}
}

func TestRelinkSessionAnswersPreservesCandidateReviewAnswer(t *testing.T) {
	f, err := seedReviewFixture(t, "保留复核答案")
	if err != nil {
		t.Fatal(err)
	}
	reviewed := f.insertCandidate(t, "reviewed-16", "判断题16", 16, 3, `{"source_identity":"x","stem":"判断题16","question_type":"true_false","answer":["正确"],"source_page_start":50,"source_page_end":50}`, `{"source_identity":"x","answer":["正确"],"analysis":"本题说法正确","source_page_start":133,"source_page_end":133}`, "candidate_review", "confirmed", 133, 133)
	control := f.insertCandidate(t, "control-20", "判断题20", 20, 3, `{"source_identity":"x","stem":"判断题20","question_type":"true_false","answer":[]}`, `[]`, "", "unresolved", 0, 0)
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO import_problems(session_id,problem_type,severity,candidate_id,source_identity,status) VALUES(?,?,?,?,?,?)`, f.sessionID, "ANSWER_SOURCE_IDENTITY_INVALID", "blocking", reviewed, "x", "open"); err != nil {
		t.Fatal(err)
	}
	// Reproduce the production replay: a completed question-extraction unit
	// plus its directory entry cause recoverQuestionsFromCompletedUnits to
	// rewrite merged_json from the raw extraction body, which carries no
	// answer. The review patch must survive this rewrite.
	identity := `["专题一","判断题"]|question|3|true_false|16`
	entry := domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"专题一", "判断题"}, SectionScopeID: "专题一", SetNo: 3, QuestionType: "判断题", CanonicalQuestionType: "true_false", SourceQuestionNo: 16, SourcePageStart: 50, SourcePageEnd: 50, SourceIdentity: identity}
	if err := f.repo.ReplaceDirectoryEntries(f.ctx, f.sessionID, f.documentID, 0, "question_book", []domain.VisionDirectoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	unitBody := `{"questions":[{"source_identity":"` + identity + `","stem":"判断题16","options":[],"question_type":"true_false","answer":[],"source_page_start":50,"source_page_end":50}]}`
	if _, err := f.repo.DB().ExecContext(f.ctx, `INSERT INTO import_agent_units(import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,input_hash,pipeline_version,document_id,output_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.jobID, "extract_question", "0050-0050", 50, 50, 50, 50, "completed", "unit-50", domain.VisionPipelineVersionV11, f.documentID, unitBody); err != nil {
		t.Fatal(err)
	}
	if err := NewImportService(f.repo, config.Load()).RelinkSessionAnswers(f.ctx, f.sessionID, 0); err != nil {
		t.Fatal(err)
	}
	var answerJSON, mergedJSON, linkStatus, linkMethod, decision, status string
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT COALESCE(answer_json,''),COALESCE(merged_json,'{}'),COALESCE(answer_link_status,''),COALESCE(answer_link_method,''),COALESCE(review_decision,'pending'),status FROM candidate_questions WHERE id=?`, reviewed).Scan(&answerJSON, &mergedJSON, &linkStatus, &linkMethod, &decision, &status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answerJSON, "正确") || !strings.Contains(mergedJSON, "正确") || linkStatus != domain.AnswerLinkConfirmed || linkMethod != "candidate_review" {
		t.Fatalf("candidate-review answer was wiped by relink: answer=%q merged=%q link=%s/%s", answerJSON, mergedJSON, linkMethod, linkStatus)
	}
	var qSpans, aSpans int
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT (SELECT COUNT(*) FROM source_evidence_spans WHERE candidate_id=? AND source_role='question' AND status='active'),(SELECT COUNT(*) FROM source_evidence_spans WHERE candidate_id=? AND source_role='answer' AND status='active')`, reviewed, reviewed).Scan(&qSpans, &aSpans); err != nil {
		t.Fatal(err)
	}
	if decision != domain.ReviewInclude || status != "validated" {
		t.Fatalf("reviewed candidate was not resolved: decision=%s status=%s questionSpans=%d answerSpans=%d", decision, status, qSpans, aSpans)
	}
	if aSpans != 1 {
		t.Fatalf("answer provenance span was not recorded, got %d", aSpans)
	}
	var blockedProblems int
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND candidate_id=? AND status='open' AND severity='blocking'`, f.sessionID, reviewed).Scan(&blockedProblems); err != nil {
		t.Fatal(err)
	}
	if blockedProblems != 0 {
		t.Fatalf("resolved candidate must not keep blocking problems, got %d", blockedProblems)
	}
	var controlAnswer, controlDecision string
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT COALESCE(answer_json,'[]'),COALESCE(review_decision,'pending') FROM candidate_questions WHERE id=?`, control).Scan(&controlAnswer, &controlDecision); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(controlAnswer) != "[]" || controlDecision != domain.ReviewPending {
		t.Fatalf("control candidate should stay pending with a cleared answer: answer=%q decision=%s", controlAnswer, controlDecision)
	}
	var controlProblems int
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND candidate_id=? AND status='open' AND problem_type='ANSWER_UNRESOLVED'`, f.sessionID, control).Scan(&controlProblems); err != nil {
		t.Fatal(err)
	}
	if controlProblems != 1 {
		t.Fatalf("control candidate must keep exactly one ANSWER_UNRESOLVED problem, got %d", controlProblems)
	}
}
