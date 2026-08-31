package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// canonicalSectionScope removes the trailing question-type heading.  A type
// heading is a presentation detail, not a topic identity; the remaining
// normalized path is stable across adjacent 4+1 windows.
func canonicalSectionScope(parts []string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		v := compactText(part)
		// Answer/explanation books prepend a document-level label which is
		// not part of the question's topic identity.
		for _, prefix := range []string{"参考答案及解析", "答案及解析", "参考答案", "答案解析"} {
			v = strings.TrimPrefix(v, prefix)
		}
		v = strings.TrimLeft(v, "·:：-/")
		// Some answer pages omit the numeric 专题 prefix while the question
		// page keeps it (e.g. 专题三《教育法》 vs 《教育法》). Keep a bare
		// 专题一 heading, but normalize 专题N + title to the title so both
		// representations share the same scope identity.
		if m := topicPrefixPattern.FindStringSubmatch(v); len(m) == 2 && m[1] != "" {
			v = m[1]
		}
		if v == "" || strings.Contains(v, "单项选择") || strings.Contains(v, "单选题") ||
			strings.Contains(v, "多项选择") || strings.Contains(v, "多选题") ||
			v == "选择题" || v == "单选" || v == "多选" ||
			strings.Contains(v, "判断题") || strings.Contains(v, "填空题") ||
			strings.Contains(v, "简答题") || strings.Contains(v, "主观题") ||
			strings.Contains(v, "案例分析题") || strings.Contains(v, "辨析题") ||
			strings.Contains(v, "论述题") || v == "答案速查" || v == "答案速查表" {
			continue
		}
		if len(values) == 0 || values[len(values)-1] != v {
			values = append(values, v)
		}
	}
	return strings.Join(values, "/")
}

var topicPrefixPattern = regexp.MustCompile(`^专题[一二三四五六七八九十百千万0-9]+(.+)$`)

func alignmentKey(scope string, setNo int, qtype string, qno int) string {
	return fmt.Sprintf("%s|%d|%s|%d", matchScopeKey(scope), setNo, domain.CanonicalQuestionType(qtype), qno)
}

type alignmentCandidate struct {
	id, documentID                              int64
	identity, sectionScope, sectionJSON, merged string
	answerJSON                                  string
	setNo, qno, pageStart, pageEnd              int
	questionType, canonicalType, subtype        string
	answerLinkMethod, answerLinkStatus          string
	reviewDecision                              string
	answerPageStart, answerPageEnd              int
}

type alignmentAnswer struct {
	id, documentID                                                        int64
	identity, sectionScope, sectionJSON, answerJSON, analysis, sourceKind string
	identityExplicit                                                      bool
	setNo, qno, pageStart, pageEnd                                        int
	rawType, canonicalType, subtype                                       string
	runID                                                                 int64
}

