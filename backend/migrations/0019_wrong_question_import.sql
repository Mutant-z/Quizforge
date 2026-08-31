-- 0019_wrong_question_import.sql
-- 错题导入 Agent：图片提取 → 侧栏草稿暂存 → 用户确认后入库
-- docs/06 多 Agent 原则（结构化输出、Orchestrator 控制）；docs/08 错题本语义。

-- 1) 题库类型：支撑每用户一个「我的错题本」个人题库
ALTER TABLE question_banks ADD COLUMN kind TEXT NOT NULL DEFAULT 'normal'; -- normal | wrong_book
CREATE INDEX IF NOT EXISTS idx_banks_kind ON question_banks(kind);
CREATE UNIQUE INDEX IF NOT EXISTS idx_banks_wrong_book_user ON question_banks(created_by)
    WHERE kind = 'wrong_book' AND created_by IS NOT NULL;

-- 2) 错题导入会话
CREATE TABLE IF NOT EXISTS wrong_import_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '错题导入',
    status TEXT NOT NULL DEFAULT 'active', -- active | committed | archived
    target_bank_id INTEGER REFERENCES question_banks(id) ON DELETE SET NULL,
    draft_count INTEGER NOT NULL DEFAULT 0,
    ready_count INTEGER NOT NULL DEFAULT 0,
    missing_answer_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    committed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_wi_sessions_user ON wrong_import_sessions(user_id, status);

-- 3) 会话图片
CREATE TABLE IF NOT EXISTS wrong_import_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    original_name TEXT,
    file_path TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    width INTEGER,
    height INTEGER,
    status TEXT NOT NULL DEFAULT 'pending', -- pending | processing | done | failed
    error_code TEXT,
    error_message TEXT,
    draft_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wi_images_session ON wrong_import_images(session_id, status);

-- 4) 提取草稿（暂存区核心表；确认前绝不写 canonical_questions）
CREATE TABLE IF NOT EXISTS wrong_import_drafts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    image_id INTEGER NOT NULL REFERENCES wrong_import_images(id) ON DELETE CASCADE,
    seq_no INTEGER NOT NULL,
    question_type TEXT NOT NULL DEFAULT 'single_choice',
    stem TEXT NOT NULL,
    options_json TEXT,
    answer_json TEXT,
    answer_source TEXT NOT NULL DEFAULT 'pending', -- image | user | none | pending
    analysis TEXT,
    difficulty REAL,
    knowledge_points_json TEXT,
    confidence REAL NOT NULL DEFAULT 0,
    warnings_json TEXT,
    region_json TEXT,                              -- 图内 bbox（0..1000 归一化）
    duplicate_of_draft_id INTEGER REFERENCES wrong_import_drafts(id) ON DELETE SET NULL,
    review_status TEXT NOT NULL DEFAULT 'extracted',
        -- extracted | missing_answer | ready | excluded | committed
    committed_question_id INTEGER REFERENCES canonical_questions(id) ON DELETE SET NULL,
    committed_wrong_id INTEGER REFERENCES wrong_questions(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wi_drafts_session ON wrong_import_drafts(session_id, review_status);
CREATE INDEX IF NOT EXISTS idx_wi_drafts_image ON wrong_import_drafts(image_id, seq_no);

-- 5) 会话消息（补答案等对话记录）
CREATE TABLE IF NOT EXISTS wrong_import_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL, -- user | agent | system
    content TEXT NOT NULL,
    metadata_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wi_messages_session ON wrong_import_messages(session_id, id);

-- 6) 轻量事件流（供侧栏 NDJSON 订阅，同 import_events 模式）
CREATE TABLE IF NOT EXISTS wrong_import_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    payload_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wi_events_session ON wrong_import_events(session_id, id);

-- 7) 错题来源标记（practice 刷题答错 | manual 手动 | import 图片导入）
ALTER TABLE wrong_questions ADD COLUMN source TEXT NOT NULL DEFAULT 'practice';
