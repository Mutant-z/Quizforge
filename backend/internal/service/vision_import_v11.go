package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
)

const catalogPromptVersion = "vision-catalog.v1.1"

type catalogPayload struct {
	Sections []catalogSection `json:"sections"`
	Pages    []catalogPage    `json:"pages"`
	Entries  []catalogEntry   `json:"entries"`
}
type catalogSection struct {
	ID           string   `json:"id"`
	Path         []string `json:"path"`
	SetNo        int      `json:"set_no"`
	QuestionType string   `json:"question_type"`
}
type catalogPage struct {
	Page      int    `json:"page"`
	Role      string `json:"role"`
	SectionID string `json:"section_id"`
}
type catalogEntry struct {
	ID                    string               `json:"id"`
	ContentRole           string               `json:"content_role"`
	SectionID             string               `json:"section_id"`
	OriginalQuestionLabel string               `json:"original_question_label"`
	QuestionType          string               `json:"question_type"`
	SequenceInScope       int                  `json:"sequence_in_scope"`
	SourceQuestionNo      int                  `json:"source_question_no"`
	SourcePageStart       int                  `json:"source_page_start"`
	SourcePageEnd         int                  `json:"source_page_end"`
	Anchor                string               `json:"anchor"`
	MarkerTop             int                  `json:"marker_top"`
	MarkerBottom          int                  `json:"marker_bottom"`
	Region                *domain.VisionRegion `json:"region,omitempty"`
}

type extractionStageResult struct {
	outputs  map[int]string
	failures int
	err      error
}