// RelinkSessionAnswers is deliberately deterministic. It first uses the
// scoped canonical identity, then falls back only when the candidate set is
// globally unique. It never uses a suffix/substring section match.
func (s *ImportService) RelinkSessionAnswers(ctx context.Context, sessionID, runID int64) error {
	// Rebuild the derived directory projection from completed Agent units before
	// matching. This is deterministic and fixes type-only continuation headings
	// that previously collided across topics; raw model runs remain auditable.
	if err := s.rebuildSessionDirectoriesFromUnits(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rekeyQuestionCandidatesToDirectory(ctx, sessionID); err != nil {
		return err
	}
	// Rows without an immutable source identity are legacy projection data. A
	// blank identity cannot be safely linked after sections repeat, so remove
	// only this derived data and rebuild it from the durable unit JSON below.
	// The original model output and run history remain untouched for audit.
	if _, err := s.repo.DB().ExecContext(ctx, `DELETE FROM import_answer_records WHERE session_id=? AND trim(COALESCE(source_identity,''))=''`, sessionID); err != nil {
		return err
	}
	if recoveredQuestions, err := s.recoverQuestionsFromCompletedUnits(ctx, sessionID, runID); err != nil {
		return err
	} else if recoveredQuestions > 0 && runID > 0 {
		run := runID
		s.emitVisionEvent(ctx, &domain.ImportJob{ID: 0, SessionID: &sessionID}, &domain.ImportEvent{RunID: &run, Type: "question_delta", Stage: "linking", AgentRole: "QuestionExtractionAgent", Summary: fmt.Sprintf("从已完成题目提取结果恢复 %d 条目录题目", recoveredQuestions)})
	}
	// Replay completed answer units before reading answer records. This restores
	// answer blocks that were previously collapsed by the legacy path key while
	// preserving the original model output for audit.
	replayedAnswers, err := s.recoverAnswersFromCompletedUnits(ctx, sessionID, runID)
	if err != nil {
		return err
	}
	if replayedAnswers > 0 && runID > 0 {
		run := runID
		s.emitVisionEvent(ctx, &domain.ImportJob{ID: 0, SessionID: &sessionID}, &domain.ImportEvent{RunID: &run, Type: "answer_delta", Stage: "linking", AgentRole: "AnswerLinkAgent", Summary: fmt.Sprintf("从已完成答案提取结果恢复 %d 条答案记录", replayedAnswers)})
	}
	// Some completed directory windows already contain an explicit answer in
	// the anchor (for example "7.BCD【解析】"), while the answer extraction
	// window may have returned an incomplete unit. Recover those unambiguous
	// printed values before matching so a relink never leaves a safe answer
	// stranded merely because a later model call omitted the record.
	recoveredAnswers, err := s.recoverExplicitAnswersFromDirectory(ctx, sessionID, runID)
	if err != nil {
		return err
	}
	if recoveredAnswers > 0 && runID > 0 {
		run := runID
		s.emitVisionEvent(ctx, &domain.ImportJob{ID: 0, SessionID: &sessionID}, &domain.ImportEvent{RunID: &run, Type: "answer_delta", Stage: "linking", AgentRole: "AnswerLinkAgent", Summary: fmt.Sprintf("从已审计答案目录恢复 %d 条显式答案", recoveredAnswers)})
	}
	if _, err := s.RepairInvalidCandidateBodiesFromIdenticalDocumentSessions(ctx, sessionID); err != nil {
		return err
	}
	if err := s.normalizeMixedCaseStudyCandidates(ctx, sessionID); err != nil {
		return err
	}
	db := s.repo.DB()
	rows, err := db.QueryContext(ctx, `SELECT id,document_id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),COALESCE(section_scope_id,''),set_no,source_question_no,COALESCE(question_type,''),COALESCE(canonical_question_type,''),COALESCE(merged_json,question_json,extracted_json,'{}'),COALESCE(answer_json,'[]'),COALESCE(question_page_start,page_start,0),COALESCE(question_page_end,page_end,0),COALESCE(answer_link_method,''),COALESCE(answer_link_status,''),COALESCE(review_decision,'pending'),COALESCE(answer_page_start,0),COALESCE(answer_page_end,0) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('persisted','duplicate','session_duplicate') ORDER BY question_page_start,id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	candidates := []alignmentCandidate{}
	for rows.Next() {
		var c alignmentCandidate
		if err := rows.Scan(&c.id, &c.documentID, &c.identity, &c.sectionJSON, &c.sectionScope, &c.setNo, &c.qno, &c.questionType, &c.canonicalType, &c.merged, &c.answerJSON, &c.pageStart, &c.pageEnd, &c.answerLinkMethod, &c.answerLinkStatus, &c.reviewDecision, &c.answerPageStart, &c.answerPageEnd); err != nil {
			return err
		}
		if c.canonicalType == "" {
			c.canonicalType = domain.CanonicalQuestionType(c.questionType)
		}
		c.canonicalType = domain.CanonicalQuestionType(c.canonicalType)
		var sectionParts []string
		_ = json.Unmarshal([]byte(c.sectionJSON), &sectionParts)
		// A prior repair may have persisted a continuation scope without the
		// original question-type heading. The immutable source identity still
		// carries that heading; restore it before subtype detection so a
		// short-answer and an argumentative question with the same number do not
		// become one source-conflict group.
		if identityParts := sectionPathFromSourceIdentity(c.identity); len(identityParts) > 0 {
			if subtype := sectionSubtype(identityParts); subtype != "" {
				if len(sectionParts) > 0 {
					replaced := false
					for i := len(sectionParts) - 1; i >= 0; i-- {
						if sectionSubtype([]string{sectionParts[i]}) != "" {
							sectionParts[i] = identityParts[len(identityParts)-1]
							replaced = true
							break
						}
					}
					if !replaced {
						sectionParts = append(sectionParts, identityParts[len(identityParts)-1])
					}
				} else {
					sectionParts = identityParts
				}
			}
		}
		if len(sectionParts) > 0 {
			encoded, _ := json.Marshal(sectionParts)
			c.sectionJSON = string(encoded)
		}
		c.subtype = candidateSectionSubtype(sectionParts, c.questionType)
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	answers := []alignmentAnswer{}
	rows2, err := db.QueryContext(ctx, `SELECT id,document_id,COALESCE(source_identity,''),COALESCE(section_path_json,'[]'),COALESCE(section_scope_id,''),set_no,source_question_no,COALESCE(question_type,''),COALESCE(canonical_question_type,''),COALESCE(answer_json,'[]'),COALESCE(analysis,''),page_start,page_end,source_kind,COALESCE(run_id,0) FROM import_answer_records WHERE session_id=? ORDER BY page_start,id`, sessionID)
	if err != nil {
		return err
	}
	for rows2.Next() {
		var a alignmentAnswer
		var rawType, canonical string
		if err := rows2.Scan(&a.id, &a.documentID, &a.identity, &a.sectionJSON, &a.sectionScope, &a.setNo, &a.qno, &rawType, &canonical, &a.answerJSON, &a.analysis, &a.pageStart, &a.pageEnd, &a.sourceKind, &a.runID); err != nil {
			rows2.Close()
			return err
		}
		a.identityExplicit = strings.TrimSpace(a.identity) != ""
		a.rawType = rawType
		a.canonicalType = canonical
		if a.canonicalType == "" {
			a.canonicalType = domain.CanonicalQuestionType(rawType)
		}
		a.canonicalType = domain.CanonicalQuestionType(a.canonicalType)
		var sectionParts []string
		_ = json.Unmarshal([]byte(a.sectionJSON), &sectionParts)
		a.subtype = answerSectionSubtype(sectionParts, a.analysis+" "+a.answerJSON)
		if a.subtype == "" {
			a.subtype = answerSubtypeFromRawType(rawType)
		}
		answers = append(answers, a)
	}
	rows2.Close()
	answerDirectory, err := s.loadAllAnswerDirectoryEntries(ctx, sessionID)
	if err != nil {
		return err
	}
	// Repair the source projection before any physical-order propagation. This
	// is the important distinction between an explicit source identity and a
	// missing heading: an answer's own section_scope_id/source_identity is never
	// allowed to be replaced by the previous page's topic.
	for i := range answers {
		a := &answers[i]
		var sectionParts []string
		_ = json.Unmarshal([]byte(a.sectionJSON), &sectionParts)
		answer := domain.VisionAnswer{
			SourceIdentity:        a.identity,
			SectionPath:           sectionParts,
			SectionScopeID:        a.sectionScope,
			SetNo:                 a.setNo,
			QuestionType:          a.rawType,
			CanonicalQuestionType: a.canonicalType,
			SourceQuestionNo:      a.qno,
			Analysis:              a.analysis,
			SourcePageStart:       a.pageStart,
			SourcePageEnd:         a.pageEnd,
		}
		if normalized, matched, ok := normalizeVisionAnswerIdentity(answer, answerDirectory[a.documentID]); ok {
			a.identity = normalized.SourceIdentity
			a.identityExplicit = strings.TrimSpace(a.identity) != ""
			a.sectionScope = normalized.SectionScopeID
			a.setNo = normalized.SetNo
			a.qno = normalized.SourceQuestionNo
			a.rawType = normalized.QuestionType
			a.canonicalType = normalized.CanonicalQuestionType
			sectionJSON, _ := json.Marshal(normalized.SectionPath)
			a.sectionJSON = string(sectionJSON)
			_ = matched
		} else if a.sectionScope == "" {
			// Keep a valid explicit path if the directory cannot prove a unique
			// answer source. Only a fully empty record may inherit page context.
			if scope := canonicalSectionScope(sectionParts); scope != "" {
				a.sectionScope = scope
			}
		}
	}
	// Models frequently emit a topic only on the first window and then emit
	// just "一、单项选择题"/"二、多项选择题" on following pages. Reconstruct
	// the active topic in physical order before building identities. This is
	// deterministic and never crosses a new explicit topic heading.
	propagateCandidateScopes(candidates)
	propagateAnswerScopes(answers)
	// A previous relink used to overwrite the answer record's scope with the
	// candidate scope. Recompute and persist the source-derived identity before
	// building indexes so every retry starts from auditable raw model output.
	answers, err = s.persistCanonicalAnswerRecords(ctx, answers)
	if err != nil {
		return err
	}
	for _, c := range candidates {
		_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET match_scope_key=? WHERE id=?`, matchScopeKey(c.sectionScope), c.id)
	}
	for _, a := range answers {
		_, _ = db.ExecContext(ctx, `UPDATE import_answer_records SET match_scope_key=? WHERE id=?`, matchScopeKey(a.sectionScope), a.id)
	}
	candidateTerminalScopes := uniqueScopeTerminals(candidates)

	byLoose := map[string][]alignmentCandidate{}
	for _, c := range candidates {
		// Persist inferred identity even when the answer is currently missing;
		// otherwise every relink would lose the section context for unresolved
		// candidates and make later answer-page retries impossible to match.
		_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET canonical_question_type=?,section_scope_id=?,section_path_json=? WHERE id=?`, c.canonicalType, c.sectionScope, c.sectionJSON, c.id)
		// Materialize the inferred scope back onto the directory row so the
		// coordinator's expected count preserves repeated question numbers.
		_, _ = db.ExecContext(ctx, `UPDATE import_directory_entries SET canonical_question_type=?,section_scope_id=?,sequence_in_scope=? WHERE session_id=? AND source_identity=?`, c.canonicalType, c.sectionScope, c.qno, sessionID, c.identity)
		byLoose[fmt.Sprintf("%d|%s|%d", c.setNo, c.canonicalType, c.qno)] = append(byLoose[fmt.Sprintf("%d|%s|%d", c.setNo, c.canonicalType, c.qno)], c)
	}
	byAnswer := map[string][]alignmentAnswer{}
	byScopeTypeQuestion := map[string][]alignmentAnswer{}
	byAnswerPath := map[string][]alignmentAnswer{}
	for _, a := range answers {
		byAnswer[alignmentKey(a.sectionScope, a.setNo, a.canonicalType, a.qno)] = append(byAnswer[alignmentKey(a.sectionScope, a.setNo, a.canonicalType, a.qno)], a)
		key := fmt.Sprintf("%s|%s|%d", a.sectionScope, a.canonicalType, a.qno)
		byScopeTypeQuestion[key] = append(byScopeTypeQuestion[key], a)
		byAnswerPath[alignmentPathKey(a.sectionJSON, a.canonicalType, a.qno)] = append(byAnswerPath[alignmentPathKey(a.sectionJSON, a.canonicalType, a.qno)], a)
	}

	// Keep the old model output, but make this run's links independently
	// auditable. Existing links are not deleted.
	_, _ = db.ExecContext(ctx, `DELETE FROM import_problems WHERE session_id=? AND status='open' AND problem_type IN ('ANSWER_CONFLICT','ANSWER_REQUIRED','ANSWER_MISSING','ANSWER_SOURCE_IDENTITY_INVALID','ANSWER_UNRESOLVED','ANSWER_INVALID','QUESTION_INVALID','QUESTION_MISSING','QUESTION_SOURCE_CONFLICT','DIRECTORY_INCOMPLETE')`, sessionID)
	// candidate_sources is a derived projection of the current links. Rebuild
	// only its answer/explanation rows so a stricter relink cannot leave a stale
	// answer thumbnail or source attached to an unresolved candidate. Raw model
	// output and historical candidate_answer_links remain untouched.
	_, _ = db.ExecContext(ctx, `DELETE FROM candidate_sources WHERE session_id=? AND source_role IN ('answer','explanation')`, sessionID)
	_, _ = db.ExecContext(ctx, `UPDATE import_answer_records SET link_status='unresolved' WHERE session_id=?`, sessionID)
	for _, c := range candidates {
		var q domain.VisionQuestion
		if json.Unmarshal([]byte(c.merged), &q) != nil {
			continue
		}
		q.QuestionType = c.canonicalType
		q.CanonicalQuestionType = c.canonicalType
		questionEvidence := strings.TrimSpace(q.Passage + " " + q.Stem)
		matches := byAnswer[alignmentKey(c.sectionScope, c.setNo, c.canonicalType, c.qno)]
		method := "canonical"
		if len(matches) == 0 {
			// Set numbers are not stable across the question and answer books:
			// the question extractor may use the printed heading ordinal while
			// the answer extractor uses the answer-table column (for example the
			// same true/false block appears as set 1 vs set 3). A full section
			// path plus type and number is a stronger identity than that number.
			pathMatches := byAnswerPath[alignmentPathKey(c.sectionJSON, c.canonicalType, c.qno)]
			if len(pathMatches) > 0 {
				matches = pathMatches
				method = "section_path"
			}
		}
		if len(matches) == 0 {
			// A few answer windows contain only the terminal topic title while
			// the question window includes its chapter prefix. Allow this
			// representation only when that terminal is unique in the question
			// document; a generic suffix match would cross chapters.
			for _, a := range answers {
				if a.canonicalType == c.canonicalType && a.qno == c.qno &&
					(a.setNo == 0 || c.setNo == 0 || a.setNo == c.setNo) &&
					scopesEquivalent(c.sectionScope, a.sectionScope, candidateTerminalScopes) {
					matches = append(matches, a)
				}
			}
			// Set ordinals are presentation metadata and can differ between
			// question and answer books. Once the terminal topic is unique,
			// allow the same scoped/type/number identity even when both sides
			// carry different non-zero set numbers. This remains bounded to a
			// unique physical topic and therefore cannot cross chapters.
			if len(matches) == 0 {
				for _, a := range answers {
					if a.canonicalType == c.canonicalType && a.qno == c.qno &&
						scopesEquivalent(c.sectionScope, a.sectionScope, candidateTerminalScopes) {
						matches = append(matches, a)
					}
				}
				if len(matches) > 0 {
					method = "section_scope_terminal_set_compatible"
				}
			}
		}
		if len(matches) == 0 {
			// Some answer sheets omit the set number. Reuse it only when the
			// scoped topic/type/question identity has one unambiguous record.
			looseSet := byScopeTypeQuestion[fmt.Sprintf("%s|%s|%d", c.sectionScope, c.canonicalType, c.qno)]
			compatible := []alignmentAnswer{}
			for _, a := range looseSet {
				if a.setNo == 0 || c.setNo == 0 || a.setNo == c.setNo {
					compatible = append(compatible, a)
				}
			}
			if len(compatible) > 0 {
				matches = compatible
				method = "canonical"
			}
		}
		if len(matches) == 0 {
			// The question and answer books may use different non-zero set
			// ordinals for the same printed block. Once the topic, canonical
			// type and number are fixed, retain the scoped evidence and let the
			// subjective subtype filter below distinguish 简答/辨析/案例 blocks.
			// Different answer values remain visible as ANSWER_CONFLICT.
			scoped := byScopeTypeQuestion[fmt.Sprintf("%s|%s|%d", c.sectionScope, c.canonicalType, c.qno)]
			if len(scoped) > 0 {
				matches = scoped
				method = "section_scope"
			}
		}
		if len(matches) == 0 {
			loose := byLoose[fmt.Sprintf("%d|%s|%d", c.setNo, c.canonicalType, c.qno)]
			looseAnswers := make([]alignmentAnswer, 0, 2)
			for _, a := range answers {
				if a.setNo == c.setNo && a.canonicalType == c.canonicalType && a.qno == c.qno {
					looseAnswers = append(looseAnswers, a)
				}
			}
			// A globally unique candidate is not enough: answer books commonly
			// repeat the same number in every topic. Only use this fallback when
			// the answer source is also unique, otherwise leave it unresolved for
			// a constrained review instead of crossing topics.
			if len(loose) == 1 && len(looseAnswers) == 1 {
				matches = looseAnswers
				method = "ordered_block"
			}
		}
		// Remove explicitly incompatible subjective subtypes before looking at
		// text evidence. Otherwise a material/case record on the same page can
		// win over a short-answer record merely because it shares generic words.
		matches = filterSubjectiveSubtype(matches, c)
		// Subjective blocks are especially prone to a wrong or missing topic
		// heading in the directory response. Before accepting the scoped
		// identity, use the question text to recover a cited law/topic and to
		// compare the answer evidence with other records carrying the same
		// printed number. This remains deterministic: a cross-scope candidate is
		// accepted only when one evidence record wins clearly; otherwise the
		// original ambiguity stays unresolved.
		if c.canonicalType == "subjective" {
			if hintedScope := citedTopicScopeFromStem(questionEvidence, answers); hintedScope != "" && !scopeContainsTopic(c.sectionScope, hintedScope) {
				c.sectionScope = hintedScope
				inferred, _ := json.Marshal(strings.Split(hintedScope, "/"))
				c.sectionJSON = string(inferred)
				matches = byAnswer[alignmentKey(c.sectionScope, c.setNo, c.canonicalType, c.qno)]
				method = "canonical"
				matches = filterSubjectiveSubtype(matches, c)
			}
			matches = expandSubjectiveEvidenceMatches(matches, answers, c, questionEvidence)
			if len(matches) == 1 && matches[0].sectionScope != "" && !scopeContainsTopic(c.sectionScope, matches[0].sectionScope) {
				// A unique evidence winner from another scope is also the best
				// deterministic correction for a question window that inherited the
				// next topic's heading. Keep the printed subjective heading (六、案例
				// 分析题/五、简答题) but replace only the topic prefix with the
				// answer-book's source-derived path.
				// Keep the original canonical scope for the global identity index.
				// Replacing it with an answer-book prefix would make the terminal
				// topic appear twice in the candidate set and disable the safe
				// unique-terminal fallback for neighbouring objective questions. The
				// corrected full path is persisted for display; the selected answer
				// source is already captured by the link table.
				_, c.sectionJSON = mergeSubjectiveEvidenceScope(c, matches[0])
			}
		}
		// Select evidence before preferring an explicit answer-source row. An
		// answer record can be a mislabelled copy of an unrelated subjective
		// question, while the explanation record contains the actual case text.
		// Filtering by source_kind first would discard that stronger evidence.
		matches = selectSubjectiveEvidenceMatch(matches, questionEvidence)
		matches = preferLatestAnswerRecords(matches)
		valid := []alignmentAnswer{}
		values := map[string]bool{}
		// Overlapping answer windows and the separate answer/explanation
		// extractors can yield the same subjective reference more than once.
		// Collapse exact evidence duplicates (keeping the richest analysis) so
		// duplicate sources do not masquerade as an unresolved conflict.
		subjectiveEvidence := map[string]alignmentAnswer{}
		for _, a := range matches {
			vals := normalizedAnswerValues(a.answerJSON, c.canonicalType)
			if c.canonicalType == "subjective" {
				vals, a.analysis = normalizeSubjectiveSource(vals, a.analysis)
				normalizedJSON, _ := json.Marshal(vals)
				a.answerJSON = string(normalizedJSON)
			}
			if len(vals) == 0 {
				if c.canonicalType == "subjective" && strings.TrimSpace(a.analysis) != "" {
					key := "text:" + normalizeSubjectiveEvidence(a.analysis)
					if previous, exists := subjectiveEvidence[key]; !exists || len(a.analysis) > len(previous.analysis) {
						subjectiveEvidence[key] = a
					}
				}
				continue
			}
			key := strings.Join(vals, ",")
			values[key] = true
			if c.canonicalType == "subjective" {
				evidenceKey := "answer:" + key
				if previous, exists := subjectiveEvidence[evidenceKey]; !exists || len(a.analysis) > len(previous.analysis) {
					subjectiveEvidence[evidenceKey] = a
				}
			} else {
				valid = append(valid, a)
			}
		}
		if c.canonicalType == "subjective" {
			for _, evidence := range subjectiveEvidence {
				valid = append(valid, evidence)
			}
		}
		// Subjective/material/case answers may be stored as an analysis-only
		// record. They are still a valid source, although there is no option
		// letter to put in question.answer. Choice questions require a real,
		// non-empty answer value and remain blocked otherwise.
		subjectiveSource := c.canonicalType == "subjective" && len(valid) == 1
		if (len(values) == 1 && len(valid) > 0) || subjectiveSource {
			a := valid[0]
			vals := normalizedAnswerValues(a.answerJSON, c.canonicalType)
			q.Answer = vals
			if a.analysis != "" {
				q.Analysis = a.analysis
			}
			merged, _ := json.Marshal(q)
			normalizedAnswerJSON, _ := json.Marshal(vals)
			for _, evidence := range valid {
				role := evidence.sourceKind
				if role != "explanation" {
					role = "answer"
				}
				contentHash := hashString(fmt.Sprintf("%s|%s|%d|%d|%s", c.identity, role, evidence.pageStart, evidence.pageEnd, evidence.answerJSON))
				_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,page_start,page_end,regions_json,agent_run_id,content_hash,extraction_confidence,status) SELECT o.id,?,?,?,COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,''),?,?,'[]',?,?,?,'active' FROM source_question_origins o JOIN documents d ON d.id=? LEFT JOIN import_session_documents sd ON sd.session_id=o.session_id AND sd.document_id=d.id WHERE o.session_id=? AND o.candidate_id=?`, c.id, role, evidence.documentID, evidence.pageStart, evidence.pageEnd, nullableRun(runID), contentHash, confidenceForMethod(method), evidence.documentID, sessionID, c.id)
			}
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,answer_json=?,answer_page_start=?,answer_page_end=?,canonical_question_type=?,section_scope_id=?,answer_link_status='confirmed',answer_link_method=?,answer_link_confidence=?,review_decision=CASE WHEN review_decision='exclude' THEN review_decision WHEN EXISTS(SELECT 1 FROM source_question_origins o JOIN source_evidence_spans q ON q.origin_id=o.id AND q.source_role='question' AND q.status='active' JOIN source_evidence_spans a ON a.origin_id=o.id AND a.source_role IN ('answer','explanation') AND a.status='active' WHERE o.candidate_id=candidate_questions.id) THEN 'include' ELSE 'pending' END,validation_issues_json='[]',updated_at=datetime('now') WHERE id=?`, string(merged), string(merged), string(normalizedAnswerJSON), a.pageStart, a.pageEnd, c.canonicalType, c.sectionScope, method, confidenceForMethod(method), c.id)
			// Keep the answer record's source-derived section scope intact. The
			// candidate-to-answer relationship is represented by the link table;
			// mutating the source row would corrupt later relink attempts.
			_, _ = db.ExecContext(ctx, `UPDATE import_answer_records SET link_status='linked' WHERE id=?`, a.id)
			for _, link := range valid {
				detail, _ := json.Marshal(map[string]interface{}{"identity": alignmentKey(c.sectionScope, c.setNo, c.canonicalType, c.qno)})
				_, _ = db.ExecContext(ctx, `INSERT INTO candidate_answer_links(session_id,candidate_id,answer_record_id,status,method,confidence,detail_json,run_id) SELECT ?,?,?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM candidate_answer_links WHERE candidate_id=? AND answer_record_id=? AND status='confirmed' AND (?=0 OR run_id=?))`, sessionID, c.id, link.id, domain.AnswerLinkConfirmed, method, confidenceForMethod(method), string(detail), nullableRun(runID), c.id, link.id, runID, nullableRun(runID))
				_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_sources(session_id,candidate_id,document_id,source_role,page_start,page_end,source_json) VALUES(?,?,?,?,?,?,?)`, sessionID, c.id, link.documentID, link.sourceKind, link.pageStart, link.pageEnd, link.answerJSON)
			}
		} else if c.reviewDecision != domain.ReviewExclude && c.answerLinkStatus == domain.AnswerLinkConfirmed && c.answerLinkMethod == "candidate_review" {
			// The answer was written by an identity-checked candidate-review
			// vision patch bound to concrete answer pages. The deterministic
			// matcher above found no record for it, but erasing the confirmed
			// answer here would undo the repair the same run just applied.
			// The raw-extraction replay in this relink may also have rewritten
			// merged_json without the patched answer, so restore it from the
			// durable answer_json column, record the answer provenance span and
			// let the validation closeout recompute status and review_decision.
			var saved domain.VisionAnswer
			var savedValues []string
			if json.Unmarshal([]byte(c.answerJSON), &saved) != nil {
				// Older projections stored the bare answer values instead of a
				// VisionAnswer object.
				_ = json.Unmarshal([]byte(c.answerJSON), &savedValues)
			} else {
				savedValues = saved.Answer
			}
			if len(savedValues) > 0 {
				q.Answer = savedValues
				if strings.TrimSpace(q.Analysis) == "" {
					q.Analysis = saved.Analysis
				}
				merged, _ := json.Marshal(q)
				_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,updated_at=datetime('now') WHERE id=?`, string(merged), string(merged), c.id)
				if c.answerPageStart > 0 && c.answerPageEnd >= c.answerPageStart {
					answerValues, _ := json.Marshal(q.Answer)
					contentHash := hashString(fmt.Sprintf("%s|%s|%d|%d|%s", c.identity, "answer", c.answerPageStart, c.answerPageEnd, string(answerValues)))
					_, _ = db.ExecContext(ctx, `INSERT OR IGNORE INTO source_evidence_spans(origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,page_start,page_end,regions_json,agent_run_id,content_hash,extraction_confidence,status) SELECT o.id,?,'answer',?,COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,''),?,?,'[]',?,?,1,'active' FROM source_question_origins o JOIN documents d ON d.id=? LEFT JOIN import_session_documents sd ON sd.session_id=o.session_id AND sd.document_id=d.id WHERE o.session_id=? AND o.candidate_id=?`, c.id, c.documentID, c.answerPageStart, c.answerPageEnd, nullableRun(runID), contentHash, c.documentID, sessionID, c.id)
				}
			}
		} else {
			// Remove any answer inherited from an older link attempt. The draft
			// must never display a stale key while the current source is unresolved.
			status := domain.AnswerLinkUnresolved
			q.Answer = nil
			merged, _ := json.Marshal(q)
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET merged_json=?,extracted_json=?,answer_json='[]',answer_page_start=NULL,answer_page_end=NULL,answer_link_status=?,answer_link_method=NULL,answer_link_confidence=NULL,review_decision='pending',updated_at=datetime('now') WHERE id=?`, string(merged), string(merged), status, c.id)
			problemType := answerProblemType(c, matches, answers, answerDirectory[c.documentID])
			if len(values) > 1 {
				problemType = "ANSWER_CONFLICT"
			}
			severity := "blocking"
			// A subjective question may legitimately have no selectable answer.
			// If its answer/explanation source exists but cannot be uniquely
			// linked, keep it pending for human review without treating it as a
			// missing choice answer. Conflicts remain blocking because selecting
			// between different references would be unsafe.
			if problemType == "ANSWER_UNRESOLVED" && c.canonicalType == "subjective" && len(matches) > 0 {
				for _, match := range matches {
					if strings.TrimSpace(match.analysis) != "" {
						severity = "warning"
						break
					}
				}
			}
			detail, _ := json.Marshal(map[string]interface{}{"identity": alignmentKey(c.sectionScope, c.setNo, c.canonicalType, c.qno), "candidate_id": c.id, "answer_count": len(matches)})
			_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,candidate_id,source_identity,detail_json) VALUES(?,?,?,?,?,?,?,?)`, sessionID, nullableRun(runID), c.documentID, problemType, severity, c.id, c.identity, string(detail))
		}
		issues := validateVisionQuestion(q)
		if requiresAnswer(c.canonicalType) && len(q.Answer) == 0 {
			issues = append(issues, issue("ANSWER_REQUIRED", q.SourcePageStart, q.SourcePageEnd, "选择题或判断题缺少明确答案"))
		}
		// Relinking clears the derived QUESTION_INVALID rows before rebuilding
		// answer links. Recreate structural question problems here as well;
		// otherwise a candidate can remain pending because of duplicate/empty
		// options while the session's blocking counter omits that issue. Missing
		// answers already have a dedicated ANSWER_* problem and must not be
		// counted twice as QUESTION_INVALID.
		for _, item := range issues {
			if !isStructuralQuestionIssue(item) {
				continue
			}
			detail, _ := json.Marshal(item)
			_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,candidate_id,source_identity,detail_json) VALUES(?,?,?,?,?,?,?,?)`, sessionID, nullableRun(runID), c.documentID, "QUESTION_INVALID", "blocking", c.id, c.identity, string(detail))
		}
		for _, item := range issues {
			code, _ := item["code"].(string)
			if code != "ANSWER_INVALID" {
				continue
			}
			detail, _ := json.Marshal(item)
			_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,candidate_id,source_identity,detail_json,resolution_hint) SELECT ?,?,?,?,?,?,?,?,'retry_answer' WHERE NOT EXISTS (SELECT 1 FROM import_problems WHERE session_id=? AND candidate_id=? AND status='open' AND problem_type='ANSWER_INVALID' AND detail_json=?)`, sessionID, nullableRun(runID), c.documentID, "ANSWER_INVALID", "blocking", c.id, c.identity, string(detail), sessionID, c.id, string(detail))
		}
		if len(issues) == 0 && q.Stem != "" && q.CanonicalQuestionType != "" {
			raw, _ := json.Marshal(issues)
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET status='validated',validation_issues_json=?,review_decision=CASE WHEN answer_link_status='confirmed' AND EXISTS(SELECT 1 FROM source_question_origins o JOIN source_evidence_spans q ON q.origin_id=o.id AND q.source_role='question' AND q.status='active' JOIN source_evidence_spans a ON a.origin_id=o.id AND a.source_role IN ('answer','explanation') AND a.status='active' WHERE o.candidate_id=candidate_questions.id) THEN 'include' ELSE 'pending' END WHERE id=?`, string(raw), c.id)
		} else {
			raw, _ := json.Marshal(issues)
			_, _ = db.ExecContext(ctx, `UPDATE candidate_questions SET status='needs_review',validation_issues_json=?,review_decision='pending' WHERE id=?`, string(raw), c.id)
		}
	}
	return s.refreshSessionReviewCounts(ctx, sessionID, runID)
}

