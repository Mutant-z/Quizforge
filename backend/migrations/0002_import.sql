-- 0002_import.sql
-- 上传 / 导入 / 流水线 / Job Queue

-- 上传文件（原始文档）
CREATE TABLE IF NOT EXISTS documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_id INTEGER REFERENCES question_banks(id) ON DELETE SET NULL,
    original_name TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_hash TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    page_count INTEGER,
    is_scanned INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'uploaded',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(file_hash)
);
CREATE INDEX IF NOT EXISTS idx_documents_user ON documents(user_id);

-- 文档页
CREATE TABLE IF NOT EXISTS document_pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    page_no INTEGER NOT NULL,
    text TEXT,
    text_len INTEGER NOT NULL DEFAULT 0,
    needs_ocr INTEGER NOT NULL DEFAULT 0,
    UNIQUE(document_id, page_no)
);

-- 导入任务
CREATE TABLE IF NOT EXISTS import_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_id INTEGER REFERENCES question_banks(id) ON DELETE SET NULL,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'uploaded',
    stage TEXT,
    total_pages INTEGER NOT NULL DEFAULT 0,
    processed_pages INTEGER NOT NULL DEFAULT 0,
    candidate_count INTEGER NOT NULL DEFAULT 0,
    parsed_count INTEGER NOT NULL DEFAULT 0,
    new_count INTEGER NOT NULL DEFAULT 0,
    duplicate_count INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    review_count INTEGER NOT NULL DEFAULT 0,
    progress REAL NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_import_jobs_user ON import_jobs(user_id, status);

-- 导入日志
CREATE TABLE IF NOT EXISTS import_task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    import_job_id INTEGER NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    stage TEXT,
    message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 候选题
CREATE TABLE IF NOT EXISTS candidate_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    import_job_id INTEGER REFERENCES import_jobs(id) ON DELETE CASCADE,
    page_start INTEGER,
    page_end INTEGER,
    raw_text TEXT NOT NULL,
    normalized_text TEXT,
    input_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    confidence REAL,
    extracted_json TEXT,
    error_code TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_candidates_doc ON candidate_questions(document_id, status);

-- 题目来源
CREATE TABLE IF NOT EXISTS question_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_question_id INTEGER REFERENCES canonical_questions(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    candidate_id INTEGER,
    page_start INTEGER,
    page_end INTEGER,
    raw_text TEXT NOT NULL,
    original_answer TEXT,
    original_analysis TEXT,
    source_hash TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_question_sources_q ON question_sources(canonical_question_id);
CREATE INDEX IF NOT EXISTS idx_question_sources_hash ON question_sources(source_hash);

-- Job Queue
CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    lease_until DATETIME,
    worker_id TEXT,
    error_code TEXT,
    error_message TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, type, priority);

-- Pipeline 运行记录
CREATE TABLE IF NOT EXISTS pipeline_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    import_job_id INTEGER REFERENCES import_jobs(id) ON DELETE CASCADE,
    pipeline_version TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Pipeline 节点运行记录（幂等 + 可观测）
CREATE TABLE IF NOT EXISTS pipeline_node_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    candidate_id INTEGER,
    import_job_id INTEGER,
    node_name TEXT NOT NULL,
    node_version TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    output_hash TEXT,
    status TEXT NOT NULL,
    provider TEXT,
    model TEXT,
    prompt_version TEXT,
    latency_ms INTEGER,
    input_tokens INTEGER,
    output_tokens INTEGER,
    error_code TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_node_runs_candidate ON pipeline_node_runs(candidate_id, node_name);
CREATE INDEX IF NOT EXISTS idx_node_runs_job ON pipeline_node_runs(import_job_id);
