-- 0003_rag_agent.sql
-- RAG / Agent / 质量 / 冲突 / 配置

-- RAG 文档（question / question_analysis / document_chunk / knowledge_point / user_note）
CREATE TABLE IF NOT EXISTS rag_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type TEXT NOT NULL,
    question_id INTEGER,
    document_id INTEGER,
    document_page INTEGER,
    bank_id INTEGER,
    subject_id INTEGER,
    chapter_id INTEGER,
    knowledge_point TEXT,
    difficulty REAL,
    question_type TEXT,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    stale INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(source_type, content_hash)
);
CREATE INDEX IF NOT EXISTS idx_rag_docs_q ON rag_documents(question_id);
CREATE INDEX IF NOT EXISTS idx_rag_docs_stale ON rag_documents(stale);

-- 向量（BLOB，float32[]）
CREATE TABLE IF NOT EXISTS rag_embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rag_document_id INTEGER NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    vector BLOB NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- FTS5 全文索引（trigram 支持中英文子串检索）
CREATE VIRTUAL TABLE IF NOT EXISTS rag_fts USING fts5(
    content, source_type, bank_id, subject_id, chapter_id, question_id,
    tokenize = 'trigram'
);

-- AI Provider（Key 加密存储）
CREATE TABLE IF NOT EXISTS ai_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'openai',
    base_url TEXT NOT NULL,
    api_key_encrypted TEXT NOT NULL,
    chat_model TEXT,
    embedding_model TEXT,
    rerank_model TEXT,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Agent 会话
CREATE TABLE IF NOT EXISTS agent_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '新会话',
    context_json TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_user ON agent_sessions(user_id, created_at);

-- Agent 消息
CREATE TABLE IF NOT EXISTS agent_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT,
    tool_calls_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 复习配置
CREATE TABLE IF NOT EXISTS review_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scheduler TEXT NOT NULL DEFAULT 'simple_v1',
    params_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id)
);

-- 质量告警
CREATE TABLE IF NOT EXISTS quality_alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_id INTEGER,
    import_job_id INTEGER,
    alert_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    message TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 题目冲突
CREATE TABLE IF NOT EXISTS question_conflicts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_question_id INTEGER,
    source_a TEXT,
    source_b TEXT,
    field TEXT NOT NULL,
    value_a TEXT,
    value_b TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    resolved_by INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_conflicts_status ON question_conflicts(status);
