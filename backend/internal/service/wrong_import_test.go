package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

func TestParseUserAnswersDeterministic(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 11, SeqNo: 1, QuestionType: domain.QuestionTypeSingleChoice, Options: []domain.QuestionOption{{Key: "A"}, {Key: "B"}, {Key: "C"}}},
		{ID: 12, SeqNo: 2, QuestionType: domain.QuestionTypeTrueFalse},
		{ID: 13, SeqNo: 3, QuestionType: domain.QuestionTypeMultipleChoice, Options: []domain.QuestionOption{{Key: "A"}, {Key: "B"}, {Key: "C"}, {Key: "D"}}},
		{ID: 14, SeqNo: 4, QuestionType: domain.QuestionTypeShortAnswer},
	}
	matched := parseUserAnswersDeterministic(drafts, "1. B\n第2题：对\n3、ACD\n4 无需答案")
	if len(matched) != 4 {
		t.Fatalf("expected 4 matched, got %d: %v", len(matched), matched)
	}
	if got := matched[11]; len(got.Answer) != 1 || got.Answer[0] != "B" {
		t.Fatalf("draft 11 answer = %v", got.Answer)
	}
	if got := matched[12]; len(got.Answer) != 1 || got.Answer[0] != "对" {
		t.Fatalf("draft 12 answer = %v", got.Answer)
	}
	if got := matched[13]; len(got.Answer) != 3 {
		t.Fatalf("draft 13 answer = %v", got.Answer)
	}
	if got := matched[14]; !got.NoAnswerNeeded {
		t.Fatalf("draft 14 should be no-answer-needed")
	}
}

func TestParseUserAnswersDeterministicRejectsInvalidKeys(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 21, SeqNo: 1, QuestionType: domain.QuestionTypeSingleChoice, Options: []domain.QuestionOption{{Key: "A"}, {Key: "B"}}},
	}
	matched := parseUserAnswersDeterministic(drafts, "1.E")
	if len(matched) != 0 {
		t.Fatalf("invalid option key should not match, got %v", matched)
	}
	matched = parseUserAnswersDeterministic(drafts, "9.B")
	if len(matched) != 0 {
		t.Fatalf("unknown seq should not match, got %v", matched)
	}
}

func TestParseUserAnswersDeterministicFreeText(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 31, SeqNo: 1, QuestionType: domain.QuestionTypeFillBlank},
		{ID: 32, SeqNo: 2, QuestionType: domain.QuestionTypeShortAnswer},
		{ID: 33, SeqNo: 3, QuestionType: domain.QuestionTypeFillBlank},
	}
	matched := parseUserAnswersDeterministic(drafts, "1.癸卯学制\n第2题：教育即生活、学校即社会。\n3、无需答案")
	if len(matched) != 3 {
		t.Fatalf("expected 3 matched, got %d: %v", len(matched), matched)
	}
	if got := matched[31]; len(got.Answer) != 1 || got.Answer[0] != "癸卯学制" {
		t.Fatalf("draft 31 answer = %v", got.Answer)
	}
	if got := matched[32]; len(got.Answer) != 2 || got.Answer[0] != "教育即生活" || got.Answer[1] != "学校即社会" {
		t.Fatalf("draft 32 answer = %v（句号应剥离、顿号应分空）", got.Answer)
	}
	if got := matched[33]; !got.NoAnswerNeeded {
		t.Fatalf("draft 33 should be no-answer-needed, got %v", got)
	}
}

func TestInterpretUserAnswerTokenTrueFalseVariants(t *testing.T) {
	draft := &domain.WrongImportDraft{QuestionType: domain.QuestionTypeTrueFalse}
	cases := map[string]string{"√": "对", "T": "对", "正确": "对", "×": "错", "F": "错", "错误": "错"}
	for token, want := range cases {
		answer, _, ok := interpretUserAnswerToken(token, draft)
		if !ok || answer[0] != want {
			t.Fatalf("token %q: got %v ok=%v, want %s", token, answer, ok, want)
		}
	}
}