func (s *ImportService) runVisionImportV11(ctx context.Context, imp *domain.ImportJob, doc *domain.Document) error {
	ctx = observability.WithUserID(ctx, imp.UserID)
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}
	s.emitVisionEvent(ctx, imp, &domain.ImportEvent{Type: "run_started", Stage: "analyzing", AgentRole: "ImportCoordinator", DocumentID: &doc.ID, Summary: "已采用 vision-agent.v1.1 平衡策略开始视觉导入"})
	registry := provider.NewRegistry(s.repo.DB(), cryptFrom(s.cfg))
	providerConfig, err := registry.Default(ctx, provider.TypeLLM)
	if err != nil {
		return s.pauseVisionImport(ctx, imp, doc.ID, domain.ImportStatusNeedsModelConfiguration, "VISION_MODEL_REQUIRED", "请先配置默认多模态 LLM Provider")
	}
	if !providerConfig.SupportsVision {
		return s.pauseVisionImport(ctx, imp, doc.ID, domain.ImportStatusNeedsModelConfiguration, "VISION_MODEL_REQUIRED", "默认 LLM 未声明支持图片输入，请在模型设置中启用并测试多模态能力")
	}
	providerConfig.TimeoutSec = s.cfg.Worker.VisionTimeoutSeconds
	llm, err := provider.NewFromConfig(*providerConfig)
	if err != nil {
		return err
	}
	if err := s.repo.SnapshotVisionProvider(ctx, imp.ID, providerConfig.ID, providerConfig.ChatModel); err != nil {
		return err
	}
	callOptions := visionCallOptions{ProviderKey: fmt.Sprintf("%d|%s|%s", providerConfig.ID, providerConfig.BaseURL, providerConfig.ChatModel), Local: isLoopbackProvider(providerConfig.BaseURL)}

	s.updateStage(ctx, imp.ID, domain.ImportStatusExtracting, "正在读取 PDF 页数与诊断文本层", 0.01)
	pages, err := parserParseDocument(ctx, doc.FilePath, doc.MimeType)
	if err != nil {
		return err
	}
	pageCount := len(pages)
	if pageCount == 0 {
		return fmt.Errorf("PDF has no pages")
	}
	if pageCount > s.cfg.Worker.VisionMaxPages {
		return s.pauseVisionImport(ctx, imp, doc.ID, domain.ImportStatusNeedsAttention, "VISION_PAGE_LIMIT", fmt.Sprintf("PDF 共 %d 页，超过视觉导入上限 %d 页，请拆分后上传", pageCount, s.cfg.Worker.VisionMaxPages))
	}
	if err := s.repo.SaveDocumentPages(ctx, doc.ID, pages); err != nil {
		return err
	}
	_ = s.repo.UpdateDocumentPages(ctx, doc.ID, pageCount)

	s.updateStage(ctx, imp.ID, domain.ImportStatusRendering, "正在生成 JPEG v2 页面资源", 0.02)
	s.emitVisionStage(ctx, imp, doc.ID, "rendering", "PageRenderAgent", fmt.Sprintf("开始生成 %d 页 Catalog 图和提取图", pageCount), 0, pageCount*2)
	if err := s.renderVisionPages(ctx, doc, pageCount, imp.PipelineVersion, func(done, total int, purpose string, page int) {
		if done == 1 || done == total || done%10 == 0 {
			s.emitVisionProgress(ctx, imp, doc.ID, "rendering", "PageRenderAgent", fmt.Sprintf("页面资源 %d/%d，最近完成物理页 %d（%s）", done, total, page, purpose), done, total, nil)
			s.persistVisionRunProgress(ctx, imp, "rendering", done, total, 0, 0, 2, "")
		}
	}); err != nil {
		return err
	}
	if err := s.runOCRForPages(ctx, doc, pages, "extraction", extractionRenderVersionV11); err != nil {
		code := "OCR_FAILED"
		if strings.Contains(err.Error(), "OCR_REQUIRED") {
			code = "OCR_REQUIRED"
		}
		return s.pauseVisionImport(ctx, imp, doc.ID, domain.ImportStatusNeedsAttention, code, err.Error())
	}

	windows := buildVisionWindows(pageCount)
	s.updateStage(ctx, imp.ID, domain.ImportStatusDirectoryBuilding, "Catalog Agent 正在一次完成页面画像与目录", 0.05)
	s.emitVisionStage(ctx, imp, doc.ID, "catalog", "CatalogAgent", fmt.Sprintf("合并画像与目录识别，共 %d 个 4+1 页窗", len(windows)), 0, len(windows))
	catalogs, catalogFailures, err := s.runVisionStage(ctx, imp, doc, llm, windows, "catalog", s.cfg.Worker.DirectoryConcurrency, func(ctx context.Context, unit *domain.ImportAgentUnit, window visionPageWindow) (string, error) {
		opts := callOptions
		opts.MaxTokens = 12000
		opts.Schema = "catalog"
		return s.callVisionAgent(ctx, imp, llm, doc, unit, "catalog", catalogPromptVersion, catalogPrompt(window), opts)
	})
	if err != nil {
		return err
	}

	documentRole, roleConfidence, global, entriesByWindow, catalogIssues, directoryComplete, badCatalogWindows := coordinateCatalogV11(catalogs, windows, catalogFailures)
	if len(badCatalogWindows) > 0 {
		reviewWindows := []visionPageWindow{}
		for _, window := range windows {
			if badCatalogWindows[window.Index] {
				reviewWindows = append(reviewWindows, window)
			}
		}
		reviewed, reviewFailures, reviewErr := s.runVisionStage(ctx, imp, doc, llm, reviewWindows, "review_catalog", 1, func(reviewCtx context.Context, unit *domain.ImportAgentUnit, window visionPageWindow) (string, error) {
			if err := s.ensureVisionReviewAssets(reviewCtx, doc, window.ContextStart, window.ContextEnd); err != nil {
				return "", err
			}
			opts := callOptions
			opts.Schema = "catalog"
			opts.MaxTokens = 12000
			opts.UseReviewAssets = true
			return s.callVisionAgent(reviewCtx, imp, llm, doc, unit, "review", catalogPromptVersion+".review", catalogPrompt(window), opts)
		})
		if reviewErr != nil {
			return reviewErr
		}
		for index, raw := range reviewed {
			catalogs[index] = raw
		}
		catalogFailures = reviewFailures
		documentRole, roleConfidence, global, entriesByWindow, catalogIssues, directoryComplete, _ = coordinateCatalogV11(catalogs, windows, catalogFailures)
	}
	issues := append([]map[string]interface{}{}, catalogIssues...)
	if imp.SessionID != nil {
		runID := int64(0)
		if imp.RunID != nil {
			runID = *imp.RunID
		}
		mismatches, coverageErr := s.verifyCatalogCoverage(ctx, *imp.SessionID, doc.ID, runID, global)
		if coverageErr != nil {
			return coverageErr
		}
		if len(mismatches) > 0 {
			global, entriesByWindow, coverageErr = s.reviewCatalogCoveragePages(ctx, imp, doc, llm, callOptions, windows, global, entriesByWindow, mismatches)
			if coverageErr != nil {
				return coverageErr
			}
			remaining, verifyErr := s.verifyCatalogCoverage(ctx, *imp.SessionID, doc.ID, runID, global)
			if verifyErr != nil {
				return verifyErr
			}
			remainingPages := map[int]bool{}
			for _, mismatch := range remaining {
				remainingPages[mismatch.Page] = true
			}
			for _, repaired := range mismatches {
				if !remainingPages[repaired.Page] {
					_, _ = s.repo.DB().ExecContext(ctx, `UPDATE import_page_coverage SET status='repaired',updated_at=datetime('now') WHERE session_id=? AND document_id=? AND page_no=?`, *imp.SessionID, doc.ID, repaired.Page)
				}
			}
			if len(remaining) > 0 {
				directoryComplete = false
				for _, mismatch := range remaining {
					issues = append(issues, coverageIssue(mismatch))
				}
			}
		}
	}
	questionEntries, answerEntries := splitVisionEntries(global)
	if len(questionEntries) == 0 && documentRole != "answer_book" && documentRole != "explanation_book" {
		directoryComplete = false
		issues = append(issues, issue("NO_QUESTION_CANDIDATES", 1, pageCount, "Catalog Agent 未识别到任何题目"))
	}
	for _, gap := range detectVisionDirectoryGaps(questionEntries) {
		directoryComplete = false
		issues = append(issues, gap)
	}
	if imp.SessionID != nil {
		profileJSON, _ := json.Marshal(map[string]interface{}{"document_role": documentRole, "catalogs": catalogs})
		_ = s.repo.UpdateSessionDocumentProfile(ctx, *imp.SessionID, doc.ID, documentRole, roleConfidence, string(profileJSON))
		allEntries := make([]domain.VisionDirectoryEntry, 0, len(global))
		for _, entry := range global {
			allEntries = append(allEntries, entry)
		}
		// The coordinator emits the complete document projection. Replace the
		// derived rows atomically so a retry cannot leave stale type-only
		// identities beside their normalized counterparts.
		if err := s.repo.ReplaceDirectoryEntries(ctx, *imp.SessionID, doc.ID, 0, documentRole, allEntries); err != nil {
			return err
		}
	}

	s.updateStage(ctx, imp.ID, domain.ImportStatusExtractingQuestions, "题目与答案 Agent 正在并行提取", 0.35)
	s.emitVisionStage(ctx, imp, doc.ID, "parallel_extraction", "ImportCoordinator", fmt.Sprintf("并行启动题目 %d 条、答案/解析 %d 条", len(questionEntries), len(answerEntries)), 0, 2)
	questionCh := make(chan extractionStageResult, 1)
	answerCh := make(chan extractionStageResult, 1)
	questions := map[string]domain.VisionQuestion{}
	candidateIDs := map[string]int64{}
	answersByKey := map[string][]domain.VisionAnswer{}
	allAnswers := []domain.VisionAnswer{}
	entryByAnswerKey := map[string]domain.VisionDirectoryEntry{}
	for _, entry := range questionEntries {
		entryByAnswerKey[visionAnswerKey(entry.SectionPath, entry.SetNo, entry.QuestionType, entry.SourceQuestionNo)] = entry
	}
	linked := map[string]bool{}
	var draftMu sync.Mutex
	questionCompleted := func(callbackCtx context.Context, window visionPageWindow, raw string) error {
		var payload questionPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		reviewedByIdentity := map[string]domain.VisionQuestion{}
		needsReview := false
		for _, question := range payload.Questions {
			_, knownIdentity := findVisionDirectoryEntry(global, question)
			if !knownIdentity || question.SourcePageStart < window.ContextStart || question.SourcePageEnd > window.ContextEnd || question.VisualConfidence < 0.65 || len(validateVisionQuestion(question)) > 0 {
				needsReview = true
				break
			}
		}
		if needsReview {
			if reviewed, reviewErr := s.reviewQuestionWindowV11(callbackCtx, imp, doc, llm, window, entriesByWindow[window.Index], callOptions); reviewErr == nil {
				payload.Questions = reviewed
				for _, item := range reviewed {
					reviewedByIdentity[item.SourceIdentity] = item
				}
			}
		}
		// Treat the directory as a conservation contract. A valid JSON response
		// that contains only part of a dense page window is incomplete and must
		// receive a focused high-resolution review for the missing identities.
		returned := map[string]bool{}
		for _, question := range payload.Questions {
			if entry, ok := findVisionDirectoryEntry(global, question); ok {
				returned[entry.SourceIdentity] = true
			} else {
				returned[question.SourceIdentity] = true
			}
		}
		missing := make([]domain.VisionDirectoryEntry, 0)
		for _, entry := range entriesByWindow[window.Index] {
			if entry.ContentRole == "question" && !returned[entry.SourceIdentity] {
				missing = append(missing, entry)
			}
		}
		if len(missing) > 0 {
			if reviewed, reviewErr := s.reviewQuestionWindowV11(callbackCtx, imp, doc, llm, window, missing, callOptions); reviewErr == nil {
				for _, question := range reviewed {
					if !returned[question.SourceIdentity] {
						payload.Questions = append(payload.Questions, question)
						returned[question.SourceIdentity] = true
					}
				}
			}
		}
		for _, q := range payload.Questions {
			entry, ok := findVisionDirectoryEntry(global, q)
			if !ok {
				draftMu.Lock()
				issues = append(issues, issue("SOURCE_ID_MISMATCH", q.SourcePageStart, q.SourcePageEnd, "模型返回目录外题目 "+q.SourceIdentity))
				draftMu.Unlock()
				continue
			}
			if reviewed, exists := reviewedByIdentity[q.SourceIdentity]; exists {
				q = reviewed
			}
			q.SourceIdentity = entry.SourceIdentity
			candidateID, insertErr := s.repo.InsertVisionCandidate(callbackCtx, imp.ID, doc.ID, entry, q)
			if insertErr != nil {
				return insertErr
			}
			answerKey := visionAnswerKey(entry.SectionPath, entry.SetNo, entry.QuestionType, entry.SourceQuestionNo)
			draftMu.Lock()
			questions[q.SourceIdentity] = q
			candidateIDs[q.SourceIdentity] = candidateID
			matches := distinctVisionAnswerEvidence(append([]domain.VisionAnswer(nil), answersByKey[answerKey]...))
			current := len(questions)
			draftMu.Unlock()
			if len(matches) == 1 {
				q.Answer, q.Analysis = matches[0].Answer, matches[0].Analysis
				if err := s.repo.LinkVisionAnswer(callbackCtx, candidateID, matches[0], q); err != nil {
					return err
				}
				draftMu.Lock()
				questions[q.SourceIdentity] = q
				linked[q.SourceIdentity] = true
				draftMu.Unlock()
			}
			s.emitVisionEvent(callbackCtx, imp, &domain.ImportEvent{Type: "draft_patch", Stage: "extracting_questions", AgentRole: "QuestionExtractionAgent", DocumentID: &doc.ID, OwnedPageStart: q.SourcePageStart, OwnedPageEnd: q.SourcePageEnd, Current: current, Total: len(questionEntries), Summary: fmt.Sprintf("草稿已新增第 %d 题", entry.SourceQuestionNo)})
		}
		return nil
	}
	answerCompleted := func(callbackCtx context.Context, window visionPageWindow, raw string) error {
		var payload answerPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		normalizeWindowAnswers := func(items []domain.VisionAnswer) (map[string]domain.VisionAnswer, []domain.VisionAnswer) {
			byIdentity := map[string]domain.VisionAnswer{}
			invalid := []domain.VisionAnswer{}
			for _, answer := range items {
				if answer.SourcePageStart < window.ContextStart || answer.SourcePageEnd > window.ContextEnd || answer.SourcePageEnd < answer.SourcePageStart {
					invalid = append(invalid, answer)
					continue
				}
				normalized, matched, ok := normalizeVisionAnswerIdentity(answer, answerEntries)
				if !ok {
					invalid = append(invalid, answer)
					continue
				}
				byIdentity[matched.Entry.SourceIdentity] = normalized
			}
			return byIdentity, invalid
		}
		normalizedByIdentity, invalid := normalizeWindowAnswers(payload.Answers)
		expectedByIdentity := map[string]domain.VisionDirectoryEntry{}
		for _, entry := range entriesByWindow[window.Index] {
			if entry.ContentRole == "answer" || entry.ContentRole == "explanation" {
				expectedByIdentity[entry.SourceIdentity] = entry
			}
		}
		missing := func() []domain.VisionDirectoryEntry {
			out := []domain.VisionDirectoryEntry{}
			for identity, entry := range expectedByIdentity {
				if _, ok := normalizedByIdentity[identity]; !ok {
					out = append(out, entry)
				}
			}
			return out
		}
		missingEntries := missing()
		if len(invalid) > 0 || len(missingEntries) > 0 {
			if reviewed, reviewErr := s.reviewAnswerWindowV11(callbackCtx, imp, doc, llm, window, entriesByWindow[window.Index], callOptions); reviewErr == nil {
				reviewedByIdentity, reviewedInvalid := normalizeWindowAnswers(reviewed)
				for identity, answer := range reviewedByIdentity {
					normalizedByIdentity[identity] = answer
				}
				invalid = reviewedInvalid
				missingEntries = missing()
			}
		}
		for _, answer := range invalid {
			code := "ANSWER_SOURCE_IDENTITY_INVALID"
			detail := "模型返回的答案身份无法在答案目录中唯一匹配"
			if answer.SourcePageStart < window.ContextStart || answer.SourcePageEnd > window.ContextEnd || answer.SourcePageEnd < answer.SourcePageStart {
				code = "ANSWER_PAGE_OUT_OF_RANGE"
				detail = "答案来源页超出 Agent 附件范围"
			}
			draftMu.Lock()
			issues = append(issues, issue(code, answer.SourcePageStart, answer.SourcePageEnd, detail))
			draftMu.Unlock()
		}
		for _, entry := range missingEntries {
			draftMu.Lock()
			issues = append(issues, issue("ANSWER_MISSING", entry.SourcePageStart, entry.SourcePageEnd, fmt.Sprintf("答案目录中的 %s 第 %d 题未被提取", entry.SectionScopeID, entry.SourceQuestionNo)))
			draftMu.Unlock()
		}
		for _, entry := range answerEntries {
			answer, ok := normalizedByIdentity[entry.SourceIdentity]
			if !ok {
				continue
			}
			key := visionAnswerKey(entry.SectionPath, entry.SetNo, entry.QuestionType, entry.SourceQuestionNo)
			draftMu.Lock()
			answersByKey[key] = append(answersByKey[key], answer)
			allAnswers = append(allAnswers, answer)
			entry, hasEntry := entryByAnswerKey[key]
			candidateID := candidateIDs[entry.SourceIdentity]
			question := questions[entry.SourceIdentity]
			matchCount := len(distinctVisionAnswerEvidence(answersByKey[key]))
			current := len(allAnswers)
			draftMu.Unlock()
			if imp.SessionID != nil {
				if err := s.repo.UpsertAnswerRecords(callbackCtx, *imp.SessionID, doc.ID, 0, []domain.VisionAnswer{answer}); err != nil {
					return err
				}
			}
			if hasEntry && candidateID > 0 && matchCount == 1 {
				question.Answer, question.Analysis = answer.Answer, answer.Analysis
				if err := s.repo.LinkVisionAnswer(callbackCtx, candidateID, answer, question); err != nil {
					return err
				}
				draftMu.Lock()
				questions[entry.SourceIdentity] = question
				linked[entry.SourceIdentity] = true
				draftMu.Unlock()
			}
			s.emitVisionEvent(callbackCtx, imp, &domain.ImportEvent{Type: "answer_delta", Stage: "extracting_answers", AgentRole: "AnswerExtractionAgent", DocumentID: &doc.ID, OwnedPageStart: answer.SourcePageStart, OwnedPageEnd: answer.SourcePageEnd, Current: current, Total: len(answerEntries), Summary: fmt.Sprintf("已提取第 %d 题印刷答案", answer.SourceQuestionNo)})
		}
		return nil
	}
	go func() {
		outputs, failures, runErr := s.runExtractionStageV11(ctx, imp, doc, llm, windows, entriesByWindow, "question", callOptions, questionCompleted)
		questionCh <- extractionStageResult{outputs: outputs, failures: failures, err: runErr}
	}()
	go func() {
		outputs, failures, runErr := s.runExtractionStageV11(ctx, imp, doc, llm, windows, entriesByWindow, "answer", callOptions, answerCompleted)
		answerCh <- extractionStageResult{outputs: outputs, failures: failures, err: runErr}
	}()
	questionResult := <-questionCh
	if questionResult.err != nil {
		return questionResult.err
	}
	answerResult := <-answerCh
	if answerResult.err != nil {
		return answerResult.err
	}
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}

	s.updateStage(ctx, imp.ID, domain.ImportStatusLinking, "正在增量对齐题目、答案与解析", 0.90)
	s.persistVisionRunProgress(ctx, imp, "linking", 0, len(questionEntries), 0, 0, 1, "")
	completeCount := 0
	for index, entry := range questionEntries {
		if err := s.ensureImportSessionActive(ctx, imp); err != nil {
			return err
		}
		q, ok := questions[entry.SourceIdentity]
		if !ok {
			issues = append(issues, issue("QUESTION_MISSING", entry.SourcePageStart, entry.SourcePageEnd, "目录题目未被执行 Agent 返回"))
			continue
		}
		candidateID := candidateIDs[entry.SourceIdentity]
		candidateIssues := []map[string]interface{}{}
		matches := answersByKey[visionAnswerKey(entry.SectionPath, entry.SetNo, entry.QuestionType, entry.SourceQuestionNo)]
		matches = distinctVisionAnswerEvidence(matches)
		if len(matches) == 1 {
			q.Answer, q.Analysis = matches[0].Answer, matches[0].Analysis
			if !linked[entry.SourceIdentity] {
				if err := s.repo.LinkVisionAnswer(ctx, candidateID, matches[0], q); err != nil {
					return err
				}
			}
		} else if len(matches) > 1 {
			candidateIssues = append(candidateIssues, issue("ANSWER_CONFLICT", entry.SourcePageStart, entry.SourcePageEnd, "同一来源身份匹配到多个答案"))
		}
		candidateIssues = append(candidateIssues, validateVisionQuestion(q)...)
		if requiresAnswer(q.QuestionType) && len(q.Answer) == 0 {
			candidateIssues = append(candidateIssues, issue("ANSWER_REQUIRED", entry.SourcePageStart, entry.SourcePageEnd, "选择题或判断题缺少答案"))
		}
		if len(candidateIssues) == 0 {
			completeCount++
			_ = s.repo.SetVisionCandidateValidation(ctx, candidateID, "validated", nil)
		} else {
			issues = append(issues, candidateIssues...)
			_ = s.repo.SetVisionCandidateValidation(ctx, candidateID, "needs_review", candidateIssues)
		}
		s.persistVisionRunProgress(ctx, imp, "linking", index+1, len(questionEntries), 0, 0, 1, "")
	}

	failedUnits := catalogFailures + questionResult.failures + answerResult.failures
	blocking := len(issues) + failedUnits
	status := domain.ImportStatusDraftReady
	if !directoryComplete || failedUnits > 0 || blocking > 0 || completeCount != len(questionEntries) {
		status = domain.ImportStatusNeedsAttention
	}
	expected := (*int)(nil)
	if directoryComplete {
		value := len(questionEntries)
		expected = &value
	}
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}
	units, _ := s.repo.ListAgentUnits(ctx, imp.ID, "")
	completedUnits := 0
	for _, unit := range units {
		if unit.Status == "completed" {
			completedUnits++
		}
	}
	if err := s.repo.UpdateVisionImportMetrics(ctx, imp.ID, status, status, directoryComplete, expected, len(questions), completeCount, len(units), completedUnits, failedUnits, blocking, llm.Model(), 1); err != nil {
		return err
	}
	s.persistVisionRunProgress(ctx, imp, "validating", 1, 1, failedUnits, 0, 1, "")
	if imp.SessionID != nil {
		if imp.RunID != nil {
			return s.reconcileImportSession(ctx, *imp.SessionID, *imp.RunID)
		}
		return s.reconcileImportSession(ctx, *imp.SessionID)
	}
	return nil
}

