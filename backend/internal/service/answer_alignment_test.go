package service

import (
	"strings"
	"testing"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestCanonicalQuestionTypeAliases(t *testing.T) {
	cases := map[string]string{
		"multi_choice":  "multiple_choice",
		"composite":     "multiple_choice",
		"judgement":     "true_false",
		"true_or_false": "true_false",
		"essay":         "subjective",
		"case_analysis": "subjective",
		"material":      "subjective",
		"fill_in":       "fill_in_blank",
		"single_choice": "single_choice",
	}
	for raw, want := range cases {
		if got := domain.CanonicalQuestionType(raw); got != want {
			t.Fatalf("%q: got %q want %q", raw, got, want)
		}
	}
}

func TestBlankFillingAlias(t *testing.T) {
	if got := domain.CanonicalQuestionType("blank_filling"); got != "fill_in_blank" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeVisionAnswerIdentityUsesExplicitScopeWithoutPath(t *testing.T) {
	entry := domain.VisionDirectoryEntry{
		ContentRole:           "explanation",
		SectionPath:           []string{"依法执教与教师违法(侵权)行为", "一、单项选择题"},
		SetNo:                 0,
		QuestionType:          "single_choice",
		CanonicalQuestionType: "single_choice",
		SectionScopeID:        "依法执教与教师违法(侵权)行为",
		SourceQuestionNo:      1,
		SourcePageStart:       134,
		SourcePageEnd:         134,
	}
	entry.SourceIdentity = visionSourceIdentity(entry)
	answer := domain.VisionAnswer{
		SectionScopeID:        entry.SectionScopeID,
		QuestionType:          "single_choice",
		CanonicalQuestionType: "single_choice",
		SourceQuestionNo:      1,
		SourcePageStart:       134,
		SourcePageEnd:         134,
		Answer:                []string{"C"},
	}
	got, match, ok := normalizeVisionAnswerIdentity(answer, []domain.VisionDirectoryEntry{entry})
	if !ok || match.Method != "identity_scope" {
		t.Fatalf("expected explicit scope match, got %#v ok=%v", match, ok)
	}
	if got.SourceIdentity != entry.SourceIdentity || got.SectionScopeID != entry.SectionScopeID || len(got.SectionPath) != 2 {
		t.Fatalf("identity was not normalized from directory: %#v", got)
	}
}

func TestNormalizeVisionAnswerIdentityUsesDirectoryScopeWhenDirectoryPathIsMissing(t *testing.T) {
	entry := domain.VisionDirectoryEntry{
		ContentRole:           "answer",
		SectionScopeID:        "依法执教",
		SetNo:                 0,
		QuestionType:          "single_choice",
		CanonicalQuestionType: "single_choice",
		SourceQuestionNo:      1,
		SourcePageStart:       10,
		SourcePageEnd:         10,
	}
	entry.SourceIdentity = visionSourceIdentity(entry)
	answer := domain.VisionAnswer{SectionScopeID: "依法执教", QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 10, SourcePageEnd: 10}
	if _, match, ok := normalizeVisionAnswerIdentity(answer, []domain.VisionDirectoryEntry{entry}); !ok || match.Method != "identity_scope" {
		t.Fatalf("directory scope should be sufficient when path is omitted: %#v ok=%v", match, ok)
	}
}

func TestNormalizeVisionAnswerIdentityRepairsStalePhysicalScopeOnlyWhenUnique(t *testing.T) {
	previous := domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"教育法律基础", "一、单项选择题"}, SectionScopeID: "教育法律基础", QuestionType: "single_choice", CanonicalQuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 128, SourcePageEnd: 128}
	current := domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"依法执教与教师违法(侵权)行为", "一、单项选择题"}, SectionScopeID: "依法执教与教师违法(侵权)行为", QuestionType: "single_choice", CanonicalQuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 134, SourcePageEnd: 134}
	previous.SourceIdentity = visionSourceIdentity(previous)
	current.SourceIdentity = visionSourceIdentity(current)
	answer := domain.VisionAnswer{SectionPath: previous.SectionPath, SectionScopeID: previous.SectionScopeID, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 134, SourcePageEnd: 134}
	got, match, ok := normalizeVisionAnswerIdentity(answer, []domain.VisionDirectoryEntry{previous, current})
	if !ok || match.Method != "physical_unique" || got.SourceIdentity != current.SourceIdentity {
		t.Fatalf("stale scope was not repaired from unique page evidence: %#v match=%#v ok=%v", got, match, ok)
	}

	current.SourcePageStart = 128
	current.SourcePageEnd = 128
	_, _, ok = normalizeVisionAnswerIdentity(answer, []domain.VisionDirectoryEntry{previous, current})
	if ok {
		t.Fatal("same number/type/page across topics must remain ambiguous")
	}
}

