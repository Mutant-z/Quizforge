-- vision-agent.v1: durable visual extraction, drafts, and repair conversations.

ALTER TABLE ai_providers ADD COLUMN supports_vision INTEGER NOT NULL DEFAULT 0;

ALTER TABLE import_jobs ADD COLUMN pipeline_version TEXT NOT NULL DEFAULT 'legacy-text.v1';
ALTER TABLE import_jobs ADD COLUMN draft_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE import_jobs ADD COLUMN directory_complete INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN expected_count INTEGER;
ALTER TABLE import_jobs ADD COLUMN extracted_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN complete_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN total_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN completed_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN failed_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN blocking_issue_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN provider_id_snapshot INTEGER;
ALTER TABLE import_jobs ADD COLUMN model_snapshot TEXT;

ALTER TABLE candidate_questions ADD COLUMN source_identity TEXT;
ALTER TABLE candidate_questions ADD COLUMN section_path_json TEXT;
ALTER TABLE candidate_questions ADD COLUMN set_no INTEGER NOT NULL DEFAULT 0;
ALTER TABLE candidate_questions ADD COLUMN source_question_no INTEGER NOT NULL DEFAULT 0;
ALTER TABLE candidate_questions ADD COLUMN question_type TEXT;
ALTER TABLE candidate_questions ADD COLUMN question_json TEXT;
ALTER TABLE candidate_questions ADD COLUMN answer_json TEXT;
ALTER TABLE candidate_questions ADD COLUMN merged_json TEXT;
ALTER TABLE candidate_questions ADD COLUMN question_page_start INTEGER;
ALTER TABLE candidate_questions ADD COLUMN question_page_end INTEGER;
ALTER TABLE candidate_questions ADD COLUMN answer_page_start INTEGER;
ALTER TABLE candidate_questions ADD COLUMN answer_page_end INTEGER;
ALTER TABLE candidate_questions ADD COLUMN visual_confidence REAL;
ALTER TABLE candidate_questions ADD COLUMN validation_issues_json TEXT;
ALTER TABLE candidate_questions ADD COLUMN draft_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE candidate_questions ADD COLUMN pipeline_version TEXT NOT NULL DEFAULT 'legacy-text.v1';

CREATE TABLE document_page_assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    page_no INTEGER NOT NULL,
    render_version TEXT NOT NULL,
    purpose TEXT NOT NULL,
    file_path TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ready',
    error TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(document_id, page_no, render_version, purpose)
);

CREATE TABLE import_agent_units (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    import_job_id INTEGER NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    unit_type TEXT NOT NULL,
    unit_key TEXT NOT NULL,
    owned_page_start INTEGER NOT NULL,
    owned_page_end INTEGER NOT NULL,
    context_page_start INTEGER NOT NULL,
    context_page_end INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0,
    input_hash TEXT NOT NULL,
    output_json TEXT,
    validation_json TEXT,
    error_code TEXT,
    error_message TEXT,
    lease_until DATETIME,
    pipeline_version TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(import_job_id, unit_type, unit_key, pipeline_version)
);

CREATE TABLE import_agent_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    unit_id INTEGER NOT NULL REFERENCES import_agent_units(id) ON DELETE CASCADE,
    agent_role TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    provider_id INTEGER,
    provider_name TEXT,
    model TEXT,
    status TEXT NOT NULL,
    retry_index INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    raw_output TEXT,
    parsed_output TEXT,
    validation_error TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE import_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    import_job_id INTEGER NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    action_json TEXT,
    status TEXT NOT NULL DEFAULT 'completed',
    draft_version_before INTEGER,
    draft_version_after INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_page_assets_document ON document_page_assets(document_id, page_no);
CREATE INDEX idx_agent_units_job ON import_agent_units(import_job_id, status, unit_type);
CREATE INDEX idx_agent_runs_unit ON import_agent_runs(unit_id, created_at);
CREATE INDEX idx_import_messages_job ON import_messages(import_job_id, id);
CREATE INDEX idx_candidates_source_identity ON candidate_questions(import_job_id, source_identity);
CREATE UNIQUE INDEX uq_candidates_import_hash ON candidate_questions(import_job_id, input_hash);
