-- import-agent.v2: conversation-first, multi-document import sessions.

CREATE TABLE import_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_id INTEGER REFERENCES question_banks(id) ON DELETE SET NULL,
    title TEXT NOT NULL DEFAULT '新建导入会话',
    status TEXT NOT NULL DEFAULT 'empty',
    pipeline_version TEXT NOT NULL DEFAULT 'import-agent.v2',
    legacy_import_job_id INTEGER UNIQUE REFERENCES import_jobs(id) ON DELETE SET NULL,
    attachment_revision INTEGER NOT NULL DEFAULT 0,
    draft_version INTEGER NOT NULL DEFAULT 1,
    active_run_id INTEGER,
    expected_count INTEGER,
    unique_count INTEGER NOT NULL DEFAULT 0,
    complete_count INTEGER NOT NULL DEFAULT 0,
    blocking_issue_count INTEGER NOT NULL DEFAULT 0,
    last_preview_hash TEXT,
    last_preview_version INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    archived_at DATETIME
);

CREATE TABLE import_session_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    import_job_id INTEGER REFERENCES import_jobs(id) ON DELETE SET NULL,
    document_role TEXT NOT NULL DEFAULT 'unknown',
    role_confidence REAL NOT NULL DEFAULT 0,
    role_status TEXT NOT NULL DEFAULT 'inferred',
    profile_json TEXT,
    attachment_revision INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'uploaded',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_id, document_id)
);

CREATE TABLE import_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    trigger_message_id INTEGER,
    run_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    base_attachment_revision INTEGER NOT NULL,
    base_draft_version INTEGER NOT NULL,
    result_draft_version INTEGER,
    action_json TEXT,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE import_session_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    parent_message_id INTEGER REFERENCES import_session_messages(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'text',
    content TEXT NOT NULL DEFAULT '',
    metadata_json TEXT,
    status TEXT NOT NULL DEFAULT 'completed',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE import_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    message_id INTEGER REFERENCES import_session_messages(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    stage TEXT,
    agent_role TEXT,
    document_id INTEGER REFERENCES documents(id) ON DELETE SET NULL,
    owned_page_start INTEGER,
    owned_page_end INTEGER,
    context_page_start INTEGER,
    context_page_end INTEGER,
    current_count INTEGER,
    total_count INTEGER,
    summary TEXT,
    payload_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE import_problems (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    document_id INTEGER REFERENCES documents(id) ON DELETE SET NULL,
    problem_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'blocking',
    page_start INTEGER,
    page_end INTEGER,
    candidate_id INTEGER REFERENCES candidate_questions(id) ON DELETE SET NULL,
    source_identity TEXT,
    detail_json TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    resolved_by_run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE import_directory_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    unit_id INTEGER REFERENCES import_agent_units(id) ON DELETE SET NULL,
    content_role TEXT NOT NULL,
    document_role TEXT NOT NULL,
    section_path_json TEXT NOT NULL DEFAULT '[]',
    normalized_section_path TEXT NOT NULL DEFAULT '',
    set_no INTEGER NOT NULL DEFAULT 0,
    question_type TEXT NOT NULL DEFAULT '',
    source_question_no INTEGER NOT NULL,
    source_page_start INTEGER NOT NULL,
    source_page_end INTEGER NOT NULL,
    anchor TEXT,
    marker_top INTEGER,
    source_identity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_id, document_id, source_identity)
);

CREATE TABLE candidate_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    candidate_id INTEGER NOT NULL REFERENCES candidate_questions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    directory_entry_id INTEGER REFERENCES import_directory_entries(id) ON DELETE SET NULL,
    source_role TEXT NOT NULL,
    page_start INTEGER NOT NULL,
    page_end INTEGER NOT NULL,
    source_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(candidate_id, document_id, source_role, page_start, page_end)
);

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
    UNIQUE(session_id, document_id, normalized_section_path, set_no, question_type, source_question_no, source_kind, page_start)
);

CREATE TABLE import_action_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    message_id INTEGER REFERENCES import_session_messages(id) ON DELETE SET NULL,
    action_type TEXT NOT NULL,
    action_json TEXT NOT NULL,
    expected_draft_version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    confirmed_at DATETIME,
    executed_run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE import_jobs ADD COLUMN session_id INTEGER REFERENCES import_sessions(id) ON DELETE SET NULL;
ALTER TABLE import_agent_units ADD COLUMN run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL;
ALTER TABLE import_agent_units ADD COLUMN document_id INTEGER REFERENCES documents(id) ON DELETE SET NULL;
ALTER TABLE candidate_questions ADD COLUMN import_session_id INTEGER REFERENCES import_sessions(id) ON DELETE CASCADE;
ALTER TABLE candidate_questions ADD COLUMN import_run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL;

CREATE INDEX idx_import_sessions_user ON import_sessions(user_id, archived_at, updated_at);
CREATE INDEX idx_session_documents_session ON import_session_documents(session_id, status);
CREATE INDEX idx_import_runs_session ON import_runs(session_id, status, id);
CREATE UNIQUE INDEX uq_import_runs_active_mutation ON import_runs(session_id) WHERE status IN ('queued','running','merging');
CREATE INDEX idx_session_messages_session ON import_session_messages(session_id, id);
CREATE INDEX idx_import_events_cursor ON import_events(session_id, id);
CREATE INDEX idx_import_problems_session ON import_problems(session_id, status, severity);
CREATE INDEX idx_directory_session_identity ON import_directory_entries(session_id, normalized_section_path, set_no, question_type, source_question_no);
CREATE INDEX idx_candidate_sources_candidate ON candidate_sources(candidate_id, source_role);
CREATE INDEX idx_answer_records_identity ON import_answer_records(session_id, normalized_section_path, set_no, question_type, source_question_no);
CREATE INDEX idx_candidates_session ON candidate_questions(import_session_id, status, id);

-- One read-compatible conversation per historical import. New sessions have no
-- legacy_import_job_id and can aggregate multiple documents.
INSERT INTO import_sessions(user_id,bank_id,title,status,pipeline_version,legacy_import_job_id,draft_version,expected_count,unique_count,complete_count,blocking_issue_count,created_at,updated_at)
SELECT j.user_id,j.bank_id,d.original_name,j.status,'import-agent.v2',j.id,j.draft_version,j.expected_count,j.candidate_count,j.complete_count,j.blocking_issue_count,j.created_at,j.updated_at
FROM import_jobs j JOIN documents d ON d.id=j.document_id
WHERE j.session_id IS NULL;

UPDATE import_jobs SET session_id=(SELECT s.id FROM import_sessions s WHERE s.legacy_import_job_id=import_jobs.id) WHERE session_id IS NULL;

INSERT OR IGNORE INTO import_session_documents(session_id,document_id,import_job_id,document_role,role_confidence,role_status,attachment_revision,status,created_at,updated_at)
SELECT j.session_id,j.document_id,j.id,'unknown',0,'inferred',1,j.status,j.created_at,j.updated_at FROM import_jobs j WHERE j.session_id IS NOT NULL;

UPDATE candidate_questions SET import_session_id=(SELECT j.session_id FROM import_jobs j WHERE j.id=candidate_questions.import_job_id) WHERE import_session_id IS NULL AND import_job_id IS NOT NULL;

INSERT INTO import_session_messages(session_id,role,message_type,content,metadata_json,status,created_at)
SELECT j.session_id,m.role,'text',m.content,m.action_json,m.status,m.created_at FROM import_messages m JOIN import_jobs j ON j.id=m.import_job_id WHERE j.session_id IS NOT NULL;