func (s *ImportService) runExtractionStageV11(ctx context.Context, imp *domain.ImportJob, doc *domain.Document, llm provider.LLMProvider, windows []visionPageWindow, entries map[int][]domain.VisionDirectoryEntry, role string, callOptions visionCallOptions, onCompleted func(context.Context, visionPageWindow, string) error) (map[int]string, int, error) {
	// recover_missing_questions is a question-only repair. The historical V11
	// coordinator launched both extraction goroutines unconditionally, which
	// made a missing-question retry spend time and model calls on unrelated
	// answer pages. Preserve the existing answer projection and let the linking
	// phase validate only the question result.
	if role == "answer" {
		if action, ok := s.repairActionForUnit(ctx, imp, "extract_question"); ok && action.Intent == "recover_missing_questions" && (action.TargetRole == "" || action.TargetRole == "question") {
			return map[int]string{}, 0, nil
		}
	}
	selected := []visionPageWindow{}
	for _, window := range windows {
		for _, entry := range entries[window.Index] {
			if (role == "question" && entry.ContentRole == "question") || (role == "answer" && (entry.ContentRole == "answer" || entry.ContentRole == "explanation")) {
				selected = append(selected, window)
				break
			}
		}
	}
	promptVersion := questionPromptVersion
	if role == "answer" {
		promptVersion = answerPromptVersion
	}
	return s.runVisionStageObserved(ctx, imp, doc, llm, selected, "extract_"+role, s.cfg.Worker.ExtractionConcurrency, func(ctx context.Context, unit *domain.ImportAgentUnit, window visionPageWindow) (string, error) {
		filtered := []domain.VisionDirectoryEntry{}
		for _, entry := range entries[window.Index] {
			if (role == "question" && entry.ContentRole == "question") || (role == "answer" && (entry.ContentRole == "answer" || entry.ContentRole == "explanation")) {
				filtered = append(filtered, entry)
			}
		}
		payload, _ := json.Marshal(filtered)
		prompt := questionExtractPrompt(window, string(payload))
		opts := callOptions
		opts.Schema = role
		opts.MaxTokens = 12000
		if role == "answer" {
			prompt = answerExtractPrompt(window, string(payload))
			opts.MaxTokens = 10000
		}
		return s.callVisionAgent(ctx, imp, llm, doc, unit, "extraction", promptVersion, prompt, opts)
	}, onCompleted)
}

