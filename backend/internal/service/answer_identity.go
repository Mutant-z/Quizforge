package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// answerIdentityMatch is deliberately small and auditable. A source identity
// is authoritative when it names an active answer-directory row. If the model
// omitted it (or changed only presentation headings), the fallback must still
// be unique within the same document, type, number, compatible set and page.
type answerIdentityMatch struct {
	Entry  domain.VisionDirectoryEntry
	Method string
}

func normalizeVisionAnswerIdentity(answer domain.VisionAnswer, entries []domain.VisionDirectoryEntry) (domain.VisionAnswer, answerIdentityMatch, bool) {
	parsedPath, parsedRole, parsedSet, parsedType, parsedNo, parsedOK := parseVisionSourceIdentity(answer.SourceIdentity)
	canonicalType := domain.CanonicalQuestionType(answer.CanonicalQuestionType)
	if canonicalType == "" {
		canonicalType = domain.CanonicalQuestionType(answer.QuestionType)
	}
	if canonicalType == "" && parsedOK {
		canonicalType = domain.CanonicalQuestionType(parsedType)
	}
	setNo := answer.SetNo
	if parsedOK && setNo == 0 {
		setNo = parsedSet
	}
	questionNo := answer.SourceQuestionNo
	if parsedOK && questionNo == 0 {
		questionNo = parsedNo
	}
	requestedScope := canonicalSectionScope(answer.SectionPath)
	if requestedScope == "" && strings.TrimSpace(answer.SectionScopeID) != "" {
		requestedScope = canonicalSectionScope(strings.Split(answer.SectionScopeID, "/"))
	}
	if requestedScope == "" && parsedOK {
		requestedScope = canonicalSectionScope(parsedPath)
	}

	// Exact identity is the strongest possible match. Do not accept a question
	// identity as an answer identity even when its number happens to match.
	if strings.TrimSpace(answer.SourceIdentity) != "" {
		for _, entry := range entries {
			if entry.ContentRole != "answer" && entry.ContentRole != "explanation" {
				continue
			}
			if entry.SourceIdentity == answer.SourceIdentity {
				return applyAnswerDirectoryIdentity(answer, entry), answerIdentityMatch{Entry: entry, Method: "source_identity"}, true
			}
		}
	}
	if questionNo <= 0 || canonicalType == "" {
		return answer, answerIdentityMatch{}, false
	}

	strict := make([]domain.VisionDirectoryEntry, 0, 2)
	physical := make([]domain.VisionDirectoryEntry, 0, 2)
	for _, entry := range entries {
		if entry.ContentRole != "answer" && entry.ContentRole != "explanation" {
			continue
		}
		if parsedOK && (parsedRole == "answer" || parsedRole == "explanation") && entry.ContentRole != parsedRole {
			continue
		}
		entryType := domain.CanonicalQuestionType(entry.CanonicalQuestionType)
		if entryType == "" {
			entryType = domain.CanonicalQuestionType(entry.QuestionType)
		}
		if entry.SourceQuestionNo != questionNo || entryType != canonicalType || !answerSetCompatible(setNo, entry.SetNo) {
			continue
		}
		if answer.SourcePageStart > 0 && answer.SourcePageEnd >= answer.SourcePageStart && !rangesOverlap([][]int{{entry.SourcePageStart, entry.SourcePageEnd}}, answer.SourcePageStart, answer.SourcePageEnd) {
			continue
		}
		if requestedScope != "" && visionDirectoryEntryScope(entry) == requestedScope {
			strict = append(strict, entry)
		}
		physical = append(physical, entry)
	}
	if len(strict) == 1 {
		return applyAnswerDirectoryIdentity(answer, strict[0]), answerIdentityMatch{Entry: strict[0], Method: "identity_scope"}, true
	}
	// A persisted record from the broken implementation may carry the previous
	// page's scope. Only repair that stale scope when the physical evidence is
	// unique; never use this branch as a general cross-topic fuzzy match.
	if len(physical) == 1 {
		return applyAnswerDirectoryIdentity(answer, physical[0]), answerIdentityMatch{Entry: physical[0], Method: "physical_unique"}, true
	}
	return answer, answerIdentityMatch{}, false
}

func visionDirectoryEntryScope(entry domain.VisionDirectoryEntry) string {
	if strings.TrimSpace(entry.SectionScopeID) != "" {
		return canonicalSectionScope(strings.Split(entry.SectionScopeID, "/"))
	}
	return canonicalSectionScope(entry.SectionPath)
}

