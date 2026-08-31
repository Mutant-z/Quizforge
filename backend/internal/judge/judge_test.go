package judge

import (
	"testing"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestJudgeSingleChoice(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeSingleChoice, Answer: []string{"B"}}
	if r := Judge(q, []string{"B"}); !r.IsCorrect {
		t.Fatal("B should be correct")
	}
	if r := Judge(q, []string{"b"}); !r.IsCorrect {
		t.Fatal("lowercase b should be correct")
	}
	if r := Judge(q, []string{"A"}); r.IsCorrect {
		t.Fatal("A should be wrong")
	}
}

func TestJudgeMultipleChoice(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeMultipleChoice, Answer: []string{"A", "C"}}
	if r := Judge(q, []string{"A", "C"}); !r.IsCorrect {
		t.Fatal("AC should be correct")
	}
	if r := Judge(q, []string{"C", "A"}); !r.IsCorrect {
		t.Fatal("order should not matter")
	}
	if r := Judge(q, []string{"A"}); r.IsCorrect {
		t.Fatal("partial should be wrong")
	}
}

func TestJudgeFillBlank(t *testing.T) {
	q := &domain.Question{Type: domain.QuestionTypeFillBlank, Answer: []string{"Monitor", "监视器锁"}}
	if r := Judge(q, []string{"Monitor"}); !r.IsCorrect {
		t.Fatal("Monitor should match")
	}
	if r := Judge(q, []string{"监视器锁"}); !r.IsCorrect {
		t.Fatal("alternative should match")
	}
	if r := Judge(q, []string{"Lock"}); r.IsCorrect {
		t.Fatal("Lock should be wrong")
	}
}