func coordinateCatalogV11(outputs map[int]string, windows []visionPageWindow, failures int) (string, float64, map[string]domain.VisionDirectoryEntry, map[int][]domain.VisionDirectoryEntry, []map[string]interface{}, bool, map[int]bool) {
	roleCounts := map[string]int{}
	global := map[string]domain.VisionDirectoryEntry{}
	byWindow := map[int][]domain.VisionDirectoryEntry{}
	issues := []map[string]interface{}{}
	complete := failures == 0
	badWindows := map[int]bool{}
	rawEntries := []directoryWindowEntry{}
	entryOrder := 0
	for _, window := range windows {
		var payload catalogPayload
		if err := json.Unmarshal([]byte(outputs[window.Index]), &payload); err != nil {
			complete = false
			badWindows[window.Index] = true
			issues = append(issues, issue("CATALOG_JSON_INVALID", window.ContextStart, window.ContextEnd, err.Error()))
			continue
		}
		sections := map[string]catalogSection{}
		for _, section := range payload.Sections {
			sections[section.ID] = section
		}
		pageRoles := map[int]string{}
		for _, page := range payload.Pages {
			roleCounts[page.Role]++
			pageRoles[page.Page] = page.Role
		}
		for _, compact := range payload.Entries {
			section, ok := sections[compact.SectionID]
			if !ok {
				complete = false
				badWindows[window.Index] = true
				issues = append(issues, issue("CATALOG_SECTION_MISSING", compact.SourcePageStart, compact.SourcePageEnd, compact.SectionID))
				continue
			}
			questionType := compact.QuestionType
			if questionType == "" {
				questionType = section.QuestionType
			}
			entry := domain.VisionDirectoryEntry{ContentRole: compact.ContentRole, SectionPath: section.Path, SetNo: section.SetNo, QuestionType: questionType, CanonicalQuestionType: domain.CanonicalQuestionType(questionType), SectionScopeID: canonicalSectionScope(section.Path), OriginalQuestionLabel: compact.OriginalQuestionLabel, SequenceInScope: compact.SequenceInScope, SourceQuestionNo: compact.SourceQuestionNo, SourcePageStart: compact.SourcePageStart, SourcePageEnd: compact.SourcePageEnd, Anchor: compact.Anchor, MarkerTop: compact.MarkerTop, MarkerBottom: compact.MarkerBottom, Region: compact.Region}
			if entry.SequenceInScope == 0 {
				entry.SequenceInScope = entry.SourceQuestionNo
			}
			if entry.ContentRole == "question" && catalogPageRejectsQuestion(pageRoles[entry.SourcePageStart]) {
				complete = false
				badWindows[window.Index] = true
				issues = append(issues, issue("DIRECTORY_ROLE_MISMATCH", entry.SourcePageStart, entry.SourcePageEnd, fmt.Sprintf("题目目录项位于 %s 页面", pageRoles[entry.SourcePageStart])))
				continue
			}
			if err := validateDirectoryEntry(entry, window); err != nil {
				complete = false
				badWindows[window.Index] = true
				issues = append(issues, issue("SOURCE_ID_MISMATCH", window.ContextStart, window.ContextEnd, err.Error()))
				continue
			}
			rawEntries = append(rawEntries, directoryWindowEntry{windowIndex: window.Index, order: entryOrder, entry: entry})
			entryOrder++
		}
	}
	// Resolve type-only continuation headings before de-duplicating source
	// identities. This prevents identical q7/q8 numbers in adjacent topics from
	// being treated as overlap duplicates when the model omitted the topic title.
	for _, scoped := range normalizeDirectoryScopes(rawEntries) {
		entry := scoped.entry
		entry.SectionScopeID = canonicalSectionScope(entry.SectionPath)
		entry.SourceIdentity = visionSourceIdentity(entry)
		if previous, exists := global[entry.SourceIdentity]; exists {
			if previous.SourcePageStart != entry.SourcePageStart || previous.SourcePageEnd != entry.SourcePageEnd {
				complete = false
				badWindows[scoped.windowIndex] = true
				issues = append(issues, issue("DUPLICATE_SOURCE_IDENTITY", entry.SourcePageStart, entry.SourcePageEnd, entry.SourceIdentity))
			}
			continue
		}
		global[entry.SourceIdentity] = entry
		byWindow[scoped.windowIndex] = append(byWindow[scoped.windowIndex], entry)
	}
	role := "question_book"
	if roleCounts["question"] > 0 && (roleCounts["answer"] > 0 || roleCounts["explanation"] > 0) {
		role = "mixed"
	} else if roleCounts["question"] == 0 && roleCounts["explanation"] > 0 {
		role = "explanation_book"
	} else if roleCounts["question"] == 0 && roleCounts["answer"] > 0 {
		role = "answer_book"
	}
	total := 0
	maxCount := 0
	for _, count := range roleCounts {
		total += count
		if count > maxCount {
			maxCount = count
		}
	}
	confidence := .5
	if total > 0 {
		confidence = float64(maxCount) / float64(total)
	}
	return role, confidence, global, byWindow, issues, complete, badWindows
}

