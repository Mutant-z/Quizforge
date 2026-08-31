export interface WrongImportSession {
  id: number
  user_id: number
  title: string
  status: 'active' | 'committed' | 'archived'
  target_bank_id?: number
  draft_count: number
  ready_count: number
  missing_answer_count: number
  created_at: string
  updated_at: string
  committed_at?: string
}

export interface WrongImportImage {
  id: number
  session_id: number
  batch_id?: string
  batch_index?: number
  extraction_mode?: 'questions' | 'answer_key'
  original_name?: string
  file_hash?: string
  mime_type: string
  size_bytes: number
  width: number
  height: number
  status: 'pending' | 'processing' | 'done' | 'failed'
  error_code?: string
  error_message?: string
  draft_count: number
  created_at: string
  updated_at: string
}

export interface WrongImportImageBatch {
  id: string
  session_id: number
  expected_count: number
  status: 'uploading' | 'sealed' | 'reconciling' | 'done' | 'failed'
  created_at: string
  sealed_at?: string
  completed_at?: string
}

export interface WrongImportAnswerFragment {
  id: number
  session_id: number
  image_id: number
  batch_id?: string
  batch_index?: number
  fragment_index: number
  section_title?: string
  seq: number
  question_hint?: string
  answer: string[]
  analysis?: string
  confidence: number
  starts_mid_answer: boolean
  ends_mid_answer: boolean
  fragment_role: string
  status: 'unresolved' | 'matched'
  matched_draft_id?: number
}

export interface WrongImportBoundary {
  starts_mid_question: boolean
  ends_mid_question: boolean
  start_marker?: string
  end_marker?: string
}

export interface WrongImportDraftSource {
  id: number
  draft_id: number
  image_id: number
  source_order: number
  fragment_role: string
  region?: { left: number; top: number; right: number; bottom: number }
}

export interface WrongImportDraft {
  id: number
  session_id: number
  image_id: number
  seq_no: number
  question_type: string
  stem: string
  options: { key: string; content: string }[]
  answer: string[]
  answer_source: 'image' | 'user' | 'none' | 'pending'
  answer_status: 'pending' | 'missing' | 'provided' | 'explicit_none' | 'unresolved'
  user_marked_no_answer: boolean
  analysis?: string
  difficulty?: number
  knowledge_points?: string[]
  confidence: number
  warnings?: string[]
  region?: { left: number; top: number; right: number; bottom: number }
  boundary?: WrongImportBoundary
  sources?: WrongImportDraftSource[]
  merge_status: 'active' | 'merged'
  merged_into_draft_id?: number
  content_version: number
  duplicate_of_draft_id?: number
  review_status: 'extracted' | 'missing_answer' | 'ready' | 'excluded' | 'committed'
  committed_question_id?: number
  committed_wrong_id?: number
  created_at: string
  updated_at: string
}

export interface WrongImportMergeCandidate {
  id: number
  session_id: number
  batch_id?: string
  left_draft_id: number
  right_draft_id: number
  score: number
  reason_code: string
  decision: 'merge' | 'review' | 'separate'
  status: 'pending' | 'accepted' | 'rejected' | 'undone'
  created_at: string
  updated_at: string
}

export interface MessageAttachment {
  id?: string | number
  url: string
  name?: string
  type?: 'image' | 'file'
  size?: number
}

export interface WrongImportMessage {
  id: number | string
  session_id: number
  role: 'user' | 'agent' | 'system'
  content: string
  attachments?: MessageAttachment[]
  metadata_json?: string
  created_at: string
}

export interface WrongImportEvent {
  event_id: number
  session_id: number
  type: string
  payload_json?: string
  timestamp: string
}

export interface WrongImportSessionDetail {
  session: WrongImportSession
  images: WrongImportImage[]
  batches?: WrongImportImageBatch[]
  answer_fragments?: WrongImportAnswerFragment[]
  drafts: WrongImportDraft[]
  messages: WrongImportMessage[]
}

export interface WrongImportCommitResult {
  committed_count: number
  linked_count: number
  created_count: number
  bank_id: number
}