func TestNormalizeVisionAnswerIdentityAcceptsCompatibleSetNumber(t *testing.T) {
	entry := domain.VisionDirectoryEntry{ContentRole: "answer", SectionPath: []string{"专题一", "一、单项选择题"}, SectionScopeID: "专题一", SetNo: 1, QuestionType: "single_choice", CanonicalQuestionType: "single_choice", SourceQuestionNo: 7, SourcePageStart: 20, SourcePageEnd: 20}
	entry.SourceIdentity = visionSourceIdentity(entry)
	answer := domain.VisionAnswer{SectionScopeID: "专题一", SetNo: 0, QuestionType: "single_choice", SourceQuestionNo: 7, SourcePageStart: 20, SourcePageEnd: 20}
	if _, match, ok := normalizeVisionAnswerIdentity(answer, []domain.VisionDirectoryEntry{entry}); !ok || match.Entry.SetNo != 1 {
		t.Fatalf("set 0 should be compatible with a unique set 1 source: %#v ok=%v", match, ok)
	}
}

func TestParseVisionSourceIdentitySupportsAnswerRoles(t *testing.T) {
	path := []string{"依法执教", "一、单项选择题"}
	raw := visionSourceIdentity(domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: path, SetNo: 0, QuestionType: "single_choice", SourceQuestionNo: 3})
	gotPath, role, setNo, qtype, qno, ok := parseVisionSourceIdentity(raw)
	if !ok || role != "explanation" || setNo != 0 || qtype != "single_choice" || qno != 3 || strings.Join(gotPath, "/") != strings.Join(path, "/") {
		t.Fatalf("unexpected parsed answer identity: %v %q %d %q %d %v", gotPath, role, setNo, qtype, qno, ok)
	}
}

func TestDistinctVisionAnswerEvidenceMergesSameValueButKeepsConflict(t *testing.T) {
	items := []domain.VisionAnswer{
		{Answer: []string{"C"}, Analysis: "短"},
		{Answer: []string{"C"}, Analysis: "更完整的解析"},
		{Answer: []string{"B"}, Analysis: "另一答案"},
	}
	got := distinctVisionAnswerEvidence(items)
	if len(got) != 2 {
		t.Fatalf("same answer should merge while conflicting value remains: %#v", got)
	}
	for _, item := range got {
		if len(item.Answer) == 1 && item.Answer[0] == "C" && item.Analysis != "更完整的解析" {
			t.Fatalf("richer duplicate explanation should win: %#v", item)
		}
	}
}

func TestCanonicalSectionScopeDropsOnlyTypeHeading(t *testing.T) {
	got := canonicalSectionScope([]string{"教育政策法规", "专题一", "二、多项选择题"})
	if got != "教育政策法规/专题一" {
		t.Fatalf("got %q", got)
	}
	if canonicalSectionScope([]string{"专题一"}) != "专题一" {
		t.Fatal("topic heading should remain")
	}
}