// normalizeMixedCaseStudyCandidates uses explicit per-item evidence instead
// of assigning one broad section type to a heterogeneous case block.
func (s *ImportService) normalizeMixedCaseStudyCandidates(ctx context.Context, sessionID int64) error {
	rows, err := s.repo.DB().QueryContext(ctx, `SELECT id,COALESCE(question_json,merged_json,'{}'),COALESCE(section_scope_id,'') FROM candidate_questions WHERE import_session_id=? AND canonical_question_type='case_study'`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw, scope string
		if err := rows.Scan(&id, &raw, &scope); err != nil {
			return err
		}
		var q domain.VisionQuestion
		if json.Unmarshal([]byte(raw), &q) != nil {
			continue
		}
		canonical := ""
		stem := compactText(q.Stem)
		switch {
		case len(q.Options) >= 2 && strings.Contains(stem, "单项选择"):
			canonical = "single_choice"
		case len(q.Options) >= 2 && strings.Contains(stem, "多项选择"):
			canonical = "multiple_choice"
		case len(q.Options) == 0:
			canonical = "subjective"
		}
		if canonical == "" {
			continue
		}
		q.QuestionType, q.CanonicalQuestionType = canonical, canonical
		normalized, _ := json.Marshal(q)
		_, err = s.repo.DB().ExecContext(ctx, `UPDATE candidate_questions SET question_type=?,canonical_question_type=?,question_json=?,extracted_json=?,merged_json=?,match_scope_key=?,updated_at=datetime('now') WHERE id=?`, canonical, canonical, string(normalized), string(normalized), string(normalized), matchScopeKey(scope), id)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func isStructuralQuestionIssue(item map[string]interface{}) bool {
	code, _ := item["code"].(string)
	switch code {
	case "STEM_REQUIRED", "OPTIONS_REQUIRED", "OPTIONS_INVALID":
		return true
	default:
		return false
	}
}

// persistCanonicalAnswerRecords updates the durable answer projection after
// identity repair. Equal evidence with the same immutable source identity is
// merged into one canonical row and its historical links are moved to the
// winner. Distinct source identities are always retained, even when their
// printed question number and page overlap.
// Raw import_agent_units and import_runs are intentionally untouched.
func (s *ImportService) persistCanonicalAnswerRecords(ctx context.Context, answers []alignmentAnswer) ([]alignmentAnswer, error) {
	groups := map[string][]int{}
	order := make([]string, 0, len(answers))
	for i, a := range answers {
		storageScope := a.sectionScope
		if storageScope == "" {
			var parts []string
			_ = json.Unmarshal([]byte(a.sectionJSON), &parts)
			storageScope = canonicalSectionScope(parts)
		}
		key := fmt.Sprintf("%d|%s|%s", a.documentID, a.sourceKind, a.identity)
		if !a.identityExplicit {
			key = fmt.Sprintf("%d|%s|%d|%s|%d|%s|%d", a.documentID, storageScope, a.setNo, a.canonicalType, a.qno, a.sourceKind, a.pageStart)
		}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	canonical := make([]alignmentAnswer, 0, len(order))
	for _, key := range order {
		indices := groups[key]
		winnerIndex := indices[0]
		for _, index := range indices[1:] {
			winner := answers[winnerIndex]
			candidate := answers[index]
			if candidate.identityExplicit && !winner.identityExplicit {
				winnerIndex = index
				continue
			}
			if candidate.identityExplicit == winner.identityExplicit && candidate.runID > winner.runID {
				winnerIndex = index
				continue
			}
			if candidate.identityExplicit == winner.identityExplicit && candidate.runID == winner.runID && candidate.id > winner.id {
				winnerIndex = index
			}
		}

		winner := answers[winnerIndex]
		for _, index := range indices {
			if index == winnerIndex {
				continue
			}
			if !sameAnswerEvidence(winner, answers[index]) {
				// Different values are deliberately retained as separate in-memory
				// evidence so the linker emits ANSWER_CONFLICT.
				continue
			}
			if len(answers[index].analysis) > len(winner.analysis) {
				winner.analysis = answers[index].analysis
			}
			if answers[index].pageEnd > winner.pageEnd {
				winner.pageEnd = answers[index].pageEnd
			}
			if err := mergeAnswerRecordLinks(ctx, s.repo.DB(), answers[index].id, winner.id); err != nil {
				return nil, err
			}
			if _, err := s.repo.DB().ExecContext(ctx, `DELETE FROM import_answer_records WHERE id=?`, answers[index].id); err != nil {
				return nil, err
			}
		}
		if err := updateCanonicalAnswerRecord(ctx, s.repo.DB(), winner); err != nil {
			return nil, err
		}
		canonical = append(canonical, winner)
		// If conflicting evidence shared the same legacy key, keep it in the
		// linker input. It will be reported instead of silently discarded.
		for _, index := range indices {
			if index != winnerIndex && !sameAnswerEvidence(winner, answers[index]) {
				if err := updateCanonicalAnswerRecord(ctx, s.repo.DB(), answers[index]); err != nil {
					// A conflicting row may still use the old scope and therefore
					// cannot be rewritten into the winner's unique key. Preserve it.
					continue
				}
				canonical = append(canonical, answers[index])
			}
		}
	}
	return canonical, nil
}

func sameAnswerEvidence(left, right alignmentAnswer) bool {
	leftValues := normalizedAnswerValues(left.answerJSON, left.canonicalType)
	rightValues := normalizedAnswerValues(right.answerJSON, right.canonicalType)
	if strings.Join(leftValues, "\x00") != strings.Join(rightValues, "\x00") {
		return false
	}
	if left.canonicalType != "subjective" {
		return true
	}
	if len(leftValues) > 0 || len(rightValues) > 0 {
		return strings.Join(leftValues, "\x00") == strings.Join(rightValues, "\x00")
	}
	// A subjective source identity identifies one printed reference answer;
	// overlapping windows often return the same answer with different amounts
	// of surrounding analysis. Merge those rows and keep the richer analysis.
	return true
}

func updateCanonicalAnswerRecord(ctx context.Context, db *sql.DB, a alignmentAnswer) error {
	storageScope := a.sectionScope
	if storageScope == "" {
		var parts []string
		_ = json.Unmarshal([]byte(a.sectionJSON), &parts)
		storageScope = canonicalSectionScope(parts)
	}
	_, err := db.ExecContext(ctx, `UPDATE import_answer_records
		SET source_identity=?,question_type=?,canonical_question_type=?,section_scope_id=?,normalized_section_path=?,section_path_json=?,set_no=?,source_question_no=?,page_end=?
		WHERE id=?`, a.identity, a.rawType, a.canonicalType, storageScope, storageScope, a.sectionJSON, a.setNo, a.qno, a.pageEnd, a.id)
	return err
}

func mergeAnswerRecordLinks(ctx context.Context, db *sql.DB, fromID, toID int64) error {
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO candidate_answer_links(session_id,candidate_id,answer_record_id,status,method,confidence,detail_json,run_id)
		SELECT session_id,candidate_id,?,status,method,confidence,detail_json,run_id
		FROM candidate_answer_links WHERE answer_record_id=?`, toID, fromID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DELETE FROM candidate_answer_links WHERE answer_record_id=?`, fromID)
	return err
}

func uniqueScopeTerminals(candidates []alignmentCandidate) map[string]bool {
	seen := map[string]map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate.sectionScope == "" {
			continue
		}
		parts := strings.Split(candidate.sectionScope, "/")
		terminal := parts[len(parts)-1]
		if seen[terminal] == nil {
			seen[terminal] = map[string]struct{}{}
		}
		seen[terminal][candidate.sectionScope] = struct{}{}
	}
	unique := map[string]bool{}
	for terminal, scopes := range seen {
		unique[terminal] = len(scopes) == 1
	}
	return unique
}

func answerProblemType(candidate alignmentCandidate, matches, answers []alignmentAnswer, directory []domain.VisionDirectoryEntry) string {
	if len(matches) > 0 {
		return "ANSWER_UNRESOLVED"
	}
	answerEvidence := false
	for _, answer := range answers {
		if answer.documentID != candidate.documentID || answer.qno != candidate.qno || answer.canonicalType != candidate.canonicalType || !answerSetCompatible(answer.setNo, candidate.setNo) {
			continue
		}
		answerEvidence = true
		break
	}
	answerDirectory := false
	for _, entry := range directory {
		entryType := domain.CanonicalQuestionType(entry.CanonicalQuestionType)
		if entryType == "" {
			entryType = domain.CanonicalQuestionType(entry.QuestionType)
		}
		if entry.SourceQuestionNo == candidate.qno && entryType == candidate.canonicalType && answerSetCompatible(entry.SetNo, candidate.setNo) {
			answerDirectory = true
			break
		}
	}
	if answerEvidence {
		return "ANSWER_SOURCE_IDENTITY_INVALID"
	}
	if answerDirectory {
		return "ANSWER_MISSING"
	}
	return "ANSWER_UNRESOLVED"
}

func scopesEquivalent(candidate, answer string, uniqueTerminals map[string]bool) bool {
	candidate = matchScopeKey(candidate)
	answer = matchScopeKey(answer)
	if candidate == answer {
		return true
	}
	if candidate == "" || answer == "" {
		return false
	}
	candidateParts := strings.Split(candidate, "/")
	answerParts := strings.Split(answer, "/")
	if len(candidateParts) == 1 && len(answerParts) > 1 {
		return candidateParts[0] == answerParts[len(answerParts)-1] && uniqueTerminals[candidateParts[0]]
	}
	if len(answerParts) == 1 && len(candidateParts) > 1 {
		return answerParts[0] == candidateParts[len(candidateParts)-1] && uniqueTerminals[answerParts[0]]
	}
	return false
}

// citedTopicScopeFromStem repairs the common case where a question window
// inherited the next page's topic heading. Legal short-answer questions often
// name the governing law verbatim, which is a stronger topic signal than a
// model-generated heading. We resolve the cited title only against scopes that
// already exist in the answer directory; no new identity is invented.
func citedTopicScopeFromStem(stem string, answers []alignmentAnswer) string {
	text := compactText(stem)
	if text == "" {
		return ""
	}
	titles := []string{
		"中华人民共和国教育法",
		"中华人民共和国义务教育法",
		"中华人民共和国教师法",
		"中华人民共和国未成年人保护法",
		"中华人民共和国预防未成年人犯罪法",
		"学生伤害事故处理办法",
		"中小学教师职业道德规范",
	}
	for _, title := range titles {
		if !strings.Contains(text, compactText(title)) {
			continue
		}
		candidates := map[string]struct{}{}
		for _, answer := range answers {
			if answer.sectionScope == "" || !scopeContainsTitle(answer.sectionScope, title) {
				continue
			}
			candidates[answer.sectionScope] = struct{}{}
		}
		if len(candidates) == 1 {
			for scope := range candidates {
				return scope
			}
		}
	}
	return ""
}

func scopeContainsTopic(scope, hinted string) bool {
	if scope == "" || hinted == "" {
		return false
	}
	return compactText(scope) == compactText(hinted)
}

func scopeContainsTitle(scope, title string) bool {
	needle := compactText(title)
	if needle == "" {
		return false
	}
	for _, part := range strings.Split(scope, "/") {
		value := compactText(part)
		if value == needle {
			return true
		}
		// Do not let 教育法 match 义务教育法; the latter is a different
		// document/topic and was the source of the original answer drift.
		if strings.Contains(value, needle) && !(needle == compactText("中华人民共和国教育法") && strings.Contains(value, compactText("义务教育法"))) {
			return true
		}
	}
	return false
}

func mergeSubjectiveEvidenceScope(candidate alignmentCandidate, answer alignmentAnswer) (string, string) {
	var candidateParts, answerParts []string
	_ = json.Unmarshal([]byte(candidate.sectionJSON), &candidateParts)
	_ = json.Unmarshal([]byte(answer.sectionJSON), &answerParts)
	heading := ""
	for i := len(candidateParts) - 1; i >= 0; i-- {
		if sectionSubtype([]string{candidateParts[i]}) != "" {
			heading = candidateParts[i]
			break
		}
	}
	path := make([]string, 0, len(answerParts)+1)
	for _, part := range answerParts {
		if sectionSubtype([]string{part}) == "" && compactText(part) != "" {
			path = append(path, part)
		}
	}
	if len(path) == 0 && answer.sectionScope != "" {
		path = strings.Split(answer.sectionScope, "/")
	}
	if heading != "" {
		path = append(path, heading)
	}
	encoded, _ := json.Marshal(path)
	return canonicalSectionScope(path), string(encoded)
}

// sectionSubtype preserves the useful distinction between subjective-style
// headings after their canonical question type is normalized to "subjective".
// It is deliberately a narrow discriminator: it is only used when both sides
// explicitly name a subtype, never as a broad topic match.
func sectionSubtype(parts []string) string {
	for i := len(parts) - 1; i >= 0; i-- {
		v := compactText(parts[i])
		v = strings.TrimSuffix(v, "(参考答案)")
		v = strings.TrimSuffix(v, "（参考答案）")
		switch {
		case strings.Contains(v, "案例分析"), strings.Contains(v, "材料分析"):
			return "case_analysis"
		case strings.Contains(v, "辨析"):
			return "discrimination"
		case strings.Contains(v, "论述"):
			return "argumentative"
		case strings.Contains(v, "简答"):
			return "short_answer"
		}
	}
	return ""
}

func candidateSectionSubtype(parts []string, rawType string) string {
	if subtype := sectionSubtype(parts); subtype != "" {
		return subtype
	}
	v := strings.ToLower(strings.TrimSpace(rawType))
	switch {
	case strings.Contains(v, "case"), strings.Contains(v, "material"), strings.Contains(v, "reading"):
		return "case_analysis"
	case strings.Contains(v, "short"), v == "qa", v == "question_answer":
		return "short_answer"
	case strings.Contains(v, "discrimin"), strings.Contains(v, "analysis") && strings.Contains(v, "statement"):
		return "discrimination"
	default:
		return ""
	}
}

// sectionSubtypeKey retains the printed block ordinal when a topic contains
// more than one subjective block (for example "六、案例分析题" and
// "七、案例分析题"). The canonical type remains subjective for validation,
// but the ordinal is part of source identity so distinct blocks with the same
// printed number are not reported as conflicting duplicates.
func sectionSubtypeKey(parts []string, rawType string) string {
	subtype := candidateSectionSubtype(parts, rawType)
	if subtype == "" {
		return ""
	}
	for i := len(parts) - 1; i >= 0; i-- {
		value := compactText(parts[i])
		if sectionSubtype([]string{parts[i]}) == "" {
			continue
		}
		prefix := strings.TrimSpace(value)
		if index := strings.IndexAny(prefix, "、.:"); index > 0 {
			prefix = prefix[:index]
		}
		if prefix != "" {
			return subtype + ":" + prefix
		}
	}
	return subtype
}

func answerSectionSubtype(parts []string, analysis string) string {
	if subtype := sectionSubtype(parts); subtype != "" {
		return subtype
	}
	// A few answer windows omit their local heading. These phrases are
	// structural markers in the printed reference answer, not content-based
	// answer guesses: discrimination answers start by judging a proposition,
	// while case answers discuss the material/case itself.
	text := strings.TrimSpace(analysis)
	if strings.Contains(text, "这种说法") || strings.Contains(text, "说法是") {
		return "discrimination"
	}
	if strings.Contains(text, "本案例") || strings.Contains(text, "案例中") || strings.Contains(text, "材料中") || strings.Contains(text, "根据案例") || strings.Contains(text, "本案") {
		return "case_analysis"
	}
	return ""
}

// answerSubtypeFromRawType recovers the printed answer block when the model
// omitted its section heading. Answer extraction often calls every subjective
// block "qa" or "subjective", but keeps a more useful raw value for case,
// material, short-answer and argumentative records. This is only a narrow
// type filter; it never invents an answer or crosses a topic scope.
func answerSubtypeFromRawType(rawType string) string {
	v := strings.ToLower(strings.TrimSpace(rawType))
	switch {
	case strings.Contains(v, "case"), strings.Contains(v, "material"), strings.Contains(v, "reading"):
		return "case_analysis"
	case strings.Contains(v, "short"), v == "question_answer":
		return "short_answer"
	case strings.Contains(v, "discrimin"), strings.Contains(v, "statement"):
		return "discrimination"
	default:
		return ""
	}
}

func preferLatestAnswerRecords(matches []alignmentAnswer) []alignmentAnswer {
	if len(matches) < 2 {
		return matches
	}
	latest := int64(0)
	for _, match := range matches {
		if match.runID > latest {
			latest = match.runID
		}
	}
	if latest == 0 {
		return matches
	}
	filtered := make([]alignmentAnswer, 0, len(matches))
	for _, match := range matches {
		if match.runID == latest {
			filtered = append(filtered, match)
		}
	}
	// A repair may contain both an answer-source row and an explanation row
	// for the same identity. Prefer the explicit answer-source row when it is
	// present; the explanation remains in the audit table.
	for _, match := range filtered {
		if match.sourceKind == "answer" {
			answersOnly := make([]alignmentAnswer, 0, len(filtered))
			for _, item := range filtered {
				if item.sourceKind == "answer" {
					answersOnly = append(answersOnly, item)
				}
			}
			return answersOnly
		}
	}
	return filtered
}

// expandSubjectiveEvidenceMatches adds same-number subjective records from
// other scopes only for an evidence-based decision. The old identity-only
// matcher could lock a question to a wrong scope (for example a case question
// on a page that also starts the next topic). We still require a single,
// sufficiently strong winner, so a generic stem never causes a speculative
// cross-topic link.
func expandSubjectiveEvidenceMatches(matches, answers []alignmentAnswer, candidate alignmentCandidate, stem string) []alignmentAnswer {
	if strings.TrimSpace(stem) == "" {
		return matches
	}
	// Once the scoped/terminal identity has produced a candidate, keep that
	// bounded set. Subjective answers repeat the same printed number across
	// topics, so widening a non-empty scoped set can replace a valid same-topic
	// reference with an unrelated answer merely because generic words such as
	// "教师" or "案例" overlap. Cross-topic evidence is only safe when the
	// identity lookup found no usable answer at all.
	if len(matches) > 0 {
		return matches
	}
	// A scoped record with meaningful evidence is safer than any cross-topic
	// candidate. Only widen the pool when the scoped evidence is weak or
	// absent; this prevents a generic word such as “发展” from replacing an
	// already-correct answer in the current topic.
	if _, ok, best, _ := subjectiveEvidenceWinner(filterSubjectiveSubtype(matches, candidate), stem); ok && best >= 36 {
		return matches
	}
	pool := append([]alignmentAnswer(nil), matches...)
	seen := map[int64]bool{}
	for _, match := range pool {
		seen[match.id] = true
	}
	for _, answer := range answers {
		if seen[answer.id] || answer.canonicalType != "subjective" || answer.qno != candidate.qno {
			continue
		}
		if answer.setNo != 0 && candidate.setNo != 0 && answer.setNo != candidate.setNo {
			continue
		}
		pool = append(pool, answer)
		seen[answer.id] = true
	}
	pool = filterSubjectiveSubtype(pool, candidate)
	if winner, ok, best, second := subjectiveEvidenceWinner(pool, stem); ok && best >= 36 && best > second {
		return []alignmentAnswer{winner}
	}
	return matches
}

func selectSubjectiveEvidenceMatch(matches []alignmentAnswer, stem string) []alignmentAnswer {
	if len(matches) < 2 || strings.TrimSpace(stem) == "" {
		return matches
	}
	if winner, ok, _, _ := subjectiveEvidenceWinner(matches, stem); ok {
		return []alignmentAnswer{winner}
	}
	return matches
}

func subjectiveEvidenceWinner(matches []alignmentAnswer, stem string) (alignmentAnswer, bool, int, int) {
	if len(matches) == 0 || strings.TrimSpace(stem) == "" {
		return alignmentAnswer{}, false, 0, 0
	}
	stemRunes := []rune(evidenceText(stem))
	if len(stemRunes) < 3 {
		return alignmentAnswer{}, false, 0, 0
	}
	type scored struct {
		match alignmentAnswer
		score int
	}
	scores := make([]scored, 0, len(matches))
	for _, match := range matches {
		text := match.analysis
		if values := normalizedAnswerValues(match.answerJSON, "subjective"); len(values) > 0 {
			text += " " + strings.Join(values, " ")
		}
		textRunes := []rune(evidenceText(text))
		score := 0
		for n := 6; n >= 3; n-- {
			if len(stemRunes) < n {
				continue
			}
			seen := map[string]struct{}{}
			for i := 0; i+n <= len(stemRunes); i++ {
				gram := string(stemRunes[i : i+n])
				if _, exists := seen[gram]; exists || isGenericEvidenceGram(gram) {
					continue
				}
				seen[gram] = struct{}{}
				if strings.Contains(string(textRunes), gram) {
					score += n * n
				}
			}
		}
		scores = append(scores, scored{match, score})
	}
	best, second := 0, 0
	bestIndex := -1
	for i, item := range scores {
		if item.score > best {
			second = best
			best = item.score
			bestIndex = i
		} else if item.score > second {
			second = item.score
		}
	}
	if bestIndex >= 0 && best > 0 && best > second {
		return scores[bestIndex].match, true, best, second
	}
	return alignmentAnswer{}, false, best, second
}

func evidenceText(value string) string {
	value = compactText(value)
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
}

func isGenericEvidenceGram(value string) bool {
	// Printed question numbers and punctuation are layout artefacts, not
	// semantic evidence. Ignoring them prevents a neighbouring "3." record
	// from winning a cross-scope comparison.
	if strings.ContainsAny(value, "0123456789.、:：()（）") {
		return true
	}
	for _, generic := range []string{"根据", "规定", "教师", "学生", "教育", "案例", "问题", "应当", "可以", "具有", "基本", "要求", "简述", "课堂", "教学", "行为", "作用", "内容", "分析", "方式", "观点", "核心", "素养", "发展", "新课程"} {
		if strings.Contains(value, generic) && len([]rune(value)) <= 3 {
			return true
		}
	}
	for _, generic := range []string{"基本要求", "课堂教学", "简述", "教学观点", "新课程改革", "课堂", "教学", "提问", "口语", "基本", "要求", "老师", "提问的基本", "口语的基本"} {
		if strings.Contains(value, generic) {
			return true
		}
	}
	return false
}

func filterSubjectiveSubtype(matches []alignmentAnswer, candidate alignmentCandidate) []alignmentAnswer {
	if candidate.canonicalType != "subjective" || candidate.subtype == "" || len(matches) < 2 {
		return matches
	}
	filtered := make([]alignmentAnswer, 0, len(matches))
	unknown := make([]alignmentAnswer, 0, len(matches))
	for _, match := range matches {
		if match.subtype == candidate.subtype {
			filtered = append(filtered, match)
		} else if match.subtype == "" {
			unknown = append(unknown, match)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	// Keep untyped records only when no explicitly incompatible block was
	// available. This permits a model-mislabelled essay record to satisfy a
	// short-answer/argumentative candidate while excluding a typed case or
	// discrimination record with the same number.
	if len(unknown) > 0 {
		return unknown
	}
	return matches
}

// normalizedAnswerValues converts the glyphs commonly used in answer sheets
// into the canonical values accepted by validation. Keeping the raw answer
// record untouched preserves auditability while the candidate gets a stable
// answer suitable for grading and display.
func normalizedAnswerValues(raw, questionType string) []string {
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	for i, value := range values {
		value = strings.TrimSpace(value)
		if questionType == "true_false" {
			switch strings.ToLower(value) {
			case "√", "✓", "✔", "对", "t", "v", "true", "正确":
				value = "正确"
			case "×", "✕", "✗", "x", "f", "错", "false", "错误":
				value = "错误"
			}
		}
		values[i] = value
	}
	return values
}

func normalizeSubjectiveSource(values []string, analysis string) ([]string, string) {
	rubric := make([]string, 0, len(values))
	text := make([]string, 0, len(values))
	for _, value := range values {
		compact := strings.ToUpper(strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '、', ',', '，', '/', '\\', '+', '&':
				return -1
			default:
				return r
			}
		}, value))
		if compact != "" && len(compact) <= 6 {
			validRubric := true
			for _, r := range compact {
				if r < 'A' || r > 'F' {
					validRubric = false
					break
				}
			}
			if validRubric {
				for _, r := range compact {
					rubric = append(rubric, string(r))
				}
				continue
			}
		}
		if strings.TrimSpace(value) != "" {
			text = append(text, strings.TrimSpace(value))
		}
	}
	if len(text) > 0 {
		if strings.TrimSpace(analysis) != "" {
			analysis = strings.Join(append(text, analysis), "\n")
		} else {
			analysis = strings.Join(text, "\n")
		}
	}
	return rubric, analysis
}

