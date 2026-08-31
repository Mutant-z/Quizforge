package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

type ImportConversationService struct {
	repo     *sqlite.Repository
	cfg      *config.Config
	importer *ImportService
}

func NewImportConversationService(repo *sqlite.Repository, cfg *config.Config) *ImportConversationService {
	return &ImportConversationService{repo: repo, cfg: cfg, importer: NewImportService(repo, cfg)}
}

func (s *ImportConversationService) ReconcileSession(ctx context.Context, sessionID int64) error {
	return s.importer.reconcileImportSession(ctx, sessionID)
}

func (s *ImportConversationService) RelinkAnswers(ctx context.Context, sessionID int64, expectedVersion int) error {
	session, err := s.repo.GetImportSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if expectedVersion > 0 && session.DraftVersion != expectedVersion {
		return fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	if err := s.importer.RelinkSessionAnswers(ctx, sessionID, 0); err != nil {
		return err
	}
	// Relinking is also the recovery entry point for a directory/extraction
	// conservation gap. A completed directory may contain questions that the
	// old extractor silently omitted; queue only the affected 4+1 windows before
	// planning answer retries, rather than asking the user to guess page ranges.
	questionAction, questionUnits, planQuestionErr := s.planMissingQuestionRetry(ctx, sessionID)
	if planQuestionErr != nil {
		return planQuestionErr
	}
	// Relinking is also the recovery entry point. If a completed directory
	// window explicitly lists an answer but the corresponding answer unit did
	// not persist a record (for example after a 429 or worker shutdown), queue
	// only the affected 4+1 windows. This avoids making the user guess which
	// answer pages need a manual retry and never re-runs question extraction.
	retryAction, retryUnits, planErr := s.planMissingAnswerRetry(ctx, sessionID)
	if planErr != nil {
		return planErr
	}
	message := "已按标准题型和章节作用域重新对齐答案；未唯一匹配的项目保留为待处理。"
	if questionUnits > 0 {
		questionAction.Instruction = "补回目录中已确认但尚未提取的题目"
		if err := s.executeRepair(ctx, session, nil, questionAction); err != nil {
			return err
		}
		message = fmt.Sprintf("已重新对齐答案，并定向重试 %d 个缺失题目页窗；未唯一匹配的项目保留为待处理。", questionUnits)
	} else if retryUnits > 0 {
		retryAction.Instruction = "补回目录中已确认但尚未提取的答案"
		if err := s.executeRepair(ctx, session, nil, retryAction); err != nil {
			return err
		}
		message = fmt.Sprintf("已重新对齐答案，并定向重试 %d 个缺失答案页窗；未唯一匹配的项目保留为待处理。", retryUnits)
	} else {
		// A direct relink can be requested after an earlier run exposed stale
		// duplicate/conflict projections. Reconcile once when no retry was
		// queued so those derived problems and counts are rebuilt using the
		// corrected scoped identity. Do not do this while a repair run is active.
		if err := s.importer.reconcileImportSession(ctx, sessionID); err != nil {
			return err
		}
		// Do not leave a generic "待处理" message after a clean relink. Read
		// the reconciled projection so the conversation card reflects the same
		// counts as the draft and legacy job APIs.
		if updated, getErr := s.repo.GetImportSession(ctx, sessionID); getErr == nil {
			var pending int
			_ = s.repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status!='session_duplicate' AND COALESCE(review_decision,'pending')='pending'`, sessionID).Scan(&pending)
			message = fmt.Sprintf("答案对齐完成：共 %d 题，完整 %d 题，待审核 %d 题，阻断 %d 项。", updated.UniqueCount, updated.CompleteCount, pending, updated.BlockingIssueCount)
		}
	}
	messageID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: sessionID, Role: "assistant", MessageType: "tool_result", Content: message, Status: "completed"})
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: sessionID, MessageID: &messageID, Type: "answer_delta", Stage: "linking", AgentRole: "AnswerLinkAgent", Summary: message})
	return nil
}

// planMissingQuestionRetry compares the durable directory with extracted
// candidates by immutable source identity and returns page ranges for only the
// missing question windows. This is intentionally separate from answer
// relinking: an omitted question must be recovered before its answer can be
// considered unresolved.
func (s *ImportConversationService) planMissingQuestionRetry(ctx context.Context, sessionID int64) (*domain.ConversationAction, int, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT d.document_id,d.source_page_start,d.source_page_end
		FROM import_directory_entries d
		LEFT JOIN candidate_questions c
		  ON c.import_session_id=d.session_id
		 AND c.document_id=d.document_id
		 AND c.source_question_no=d.source_question_no
		 AND c.question_page_start=d.source_page_start
		 AND c.question_page_end=d.source_page_end
		 AND COALESCE(c.canonical_question_type,c.question_type)=d.canonical_question_type
		 AND COALESCE(c.section_scope_id,'')=COALESCE(d.section_scope_id,'')
		 AND c.status NOT IN ('session_duplicate','persisted','duplicate')
		WHERE d.session_id=? AND d.content_role='question' AND d.status='active' AND c.id IS NULL
		ORDER BY d.document_id,d.source_page_start,d.source_page_end`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	type pageRange struct {
		documentID int64
		start, end int
	}
	ranges := []pageRange{}
	for rows.Next() {
		var r pageRange
		if err := rows.Scan(&r.documentID, &r.start, &r.end); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if r.start <= 0 || r.end < r.start {
			continue
		}
		ranges = append(ranges, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	// A candidate may exist but still need question re-extraction because its
	// stem/options are structurally invalid. Missing answers are intentionally
	// excluded here; those belong to the much cheaper answer relink/retry path.
	invalidRows, err := s.repo.DB().QueryContext(ctx, `SELECT document_id,COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),COALESCE(validation_issues_json,'[]') FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('validated','session_duplicate','persisted','duplicate')`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	for invalidRows.Next() {
		var r pageRange
		var raw string
		if err := invalidRows.Scan(&r.documentID, &r.start, &r.end, &raw); err != nil {
			invalidRows.Close()
			return nil, 0, err
		}
		var issues []map[string]interface{}
		_ = json.Unmarshal([]byte(raw), &issues)
		structural := false
		for _, item := range issues {
			if isStructuralQuestionIssue(item) {
				structural = true
				break
			}
		}
		if structural && r.start > 0 && r.end >= r.start {
			ranges = append(ranges, r)
		}
	}
	if err := invalidRows.Err(); err != nil {
		invalidRows.Close()
		return nil, 0, err
	}
	invalidRows.Close()
	if len(ranges) == 0 {
		return &domain.ConversationAction{}, 0, nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].documentID != ranges[j].documentID {
			return ranges[i].documentID < ranges[j].documentID
		}
		if ranges[i].start != ranges[j].start {
			return ranges[i].start < ranges[j].start
		}
		return ranges[i].end < ranges[j].end
	})
	merged := make([]pageRange, 0, len(ranges))
	for _, r := range ranges {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if last.documentID == r.documentID && r.start <= last.end+1 {
				if r.end > last.end {
					last.end = r.end
				}
				continue
			}
		}
		merged = append(merged, r)
	}
	action := &domain.ConversationAction{Intent: "recover_missing_questions", RequiresConfirmation: false}
	seenDocuments := map[int64]bool{}
	for _, r := range merged {
		if !seenDocuments[r.documentID] {
			action.DocumentIDs = append(action.DocumentIDs, r.documentID)
			seenDocuments[r.documentID] = true
		}
		action.PageRanges = append(action.PageRanges, []int{r.start, r.end})
	}
	return action, len(merged), nil
}

type answerRetryUnit struct {
	id, jobID, documentID    int64
	contextStart, contextEnd int
	status                   string
}

type answerRetryCandidate struct {
	documentID           int64
	scope, canonicalType string
	setNo, questionNo    int
}

type answerRetryDirectoryEntry struct {
	documentID           int64
	scope, canonicalType string
	setNo, questionNo    int
	pageStart, pageEnd   int
}

type answerRetryKey struct {
	documentID           int64
	scope, canonicalType string
	setNo, questionNo    int
}

// planMissingAnswerRetry returns a constrained relink action for answer units
// whose source directory contains an explicit answer but whose answer record
// is absent or empty. Failed answer units are included as a fallback only when
// an unresolved choice has no usable directory row. The caller turns the
// returned unit windows into a normal repair run, so reset, lease, progress
// and audit behavior stay identical to other retries.
func (s *ImportConversationService) planMissingAnswerRetry(ctx context.Context, sessionID int64) (*domain.ConversationAction, int, error) {
	db := s.repo.DB()
	candidateRows, err := db.QueryContext(ctx, `SELECT document_id,COALESCE(section_scope_id,''),COALESCE(canonical_question_type,question_type,''),set_no,source_question_no FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate') AND answer_link_status!='confirmed'`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	candidates := []answerRetryCandidate{}
	for candidateRows.Next() {
		var candidate answerRetryCandidate
		if err := candidateRows.Scan(&candidate.documentID, &candidate.scope, &candidate.canonicalType, &candidate.setNo, &candidate.questionNo); err != nil {
			candidateRows.Close()
			return nil, 0, err
		}
		candidate.canonicalType = domain.CanonicalQuestionType(candidate.canonicalType)
		if requiresAnswer(candidate.canonicalType) && strings.TrimSpace(candidate.scope) != "" {
			candidates = append(candidates, candidate)
		}
	}
	candidateRows.Close()
	if err := candidateRows.Err(); err != nil {
		return nil, 0, err
	}

	// A non-empty answer record means the source was already extracted; if its
	// link is ambiguous, relink/review should resolve it without spending a
	// second vision call.
	answerRows, err := db.QueryContext(ctx, `SELECT document_id,COALESCE(section_scope_id,''),COALESCE(canonical_question_type,question_type,''),set_no,source_question_no,COALESCE(answer_json,'[]') FROM import_answer_records WHERE session_id=?`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	nonEmpty := map[answerRetryKey]bool{}
	for answerRows.Next() {
		var documentID int64
		var scope, canonicalType, answerJSON string
		var setNo, questionNo int
		if err := answerRows.Scan(&documentID, &scope, &canonicalType, &setNo, &questionNo, &answerJSON); err != nil {
			answerRows.Close()
			return nil, 0, err
		}
		if len(normalizedAnswerValues(answerJSON, domain.CanonicalQuestionType(canonicalType))) > 0 {
			nonEmpty[answerRetryKey{documentID: documentID, scope: scope, canonicalType: domain.CanonicalQuestionType(canonicalType), setNo: setNo, questionNo: questionNo}] = true
		}
	}
	answerRows.Close()
	if err := answerRows.Err(); err != nil {
		return nil, 0, err
	}

	directoryRows, err := db.QueryContext(ctx, `SELECT document_id,COALESCE(section_scope_id,''),COALESCE(canonical_question_type,question_type,''),set_no,source_question_no,source_page_start,source_page_end FROM import_directory_entries WHERE session_id=? AND status='active' AND content_role IN ('answer','explanation')`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	directory := []answerRetryDirectoryEntry{}
	for directoryRows.Next() {
		var entry answerRetryDirectoryEntry
		if err := directoryRows.Scan(&entry.documentID, &entry.scope, &entry.canonicalType, &entry.setNo, &entry.questionNo, &entry.pageStart, &entry.pageEnd); err != nil {
			directoryRows.Close()
			return nil, 0, err
		}
		entry.canonicalType = domain.CanonicalQuestionType(entry.canonicalType)
		if entry.pageStart > 0 && entry.pageEnd >= entry.pageStart {
			directory = append(directory, entry)
		}
	}
	directoryRows.Close()
	if err := directoryRows.Err(); err != nil {
		return nil, 0, err
	}

	unitRows, err := db.QueryContext(ctx, `SELECT u.id,u.import_job_id,j.document_id,u.context_page_start,u.context_page_end,u.status FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=? AND u.unit_type='extract_answer'`, sessionID)
	if err != nil {
		return nil, 0, err
	}
	units := []answerRetryUnit{}
	for unitRows.Next() {
		var unit answerRetryUnit
		if err := unitRows.Scan(&unit.id, &unit.jobID, &unit.documentID, &unit.contextStart, &unit.contextEnd, &unit.status); err != nil {
			unitRows.Close()
			return nil, 0, err
		}
		units = append(units, unit)
	}
	unitRows.Close()
	if err := unitRows.Err(); err != nil {
		return nil, 0, err
	}

	selected := map[int64]answerRetryUnit{}
	missingWithoutDirectory := false
	for _, candidate := range candidates {
		found := false
		for key := range nonEmpty {
			if key.documentID == candidate.documentID && key.scope == candidate.scope && key.canonicalType == candidate.canonicalType && key.questionNo == candidate.questionNo && answerSetCompatible(key.setNo, candidate.setNo) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		matchedDirectory := false
		for _, entry := range directory {
			if entry.documentID != candidate.documentID || entry.scope != candidate.scope || entry.canonicalType != candidate.canonicalType || entry.questionNo != candidate.questionNo || !answerSetCompatible(entry.setNo, candidate.setNo) {
				continue
			}
			matchedDirectory = true
			for _, unit := range units {
				if unit.documentID == entry.documentID && rangesOverlap([][]int{{entry.pageStart, entry.pageEnd}}, unit.contextStart, unit.contextEnd) {
					selected[unit.id] = unit
				}
			}
		}
		if !matchedDirectory {
			// A missing directory row means the only evidence is a failed
			// answer unit. In that case include failed windows as a recovery
			// fallback; otherwise do not re-run a failed window whose explicit
			// answer was already recovered from its directory anchor.
			missingWithoutDirectory = true
		}
	}
	if missingWithoutDirectory {
		for _, unit := range units {
			if unit.status == "failed" {
				selected[unit.id] = unit
			}
		}
	}
	if len(selected) == 0 {
		return nil, 0, nil
	}
	ordered := make([]answerRetryUnit, 0, len(selected))
	for _, unit := range selected {
		ordered = append(ordered, unit)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].documentID != ordered[j].documentID {
			return ordered[i].documentID < ordered[j].documentID
		}
		return ordered[i].contextStart < ordered[j].contextStart
	})
	action := &domain.ConversationAction{Intent: "relink_answers", PageRanges: make([][]int, 0, len(ordered))}
	for _, unit := range ordered {
		action.PageRanges = append(action.PageRanges, []int{unit.contextStart, unit.contextEnd})
	}
	return action, len(ordered), nil
}

func answerSetCompatible(left, right int) bool {
	return left == right || left == 0 || right == 0
}

// RetryAnswerPages resets only answer/explanation units intersecting the
// requested physical page window. It intentionally does not touch directory
// or question extraction units.
func (s *ImportConversationService) RetryAnswerPages(ctx context.Context, sessionID int64, expectedVersion, start, end int) error {
	if start <= 0 || end < start {
		return fmt.Errorf("INVALID_PAGE_RANGE")
	}
	session, err := s.repo.GetImportSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if expectedVersion > 0 && session.DraftVersion != expectedVersion {
		return fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	content := fmt.Sprintf("重新识别答案页 %d-%d", start, end)
	messageID, err := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: sessionID, Role: "user", MessageType: "text", Content: content, Status: "completed"})
	if err != nil {
		return err
	}
	action := &domain.ConversationAction{Intent: "relink_answers", PageRanges: [][]int{{start, end}}, Instruction: content}
	return s.executeRepair(ctx, session, &messageID, action)
}

var commitPhrases = map[string]bool{"确认提交题库": true, "确认入库": true, "提交当前草稿": true}

func (s *ImportConversationService) Preview(ctx context.Context, sessionID int64) (map[string]interface{}, error) {
	preview, err := s.repo.PreviewImportSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(preview)
	messageID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: sessionID, Role: "assistant", MessageType: "preview", Content: "预检已完成。", MetadataJSON: string(raw), Status: "completed"})
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: sessionID, MessageID: &messageID, Type: "preview_ready", Stage: "validating", AgentRole: "ValidationAgent", Summary: "草稿预检已完成", PayloadJSON: string(raw)})
	return preview, nil
}