func splitVisionEntries(global map[string]domain.VisionDirectoryEntry) ([]domain.VisionDirectoryEntry, []domain.VisionDirectoryEntry) {
	questions := []domain.VisionDirectoryEntry{}
	answers := []domain.VisionDirectoryEntry{}
	for _, entry := range global {
		if entry.ContentRole == "question" {
			questions = append(questions, entry)
		} else {
			answers = append(answers, entry)
		}
	}
	sort.Slice(questions, func(i, j int) bool {
		if questions[i].SourcePageStart == questions[j].SourcePageStart {
			return questions[i].SourceQuestionNo < questions[j].SourceQuestionNo
		}
		return questions[i].SourcePageStart < questions[j].SourcePageStart
	})
	return questions, answers
}

func catalogPrompt(window visionPageWindow) string {
	return fmt.Sprintf("当前图片依次对应 PDF 物理页 %d-%d，本页窗只拥有 %d-%d 页，末尾重叠页仅补全跨页内容。一次完成页面角色画像和题目/答案/解析目录。用 sections 的短 id 复用章节路径；页面 role 只允许 cover/index/question/answer/explanation/mixed/blank；entries 的 content_role 只允许 question/answer/explanation，题号首次出现页必须在拥有范围。original_question_label 必须保留版面上的完整原文题号（如 16.、（一）、第3题），source_question_no 只放可计算数字，sequence_in_scope 是当前章节和题型作用域内的阅读顺序。对题目区域给出相对页面图像的 marker_top/marker_bottom 和 region(left,top,right,bottom)，不确定时填 0/省略。只返回 {\"sections\":[{\"id\":\"s1\",\"path\":[],\"set_no\":1,\"question_type\":\"single_choice\"}],\"pages\":[{\"page\":1,\"role\":\"question\",\"section_id\":\"s1\"}],\"entries\":[{\"id\":\"e1\",\"content_role\":\"question\",\"section_id\":\"s1\",\"original_question_label\":\"1.\",\"source_question_no\":1,\"sequence_in_scope\":1,\"source_page_start\":1,\"source_page_end\":1,\"anchor\":\"题干开头\",\"marker_top\":100,\"marker_bottom\":560,\"region\":{\"left\":0,\"top\":100,\"right\":1000,\"bottom\":560}}]}。所有 page 均为 PDF 物理页码。", window.ContextStart, window.ContextEnd, window.OwnedStart, window.OwnedEnd)
}

