-- Answer records are keyed by the immutable source identity, not by the
-- lossy normalized topic/type/number/page tuple. The old tuple merged distinct
-- subjective blocks such as 辨析题 1 and 简答题 1 when they shared a page.
-- Keep all existing rows and audit fields while removing that lossy UNIQUE.
ALTER TABLE import_answer_records RENAME TO import_answer_records_legacy;

CREATE TABLE import_answer_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    normalized_section_path TEXT NOT NULL DEFAULT '',
    section_path_json TEXT NOT NULL DEFAULT '[]',
    set_no INTEGER NOT NULL DEFAULT 0,
    question_type TEXT NOT NULL DEFAULT '',
    source_question_no INTEGER NOT NULL,
    answer_json TEXT NOT NULL,
    analysis TEXT,
    page_start INTEGER NOT NULL,
    page_end INTEGER NOT NULL,
    source_kind TEXT NOT NULL DEFAULT 'answer',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    canonical_question_type TEXT NOT NULL DEFAULT '',
    section_scope_id TEXT NOT NULL DEFAULT '',
    link_status TEXT NOT NULL DEFAULT 'unresolved',
    source_identity TEXT NOT NULL DEFAULT ''
);

INSERT INTO import_answer_records(
    id,session_id,document_id,run_id,normalized_section_path,section_path_json,
    set_no,question_type,source_question_no,answer_json,analysis,page_start,
    page_end,source_kind,created_at,canonical_question_type,section_scope_id,
    link_status,source_identity
)
SELECT
    id,session_id,document_id,run_id,normalized_section_path,section_path_json,
    set_no,question_type,source_question_no,answer_json,analysis,page_start,
    page_end,source_kind,created_at,canonical_question_type,section_scope_id,
    link_status,source_identity
FROM import_answer_records_legacy;

DROP TABLE import_answer_records_legacy;

CREATE INDEX idx_answer_records_identity
    ON import_answer_records(session_id, normalized_section_path, set_no, question_type, source_question_no);
CREATE INDEX idx_answer_scope
    ON import_answer_records(session_id, section_scope_id, canonical_question_type, set_no, source_question_no, link_status);
CREATE INDEX idx_answer_records_source_identity
    ON import_answer_records(session_id, document_id, source_identity, source_kind, page_start);