func TestCanonicalSectionScopeNormalizesAnswerBookPrefix(t *testing.T) {
	got := canonicalSectionScope([]string{"参考答案及解析 · 小四门部分", "第一章 教育政策法规", "专题一 教育法律基础"})
	if got != "小四门部分/第一章教育政策法规/教育法律基础" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalSectionScopeNormalizesTopicNumberWhenTitleExists(t *testing.T) {
	if got := canonicalSectionScope([]string{"专题三《中华人民共和国教育法》", "一、单项选择题"}); got != "《中华人民共和国教育法》" {
		t.Fatalf("got %q", got)
	}
	if got := canonicalSectionScope([]string{"专题一"}); got != "专题一" {
		t.Fatalf("bare topic heading should be retained, got %q", got)
	}
}

func TestCanonicalSectionScopeNormalizesFullWidthPunctuation(t *testing.T) {
	fullWidth := canonicalSectionScope([]string{"（一）案例客观题"})
	ascii := canonicalSectionScope([]string{"(一)案例客观题"})
	if fullWidth == "" || fullWidth != ascii {
		t.Fatalf("full-width and ASCII section punctuation must match: %q vs %q", fullWidth, ascii)
	}
}

func TestScopesEquivalentIgnoresBookTitleMarks(t *testing.T) {
	if !scopesEquivalent("《中华人民共和国教育法》", "中华人民共和国教育法", map[string]bool{}) {
		t.Fatal("book-title punctuation must not split one source topic")
	}
}

func TestNormalizedAnswerValuesNormalizesJudgementGlyphs(t *testing.T) {
	got := normalizedAnswerValues(`["√"]`, "true_false")
	if len(got) != 1 || got[0] != "正确" {
		t.Fatalf("got %#v", got)
	}
	got = normalizedAnswerValues(`["X"]`, "true_false")
	if len(got) != 1 || got[0] != "错误" {
		t.Fatalf("ASCII X should normalize to false, got %#v", got)
	}
	got = normalizedAnswerValues(`["✕"]`, "true_false")
	if len(got) != 1 || got[0] != "错误" {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizedAnswerValuesTrueFalseLetters(t *testing.T) {
	got := normalizedAnswerValues(`["T","F","V"]`, "true_false")
	if strings.Join(got, ",") != "正确,错误,正确" {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizeSubjectiveSourceKeepsReferenceTextOutOfChoiceAnswer(t *testing.T) {
	values, analysis := normalizeSubjectiveSource([]string{"参考答案第一点", "ABD"}, "")
	if strings.Join(values, "") != "ABD" || !strings.Contains(analysis, "参考答案第一点") {
		t.Fatalf("values=%v analysis=%q", values, analysis)
	}
}

func TestReferenceAnswerSectionGuard(t *testing.T) {
	if !isReferenceAnswerSection(`["四、案例分析题（参考答案）"]`) {
		t.Fatal("reference answer heading should be accepted")
	}
	if isReferenceAnswerSection(`["四、案例分析题"]`) {
		t.Fatal("plain question heading must not be promoted to an answer source")
	}
}

func TestExplicitAnswerFromAnchorAcceptsASCIIFalseMarker(t *testing.T) {
	values, _, ok := explicitAnswerFromAnchor("2. X 【解析】该命题错误。", 2, "true_false")
	if !ok || len(values) != 1 || values[0] != "错误" {
		t.Fatalf("expected ASCII X to be parsed as false, values=%v ok=%v", values, ok)
	}
}

func TestScopesEquivalentOnlyAllowsUniqueTerminalTopic(t *testing.T) {
	if !scopesEquivalent("第四章/课堂教学技能", "课堂教学技能", map[string]bool{"课堂教学技能": true}) {
		t.Fatal("unique terminal topic should match")
	}
	if scopesEquivalent("第一章/课堂教学技能", "课堂教学技能", map[string]bool{"课堂教学技能": false}) {
		t.Fatal("ambiguous terminal topic must not match")
	}
}

func TestSectionSubtypeKeepsSubjectiveKindsDistinct(t *testing.T) {
	if got := sectionSubtype([]string{"五、案例分析题（参考答案）"}); got != "case_analysis" {
		t.Fatalf("case subtype: got %q", got)
	}
	if got := sectionSubtype([]string{"四、简答题(参考答案)"}); got != "short_answer" {
		t.Fatalf("short subtype: got %q", got)
	}
	if got := sectionSubtype([]string{"二、多项选择题"}); got != "" {
		t.Fatalf("choice headings must not become subjective subtype: got %q", got)
	}
}

func TestFilterSubjectiveSubtypeRejectsDifferentExplicitKind(t *testing.T) {
	candidate := alignmentCandidate{canonicalType: "subjective", subtype: "short_answer"}
	matches := []alignmentAnswer{
		{subtype: "case_analysis", answerJSON: `["case"]`},
		{subtype: "short_answer", answerJSON: `["short"]`},
	}
	filtered := filterSubjectiveSubtype(matches, candidate)
	if len(filtered) != 1 || filtered[0].subtype != "short_answer" {
		t.Fatalf("got %#v", filtered)
	}
}

func TestSubjectiveEvidenceDoesNotConfuseQuestionAndOralLanguageRequirements(t *testing.T) {
	matches := []alignmentAnswer{
		{id: 602, analysis: "(1)符合规范,内容科学,合乎逻辑;(2)通俗易懂,生动活泼,富于启发;(3)条理清晰,层次分明,重点突出;(4)富于创造性,有独特的风格。"},
		{id: 1163, analysis: "3. 简述课堂提问的基本要求。"},
	}
	winner, ok, _, _ := subjectiveEvidenceWinner(matches, "3. 简述课堂教学口语的基本要求。")
	if ok && winner.id == 1163 {
		t.Fatalf("generic requirement wording must not link to the neighbouring question")
	}
}

func TestSubjectiveEvidenceDoesNotWidenScopedMatches(t *testing.T) {
	candidate := alignmentCandidate{canonicalType: "subjective", subtype: "case_analysis", qno: 1}
	scoped := []alignmentAnswer{{id: 566, sectionScope: "第二章新课程改革/教学方式与学习方式的变革", qno: 1, canonicalType: "subjective", subtype: "case_analysis", analysis: "材料中的张老师根据学生意见调整了教学计划。"}}
	unrelated := []alignmentAnswer{{id: 181, sectionScope: "其他相关教育政策法规", qno: 1, canonicalType: "subjective", subtype: "case_analysis", analysis: "教育惩戒案例与教师管理。"}}
	got := expandSubjectiveEvidenceMatches(scoped, append(scoped, unrelated...), candidate, "张老师请学生共同设计课本剧并调整了教学安排。")
	if len(got) != 1 || got[0].id != 566 {
		t.Fatalf("scoped match was widened to an unrelated answer: %#v", got)
	}
}

func TestCitedTopicScopePrefersTheNamedEducationLaw(t *testing.T) {
	answers := []alignmentAnswer{
		{sectionScope: "《中华人民共和国义务教育法》"},
		{sectionScope: "《中华人民共和国教育法》"},
	}
	got := citedTopicScopeFromStem("简述《中华人民共和国教育法》中关于受教育者权利的相关规定。", answers)
	if got != "《中华人民共和国教育法》" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeDirectoryScopesBeforeIdentityDedup(t *testing.T) {
	items := []directoryWindowEntry{
		{windowIndex: 1, order: 0, entry: domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"专题一 教育法律基础", "一、单项选择题"}, SourcePageStart: 100}},
		{windowIndex: 1, order: 1, entry: domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"二、多项选择题"}, SourceQuestionNo: 7, SourcePageStart: 101}},
		{windowIndex: 2, order: 2, entry: domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"专题二 依法执教与教师违法(侵权)行为", "一、单项选择题"}, SourcePageStart: 102}},
		{windowIndex: 2, order: 3, entry: domain.VisionDirectoryEntry{ContentRole: "explanation", SectionPath: []string{"二、多项选择题"}, SourceQuestionNo: 7, SourcePageStart: 103}},
	}
	normalized := normalizeDirectoryScopes(items)
	if got := normalized[1].entry.SectionPath; len(got) != 2 || got[0] != "专题一教育法律基础" || got[1] != "二、多项选择题" {
		t.Fatalf("first inherited scope: %#v", got)
	}
	if got := normalized[3].entry.SectionPath; len(got) != 2 || got[0] != "专题二依法执教与教师违法(侵权)行为" || got[1] != "二、多项选择题" {
		t.Fatalf("second inherited scope: %#v", got)
	}
	if canonicalSectionScope(normalized[1].entry.SectionPath) == canonicalSectionScope(normalized[3].entry.SectionPath) {
		t.Fatal("continuation entries from different topics must remain distinct")
	}
}

