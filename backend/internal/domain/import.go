package domain

import "time"

// Document 上传的原始文档
type Document struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	BankID       *int64    `json:"bank_id,omitempty"`
	OriginalName string    `json:"original_name"`
	StoredName   string    `json:"-"`
	FilePath     string    `json:"-"`
	FileSize     int64     `json:"file_size"`
	FileHash     string    `json:"file_hash"`
	MimeType     string    `json:"mime_type"`
	PageCount    int       `json:"page_count,omitempty"`
	IsScanned    bool      `json:"is_scanned"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// ImportJob 导入任务
type ImportJob struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	BankID             *int64    `json:"bank_id,omitempty"`
	DocumentID         int64     `json:"document_id"`
	Status             string    `json:"status"`
	Stage              string    `json:"stage,omitempty"`
	TotalPages         int       `json:"total_pages"`
	ProcessedPages     int       `json:"processed_pages"`
	CandidateCount     int       `json:"candidate_count"`
	ParsedCount        int       `json:"parsed_count"`
	NewCount           int       `json:"new_count"`
	DuplicateCount     int       `json:"duplicate_count"`
	ConflictCount      int       `json:"conflict_count"`
	FailedCount        int       `json:"failed_count"`
	ReviewCount        int       `json:"review_count"`
	Progress           float64   `json:"progress"`
	ErrorCode          string    `json:"error_code,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	PipelineVersion    string    `json:"pipeline_version"`
	DraftVersion       int       `json:"draft_version"`
	DirectoryComplete  bool      `json:"directory_complete"`
	ExpectedCount      *int      `json:"expected_count,omitempty"`
	ExtractedCount     int       `json:"extracted_count"`
	CompleteCount      int       `json:"complete_count"`
	TotalUnits         int       `json:"total_units"`
	CompletedUnits     int       `json:"completed_units"`
	FailedUnits        int       `json:"failed_units"`
	BlockingIssueCount int       `json:"blocking_issue_count"`
	ProviderIDSnapshot *int64    `json:"provider_id_snapshot,omitempty"`
	ModelSnapshot      string    `json:"model_snapshot,omitempty"`
	SessionID          *int64    `json:"session_id,omitempty"`
	RunID              *int64    `json:"run_id,omitempty"`
}

// ImportTaskLog 导入日志
type ImportTaskLog struct {
	ID          int64     `json:"id"`
	ImportJobID int64     `json:"import_job_id"`
	Stage       string    `json:"stage,omitempty"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}

// CandidateQuestion 候选题
type CandidateQuestion struct {
	ID             int64     `json:"id"`
	DocumentID     int64     `json:"document_id"`
	ImportJobID    *int64    `json:"import_job_id,omitempty"`
	PageStart      int       `json:"page_start"`
	PageEnd        int       `json:"page_end"`
	RawText        string    `json:"raw_text"`
	NormalizedText string    `json:"normalized_text,omitempty"`
	InputHash      string    `json:"-"`
	Status         string    `json:"status"`
	Confidence     float64   `json:"confidence,omitempty"`
	ExtractedJSON  string    `json:"extracted_json,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	RetryCount     int       `json:"retry_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// 导入状态机（docs/05）
const (
	ImportStatusUploaded                = "uploaded"
	ImportStatusExtracting              = "extracting"
	ImportStatusCleaning                = "cleaning"
	ImportStatusDetecting               = "detecting"
	ImportStatusParsing                 = "parsing"
	ImportStatusValidating              = "validating"
	ImportStatusDeduplicating           = "deduplicating"
	ImportStatusClassifying             = "classifying"
	ImportStatusReviewing               = "reviewing"
	ImportStatusEmbedding               = "embedding"
	ImportStatusCompleted               = "completed"
	ImportStatusFailed                  = "failed"
	ImportStatusCancelled               = "cancelled"
	ImportStatusRendering               = "rendering"
	ImportStatusProfiling               = "profiling"
	ImportStatusDirectoryBuilding       = "directory_building"
	ImportStatusExtractingQuestions     = "extracting_questions"
	ImportStatusExtractingAnswers       = "extracting_answers"
	ImportStatusLinking                 = "linking"
	ImportStatusDraftReady              = "draft_ready"
	ImportStatusNeedsAttention          = "needs_attention"
	ImportStatusNeedsModelConfiguration = "needs_model_configuration"
	ImportStatusCommitting              = "committing"
)

// Job 任务队列
type Job struct {
	ID           int64      `json:"id"`
	Type         string     `json:"type"`
	Payload      string     `json:"payload"`
	Status       string     `json:"status"`
	Attempt      int        `json:"attempt"`
	MaxAttempts  int        `json:"max_attempts"`
	LeaseUntil   *time.Time `json:"-"`
	WorkerID     string     `json:"-"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Priority     int        `json:"priority"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
