package domain

import (
	"strings"
	"time"
)

const (
	VisionPipelineVersion    = "vision-agent.v1"
	VisionPipelineVersionV11 = "vision-agent.v1.1"
)

func IsVisionPipeline(version string) bool {
	return version == VisionPipelineVersion || version == VisionPipelineVersionV11
}

type DocumentPageAsset struct {
	ID            int64  `json:"id"`
	DocumentID    int64  `json:"document_id"`
	PageNo        int    `json:"page_no"`
	RenderVersion string `json:"render_version"`
	Purpose       string `json:"purpose"`
	FilePath      string `json:"-"`
	MIMEType      string `json:"mime_type"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	SHA256        string `json:"sha256"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type ImportAgentUnit struct {
	ID               int64      `json:"id"`
	ImportJobID      int64      `json:"import_job_id"`
	UnitType         string     `json:"unit_type"`
	UnitKey          string     `json:"unit_key"`
	OwnedPageStart   int        `json:"owned_page_start"`
	OwnedPageEnd     int        `json:"owned_page_end"`
	ContextPageStart int        `json:"context_page_start"`
	ContextPageEnd   int        `json:"context_page_end"`
	Status           string     `json:"status"`
	Attempt          int        `json:"attempt"`
	InputHash        string     `json:"-"`
	OutputJSON       string     `json:"output_json,omitempty"`
	ValidationJSON   string     `json:"validation_json,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	PipelineVersion  string     `json:"pipeline_version"`
	RunID            *int64     `json:"run_id,omitempty"`
	DocumentID       *int64     `json:"document_id,omitempty"`
	QueuedAt         *time.Time `json:"queued_at,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	QueueLatencyMS   int        `json:"queue_latency_ms"`
	PayloadBytes     int        `json:"payload_bytes"`
	ResultCount      int        `json:"result_count"`
	ImageProfile     string     `json:"image_profile,omitempty"`
}

type ImportMessage struct {
	ID                 int64     `json:"id"`
	ImportJobID        int64     `json:"import_job_id"`
	Role               string    `json:"role"`
	Content            string    `json:"content"`
	ActionJSON         string    `json:"action_json,omitempty"`
	Status             string    `json:"status"`
	DraftVersionBefore int       `json:"draft_version_before,omitempty"`
	DraftVersionAfter  int       `json:"draft_version_after,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type VisionDirectoryEntry struct {
	ContentRole           string        `json:"content_role"`
	SectionPath           []string      `json:"section_path"`
	SetNo                 int           `json:"set_no"`
	QuestionType          string        `json:"question_type"`
	CanonicalQuestionType string        `json:"canonical_question_type,omitempty"`
	SectionScopeID        string        `json:"section_scope_id,omitempty"`
	SequenceInScope       int           `json:"sequence_in_scope,omitempty"`
	OriginalQuestionLabel string        `json:"original_question_label,omitempty"`
	SourceQuestionNo      int           `json:"source_question_no"`
	SourcePageStart       int           `json:"source_page_start"`
	SourcePageEnd         int           `json:"source_page_end"`
	Anchor                string        `json:"anchor"`
	MarkerTop             int           `json:"marker_top"`
	MarkerBottom          int           `json:"marker_bottom,omitempty"`
	Region                *VisionRegion `json:"region,omitempty"`
	SourceIdentity        string        `json:"source_identity,omitempty"`
}

type VisionRegion struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

type VisionQuestion struct {
	SourceIdentity        string           `json:"source_identity"`
	Stem                  string           `json:"stem"`
	Passage               string           `json:"passage,omitempty"`
	Options               []QuestionOption `json:"options"`
	QuestionType          string           `json:"question_type"`
	CanonicalQuestionType string           `json:"canonical_question_type,omitempty"`
	Answer                []string         `json:"answer,omitempty"`
	Analysis              string           `json:"analysis,omitempty"`
	SourcePageStart       int              `json:"source_page_start"`
	SourcePageEnd         int              `json:"source_page_end"`
	VisualConfidence      float64          `json:"visual_confidence"`
	Warnings              []string         `json:"warnings,omitempty"`
}

type VisionAnswer struct {
	SourceIdentity        string   `json:"source_identity,omitempty"`
	SectionPath           []string `json:"section_path"`
	SetNo                 int      `json:"set_no"`
	QuestionType          string   `json:"question_type"`
	CanonicalQuestionType string   `json:"canonical_question_type,omitempty"`
	SectionScopeID        string   `json:"section_scope_id,omitempty"`
	SourceQuestionNo      int      `json:"source_question_no"`
	Answer                []string `json:"answer"`
	Analysis              string   `json:"analysis,omitempty"`
	SourcePageStart       int      `json:"source_page_start"`
	SourcePageEnd         int      `json:"source_page_end"`
}

// CanonicalQuestionType deliberately accepts only aliases observed in model
// output. Unknown values are retained (normalized) so validation can flag
// them instead of silently treating them as a choice question.
func CanonicalQuestionType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.NewReplacer("-", "_", " ", "", "、", "").Replace(v)
	switch v {
	case "single", "single_choice", "singlechoice", "choice", "one_choice":
		return "single_choice"
	case "multiple", "multiple_choice", "multiplechoice", "multi_choice", "multichoice":
		return "multiple_choice"
	case "composite", "case_multiple_choice", "case_choice", "case_multiple":
		// “案例客观题” blocks still contain choice questions. Normalize the
		// broad model label so they can match the answer directory.
		return "multiple_choice"
	case "true_false", "trueorfalse", "true_or_false", "judge", "judgement", "judgment", "statement_or_true_false", "tf":
		return "true_false"
	case "fill", "fill_in", "fill_in_blank", "fill_in_the_blank", "fillblank", "fill_blank", "blank_filling", "blank":
		return "fill_in_blank"
	case "qa", "essay", "short_answer", "shortanswer", "subjective", "subjective_question",
		"case_analysis", "case", "analysis", "explain", "material", "material_analysis", "reading_comprehension",
		"简答题", "论述题", "辨析题", "案例分析题":
		return "subjective"
	default:
		return v
	}
}

const (
	ReviewInclude        = "include"
	ReviewExclude        = "exclude"
	ReviewPending        = "pending"
	AnswerLinkConfirmed  = "confirmed"
	AnswerLinkUnresolved = "unresolved"
)

type ImportPreview struct {
	DirectoryComplete       bool                     `json:"directory_complete"`
	DirectoryCoverageStatus string                   `json:"directory_coverage_status"`
	VerifiedPageCount       int                      `json:"verified_page_count"`
	MismatchPageCount       int                      `json:"mismatch_page_count"`
	ExpectedCountReady      bool                     `json:"expected_count_ready"`
	ExpectedCount           *int                     `json:"expected_count,omitempty"`
	ExtractedCount          int                      `json:"extracted_count"`
	UniqueCount             int                      `json:"unique_count"`
	CompleteCount           int                      `json:"complete_count"`
	FailedUnits             int                      `json:"failed_units"`
	BlockingIssueCount      int                      `json:"blocking_issue_count"`
	CanCommit               bool                     `json:"can_commit"`
	IncludedCount           int                      `json:"included_count"`
	ExcludedCount           int                      `json:"excluded_count"`
	PendingReviewCount      int                      `json:"pending_review_count"`
	IncludedCompleteCount   int                      `json:"included_complete_count"`
	UnlinkedAnswerCount     int                      `json:"unlinked_answer_count"`
	AnswerConflictCount     int                      `json:"answer_conflict_count"`
	Issues                  []map[string]interface{} `json:"issues"`
}
