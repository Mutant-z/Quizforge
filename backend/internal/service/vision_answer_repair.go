package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
)

// targetedAnswerRepairAction returns the persisted action for a conversational
// answer relink run.  It is deliberately strict: only an active repair whose
// intent is relink_answers may bypass the normal import pipeline.
func (s *ImportService) targetedAnswerRepairAction(ctx context.Context, sessionID int64) (*domain.ImportRun, *domain.ConversationAction, bool, error) {
	session, err := s.repo.GetImportSession(ctx, sessionID)
	if err != nil {
		return nil, nil, false, err
	}
	if session.ActiveRunID == nil {
		return nil, nil, false, nil
	}
	run, err := s.repo.GetImportRun(ctx, *session.ActiveRunID)
	if err != nil {
		return nil, nil, false, err
	}
	if run.RunType != "repair" || run.Status != "running" {
		return run, nil, false, nil
	}
	var action domain.ConversationAction
	if json.Unmarshal([]byte(run.ActionJSON), &action) != nil || action.Intent != "relink_answers" || len(action.PageRanges) == 0 {
		return run, nil, false, nil
	}
	return run, &action, true, nil
}

func (s *ImportService) isTargetedAnswerRepair(ctx context.Context, sessionID int64) (bool, error) {
	_, _, targeted, err := s.targetedAnswerRepairAction(ctx, sessionID)
	return targeted, err
}

// runTargetedAnswerRepair is the recovery path for "relink answers" actions.
// It consumes the durable directory projection and invokes vision only for
// the requested answer windows.  In particular, it must not recreate profile,
// directory, or question units: doing so was the source of long apparent
// hangs when a retry was intended to touch only answer pages.
func (s *ImportService) runTargetedAnswerRepair(ctx context.Context, imp *domain.ImportJob, doc *domain.Document) error {
	ctx = observability.WithUserID(ctx, imp.UserID)
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}
	run, action, targeted, err := s.targetedAnswerRepairAction(ctx, *imp.SessionID)
	if err != nil {
		return err
	}
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}
	if !targeted || run == nil || action == nil {
		return fmt.Errorf("TARGETED_REPAIR_NOT_ACTIVE")
	}

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

	pageCount := doc.PageCount
	if pageCount <= 0 {
		pages, parseErr := parserParseDocument(ctx, doc.FilePath, doc.MimeType)
		if parseErr != nil {
			return parseErr
		}
		pageCount = len(pages)
	}
	if pageCount <= 0 {
		return fmt.Errorf("PDF has no pages")
	}
	windows := buildVisionWindows(pageCount)
	selected := make([]visionPageWindow, 0, len(windows))
	for _, window := range windows {
		if rangesOverlap(action.PageRanges, window.ContextStart, window.ContextEnd) {
			selected = append(selected, window)
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("NO_TARGET_UNITS")
	}

	entriesByWindow, err := s.loadAnswerDirectoryEntries(ctx, *imp.SessionID, doc.ID, selected)
	if err != nil {
		return err
	}
	s.updateStage(ctx, imp.ID, domain.ImportStatusExtractingAnswers, "答案执行 Agent 正在定向重试指定答案页", 0.70)
	s.emitVisionStage(ctx, imp, doc.ID, "extracting_answers", "AnswerExtractionAgent", fmt.Sprintf("只重试答案页 %d-%d，共 %d 个页窗", action.PageRanges[0][0], action.PageRanges[0][1], len(selected)), 0, len(selected))

	callOptions := visionCallOptions{}
	if imp.PipelineVersion == domain.VisionPipelineVersionV11 {
		callOptions = visionCallOptions{
			ProviderKey: fmt.Sprintf("%d|%s|%s", providerConfig.ID, providerConfig.BaseURL, providerConfig.ChatModel),
			Local:       isLoopbackProvider(providerConfig.BaseURL),
			Schema:      "answer",
			MaxTokens:   10000,
		}
	}
	outputs, _, err := s.runVisionStageObserved(ctx, imp, doc, llm, selected, "extract_answer", s.cfg.Worker.ExtractionConcurrency, func(callCtx context.Context, unit *domain.ImportAgentUnit, window visionPageWindow) (string, error) {
		filtered := entriesByWindow[window.Index]
		payload, _ := json.Marshal(filtered)
		prompt := answerExtractPrompt(window, string(payload))
		if imp.PipelineVersion == domain.VisionPipelineVersionV11 {
			return s.callVisionAgent(callCtx, imp, llm, doc, unit, "extraction", answerPromptVersion, prompt, callOptions)
		}
		return s.callVisionAgent(callCtx, imp, llm, doc, unit, "extraction", answerPromptVersion, prompt, nil)
	}, nil)
	if err != nil {
		return err
	}

	answers := make([]domain.VisionAnswer, 0)
	answerDirectory := make([]domain.VisionDirectoryEntry, 0)
	seenDirectory := map[string]bool{}
	for _, entries := range entriesByWindow {
		for _, entry := range entries {
			key := entry.SourceIdentity
			if key == "" {
				key = fmt.Sprintf("%s|%d|%s|%d|%d", entry.SectionScopeID, entry.SetNo, entry.QuestionType, entry.SourceQuestionNo, entry.SourcePageStart)
			}
			if seenDirectory[key] {
				continue
			}
			seenDirectory[key] = true
			answerDirectory = append(answerDirectory, entry)
		}
	}
	for _, window := range selected {
		raw := outputs[window.Index]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var payload answerPayload
		if json.Unmarshal([]byte(raw), &payload) != nil {
			// callVisionAgent validates the unit before returning; retain this
			// guard for older providers and leave the unit auditable as failed.
			continue
		}
		for _, answer := range payload.Answers {
			if answer.SourcePageStart < window.ContextStart || answer.SourcePageEnd > window.ContextEnd || answer.SourcePageEnd < answer.SourcePageStart {
				continue
			}
			normalized, _, ok := normalizeVisionAnswerIdentity(answer, answerDirectory)
			if !ok {
				// Keep the raw unit output for audit, but do not persist an
				// unproven answer row that could later be matched across topics.
				continue
			}
			answers = append(answers, normalized)
		}
	}
	if len(answers) > 0 {
		if err := s.repo.UpsertAnswerRecords(ctx, *imp.SessionID, doc.ID, run.ID, answers); err != nil {
			return err
		}
	}
	if err := s.ensureImportSessionActive(ctx, imp); err != nil {
		return err
	}

	// Rebuild links from all durable answer records, not only the newly
	// returned rows. This preserves previously confirmed answers and makes the
	// result deterministic after a partial retry.
	if err := s.RelinkSessionAnswers(ctx, *imp.SessionID, run.ID); err != nil {
		return err
	}
	updated, err := s.repo.GetImportSession(ctx, *imp.SessionID)
	if err != nil {
		return err
	}
	units, err := s.repo.ListAgentUnits(ctx, imp.ID, "")
	if err != nil {
		return err
	}
	totalUnits, completedUnits, failedUnits := len(units), 0, 0
	directoryComplete := true
	for _, unit := range units {
		if unit.Status == "completed" {
			completedUnits++
		}
		if unit.Status == "failed" {
			failedUnits++
		}
		if (unit.UnitType == "profile" || unit.UnitType == "directory") && unit.Status != "completed" {
			directoryComplete = false
		}
	}
	status := domain.ImportStatusNeedsAttention
	if directoryComplete && failedUnits == 0 && updated.BlockingIssueCount == 0 && updated.ExpectedCount != nil && *updated.ExpectedCount == updated.UniqueCount && updated.UniqueCount == updated.CompleteCount {
		status = domain.ImportStatusDraftReady
	}
	if err := s.repo.UpdateVisionImportMetrics(ctx, imp.ID, status, domain.ImportStatusValidating, directoryComplete, updated.ExpectedCount, updated.UniqueCount, updated.CompleteCount, totalUnits, completedUnits, failedUnits, updated.BlockingIssueCount, llm.Model(), 0.95); err != nil {
		return err
	}

	// Mark the document job terminal before reconciliation. The coordinator
	// then atomically claims/finishes this repair run and recalculates session
	// counts and blocking problems without starting another extraction pass.
	if imp.RunID != nil {
		return s.reconcileImportSession(ctx, *imp.SessionID, *imp.RunID)
	}
	return s.reconcileImportSession(ctx, *imp.SessionID)
}

