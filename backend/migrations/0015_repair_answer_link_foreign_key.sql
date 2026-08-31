-- Migration 0014 rebuilds import_answer_records. SQLite updates the child
-- table's foreign-key text to the temporary legacy name during that rebuild;
-- recreate the link projection so it points at the live answer table.
ALTER TABLE candidate_answer_links RENAME TO candidate_answer_links_legacy;

CREATE TABLE candidate_answer_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    candidate_id INTEGER NOT NULL REFERENCES candidate_questions(id) ON DELETE CASCADE,
    answer_record_id INTEGER NOT NULL REFERENCES import_answer_records(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'proposed',
    method TEXT NOT NULL DEFAULT 'exact',
    confidence REAL NOT NULL DEFAULT 0,
    detail_json TEXT,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(candidate_id, answer_record_id, run_id)
);

INSERT INTO candidate_answer_links(
    id,session_id,candidate_id,answer_record_id,status,method,confidence,
    detail_json,run_id,created_at,updated_at
)
SELECT
    id,session_id,candidate_id,answer_record_id,status,method,confidence,
    detail_json,run_id,created_at,updated_at
FROM candidate_answer_links_legacy;

DROP TABLE candidate_answer_links_legacy;

CREATE INDEX idx_answer_links_candidate
    ON candidate_answer_links(session_id, candidate_id, status);
