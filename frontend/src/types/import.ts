export interface CandidateQuestion {
  id: number
  document_id: number
  import_job_id?: number
  page_start: number
  page_end: number
  raw_text: string
  status: string
  confidence?: number
  extracted_json?: string
  error_code?: string
}
