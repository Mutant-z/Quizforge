package domain

import "time"

// PracticeSession 刷题会话
type PracticeSession struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	BankID           *int64     `json:"bank_id,omitempty"`
	BankIDs          []int64    `json:"bank_ids"`
	SubjectID        *int64     `json:"subject_id,omitempty"`
	ChapterID        *int64     `json:"chapter_id,omitempty"`
	Mode             string     `json:"mode"`
	TimeLimitSeconds *int       `json:"time_limit_seconds,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	QuestionIDs      []int64    `json:"question_ids"`
	CurrentIndex     int        `json:"current_index"`
	TotalCount       int        `json:"total_count"`
	CorrectCount     int        `json:"correct_count"`
	WrongCount       int        `json:"wrong_count"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// QuestionAttempt 答题记录
type QuestionAttempt struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	QuestionID int64     `json:"question_id"`
	SessionID  *int64    `json:"session_id,omitempty"`
	Answer     []string  `json:"answer"`
	IsCorrect  bool      `json:"is_correct"`
	Score      float64   `json:"score"`
	DurationMs int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// WrongQuestion 错题
type WrongQuestion struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	QuestionID       int64      `json:"question_id"`
	WrongCount       int        `json:"wrong_count"`
	CorrectCount     int        `json:"correct_count"`
	ReviewCount      int        `json:"review_count"`
	MasteryScore     float64    `json:"mastery_score"`
	IntervalDays     float64    `json:"interval_days"`
	DifficultyFactor float64    `json:"difficulty_factor"`
	PriorityScore    float64    `json:"priority_score"`
	FirstWrongAt     *time.Time `json:"first_wrong_at,omitempty"`
	LastWrongAt      *time.Time `json:"last_wrong_at,omitempty"`
	LastReviewAt     *time.Time `json:"last_review_at,omitempty"`
	NextReviewAt     *time.Time `json:"next_review_at,omitempty"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// 填充字段
	Question *Question `json:"question,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// QuestionNote 错题笔记（Markdown）
type QuestionNote struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	QuestionID int64     `json:"question_id"`
	Content    string    `json:"content"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ReviewResult 复习提交结果
const (
	ReviewResultAgain = "again"
	ReviewResultHard  = "hard"
	ReviewResultGood  = "good"
	ReviewResultEasy  = "easy"
)
