-- Page-level catalog conservation and a derived identity used only for
-- matching. Immutable source_identity/origin_uid values are intentionally
-- left untouched.
CREATE TABLE import_page_coverage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    unit_id INTEGER REFERENCES import_agent_units(id) ON DELETE SET NULL,
    page_no INTEGER NOT NULL,
    source_role TEXT NOT NULL DEFAULT 'mixed',
    ocr_manifest_json TEXT NOT NULL DEFAULT '{}',
    catalog_manifest_json TEXT NOT NULL DEFAULT '{}',
    missing_json TEXT NOT NULL DEFAULT '[]',
    extra_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'unverifiable'
        CHECK(status IN ('verified','mismatch','repaired','unverifiable')),
    detector_version TEXT NOT NULL DEFAULT 'catalog-coverage.v1',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_id,document_id,page_no,source_role)
);
CREATE INDEX idx_page_coverage_session_status
    ON import_page_coverage(session_id,status,page_no);

ALTER TABLE import_directory_entries ADD COLUMN original_question_label TEXT NOT NULL DEFAULT '';
ALTER TABLE import_directory_entries ADD COLUMN match_scope_key TEXT NOT NULL DEFAULT '';
ALTER TABLE candidate_questions ADD COLUMN match_scope_key TEXT NOT NULL DEFAULT '';
ALTER TABLE import_answer_records ADD COLUMN match_scope_key TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_directory_match_scope
    ON import_directory_entries(session_id,match_scope_key,canonical_question_type,source_question_no);
CREATE INDEX idx_candidate_match_scope
    ON candidate_questions(import_session_id,match_scope_key,canonical_question_type,source_question_no);
CREATE INDEX idx_answer_match_scope
    ON import_answer_records(session_id,match_scope_key,canonical_question_type,source_question_no);

-- Existing rows are finalized by the application normalizer on first relink.
-- Copying the old scope here keeps queries useful before that pass runs.
UPDATE import_directory_entries SET match_scope_key=section_scope_id WHERE match_scope_key='';
UPDATE candidate_questions SET match_scope_key=section_scope_id WHERE match_scope_key='';
UPDATE import_answer_records SET match_scope_key=section_scope_id WHERE match_scope_key='';

DROP TRIGGER IF EXISTS trg_import_problem_provenance_after_insert;
CREATE TRIGGER trg_import_problem_provenance_after_insert
AFTER INSERT ON import_problems
BEGIN
  UPDATE import_problems
  SET origin_id=COALESCE(NEW.origin_id,(SELECT o.id FROM source_question_origins o WHERE o.session_id=NEW.session_id AND o.candidate_id=NEW.candidate_id LIMIT 1)),
      resolution_hint=CASE
        WHEN NEW.problem_type IN ('QUESTION_INVALID','QUESTION_MISSING','OPTIONS_MISSING','DIRECTORY_COVERAGE_MISMATCH') THEN 'retry_question'
        WHEN NEW.problem_type IN ('ANSWER_MISSING','ANSWER_UNRESOLVED','ANSWER_CONFLICT','ANSWER_INVALID','ANSWER_REQUIRED') THEN 'retry_answer'
        WHEN NEW.problem_type='ANSWER_SOURCE_IDENTITY_INVALID' THEN 'normalize_identity'
        ELSE 'manual_review' END
  WHERE id=NEW.id;
END;
