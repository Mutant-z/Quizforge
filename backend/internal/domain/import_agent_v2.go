package domain

import "time"

const ImportAgentV2Pipeline = "import-agent.v2"

type ImportSession struct {
	ID                 int64      `json:"id"`
	UserID             int64      `json:"user_id"`
	BankID             *int64     `json:"bank_id,omitempty"`
	Title              string     `json:"title"`
	Status             string     `json:"status"`
	PipelineVersion    string     `json:"pipeline_version"`
	AttachmentRevision int        `json:"attachment_revision"`
	DraftVersion       int        `json:"draft_version"`
	ActiveRunID        *int64     `json:"active_run_id,omitempty"`
	ExpectedCount      *int       `json:"expected_count,omitempty"`
	UniqueCount        int        `json:"unique_count"`
	CompleteCount      int        `json:"complete_count"`
	BlockingIssueCount int        `json:"blocking_issue_count"`
	LastPreviewHash    string     `json:"last_preview_hash,omitempty"`
	LastPreviewVersion *int       `json:"last_preview_version,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
}

type ImportSessionDocument struct {
	ID                 int64   `json:"id"`
	SessionID          int64   `json:"session_id"`
	DocumentID         int64   `json:"document_id"`
	ImportJobID        *int64  `json:"import_job_id,omitempty"`
	OriginalName       string  `json:"original_name,omitempty"`
	PageCount          int     `json:"page_count,omitempty"`
	IsScanned          bool    `json:"is_scanned"`
	OCRRequiredCount   int     `json:"ocr_required_count"`
	OCRCompletedCount  int     `json:"ocr_completed_count"`
	OCRFailedCount     int     `json:"ocr_failed_count"`
	DocumentRole       string  `json:"document_role"`
	RoleConfidence     float64 `json:"role_confidence"`
	RoleStatus         string  `json:"role_status"`
	ProfileJSON        string  `json:"profile_json,omitempty"`
	AttachmentRevision int     `json:"attachment_revision"`
	Status             string  `json:"status"`
}

type ImportRun struct {
	ID                     int64      `json:"id"`
	SessionID              int64      `json:"session_id"`
	TriggerMessageID       *int64     `json:"trigger_message_id,omitempty"`
	RunType                string     `json:"run_type"`
	Status                 string     `json:"status"`
	BaseAttachmentRevision int        `json:"base_attachment_revision"`
	BaseDraftVersion       int        `json:"base_draft_version"`
	ResultDraftVersion     *int       `json:"result_draft_version,omitempty"`
	ActionJSON             string     `json:"action_json,omitempty"`
	CancelRequested        bool       `json:"cancel_requested"`
	ErrorCode              string     `json:"error_code,omitempty"`
	ErrorMessage           string     `json:"error_message,omitempty"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	FinishedAt             *time.Time `json:"finished_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	StrategyVersion        string     `json:"strategy_version"`
	CurrentStage           string     `json:"current_stage,omitempty"`
	CompletedUnits         int        `json:"completed_units"`
	TotalUnits             int        `json:"total_units"`
	FailedUnits            int        `json:"failed_units"`
	LastProgressAt         *time.Time `json:"last_progress_at,omitempty"`
	ETASeconds             *int       `json:"eta_seconds,omitempty"`
	AdaptiveConcurrency    int        `json:"adaptive_concurrency"`
	DegradedReason         string     `json:"degraded_reason,omitempty"`
	StageProgress          float64    `json:"stage_progress"`
	OverallProgress        float64    `json:"overall_progress"`
	ResultStatus           string     `json:"result_status"`
	ModelSuccessCount      int        `json:"model_success_count"`
	CandidateUpdatedCount  int        `json:"candidate_updated_count"`
	ResolvedCandidateCount int        `json:"resolved_candidate_count"`
	RemainingBlockedCount  int        `json:"remaining_blocked_count"`
	ResultSummaryJSON      string     `json:"result_summary_json,omitempty"`
}

type ImportSessionMessage struct {
	ID              int64     `json:"id"`
	SessionID       int64     `json:"session_id"`
	RunID           *int64    `json:"run_id,omitempty"`
	ParentMessageID *int64    `json:"parent_message_id,omitempty"`
	Role            string    `json:"role"`
	MessageType     string    `json:"message_type"`
	Content         string    `json:"content"`
	MetadataJSON    string    `json:"metadata_json,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ImportEvent struct {
	ID               int64     `json:"event_id"`
	SessionID        int64     `json:"session_id"`
	RunID            *int64    `json:"run_id,omitempty"`
	MessageID        *int64    `json:"message_id,omitempty"`
	Type             string    `json:"type"`
	Stage            string    `json:"stage,omitempty"`
	AgentRole        string    `json:"agent_role,omitempty"`
	DocumentID       *int64    `json:"document_id,omitempty"`
	OwnedPageStart   int       `json:"owned_page_start,omitempty"`
	OwnedPageEnd     int       `json:"owned_page_end,omitempty"`
	ContextPageStart int       `json:"context_page_start,omitempty"`
	ContextPageEnd   int       `json:"context_page_end,omitempty"`
	Current          int       `json:"current,omitempty"`
	Total            int       `json:"total,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	PayloadJSON      string    `json:"payload_json,omitempty"`
	CreatedAt        time.Time `json:"timestamp"`
}

type ImportProblem struct {
	ID              int64      `json:"id"`
	SessionID       int64      `json:"session_id"`
	RunID           *int64     `json:"run_id,omitempty"`
	DocumentID      *int64     `json:"document_id,omitempty"`
	ProblemType     string     `json:"problem_type"`
	Severity        string     `json:"severity"`
	PageStart       int        `json:"page_start,omitempty"`
	PageEnd         int        `json:"page_end,omitempty"`
	CandidateID     *int64     `json:"candidate_id,omitempty"`
	SourceIdentity  string     `json:"source_identity,omitempty"`
	DetailJSON      string     `json:"detail_json,omitempty"`
	Status          string     `json:"status"`
	OriginID        *int64     `json:"origin_id,omitempty"`
	SourceSpanID    *int64     `json:"source_span_id,omitempty"`
	ResolutionHint  string     `json:"resolution_hint,omitempty"`
	ResolvedByRunID *int64     `json:"resolved_by_run_id,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
}