func TestWrongImportStemHashNormalization(t *testing.T) {
	a := wrongImportStemHash(domain.QuestionTypeSingleChoice, "下列说法 正确的是？（ ）", []domain.QuestionOption{{Key: "A", Content: "对立统一 规律。"}, {Key: "B", Content: "质量互变"}})
	b := wrongImportStemHash(domain.QuestionTypeSingleChoice, "下列说法正确的是", []domain.QuestionOption{{Key: "a", Content: "对立统一规律"}, {Key: "B", Content: "质量互变"}})
	if a != b {
		t.Fatalf("hash should ignore whitespace/punct/case:\n%s\n%s", a, b)
	}
	c := wrongImportStemHash(domain.QuestionTypeSingleChoice, "完全不同的题干", nil)
	if a == c {
		t.Fatalf("different stems must hash differently")
	}
	withNumber := wrongImportStemHash(domain.QuestionTypeShortAnswer, "1. 简述教育法律关系中的几种客体。", nil)
	withoutNumber := wrongImportStemHash(domain.QuestionTypeShortAnswer, "简述教育法律关系中的几种客体。", nil)
	if withNumber != withoutNumber {
		t.Fatalf("question number should not change stem hash")
	}
}

func TestParseWrongImportExtraction(t *testing.T) {
	raw := `以下是结果：
{"questions":[
  {"seq":1,"type":"single","stem":"题干一","options":[{"key":"A","content":"x"},{"key":"B","content":""}],"answer":["B"],"analysis":"","region":{"left":0,"top":0,"right":500,"bottom":200},"confidence":0.9},
  {"seq":2,"type":"简答题","stem":"题干二","options":[],"answer":[],"analysis":"解析内容"},
  {"seq":3,"type":"fill_in_blank","stem":"题干三","options":[],"answer":["答案1"]}
],}`
	questions, err := parseWrongImportExtraction(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(questions) != 3 {
		t.Fatalf("expected 3 questions, got %d", len(questions))
	}
	if questions[0].Type != domain.QuestionTypeSingleChoice {
		t.Fatalf("single should normalize, got %q", questions[0].Type)
	}
	if len(questions[0].Options) != 1 {
		t.Fatalf("empty option should be removed, got %v", questions[0].Options)
	}
	if questions[1].Type != domain.QuestionTypeShortAnswer {
		t.Fatalf("subjective should map to short_answer, got %q", questions[1].Type)
	}
	if questions[2].Type != domain.QuestionTypeFillBlank {
		t.Fatalf("fill_in_blank should map to fill_blank, got %q", questions[2].Type)
	}
}

func TestWrongImportMaterialAnalysisTypeIsPreserved(t *testing.T) {
	raw := `{"questions":[{"seq":20,"type":"案例分析题","stem":"王某和陆某发生事故。本案应承担责任的主体有哪些？（1）学校承担什么责任？（2）陆某承担什么责任？","options":[],"answer":[]}]}`
	questions, err := parseWrongImportExtraction(raw)
	if err != nil {
		t.Fatalf("parse material analysis failed: %v", err)
	}
	if questions[0].Type != domain.QuestionTypeMaterialAnalysis {
		t.Fatalf("material analysis type was collapsed: %q", questions[0].Type)
	}
}

func TestParseWrongImportAnswerExtraction(t *testing.T) {
	raw := `识别结果：
{"document_type":"answer_key","answers":[
  {"seq":1,"question_hint":"简述教育法律关系中的几种客体。","answer":["教育法律关系的客体一般包括物质财富、非物质财富、行为三个大的方面。"],"analysis":"","confidence":0.98},
  {"seq":2,"question_hint":"2. 简述教师申诉的范围及其受理机关。","answer":["根据《中华人民共和国教师法》的规定，教师申诉的范围包括……"],"analysis":"","confidence":0.97}
]}`
	payload, err := parseWrongImportAnswerExtraction(raw)
	if err != nil {
		t.Fatalf("parse answer extraction failed: %v", err)
	}
	if len(payload.Answers) != 2 || payload.Answers[0].Seq != 1 || len(payload.Answers[1].Answer) != 1 {
		t.Fatalf("unexpected answer payload: %+v", payload.Answers)
	}
}

func TestMatchWrongImportAnswerEntryUsesQuestionHintBeforeNumber(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 88, SeqNo: 18, Stem: "简述教育法律关系中的几种客体。"},
		{ID: 89, SeqNo: 19, Stem: "2. 简述教师申诉的范围及其受理机关。"},
	}
	used := map[int64]bool{}
	draft, score := matchWrongImportAnswerEntry(wrongImportAnswerEntry{
		Seq:          1,
		QuestionHint: "简述教育法律关系中的几种客体。",
	}, drafts, used)
	if draft == nil || draft.ID != 88 || score < 0.99 {
		t.Fatalf("question hint should win over unrelated seq: draft=%v score=%.2f", draft, score)
	}

	used[draft.ID] = true
	draft, score = matchWrongImportAnswerEntry(wrongImportAnswerEntry{
		Seq:          2,
		QuestionHint: "2. 简述教师申诉的范围及其受理机关。",
	}, drafts, used)
	if draft == nil || draft.ID != 89 || score < 0.99 {
		t.Fatalf("numbered question hint should match after normalization: draft=%v score=%.2f", draft, score)
	}
}

