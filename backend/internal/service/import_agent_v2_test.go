package service

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

func newImportAgentTestRepo(t *testing.T) (*sqlite.Repository, int64, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.db")
	db, err := database.Open(path, 5000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = database.Migrate(context.Background(), db, "/Users/mutant/Documents/project/QuizForge/backend/migrations"); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	hash, _ := security.HashPassword("secret")
	user, err := repo.CreateUser(context.Background(), "agent-user", "agent@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := repo.CreateBank(context.Background(), "导入测试", "", "private", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, user.ID, bank.ID
}

func TestImportAgentSessionCoalescesConsecutiveAttachments(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "小四门")
	if err != nil {
		t.Fatal(err)
	}
	doc1, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "q.pdf", FilePath: "/tmp/q.pdf", FileSize: 1, FileHash: "agent-q", MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "答案.pdf", StoredName: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 1, FileHash: "agent-a", MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	_, job1, run1, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, run2, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run1.ID != run2.ID {
		t.Fatalf("consecutive attachments should share one active run: %d != %d", run1.ID, run2.ID)
	}
	if job1.PipelineVersion != domain.VisionPipelineVersionV11 || run1.StrategyVersion != domain.VisionPipelineVersionV11 {
		t.Fatalf("new attachments must use v1.1: job=%s run=%s", job1.PipelineVersion, run1.StrategyVersion)
	}
	if err := repo.UpdateImportRunProgress(ctx, run1.ID, "catalog", 10, 20, 0, 300, 2, ""); err != nil {
		t.Fatal(err)
	}
	progressRun, err := repo.GetImportRun(ctx, run1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progressRun.StageProgress != .5 || progressRun.OverallProgress != .2 || progressRun.AdaptiveConcurrency != 2 {
		t.Fatalf("unexpected persisted progress: %#v", progressRun)
	}
	updated, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AttachmentRevision != 2 {
		t.Fatalf("attachment revision=%d", updated.AttachmentRevision)
	}
	docs, err := repo.ListSessionDocuments(ctx, session.ID)
	if err != nil || len(docs) != 2 {
		t.Fatalf("documents=%d err=%v", len(docs), err)
	}
	messages, err := repo.ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	agentCards := 0
	for _, message := range messages {
		if message.RunID != nil && *message.RunID == run1.ID && message.Role == "assistant" && message.MessageType == "agent_run" {
			agentCards++
		}
	}
	if agentCards != 1 {
		t.Fatalf("one coalesced run must own one persistent agent card, got %d", agentCards)
	}
	events, err := repo.ListImportEvents(ctx, session.ID, 0, 20)
	if err != nil || len(events) != 2 {
		t.Fatalf("attachment events=%d err=%v", len(events), err)
	}
	if events[0].MessageID == nil || events[1].MessageID == nil || *events[0].MessageID != *events[1].MessageID {
		t.Fatal("coalesced attachment events must target the persistent agent card")
	}
}

func TestVisionCandidateKeepsStableOriginAcrossRetry(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "来源稳定性")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "source.pdf", StoredName: "source.pdf", FilePath: "/tmp/source.pdf", FileSize: 1, FileHash: "stable-source-hash", MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	_, job, _, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"第一章", "单选题"}, QuestionType: "single_choice", CanonicalQuestionType: "single_choice", SectionScopeID: "第一章", OriginalQuestionLabel: "16.", SourceQuestionNo: 16, SequenceInScope: 3, SourcePageStart: 8, SourcePageEnd: 8, SourceIdentity: "stable|q|16"}
	question := domain.VisionQuestion{SourceIdentity: entry.SourceIdentity, Stem: "第一次识别", QuestionType: "single_choice", SourcePageStart: 8, SourcePageEnd: 8, VisualConfidence: .8}
	first, err := repo.InsertVisionCandidate(ctx, job.ID, doc.ID, entry, question)
	if err != nil {
		t.Fatal(err)
	}
	question.Stem = "第二次识别修正"
	second, err := repo.InsertVisionCandidate(ctx, job.ID, doc.ID, entry, question)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("retry generated a new candidate: %d != %d", first, second)
	}
	provenance, err := repo.GetCandidateProvenance(ctx, session.ID, first)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.OriginUID != fmt.Sprintf("qf-origin:%d:%d", session.ID, first) || provenance.OriginalQuestionLabel != "16." || provenance.SequenceInScope != 3 {
		t.Fatalf("unstable or incomplete origin: %#v", provenance)
	}
	if len(provenance.SourceSpans) == 0 || provenance.SourceSpans[0].PageStart != 8 {
		t.Fatalf("question evidence missing: %#v", provenance.SourceSpans)
	}
}

