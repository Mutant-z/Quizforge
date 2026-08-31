package judge

import (
	"testing"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestJudgeTrueFalse(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeTrueFalse, Answer: []string{"A"}}
	if r := Judge(q, []string{"A"}); !r.IsCorrect {
		t.Fatal("A should be correct")
	}
	if r := Judge(q, []string{"B"}); r.IsCorrect {
		t.Fatal("B should be wrong")
	}
}

func TestJudgeEmptyAnswer(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeSingleChoice, Answer: []string{"A"}}
	if r := Judge(q, nil); r.IsCorrect {
		t.Fatal("empty answer should be wrong")
	}
}

func TestJudgeSubjective(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeShortAnswer, Answer: []string{"..."}}
	if r := Judge(q, []string{"..."}); r.IsCorrect {
		t.Fatal("subjective should need LLM")
	}
}
