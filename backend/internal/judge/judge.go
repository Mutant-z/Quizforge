package judge

import (
	"sort"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// Result 判题结果。
type Result struct {
	IsCorrect bool
	Score     float64
	Correct   []string
	Detail    string // 判题说明（主观题等）
}

// Judge 本地确定性判题：单选/多选/判断/填空。
// 主观题（简答/论述/编程）返回 needs_llm，由上层决定是否调用 LLM 评分。
func Judge(q *domain.Question, userAnswer []string) Result {
	switch q.Type {
	case domain.QuestionTypeSingleChoice, domain.QuestionTypeMultipleChoice:
		return judgeChoice(q.Answer, userAnswer)
	case domain.QuestionTypeTrueFalse:
		return judgeChoice(q.Answer, userAnswer)
	case domain.QuestionTypeFillBlank:
		return judgeFillBlank(q.Answer, userAnswer)
	default:
		return Result{IsCorrect: false, Score: 0, Correct: q.Answer, Detail: "主观题需要 AI 评分"}
	}
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	// 全角转半角
	r := []rune(s)
	for i, ch := range r {
		if ch >= 0xFF01 && ch <= 0xFF5E {
			r[i] = ch - 0xFEE0
		}
	}
	s = string(r)
	// 去掉常见标点
	s = strings.Map(func(ch rune) rune {
		switch ch {
		case '，', '。', '、', '；', '：', '！', '？', '.', ',', ';', ':', '!', '?', '"', '\'', '（', '）', '(', ')', '“', '”', '‘', '’':
			return ' '
		}
		return ch
	}, s)
	return strings.Join(strings.Fields(s), "")
}

func judgeChoice(correct, user []string) Result {
	if len(user) == 0 {
		return Result{IsCorrect: false, Score: 0, Correct: correct}
	}
	normCorrect := normalizeSet(correct)
	normUser := normalizeSet(user)
	ok := equalSet(normCorrect, normUser)
	score := 0.0
	if ok {
		score = 1
	}
	return Result{IsCorrect: ok, Score: score, Correct: correct}
}

// judgeFillBlank 填空：支持多个 alternative answers；归一化后比对。
func judgeFillBlank(correct, user []string) Result {
	if len(user) == 0 {
		return Result{IsCorrect: false, Score: 0, Correct: correct}
	}
	correctNorm := normalize(correct[0])
	userNorm := normalize(user[0])
	ok := correctNorm == userNorm
	if !ok && len(correct) > 1 {
		for _, alt := range correct[1:] {
			if normalize(alt) == userNorm {
				ok = true
				break
			}
		}
	}
	score := 0.0
	if ok {
		score = 1
	}
	return Result{IsCorrect: ok, Score: score, Correct: correct}
}

func normalizeSet(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, normalize(it))
	}
	sort.Strings(out)
	return out
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