func TestDeferredAttachmentWaitsForPrompt(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "文件和指令一起发送")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID:       userID,
		BankID:       &bankID,
		OriginalName: "题本.pdf",
		StoredName:   "deferred-question.pdf",
		FilePath:     "/tmp/deferred-question.pdf",
		FileSize:     1,
		FileHash:     "deferred-attachment",
		MimeType:     "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, run, err := repo.AttachDocumentToSessionDeferred(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Fatalf("deferred attachment unexpectedly created run: %#v", run)
	}
	updated, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ActiveRunID != nil || updated.Status != "receiving_files" {
		t.Fatalf("deferred attachment changed session state: %#v", updated)
	}
	var agentRuns int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM import_session_messages WHERE session_id=? AND message_type='agent_run'`, session.ID).Scan(&agentRuns); err != nil {
		t.Fatal(err)
	}
	if agentRuns != 0 {
		t.Fatalf("deferred attachment created %d agent run cards", agentRuns)
	}
	if job.Status != domain.ImportStatusUploaded {
		t.Fatalf("deferred import job status=%s", job.Status)
	}
	if action := deterministicConversationAction("读取这里的题目。"); action.Intent != "start_full_extraction" {
		t.Fatalf("prompt should start one import run, got %s", action.Intent)
	}
	action := deterministicConversationAction("读取这里的题目。")
	svc := NewImportConversationService(repo, config.Load())
	if err := svc.executeFullExtraction(ctx, updated, nil, action); err != nil {
		t.Fatal(err)
	}
	started, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.ActiveRunID == nil {
		t.Fatal("prompt did not create the combined import run")
	}
	var queued int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE json_extract(payload,'$.import_job_id')=? AND status='queued'`, job.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("expected exactly one queued import job, got %d", queued)
	}
}

func TestImportConversationClarifiesAmbiguousPageRange(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, _ := repo.CreateImportSession(ctx, userID, &bankID, "多文档")
	for index, name := range []string{"题本.pdf", "答案.pdf"} {
		doc, _ := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: name, StoredName: name, FilePath: "/tmp/" + name, FileSize: 1, FileHash: "ambiguous-" + string(rune('a'+index)), MimeType: "application/pdf"})
		_, _, _, _ = repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	}
	svc := NewImportConversationService(repo, config.Load())
	current, _ := repo.GetImportSession(ctx, session.ID)
	action, err := svc.HandleMessage(ctx, current, "重新识别 20 到 24 页", current.DraftVersion)
	if err != nil {
		t.Fatal(err)
	}
	if action.Intent != "retry_pages" {
		t.Fatalf("intent=%s", action.Intent)
	}
	messages, err := repo.ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if last.MessageType != "clarification" {
		t.Fatalf("message type=%s", last.MessageType)
	}
}

