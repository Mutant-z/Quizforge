package domain

import "time"

// 题目类型
const (
	QuestionTypeSingleChoice     = "single_choice"
	QuestionTypeMultipleChoice   = "multiple_choice"
	QuestionTypeTrueFalse        = "true_false"
	QuestionTypeFillBlank        = "fill_blank"
	QuestionTypeShortAnswer      = "short_answer"
	QuestionTypeMaterialAnalysis = "material_analysis"
	QuestionTypeEssay            = "essay"
	QuestionTypeCode             = "code"
)

// 题目状态
const (
	QuestionStatusDraft       = "draft"
	QuestionStatusValidated   = "validated"
	QuestionStatusPublished   = "published"
	QuestionStatusNeedsReview = "needs_review"
	QuestionStatusConflicted  = "conflicted"
	QuestionStatusArchived    = "archived"
)

type User struct {
	ID               int64     `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	PasswordHash     string    `json:"-"`
	Role             string    `json:"role"`
	Avatar           string    `json:"avatar,omitempty"`
	Bio              string    `json:"bio,omitempty"`
	DefaultBankID    int64     `json:"default_bank_id,omitempty"`
	StudyPreferences string    `json:"study_preferences,omitempty"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type QuestionBank struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Cover         string    `json:"cover,omitempty"`
	Visibility    string    `json:"visibility"`
	Status        string    `json:"status"`
	QuestionCount int64     `json:"question_count"`
	SubjectCount  int64     `json:"subject_count,omitempty"`
	CreatedBy     int64     `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Subject struct {
	ID        int64     `json:"id"`
	BankID    int64     `json:"bank_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type Chapter struct {
	ID             int64      `json:"id"`
	SubjectID      int64      `json:"subject_id"`
	ParentID       *int64     `json:"parent_id,omitempty"`
	Name           string     `json:"name"`
	NormalizedName string     `json:"normalized_name,omitempty"`
	Level          int        `json:"level"`
	SortOrder      int        `json:"sort_order"`
	Status         string     `json:"status"`
	QuestionCount  int64      `json:"question_count,omitempty"`
	MasteryScore   float64    `json:"mastery_score,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Children       []*Chapter `json:"children,omitempty"`
}

type KnowledgePoint struct {
	ID        int64     `json:"id"`
	BankID    int64     `json:"bank_id"`
	SubjectID *int64    `json:"subject_id,omitempty"`
	ChapterID *int64    `json:"chapter_id,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// QuestionOption 选项
type QuestionOption struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

type Question struct {
	ID                 int64            `json:"id"`
	BankID             int64            `json:"bank_id"`
	SubjectID          *int64           `json:"subject_id,omitempty"`
	ChapterID          *int64           `json:"chapter_id,omitempty"`
	Type               string           `json:"type"`
	Stem               string           `json:"stem"`
	Options            []QuestionOption `json:"options,omitempty"`
	Answer             []string         `json:"answer"`
	OriginalAnalysis   string           `json:"original_analysis,omitempty"`
	AIEnhancedAnalysis string           `json:"ai_enhanced_analysis,omitempty"`
	Difficulty         float64          `json:"difficulty"`
	KnowledgePoints    []string         `json:"knowledge_points,omitempty"`
	QualityScore       float64          `json:"quality_score"`
	Status             string           `json:"status"`
	SourceHash         string           `json:"-"`
	Version            int              `json:"version"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	// 用户相关（可选填充）
	IsWrong bool `json:"is_wrong,omitempty"`
	IsFav   bool `json:"is_favorite,omitempty"`
	HasNote bool `json:"has_note,omitempty"`
}

type QuestionSource struct {
	ID                  int64     `json:"id"`
	CanonicalQuestionID *int64    `json:"canonical_question_id,omitempty"`
	DocumentID          int64     `json:"document_id"`
	CandidateID         *int64    `json:"candidate_id,omitempty"`
	PageStart           int       `json:"page_start"`
	PageEnd             int       `json:"page_end"`
	RawText             string    `json:"raw_text"`
	OriginalAnswer      string    `json:"original_answer,omitempty"`
	OriginalAnalysis    string    `json:"original_analysis,omitempty"`
	SourceHash          string    `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
}