func normalizeSubjectiveEvidence(value string) string {
	value = strings.TrimSpace(value)
	// v1.1 answer records often retain the printed question number while the
	// older extractor does not ("1.(1)..." vs "(1)..."). Remove only that
	// leading label; the body remains source evidence and is never synthesized.
	value = regexp.MustCompile(`^[0-9一二三四五六七八九十]+[.、:：]?\s*`).ReplaceAllString(value, "")
	normalized := compactText(value)
	// Duplicate v1/v1.1 records may differ only because one extractor stopped
	// at the next page. A stable prefix is enough to identify that same source,
	// while retaining enough content to distinguish adjacent answers with the
	// same printed number.
	runes := []rune(normalized)
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return string(runes)
}

func propagateCandidateScopes(candidates []alignmentCandidate) {
	active := ""
	for i := range candidates {
		var parts []string
		_ = json.Unmarshal([]byte(candidates[i].sectionJSON), &parts)
		explicit := canonicalSectionScope(parts)
		if explicit != "" {
			active = explicit
		} else if active == "" && candidates[i].sectionScope != "" {
			// A missing heading in a continuation window inherits the last
			// physical-page scope. Only use the stored scope to seed the stream
			// when no earlier explicit heading is available; otherwise a stale
			// scope from an older relink could move a question across a topic
			// boundary (for example page 98's previous topic vs 专题二).
			active = canonicalSectionScope(strings.Split(candidates[i].sectionScope, "/"))
		}
		candidates[i].sectionScope = active
		if explicit == "" && active != "" {
			// Persist the inferred path as well as the scope so the session
			// reconciler (which intentionally reads durable source paths) sees
			// the same physical-order decision on its next run.
			inferred, _ := json.Marshal(strings.Split(active, "/"))
			candidates[i].sectionJSON = string(inferred)
		}
	}
}

