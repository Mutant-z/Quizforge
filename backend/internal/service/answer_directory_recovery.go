package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// recoverExplicitAnswersFromDirectory fills only answers that are printed in
// an already validated answer/explanation directory entry. This is not an LLM
// guess: the directory anchor must contain the entry's own question number
// followed by an answer token (for example "7.BCD【解析】" or "6.√"). The raw
// directory output remains the audit source and the normal answer linker still
// validates the resulting value against the question options.
func (s *ImportService) recoverExplicitAnswersFromDirectory(ctx context.Context, sessionID, runID int64) (int, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT document_id,content_role,section_path_json,section_scope_id,set_no,canonical_question_type,
		       source_question_no,source_page_start,source_page_end,COALESCE(anchor,''),COALESCE(source_identity,'')
		FROM import_directory_entries
		WHERE session_id=? AND status='active'
		  AND content_role IN ('answer','explanation')
		  AND canonical_question_type IN ('single_choice','multiple_choice','true_false','fill_in_blank','subjective')
		ORDER BY source_page_start,source_page_end,id`, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type directoryAnswer struct {
		documentID                            int64
		contentRole                           string
		sectionJSON, sectionScope, type_      string
		setNo, questionNo, pageStart, pageEnd int
		anchor, identity                      string
	}
	items := []directoryAnswer{}
	for rows.Next() {
		var item directoryAnswer
		if err := rows.Scan(&item.documentID, &item.contentRole, &item.sectionJSON, &item.sectionScope, &item.setNo, &item.type_, &item.questionNo, &item.pageStart, &item.pageEnd, &item.anchor, &item.identity); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	recovered := 0
	for _, item := range items {
		values, analysis, ok := explicitAnswerFromAnchor(item.anchor, item.questionNo, item.type_)
		if item.type_ == "subjective" {
			// Subjective answer sections often have no option token: the
			// directory anchor itself is the beginning of the reference answer
			// (for example "1.(1)学校应该承担责任……"). Preserve it as an
			// analysis-only source. This is evidence copied from the PDF, not a
			// generated answer. An explicit rubric token such as "1.ABD" is
			// retained in Answer as a reference marker as well.
			if !ok || len(values) == 0 {
				if !isReferenceAnswerSection(item.sectionJSON) {
					// A directory row under a plain question heading (without
					// “参考答案”/“解析”) may simply be the question restated on
					// the next page. Never promote that text to an answer source.
					continue
				}
				if strings.TrimSpace(item.anchor) == "" {
					continue
				}
				values = []string{}
				analysis = strings.TrimSpace(item.anchor)
				ok = true
			}
		}
		if !ok || (len(values) == 0 && strings.TrimSpace(analysis) == "") {
			continue
		}
		// Never replace a non-empty answer already extracted from the model. Match
		// the durable source by its document/type/number/page and source role, not
		// by section_scope_id: older sessions may have the right answer under the
		// previous page's (wrong) scope. Relink will repair that projection in
		// place; requiring the stale scope here would create a duplicate row.
		var existing int
		_ = s.repo.DB().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM import_answer_records
			WHERE session_id=? AND document_id=? AND source_kind=?
		  AND canonical_question_type=? AND set_no=? AND source_question_no=?
		  AND page_start=? AND json_valid(COALESCE(answer_json,'[]'))=1
		  AND (json_array_length(COALESCE(answer_json,'[]'))>0 OR (?='subjective' AND trim(COALESCE(analysis,''))<>''))`,
			sessionID, item.documentID, item.contentRole, domain.CanonicalQuestionType(item.type_), item.setNo, item.questionNo, item.pageStart, domain.CanonicalQuestionType(item.type_)).Scan(&existing)
		if existing > 0 {
			continue
		}
		var sectionPath []string
		if json.Unmarshal([]byte(item.sectionJSON), &sectionPath) != nil {
			sectionPath = nil
		}
		identity := item.identity
		if identity == "" {
			identity = visionSourceIdentity(domain.VisionDirectoryEntry{ContentRole: item.contentRole, SectionPath: sectionPath, SetNo: item.setNo, QuestionType: item.type_, SourceQuestionNo: item.questionNo})
		}
		answer := domain.VisionAnswer{
			SourceIdentity:        identity,
			SectionPath:           sectionPath,
			SectionScopeID:        item.sectionScope,
			SetNo:                 item.setNo,
			QuestionType:          domain.CanonicalQuestionType(item.type_),
			CanonicalQuestionType: domain.CanonicalQuestionType(item.type_),
			SourceQuestionNo:      item.questionNo,
			Answer:                values,
			Analysis:              analysis,
			SourcePageStart:       item.pageStart,
			SourcePageEnd:         item.pageEnd,
		}
		if err := s.repo.UpsertAnswerRecords(ctx, sessionID, item.documentID, runID, []domain.VisionAnswer{answer}); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func isReferenceAnswerSection(sectionJSON string) bool {
	text := strings.TrimSpace(sectionJSON)
	return strings.Contains(text, "参考答案") || strings.Contains(text, "解析")
}