func (s *ImportService) loadAnswerDirectoryEntries(ctx context.Context, sessionID, documentID int64, windows []visionPageWindow) (map[int][]domain.VisionDirectoryEntry, error) {
	entriesByWindow := make(map[int][]domain.VisionDirectoryEntry, len(windows))
	for _, window := range windows {
		entriesByWindow[window.Index] = []domain.VisionDirectoryEntry{}
	}
	rows, err := s.repo.DB().QueryContext(ctx, `SELECT content_role,COALESCE(section_path_json,'[]'),set_no,COALESCE(question_type,''),COALESCE(canonical_question_type,''),COALESCE(section_scope_id,''),COALESCE(sequence_in_scope,0),source_question_no,source_page_start,source_page_end,COALESCE(anchor,''),COALESCE(marker_top,0),COALESCE(source_identity,'') FROM import_directory_entries WHERE session_id=? AND document_id=? AND status='active' AND content_role IN ('answer','explanation') ORDER BY source_page_start,source_question_no,id`, sessionID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry domain.VisionDirectoryEntry
		var sectionJSON string
		if err := rows.Scan(&entry.ContentRole, &sectionJSON, &entry.SetNo, &entry.QuestionType, &entry.CanonicalQuestionType, &entry.SectionScopeID, &entry.SequenceInScope, &entry.SourceQuestionNo, &entry.SourcePageStart, &entry.SourcePageEnd, &entry.Anchor, &entry.MarkerTop, &entry.SourceIdentity); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(sectionJSON), &entry.SectionPath)
		entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.CanonicalQuestionType)
		if entry.CanonicalQuestionType == "" {
			entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
		}
		if entry.QuestionType == "" {
			entry.QuestionType = entry.CanonicalQuestionType
		}
		if entry.SectionScopeID == "" {
			entry.SectionScopeID = canonicalSectionScope(entry.SectionPath)
		}
		for _, window := range windows {
			if !rangesOverlap([][]int{{entry.SourcePageStart, entry.SourcePageEnd}}, window.ContextStart, window.ContextEnd) {
				continue
			}
			duplicate := false
			for _, existing := range entriesByWindow[window.Index] {
				if existing.SourceIdentity != "" && existing.SourceIdentity == entry.SourceIdentity {
					duplicate = true
					break
				}
			}
			if !duplicate {
				entriesByWindow[window.Index] = append(entriesByWindow[window.Index], entry)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, entries := range entriesByWindow {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].SourcePageStart != entries[j].SourcePageStart {
				return entries[i].SourcePageStart < entries[j].SourcePageStart
			}
			return entries[i].SourceQuestionNo < entries[j].SourceQuestionNo
		})
	}
	return entriesByWindow, nil
}