func (s *ImportConversationService) Commit(ctx context.Context, sessionID int64, expectedVersion int, previewHash string) (*sqlite.VisionCommitResult, error) {
	result, err := s.repo.CommitImportSession(ctx, sessionID, expectedVersion, previewHash)
	if err != nil {
		return nil, err
	}
	for _, id := range result.QuestionIDs {
		q, getErr := s.repo.GetQuestion(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		if getErr = s.repo.BuildQuestionFTS(ctx, q); getErr != nil {
			return nil, getErr
		}
	}
	docs, _ := s.repo.ListSessionDocuments(ctx, sessionID)
	for _, doc := range docs {
		if doc.ImportJobID != nil {
			_, _ = s.repo.EnqueueJob(ctx, "embedding", ImportPayload{ImportJobID: *doc.ImportJobID}, 0)
			break
		}
	}
	content := fmt.Sprintf("已提交题库：新增 %d 题，重复 %d 题。正在生成检索索引。", result.Inserted, result.Duplicates)
	messageID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: sessionID, Role: "assistant", MessageType: "tool_result", Content: content, Status: "completed"})
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: sessionID, MessageID: &messageID, Type: "tool_completed", Stage: "embedding", AgentRole: "CommitAgent", Summary: content})
	return result, nil
}

func (s *ImportConversationService) HandleMessage(ctx context.Context, session *domain.ImportSession, content string, expectedVersion int) (*domain.ConversationAction, error) {
	content = strings.TrimSpace(content)
	if expectedVersion != session.DraftVersion {
		return nil, fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	user := &domain.ImportSessionMessage{SessionID: session.ID, Role: "user", MessageType: "text", Content: content, Status: "completed"}
	messageID, err := s.repo.AddSessionMessage(ctx, user)
	if err != nil {
		return nil, err
	}
	if commitPhrases[content] {
		preview, err := s.repo.PreviewImportSession(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		if ok, _ := preview["can_commit"].(bool); !ok {
			return nil, fmt.Errorf("IMPORT_PREVIEW_BLOCKED")
		}
		hash, _ := preview["preview_hash"].(string)
		_, err = s.Commit(ctx, session.ID, expectedVersion, hash)
		return &domain.ConversationAction{Intent: "commit_draft", Instruction: content}, err
	}
	action := s.planAction(ctx, session.UserID, content)
	action.Instruction = content
	docs, err := s.repo.ListSessionDocuments(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if len(docs) > 1 && len(action.PageRanges) > 0 && len(action.DocumentIDs) == 0 {
		meta, _ := json.Marshal(map[string]interface{}{"question": "请确认要重识别哪一份文档的页码。", "documents": docs})
		assistantID, _ := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, ParentMessageID: &messageID, Role: "assistant", MessageType: "clarification", Content: "当前会话包含多份文档，请先指定题本或具体文件名。", MetadataJSON: string(meta), Status: "completed"})
		_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: session.ID, MessageID: &assistantID, Type: "clarification_required", Stage: "planning", AgentRole: "ImportConversationAgent", Summary: "页码在多文档会话中存在歧义", PayloadJSON: string(meta)})
		return action, nil
	}
	for _, documentID := range action.DocumentIDs {
		if !s.repo.SessionOwnsDocument(ctx, session.ID, documentID) {
			return nil, fmt.Errorf("DOCUMENT_SCOPE_VIOLATION")
		}
	}
	for _, candidateID := range action.CandidateIDs {
		if !s.repo.SessionOwnsCandidate(ctx, session.ID, candidateID) {
			return nil, fmt.Errorf("CANDIDATE_SCOPE_VIOLATION")
		}
	}
	// A file-only upload starts the session's import run immediately. A combined
	// file + prompt upload defers that run until this message arrives, while a
	// follow-up instruction must not create a second run when work is already
	// active. Keep the clarification path above available for multi-document
	// page requests.
	if session.ActiveRunID != nil && actionStartsImportRun(action.Intent) {
		if run, runErr := s.repo.GetImportRun(ctx, *session.ActiveRunID); runErr != nil {
			return nil, runErr
		} else if run.Status == "queued" || run.Status == "running" || run.Status == "merging" {
			reply := "当前会话已有识别任务正在执行，刚上传的文档正在处理中，不需要重复发送导入指令。请等待本轮完成后再继续操作。"
			_, replyErr := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{
				SessionID:       session.ID,
				ParentMessageID: &messageID,
				Role:            "assistant",
				MessageType:     "text",
				Content:         reply,
				Status:          "completed",
			})
			return &domain.ConversationAction{Intent: "explain_status", Instruction: content}, replyErr
		}
	}
	switch action.Intent {
	case "start_full_extraction":
		return action, s.executeFullExtraction(ctx, session, &messageID, action)
	case "preview_draft":
		_, err = s.Preview(ctx, session.ID)
		return action, err
	case "explain_status":
		reply := fmt.Sprintf("当前会话状态为 %s：统一草稿 %d 题，完整 %d 题，阻断 %d 项。", session.Status, session.UniqueCount, session.CompleteCount, session.BlockingIssueCount)
		_, err = s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, ParentMessageID: &messageID, Role: "assistant", MessageType: "text", Content: reply, Status: "completed"})
		return action, err
	case "propose_delete_candidates":
		action.RequiresConfirmation = true
		actionID, createErr := s.repo.CreateActionRequest(ctx, session.ID, &messageID, "delete_candidates", action, expectedVersion)
		if createErr != nil {
			return nil, createErr
		}
		meta, _ := json.Marshal(map[string]interface{}{"action_id": actionID, "action": action})
		_, err = s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, ParentMessageID: &messageID, Role: "assistant", MessageType: "action_card", Content: "删除会改变统一草稿，需要确认后执行。", MetadataJSON: string(meta), Status: "pending"})
		return action, err
	case "retry_pages", "recover_missing_questions", "relink_answers":
		return action, s.executeRepair(ctx, session, &messageID, action)
	case "review_pending_candidates":
		if action.ReviewScope == "" {
			action.ReviewScope = "pending"
		}
		// "auto" lets the engine route each candidate by its own blocking
		// problem. Most pending candidates are blocked by answer-side issues,
		// which a forced question-role review can never repair.
		if action.TargetRole == "" {
			action.TargetRole = "auto"
		}
		_, _, err = s.StartCandidateReview(ctx, session, &messageID, action)
		return action, err
	default:
		reply := "我已读取当前会话。你可以要求我重新识别指定文档页码、补回缺题、重新对齐答案、预检或提交题库。"
		_, err = s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, ParentMessageID: &messageID, Role: "assistant", MessageType: "text", Content: reply, Status: "completed"})
		return action, err
	}
}

