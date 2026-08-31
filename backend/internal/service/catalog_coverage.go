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
	"github.com/quiztrace/quiztrace/internal/domain"
)

const catalogCoverageVersion = "catalog-coverage.v1"

var printedQuestionMarker = regexp.MustCompile(`(?m)^\s*([0-9]{1,3})\s*(?:[.、,，]\s+|[.、,，]《|[《（(])`)

type pageCoverageResult struct {
	Page    int
	Role    string
	Missing []int
}

// verifyCatalogCoverage treats OCR as an independent conservation manifest.
// It is deliberately one-sided: OCR may omit labels, but labels visible to OCR
// and absent from Catalog prove that the Catalog projection is incomplete.
func (s *ImportService) verifyCatalogCoverage(ctx context.Context, sessionID, documentID, runID int64, entries map[string]domain.VisionDirectoryEntry) ([]pageCoverageResult, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `SELECT page_no,COALESCE(text,'') FROM document_pages WHERE document_id=? ORDER BY page_no`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pageText := map[int]string{}
	for rows.Next() {
		var page int
		var text string
		if err := rows.Scan(&page, &text); err != nil {
			return nil, err
		}
		pageText[page] = text
	}
	byPage := map[int][]domain.VisionDirectoryEntry{}
	for _, entry := range entries {
		if entry.SourcePageStart > 0 {
			byPage[entry.SourcePageStart] = append(byPage[entry.SourcePageStart], entry)
		}
	}
	pages := make([]int, 0, len(byPage))
	for page := range byPage {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	results := []pageCoverageResult{}
	for _, page := range pages {
		ocrNumbers := markerNumbers(pageText[page])
		catalogNumbers := make([]int, 0, len(byPage[page]))
		roles := map[string]int{}
		catalogManifest := make([]map[string]interface{}, 0, len(byPage[page]))
		for _, entry := range byPage[page] {
			if entry.SourceQuestionNo <= 0 {
				continue
			}
			catalogNumbers = append(catalogNumbers, entry.SourceQuestionNo)
			roles[entry.ContentRole]++
			catalogManifest = append(catalogManifest, map[string]interface{}{"number": entry.SourceQuestionNo, "label": entry.OriginalQuestionLabel, "type": domain.CanonicalQuestionType(entry.QuestionType), "role": entry.ContentRole})
		}
		// Existing validated candidate bodies are independent durable evidence
		// for legacy sessions whose raw Catalog unit omitted a row. Include their
		// printed markers in coverage, but do not rewrite the Catalog identity.
		candidateRows, candidateErr := s.repo.DB().QueryContext(ctx, `SELECT source_question_no FROM candidate_questions WHERE import_session_id=? AND document_id=? AND question_page_start=? AND status NOT IN ('session_duplicate','duplicate','persisted') AND source_question_no>0`, sessionID, documentID, page)
		if candidateErr == nil {
			for candidateRows.Next() {
				var number int
				if candidateRows.Scan(&number) == nil {
					catalogNumbers = append(catalogNumbers, number)
				}
			}
			candidateRows.Close()
		}
		answerRows, answerErr := s.repo.DB().QueryContext(ctx, `SELECT source_question_no FROM import_answer_records WHERE session_id=? AND document_id=? AND page_start=? AND source_question_no>0`, sessionID, documentID, page)
		if answerErr == nil {
			for answerRows.Next() { var number int; if answerRows.Scan(&number)==nil { catalogNumbers=append(catalogNumbers,number) } }
			answerRows.Close()
		}
		missing := multisetDifference(ocrNumbers, catalogNumbers)
		status := "verified"
		if len(ocrNumbers) == 0 || len(catalogNumbers) == 0 {
			status = "unverifiable"
		}
		if len(missing) > 0 {
			status = "mismatch"
			results = append(results, pageCoverageResult{Page: page, Role: dominantCoverageRole(roles), Missing: missing})
		}
		role := dominantCoverageRole(roles)
		ocrJSON, _ := json.Marshal(map[string]interface{}{"numbers": ocrNumbers, "headings": coverageHeadings(pageText[page])})
		catalogJSON, _ := json.Marshal(map[string]interface{}{"entries": catalogManifest})
		missingJSON, _ := json.Marshal(missing)
		_, err = s.repo.DB().ExecContext(ctx, `INSERT INTO import_page_coverage(session_id,document_id,run_id,page_no,source_role,ocr_manifest_json,catalog_manifest_json,missing_json,status,detector_version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,datetime('now')) ON CONFLICT(session_id,document_id,page_no,source_role) DO UPDATE SET run_id=excluded.run_id,ocr_manifest_json=excluded.ocr_manifest_json,catalog_manifest_json=excluded.catalog_manifest_json,missing_json=excluded.missing_json,status=excluded.status,detector_version=excluded.detector_version,updated_at=datetime('now')`, sessionID, documentID, nullableRun(runID), page, role, string(ocrJSON), string(catalogJSON), string(missingJSON), status, catalogCoverageVersion)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func markerNumbers(text string) []int {
	matches := printedQuestionMarker.FindAllStringSubmatch(text, -1)
	out := make([]int, 0, len(matches))
	for _, match := range matches {
		if n, err := strconv.Atoi(match[1]); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func multisetDifference(left, right []int) []int {
	counts := map[int]int{}
	for _, value := range right {
		counts[value]++
	}
	missing := []int{}
	for _, value := range left {
		if counts[value] > 0 {
			counts[value]--
			continue
		}
		missing = append(missing, value)
	}
	return missing
}

func coverageHeadings(text string) []string {
	out := []string{}
	for _, heading := range []string{"单项选择", "多项选择", "判断", "填空", "简答", "案例"} {
		if strings.Contains(text, heading) {
			out = append(out, heading)
		}
	}
	return out
}

func dominantCoverageRole(roles map[string]int) string {
	role, count := "mixed", 0
	for key, value := range roles {
		if value > count {
			role, count = key, value
		}
	}
	return role
}

func coverageIssue(result pageCoverageResult) map[string]interface{} {
	return issue("DIRECTORY_COVERAGE_MISMATCH", result.Page, result.Page, fmt.Sprintf("OCR 可见但目录缺少题号 %v", result.Missing))
}

func (s *ImportService) reviewCatalogCoveragePages(ctx context.Context, imp *domain.ImportJob, doc *domain.Document, llm provider.LLMProvider, callOptions visionCallOptions, baseWindows []visionPageWindow, global map[string]domain.VisionDirectoryEntry, entriesByWindow map[int][]domain.VisionDirectoryEntry, mismatches []pageCoverageResult) (map[string]domain.VisionDirectoryEntry, map[int][]domain.VisionDirectoryEntry, error) {
	seen := map[int]bool{}
	windows := []visionPageWindow{}
	pageCount := doc.PageCount
	if len(baseWindows) > 0 && baseWindows[len(baseWindows)-1].ContextEnd > pageCount {
		pageCount = baseWindows[len(baseWindows)-1].ContextEnd
	}
	for _, mismatch := range mismatches {
		if seen[mismatch.Page] {
			continue
		}
		seen[mismatch.Page] = true
		start, end := mismatch.Page-1, mismatch.Page+1
		if start < 1 {
			start = 1
		}
		if end > pageCount {
			end = pageCount
		}
		windows = append(windows, visionPageWindow{Index: len(baseWindows) + len(windows), OwnedStart: mismatch.Page, OwnedEnd: mismatch.Page, ContextStart: start, ContextEnd: end})
	}
	if len(windows) == 0 {
		return global, entriesByWindow, nil
	}
	outputs, failures, err := s.runVisionStage(ctx, imp, doc, llm, windows, "review_catalog_coverage", 1, func(reviewCtx context.Context, unit *domain.ImportAgentUnit, window visionPageWindow) (string, error) {
		if err := s.ensureVisionReviewAssets(reviewCtx, doc, window.ContextStart, window.ContextEnd); err != nil {
			return "", err
		}
		opts := callOptions
		opts.Schema, opts.MaxTokens, opts.UseReviewAssets = "catalog", 12000, true
		return s.callVisionAgent(reviewCtx, imp, llm, doc, unit, "review", catalogPromptVersion+".coverage", catalogPrompt(window), opts)
	})
	if err != nil {
		return global, entriesByWindow, err
	}
	_, _, repaired, _, _, complete, _ := coordinateCatalogV11(outputs, windows, failures)
	if !complete {
		return global, entriesByWindow, nil
	}
	for page := range seen {
		for identity, entry := range global {
			if entry.SourcePageStart == page {
				delete(global, identity)
			}
		}
		for index, entries := range entriesByWindow {
			filtered := entries[:0]
			for _, entry := range entries {
				if entry.SourcePageStart != page {
					filtered = append(filtered, entry)
				}
			}
			entriesByWindow[index] = filtered
		}
	}
	for identity, entry := range repaired {
		if !seen[entry.SourcePageStart] {
			continue
		}
		global[identity] = entry
		for _, window := range baseWindows {
			if entry.SourcePageStart >= window.OwnedStart && entry.SourcePageStart <= window.OwnedEnd {
				entriesByWindow[window.Index] = append(entriesByWindow[window.Index], entry)
				break
			}
		}
	}
	return global, entriesByWindow, nil
}
