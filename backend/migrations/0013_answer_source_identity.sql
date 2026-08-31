-- Persist the immutable answer-directory identity returned by the vision
-- extractor.  The section/path columns remain derived projections used by
-- older queries, while source_identity is the audit-safe repair key.
ALTER TABLE import_answer_records ADD COLUMN source_identity TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_answer_records_source_identity
ON import_answer_records(session_id, document_id, source_identity, source_kind, page_start);
