package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestResolvePDFToPPMUsesConfiguredExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pdftoppm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QT_PDFTOPPM_PATH", path)
	resolved, err := resolvePDFToPPM()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolved %q, want %q", resolved, path)
	}
}

func TestBuildVisionWindowsUsesFourOwnedPlusOneOverlap(t *testing.T) {
	windows := buildVisionWindows(10)
	want := []visionPageWindow{
		{Index: 1, OwnedStart: 1, OwnedEnd: 4, ContextStart: 1, ContextEnd: 5},
		{Index: 2, OwnedStart: 5, OwnedEnd: 8, ContextStart: 5, ContextEnd: 9},
		{Index: 3, OwnedStart: 9, OwnedEnd: 10, ContextStart: 9, ContextEnd: 10},
	}
	if len(windows) != len(want) {
		t.Fatalf("got %d windows", len(windows))
	}
	for i := range want {
		if windows[i] != want[i] {
			t.Fatalf("window %d: got %#v want %#v", i, windows[i], want[i])
		}
	}
}

func TestDirectoryOwnershipRejectsQuestionStartingOnOverlap(t *testing.T) {
	window := visionPageWindow{OwnedStart: 1, OwnedEnd: 4, ContextStart: 1, ContextEnd: 5}
	entry := domain.VisionDirectoryEntry{ContentRole: "question", SourceQuestionNo: 17, SourcePageStart: 5, SourcePageEnd: 5}
	if validateDirectoryEntry(entry, window) == nil {
		t.Fatal("question beginning on overlap page must belong to the next window")
	}
	entry.SourcePageStart, entry.SourcePageEnd = 4, 5
	if err := validateDirectoryEntry(entry, window); err != nil {
		t.Fatalf("cross-page question beginning on owned page should pass: %v", err)
	}
}

func TestDirectoryGapGroupingDoesNotMixRestartedSections(t *testing.T) {
	entries := []domain.VisionDirectoryEntry{
		{SectionPath: []string{"专题一"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 2},
		{SectionPath: []string{"专题一"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 3, SourcePageStart: 3},
		{SectionPath: []string{"专题一"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 4, SourcePageStart: 4},
		{SectionPath: []string{"专题二"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 1, SourcePageStart: 9},
		{SectionPath: []string{"专题二"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 2, SourcePageStart: 10},
		{SectionPath: []string{"专题二"}, SetNo: 1, QuestionType: "single_choice", SourceQuestionNo: 3, SourcePageStart: 11},
	}
	issues := detectVisionDirectoryGaps(entries)
	if len(issues) != 1 || issues[0]["code"] != "DIRECTORY_NUMBER_GAP" {
		t.Fatalf("expected one gap isolated to 专题一, got %#v", issues)
	}
}

func TestVisionQuestionRejectsAnswerOutsideOptions(t *testing.T) {
	q := domain.VisionQuestion{Stem: "题干", QuestionType: "single_choice", Options: []domain.QuestionOption{{Key: "A", Content: "甲"}, {Key: "B", Content: "乙"}}, Answer: []string{"D"}, SourcePageStart: 1, SourcePageEnd: 1}
	issues := validateVisionQuestion(q)
	if len(issues) != 1 || issues[0]["code"] != "ANSWER_INVALID" {
		t.Fatalf("expected invalid answer issue, got %#v", issues)
	}
}

func TestValidateTrueFalseDoesNotRequireChoiceOptions(t *testing.T) {
	issues := validateVisionQuestion(domain.VisionQuestion{Stem: "陈述", QuestionType: "true_false", Answer: []string{"正确"}})
	for _, current := range issues {
		if current["code"] == "OPTIONS_REQUIRED" {
			t.Fatalf("true/false question should not require A/B options: %#v", issues)
		}
	}
}