func propagateAnswerScopes(answers []alignmentAnswer) {
	active := ""
	for i := range answers {
		var parts []string
		_ = json.Unmarshal([]byte(answers[i].sectionJSON), &parts)
		explicit := canonicalSectionScope(parts)
		stored := canonicalSectionScope(strings.Split(strings.TrimSpace(answers[i].sectionScope), "/"))
		if stored != "" {
			// section_scope_id is an explicit source field. In particular, do not
			// replace a valid scope with the previous page's scope merely because
			// section_path_json was omitted by the model.
			active = stored
			if explicit != stored {
				inferred, _ := json.Marshal(strings.Split(stored, "/"))
				answers[i].sectionJSON = string(inferred)
			}
		} else if explicit != "" {
			active = explicit
			answers[i].sectionScope = explicit
		} else if active != "" {
			answers[i].sectionScope = active
			inferred, _ := json.Marshal(strings.Split(active, "/"))
			answers[i].sectionJSON = string(inferred)
		}
	}
}

func alignmentPathKey(sectionJSON, canonicalType string, qno int) string {
	var parts []string
	_ = json.Unmarshal([]byte(sectionJSON), &parts)
	for index := range parts {
		parts[index] = compactText(parts[index])
	}
	encoded, _ := json.Marshal(parts)
	return fmt.Sprintf("%s|%s|%d", string(encoded), canonicalType, qno)
}