var explicitAnswerPrefix = regexp.MustCompile(`^\s*(\d{1,4})\s*[\.．、:：\s]+((?:[A-Fa-f](?:\s*[、,，/\\+&]\s*)?){1,6}|[√✓✔×✕✗Xx]|正确|错误)`)

// explicitAnswerFromAnchor recognizes only the answer token immediately
// following the directory entry's own question number. It intentionally does
// not scan arbitrary text for letters, which would turn explanation prose
// into a fabricated answer.
func explicitAnswerFromAnchor(anchor string, questionNo int, questionType string) ([]string, string, bool) {
	if questionNo <= 0 || strings.TrimSpace(anchor) == "" {
		return nil, "", false
	}
	canonical := domain.CanonicalQuestionType(questionType)
	if canonical == "fill_in_blank" {
		// Fill-in answers are printed as free text rather than A-F tokens. The
		// directory entry is already tied to one physical answer row, so only
		// accept the text immediately after that row's own number and never scan
		// arbitrary explanation prose.
		fillPrefix := regexp.MustCompile(`^\s*(\d{1,4})\s*[\.．、:：\s]+(.+)$`)
		matches := fillPrefix.FindStringSubmatch(anchor)
		if len(matches) != 3 {
			return nil, "", false
		}
		var parsedNo int
		if _, err := fmt.Sscanf(matches[1], "%d", &parsedNo); err != nil || parsedNo != questionNo {
			return nil, "", false
		}
		value := strings.TrimSpace(matches[2])
		analysis := ""
		for _, marker := range []string{"【解析】", "（解析）", "[解析]", "解析：", "解析:"} {
			if index := strings.Index(value, marker); index >= 0 {
				analysis = strings.TrimSpace(value[index+len(marker):])
				value = strings.TrimSpace(value[:index])
				break
			}
		}
		if value == "" {
			return nil, "", false
		}
		return []string{value}, analysis, true
	}
	matches := explicitAnswerPrefix.FindStringSubmatch(anchor)
	if len(matches) != 3 {
		return nil, "", false
	}
	var parsedNo int
	if _, err := fmt.Sscanf(matches[1], "%d", &parsedNo); err != nil || parsedNo != questionNo {
		return nil, "", false
	}
	token := strings.TrimSpace(matches[2])
	values := []string{}
	if canonical == "true_false" {
		values = normalizedAnswerValues(mustJSON([]string{token}), canonical)
	} else {
		token = strings.ToUpper(strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '、', ',', '，', '/', '\\', '+', '&':
				return -1
			default:
				return r
			}
		}, token))
		for _, r := range token {
			if r < 'A' || r > 'F' {
				return nil, "", false
			}
			values = append(values, string(r))
		}
	}
	if len(values) == 0 {
		return nil, "", false
	}
	// Keep the explanation after the explicit answer marker as a source note.
	rest := strings.TrimSpace(anchor[len(matches[0]):])
	analysis := ""
	for _, marker := range []string{"【解析】", "（解析）", "[解析]", "解析：", "解析:"} {
		if index := strings.Index(rest, marker); index >= 0 {
			analysis = strings.TrimSpace(rest[index+len(marker):])
			break
		}
	}
	return values, analysis, true
}

func mustJSON(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}