func (s *ImportService) reviewQuestionWindowV11(ctx context.Context, imp *domain.ImportJob, doc *domain.Document, llm provider.LLMProvider, window visionPageWindow, entries []domain.VisionDirectoryEntry, opts visionCallOptions) ([]domain.VisionQuestion, error) {
	if err := s.ensureVisionReviewAssets(ctx, doc, window.ContextStart, window.ContextEnd); err != nil {
		return nil, err
	}
	unit := newVisionUnit(imp.ID, "review_question", window, imp.PipelineVersion)
	documentID := doc.ID
	unit.DocumentID = &documentID
	if imp.SessionID != nil {
		unit.RunID = imp.RunID
		if unit.RunID == nil {
			if session, err := s.repo.GetImportSession(ctx, *imp.SessionID); err == nil {
				unit.RunID = session.ActiveRunID
			}
		}
	}
	unit.UnitKey += "-review"
	if err := s.repo.UpsertAgentUnit(ctx, unit); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(entries)
	opts.Schema = "question"
	opts.MaxTokens = 12000
	opts.UseReviewAssets = true
	raw, err := s.callVisionAgent(ctx, imp, llm, doc, unit, "review", questionPromptVersion+".review", questionExtractPrompt(window, string(payload)), opts)
	if err != nil {
		return nil, err
	}
	var value questionPayload
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value.Questions, nil
}

