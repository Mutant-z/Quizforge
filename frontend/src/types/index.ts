// API 统一类型定义（对应后端 /api/v1 响应）
export interface ApiResponse<T> {
  data: T
  error: { code: string; message: string; detail?: string } | null
  request_id: string
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface User {
  id: number
  username: string
  email: string
  role: string
  avatar?: string
  bio?: string
  default_bank_id?: number
  study_preferences?: string
  created_at: string
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface QuestionBank {
  id: number
  name: string
  description?: string
  cover?: string
  visibility: string
  status: string
  question_count: number
  subject_count?: number
  created_by?: number
  created_at: string
  updated_at: string
}

export interface Subject {
  id: number
  bank_id: number
  name: string
  sort_order: number
}

export interface Chapter {
  id: number
  subject_id: number
  parent_id?: number
  name: string
  level: number
  sort_order: number
  question_count?: number
  mastery_score?: number
  children?: Chapter[]
}

export interface QuestionOption {
  key: string
  content: string
}

export type QuestionType = 'single_choice' | 'multiple_choice' | 'true_false' | 'fill_blank' | 'short_answer' | 'material_analysis' | 'essay' | 'code'

export interface Question {
  id: number
  bank_id: number
  subject_id?: number
  chapter_id?: number
  type: QuestionType
  stem: string
  options?: QuestionOption[]
  answer: string[]
  original_analysis?: string
  ai_enhanced_analysis?: string
  difficulty: number
  knowledge_points?: string[]
  quality_score: number
  status: string
  version: number
  created_at: string
  updated_at: string
  is_wrong?: boolean
  is_favorite?: boolean
  has_note?: boolean
}

export interface PracticeSession {
  id: number
  user_id: number
  bank_id?: number
  bank_ids: number[]
  subject_id?: number
  chapter_id?: number
  mode: string
  time_limit_seconds?: number | null
  expires_at?: string | null
  question_ids: number[]
  current_index: number
  total_count: number
  correct_count: number
  wrong_count: number
  status: string
  started_at: string
  finished_at?: string
}

export type PracticeMode = 'sequential' | 'random' | 'unattempted' | 'wrong' | 'favorite' | 'due'

export interface PracticeSessionRequest {
  bank_ids: number[]
  subject_id?: number
  chapter_id?: number
  mode: PracticeMode
  type?: QuestionType | ''
  limit: number
  time_limit_seconds?: number | null
}

export interface PracticePreview {
  available_count: number
  selected_count: number
}

export interface AnswerResult {
  correct: boolean
  score: number
  user_answer: string[]
  correct_answer: string[]
  analysis: string
  question: Question
  wrong_added: boolean
  next_review_at?: string
  ai_explanation?: string
}

export interface WrongQuestion {
  id: number
  user_id: number
  question_id: number
  wrong_count: number
  correct_count: number
  review_count: number
  mastery_score: number
  interval_days: number
  priority_score: number
  first_wrong_at?: string
  last_wrong_at?: string
  last_review_at?: string
  next_review_at?: string
  status: string
  question?: Question
  note?: string
}

export interface QuestionNote {
  id: number
  user_id: number
  question_id: number
  content: string
  version: number
  updated_at: string
}

export interface Document {
  id: number
  user_id: number
  bank_id?: number
  original_name: string
  file_size: number
  file_hash: string
  mime_type: string
  page_count?: number
  is_scanned: boolean
  status: string
  created_at: string
}

export interface ImportJob {
  id: number
  user_id: number
  bank_id?: number
  document_id: number
  status: string
  stage?: string
  total_pages: number
  processed_pages: number
  candidate_count: number
  parsed_count: number
  new_count: number
  duplicate_count: number
  conflict_count: number
  failed_count: number
  review_count: number
  progress: number
  error_code?: string
  error_message?: string
  pipeline_version?: string
  draft_version: number
  directory_complete: boolean
  expected_count?: number
  extracted_count: number
  complete_count: number
  total_units: number
  completed_units: number
  failed_units: number
  blocking_issue_count: number
  model_snapshot?: string
  created_at: string
}

export interface ImportAgentUnit {
  id: number
  unit_type: string
  unit_key: string
  owned_page_start: number
  owned_page_end: number
  context_page_start: number
  context_page_end: number
  status: string
  attempt: number
  error_code?: string
  error_message?: string
}

export interface ImportPreview {
  directory_complete: boolean
	directory_coverage_status?: 'verified' | 'mismatch' | 'unverifiable'
	verified_page_count?: number
	mismatch_page_count?: number
	expected_count_ready?: boolean
  expected_count?: number
  extracted_count: number
  unique_count: number
  complete_count: number
  failed_units: number
  blocking_issue_count: number
  can_commit: boolean
  included_count?: number
  excluded_count?: number
  pending_review_count?: number
  included_complete_count?: number
  unlinked_answer_count?: number
  answer_conflict_count?: number
  issues: Array<Record<string, unknown>>
}

export interface VisionDraftCandidate {
  id: number
  document_id?: number
  answer_document_id?: number
  status: string
  section_path: string[]
  set_no: number
  source_question_no: number
	origin_uid?: string
	original_question_label?: string
	sequence_in_scope?: number
	provenance_complete?: boolean
	source_spans?: SourceEvidenceSpan[]
	remaining_problems?: Array<{ id: number; problem_type: string; severity: string; resolution_hint: string; page_start?: number; page_end?: number; detail_json?: string }>
  question_type: string
  canonical_question_type?: string
  section_scope_id?: string
  review_decision?: 'include' | 'exclude' | 'pending' | string
  exclusion_reason?: string
  answer_link_status?: 'confirmed' | 'unresolved' | string
  answer_link_method?: string
  answer_link_confidence?: number
  answer_resolution?: 'ANSWER_MISSING' | 'ANSWER_SOURCE_IDENTITY_INVALID' | 'ANSWER_CONFLICT' | 'ANSWER_UNRESOLVED' | string
  answer_sources?: Array<{ document_id?: number; page_start?: number; page_end?: number; source_kind?: string; confidence?: number }>
  question: {
    stem?: string
    passage?: string
    options?: Array<{ key: string; content: string }>
    answer?: string[]
    analysis?: string
  }
  validation_issues: Array<Record<string, unknown>>
  visual_confidence: number
  question_page_start: number
  question_page_end: number
  /** Normalized vertical position on the source PDF page (0 means unknown). */
  source_marker_top?: number
  answer_page_start: number
  answer_page_end: number
  draft_version: number
}

export interface SourceEvidenceSpan {
	id: number
	origin_id: number
	source_role: 'question' | 'answer' | 'explanation'
	document_id: number
	document_name?: string
	attachment_revision: number
	document_file_hash: string
	page_start: number
	page_end: number
	regions?: Array<{ left: number; top: number; right: number; bottom: number }>
	marker_top?: number
	marker_bottom?: number
	anchor_text?: string
	page_asset_render_version?: string
	page_asset_sha256?: string
	content_hash: string
	extraction_confidence: number
	status: string
}

export interface ImportMessage {
  id: number
  role: string
  content: string
  status: string
  created_at: string
}

export interface ImportAgentSession {
  id: number
  user_id: number
  bank_id?: number
  title: string
  status: string
  pipeline_version: string
  attachment_revision: number
  draft_version: number
  active_run_id?: number
  expected_count?: number
  unique_count: number
  complete_count: number
  blocking_issue_count: number
  last_preview_hash?: string
  last_preview_version?: number
  created_at: string
  updated_at: string
}

export interface ImportAgentRun {
  id: number
  session_id: number
  run_type: string
  status: string
  strategy_version: string
  current_stage?: string
  completed_units: number
  total_units: number
  failed_units: number
  started_at?: string
  cancel_requested?: boolean
  last_progress_at?: string
  eta_seconds?: number
  adaptive_concurrency: number
  degraded_reason?: string
  stage_progress: number
  overall_progress: number
	result_status?: 'resolved' | 'partial' | 'unchanged' | 'failed'
	model_success_count?: number
	candidate_updated_count?: number
	resolved_candidate_count?: number
	remaining_blocked_count?: number
	result_summary_json?: string
}

export interface ImportAgentDocument {
  id: number
  session_id: number
  document_id: number
  import_job_id?: number
  original_name: string
  page_count: number
  is_scanned?: boolean
  ocr_required_count?: number
  ocr_completed_count?: number
  ocr_failed_count?: number
  document_role: 'question_book' | 'answer_book' | 'explanation_book' | 'mixed' | 'supplement' | 'unknown'
  role_confidence: number
  role_status: string
  attachment_revision: number
  status: string
}

export interface ImportAgentMessage {
  id: number
  session_id: number
  run_id?: number
  parent_message_id?: number
  role: 'user' | 'assistant' | 'tool'
  message_type: 'text' | 'attachment' | 'agent_run' | 'tool_result' | 'clarification' | 'action_card' | 'preview' | 'error'
  content: string
  metadata_json?: string
  status: string
  created_at: string
}

export interface ImportAgentEvent {
  event_id: number
  session_id: number
  run_id?: number
  message_id?: number
  type: string
  stage?: string
  agent_role?: string
  document_id?: number
  owned_page_start?: number
  owned_page_end?: number
  context_page_start?: number
  context_page_end?: number
  current?: number
  total?: number
  summary?: string
  payload_json?: string
  timestamp?: string
}

export interface ImportAgentProblem {
  id: number
  document_id?: number
  problem_type: string
  severity: string
  page_start?: number
  page_end?: number
  candidate_id?: number
  source_identity?: string
  detail_json?: string
  status: string
	origin_id?: number
	source_span_id?: number
	resolution_hint?: 'retry_question' | 'retry_answer' | 'normalize_identity' | 'manual_review' | string
	resolved_by_run_id?: number
	resolved_at?: string
}

export interface ImportTaskLog {
  id: number
  import_job_id: number
  stage?: string
  message: string
  created_at: string
}

export interface RagDocument {
  id: number
  source_type: string
  question_id?: number
  document_id?: number
  document_page?: number
  bank_id?: number
  subject_id?: number
  chapter_id?: number
  knowledge_point?: string
  question_type?: string
  difficulty?: number
  content: string
  score: number
}

export interface AgentSession {
  id: number
  user_id: number
  title: string
  context_json?: string
  status: string
  created_at: string
  updated_at: string
}

export interface AgentMessage {
  id: number
  session_id: number
  role: string
  content?: string
  tool_calls_json?: string
  created_at: string
}

export type ProviderType = 'llm' | 'embedding' | 'rerank'

export interface ProviderConfig {
  id: number
  provider_type: ProviderType
  name: string
  protocol: string
  base_url: string
  api_key_masked: string
  model_name: string
  is_default: boolean
  supports_vision: boolean
}

export interface StatOverview {
  today: { question_count: number; correct_count: number; wrong_count: number; review_count: number }
  today_accuracy: number
  total_question_count: number
  accuracy: number
  wrong_count: number
  due_review_count: number
  streak_days: number
}

export interface ChapterMastery {
  chapter_id: number
  chapter_name: string
  subject_id: number
  subject_name: string
  attempts: number
  correct: number
  accuracy: number
  mastery_score: number
}

export interface ReviewConfig {
  id: number
  user_id: number
  scheduler: string
  params: string
}