func actionStartsImportRun(intent string) bool {
	switch intent {
	case "start_full_extraction", "retry_pages", "recover_missing_questions", "review_pending_candidates", "relink_answers":
		return true
	default:
		return false
	}
}

func (s *ImportConversationService) executeFullExtraction(ctx context.Context, session *domain.ImportSession, messageID *int64, action *domain.ConversationAction) error {
	run, err := s.repo.CreateSessionRun(ctx, session.ID, messageID, "initial_extract", action)
	if err != nil {
		return err
	}
	docs, err := s.repo.ListSessionDocuments(ctx, session.ID)
	if err != nil {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "DOCUMENT_QUERY_FAILED", err.Error(), session.DraftVersion)
		return err
	}
	jobs := 0
	for _, doc := range docs {
		if doc.ImportJobID == nil || (len(action.DocumentIDs) > 0 && !containsInt64(action.DocumentIDs, doc.DocumentID)) {
			continue
		}
		if _, err = s.repo.DB().ExecContext(ctx, `UPDATE import_jobs SET status='uploaded',stage='uploaded',error_code=NULL,error_message=NULL,progress=0,updated_at=datetime('now') WHERE id=? AND session_id=?`, *doc.ImportJobID, session.ID); err != nil {
			_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "IMPORT_RESET_FAILED", err.Error(), session.DraftVersion)
			return err
		}
		jobs++
	}
	if jobs == 0 {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "NO_TARGET_DOCUMENTS", "当前会话没有可重新识别的文档", session.DraftVersion)
		return fmt.Errorf("NO_TARGET_DOCUMENTS")
	}
	summary := fmt.Sprintf("已重新启动视觉识别，将处理 %d 份文档。", jobs)
	assistantID, messageErr := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, RunID: &run.ID, ParentMessageID: messageID, Role: "assistant", MessageType: "agent_run", Content: summary, Status: "running"})
	if messageErr != nil {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "IMPORT_MESSAGE_FAILED", messageErr.Error(), session.DraftVersion)
		return messageErr
	}
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: session.ID, RunID: &run.ID, MessageID: &assistantID, Type: "run_started", Stage: "analyzing", AgentRole: "ImportConversationAgent", Summary: summary})
	runID := run.ID
	for _, doc := range docs {
		if doc.ImportJobID == nil || (len(action.DocumentIDs) > 0 && !containsInt64(action.DocumentIDs, doc.DocumentID)) {
			continue
		}
		if _, err = s.repo.EnqueueJob(ctx, "import_pdf", ImportPayload{ImportJobID: *doc.ImportJobID, RunID: &runID}, 10); err != nil {
			_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "IMPORT_ENQUEUE_FAILED", err.Error(), session.DraftVersion)
			return err
		}
	}
	return nil
}