func TestMatchWrongImportAnswerEntryExactHintBeatsEvidenceTie(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{
			ID:           91,
			SeqNo:        1,
			QuestionType: domain.QuestionTypeMaterialAnalysis,
			Stem:         "教育法律关系中的几种客体包括物质财富、非物质财富和行为，这是一道材料分析题。",
		},
		{
			ID:     92,
			SeqNo:  18,
			Stem:   "四、简答题\n1. 简述教育法律关系中的几种客体。",
			Answer: []string{},
		},
	}
	draft, score := matchWrongImportAnswerEntry(wrongImportAnswerEntry{
		Seq:          1,
		QuestionHint: "简述教育法律关系中的几种客体。",
		Answer:       []string{"教育法律关系中的几种客体包括物质财富、非物质财富和行为。"},
	}, drafts, map[int64]bool{})
	if draft == nil || draft.ID != 92 || score < 0.99 {
		t.Fatalf("exact question hint should beat unrelated evidence tie: draft=%v score=%.2f", draft, score)
	}
}

func TestMatchWrongImportAnswerEntryRejectsWeakHint(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 1, SeqNo: 1, Stem: "简述教育法律关系中的几种客体。"},
		{ID: 2, SeqNo: 2, Stem: "简述教师申诉的范围及其受理机关。"},
	}
	draft, score := matchWrongImportAnswerEntry(wrongImportAnswerEntry{
		QuestionHint: "简述。",
	}, drafts, map[int64]bool{})
	if draft != nil || score != 0 {
		t.Fatalf("weak hint must not auto-match: draft=%v score=%.2f", draft, score)
	}
}

func TestMatchWrongImportAnswerEntryIgnoresSectionNumber(t *testing.T) {
	drafts := []*domain.WrongImportDraft{
		{ID: 120, SeqNo: 4, QuestionType: domain.QuestionTypeSingleChoice, Stem: "5.（ ）是教育法律关系发生、变更和消灭的根据。"},
		{ID: 136, SeqNo: 20, QuestionType: domain.QuestionTypeMaterialAnalysis, Stem: "王某和陆某是某小学同班同学。班主任没有采取措施，导致王某左眼失明。王某家长将陆某和学校告上法庭。（1）本案涉及的教育法律关系主体有哪些？（2）本案应承担责任的主体有哪些？各自承担什么责任？"},
	}
	entry := wrongImportAnswerEntry{
		Seq:          5,
		QuestionHint: "五、案例分析题（参考答案）",
		SectionTitle: "案例分析题（参考答案）",
		Answer:       []string{"本案涉及的教育法律关系主体有学校、班主任、陆某及其监护人、王某及其监护人。班主任没有及时采取措施，学校和相关人员应承担相应责任。"},
	}
	draft, score := matchWrongImportAnswerEntry(entry, drafts, map[int64]bool{})
	if draft == nil || draft.ID != 136 || score < 0.92 {
		t.Fatalf("section heading should match material draft by body evidence: draft=%v score=%.2f", draft, score)
	}
	noBody := entry
	noBody.Answer = []string{"参考答案"}
	draft, score = matchWrongImportAnswerEntry(noBody, drafts, map[int64]bool{})
	if draft != nil || score != 0 {
		t.Fatalf("section number alone must not match a question: draft=%v score=%.2f", draft, score)
	}
}

func TestMergeAnswerTextRemovesContinuationOverlap(t *testing.T) {
	got := mergeAnswerText([]string{"学校应承担相应责任，班主任应及时采取措施"}, []string{"采取措施，通知家长。"})
	if len(got) != 1 || got[0] != "学校应承担相应责任，班主任应及时采取措施，通知家长。" {
		t.Fatalf("unexpected answer continuation merge: %v", got)
	}
}

