package service

import (
	"testing"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestParseWrongImportAutoExtractionQuestionPage(t *testing.T) {
	payload, err := parseWrongImportAutoExtraction(`{
		"document_type":"questions",
		"questions":[{"seq":1,"type":"single_choice","stem":"教育法律责任的类型有（ ）","options":[{"key":"A","content":"违约责任"}],"answer":["A"]}],
		"answers":[]
	}`)
	if err != nil {
		t.Fatalf("parse auto question page: %v", err)
	}
	if payload.DocumentType != domain.WrongImportExtractionQuestions || len(payload.Questions) != 1 {
		t.Fatalf("unexpected question payload: %+v", payload)
	}
	if payload.Questions[0].Answer[0] != "A" {
		t.Fatalf("question answer was not preserved: %+v", payload.Questions[0])
	}
}

func TestParseWrongImportAutoExtractionAnswerPage(t *testing.T) {
	payload, err := parseWrongImportAutoExtraction(`{
		"document_type":"answer_key",
		"questions":[],
		"answers":[{"seq":1,"question_hint":"教育法律责任的类型有（ ）","answer":["A"],"analysis":"","confidence":0.95}]
	}`)
	if err != nil {
		t.Fatalf("parse auto answer page: %v", err)
	}
	if payload.DocumentType != domain.WrongImportExtractionAnswerKey || len(payload.Answers) != 1 {
		t.Fatalf("unexpected answer payload: %+v", payload)
	}
}

func TestParseWrongImportAutoExtractionInfersIrrelevantPage(t *testing.T) {
	payload, err := parseWrongImportAutoExtraction(`{"document_type":"irrelevant","questions":[],"answers":[]}`)
	if err != nil {
		t.Fatalf("parse auto irrelevant page: %v", err)
	}
	if payload.DocumentType != "irrelevant" {
		t.Fatalf("unexpected irrelevant payload: %+v", payload)
	}
}
