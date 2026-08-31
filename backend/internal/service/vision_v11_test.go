package service

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestCoordinateCatalogV11ExpandsCompactSections(t *testing.T) {
	windows := buildVisionWindows(8)
	outputs := map[int]string{
		1: `{"sections":[{"id":"s1","path":["法规","专题一"],"set_no":1,"question_type":"single_choice"}],"pages":[{"page":1,"role":"question","section_id":"s1"}],"entries":[{"id":"e1","content_role":"question","section_id":"s1","source_question_no":1,"source_page_start":1,"source_page_end":1,"anchor":"题干","marker_top":100}]}`,
		2: `{"sections":[],"pages":[{"page":5,"role":"blank","section_id":""}],"entries":[]}`,
	}
	role, confidence, global, byWindow, issues, complete, badWindows := coordinateCatalogV11(outputs, windows, 0)
	if !complete || len(issues) != 0 {
		t.Fatalf("catalog should be complete: %#v", issues)
	}
	if len(badWindows) != 0 {
		t.Fatalf("unexpected bad windows: %#v", badWindows)
	}
	if role != "question_book" || confidence <= 0 {
		t.Fatalf("unexpected role %s confidence %f", role, confidence)
	}
	if len(global) != 1 || len(byWindow[1]) != 1 {
		t.Fatalf("entry was not expanded: %#v", global)
	}
	entry := byWindow[1][0]
	if entry.SectionPath[1] != "专题一" || entry.SourceIdentity == "" {
		t.Fatalf("invalid expanded entry: %#v", entry)
	}
}

func TestCoordinateCatalogV11ScopesTypeOnlyContinuationByPhysicalOrder(t *testing.T) {
	windows := buildVisionWindows(8)
	outputs := map[int]string{
		1: `{"sections":[{"id":"s1","path":["专题一 教育法律基础","一、单项选择题"],"set_no":0,"question_type":"single_choice"}],"pages":[{"page":1,"role":"question","section_id":"s1"}],"entries":[{"id":"e1","content_role":"question","section_id":"s1","source_question_no":1,"source_page_start":1,"source_page_end":1}]}`,
		2: `{"sections":[{"id":"s2","path":["专题二 依法执教","一、单项选择题"],"set_no":0,"question_type":"single_choice"},{"id":"s3","path":["二、多项选择题"],"set_no":0,"question_type":"multiple_choice"}],"pages":[{"page":5,"role":"explanation","section_id":"s2"},{"page":6,"role":"explanation","section_id":"s3"}],"entries":[{"id":"e2","content_role":"explanation","section_id":"s2","source_question_no":1,"source_page_start":5,"source_page_end":5},{"id":"e3","content_role":"explanation","section_id":"s3","source_question_no":7,"source_page_start":6,"source_page_end":6}]}`,
	}
	_, _, global, _, issues, complete, _ := coordinateCatalogV11(outputs, windows, 0)
	if !complete || len(issues) != 0 {
		t.Fatalf("catalog should be complete: %#v", issues)
	}
	var continuation domain.VisionDirectoryEntry
	for _, entry := range global {
		if entry.SourceQuestionNo == 7 {
			continuation = entry
		}
	}
	if continuation.SectionScopeID != "依法执教" {
		t.Fatalf("type-only continuation inherited wrong scope: %#v", continuation)
	}
}

func TestAdaptiveVisionLimiterChangesConcurrency(t *testing.T) {
	limiter := newAdaptiveVisionLimiter(2, 3, time.Minute, 3*time.Minute)
	for i := 0; i < 10; i++ {
		permit, err := limiter.acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		permit.release(nil, 10*time.Second)
	}
	limit, _ := limiter.snapshot()
	if limit != 3 {
		t.Fatalf("expected concurrency 3, got %d", limit)
	}
	permit, err := limiter.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	permit.release(errors.New("context deadline exceeded"), 240*time.Second)
	limit, reason := limiter.snapshot()
	if limit != 1 || reason == "" {
		t.Fatalf("expected degraded concurrency 1, got %d %q", limit, reason)
	}
}

func TestRepairJSONLocallyDoesNotNeedImages(t *testing.T) {
	raw := "```json\n{\"questions\":[{\"stem\":\"题干\",}],}\n```"
	repaired := repairJSONLocally(raw)
	if _, err := validateVisionUnitJSON("question", "extract_question", repaired); err != nil {
		t.Fatalf("local repair failed: %v: %s", err, repaired)
	}
}

