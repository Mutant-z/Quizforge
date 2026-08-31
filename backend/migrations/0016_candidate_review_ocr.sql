-- Candidate-level PDF review history and explicit OCR provenance.
CREATE TABLE IF NOT EXISTS candidate_revision_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    candidate_id INTEGER NOT NULL REFERENCES candidate_questions(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    target_role TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    change_status TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_candidate_revision_history_candidate
    ON candidate_revision_history(candidate_id, id);

ALTER TABLE document_pages ADD COLUMN text_source TEXT NOT NULL DEFAULT 'text_layer';
ALTER TABLE document_pages ADD COLUMN ocr_status TEXT NOT NULL DEFAULT 'not_required';
ALTER TABLE document_pages ADD COLUMN ocr_confidence REAL;
ALTER TABLE document_pages ADD COLUMN ocr_boxes_json TEXT;
ALTER TABLE document_pages ADD COLUMN ocr_provider TEXT;
ALTER TABLE document_pages ADD COLUMN ocr_version TEXT;

ALTER TABLE import_directory_entries ADD COLUMN marker_bottom INTEGER;
ALTER TABLE import_directory_entries ADD COLUMN region_json TEXT;