func (s *ImportService) reviewAnswerWindowV11(ctx context.Context, imp *domain.ImportJob, doc *domain.Document, llm provider.LLMProvider, window visionPageWindow, entries []domain.VisionDirectoryEntry, opts visionCallOptions) ([]domain.VisionAnswer, error) {
	if err := s.ensureVisionReviewAssets(ctx, doc, window.ContextStart, window.ContextEnd); err != nil {
		return nil, err
	}
	unit := newVisionUnit(imp.ID, "review_answer", window, imp.PipelineVersion)
	documentID := doc.ID
	unit.DocumentID = &documentID
	if imp.SessionID != nil {
		unit.RunID = imp.RunID
		if unit.RunID == nil {
			if session, err := s.repo.GetImportSession(ctx, *imp.SessionID); err == nil {
				unit.RunID = session.ActiveRunID
			}
		}
	}
	unit.UnitKey += "-review"
	if err := s.repo.UpsertAgentUnit(ctx, unit); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(entries)
	opts.Schema = "answer"
	opts.MaxTokens = 10000
	opts.UseReviewAssets = true
	raw, err := s.callVisionAgent(ctx, imp, llm, doc, unit, "review", answerPromptVersion+".review", answerExtractPrompt(window, string(payload)), opts)
	if err != nil {
		return nil, err
	}
	var value answerPayload
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value.Answers, nil
}

