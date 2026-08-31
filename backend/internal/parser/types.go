// Package parser 负责文档解析与候选题构建（docs/05）。
package parser

// Page 一页文本。
type Page struct {
	PageNo        int
	Text          string
	NeedsOCR      bool // 无文本层 → 需要 OCR
	TextSource    string
	OCRStatus     string
	OCRConfidence float64
	OCRBoxesJSON  string
}

// Candidate 候选题。
type Candidate struct {
	ID             int64   `json:"id"`
	DocumentID     int64   `json:"document_id"`
	ImportJobID    *int64  `json:"import_job_id,omitempty"`
	PageStart      int     `json:"page_start"`
	PageEnd        int     `json:"page_end"`
	RawText        string  `json:"raw_text"`
	NormalizedText string  `json:"normalized_text,omitempty"`
	InputHash      string  `json:"-"`
	Status         string  `json:"status"`
	Confidence     float64 `json:"confidence,omitempty"`
	ExtractedJSON  string  `json:"extracted_json,omitempty"`
	ErrorCode      string  `json:"error_code,omitempty"`
	RetryCount     int     `json:"retry_count"`
}