func (s *ImportConversationService) executeRepair(ctx context.Context, session *domain.ImportSession, messageID *int64, action *domain.ConversationAction) error {
	// Natural-language requests such as “补充识别待定题目” do not carry page
	// ranges. Previously that empty range selected every extract_question unit,
	// turning one malformed question into a two-hour near-full re-extraction.
	// Resolve the durable missing/structural candidates first and persist those
	// ranges in the run action so the worker and progress UI share the same scope.
	if action.Intent == "recover_missing_questions" && len(action.PageRanges) == 0 {
		planned, count, planErr := s.planMissingQuestionRetry(ctx, session.ID)
		if planErr != nil {
			return planErr
		}
		if count == 0 {
			return fmt.Errorf("NO_TARGET_UNITS")
		}
		action.PageRanges = planned.PageRanges
		if len(action.DocumentIDs) == 0 {
			action.DocumentIDs = planned.DocumentIDs
		}
	}
	run, err := s.repo.CreateSessionRun(ctx, session.ID, messageID, "repair", action)
	if err != nil {
		return err
	}
	query := `SELECT u.id,j.id,u.unit_type,u.context_page_start,u.context_page_end,j.document_id FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=?`
	rows, err := s.repo.DB().QueryContext(ctx, query, session.ID)
	if err != nil {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "UNIT_QUERY_FAILED", err.Error(), session.DraftVersion)
		return err
	}
	unitIDs := []int64{}
	jobs := map[int64]bool{}
	for rows.Next() {
		var unitID, jobID, documentID int64
		var unitType string
		var start, end int
		if err := rows.Scan(&unitID, &jobID, &unitType, &start, &end, &documentID); err != nil {
			rows.Close()
			_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "UNIT_QUERY_FAILED", err.Error(), session.DraftVersion)
			return err
		}
		if len(action.DocumentIDs) > 0 && !containsInt64(action.DocumentIDs, documentID) {
			continue
		}
		typeMatch := action.Intent == "retry_pages" || (action.Intent == "recover_missing_questions" && (unitType == "directory" || unitType == "extract_question")) || (action.Intent == "relink_answers" && unitType == "extract_answer")
		if !typeMatch {
			continue
		}
		if len(action.PageRanges) > 0 && !rangesOverlap(action.PageRanges, start, end) {
			continue
		}
		unitIDs = append(unitIDs, unitID)
		jobs[jobID] = true
	}
	rows.Close()
	if len(unitIDs) == 0 {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "NO_TARGET_UNITS", "没有找到符合指令的 Agent 单元", session.DraftVersion)
		return fmt.Errorf("NO_TARGET_UNITS")
	}
	_, err = s.repo.RetrySessionAgentUnits(ctx, session.ID, unitIDs)
	if err != nil {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "UNIT_RETRY_FAILED", err.Error(), session.DraftVersion)
		return err
	}
	summary := fmt.Sprintf("已创建定向修复，将重新执行 %d 个 Agent 单元。", len(unitIDs))
	assistantID, messageErr := s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, RunID: &run.ID, ParentMessageID: messageID, Role: "assistant", MessageType: "agent_run", Content: summary, Status: "running"})
	if messageErr != nil {
		_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "IMPORT_MESSAGE_FAILED", messageErr.Error(), session.DraftVersion)
		return messageErr
	}
	_, _ = s.repo.AddImportEvent(ctx, &domain.ImportEvent{SessionID: session.ID, RunID: &run.ID, MessageID: &assistantID, Type: "run_started", Stage: "repair", AgentRole: "ImportConversationAgent", Summary: summary})
	runID := run.ID
	for jobID := range jobs {
		if _, err = s.repo.EnqueueJob(ctx, "import_pdf", ImportPayload{ImportJobID: jobID, RunID: &runID}, 10); err != nil {
			_ = s.repo.FinishSessionRun(ctx, session.ID, run.ID, "failed", "IMPORT_ENQUEUE_FAILED", err.Error(), session.DraftVersion)
			return err
		}
	}
	return nil
}

