package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

type questionCandidateProjection struct {
	id, documentID                                                           int64
	identity, sectionJSON, sectionScope, questionType, canonicalQuestionType string
	setNo, questionNo, pageStart, pageEnd                                    int
	status                                                                   string
}

// rekeyQuestionCandidatesToDirectory repairs candidate rows created before
// the directory became authoritative. Matching first uses the immutable
// identity, then a unique physical page/number/type tuple. This also fixes
// whitespace/alias changes such as multi_choice -> multiple_choice without
// merging two question blocks that share q1 on the same page.
func (s *ImportService) rekeyQuestionCandidatesToDirectory(ctx context.Context, sessionID int64) error {
	directories, err := s.loadQuestionDirectoryEntries(ctx, sessionID)
	if err != nil {
		return err
	}
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT id,document_id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),
		       COALESCE(section_scope_id,''),set_no,source_question_no,
		       COALESCE(question_type,''),COALESCE(canonical_question_type,''),
		       COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),status
		FROM candidate_questions
		WHERE import_session_id=? AND status NOT IN ('persisted','duplicate')
		ORDER BY id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	candidates := []questionCandidateProjection{}
	for rows.Next() {
		var candidate questionCandidateProjection
		if err := rows.Scan(&candidate.id, &candidate.documentID, &candidate.identity, &candidate.sectionJSON, &candidate.sectionScope, &candidate.setNo, &candidate.questionNo, &candidate.questionType, &candidate.canonicalQuestionType, &candidate.pageStart, &candidate.pageEnd, &candidate.status); err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	byIdentity := map[string][]domain.VisionDirectoryEntry{}
	byPhysical := map[string][]domain.VisionDirectoryEntry{}
	for documentID, entries := range directories {
		for _, entry := range entries {
			byIdentity[fmt.Sprintf("%d|%s", documentID, entry.SourceIdentity)] = append(byIdentity[fmt.Sprintf("%d|%s", documentID, entry.SourceIdentity)], entry)
			key := fmt.Sprintf("%d|%d|%d|%d|%s", documentID, entry.SourcePageStart, entry.SourcePageEnd, entry.SourceQuestionNo, domain.CanonicalQuestionType(entry.QuestionType))
			byPhysical[key] = append(byPhysical[key], entry)
		}
	}
	for _, candidate := range candidates {
		var path []string
		_ = json.Unmarshal([]byte(candidate.sectionJSON), &path)
		var matches []domain.VisionDirectoryEntry
		matches = append(matches, byIdentity[fmt.Sprintf("%d|%s", candidate.documentID, candidate.identity)]...)
		if len(matches) != 1 {
			matches = nil
			canonical := domain.CanonicalQuestionType(candidate.canonicalQuestionType)
			if canonical == "" {
				canonical = domain.CanonicalQuestionType(candidate.questionType)
			}
			key := fmt.Sprintf("%d|%d|%d|%d|%s", candidate.documentID, candidate.pageStart, candidate.pageEnd, candidate.questionNo, canonical)
			matches = append(matches, byPhysical[key]...)
			if len(matches) != 1 && candidate.pageStart > 0 && candidate.pageEnd >= candidate.pageStart {
				// Extraction retries can widen a cross-page span by one page. Use
				// overlap only when the number/type remains unique.
				for _, entry := range directories[candidate.documentID] {
					if entry.SourceQuestionNo != candidate.questionNo || domain.CanonicalQuestionType(entry.QuestionType) != canonical || !rangesOverlap([][]int{{entry.SourcePageStart, entry.SourcePageEnd}}, candidate.pageStart, candidate.pageEnd) {
						continue
					}
					matches = append(matches, entry)
				}
			}
		}
		if len(matches) != 1 {
			continue
		}
		entry := matches[0]
		sectionJSON, _ := json.Marshal(entry.SectionPath)
		canonical := domain.CanonicalQuestionType(entry.QuestionType)
		_, err := s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET
			source_identity=?,section_path_json=?,section_scope_id=?,set_no=?,
			question_type=?,canonical_question_type=?,question_page_start=?,question_page_end=?,
			page_start=?,page_end=?,updated_at=datetime('now') WHERE id=?`,
			entry.SourceIdentity, string(sectionJSON), entry.SectionScopeID, entry.SetNo,
			entry.QuestionType, canonical, entry.SourcePageStart, entry.SourcePageEnd,
			entry.SourcePageStart, entry.SourcePageEnd, candidate.id)
		if err != nil {
			return err
		}
	}
	return s.dedupeQuestionCandidatesByDirectory(ctx, sessionID)
}

func (s *ImportService) dedupeQuestionCandidatesByDirectory(ctx context.Context, sessionID int64) error {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT id,COALESCE(source_identity,''),COALESCE(question_page_start,page_start,0),
		       COALESCE(question_page_end,page_end,0),status,COALESCE(answer_link_status,'unresolved')
		FROM candidate_questions
		WHERE import_session_id=? AND status NOT IN ('persisted','duplicate')
		ORDER BY id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id, pageStart, pageEnd         int64
		identity, status, answerStatus string
	}
	groups := map[string][]row{}
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.identity, &item.pageStart, &item.pageEnd, &item.status, &item.answerStatus); err != nil {
			return err
		}
		if item.identity == "" {
			continue
		}
		key := fmt.Sprintf("%s|%d|%d", item.identity, item.pageStart, item.pageEnd)
		groups[key] = append(groups[key], item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, items := range groups {
		if len(items) < 2 {
			if items[0].status == "session_duplicate" {
				if _, err := s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET status='extracted',updated_at=datetime('now') WHERE id=?`, items[0].id); err != nil {
					return err
				}
			}
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			priority := func(item row) int {
				score := 0
				if item.status != "session_duplicate" {
					score += 4
				}
				if item.answerStatus == "confirmed" {
					score += 2
				}
				return score
			}
			left, right := priority(items[i]), priority(items[j])
			if left != right {
				return left > right
			}
			return items[i].id < items[j].id
		})
		primary := items[0]
		if primary.status == "session_duplicate" {
			if _, err := s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET status='extracted',updated_at=datetime('now') WHERE id=?`, primary.id); err != nil {
				return err
			}
		}
		for _, duplicate := range items[1:] {
			if _, err := s.repo.DB().ExecContext(ctx, `
				INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json)
				SELECT session_id,?,document_id,source_role,page_start,page_end,source_json
				FROM candidate_sources WHERE candidate_id=?`, primary.id, duplicate.id); err != nil {
				return err
			}
			if _, err := s.repo.DB().ExecContext(ctx, `
				INSERT OR IGNORE INTO candidate_answer_links(session_id,candidate_id,answer_record_id,status,method,confidence,detail_json,run_id)
				SELECT session_id,?,answer_record_id,status,method,confidence,detail_json,run_id
				FROM candidate_answer_links WHERE candidate_id=?`, primary.id, duplicate.id); err != nil {
				return err
			}
			if _, err := s.repo.DB().ExecContext(ctx, `DELETE FROM candidate_sources WHERE candidate_id=?`, duplicate.id); err != nil {
				return err
			}
			if _, err := s.repo.DB().ExecContext(ctx, `DELETE FROM candidate_answer_links WHERE candidate_id=?`, duplicate.id); err != nil {
				return err
			}
			if _, err := s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET status='session_duplicate',updated_at=datetime('now') WHERE id=?`, duplicate.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// recoverQuestionsFromCompletedUnits rebuilds the candidate projection from