func TestImportConversationDoesNotStartDuplicateRunWhileImporting(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "正在导入")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID:       userID,
		BankID:       &bankID,
		OriginalName: "题本.pdf",
		StoredName:   "question.pdf",
		FilePath:     "/tmp/question.pdf",
		FileSize:     1,
		FileHash:     "active-run",
		MimeType:     "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, run, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}

	svc := NewImportConversationService(repo, config.Load())
	current, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	action, err := svc.HandleMessage(ctx, current, "导入这个pdf内的全部题目", current.DraftVersion)
	if err != nil {
		t.Fatal(err)
	}
	if action.Intent != "explain_status" {
		t.Fatalf("intent=%s", action.Intent)
	}

	var runCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM import_runs WHERE session_id=?`, session.ID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("active import should not create a duplicate run, got %d runs", runCount)
	}
	updated, err := repo.GetImportRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "running" {
		t.Fatalf("existing run status=%s", updated.Status)
	}
	messages, err := repo.ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || last.MessageType != "text" || last.Status != "completed" {
		t.Fatalf("unexpected reply: %#v", last)
	}
}

func TestArchiveImportSessionCancelsOwnedWork(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "待删除会话")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID:       userID,
		BankID:       &bankID,
		OriginalName: "题本.pdf",
		StoredName:   "archive-question.pdf",
		FilePath:     "/tmp/archive-question.pdf",
		FileSize:     1,
		FileHash:     "archive-session",
		MimeType:     "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, importJob, run, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	queueJobID, err := repo.EnqueueJob(ctx, "import_pdf", ImportPayload{ImportJobID: importJob.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.ArchiveImportSession(ctx, session.ID, userID); err != nil {
		t.Fatal(err)
	}
	archived, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil || archived.ActiveRunID != nil || archived.Status != domain.ImportStatusCancelled {
		t.Fatalf("session was not cancelled and archived: %#v", archived)
	}
	updatedRun, err := repo.GetImportRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.Status != domain.ImportStatusCancelled || !updatedRun.CancelRequested {
		t.Fatalf("run was not cancelled: %#v", updatedRun)
	}
	updatedImportJob, err := repo.GetImportJob(ctx, importJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedImportJob.Status != domain.ImportStatusCancelled {
		t.Fatalf("import job status=%s, want cancelled", updatedImportJob.Status)
	}
	queueJob, err := repo.GetJob(ctx, queueJobID)
	if err != nil {
		t.Fatal(err)
	}
	if queueJob.Status != "cancelled" || queueJob.ErrorCode != "SESSION_ARCHIVED" {
		t.Fatalf("queue job was not cancelled: %#v", queueJob)
	}
	if err := repo.CompleteJob(ctx, queueJobID, true, ""); err != nil {
		t.Fatal(err)
	}
	queueJob, err = repo.GetJob(ctx, queueJobID)
	if err != nil {
		t.Fatal(err)
	}
	if queueJob.Status != "cancelled" {
		t.Fatalf("cancelled queue job was overwritten: %s", queueJob.Status)
	}
}

func TestInferVisionDocumentRole(t *testing.T) {
	profiles := map[int]string{1: `{"pages":[{"page":1,"role":"answer","section_path":[],"question_type":"single_choice","set_no":1},{"page":2,"role":"answer","section_path":[],"question_type":"single_choice","set_no":1}],"has_answers":true}`}
	role, confidence := inferVisionDocumentRole(profiles)
	if role != "answer_book" || confidence != 1 {
		t.Fatalf("role=%s confidence=%f", role, confidence)
	}
}

func TestConversationCanRestartWholeDocumentAfterModelConfiguration(t *testing.T) {
	action := deterministicConversationAction("视觉模型已经配置好，重新识别整份文档")
	if action.Intent != "start_full_extraction" {
		t.Fatalf("intent=%s", action.Intent)
	}
}

func TestPendingQuestionCountIsNotParsedAsPageRange(t *testing.T) {
	action := deterministicConversationAction("对待审核的 48 题题目进行补充")
	if action.Intent != "review_pending_candidates" {
		t.Fatalf("pending request intent=%s", action.Intent)
	}
	if action.ReviewScope != "pending" || action.TargetRole != "auto" {
		t.Fatalf("pending request scope=%q target=%q", action.ReviewScope, action.TargetRole)
	}
	if len(action.PageRanges) != 0 {
		t.Fatalf("question count must not become page range: %#v", action.PageRanges)
	}
	pageAction := deterministicConversationAction("重新识别物理页 48-52")
	if pageAction.Intent != "retry_pages" || len(pageAction.PageRanges) != 1 || pageAction.PageRanges[0][0] != 48 || pageAction.PageRanges[0][1] != 52 {
		t.Fatalf("explicit page range was not preserved: %#v", pageAction)
	}
}

func TestCandidateReviewWorkerRestoresImportOwnerForProvider(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	cfg := config.Load()
	registry := provider.NewRegistry(repo.DB(), security.NewAESCrypt(cfg.Auth.MasterKey))
	configured, err := registry.Save(observability.WithUserID(ctx, userID), provider.TypeLLM, "review-test", "openai", "http://127.0.0.1:1/v1", "test-key", "test-model", true, true)
	if err != nil {
		t.Fatal(err)
	}

	session, err := repo.CreateImportSession(ctx, userID, &bankID, "候选题复核 Provider 上下文")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID: userID, BankID: &bankID, OriginalName: "review.pdf", StoredName: "review.pdf",
		FilePath: "/tmp/quiztrace-review-fixture-does-not-exist.pdf", FileSize: 1, FileHash: "review-provider-context", MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, importJob, run, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	questionJSON := `{"source_identity":"review|question|1","stem":"测试题","options":[],"question_type":"single_choice","answer":[],"source_page_start":1,"source_page_end":1}`
	_, err = repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions(document_id,import_job_id,page_start,page_end,raw_text,input_hash,status,source_identity,section_path_json,set_no,source_question_no,question_type,canonical_question_type,question_json,question_page_start,question_page_end,import_session_id,answer_link_status,review_decision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		doc.ID, importJob.ID, 1, 1, "测试题", "review-provider-context-input", "extracted", "review|question|1", "[]", 0, 1, "single_choice", "single_choice", questionJSON, 1, 1, session.ID, "unresolved", "pending")
	if err != nil {
		t.Fatal(err)
	}
	var candidateID int64
	if err := repo.DB().QueryRowContext(ctx, `SELECT id FROM candidate_questions WHERE input_hash=?`, "review-provider-context-input").Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	queueID, err := repo.EnqueueJob(ctx, "import_candidate_review", CandidateReviewPayload{SessionID: session.ID, RunID: run.ID, CandidateIDs: []int64{candidateID}, TargetRole: "question"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	queueJob, err := repo.GetJob(ctx, queueID)
	if err != nil {
		t.Fatal(err)
	}

	// The fixture intentionally has no PDF. The review should get past model
	// resolution and fail on the fixture, proving the provider was resolved
	// from the import owner's user scope rather than user_id=0.
	if err := NewImportService(repo, cfg).RunCandidateReview(ctx, queueJob); err != nil {
		t.Fatal(err)
	}
	updatedJob, err := repo.GetImportJob(ctx, importJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedJob.ProviderIDSnapshot == nil || *updatedJob.ProviderIDSnapshot != configured.ID {
		t.Fatalf("candidate review did not snapshot the owner's provider: %#v", updatedJob.ProviderIDSnapshot)
	}
	updatedRun, err := repo.GetImportRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.ErrorCode == "VISION_MODEL_REQUIRED" {
		t.Fatalf("candidate review still resolved provider in the global scope: %#v", updatedRun)
	}
}

func TestCreateSessionRunRejectsConcurrentActiveRun(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "CAS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateSessionRun(ctx, session.ID, nil, "repair", domain.ConversationAction{Intent: "retry_pages"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateSessionRun(ctx, session.ID, nil, "repair", domain.ConversationAction{Intent: "retry_pages"}); err == nil || err.Error() != "ACTIVE_RUN_EXISTS" {
		t.Fatalf("second active run should be rejected explicitly, got %v", err)
	}
}

func TestPlanMissingAnswerRetryTargetsOnlyAffectedWindows(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "答案恢复")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "question.pdf", FilePath: "/tmp/question.pdf", FileSize: 1, FileHash: "retry-answer", MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	_, job, _, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"专题一"}, SectionScopeID: "专题一", SetNo: 0, QuestionType: "single_choice", CanonicalQuestionType: "single_choice", SourceQuestionNo: 2, SourcePageStart: 101, SourcePageEnd: 101, Anchor: "2.B【解析】答案"}
	entry.SourceIdentity = "[\"专题一\"]|explanation|0|single_choice|2"
	if err := repo.ReplaceDirectoryEntries(ctx, session.ID, doc.ID, 0, "question_book", []domain.VisionDirectoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	_, err = repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions(document_id,import_job_id,page_start,page_end,raw_text,input_hash,status,source_identity,section_path_json,set_no,source_question_no,question_type,canonical_question_type,section_scope_id,question_page_start,question_page_end,import_session_id,answer_link_status,review_decision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, doc.ID, job.ID, 50, 50, "题干", "retry-input", "extracted", "[\"专题一\"]|question|0|single_choice|2", `["专题一"]`, 0, 2, "single_choice", "single_choice", "专题一", 50, 50, session.ID, "unresolved", "pending")
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []struct {
		key, status string
		start, end  int
	}{
		{"0100-0104", "completed", 100, 104},
		{"0200-0204", "failed", 200, 204},
	} {
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO import_agent_units(import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,input_hash,pipeline_version,document_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, job.ID, "extract_answer", unit.key, unit.start, unit.end-1, unit.start, unit.end, unit.status, unit.key, domain.VisionPipelineVersionV11, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	svc := NewImportConversationService(repo, config.Load())
	action, count, err := svc.planMissingAnswerRetry(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || action == nil || len(action.PageRanges) != 1 || action.PageRanges[0][0] != 100 || action.PageRanges[0][1] != 104 {
		t.Fatalf("unexpected retry plan: action=%#v count=%d", action, count)
	}
}

func TestSessionDraftReturnsAllRowsAcrossPages(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "分页完整性")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "draft.pdf",
		FilePath: "/tmp/draft.pdf", FileSize: 1, FileHash: "draft-pagination", MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 250; i++ {
		questionJSON := `{"stem":"题目","options":[{"key":"A","content":"选项 A"}],"answer":["A"]}`
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions
			(document_id,page_start,page_end,raw_text,input_hash,status,source_question_no,
			 question_type,canonical_question_type,question_json,merged_json,question_page_start,
			 question_page_end,import_session_id,answer_link_status,review_decision)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			doc.ID, i, i, "题目", fmt.Sprintf("pagination-%03d", i), "validated", i,
			"single_choice", "single_choice", questionJSON, questionJSON, i, i,
			session.ID, "confirmed", "include")
		if err != nil {
			t.Fatal(err)
		}
	}

	first, total, err := repo.SessionDraft(ctx, session.ID, 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	second, secondTotal, err := repo.SessionDraft(ctx, session.ID, 2, 200)
	if err != nil {
		t.Fatal(err)
	}
	oversized, oversizedTotal, err := repo.SessionDraft(ctx, session.ID, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if total != 250 || secondTotal != 250 || len(first) != 200 || len(second) != 50 {
		t.Fatalf("draft pagination truncated: total=%d/%d page sizes=%d/%d", total, secondTotal, len(first), len(second))
	}
	if oversizedTotal != 250 || len(oversized) != 200 {
		t.Fatalf("oversized page size was not capped at 200: total=%d rows=%d", oversizedTotal, len(oversized))
	}
	if first[0]["id"] == second[0]["id"] || first[199]["id"] == second[0]["id"] {
		t.Fatal("draft pages overlap")
	}
}

func TestStructuralQuestionIssueClassification(t *testing.T) {
	for _, code := range []string{"STEM_REQUIRED", "OPTIONS_REQUIRED", "OPTIONS_INVALID"} {
		if !isStructuralQuestionIssue(map[string]interface{}{"code": code}) {
			t.Fatalf("structural issue %s was not classified", code)
		}
	}
	if isStructuralQuestionIssue(map[string]interface{}{"code": "ANSWER_REQUIRED"}) {
		t.Fatal("missing answers must use the dedicated ANSWER_* problem")
	}
	if isStructuralQuestionIssue(map[string]interface{}{"code": "ANSWER_INVALID"}) {
		t.Fatal("invalid answers must use the dedicated ANSWER_* problem")
	}
}

func TestVisionJSONRepairOnlyHandlesSyntaxErrors(t *testing.T) {
	if !shouldRepairVisionJSON(fmt.Errorf("模型未返回有效 JSON")) || !shouldRepairVisionJSON(fmt.Errorf("unexpected end of JSON input")) {
		t.Fatal("syntax errors should use the text-only JSON repair")
	}
	if shouldRepairVisionJSON(fmt.Errorf("缺少 questions 数组")) || shouldRepairVisionJSON(fmt.Errorf("缺少 answers 数组")) {
		t.Fatal("semantic schema errors need a fresh vision response, not text-only JSON repair")
	}
}

func TestCancelImportRunCancelsOwnedPendingUnits(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "取消任务")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "cancel.pdf", FilePath: "/tmp/cancel.pdf", FileSize: 1, FileHash: "cancel-run", MimeType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	_, job, run, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, status := range []string{"pending", "running", "completed"} {
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO import_agent_units(import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,input_hash,pipeline_version,run_id,document_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, job.ID, "extract_question", fmt.Sprintf("cancel-%d", index), 1, 4, 1, 5, status, fmt.Sprintf("cancel-%d", index), domain.VisionPipelineVersionV11, run.ID, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CancelImportRun(ctx, session.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.DB().QueryContext(ctx, `SELECT status FROM import_agent_units WHERE run_id=? ORDER BY id`, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	statuses := []string{}
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, status)
	}
	if fmt.Sprint(statuses) != "[cancelled cancelled completed]" {
		t.Fatalf("unexpected unit statuses after cancellation: %v", statuses)
	}
	updated, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ActiveRunID != nil || updated.Status != "cancelled" {
		t.Fatalf("session still appears active after cancellation: %#v", updated)
	}
}

func TestSessionSummaryReturnsAuthoritativeDraftStats(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "审核统计")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "stats.pdf",
		FilePath: "/tmp/stats.pdf", FileSize: 1, FileHash: "summary-stats", MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		status, decision, link string
	}{
		{"validated", "include", "confirmed"},
		{"needs_review", "pending", "confirmed"},
		{"validated", "pending", "unresolved"},
		{"validated", "exclude", "confirmed"},
	}
	for index, row := range rows {
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions
			(document_id,page_start,page_end,raw_text,input_hash,status,source_question_no,
			 question_type,canonical_question_type,question_page_start,question_page_end,
			 import_session_id,answer_link_status,review_decision)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, doc.ID, index+1, index+1, "题目", fmt.Sprintf("summary-%d", index), row.status, index+1,
			"single_choice", "single_choice", index+1, index+1, session.ID, row.link, row.decision)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = repo.DB().ExecContext(ctx, `INSERT INTO import_problems(session_id,problem_type,severity,status) VALUES(?, 'QUESTION_INVALID', 'blocking', 'open')`, session.ID); err != nil {
		t.Fatal(err)
	}
	summary, err := repo.SessionSummary(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	stats, ok := summary["draft_stats"].(map[string]int)
	if !ok {
		t.Fatalf("missing draft stats: %#v", summary["draft_stats"])
	}
	if stats["total_count"] != 4 || stats["included_count"] != 1 || stats["excluded_count"] != 1 || stats["pending_review_count"] != 2 || stats["unresolved_answer_count"] != 1 || stats["needs_review_count"] != 1 || stats["open_problem_count"] != 1 || stats["open_blocking_problem_count"] != 1 {
		t.Fatalf("unexpected draft stats: %#v", stats)
	}
}

func TestRefreshSessionReviewCountsSynchronizesLegacyJobProjection(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "同步任务投影")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID: userID, BankID: &bankID, OriginalName: "source.pdf", StoredName: "source.pdf",
		FilePath: "/tmp/source.pdf", FileSize: 1, FileHash: "projection-sync", MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, _, err := repo.AttachDocumentToSession(ctx, session.ID, userID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.VisionDirectoryEntry{
		ContentRole: "question", SectionPath: []string{"专题一", "一、单项选择题"},
		SectionScopeID: "专题一", SetNo: 0, QuestionType: "single_choice", CanonicalQuestionType: "single_choice",
		SourceQuestionNo: 1, SourcePageStart: 4, SourcePageEnd: 4, Anchor: "1. 题目",
	}
	entry.SourceIdentity = visionSourceIdentity(entry)
	if err := repo.ReplaceDirectoryEntries(ctx, session.ID, doc.ID, 0, "question_book", []domain.VisionDirectoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO import_agent_units(import_job_id,unit_type,unit_key,owned_page_start,owned_page_end,context_page_start,context_page_end,status,input_hash,pipeline_version,document_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, job.ID, "catalog", "catalog-1", 1, 4, 1, 4, "completed", "catalog-1", domain.VisionPipelineVersionV11, doc.ID); err != nil {
		t.Fatal(err)
	}
	questionJSON := `{"stem":"题目","options":[{"key":"A","content":"选项"}],"answer":["A"]}`
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions(document_id,import_job_id,page_start,page_end,raw_text,input_hash,status,source_identity,section_path_json,set_no,source_question_no,question_type,canonical_question_type,question_json,merged_json,question_page_start,question_page_end,import_session_id,answer_link_status,review_decision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, doc.ID, job.ID, 4, 4, "题目", "projection-candidate", "validated", entry.SourceIdentity, `["专题一","一、单项选择题"]`, 0, 1, "single_choice", "single_choice", questionJSON, questionJSON, 4, 4, session.ID, "confirmed", "include"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE import_jobs SET status='needs_attention',stage='needs_attention',expected_count=NULL,candidate_count=1,extracted_count=1,complete_count=0,blocking_issue_count=1069,progress=1.0 WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}

	svc := NewImportService(repo, config.Load())
	if err := svc.refreshSessionReviewCounts(ctx, session.ID, 0); err != nil {
		t.Fatal(err)
	}
	updatedSession, err := repo.GetImportSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedSession.Status != domain.ImportStatusDraftReady || updatedSession.ExpectedCount == nil || *updatedSession.ExpectedCount != 1 || updatedSession.UniqueCount != 1 || updatedSession.CompleteCount != 1 || updatedSession.BlockingIssueCount != 0 {
		t.Fatalf("session projection not refreshed: %#v", updatedSession)
	}
	updatedJob, err := repo.GetImportJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedJob.Status != domain.ImportStatusDraftReady || updatedJob.Stage != domain.ImportStatusDraftReady || !updatedJob.DirectoryComplete || updatedJob.ExpectedCount == nil || *updatedJob.ExpectedCount != 1 || updatedJob.CandidateCount != 1 || updatedJob.ExtractedCount != 1 || updatedJob.CompleteCount != 1 || updatedJob.ReviewCount != 0 || updatedJob.BlockingIssueCount != 0 || updatedJob.Progress != 1 {
		t.Fatalf("legacy job projection not refreshed: %#v", updatedJob)
	}
}

func TestSessionDraftPreservesPhysicalOrderWhenQuestionNumbersRestart(t *testing.T) {
	repo, userID, bankID := newImportAgentTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateImportSession(ctx, userID, &bankID, "同页题号重置")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.CreateDocument(ctx, &domain.Document{
		UserID: userID, BankID: &bankID, OriginalName: "题本.pdf", StoredName: "order.pdf",
		FilePath: "/tmp/order.pdf", FileSize: 1, FileHash: "draft-physical-order", MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Insert the later block first to prove the ordering is driven by the PDF
	// marker rather than candidate insertion IDs or restarted question numbers.
	itemsToInsert := []struct {
		qno, page, marker int
		typeName          string
		inputHash         string
	}{
		{1, 7, 585, "multiple_choice", "physical-order-multi"},
		{33, 7, 77, "single_choice", "physical-order-single"},
	}
	entries := make([]domain.VisionDirectoryEntry, 0, len(itemsToInsert))
	for _, item := range itemsToInsert {
		questionJSON := fmt.Sprintf(`{"stem":"题目 %d","options":[{"key":"A","content":"选项 A"}],"answer":["A"],"question_type":"%s"}`, item.qno, item.typeName)
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO candidate_questions
			(document_id,page_start,page_end,raw_text,input_hash,status,source_question_no,
			 question_type,canonical_question_type,question_json,merged_json,question_page_start,
			 question_page_end,import_session_id,answer_link_status,review_decision)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			doc.ID, item.page, item.page, "题目", item.inputHash, "validated", item.qno,
			item.typeName, item.typeName, questionJSON, questionJSON, item.page, item.page,
			session.ID, "confirmed", "include")
		if err != nil {
			t.Fatal(err)
		}
		entry := domain.VisionDirectoryEntry{
			ContentRole: "question", SectionPath: []string{"专题一"}, SetNo: 1,
			QuestionType: item.typeName, CanonicalQuestionType: item.typeName,
			SectionScopeID: "专题一", SourceQuestionNo: item.qno,
			SourcePageStart: item.page, SourcePageEnd: item.page, MarkerTop: item.marker,
		}
		entries = append(entries, entry)
	}
	if err := repo.ReplaceDirectoryEntries(ctx, session.ID, doc.ID, 0, "question_book", entries); err != nil {
		t.Fatal(err)
	}
	items, _, err := repo.SessionDraft(ctx, session.ID, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0]["source_question_no"] != 33 || items[1]["source_question_no"] != 1 {
		t.Fatalf("draft did not follow physical markers: %#v", items)
	}
	if items[0]["source_marker_top"] != 77 || items[1]["source_marker_top"] != 585 {
		t.Fatalf("marker positions missing from draft: %#v", items)
	}
}

func TestSessionIdentityKeepsSubjectiveSubtypesSeparate(t *testing.T) {
	shortAnswer := sessionIdentity([]string{"专题一", "五、简答题"}, 0, "essay", 1)
	caseAnalysis := sessionIdentity([]string{"专题一", "六、案例分析题"}, 0, "essay", 1)
	if shortAnswer == caseAnalysis {
		t.Fatalf("subjective subtypes must not share reconciliation identity: %q", shortAnswer)
	}
	if sessionIdentity([]string{"专题一", "五、简答题"}, 0, "essay", 1) != shortAnswer {
		t.Fatal("session identity is not deterministic")
	}
}
