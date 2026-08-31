package review

import (
	"context"
	"math"
	"time"
)

// ReviewInput 调度输入。
type ReviewInput struct {
	WrongCount      int       // 错误次数
	CorrectCount    int       // 连续/累计答对
	ReviewCount     int       // 复习次数
	MasteryScore    float64   // 当前掌握度 0-100
	IntervalDays    float64   // 当前间隔
	Difficulty      float64   // 题目难度 1-5
	LastReviewAt    time.Time // 上次复习
	LastWrongAt     time.Time // 上次错误
	Result          string    // 本次复习结果: again/hard/good/easy
}

// ReviewResult 调度结果。
type ReviewResult struct {
	MasteryScore   float64
	IntervalDays   float64
	PriorityScore  float64
	NextReviewAt   time.Time
	Status         string // learning | reviewing | mastered
}

// ReviewScheduler 复习调度接口（可插拔）。
type ReviewScheduler interface {
	Name() string
	Calculate(ctx context.Context, input ReviewInput) (ReviewResult, error)
}

// SimpleV1 默认策略：
// 错误 → 10 分钟内复习；连续正确 → 1/3/7/15/30 天。
type SimpleV1 struct {
	// 可配置参数
	WrongWeight   float64
	OverdueWeight float64
	DifficultyWeight float64
	ForgettingWeight float64
	MasteryWeight float64
}

func NewSimpleV1() *SimpleV1 {
	return &SimpleV1{
		WrongWeight:      2.0,
		OverdueWeight:    1.5,
		DifficultyWeight: 0.5,
		ForgettingWeight: 1.0,
		MasteryWeight:    0.4,
	}
}

func (s *SimpleV1) Name() string { return "simple_v1" }

func (s *SimpleV1) Calculate(ctx context.Context, input ReviewInput) (ReviewResult, error) {
	now := time.Now().UTC()

	// 间隔阶梯
	stages := []float64{1, 3, 7, 15, 30}
	stageIdx := int(math.Min(float64(input.ReviewCount), float64(len(stages)-1)))
	interval := stages[stageIdx]

	// 再次错误：间隔大幅缩短，10 分钟
	if input.Result == "again" {
		interval = 0.007 // 约 10 分钟
	} else if input.Result == "hard" {
		interval = math.Max(0.5, interval*0.5)
	} else if input.Result == "easy" {
		interval = interval * 2
	}

	// 掌握度调整
	mastery := input.MasteryScore
	switch input.Result {
	case "again":
		mastery = math.Max(0, mastery-20)
	case "hard":
		mastery = math.Max(0, mastery-5)
	case "good":
		mastery = math.Min(100, mastery+10)
	case "easy":
		mastery = math.Min(100, mastery+15)
	}
	// 错误次数惩罚
	if input.WrongCount > 1 {
		mastery = math.Max(0, mastery-float64(input.WrongCount-1)*3)
	}

	// 优先级：错误越多、越久未复习、越难 → 越高；掌握度越高 → 越低
	overdueDays := 0.0
	if !input.LastReviewAt.IsZero() {
		overdueDays = now.Sub(input.LastReviewAt).Hours() / 24
	}
	wrongScore := math.Min(1, float64(input.WrongCount)/10)
	overdueScore := math.Min(1, overdueDays/7)
	difficultyScore := (input.Difficulty - 1) / 4
	forgettingScore := 1 - math.Min(1, float64(input.CorrectCount)/3)
	masteryScore := input.MasteryScore / 100

	priority := s.WrongWeight*wrongScore +
		s.OverdueWeight*overdueScore +
		s.DifficultyWeight*difficultyScore +
		s.ForgettingWeight*forgettingScore -
		s.MasteryWeight*masteryScore
	priority = math.Max(0, priority)

	// 状态
	status := "learning"
	if input.Result == "again" {
		status = "learning"
	} else if mastery >= 80 && input.ReviewCount >= 2 {
		status = "mastered"
	} else if input.ReviewCount > 0 {
		status = "reviewing"
	}

	next := now.Add(time.Duration(interval * 24 * float64(time.Hour)))
	return ReviewResult{
		MasteryScore:  math.Round(mastery*10) / 10,
		IntervalDays:  interval,
		PriorityScore: math.Round(priority*100) / 100,
		NextReviewAt:  next,
		Status:        status,
	}, nil
}
