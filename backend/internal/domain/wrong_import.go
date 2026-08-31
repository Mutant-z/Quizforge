package domain

import (
	"strings"
	"time"
)

const (
	// WrongImportPipeline 错题导入 pipeline 版本（docs/06 prompt 版本化要求）。
	WrongImportPipeline = "wrong-import.v2"

	WrongImportSessionStatusActive    = "active"
	WrongImportSessionStatusCommitted = "committed"
	WrongImportSessionStatusArchived  = "archived"

	WrongImportImagePending    = "pending"
	WrongImportImageProcessing = "processing"
	WrongImportImageDone       = "done"
	WrongImportImageFailed     = "failed"

	WrongImportExtractionQuestions = "questions"
	WrongImportExtractionAnswerKey = "answer_key"
	// WrongImportExtractionAuto 用于一次上传中混合了题目图和答案参考图的场景。
	WrongImportExtractionAuto = "auto"

	WrongImportBatchUploading   = "uploading"
	WrongImportBatchSealed      = "sealed"
	WrongImportBatchReconciling = "reconciling"
	WrongImportBatchDone        = "done"
	WrongImportBatchFailed      = "failed"

	WrongImportMergeStatusActive = "active"
	WrongImportMergeStatusMerged = "merged"
	WrongImportMergePending      = "pending"
	WrongImportMergeAccepted     = "accepted"
	WrongImportMergeRejected     = "rejected"
	WrongImportMergeUndone       = "undone"

	// 草稿审核状态：extracted 刚提取 / missing_answer 缺答案待用户补充 /
	// ready 可入库 / excluded 用户排除 / committed 已入库。
	WrongImportDraftReviewExtracted     = "extracted"
	WrongImportDraftReviewMissingAnswer = "missing_answer"
	WrongImportDraftReviewReady         = "ready"
	WrongImportDraftReviewExcluded      = "excluded"
	WrongImportDraftReviewCommitted     = "committed"

	// 答案来源：image 图片识别 / user 用户补充 / none 主观题无需答案 / pending 待定。
	WrongImportAnswerSourcePending = "pending"
	WrongImportAnswerSourceImage   = "image"
	WrongImportAnswerSourceUser    = "user"
	WrongImportAnswerSourceNone    = "none"

	WrongImportAnswerStatusPending      = "pending"
	WrongImportAnswerStatusMissing      = "missing"
	WrongImportAnswerStatusProvided     = "provided"
	WrongImportAnswerStatusExplicitNone = "explicit_none"
	WrongImportAnswerStatusUnresolved   = "unresolved"

	WrongImportEventImageQueued      = "image.queued"
	WrongImportEventImageStarted     = "image.started"
	WrongImportEventImageDone        = "image.completed"
	WrongImportEventImageFailed      = "image.failed"
	WrongImportEventDraftCreated     = "draft.created"
	WrongImportEventDraftUpdated     = "draft.updated"
	WrongImportEventDraftDeleted     = "draft.deleted"
	WrongImportEventReconcileStarted = "reconcile.started"
	WrongImportEventReconcileDone    = "reconcile.completed"
	WrongImportEventDraftMerged      = "draft.merged"
	WrongImportEventMergeUndone      = "draft.merge_undone"
	WrongImportEventSessionMsg       = "session.message"
	WrongImportEventCommitDone       = "commit.completed"

	BankKindNormal    = "normal"
	BankKindWrongBook = "wrong_book"

	WrongSourcePractice = "practice"
	WrongSourceManual   = "manual"
	WrongSourceImport   = "import"
)