type SourceQuestionOrigin struct {
	ID                    int64                `json:"id"`
	OriginUID             string               `json:"origin_uid"`
	SessionID             int64                `json:"session_id"`
	CandidateID           *int64               `json:"candidate_id,omitempty"`
	CanonicalQuestionID   *int64               `json:"canonical_question_id,omitempty"`
	QuestionDocumentID    *int64               `json:"question_document_id,omitempty"`
	AttachmentRevision    int                  `json:"attachment_revision"`
	DocumentFileHash      string               `json:"document_file_hash"`
	SourceIdentity        string               `json:"source_identity"`
	IdentityVersion       int                  `json:"identity_version"`
	OriginalQuestionLabel string               `json:"original_question_label"`
	OriginalQuestionNo    int                  `json:"original_question_no"`
	SequenceInScope       int                  `json:"sequence_in_scope"`
	SectionPath           interface{}          `json:"section_path"`
	SectionScopeID        string               `json:"section_scope_id"`
	SetNo                 int                  `json:"set_no"`
	RawQuestionType       string               `json:"raw_question_type"`
	CanonicalQuestionType string               `json:"canonical_question_type"`
	IdentityConfidence    string               `json:"identity_confidence"`
	Status                string               `json:"status"`
	SourceSpans           []SourceEvidenceSpan `json:"source_spans"`
}

type SourceEvidenceSpan struct {
	ID                     int64       `json:"id"`
	OriginID               int64       `json:"origin_id"`
	CandidateID            *int64      `json:"candidate_id,omitempty"`
	CanonicalQuestionID    *int64      `json:"canonical_question_id,omitempty"`
	SourceRole             string      `json:"source_role"`
	DocumentID             int64       `json:"document_id"`
	DocumentName           string      `json:"document_name,omitempty"`
	AttachmentRevision     int         `json:"attachment_revision"`
	DocumentFileHash       string      `json:"document_file_hash"`
	PageStart              int         `json:"page_start"`
	PageEnd                int         `json:"page_end"`
	Regions                interface{} `json:"regions"`
	MarkerTop              *int        `json:"marker_top,omitempty"`
	MarkerBottom           *int        `json:"marker_bottom,omitempty"`
	AnchorText             string      `json:"anchor_text,omitempty"`
	PageAssetRenderVersion string      `json:"page_asset_render_version,omitempty"`
	PageAssetSHA256        string      `json:"page_asset_sha256,omitempty"`
	ContentHash            string      `json:"content_hash"`
	ExtractionConfidence   float64     `json:"extraction_confidence"`
	Status                 string      `json:"status"`
}

type ConversationAction struct {
	Intent               string  `json:"intent"`
	DocumentIDs          []int64 `json:"document_ids"`
	PageRanges           [][]int `json:"page_ranges"`
	CandidateIDs         []int64 `json:"candidate_ids"`
	ProblemIDs           []int64 `json:"problem_ids"`
	ReviewScope          string  `json:"review_scope,omitempty"` // pending | missing | page
	TargetRole           string  `json:"target_role,omitempty"`  // question | answer | both
	Instruction          string  `json:"instruction"`
	RequiresConfirmation bool    `json:"requires_confirmation"`
}