func confidenceForMethod(method string) float64 {
	if method == "canonical" {
		return 0.98
	}
	if method == "ordered_block" {
		return 0.88
	}
	return 1
}
func nullableRun(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}

func (s *ImportService) refreshSessionReviewCounts(ctx context.Context, sessionID, runID int64) error {
	db := s.repo.DB()
	var expected, unique, complete, blocking int
	_, _ = db.ExecContext(ctx, `DELETE FROM import_problems WHERE session_id=? AND status='open' AND problem_type='IMPORT_UNIT_FAILED'`, sessionID)
	failedRows, _ := db.QueryContext(ctx, `SELECT j.document_id,u.owned_page_start,u.owned_page_end,u.unit_type,COALESCE(u.error_message,'') FROM import_agent_units u JOIN import_jobs j ON j.id=u.import_job_id WHERE j.session_id=? AND u.status='failed'`, sessionID)
	if failedRows != nil {
		for failedRows.Next() {
			var docID int64
			var start, end int
			var unitType, message string
			if failedRows.Scan(&docID, &start, &end, &unitType, &message) == nil {
				detail, _ := json.Marshal(map[string]interface{}{"unit_type": unitType, "message": message})
				_, _ = db.ExecContext(ctx, `INSERT INTO import_problems(session_id,run_id,document_id,problem_type,severity,page_start,page_end,detail_json) VALUES(?,?,?,'IMPORT_UNIT_FAILED','blocking',?,?,?)`, sessionID, nullableRun(runID), docID, start, end, string(detail))
			}
		}
		failedRows.Close()
	}
	// Older runs may have persisted directory_complete=0 after a transient
	// failure even though every profile/directory unit was later retried and
	// completed. Recompute the flag from durable unit state instead of keeping
	// the stale job-level boolean as a permanent blocker.
	incomplete := 0
	jobRows, _ := db.QueryContext(ctx, `SELECT DISTINCT j.id FROM import_session_documents sd JOIN import_jobs j ON j.id=sd.import_job_id WHERE sd.session_id=? AND sd.document_role IN ('question_book','mixed','supplement')`, sessionID)
	if jobRows != nil {
		for jobRows.Next() {
			var jobID int64
			if jobRows.Scan(&jobID) != nil {
				incomplete++
				continue
			}
			var totalUnits, incompleteUnits int
			// V1 uses profile/directory; V1.1 uses the combined catalog stage.
			// Count both schemas so a completed V1.1 catalog is not reported as
			// an incomplete directory with expected_count=nil.
			_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_agent_units WHERE import_job_id=? AND unit_type IN ('profile','directory','catalog')`, jobID).Scan(&totalUnits)
			_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_agent_units WHERE import_job_id=? AND unit_type IN ('profile','directory','catalog') AND status!='completed'`, jobID).Scan(&incompleteUnits)
			if totalUnits == 0 || incompleteUnits > 0 {
				incomplete++
			} else {
				_, _ = db.ExecContext(ctx, `UPDATE import_jobs SET directory_complete=1,updated_at=datetime('now') WHERE id=?`, jobID)
			}
		}
		jobRows.Close()
	}
	if value, countErr := countSessionDirectoryIdentities(ctx, db, sessionID); countErr == nil {
		expected = value
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate')`, sessionID).Scan(&unique)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND review_decision='include' AND answer_link_status='confirmed' AND status='validated'`, sessionID).Scan(&complete)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND status='open' AND severity='blocking'`, sessionID).Scan(&blocking)
	var review int
	// import_jobs is a denormalized projection used by the legacy job API and
	// by older clients. Reconciliation used to update only import_sessions,
	// leaving a repaired session showing the old failure counts (for example
	// 1,069 blocking units after the session had reached 0). Keep the job
	// projection in sync without touching the durable unit/message audit trail.
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate') AND (COALESCE(review_decision,'pending')='pending' OR COALESCE(answer_link_status,'unresolved')!='confirmed' OR status!='validated')`, sessionID).Scan(&review)
	status := "draft_ready"
	if incomplete > 0 || expected == 0 || expected != unique || blocking > 0 {
		status = "needs_attention"
	}
	var expectedValue interface{} = expected
	if incomplete > 0 {
		expectedValue = nil
	}
	jobProgress := 0
	if incomplete == 0 && expected > 0 {
		jobProgress = 1
	}
	directoryComplete := 0
	if incomplete == 0 {
		directoryComplete = 1
	}
	_, _ = db.ExecContext(ctx, `UPDATE import_jobs SET status=?,stage=?,directory_complete=?,expected_count=?,candidate_count=?,extracted_count=?,complete_count=?,review_count=?,blocking_issue_count=?,progress=CASE WHEN ?=1 THEN 1.0 ELSE progress END,updated_at=datetime('now') WHERE session_id=? AND status<>'cancelled'`, status, status, directoryComplete, expectedValue, unique, unique, complete, review, blocking, jobProgress, sessionID)
	_, err := db.ExecContext(ctx, `UPDATE import_sessions SET status=?,expected_count=?,unique_count=?,complete_count=?,blocking_issue_count=?,draft_version=draft_version+1,last_preview_hash=NULL,last_preview_version=NULL,updated_at=datetime('now') WHERE id=?`, status, expectedValue, unique, complete, blocking, sessionID)
	return err
}