// completed question extraction outputs after the directory has been repaired.
// This is intentionally deterministic: it never asks the model to guess a
// missing question and only accepts a unique directory identity.
func (s *ImportService) recoverQuestionsFromCompletedUnits(ctx context.Context, sessionID int64, runID int64) (int, error) {
	directories, err := s.loadQuestionDirectoryEntries(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if len(directories) == 0 {
		return 0, nil
	}

	seen := map[string]bool{}
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT j.id,j.document_id,COALESCE(u.output_json,'')
		FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id
		WHERE j.session_id=? AND u.unit_type='extract_question' AND u.status='completed'
		  AND trim(COALESCE(u.output_json,''))<>''
		ORDER BY u.id`, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	recovered := 0
	for rows.Next() {
		var jobID, documentID int64
		var raw string
		if err := rows.Scan(&jobID, &documentID, &raw); err != nil {
			return recovered, err
		}
		var payload questionPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}
		for _, question := range payload.Questions {
			entry, ok := findQuestionDirectoryEntryForRecovery(directories[documentID], question)
			seenKey := fmt.Sprintf("%d|%s", documentID, entry.SourceIdentity)
			if !ok || entry.SourceIdentity == "" || seen[seenKey] {
				continue
			}
			seen[seenKey] = true
			// The directory is authoritative for source position and type. The
			// extracted stem/options/visual confidence remain the model evidence.
			question.SourceIdentity = entry.SourceIdentity
			question.QuestionType = entry.QuestionType
			question.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
			question.SourcePageStart = entry.SourcePageStart
			question.SourcePageEnd = entry.SourcePageEnd
			candidateID, err := s.repo.InsertVisionCandidate(ctx, jobID, documentID, entry, question)
			if err != nil {
				return recovered, err
			}
			if candidateID > 0 {
				// InsertVisionCandidate is intentionally idempotent and may return a
				// row created by an older extractor. The directory identity is still
				// authoritative. Replace the candidate projection with this mapped
				// raw extraction as well: historical weak-hash/physical-key reuse can
				// leave a row whose scalar identity says q1 简答题 while its JSON body
				// is actually q1 辨析题. Keeping that body would make answer evidence
				// appear conflicting even after the identity is repaired.
				sectionJSON, _ := json.Marshal(entry.SectionPath)
				// Replaying the raw extraction body must not erase a confirmed
				// answer patch (for example from a candidate review) that the
				// replay does not carry. Keep the previous answer fields when
				// the replayed body has none, mirroring
				// updateExistingVisionCandidate.
				var previousJSON string
				var previous domain.VisionQuestion
				if err := s.repo.DB().QueryRowContext(ctx, `SELECT COALESCE(merged_json,question_json,extracted_json,'{}') FROM candidate_questions WHERE id=? AND import_session_id=?`, candidateID, sessionID).Scan(&previousJSON); err == nil {
					if json.Unmarshal([]byte(previousJSON), &previous) == nil {
						if len(question.Answer) == 0 {
							question.Answer = previous.Answer
						}
						if strings.TrimSpace(question.Analysis) == "" {
							question.Analysis = previous.Analysis
						}
					}
				}
				questionJSON, _ := json.Marshal(question)
				_, err = s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET
					source_identity=?,section_path_json=?,section_scope_id=?,set_no=?,
					question_type=?,canonical_question_type=?,question_page_start=?,question_page_end=?,
					page_start=?,page_end=?,question_json=?,extracted_json=?,merged_json=?,raw_text=?,normalized_text=?,
					confidence=?,visual_confidence=?,updated_at=datetime('now') WHERE id=? AND import_session_id=?`,
					entry.SourceIdentity, string(sectionJSON), entry.SectionScopeID, entry.SetNo,
					entry.QuestionType, domain.CanonicalQuestionType(entry.QuestionType), entry.SourcePageStart, entry.SourcePageEnd,
					entry.SourcePageStart, entry.SourcePageEnd, string(questionJSON), string(questionJSON), string(questionJSON), question.Stem, question.Stem,
					question.VisualConfidence, question.VisualConfidence, candidateID, sessionID)
				if err != nil {
					return recovered, err
				}
			}
			recovered++
		}
	}
	if err := rows.Err(); err != nil {
		return recovered, err
	}
	// A previous reconciliation may have marked the only row for an identity
	// as session_duplicate because its old section/type projection collided
	// with another question. Recovery has now restored the directory identity;
	// reactivate one row per identity before checking conservation. Genuine
	// repeated extraction rows are still merged by the same idempotent pass.
	if err := s.dedupeQuestionCandidatesByDirectory(ctx, sessionID); err != nil {
		return recovered, err
	}

	// A complete directory with no candidate body is an explicit conservation
	// failure. Keep it visible to the existing targeted-retry workflow instead
	// of silently reporting a smaller question count.
	missingRows, err := s.repo.DB().QueryContext(ctx, `
		SELECT d.document_id,d.source_identity,d.source_page_start,d.source_page_end,d.source_question_no
		FROM import_directory_entries d
		WHERE d.session_id=? AND d.status='active' AND d.content_role='question'
		  AND NOT EXISTS (
			SELECT 1 FROM candidate_questions c
			WHERE c.import_session_id=d.session_id AND c.document_id=d.document_id
			  AND c.status NOT IN ('session_duplicate','persisted','duplicate')
			  AND c.source_identity=d.source_identity
		  )`, sessionID)
	if err != nil {
		return recovered, err
	}
	defer missingRows.Close()
	for missingRows.Next() {
		var documentID int64
		var identity string
		var pageStart, pageEnd, questionNo int
		if err := missingRows.Scan(&documentID, &identity, &pageStart, &pageEnd, &questionNo); err != nil {
			return recovered, err
		}
		detail, _ := json.Marshal(map[string]interface{}{
			"source_identity":    identity,
			"source_page_start":  pageStart,
			"source_page_end":    pageEnd,
			"source_question_no": questionNo,
			"message":            "目录存在，但已完成的题目提取结果中没有对应题目",
		})
		_, _ = s.repo.DB().ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,source_identity,detail_json) VALUES(?,?,?,'QUESTION_MISSING','blocking',?,?)`, sessionID, nullableRun(runID), documentID, identity, string(detail))
	}
	return recovered, missingRows.Err()
}

func (s *ImportService) loadQuestionDirectoryEntries(ctx context.Context, sessionID int64) (map[int64][]domain.VisionDirectoryEntry, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT document_id,COALESCE(section_path_json,'[]'),set_no,
		       COALESCE(question_type,''),COALESCE(canonical_question_type,''),
		       COALESCE(section_scope_id,''),source_question_no,source_page_start,
		       source_page_end,COALESCE(anchor,''),COALESCE(source_identity,'')
		FROM import_directory_entries
		WHERE session_id=? AND status='active' AND content_role='question'
		ORDER BY document_id,source_page_start,source_page_end,source_question_no,id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDocument := map[int64][]domain.VisionDirectoryEntry{}
	for rows.Next() {
		var documentID int64
		var sectionJSON string
		var entry domain.VisionDirectoryEntry
		if err := rows.Scan(&documentID, &sectionJSON, &entry.SetNo, &entry.QuestionType, &entry.CanonicalQuestionType, &entry.SectionScopeID, &entry.SourceQuestionNo, &entry.SourcePageStart, &entry.SourcePageEnd, &entry.Anchor, &entry.SourceIdentity); err != nil {
			return nil, err
		}
		entry.ContentRole = "question"
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
		if entry.SourceIdentity == "" {
			entry.SourceIdentity = visionSourceIdentity(entry)
		}
		byDocument[documentID] = append(byDocument[documentID], entry)
	}
	return byDocument, rows.Err()
}

func findQuestionDirectoryEntryForRecovery(entries []domain.VisionDirectoryEntry, question domain.VisionQuestion) (domain.VisionDirectoryEntry, bool) {
	global := map[string]domain.VisionDirectoryEntry{}
	for _, entry := range entries {
		if entry.ContentRole == "question" {
			global[entry.SourceIdentity] = entry
		}
	}
	if entry, ok := findVisionDirectoryEntry(global, question); ok {
		return entry, true
	}

	qno := 0
	parsedPath, _, _, _, parsedNo, parsedOK := parseVisionSourceIdentity(question.SourceIdentity)
	if parsedOK {
		qno = parsedNo
	}
	if qno <= 0 || question.SourcePageStart <= 0 || question.SourcePageEnd < question.SourcePageStart {
		return domain.VisionDirectoryEntry{}, false
	}
	canonical := domain.CanonicalQuestionType(question.QuestionType)
	if canonical == "" {
		canonical = domain.CanonicalQuestionType(question.CanonicalQuestionType)
	}
	physical := make([]domain.VisionDirectoryEntry, 0, 2)
	typed := make([]domain.VisionDirectoryEntry, 0, 2)
	pathExact := make([]domain.VisionDirectoryEntry, 0, 2)
	for _, entry := range entries {
		if entry.ContentRole != "question" || entry.SourceQuestionNo != qno || !rangesOverlap([][]int{{entry.SourcePageStart, entry.SourcePageEnd}}, question.SourcePageStart, question.SourcePageEnd) {
			continue
		}
		physical = append(physical, entry)
		entryCanonical := domain.CanonicalQuestionType(entry.QuestionType)
		if canonical != "" && entryCanonical == canonical {
			typed = append(typed, entry)
		}
		if sectionPathEqual(parsedPath, entry.SectionPath) &&
			(canonical == "" || entryCanonical == canonical) {
			pathExact = append(pathExact, entry)
		}
	}
	// The path in the model identity is more discriminating than a set number
	// or a type alias. This is required for pages that intentionally contain
	// q1 in both 简答题 and 论述题 blocks, and for model outputs that invent a
	// non-zero set number for an otherwise unnumbered block.
	if len(pathExact) == 1 {
		return pathExact[0], true
	}
	if len(pathExact) > 1 {
		if entry, ok := chooseQuestionDirectoryByAnchor(pathExact, question); ok {
			return entry, true
		}
	}
	if len(typed) == 1 {
		return typed[0], true
	}
	if len(physical) > 1 {
		if entry, ok := chooseQuestionDirectoryByAnchor(physical, question); ok {
			return entry, true
		}
	}
	// A type mismatch is accepted only when the physical page/number is
	// unique. This repairs q35–38 on page 73, where the extractor repeated the
	// stale single-choice type from the bad directory, without allowing q1 on a
	// page containing multiple question blocks to cross-link.
	if len(physical) == 1 {
		return physical[0], true
	}
	return domain.VisionDirectoryEntry{}, false
}

func sectionPathEqual(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		if compactText(left[index]) != compactText(right[index]) {
			return false
		}
	}
	return true
}

// chooseQuestionDirectoryByAnchor is only used after page, number and (when
// available) type have already narrowed the candidates. It accepts a match
// only when the extracted stem contains the directory anchor, or when one
// candidate has a clear prefix advantage. This handles a catalog row whose
// local type heading was omitted without allowing a same-number question in a
// neighbouring block to be guessed.
func chooseQuestionDirectoryByAnchor(entries []domain.VisionDirectoryEntry, question domain.VisionQuestion) (domain.VisionDirectoryEntry, bool) {
	if len(entries) == 0 {
		return domain.VisionDirectoryEntry{}, false
	}
	text := compactText(strings.TrimSpace(question.Stem + " " + question.Passage))
	if text == "" {
		return domain.VisionDirectoryEntry{}, false
	}
	type scoredEntry struct {
		entry domain.VisionDirectoryEntry
		score float64
	}
	scored := make([]scoredEntry, 0, len(entries))
	for _, entry := range entries {
		anchor := compactText(strings.TrimSpace(entry.Anchor))
		if anchor == "" {
			continue
		}
		score := 0.0
		if strings.Contains(text, anchor) || strings.Contains(anchor, text) {
			score = 1
		} else {
			limit := len([]rune(anchor))
			if textLength := len([]rune(text)); textLength < limit {
				limit = textLength
			}
			prefix := 0
			anchorRunes, textRunes := []rune(anchor), []rune(text)
			for prefix < limit && anchorRunes[prefix] == textRunes[prefix] {
				prefix++
			}
			if limit > 0 {
				score = float64(prefix) / float64(limit)
			}
		}
		scored = append(scored, scoredEntry{entry: entry, score: score})
	}
	if len(scored) == 0 {
		return domain.VisionDirectoryEntry{}, false
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if scored[0].score < 0.75 || (len(scored) > 1 && scored[0].score <= scored[1].score+0.1) {
		return domain.VisionDirectoryEntry{}, false
	}
	return scored[0].entry, true
}