func TestNormalizeDirectoryScopesUsesDurableScopeForEmptyContinuation(t *testing.T) {
	items := []directoryWindowEntry{
		{order: 0, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"五、简答题"}, SectionScopeID: "依法执教与教师违法(侵权)行为", SourceQuestionNo: 1, SourcePageStart: 16, SourcePageEnd: 16}},
		{order: 1, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"四、辨析题"}, SectionScopeID: "依法执教与教师违法(侵权)行为", SourceQuestionNo: 1, SourcePageStart: 16, SourcePageEnd: 16}},
		{order: 2, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"六、案例分析题"}, SectionScopeID: "依法执教与教师违法(侵权)行为", SourceQuestionNo: 1, SourcePageStart: 16, SourcePageEnd: 17}},
		{order: 3, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: nil, SectionScopeID: "依法执教与教师违法(侵权)行为", SourceQuestionNo: 2, SourcePageStart: 17, SourcePageEnd: 17}},
	}
	got := normalizeDirectoryScopes(items)
	if len(got[3].entry.SectionPath) != 2 || got[3].entry.SectionPath[1] != "六、案例分析题" {
		t.Fatalf("empty continuation should inherit the covering case heading: %#v", got[3].entry.SectionPath)
	}
}

func TestDirectoryEntryFillsOnlyInternalCandidateGap(t *testing.T) {
	numbers := map[string]map[int]bool{
		"scope|0|single_choice": {1: true, 6: true},
	}
	missing := domain.VisionDirectoryEntry{SectionPath: []string{"scope", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 3}
	if !directoryEntryFillsCandidateGap(missing, numbers) {
		t.Fatal("q3 should fill the bracketed 1..6 gap")
	}
	tail := missing
	tail.SourceQuestionNo = 7
	if directoryEntryFillsCandidateGap(tail, numbers) {
		t.Fatal("unbounded tail rows must not be promoted from legacy directory noise")
	}
}

func TestDirectoryEntriesExtendingCandidateTail(t *testing.T) {
	candidates := []domain.VisionDirectoryEntry{
		{ContentRole: "question", SectionPath: []string{"专题一 教育法律基础", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 20, SourcePageEnd: 20},
		{ContentRole: "question", SectionPath: []string{"专题一 教育法律基础", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 2, SourcePageStart: 20, SourcePageEnd: 20},
	}
	items := []directoryWindowEntry{
		{windowIndex: 0, entry: candidates[0]},
		{windowIndex: 1, entry: candidates[1]},
		{windowIndex: 2, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"专题一 教育法律基础", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 3, SourcePageStart: 21, SourcePageEnd: 21}},
		{windowIndex: 3, entry: domain.VisionDirectoryEntry{ContentRole: "question", SectionPath: []string{"专题一 教育法律基础", "一、单项选择题"}, QuestionType: "single_choice", SourceQuestionNo: 4, SourcePageStart: 21, SourcePageEnd: 21}},
	}
	for i := range items {
		items[i].entry.SectionScopeID = "专题一教育法律基础"
	}
	physical := map[string]bool{directoryPhysicalKey(candidates[0]): true, directoryPhysicalKey(candidates[1]): true}
	numbers := map[string]map[int]bool{directoryQuestionGroup(candidates[0]): {1: true, 2: true}}
	got := directoryEntriesExtendingCandidateTail(items, 2, candidates, physical, numbers)
	if got[directoryPhysicalKey(items[2].entry)].ContentRole == "" || got[directoryPhysicalKey(items[3].entry)].ContentRole == "" {
		t.Fatalf("contiguous tail was not promoted: %#v", got)
	}
	far := items[2].entry
	far.SourcePageStart = 30
	far.SourcePageEnd = 30
	items[2].entry = far
	got = directoryEntriesExtendingCandidateTail(items, 2, candidates, physical, numbers)
	if got[directoryPhysicalKey(far)].ContentRole != "" {
		t.Fatal("non-adjacent raw rows must not be promoted")
	}
}

func TestParseDirectoryUnitKey(t *testing.T) {
	start, end, ok := parseDirectoryUnitKey("0137-0141")
	if !ok || start != 137 || end != 141 {
		t.Fatalf("got %d-%d ok=%v", start, end, ok)
	}
	if _, _, ok := parseDirectoryUnitKey("catalog-review"); ok {
		t.Fatal("review unit key must not be treated as a page range")
	}
}

func TestExplicitAnswerFromDirectoryAnchor(t *testing.T) {
	values, analysis, ok := explicitAnswerFromAnchor("7.BCD【解析】依法执教就是要求教师依法规范教育活动。", 7, "multiple_choice")
	if !ok || len(values) != 3 || values[0] != "B" || values[1] != "C" || values[2] != "D" || analysis == "" {
		t.Fatalf("got values=%v analysis=%q ok=%v", values, analysis, ok)
	}
	values, _, ok = explicitAnswerFromAnchor("8.A、C、D【解析】", 8, "multiple_choice")
	if !ok || strings.Join(values, "") != "ACD" {
		t.Fatalf("separator answer=%v ok=%v", values, ok)
	}
	values, _, ok = explicitAnswerFromAnchor("6. √ 【解析】学生有休息权和娱乐权。", 6, "true_false")
	if !ok || len(values) != 1 || values[0] != "正确" {
		t.Fatalf("true/false answer=%v ok=%v", values, ok)
	}
	values, analysis, ok = explicitAnswerFromAnchor("1. ABD 【解析】所谓监护，是对未成年人的保护。", 1, "subjective")
	if !ok || strings.Join(values, "") != "ABD" || analysis == "" {
		t.Fatalf("subjective rubric answer=%v analysis=%q ok=%v", values, analysis, ok)
	}
	if _, _, ok = explicitAnswerFromAnchor("17. 根据相关法律法规可知", 17, "single_choice"); ok {
		t.Fatal("explanation prose without an explicit answer must not be recovered")
	}
}
