package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// rebuildSessionDirectoriesFromUnits reconstructs the durable directory
// projection from completed Agent outputs. It is used by relink runs so a
// deterministic repair can recover entries that were previously discarded by
// a raw section-path collision, without spending another vision call.
func (s *ImportService) rebuildSessionDirectoriesFromUnits(ctx context.Context, sessionID int64) error {
	query := `SELECT sd.document_id,sd.document_role,j.id,u.id,u.unit_type,u.unit_key,u.owned_page_start,u.owned_page_end,u.context_page_start,u.context_page_end,u.status,COALESCE(u.output_json,'')
		FROM import_session_documents sd JOIN import_jobs j ON j.id=sd.import_job_id
		JOIN import_agent_units u ON u.import_job_id=j.id AND u.unit_type IN ('directory','catalog')
		WHERE sd.session_id=? ORDER BY sd.document_id,u.context_page_start,u.id`
	rows, err := s.repo.DB().QueryContext(ctx, query, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type jobState struct {
		documentID int64
		role       string
		total      int
		completed  int
		entries    []directoryWindowEntry
	}
	jobs := map[int64]*jobState{}
	// Existing question candidates are the authoritative, already validated
	// question directory. Raw directory units can contain overlap-window
	// hallucinations (for example, repeating q1–q10 on the next page), so they
	// must not inflate the session's expected question count during a repair.
	candidateEntries := map[int64][]domain.VisionDirectoryEntry{}
	coverageEntries := map[int64][]domain.VisionDirectoryEntry{}
	coverageRows, coverageErr := s.repo.DB().QueryContext(ctx, `SELECT de.document_id,de.content_role,de.section_path_json,de.set_no,de.question_type,de.canonical_question_type,de.section_scope_id,de.sequence_in_scope,COALESCE(de.original_question_label,''),de.source_question_no,de.source_page_start,de.source_page_end,COALESCE(de.anchor,''),COALESCE(de.marker_top,0),COALESCE(de.marker_bottom,0),de.source_identity FROM import_directory_entries de JOIN import_page_coverage pc ON pc.session_id=de.session_id AND pc.document_id=de.document_id AND pc.page_no=de.source_page_start WHERE de.session_id=? AND pc.status IN ('mismatch','repaired') AND EXISTS(SELECT 1 FROM json_each(pc.missing_json) WHERE CAST(value AS INTEGER)=de.source_question_no)`, sessionID)
	if coverageErr == nil {
		for coverageRows.Next() {
			var documentID int64
			var entry domain.VisionDirectoryEntry
			var sectionJSON string
			if coverageRows.Scan(&documentID, &entry.ContentRole, &sectionJSON, &entry.SetNo, &entry.QuestionType, &entry.CanonicalQuestionType, &entry.SectionScopeID, &entry.SequenceInScope, &entry.OriginalQuestionLabel, &entry.SourceQuestionNo, &entry.SourcePageStart, &entry.SourcePageEnd, &entry.Anchor, &entry.MarkerTop, &entry.MarkerBottom, &entry.SourceIdentity) == nil {
				_ = json.Unmarshal([]byte(sectionJSON), &entry.SectionPath)
				coverageEntries[documentID] = append(coverageEntries[documentID], entry)
			}
		}
		coverageRows.Close()
	}
	verifiedHistory := map[string]bool{}
	historyRows, historyErr := s.repo.DB().QueryContext(ctx, `SELECT c.document_id,COALESCE(c.source_identity,'') FROM source_question_origins o JOIN candidate_questions c ON c.id=o.candidate_id WHERE o.session_id=? AND o.identity_confidence='verified_history' AND o.status='active'`, sessionID)
	if historyErr == nil {
		for historyRows.Next() {
			var documentID int64
			var identity string
			if historyRows.Scan(&documentID, &identity) == nil {
				verifiedHistory[fmt.Sprintf("%d|%s", documentID, identity)] = true
			}
		}
		historyRows.Close()
	}
	candidateRows, candidateErr := s.repo.DB().QueryContext(ctx, `SELECT document_id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),COALESCE(section_scope_id,''),set_no,source_question_no,COALESCE(question_type,''),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate') ORDER BY document_id,question_page_start,id`, sessionID)
	if candidateErr == nil {
		for candidateRows.Next() {
			var documentID int64
			var identity, sectionJSON, sectionScope, questionType string
			var setNo, questionNo, pageStart, pageEnd int
			if candidateRows.Scan(&documentID, &identity, &sectionJSON, &sectionScope, &setNo, &questionNo, &questionType, &pageStart, &pageEnd) != nil {
				continue
			}
			var sectionPath []string
			if json.Unmarshal([]byte(sectionJSON), &sectionPath) != nil {
				sectionPath = nil
			}
			// source_identity is the immutable path captured when the question
			// was first extracted. Prefer it over a previously repaired (or
			// accidentally flattened) section_path_json so a relink can be
			// repeated without losing the case/short-answer discriminator.
			if identityPath := sectionPathFromSourceIdentity(identity); len(identityPath) > 0 {
				// A source identity containing only a type heading (for example
				// ["六、案例分析题"]) is not a topic identity. Treat it as
				// missing so the raw directory stream can restore the physical
				// topic context; retaining a scope inferred by an earlier relink
				// would duplicate the terminal topic and break objective answer
				// matching in neighbouring sections.
				if canonicalSectionScope(identityPath) == "" {
					sectionPath = nil
					sectionScope = ""
				} else {
					sectionPath = identityPath
				}
			} else {
				// An empty identity means the original model did not provide a
				// reliable heading. Do not trust a prior relink's synthetic
				// "scope/type" path; the normalizer will reconstruct it from
				// physical-page context below.
				sectionPath = nil
			}
			// An empty source identity means the model omitted the heading. Do not
			// trust a synthetic scope persisted by an earlier repair; the raw
			// directory stream (including marker_top) will restore it by physical
			// order below. Explicit identities remain authoritative.
			if len(sectionPath) == 0 {
				sectionScope = ""
			} else if strings.TrimSpace(sectionScope) == "" {
				sectionScope = canonicalSectionScope(sectionPath)
			}
			candidateEntries[documentID] = append(candidateEntries[documentID], domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: sectionPath, SetNo: setNo, QuestionType: questionType, CanonicalQuestionType: domain.CanonicalQuestionType(questionType), SectionScopeID: sectionScope, SourceQuestionNo: questionNo, SourcePageStart: pageStart, SourcePageEnd: pageEnd, SourceIdentity: identity, SequenceInScope: questionNo})
		}
		candidateRows.Close()
	}
	order := 0
	for rows.Next() {
		var documentID, jobID, unitID int64
		var role, unitType, unitKey, status, raw string
		var ownedStart, ownedEnd, contextStart, contextEnd int
		if err := rows.Scan(&documentID, &role, &jobID, &unitID, &unitType, &unitKey, &ownedStart, &ownedEnd, &contextStart, &contextEnd, &status, &raw); err != nil {
			return err
		}
		state := jobs[jobID]
		if state == nil {
			state = &jobState{documentID: documentID, role: role}
			jobs[jobID] = state
		}
		state.total++
		if status != "completed" || strings.TrimSpace(raw) == "" {
			continue
		}
		state.completed++
		start, end, ok := parseDirectoryUnitKey(unitKey)
		if !ok {
			start, end = contextStart, contextEnd
		}
		window := visionPageWindow{Index: order, OwnedStart: ownedStart, OwnedEnd: ownedEnd, ContextStart: start, ContextEnd: end}
		entries := []domain.VisionDirectoryEntry{}
		if unitType == "catalog" {
			var payload catalogPayload
			if json.Unmarshal([]byte(raw), &payload) != nil {
				continue
			}
			sections := map[string]catalogSection{}
			for _, section := range payload.Sections {
				sections[section.ID] = section
			}
			pageRoles := map[int]string{}
			for _, page := range payload.Pages {
				pageRoles[page.Page] = page.Role
			}
			for _, compact := range payload.Entries {
				section, exists := sections[compact.SectionID]
				if !exists {
					continue
				}
				entry := domain.VisionDirectoryEntry{ContentRole: compact.ContentRole, SectionPath: section.Path, SetNo: section.SetNo, QuestionType: section.QuestionType, SourceQuestionNo: compact.SourceQuestionNo, SourcePageStart: compact.SourcePageStart, SourcePageEnd: compact.SourcePageEnd, Anchor: compact.Anchor, MarkerTop: compact.MarkerTop}
				if entry.ContentRole == "question" && catalogPageRejectsQuestion(pageRoles[entry.SourcePageStart]) {
					continue
				}
				entries = append(entries, entry)
			}
		} else {
			var payload directoryPayload
			if json.Unmarshal([]byte(raw), &payload) != nil {
				continue
			}
			entries = payload.Entries
		}
		for _, entry := range entries {
			if validateDirectoryEntry(entry, window) != nil {
				continue
			}
			entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
			state.entries = append(state.entries, directoryWindowEntry{windowIndex: order, order: order, entry: entry})
			order++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, state := range jobs {
		// Never replace a partially completed directory. Keeping the previous
		// projection lets the next retry resume failed pages safely.
		if state.total == 0 || state.completed != state.total || len(state.entries) == 0 {
			continue
		}
		// A complete catalog is authoritative for the question directory. The
		// old repair path made existing candidates authoritative and used raw
		// directory rows only for narrow gap/tail promotion, which permanently
		// lost every question whose extraction identity had collided earlier.
		// Normalize the raw stream on its own, then synchronize existing candidate
		// metadata by physical source. Missing candidate bodies are recovered from
		// the completed extract_question units immediately after this projection.
		rawAuthoritative := state.completed == state.total && len(state.entries) > 0
		var normalized []directoryWindowEntry
		if rawAuthoritative {
			normalized = normalizeDirectoryScopes(state.entries)
			physical := map[string]bool{}
			for _, item := range normalized {
				if item.entry.ContentRole == "question" {
					physical[directoryPhysicalKey(item.entry)] = true
				}
			}
			for _, candidate := range candidateEntries[state.documentID] {
				if !verifiedHistory[fmt.Sprintf("%d|%s", state.documentID, candidate.SourceIdentity)] || physical[directoryPhysicalKey(candidate)] {
					continue
				}
				normalized = append(normalized, directoryWindowEntry{windowIndex: len(normalized), order: len(normalized), entry: candidate})
				physical[directoryPhysicalKey(candidate)] = true
			}
			for _, recovered := range coverageEntries[state.documentID] {
				key := recovered.ContentRole + "|" + directoryPhysicalKey(recovered)
				found := false
				for _, item := range normalized {
					if item.entry.ContentRole+"|"+directoryPhysicalKey(item.entry) == key {
						found = true
						break
					}
				}
				if !found {
					normalized = append(normalized, directoryWindowEntry{windowIndex: len(normalized), order: len(normalized), entry: recovered})
				}
			}
			normalized = normalizeDirectoryScopes(normalized)
			for _, item := range normalized {
				if item.entry.ContentRole == "question" && item.entry.SourceQuestionNo > 0 {
					if err := s.syncCandidateDirectoryMetadata(ctx, sessionID, state.documentID, item.entry); err != nil {
						return err
					}
				}
			}
		} else {
			// For a partial directory keep the conservative legacy behavior: raw
			// entries can repair a proven candidate gap, but cannot inflate the
			// expected count by themselves.
			allEntries := make([]directoryWindowEntry, 0, len(candidateEntries[state.documentID])+len(state.entries))
			for index, entry := range candidateEntries[state.documentID] {
				allEntries = append(allEntries, directoryWindowEntry{windowIndex: index, order: index, entry: entry})
			}
			candidateCount := len(candidateEntries[state.documentID])
			for _, rawItem := range state.entries {
				rawItem.windowIndex += candidateCount
				allEntries = append(allEntries, rawItem)
			}
			normalized = normalizeDirectoryScopes(allEntries)
		}
		candidatePhysical := map[string]bool{}
		candidateNumbers := map[string]map[int]bool{}
		for _, candidate := range candidateEntries[state.documentID] {
			candidatePhysical[directoryPhysicalKey(candidate)] = true
		}
		for _, item := range normalized {
			if item.entry.ContentRole != "question" {
				continue
			}
			entry := item.entry
			group := directoryQuestionGroup(entry)
			if candidateNumbers[group] == nil {
				candidateNumbers[group] = map[int]bool{}
			}
			candidateNumbers[group][entry.SourceQuestionNo] = true
		}
		promotableTail := map[string]domain.VisionDirectoryEntry{}
		if !rawAuthoritative {
			promotableTail = directoryEntriesExtendingCandidateTail(normalized, len(candidateEntries[state.documentID]), candidateEntries[state.documentID], candidatePhysical, candidateNumbers)
		}
		seen := map[string]domain.VisionDirectoryEntry{}
		entries := make([]domain.VisionDirectoryEntry, 0, len(normalized))
		for _, item := range normalized {
			entry := item.entry
			if !rawAuthoritative && entry.ContentRole == "question" && len(candidateEntries[state.documentID]) > 0 && item.windowIndex < len(candidateEntries[state.documentID]) {
				// Candidate rows are authoritative for question entries. Raw
				// overlap windows are still present in `normalized` only to supply
				// context; their empty section path must never overwrite the
				// repaired candidate path after it has been inferred.
				// Persist the repaired path on the candidate itself. This makes
				// SectionSubtype available to AnswerLinkAgent and also exposes the
				// recovered type in the draft UI after a relink.
				sectionJSON, _ := json.Marshal(entry.SectionPath)
				_, _ = s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET section_path_json=?,section_scope_id=?,canonical_question_type=? WHERE import_session_id=? AND document_id=? AND source_question_no=? AND set_no=? AND question_page_start=? AND question_page_end=? AND (question_type=? OR canonical_question_type=?)`, string(sectionJSON), canonicalSectionScope(entry.SectionPath), domain.CanonicalQuestionType(entry.QuestionType), sessionID, state.documentID, entry.SourceQuestionNo, entry.SetNo, entry.SourcePageStart, entry.SourcePageEnd, entry.QuestionType, domain.CanonicalQuestionType(entry.QuestionType))
				entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
				entry.SectionScopeID = canonicalSectionScope(entry.SectionPath)
				entry.SequenceInScope = entry.SourceQuestionNo
				dedupeKey := directoryDedupeKey(entry)
				if _, exists := seen[dedupeKey]; exists {
					continue
				}
				seen[dedupeKey] = entry
				entries = append(entries, entry)
				continue
			}
			if !rawAuthoritative && entry.ContentRole == "question" && len(candidateEntries[state.documentID]) > 0 && item.windowIndex >= len(candidateEntries[state.documentID]) && strings.TrimSpace(entry.SourceIdentity) == "" {
				// Raw rows with no immutable identity normally only provide
				// continuation context. There is one important exception: when a
				// directory row has no corresponding candidate, it is evidence of a
				// real extraction omission (the model returned the directory item,
				// but the question extractor did not). Promote that row so the next
				// targeted retry receives an explicit expected identity. Physical
				// overlap rows that already have a candidate remain context-only.
				if entry.ContentRole != "question" || candidatePhysical[directoryPhysicalKey(entry)] || (!directoryEntryFillsCandidateGap(entry, candidateNumbers) && promotableTail[directoryPhysicalKey(entry)].ContentRole == "") {
					continue
				}
				// The raw continuation row may have inherited a neighbouring topic
				// from a retry window. When the tail detector found a unique
				// preceding candidate block, restore that durable context before
				// creating the immutable identity.
				if context, ok := promotableTail[directoryPhysicalKey(entry)]; ok {
					// This row was admitted only because it extends that unique
					// candidate block. Always copy its topic path; retaining a stale
					// inherited path would create a false cross-topic identity.
					entry.SectionPath = append([]string(nil), context.SectionPath...)
					if len(entry.SectionPath) == 0 && context.SectionScopeID != "" {
						entry.SectionPath = []string{context.SectionScopeID}
					}
					entry.SectionScopeID = context.SectionScopeID
				}
			}
			entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
			entry.SectionScopeID = canonicalSectionScope(entry.SectionPath)
			entry.SequenceInScope = entry.SourceQuestionNo
			entry.SourceIdentity = visionSourceIdentity(entry)
			dedupeKey := directoryDedupeKey(entry)
			if previous, exists := seen[dedupeKey]; exists {
				if previous.SourcePageStart == entry.SourcePageStart && previous.SourcePageEnd == entry.SourcePageEnd {
					continue
				}
				// Preserve the first occurrence; relink will surface a duplicate
				// problem if the source is genuinely ambiguous.
				continue
			}
			seen[dedupeKey] = entry
			entries = append(entries, entry)
		}
		if err := s.repo.ReplaceDirectoryEntries(ctx, sessionID, state.documentID, 0, state.role, entries); err != nil {
			return err
		}
	}
	return nil
}

func directoryPhysicalKey(entry domain.VisionDirectoryEntry) string {
	return fmt.Sprintf("%d|%d|%d|%s", entry.SourcePageStart, entry.SourcePageEnd, entry.SourceQuestionNo, domain.CanonicalQuestionType(entry.QuestionType))
}

// syncCandidateDirectoryMetadata moves an already extracted question onto the
// canonical directory identity. Matching by physical page and printed number
// is safe here because this function is only called for one complete directory
// row; it also fixes historical rows whose old question type was wrong (for
// example q35 on page 73 was stored as single_choice instead of multiple_choice).
func (s *ImportService) syncCandidateDirectoryMetadata(ctx context.Context, sessionID, documentID int64, entry domain.VisionDirectoryEntry) error {
	sectionPath := append([]string(nil), entry.SectionPath...)
	sectionJSON, _ := json.Marshal(sectionPath)
	scope := canonicalSectionScope(sectionPath)
	canonical := domain.CanonicalQuestionType(entry.QuestionType)
	identity := entry.SourceIdentity
	if identity == "" {
		identity = visionSourceIdentity(entry)
	}
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),
		       COALESCE(question_type,''),COALESCE(canonical_question_type,'')
		FROM candidate_questions
		WHERE import_session_id=? AND document_id=?
		  AND status NOT IN ('session_duplicate','persisted','duplicate')
		  AND source_question_no=? AND question_page_start=? AND question_page_end=?`,
		sessionID, documentID, entry.SourceQuestionNo, entry.SourcePageStart, entry.SourcePageEnd)
	if err != nil {
		return err
	}
	defer rows.Close()
	type candidateMetadata struct {
		id                                                 int64
		identity, sectionJSON, questionType, canonicalType string
	}
	candidates := []candidateMetadata{}
	for rows.Next() {
		var candidateID int64
		var candidate candidateMetadata
		if err := rows.Scan(&candidateID, &candidate.identity, &candidate.sectionJSON, &candidate.questionType, &candidate.canonicalType); err != nil {
			return err
		}
		candidate.id = candidateID
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	selected := []candidateMetadata{}
	for _, candidate := range candidates {
		if candidate.identity == identity {
			selected = append(selected, candidate)
		}
	}
	if len(selected) == 0 {
		for _, candidate := range candidates {
			canonical := domain.CanonicalQuestionType(candidate.canonicalType)
			if canonical == "" {
				canonical = domain.CanonicalQuestionType(candidate.questionType)
			}
			var path []string
			_ = json.Unmarshal([]byte(candidate.sectionJSON), &path)
			// Preserve distinct subjective blocks that share the same page and
			// printed number. A physical fallback is allowed only when the old
			// projection has one compatible subtype, or when there is exactly one
			// candidate row (the q35 type-correction case).
			if canonical == domain.CanonicalQuestionType(entry.QuestionType) && sectionSubtypeKey(path, candidate.questionType) == sectionSubtypeKey(entry.SectionPath, entry.QuestionType) {
				selected = append(selected, candidate)
			}
		}
		if len(selected) == 0 && len(candidates) == 1 {
			selected = candidates
		}
	}
	for _, candidate := range selected {
		// Keep the extracted stem/options/answer evidence intact. Relink reads
		// the scalar canonical type below and will rewrite merged_json with the
		// repaired type after it selects the answer source.
		_, err := s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET
			source_identity=?,section_path_json=?,section_scope_id=?,set_no=?,
			question_type=?,canonical_question_type=?,updated_at=datetime('now')
			WHERE id=?`, identity, string(sectionJSON), scope, entry.SetNo, entry.QuestionType, canonical, candidate.id)
		if err != nil {
			return err
		}
	}
	return nil
}

func directoryDedupeKey(entry domain.VisionDirectoryEntry) string {
	identity := entry.SourceIdentity
	if identity == "" {
		identity = visionSourceIdentity(entry)
	}
	// The full source identity intentionally includes the subjective subtype.
	// Using only canonicalSectionScope here collapses e.g. q1 简答题 and q1
	// 案例分析题 when both begin on the same physical page.
	return fmt.Sprintf("%s|%d|%d", identity, entry.SourcePageStart, entry.SourcePageEnd)
}

func directoryQuestionGroup(entry domain.VisionDirectoryEntry) string {
	scope := canonicalSectionScope(entry.SectionPath)
	// Persisted candidates from an earlier relink may retain only a durable
	// section_scope_id in section_path_json.  Use that scope as the grouping
	// fallback; otherwise a raw continuation row with an inherited topic can
	// never be recognized as the tail of those candidates.
	if scope == "" {
		scope = strings.TrimSpace(entry.SectionScopeID)
	}
	return fmt.Sprintf("%s|%d|%s", scope, entry.SetNo, domain.CanonicalQuestionType(entry.QuestionType))
}

// directoryEntryFillsCandidateGap deliberately promotes only an internal
// numeric hole bracketed by existing candidates in the same scoped question
// block. Legacy directory units contain many answer-page false positives; a
// broad "raw row missing from candidates" rule would turn those into hundreds
// of bogus expected questions during relink.
func directoryEntryFillsCandidateGap(entry domain.VisionDirectoryEntry, candidateNumbers map[string]map[int]bool) bool {
	if entry.SourceQuestionNo <= 0 {
		return false
	}
	numbers := candidateNumbers[directoryQuestionGroup(entry)]
	if len(numbers) < 2 || numbers[entry.SourceQuestionNo] {
		return false
	}
	lower, upper := 0, 0
	for number := range numbers {
		if number < entry.SourceQuestionNo && number > lower {
			lower = number
		}
		if number > entry.SourceQuestionNo && (upper == 0 || number < upper) {
			upper = number
		}
	}
	return lower > 0 && upper > 0
}

// directoryEntriesExtendingCandidateTail finds raw directory rows that form a
// contiguous suffix immediately following the highest persisted candidate in
// the same topic/type block.  It is intentionally page-bounded: a suffix from
// a later answer page (where numbers commonly restart at 1) must not be
// promoted as a missing question block.
func directoryEntriesExtendingCandidateTail(items []directoryWindowEntry, candidateCount int, candidates []domain.VisionDirectoryEntry, candidatePhysical map[string]bool, candidateNumbers map[string]map[int]bool) map[string]domain.VisionDirectoryEntry {
	promotable := map[string]domain.VisionDirectoryEntry{}
	if candidateCount == 0 || len(candidates) == 0 {
		return promotable
	}
	type candidateBounds struct {
		maxNo   int
		pageEnd int
		context domain.VisionDirectoryEntry
	}
	bounds := map[string]candidateBounds{}
	for _, candidate := range candidates {
		if candidate.ContentRole != "question" || candidate.SourceQuestionNo <= 0 {
			continue
		}
		group := directoryQuestionGroup(candidate)
		b := bounds[group]
		if candidate.SourceQuestionNo > b.maxNo {
			b.maxNo = candidate.SourceQuestionNo
		}
		if candidate.SourcePageEnd > b.pageEnd {
			b.pageEnd = candidate.SourcePageEnd
		}
		if b.context.ContentRole == "" || candidate.SourceQuestionNo == b.maxNo {
			b.context = candidate
		}
		bounds[group] = b
	}
	rawByGroup := map[string][]domain.VisionDirectoryEntry{}
	for _, item := range items {
		if item.windowIndex < candidateCount || item.entry.ContentRole != "question" {
			continue
		}
		entry := item.entry
		if entry.SourceQuestionNo <= 0 || candidatePhysical[directoryPhysicalKey(entry)] {
			continue
		}
		group := directoryQuestionGroup(entry)
		if _, ok := bounds[group]; !ok {
			// Scope propagation can be corrupted by a later retry window. Recover
			// the intended block from the nearest persisted candidate on the same
			// (or immediately preceding) physical page, but only for the same type.
			nearestGroup, nearestPage := "", -1
			for candidateGroup, candidateBounds := range bounds {
				if domain.CanonicalQuestionType(entry.QuestionType) != domain.CanonicalQuestionType(candidateBounds.context.QuestionType) || candidateBounds.context.SourcePageStart > entry.SourcePageStart {
					continue
				}
				if candidateBounds.context.SourcePageStart > nearestPage {
					nearestGroup, nearestPage = candidateGroup, candidateBounds.context.SourcePageStart
				}
			}
			if nearestGroup == "" {
				continue
			}
			group = nearestGroup
		}
		rawByGroup[group] = append(rawByGroup[group], entry)
	}
	for group, raw := range rawByGroup {
		b, ok := bounds[group]
		if !ok || b.maxNo <= 0 || len(candidateNumbers[group]) == 0 {
			continue
		}
		// De-duplicate overlap rows by question number and retain the earliest
		// physical occurrence for adjacency checks.
		byNo := map[int]domain.VisionDirectoryEntry{}
		for _, entry := range raw {
			if previous, exists := byNo[entry.SourceQuestionNo]; !exists || entry.SourcePageStart < previous.SourcePageStart {
				byNo[entry.SourceQuestionNo] = entry
			}
		}
		first, exists := byNo[b.maxNo+1]
		if !exists || first.SourcePageStart > b.pageEnd+1 {
			continue
		}
		for number := b.maxNo + 1; ; number++ {
			entry, exists := byNo[number]
			if !exists {
				break
			}
			context := b.context
			context.ContentRole = "question"
			promotable[directoryPhysicalKey(entry)] = context
		}
	}
	return promotable
}

func sectionPathFromSourceIdentity(identity string) []string {
	marker := "|question|"
	index := strings.Index(identity, marker)
	if index <= 0 {
		return nil
	}
	var path []string
	if json.Unmarshal([]byte(identity[:index]), &path) != nil {
		return nil
	}
	return path
}

func parseDirectoryUnitKey(key string) (int, int, bool) {
	parts := strings.SplitN(key, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, errStart := strconv.Atoi(parts[0])
	end, errEnd := strconv.Atoi(parts[1])
	return start, end, errStart == nil && errEnd == nil && start > 0 && end >= start
}

// directoryWindowEntry keeps the page-window ownership alongside a directory
// entry while the coordinator reconstructs omitted section headings. Models
// often emit only "二、多项选择题" on continuation pages; that heading is not
// a stable identity by itself and must inherit the last explicit topic in the
// same document stream before source identities are generated.
type directoryWindowEntry struct {
	windowIndex int
	order       int
	entry       domain.VisionDirectoryEntry
}

func normalizeDirectoryScopes(items []directoryWindowEntry) []directoryWindowEntry {
	if len(items) < 2 {
		return items
	}
	sorted := append([]directoryWindowEntry(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i].entry, sorted[j].entry
		leftBucket, rightBucket := directoryScopeBucket(left.ContentRole), directoryScopeBucket(right.ContentRole)
		if leftBucket != rightBucket {
			return leftBucket < rightBucket
		}
		if left.SourcePageStart != right.SourcePageStart {
			return left.SourcePageStart < right.SourcePageStart
		}
		if left.SourcePageEnd != right.SourcePageEnd {
			return left.SourcePageEnd < right.SourcePageEnd
		}
		if left.MarkerTop != right.MarkerTop {
			// marker_top is a 0-1000 vertical anchor. Zero means unavailable;
			// place unavailable rows after anchored rows so they cannot reset the
			// active scope before a physically earlier heading is processed.
			if left.MarkerTop == 0 {
				return false
			}
			if right.MarkerTop == 0 {
				return true
			}
			return left.MarkerTop < right.MarkerTop
		}
		return sorted[i].order < sorted[j].order
	})

	active := map[string][]string{}
	type headingContext struct {
		base    []string
		heading []string
		endPage int
		order   int
	}
	contexts := map[string][]headingContext{}
	for i := range sorted {
		entry := sorted[i].entry
		bucket := directoryScopeBucket(entry.ContentRole)
		base, heading := splitDirectorySectionPath(entry.SectionPath)
		hadTopic := len(base) > 0
		// A page/window boundary may repeat only a chapter heading while the
		// numbered block from the preceding specific topic continues (page 209
		// is a representative example). A q2+ continuation cannot open a new
		// topic merely because the model returned that generic parent heading.
		if len(base) > 0 && len(active[bucket]) > 0 && isGenericDirectoryParent(base) && !isGenericDirectoryParent(active[bucket]) {
			base = append([]string(nil), active[bucket]...)
			hadTopic = false
		}
		if len(base) == 0 && strings.TrimSpace(entry.SectionScopeID) != "" {
			// Candidate projections may carry a durable scope even when their
			// original section_path_json is empty. Treat it as the explicit
			// topic anchor; otherwise a continuation case/short-answer row
			// cannot inherit its correct answer section.
			base = []string{strings.TrimSpace(entry.SectionScopeID)}
		}
		if len(base) > 0 {
			active[bucket] = append([]string(nil), base...)
		}
		if len(heading) > 0 {
			// Keep a bounded physical-page context for continuation rows. A
			// later heading on the same page must not steal a case-analysis
			// row that continues onto the next page.
			if len(active[bucket]) > 0 {
				contexts[bucket] = append(contexts[bucket], headingContext{
					base: append([]string(nil), active[bucket]...), heading: append([]string(nil), heading...),
					endPage: entry.SourcePageEnd, order: sorted[i].order,
				})
			}
		}
		if len(entry.SectionPath) == 0 && len(base) > 0 {
			// Select the nearest preceding heading whose physical range still
			// covers this row. This is what restores a page-17 case question
			// after page-16 also contained short-answer and discrimination
			// headings.
			best := headingContext{}
			for _, candidate := range contexts[bucket] {
				if canonicalSectionScope(candidate.base) != canonicalSectionScope(base) || candidate.endPage < entry.SourcePageStart {
					continue
				}
				if best.heading == nil || candidate.endPage > best.endPage || (candidate.endPage == best.endPage && candidate.order > best.order) {
					best = candidate
				}
			}
			if len(best.heading) > 0 && (entry.CanonicalQuestionType != "subjective" || sectionSubtype(best.heading) != "") {
				heading = best.heading
			}
		}
		if len(active[bucket]) > 0 && (!hadTopic || len(entry.SectionPath) == 0) {
			// Preserve an explicit local type heading while attaching the
			// inherited topic. This makes q7 in topic B distinct from q7 in
			// topic A even when both model responses said only "二、多选".
			inherited := append([]string(nil), active[bucket]...)
			if len(heading) > 0 {
				inherited = append(inherited, heading...)
			}
			entry.SectionPath = inherited
		}
		sorted[i].entry = entry
	}
	// A model may return only the durable topic scope (without either a local
	// heading or a path) for a continuation row. Recover the heading in a
	// second pass from normalized rows whose physical page span covers that
	// row. This handles the common case where several section headings share
	// the preceding page and the actual case question starts on the next one.
	for i := range sorted {
		entry := sorted[i].entry
		if directoryScopeBucket(entry.ContentRole) != "question" || entry.CanonicalQuestionType != "subjective" || len(entry.SectionPath) != 1 || strings.TrimSpace(entry.SectionScopeID) == "" || strings.TrimSpace(entry.SectionPath[0]) != strings.TrimSpace(entry.SectionScopeID) {
			continue
		}
		best := headingContext{}
		for _, candidate := range sorted {
			if candidate.entry.ContentRole != "question" || candidate.order == sorted[i].order || candidate.entry.SourcePageEnd < entry.SourcePageStart {
				continue
			}
			candidateBase, candidateHeading := splitDirectorySectionPath(candidate.entry.SectionPath)
			if len(candidateHeading) == 0 || (entry.CanonicalQuestionType == "subjective" && sectionSubtype(candidateHeading) == "") || canonicalSectionScope(candidateBase) != canonicalSectionScope([]string{entry.SectionScopeID}) {
				continue
			}
			if best.heading == nil || candidate.entry.SourcePageEnd > best.endPage || (candidate.entry.SourcePageEnd == best.endPage && candidate.order > best.order) {
				best = headingContext{heading: append([]string(nil), candidateHeading...), endPage: candidate.entry.SourcePageEnd, order: candidate.order}
			}
		}
		if len(best.heading) > 0 {
			entry.SectionPath = append([]string{entry.SectionScopeID}, best.heading...)
			sorted[i].entry = entry
		}
	}
	// Catalog windows sometimes omit a repeated type heading at the exact
	// boundary where a question block continues.  A raw row such as q35 can
	// therefore say "single_choice" even though q34 in the preceding window
	// is the last row of the same multiple-choice block.  Repair only a
	// physically adjacent, numerically contiguous row in the same topic.  A
	// reset to q1 never inherits the previous type, so separate blocks and
	// separate topics remain isolated.
	sorted = normalizeDirectoryQuestionBlocks(sorted)
	// Restore deterministic input order for callers that attach entries to a
	// particular window. Identity generation has already seen normalized paths.
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].order < sorted[j].order })
	return sorted
}

func isGenericDirectoryParent(parts []string) bool {
	if len(parts) != 1 {
		return false
	}
	v := compactText(parts[0])
	return regexp.MustCompile(`^第[一二三四五六七八九十百千万0-9]+章`).MatchString(v) ||
		regexp.MustCompile(`^专题[一二三四五六七八九十百千万0-9]+$`).MatchString(v) ||
		v == "上一章/专题" || v == "上一章专题"
}

type directoryQuestionCursor struct {
	questionType string
	setNo        int
	questionNo   int
	pageEnd      int
}

// normalizeDirectoryQuestionBlocks fills the two pieces of question identity
// that are most often omitted at a page/window boundary: the local type and
// the set number. It also gives a stable ordinal to an explicitly unnumbered
// subjective prompt, but only when that subjective section contains no other
// numbered entry. We never invent a number inside a numbered section.
func normalizeDirectoryQuestionBlocks(items []directoryWindowEntry) []directoryWindowEntry {
	if len(items) == 0 {
		return items
	}
	sorted := append([]directoryWindowEntry(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i].entry, sorted[j].entry
		if left.SourcePageStart != right.SourcePageStart {
			return left.SourcePageStart < right.SourcePageStart
		}
		if left.SourcePageEnd != right.SourcePageEnd {
			return left.SourcePageEnd < right.SourcePageEnd
		}
		if left.MarkerTop != right.MarkerTop {
			if left.MarkerTop == 0 {
				return false
			}
			if right.MarkerTop == 0 {
				return true
			}
			return left.MarkerTop < right.MarkerTop
		}
		return sorted[i].order < sorted[j].order
	})

	// First identify explicitly numbered subjective blocks.  An unnumbered
	// anchor in a block that already has q1/q2 is usually a continuation or a
	// heading, not a separate question.
	numberedByBlock := map[string]bool{}
	for _, item := range sorted {
		entry := item.entry
		if entry.ContentRole != "question" || entry.SourceQuestionNo <= 0 {
			continue
		}
		key := directoryQuestionBlockKey(entry)
		numberedByBlock[key] = true
	}

	lastByScope := map[string]directoryQuestionCursor{}
	for i := range sorted {
		entry := sorted[i].entry
		if entry.ContentRole != "question" {
			continue
		}
		entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
		if entry.CanonicalQuestionType == "" {
			entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.CanonicalQuestionType)
		}

		if entry.SourceQuestionNo <= 0 && entry.CanonicalQuestionType == "subjective" && !numberedByBlock[directoryQuestionBlockKey(entry)] {
			// The printed prompt on page 90 is a real standalone论述题 but has no
			// numeric marker.  The first physical prompt in this otherwise empty
			// block is its auditable ordinal; this is not a cross-section guess.
			entry.SourceQuestionNo = 1
		}

		scope := canonicalSectionScope(entry.SectionPath)
		previous, hasPrevious := lastByScope[scope]
		currentType := domain.CanonicalQuestionType(entry.QuestionType)
		if currentType == "" {
			currentType = entry.CanonicalQuestionType
		}
		if hasPrevious && scope != "" && entry.SourceQuestionNo > 0 &&
			entry.SourceQuestionNo == previous.questionNo+1 &&
			entry.SourcePageStart <= previous.pageEnd+1 {
			// A missing local heading is represented by a path containing only
			// the topic. If a heading is present, the model explicitly opened a
			// new block and its type must win.
			if sectionSubtype(entry.SectionPath) == "" {
				if currentType == "" || currentType != previous.questionType {
					entry.QuestionType = previous.questionType
					entry.CanonicalQuestionType = previous.questionType
				}
				// When the local heading is omitted, model-produced set numbers
				// are often the ordinal of the heading it last saw (for example
				// q6 on the next page may say set 1 after a set-3 判断题 block).
				// The contiguous physical block is stronger evidence, so carry
				// forward the previous set unconditionally rather than splitting
				// one printed block and losing its answer section.
				if previous.setNo != 0 {
					entry.SetNo = previous.setNo
				}
			}
		}
		if entry.CanonicalQuestionType == "" {
			entry.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
		}
		if entry.SourceQuestionNo > 0 && scope != "" && entry.CanonicalQuestionType != "" {
			lastByScope[scope] = directoryQuestionCursor{questionType: entry.CanonicalQuestionType, setNo: entry.SetNo, questionNo: entry.SourceQuestionNo, pageEnd: entry.SourcePageEnd}
		}
		sorted[i].entry = entry
	}
	return sorted
}

func directoryQuestionBlockKey(entry domain.VisionDirectoryEntry) string {
	path, _ := json.Marshal(entry.SectionPath)
	return fmt.Sprintf("%s|%d|%s", string(path), entry.SetNo, domain.CanonicalQuestionType(entry.QuestionType))
}

func directoryScopeBucket(contentRole string) string {
	if contentRole == "question" {
		return "question"
	}
	// answer and explanation pages belong to one document stream: an answer
	// table can establish the topic for the following explanation pages.
	return "answer"
}

func splitDirectorySectionPath(parts []string) (base, heading []string) {
	for _, part := range parts {
		cleaned := cleanDirectorySectionPart(part)
		if cleaned == "" {
			continue
		}
		if isDirectoryQuestionHeading(cleaned) {
			heading = append(heading, cleaned)
			continue
		}
		base = append(base, cleaned)
	}
	return base, heading
}

func cleanDirectorySectionPart(part string) string {
	v := compactText(part)
	for _, prefix := range []string{"参考答案及解析", "答案及解析", "参考答案", "答案解析"} {
		v = strings.TrimPrefix(v, prefix)
	}
	v = strings.TrimLeft(v, "·:：-/")
	return strings.TrimSpace(v)
}

func isDirectoryQuestionHeading(value string) bool {
	return value == "一、单项选择题" || value == "一、单选题" ||
		value == "选择题" || value == "单选" || value == "多选" ||
		value == "二、多项选择题" || value == "二、多选题" ||
		value == "三、判断题" || value == "四、判断题" ||
		value == "四、填空题" || value == "五、填空题" ||
		value == "四、简答题" || value == "五、简答题" ||
		value == "四、案例分析题" || value == "五、案例分析题" || value == "六、案例分析题" ||
		value == "四、辨析题" || value == "五、辨析题" ||
		value == "四、论述题" || value == "五、论述题" ||
		strings.Contains(value, "单项选择题") || strings.Contains(value, "单选题") ||
		strings.Contains(value, "多项选择题") || strings.Contains(value, "多选题") ||
		strings.Contains(value, "判断题") || strings.Contains(value, "填空题") ||
		strings.Contains(value, "简答题") || strings.Contains(value, "主观题") ||
		strings.Contains(value, "案例分析题") || strings.Contains(value, "辨析题") ||
		strings.Contains(value, "论述题")
}