// WrongImportSession 错题导入会话。
type WrongImportSession struct {
	ID                 int64      `json:"id"`
	UserID             int64      `json:"user_id"`
	Title              string     `json:"title"`
	Status             string     `json:"status"`
	TargetBankID       *int64     `json:"target_bank_id,omitempty"`
	DraftCount         int        `json:"draft_count"`
	ReadyCount         int        `json:"ready_count"`
	MissingAnswerCount int        `json:"missing_answer_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CommittedAt        *time.Time `json:"committed_at,omitempty"`
}

// WrongImportImage 会话内上传的错题图片。
type WrongImportImage struct {
	ID             int64     `json:"id"`
	SessionID      int64     `json:"session_id"`
	BatchID        string    `json:"batch_id,omitempty"`
	BatchIndex     *int      `json:"batch_index,omitempty"`
	ExtractionMode string    `json:"extraction_mode,omitempty"`
	OriginalName   string    `json:"original_name,omitempty"`
	FilePath       string    `json:"-"`
	FileHash       string    `json:"file_hash,omitempty"`
	MIMEType       string    `json:"mime_type"`
	SizeBytes      int64     `json:"size_bytes"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	Status         string    `json:"status"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	DraftCount     int       `json:"draft_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WrongImportImageBatch 一次选择/粘贴/拖拽上传的图片批次。
type WrongImportImageBatch struct {
	ID            string     `json:"id"`
	SessionID     int64      `json:"session_id"`
	ExpectedCount int        `json:"expected_count"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	SealedAt      *time.Time `json:"sealed_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// WrongImportBoundary 描述题目在单张图片边界处是否可能被截断。
type WrongImportBoundary struct {
	StartsMidQuestion bool   `json:"starts_mid_question"`
	EndsMidQuestion   bool   `json:"ends_mid_question"`
	StartMarker       string `json:"start_marker,omitempty"`
	EndMarker         string `json:"end_marker,omitempty"`
}

// WrongImportDraftSource 保留逻辑题目对应的每张原图和区域。
type WrongImportDraftSource struct {
	ID           int64         `json:"id"`
	DraftID      int64         `json:"draft_id"`
	ImageID      int64         `json:"image_id"`
	SourceOrder  int           `json:"source_order"`
	FragmentRole string        `json:"fragment_role"`
	Region       *VisionRegion `json:"region,omitempty"`
}

// WrongImportAnswerFragment 是答案参考图的可重试识别片段。
// 答案图可能没有题号，先保留片段，再由批次整理器决定是否拼接和匹配。
type WrongImportAnswerFragment struct {
	ID              int64     `json:"id"`
	SessionID       int64     `json:"session_id"`
	ImageID         int64     `json:"image_id"`
	BatchID         string    `json:"batch_id,omitempty"`
	BatchIndex      *int      `json:"batch_index,omitempty"`
	FragmentIndex   int       `json:"fragment_index"`
	SectionTitle    string    `json:"section_title,omitempty"`
	Seq             int       `json:"seq"`
	QuestionHint    string    `json:"question_hint,omitempty"`
	Answer          []string  `json:"answer,omitempty"`
	Analysis        string    `json:"analysis,omitempty"`
	Confidence      float64   `json:"confidence"`
	StartsMidAnswer bool      `json:"starts_mid_answer"`
	EndsMidAnswer   bool      `json:"ends_mid_answer"`
	FragmentRole    string    `json:"fragment_role"`
	Status          string    `json:"status"`
	MatchedDraftID  *int64    `json:"matched_draft_id,omitempty"`
	PayloadJSON     string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// WrongImportDraft 提取出的错题草稿（暂存，不入 canonical_questions）。
type WrongImportDraft struct {
	ID                  int64                    `json:"id"`
	SessionID           int64                    `json:"session_id"`
	ImageID             int64                    `json:"image_id"`
	SeqNo               int                      `json:"seq_no"`
	QuestionType        string                   `json:"question_type"`
	Stem                string                   `json:"stem"`
	Options             []QuestionOption         `json:"options,omitempty"`
	Answer              []string                 `json:"answer,omitempty"`
	AnswerSource        string                   `json:"answer_source"`
	AnswerStatus        string                   `json:"answer_status"`
	UserMarkedNoAnswer  bool                     `json:"user_marked_no_answer"`
	Analysis            string                   `json:"analysis,omitempty"`
	Difficulty          *float64                 `json:"difficulty,omitempty"`
	KnowledgePoints     []string                 `json:"knowledge_points,omitempty"`
	Confidence          float64                  `json:"confidence"`
	Warnings            []string                 `json:"warnings,omitempty"`
	Region              *VisionRegion            `json:"region,omitempty"`
	Boundary            *WrongImportBoundary     `json:"boundary,omitempty"`
	Sources             []WrongImportDraftSource `json:"sources,omitempty"`
	MergeStatus         string                   `json:"merge_status"`
	MergedIntoDraftID   *int64                   `json:"merged_into_draft_id,omitempty"`
	ContentVersion      int                      `json:"content_version"`
	DuplicateOfDraftID  *int64                   `json:"duplicate_of_draft_id,omitempty"`
	ReviewStatus        string                   `json:"review_status"`
	CommittedQuestionID *int64                   `json:"committed_question_id,omitempty"`
	CommittedWrongID    *int64                   `json:"committed_wrong_id,omitempty"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
}

// WrongImportMergeCandidate 表示一对相邻草稿是否可能属于同一道题。
type WrongImportMergeCandidate struct {
	ID                 int64     `json:"id"`
	SessionID          int64     `json:"session_id"`
	BatchID            string    `json:"batch_id,omitempty"`
	LeftDraftID        int64     `json:"left_draft_id"`
	RightDraftID       int64     `json:"right_draft_id"`
	Score              float64   `json:"score"`
	ReasonCode         string    `json:"reason_code"`
	Decision           string    `json:"decision"`
	Status             string    `json:"status"`
	TargetVersionAfter int       `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	BeforeJSON         string    `json:"-"`
	AfterJSON          string    `json:"-"`
}

// WrongImportDraftSnapshot 用于合并撤销时恢复草稿内容和来源关系。
type WrongImportDraftSnapshot struct {
	QuestionType       string                   `json:"question_type"`
	Stem               string                   `json:"stem"`
	Options            []QuestionOption         `json:"options"`
	Answer             []string                 `json:"answer"`
	AnswerSource       string                   `json:"answer_source"`
	AnswerStatus       string                   `json:"answer_status"`
	UserMarkedNoAnswer bool                     `json:"user_marked_no_answer"`
	Analysis           string                   `json:"analysis"`
	Boundary           *WrongImportBoundary     `json:"boundary,omitempty"`
	Difficulty         *float64                 `json:"difficulty,omitempty"`
	KnowledgePoints    []string                 `json:"knowledge_points,omitempty"`
	Confidence         float64                  `json:"confidence"`
	Warnings           []string                 `json:"warnings,omitempty"`
	ReviewStatus       string                   `json:"review_status"`
	MergeStatus        string                   `json:"merge_status"`
	MergedIntoDraftID  *int64                   `json:"merged_into_draft_id,omitempty"`
	Sources            []WrongImportDraftSource `json:"sources"`
}

// WrongImportMergeApply 是一次合并写入目标草稿的内容。
type WrongImportMergeApply struct {
	TargetID int64                    `json:"target_id"`
	SourceID int64                    `json:"source_id"`
	Target   WrongImportDraftSnapshot `json:"target"`
}

// WrongImportMergeBefore 是合并前的双草稿快照。
type WrongImportMergeBefore struct {
	TargetID int64                    `json:"target_id"`
	SourceID int64                    `json:"source_id"`
	Target   WrongImportDraftSnapshot `json:"target"`
	Source   WrongImportDraftSnapshot `json:"source"`
}

// NeedsAnswer 客观题和材料分析题通常需要参考答案；普通主观题允许无答案。
func (d *WrongImportDraft) NeedsAnswer() bool {
	switch d.QuestionType {
	case QuestionTypeShortAnswer, QuestionTypeEssay:
		return false
	case QuestionTypeMaterialAnalysis:
		return true
	// 代码题按主观处理
	case QuestionTypeCode:
		return false
	default:
		return true
	}
}

// CanonicalWrongImportQuestionType 是错题图片导入专用的题型归一化。
// PDF 导入仍使用 CanonicalQuestionType，避免把 PDF 的 subjective 语义带入错题草稿。
func CanonicalWrongImportQuestionType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.NewReplacer("-", "_", " ", "", "、", "").Replace(v)
	switch v {
	case "single", "single_choice", "singlechoice", "choice", "one_choice":
		return QuestionTypeSingleChoice
	case "multiple", "multiple_choice", "multiplechoice", "multi_choice", "multichoice":
		return QuestionTypeMultipleChoice
	case "true_false", "trueorfalse", "true_or_false", "judge", "judgement", "judgment", "tf":
		return QuestionTypeTrueFalse
	case "fill", "fill_in", "fill_in_blank", "fill_in_the_blank", "fillblank", "fill_blank", "blank_filling", "blank":
		return QuestionTypeFillBlank
	case "material_analysis", "materialanalysis", "case_analysis", "caseanalysis", "case", "case_study", "material", "材料分析题", "案例分析题", "案例题", "材料题":
		return QuestionTypeMaterialAnalysis
	case "qa", "short_answer", "shortanswer", "subjective", "subjective_question", "简答题":
		return QuestionTypeShortAnswer
	case "essay", "论述题", "辨析题":
		return QuestionTypeEssay
	case "code":
		return QuestionTypeCode
	default:
		return v
	}
}

// NormalizeWrongImportAnswerState fills the new explicit state for old callers
// and keeps legacy drafts readable during the migration window.
func (d *WrongImportDraft) NormalizeWrongImportAnswerState() {
	if d.AnswerStatus != "" {
		return
	}
	if len(d.Answer) > 0 {
		d.AnswerStatus = WrongImportAnswerStatusProvided
	} else if d.AnswerSource == WrongImportAnswerSourceNone && d.UserMarkedNoAnswer {
		d.AnswerStatus = WrongImportAnswerStatusExplicitNone
	} else if d.AnswerSource == WrongImportAnswerSourceImage {
		d.AnswerStatus = WrongImportAnswerStatusUnresolved
	} else {
		d.AnswerStatus = WrongImportAnswerStatusMissing
	}
}

// WrongImportMessage 会话消息。
type WrongImportMessage struct {
	ID           int64     `json:"id"`
	SessionID    int64     `json:"session_id"`
	Role         string    `json:"role"`
	Content      string    `json:"content"`
	MetadataJSON string    `json:"metadata_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// WrongImportEvent 侧栏实时事件。
type WrongImportEvent struct {
	ID          int64     `json:"event_id"`
	SessionID   int64     `json:"session_id"`
	Type        string    `json:"type"`
	PayloadJSON string    `json:"payload_json,omitempty"`
	CreatedAt   time.Time `json:"timestamp"`
}

// WrongImportCommitResult 确认入库结果。
type WrongImportCommitResult struct {
	CommittedCount int   `json:"committed_count"`
	LinkedCount    int   `json:"linked_count"` // 兼容旧客户端；直接入库模式下始终为 0
	CreatedCount   int   `json:"created_count"`
	BankID         int64 `json:"bank_id"`
}