func (s *ImportConversationService) ConfirmAction(ctx context.Context, session *domain.ImportSession, actionID int64, expectedVersion int) error {
	actionType, raw, err := s.repo.ConfirmActionRequest(ctx, session.ID, actionID, expectedVersion)
	if err != nil {
		return err
	}
	if actionType != "delete_candidates" {
		return fmt.Errorf("UNSUPPORTED_ACTION")
	}
	var action domain.ConversationAction
	if json.Unmarshal([]byte(raw), &action) != nil {
		return fmt.Errorf("INVALID_ACTION")
	}
	for _, id := range action.CandidateIDs {
		if !s.repo.SessionOwnsCandidate(ctx, session.ID, id) {
			return fmt.Errorf("CANDIDATE_SCOPE_VIOLATION")
		}
	}
	tx, err := s.repo.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range action.CandidateIDs {
		if _, err = tx.ExecContext(ctx, `DELETE FROM candidate_questions WHERE id=? AND import_session_id=?`, id, session.ID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE import_sessions SET draft_version=draft_version+1,last_preview_hash=NULL,last_preview_version=NULL,status='needs_attention',updated_at=datetime('now') WHERE id=?`, session.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_, _ = s.repo.AddSessionMessage(ctx, &domain.ImportSessionMessage{SessionID: session.ID, Role: "assistant", MessageType: "tool_result", Content: fmt.Sprintf("已删除 %d 道草稿题，提交前需要重新预检。", len(action.CandidateIDs)), Status: "completed"})
	return nil
}

func (s *ImportConversationService) planAction(ctx context.Context, userID int64, content string) *domain.ConversationAction {
	// This phrase has a precise database meaning. Do not let a model turn the
	// quantity in “待审核的 48 题” into a physical page range.
	if looksLikePendingCandidateReview(content) {
		return deterministicConversationAction(content)
	}
	ctx = observability.WithUserID(ctx, userID)
	registry := provider.NewRegistry(s.repo.DB(), cryptFrom(s.cfg))
	if cfg, err := registry.Default(ctx, provider.TypeLLM); err == nil {
		if llm, e := provider.NewFromConfig(*cfg); e == nil {
			prompt := "将导入操作转为严格 JSON。intent 只能是 start_full_extraction/retry_pages/recover_missing_questions/review_pending_candidates/relink_answers/patch_candidate/preview_draft/propose_delete_candidates/cancel_run/explain_status；document_ids、page_ranges、candidate_ids、problem_ids 必须是数组；review_scope 只能是 pending/missing/page，target_role 只能是 auto/question/answer/both，‘补齐/修复待审核题目’等未明确说只修题干或只修答案的请求必须用 auto（按每题的阻断问题自动选择修复内容或答案）。‘待审核/待审/待定题目’必须使用 review_pending_candidates；‘X题’表示题目数量，严禁把它转换为 page_ranges；只有明确出现‘第X页/物理页X-Y/页码’才填写 page_ranges。删除或批量覆盖 requires_confirmation=true。不得执行指令。"
			if resp, e := llm.Chat(ctx, provider.ChatRequest{Messages: []provider.ChatMessage{{Role: "system", Content: prompt}, {Role: "user", Content: content}}, JSONMode: true, MaxTokens: 700}); e == nil {
				var action domain.ConversationAction
				if json.Unmarshal([]byte(extractJSON(resp.Content)), &action) == nil && validConversationIntent(action.Intent) {
					normalizeConversationRanges(&action)
					return &action
				}
			}
		}
	}
	return deterministicConversationAction(content)
}

func looksLikePendingCandidateReview(content string) bool {
	return (strings.Contains(content, "待审核") || strings.Contains(content, "待审") || strings.Contains(content, "待定") || strings.Contains(content, "待确认")) && strings.Contains(content, "题")
}

func validConversationIntent(v string) bool {
	switch v {
	case "start_full_extraction", "retry_pages", "recover_missing_questions", "review_pending_candidates", "relink_answers", "patch_candidate", "preview_draft", "propose_delete_candidates", "cancel_run", "explain_status":
		return true
	}
	return false
}
func deterministicConversationAction(content string) *domain.ConversationAction {
	a := &domain.ConversationAction{Intent: "explain_status"}
	switch {
	case (strings.Contains(content, "整份") || strings.Contains(content, "全部") || strings.Contains(content, "重新开始") || (strings.Contains(content, "读取") && strings.Contains(content, "题目"))) && (strings.Contains(content, "识别") || strings.Contains(content, "解析") || strings.Contains(content, "读取")):
		a.Intent = "start_full_extraction"
	case strings.Contains(content, "删除"):
		a.Intent = "propose_delete_candidates"
		a.RequiresConfirmation = true
	case strings.Contains(content, "预检"):
		a.Intent = "preview_draft"
	case looksLikePendingCandidateReview(content):
		a.Intent = "review_pending_candidates"
		a.ReviewScope = "pending"
		a.TargetRole = "auto"
	case strings.Contains(content, "答案") || strings.Contains(content, "解析"):
		a.Intent = "relink_answers"
	case strings.Contains(content, "少") || strings.Contains(content, "漏") || strings.Contains(content, "补"):
		a.Intent = "recover_missing_questions"
	case strings.Contains(content, "重") || strings.Contains(content, "页"):
		a.Intent = "retry_pages"
	}
	// Bare numbers in “48题” are counts, not physical PDF pages. Only page
	// wording can populate a page range; this prevents the historical [48,48]
	// misrouting of pending-candidate requests.
	pageText := strings.Contains(content, "页") || strings.Contains(content, "页码") || strings.Contains(content, "物理页")
	if !pageText {
		return a
	}
	numbers := regexp.MustCompile(`\d+`).FindAllString(content, -1)
	values := []int{}
	for _, value := range numbers {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			values = append(values, n)
		}
	}
	if len(values) == 1 {
		a.PageRanges = [][]int{{values[0], values[0]}}
	} else if len(values) >= 2 {
		a.PageRanges = [][]int{{values[0], values[1]}}
	}
	return a
}
func normalizeConversationRanges(a *domain.ConversationAction) {
	for i, r := range a.PageRanges {
		if len(r) >= 2 && r[0] > r[1] {
			a.PageRanges[i][0], a.PageRanges[i][1] = r[1], r[0]
		}
	}
}
func containsInt64(values []int64, target int64) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func rangesOverlap(ranges [][]int, start, end int) bool {
	for _, r := range ranges {
		if len(r) >= 2 && end >= r[0] && start <= r[1] {
			return true
		}
	}
	return false
}