func TestCandidateReviewSchemaIsPatchBased(t *testing.T) {
	valid := `{"patches":[{"candidate_id":7,"source_identity":"source|7","stem":"题干","options":[],"question_type":"subjective","source_page_start":4,"source_page_end":4}]}`
	if count, err := validateVisionUnitJSON("candidate_question", "candidate_question", valid); err != nil || count != 1 {
		t.Fatalf("candidate patch schema rejected: count=%d err=%v", count, err)
	}
	if _, err := validateVisionUnitJSON("candidate_question", "candidate_question", `{"questions":[]}`); err == nil {
		t.Fatal("legacy questions payload must not satisfy candidate review schema")
	}
}

func TestAutoCandidateReviewTargetsAnswerProblemsOnly(t *testing.T) {
	target := candidateReviewTarget{ID: 9, QuestionPageStart: 10, QuestionPageEnd: 10, AnswerPageStart: 107, AnswerPageEnd: 107, ProblemTypes: "ANSWER_SOURCE_IDENTITY_INVALID,ANSWER_UNRESOLVED"}
	roles := reviewRolesForTarget(target, "auto")
	if len(roles) != 1 || roles[0] != "answer" {
		t.Fatalf("answer blockers must not rescan question pages: %#v", roles)
	}
	plan := buildCandidateReviewPlan(30, []candidateReviewTarget{target}, "auto")
	if plan.ExpectedCalls != 1 || plan.RoleCalls["answer"] != 1 || plan.RoleCalls["question"] != 0 {
		t.Fatalf("unexpected targeted plan: %#v", plan)
	}
}

func TestAutoCandidateReviewNeverFallsBackWithoutAnswerSpan(t *testing.T) {
	target := candidateReviewTarget{ID: 9, QuestionPageStart: 10, QuestionPageEnd: 10, ProblemTypes: "ANSWER_UNRESOLVED"}
	plan := buildCandidateReviewPlan(30, []candidateReviewTarget{target}, "auto")
	if plan.ExpectedCalls != 0 || len(plan.Unlocatable) != 1 || plan.Unlocatable[0].Code != "SOURCE_SPAN_MISSING" {
		t.Fatalf("missing answer evidence must not fall back to question/full PDF: %#v", plan)
	}
}

func TestCropVisionAssetUsesNormalizedRegion(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	cropped, err := cropVisionAsset(encoded.Bytes(), domain.VisionRegion{Left: 100, Top: 250, Right: 900, Bottom: 750})
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(cropped))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 80 || decoded.Bounds().Dy() != 40 {
		t.Fatalf("normalized crop has unexpected size: %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestLoopbackProviderDetection(t *testing.T) {
	if !isLoopbackProvider("http://127.0.0.1:8045/v1") {
		t.Fatal("127.0.0.1 should be local")
	}
	if isLoopbackProvider("https://api.example.com/v1") {
		t.Fatal("remote endpoint should not be local")
	}
}

func TestFindVisionDirectoryEntryUsesUniquePhysicalFallback(t *testing.T) {
	entry := domain.VisionDirectoryEntry{
		ContentRole:      "question",
		SectionPath:      []string{"专题四《中华人民共和国义务教育法》", "一、单项选择题"},
		QuestionType:     "single_choice",
		SourceQuestionNo: 37,
		SourcePageStart:  29,
		SourcePageEnd:    29,
		SourceIdentity:   `["专题四《中华人民共和国义务教育法》","一、单项选择题"]|question|0|single_choice|37`,
	}
	got, ok := findVisionDirectoryEntry(map[string]domain.VisionDirectoryEntry{entry.SourceIdentity: entry}, domain.VisionQuestion{
		SourceIdentity:  `["一、单项选择题"]|question|0|single_choice|37`,
		QuestionType:    "single_choice",
		SourcePageStart: 29,
		SourcePageEnd:   29,
	})
	if !ok || got.SourceIdentity != entry.SourceIdentity {
		t.Fatalf("type-only identity did not resolve to unique physical entry: %#v %v", got, ok)
	}
}

func TestFindVisionDirectoryEntryRejectsAmbiguousQuestionNumber(t *testing.T) {
	first := domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"专题一", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 20, SourcePageEnd: 20, SourceIdentity: `["专题一","一、单项选择题"]|question|0|single_choice|1`}
	second := domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"专题二", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 20, SourcePageEnd: 20, SourceIdentity: `["专题二","一、单项选择题"]|question|0|single_choice|1`}
	if _, ok := findVisionDirectoryEntry(map[string]domain.VisionDirectoryEntry{first.SourceIdentity: first, second.SourceIdentity: second}, domain.VisionQuestion{SourceIdentity: `["一、单项选择题"]|question|0|single_choice|1`, QuestionType: "single_choice", SourcePageStart: 20, SourcePageEnd: 20}); ok {
		t.Fatal("ambiguous type-only identity must not cross-link")
	}
}