func isLoopbackProvider(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *ImportService) persistVisionRunProgress(ctx context.Context, imp *domain.ImportJob, stage string, completed, total, failed, eta, concurrency int, degraded string) {
	if imp.SessionID == nil || imp.RunID == nil {
		return
	}
	if strings.HasPrefix(stage, "extract_") {
		units, listErr := s.repo.ListAgentUnits(ctx, imp.ID, "")
		if listErr == nil {
			completed, total, failed = 0, 0, 0
			for _, unit := range units {
				if imp.RunID == nil || unit.RunID == nil || *unit.RunID != *imp.RunID {
					continue
				}
				if unit.UnitType != "extract_question" && unit.UnitType != "extract_answer" {
					continue
				}
				total++
				if unit.Status == "completed" {
					completed++
				}
				if unit.Status == "failed" {
					failed++
				}
			}
			stage = "parallel_extraction"
		}
	}
	if total <= 0 && (stage == "catalog" || stage == "review_question") {
		units, listErr := s.repo.ListAgentUnits(ctx, imp.ID, stage)
		if listErr == nil {
			total, completed, failed = 0, 0, 0
			for _, unit := range units {
				if imp.RunID == nil || unit.RunID == nil || *unit.RunID != *imp.RunID {
					continue
				}
				total++
				if unit.Status == "completed" {
					completed++
				}
				if unit.Status == "failed" {
					failed++
				}
			}
		}
	}
	if eta <= 0 && total > completed {
		unitType := stage
		if stage == "parallel_extraction" {
			unitType = "extract_question"
		}
		if median, medianErr := s.repo.RecentSuccessfulUnitMedianMS(ctx, imp.ID, unitType, 20); medianErr == nil && median > 0 {
			if concurrency < 1 {
				concurrency = 1
			}
			eta = int((median * int64(total-completed) / int64(concurrency)) / 1000)
		}
	}
	_ = s.repo.UpdateImportRunProgress(ctx, *imp.RunID, stage, completed, total, failed, eta, concurrency, degraded)
}