func applyAnswerDirectoryIdentity(answer domain.VisionAnswer, entry domain.VisionDirectoryEntry) domain.VisionAnswer {
	answer.SourceIdentity = entry.SourceIdentity
	answer.SectionPath = append([]string(nil), entry.SectionPath...)
	answer.SectionScopeID = entry.SectionScopeID
	if answer.SectionScopeID == "" {
		answer.SectionScopeID = canonicalSectionScope(entry.SectionPath)
	}
	answer.SetNo = entry.SetNo
	answer.QuestionType = entry.QuestionType
	answer.CanonicalQuestionType = domain.CanonicalQuestionType(entry.CanonicalQuestionType)
	if answer.CanonicalQuestionType == "" {
		answer.CanonicalQuestionType = domain.CanonicalQuestionType(entry.QuestionType)
	}
	answer.SourceQuestionNo = entry.SourceQuestionNo
	return answer
}

// distinctVisionAnswerEvidence collapses overlapping answer/explanation
// sources that carry the same value while retaining different values for the
// linker to report as ANSWER_CONFLICT. The richer explanation wins when the
// printed answer is identical.
func distinctVisionAnswerEvidence(items []domain.VisionAnswer) []domain.VisionAnswer {
	if len(items) < 2 {
		return items
	}
	byValue := map[string]int{}
	out := make([]domain.VisionAnswer, 0, len(items))
	for _, item := range items {
		key := strings.Join(item.Answer, "\x00")
		if key == "" {
			key = "analysis:" + normalizeSubjectiveEvidence(item.Analysis)
		}
		if index, exists := byValue[key]; exists {
			if len(item.Analysis) > len(out[index].Analysis) {
				out[index] = item
			}
			continue
		}
		byValue[key] = len(out)
		out = append(out, item)
	}
	return out
}

func parseVisionSourceIdentity(identity string) (path []string, role string, setNo int, qtype string, qno int, ok bool) {
	separator := strings.Index(identity, "|")
	if separator <= 0 {
		return nil, "", 0, "", 0, false
	}
	if json.Unmarshal([]byte(identity[:separator]), &path) != nil {
		return nil, "", 0, "", 0, false
	}
	parts := strings.Split(identity[separator+1:], "|")
	if len(parts) != 4 {
		return nil, "", 0, "", 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &setNo); err != nil {
		return nil, "", 0, "", 0, false
	}
	if _, err := fmt.Sscanf(parts[3], "%d", &qno); err != nil {
		return nil, "", 0, "", 0, false
	}
	role, qtype = parts[0], parts[2]
	return path, role, setNo, qtype, qno, true
}

func (s *ImportService) loadAllAnswerDirectoryEntries(ctx context.Context, sessionID int64) (map[int64][]domain.VisionDirectoryEntry, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `SELECT document_id,content_role,COALESCE(section_path_json,'[]'),set_no,COALESCE(question_type,''),COALESCE(canonical_question_type,''),COALESCE(section_scope_id,''),source_question_no,source_page_start,source_page_end,COALESCE(source_identity,'') FROM import_directory_entries WHERE session_id=? AND status='active' AND content_role IN ('answer','explanation') ORDER BY document_id,source_page_start,source_question_no,id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDocument := map[int64][]domain.VisionDirectoryEntry{}
	for rows.Next() {
		var documentID int64
		var entry domain.VisionDirectoryEntry
		var sectionJSON string
		if err := rows.Scan(&documentID, &entry.ContentRole, &sectionJSON, &entry.SetNo, &entry.QuestionType, &entry.CanonicalQuestionType, &entry.SectionScopeID, &entry.SourceQuestionNo, &entry.SourcePageStart, &entry.SourcePageEnd, &entry.SourceIdentity); err != nil {
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
		if len(entry.SectionPath) == 0 && entry.SectionScopeID != "" {
			entry.SectionPath = strings.Split(entry.SectionScopeID, "/")
		}
		if entry.SourceIdentity == "" {
			entry.SourceIdentity = visionSourceIdentity(entry)
		}
		byDocument[documentID] = append(byDocument[documentID], entry)
	}
	return byDocument, rows.Err()
}

func answerRecordSectionPath(sectionJSON, sectionScope string) ([]string, string) {
	var path []string
	if json.Unmarshal([]byte(sectionJSON), &path) != nil {
		path = nil
	}
	scope := canonicalSectionScope(path)
	if scope == "" && strings.TrimSpace(sectionScope) != "" {
		scope = canonicalSectionScope(strings.Split(sectionScope, "/"))
		if len(path) == 0 {
			path = strings.Split(scope, "/")
		}
	}
	return path, scope
}