func TestWrongImportAnswerBatchReconcilesMaterialContinuation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open("/tmp/qt_wrong_answer_reconcile_test_"+uuidStr()+".db", 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "/Users/mutant/Documents/project/QuizForge/backend/migrations"); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	hash, _ := security.HashPassword("secret")
	user, err := repo.CreateUser(ctx, "answer-reconcile-test-"+uuidStr(), "answer-reconcile-"+uuidStr()+"@test.local", hash)
	if err != nil {
		t.Fatal(err)
	}
	title := "答案续图测试"
	session, err := repo.CreateWrongImportSession(ctx, user.ID, &title, nil)
	if err != nil {
		t.Fatal(err)
	}
	questionImage, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, FilePath: "question.png", FileHash: "question-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	noiseImage, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, FilePath: "noise.png", FileHash: "noise-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	target, err := repo.CreateWrongImportDraft(ctx, &domain.WrongImportDraft{
		SessionID: session.ID, ImageID: questionImage.ID, SeqNo: 20, QuestionType: domain.QuestionTypeMaterialAnalysis,
		Stem:         "王某和陆某发生事故，班主任没有采取措施，导致王某左眼失明。（1）本案涉及的教育法律关系主体有哪些？（2）本案应承担责任的主体有哪些？各自承担什么责任？",
		AnswerSource: domain.WrongImportAnswerSourceNone, AnswerStatus: domain.WrongImportAnswerStatusMissing,
		ReviewStatus: domain.WrongImportDraftReviewMissingAnswer,
	})
	if err != nil {
		t.Fatal(err)
	}
	noise, err := repo.CreateWrongImportDraft(ctx, &domain.WrongImportDraft{
		SessionID: session.ID, ImageID: noiseImage.ID, SeqNo: 4, QuestionType: domain.QuestionTypeSingleChoice,
		Stem: "5.（ ）是教育法律关系发生、变更和消灭的根据。", Answer: []string{"本案涉及的教育法律关系主体有学校、班主任、陆某及其监护人、王某及其监护人。班主任应及时采取措施"},
		AnswerSource: domain.WrongImportAnswerSourceImage, ContentVersion: 1,
		AnswerStatus: domain.WrongImportAnswerStatusMissing, ReviewStatus: domain.WrongImportDraftReviewMissingAnswer,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := repo.EnsureWrongImportImageBatch(ctx, "answer-batch-"+uuidStr(), session.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	zero, one := 0, 1
	answerImage1, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, BatchID: batch.ID, BatchIndex: &zero, ExtractionMode: domain.WrongImportExtractionAnswerKey, FilePath: "answer-1.png", FileHash: "answer-1-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	answerImage2, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, BatchID: batch.ID, BatchIndex: &one, ExtractionMode: domain.WrongImportExtractionAnswerKey, FilePath: "answer-2.png", FileHash: "answer-2-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	first := &domain.WrongImportAnswerFragment{
		SessionID: session.ID, ImageID: answerImage1.ID, BatchID: batch.ID, BatchIndex: &zero, FragmentIndex: 0,
		SectionTitle: "案例分析题（参考答案）", Seq: 5, QuestionHint: "五、案例分析题（参考答案）",
		Answer:        []string{"本案涉及的教育法律关系主体有学校、班主任、陆某及其监护人、王某及其监护人。班主任应及时采取措施"},
		EndsMidAnswer: true, Status: "unresolved", FragmentRole: "source",
	}
	if _, err := repo.CreateWrongImportAnswerFragment(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := &domain.WrongImportAnswerFragment{
		SessionID: session.ID, ImageID: answerImage2.ID, BatchID: batch.ID, BatchIndex: &one, FragmentIndex: 0,
		Answer: []string{"，通知家长并承担相应责任。"}, StartsMidAnswer: true, Status: "unresolved", FragmentRole: "source",
	}
	if _, err := repo.CreateWrongImportAnswerFragment(ctx, second); err != nil {
		t.Fatal(err)
	}
	service := &WrongImportService{repo: repo}
	if err := service.reconcileAnswerFragments(ctx, session.ID, batch.ID); err != nil {
		t.Fatal(err)
	}
	updatedTarget, err := repo.GetWrongImportDraft(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedTarget.QuestionType != domain.QuestionTypeMaterialAnalysis || len(updatedTarget.Answer) != 1 || !strings.Contains(updatedTarget.Answer[0], "通知家长") || updatedTarget.AnswerStatus != domain.WrongImportAnswerStatusProvided {
		t.Fatalf("material answer was not reconciled: %+v", updatedTarget)
	}
	updatedNoise, err := repo.GetWrongImportDraft(ctx, noise.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedNoise.Answer) != 0 || updatedNoise.AnswerSource != domain.WrongImportAnswerSourcePending || updatedNoise.AnswerStatus != domain.WrongImportAnswerStatusMissing {
		t.Fatalf("stale machine answer should be cleared from unrelated question: %+v", updatedNoise)
	}
	fragments, err := repo.ListWrongImportAnswerFragmentsByBatch(ctx, session.ID, batch.ID)
	if err != nil || len(fragments) != 2 || fragments[0].Status != "matched" || fragments[1].Status != "matched" {
		t.Fatalf("answer fragments were not marked matched: err=%v fragments=%+v", err, fragments)
	}
}

func TestWrongImportAnswerIntentAndLegacyUploadDetection(t *testing.T) {
	if !isWrongImportAnswerExtractionRequest("帮我识别这里的答案") {
		t.Fatal("answer extraction request was not recognized")
	}
	if isWrongImportAnswerExtractionRequest("我的答案是 A") {
		t.Fatal("ordinary user answer must not be treated as answer image extraction")
	}
	image := &domain.WrongImportImage{ID: 42, OriginalName: "answers.png"}
	images := []*domain.WrongImportImage{image}
	messages := []*domain.WrongImportMessage{
		{Role: "user", Content: "上传了错题图片「answers.png」，请提取其中的题目"},
		{Role: "agent", Content: "正在识别图片，请稍候"},
		{Role: "user", Content: "帮我识别这里的答案"},
	}
	if got := recentWrongImportAnswerImage(images, messages); got != image {
		t.Fatalf("expected the latest legacy upload image, got %+v", got)
	}
}

func TestWrongImportBoundaryAndGoldenCrossImageMerge(t *testing.T) {
	left := &domain.WrongImportDraft{
		ID: 20, ImageID: 100, QuestionType: domain.QuestionTypeShortAnswer,
		Stem:       "王某病情暂时稳定下来，但随时可能再次发作，甚至可能导致双目",
		Warnings:   []string{"题目内容被截断，缺少后续问题设问"},
		Confidence: 0.96,
	}
	right := &domain.WrongImportDraft{
		ID: 21, ImageID: 101, QuestionType: domain.QuestionTypeShortAnswer,
		Stem:       "失明。（1）本案涉及的教育法律关系主体有哪些？（2）本案应承担责任的主体有哪些？各自承担什么责任？",
		Warnings:   []string{"题干开头疑似缺失截断"},
		Confidence: 0.95,
	}

	score, reason, hardSeparate := scoreWrongImportBoundary(left, right, false)
	if hardSeparate || score < wrongImportMergeAutoThreshold {
		t.Fatalf("golden case should be a strong continuation: score=%.2f reason=%s hard=%v", score, reason, hardSeparate)
	}
	apply, before, ok, mergeReason := buildWrongImportMerge(left, right)
	if !ok {
		t.Fatalf("golden case should merge: %s", mergeReason)
	}
	if !strings.Contains(apply.Target.Stem, "甚至可能导致双目失明") {
		t.Fatalf("merged stem lost the cross-image sentence: %q", apply.Target.Stem)
	}
	if strings.Count(apply.Target.Stem, "失明") != 1 {
		t.Fatalf("merged stem should not repeat the contained continuation: %q", apply.Target.Stem)
	}
	if apply.Target.Boundary == nil || apply.Target.Boundary.StartsMidQuestion || apply.Target.Boundary.EndsMidQuestion {
		t.Fatalf("merged boundary should describe the complete question: %+v", apply.Target.Boundary)
	}
	if len(apply.Target.Warnings) != 0 {
		t.Fatalf("stale boundary warnings should be removed after merge: %v", apply.Target.Warnings)
	}
	if len(apply.Target.Sources) != 2 || len(before.Target.Sources) != 1 || len(before.Source.Sources) != 1 {
		t.Fatalf("source snapshots not retained: apply=%v before=%v", apply.Target.Sources, before)
	}
}

func TestWrongImportMergeRejectsTypeAndAnswerConflicts(t *testing.T) {
	left := &domain.WrongImportDraft{ID: 1, QuestionType: domain.QuestionTypeSingleChoice, Stem: "题干", Answer: []string{"A"}}
	right := &domain.WrongImportDraft{ID: 2, QuestionType: domain.QuestionTypeMultipleChoice, Stem: "继续", Answer: []string{"B"}}
	if _, _, ok, reason := buildWrongImportMerge(left, right); ok || reason != "question_type_conflict" {
		t.Fatalf("question type conflict should require review: ok=%v reason=%s", ok, reason)
	}

	right.QuestionType = domain.QuestionTypeSingleChoice
	if _, _, ok, reason := buildWrongImportMerge(left, right); ok || reason != "answer_conflict" {
		t.Fatalf("answer conflict should require review: ok=%v reason=%s", ok, reason)
	}
}

func TestWrongImportMergeTextRemovesBoundaryOverlap(t *testing.T) {
	if got := mergeFragmentText("甚至可能导致双目", "双目失明。"); got != "甚至可能导致双目失明。" {
		t.Fatalf("unexpected overlap merge: %q", got)
	}
	leftText := "王某病情暂时稳定下来，甚至可能导致双目失明。王某的家长要求赔偿。"
	rightText := "失明。王某的家长要求赔偿。"
	if got := mergeFragmentText(leftText, rightText); got != leftText {
		t.Fatalf("contained continuation should not be appended twice: %q", got)
	}
	noBoundaryLeft := &domain.WrongImportDraft{ID: 3, QuestionType: domain.QuestionTypeShortAnswer, Stem: "第一道题已经完整结束。"}
	noBoundaryRight := &domain.WrongImportDraft{ID: 4, QuestionType: domain.QuestionTypeShortAnswer, Stem: "第二道题也已经完整结束。"}
	if score, reason, hardSeparate := scoreWrongImportBoundary(noBoundaryLeft, noBoundaryRight, true); hardSeparate || score >= wrongImportMergeReviewThreshold || reason != "no_continuation_boundary" {
		t.Fatalf("normal adjacent questions must not become a merge candidate: score=%.2f reason=%s hard=%v", score, reason, hardSeparate)
	}
	left := &domain.WrongImportDraft{ID: 1, QuestionType: domain.QuestionTypeShortAnswer, Stem: "前一题已经结束。"}
	right := &domain.WrongImportDraft{ID: 2, QuestionType: domain.QuestionTypeShortAnswer, Stem: "12. 下一道题？"}
	_, _, hardSeparate := scoreWrongImportBoundary(left, right, true)
	if !hardSeparate {
		t.Fatal("an explicit new question marker must never be auto-merged")
	}
	left.Boundary = &domain.WrongImportBoundary{EndsMidQuestion: true}
	_, _, hardSeparate = scoreWrongImportBoundary(left, right, true)
	if !hardSeparate {
		t.Fatal("an explicit new question marker must remain separate even when the left boundary is uncertain")
	}
}

func TestWrongImportLowConfidenceConflictStaysSeparate(t *testing.T) {
	decision := finalizeWrongImportReconcileDecision(wrongImportReconcileDecision{
		Decision:   "separate",
		Confidence: 0.05,
	}, false, "question_type_conflict", false)
	if decision.Decision != "separate" || decision.Confidence != 0.05 {
		t.Fatalf("low-confidence conflict should stay separate: %+v", decision)
	}

	decision = finalizeWrongImportReconcileDecision(wrongImportReconcileDecision{
		Decision:   "merge",
		Confidence: 0.82,
	}, false, "answer_conflict", false)
	if decision.Decision != "review" || decision.Confidence != wrongImportMergeReviewThreshold {
		t.Fatalf("stronger conflict should require review: %+v", decision)
	}
}

func TestWrongImportThreeImageChainUsesMergedSourceBoundary(t *testing.T) {
	first := &domain.WrongImportDraft{ID: 1, ImageID: 11, SeqNo: 1, QuestionType: domain.QuestionTypeShortAnswer, Stem: "第一段", Boundary: &domain.WrongImportBoundary{EndsMidQuestion: true}}
	second := &domain.WrongImportDraft{ID: 2, ImageID: 12, SeqNo: 2, QuestionType: domain.QuestionTypeShortAnswer, Stem: "第二段", Boundary: &domain.WrongImportBoundary{StartsMidQuestion: true, EndsMidQuestion: true}}
	third := &domain.WrongImportDraft{ID: 3, ImageID: 13, SeqNo: 3, QuestionType: domain.QuestionTypeShortAnswer, Stem: "第三段。"}
	apply, _, ok, _ := buildWrongImportMerge(first, second)
	if !ok {
		t.Fatal("first and second fragments should merge")
	}
	merged := &domain.WrongImportDraft{ID: 1, ImageID: 11, SeqNo: 1, QuestionType: apply.Target.QuestionType, Stem: apply.Target.Stem, Sources: apply.Target.Sources, MergeStatus: domain.WrongImportMergeStatusActive}
	third.MergeStatus = domain.WrongImportMergeStatusActive
	left, right := adjacentWrongImportDrafts([]*domain.WrongImportDraft{merged, third}, 12, 13)
	if len(left) != 1 || left[0].ID != 1 || len(right) != 1 || right[0].ID != 3 {
		t.Fatalf("merged target should continue from image 2: left=%v right=%v", left, right)
	}
}

func TestWrongImportBatchMergeAndUndoTransaction(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open("/tmp/qt_wrong_reconcile_test_"+uuidStr()+".db", 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "/Users/mutant/Documents/project/QuizForge/backend/migrations"); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	hash, _ := security.HashPassword("secret")
	user, err := repo.CreateUser(ctx, "wrong-import-test-"+uuidStr(), "wrong-import-"+uuidStr()+"@test.local", hash)
	if err != nil {
		t.Fatal(err)
	}
	title := "跨图测试"
	session, err := repo.CreateWrongImportSession(ctx, user.ID, &title, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := repo.EnsureWrongImportImageBatch(ctx, "batch-"+uuidStr(), session.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	index0, index1 := 0, 1
	image1, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, BatchID: batch.ID, BatchIndex: &index0, FilePath: "one.png", FileHash: "hash-one-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	image2, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, BatchID: batch.ID, BatchIndex: &index1, FilePath: "two.png", FileHash: "hash-two-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone})
	if err != nil {
		t.Fatal(err)
	}
	answerImage, err := repo.CreateWrongImportImage(ctx, &domain.WrongImportImage{SessionID: session.ID, FilePath: "answers.png", FileHash: "hash-answers-" + uuidStr(), MIMEType: "image/png", SizeBytes: 1, Status: domain.WrongImportImageDone, ExtractionMode: domain.WrongImportExtractionAnswerKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateWrongImportDraft(ctx, &domain.WrongImportDraft{SessionID: session.ID, ImageID: answerImage.ID, SeqNo: 1, QuestionType: domain.QuestionTypeShortAnswer, Stem: "答案正文不应成为新题", ReviewStatus: domain.WrongImportDraftReviewReady}); err == nil {
		t.Fatal("answer reference images must not create question drafts")
	}
	if _, err := repo.SealWrongImportImageBatch(ctx, batch.ID, session.ID, 2); err != nil {
		t.Fatal(err)
	}
	left, err := repo.CreateWrongImportDraft(ctx, &domain.WrongImportDraft{SessionID: session.ID, ImageID: image1.ID, SeqNo: 1, QuestionType: domain.QuestionTypeShortAnswer, Stem: "前半句", ReviewStatus: domain.WrongImportDraftReviewReady, Boundary: &domain.WrongImportBoundary{EndsMidQuestion: true}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := repo.CreateWrongImportDraft(ctx, &domain.WrongImportDraft{SessionID: session.ID, ImageID: image2.ID, SeqNo: 2, QuestionType: domain.QuestionTypeShortAnswer, Stem: "后半句。", ReviewStatus: domain.WrongImportDraftReviewReady, Boundary: &domain.WrongImportBoundary{StartsMidQuestion: true}})
	if err != nil {
		t.Fatal(err)
	}
	apply, before, ok, reason := buildWrongImportMerge(left, right)
	if !ok {
		t.Fatalf("build merge: %s", reason)
	}
	beforeJSON, _ := json.Marshal(before)
	// 用旧版本的错误结果模拟“后片段已经被拼接过但仍重复写入”的历史数据。
	legacyApply := apply
	legacyApply.Target.Stem = "前半句后半句。后半句。"
	legacyApply.Target.Boundary = &domain.WrongImportBoundary{EndsMidQuestion: true, EndMarker: "题目被截断"}
	legacyApply.Target.Warnings = []string{"题目被截断，缺少后续内容"}
	legacyApply.Target.Sources = []domain.WrongImportDraftSource{before.Target.Sources[0], before.Source.Sources[0]}
	legacyAfterJSON, _ := json.Marshal(legacyApply)
	candidate, err := repo.CreateWrongImportMergeCandidate(ctx, &domain.WrongImportMergeCandidate{SessionID: session.ID, BatchID: batch.ID, LeftDraftID: left.ID, RightDraftID: right.ID, Score: .98, Decision: "merge", Status: domain.WrongImportMergePending, BeforeJSON: string(beforeJSON), AfterJSON: string(legacyAfterJSON)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyWrongImportMerge(ctx, candidate.ID, legacyApply); err != nil {
		t.Fatal(err)
	}
	active, err := repo.ListWrongImportDrafts(ctx, session.ID)
	if err != nil || len(active) != 1 || active[0].ID != left.ID || len(active[0].Sources) != 2 {
		t.Fatalf("merged projection incorrect: err=%v drafts=%v", err, active)
	}
	service := &WrongImportService{repo: repo}
	if err := service.repairAcceptedWrongImportMerges(ctx, session.ID, batch.ID, true); err != nil {
		t.Fatalf("repair accepted merge: %v", err)
	}
	active, err = repo.ListWrongImportDrafts(ctx, session.ID)
	if err != nil || len(active) != 1 || active[0].Stem != "前半句后半句。" || len(active[0].Warnings) != 0 {
		t.Fatalf("repaired projection incorrect: err=%v drafts=%v", err, active)
	}
	if err := repo.UndoWrongImportMerge(ctx, candidate.ID); err != nil {
		t.Fatal(err)
	}
	active, err = repo.ListWrongImportDrafts(ctx, session.ID)
	if err != nil || len(active) != 2 {
		t.Fatalf("undo should restore two drafts: err=%v count=%d", err, len(active))
	}
	lowCandidate, err := repo.CreateWrongImportMergeCandidate(ctx, &domain.WrongImportMergeCandidate{
		SessionID: session.ID, BatchID: batch.ID, LeftDraftID: left.ID, RightDraftID: right.ID,
		Score: .05, Decision: "review", Status: domain.WrongImportMergePending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.rejectLowConfidenceMergeCandidates(ctx, session.ID, batch.ID, true); err != nil {
		t.Fatalf("reject stale low-confidence candidate: %v", err)
	}
	latest, err := repo.GetLatestWrongImportMergeCandidate(ctx, session.ID, left.ID, right.ID)
	if err != nil || latest.ID != lowCandidate.ID || latest.Status != domain.WrongImportMergeRejected {
		t.Fatalf("low-confidence candidate should be rejected: err=%v latest=%+v", err, latest)
	}
}

func TestParseWrongImportExtractionRejectsEmptyStem(t *testing.T) {
	if _, err := parseWrongImportExtraction(`{"questions":[{"seq":1,"type":"single_choice","stem":"  ","options":[]}]}`); err == nil {
		t.Fatal("empty stem should be rejected")
	}
}

func TestNormalizeWrongImageScalesDown(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	for y := 0; y < 1500; y += 3 {
		for x := 0; x < 3000; x += 3 {
			big.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, big); err != nil {
		t.Fatal(err)
	}
	w, h, data, mime, err := normalizeWrongImage(buf.Bytes(), "image/png", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if w != 2000 || h != 1000 {
		t.Fatalf("expected 2000x1000, got %dx%d", w, h)
	}
	if mime != "image/jpeg" {
		t.Fatalf("scaled image should be jpeg, got %s", mime)
	}
	if len(data) == 0 {
		t.Fatal("scaled data empty")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("scaled output must be decodable: %v", err)
	}
}

func TestNormalizeWrongImageKeepsSmallImage(t *testing.T) {
	small := image.NewRGBA(image.Rect(0, 0, 800, 600))
	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err != nil {
		t.Fatal(err)
	}
	w, h, data, mime, err := normalizeWrongImage(buf.Bytes(), "image/png", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if w != 800 || h != 600 || mime != "image/png" || len(data) == 0 {
		t.Fatalf("small image should pass through: %dx%d %s", w, h, mime)
	}
}

func TestExtractJSONObjectBalanced(t *testing.T) {
	raw := `前置说明 {"a":{"b":"}\""} } 后缀`
	got := extractJSONObject(raw)
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("unexpected extraction: %q", got)
	}
	if _, err := sanitizeJSONForTest(got); err != nil {
		t.Fatalf("extracted object not valid json: %v (%q)", err, got)
	}
}

// sanitizeJSONForTest 复用本地修复逻辑验证提取结果可解析。
func sanitizeJSONForTest(s string) (string, error) {
	fixed := trailingJSONComma.ReplaceAllString(s, "$1")
	if !json.Valid([]byte(fixed)) {
		return fixed, errors.New("invalid json")
	}
	return fixed, nil
}
