-- 0001_init.sql
-- QuizTrace 核心 Schema：用户 / 题库 / 题目 / 刷题 / 错题 / 复习 / 统计

-- 用户
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    avatar TEXT,
    bio TEXT,
    default_bank_id INTEGER,
    study_preferences TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

-- 题库
CREATE TABLE IF NOT EXISTS question_banks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    cover TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    status TEXT NOT NULL DEFAULT 'active',
    question_count INTEGER NOT NULL DEFAULT 0,
    created_by INTEGER REFERENCES users(id),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 科目
CREATE TABLE IF NOT EXISTS subjects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bank_id INTEGER NOT NULL REFERENCES question_banks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(bank_id, name)
);
CREATE INDEX IF NOT EXISTS idx_subjects_bank ON subjects(bank_id);

-- 章节（树形：level 1 章 / 2 节 / 3 小节）
CREATE TABLE IF NOT EXISTS chapters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    parent_id INTEGER REFERENCES chapters(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    normalized_name TEXT,
    level INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_chapters_subject ON chapters(subject_id, parent_id);

-- 知识点
CREATE TABLE IF NOT EXISTS knowledge_points (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bank_id INTEGER NOT NULL REFERENCES question_banks(id) ON DELETE CASCADE,
    subject_id INTEGER REFERENCES subjects(id),
    chapter_id INTEGER REFERENCES chapters(id),
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 规范题
CREATE TABLE IF NOT EXISTS canonical_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bank_id INTEGER NOT NULL REFERENCES question_banks(id) ON DELETE CASCADE,
    subject_id INTEGER REFERENCES subjects(id),
    chapter_id INTEGER REFERENCES chapters(id),
    type TEXT NOT NULL,
    stem TEXT NOT NULL,
    options_json TEXT,
    answer_json TEXT,
    original_analysis TEXT,
    ai_enhanced_analysis TEXT,
    difficulty REAL NOT NULL DEFAULT 3,
    knowledge_points_json TEXT,
    quality_score REAL NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft',
    source_hash TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_questions_bank ON canonical_questions(bank_id, subject_id, chapter_id);
CREATE INDEX IF NOT EXISTS idx_questions_status ON canonical_questions(status);
CREATE INDEX IF NOT EXISTS idx_questions_hash ON canonical_questions(source_hash);

-- 选项
CREATE TABLE IF NOT EXISTS question_options (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    option_key TEXT NOT NULL,
    content TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_options_question ON question_options(question_id);

-- 题目-知识点关联
CREATE TABLE IF NOT EXISTS question_knowledge_points (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    knowledge_point_id INTEGER NOT NULL REFERENCES knowledge_points(id) ON DELETE CASCADE,
    UNIQUE(question_id, knowledge_point_id)
);

-- 刷题会话
CREATE TABLE IF NOT EXISTS practice_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_id INTEGER REFERENCES question_banks(id),
    subject_id INTEGER REFERENCES subjects(id),
    chapter_id INTEGER REFERENCES chapters(id),
    mode TEXT NOT NULL DEFAULT 'sequential',
    question_ids_json TEXT NOT NULL,
    current_index INTEGER NOT NULL DEFAULT 0,
    total_count INTEGER NOT NULL DEFAULT 0,
    correct_count INTEGER NOT NULL DEFAULT 0,
    wrong_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON practice_sessions(user_id, status);

-- 答题记录
CREATE TABLE IF NOT EXISTS question_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    session_id INTEGER REFERENCES practice_sessions(id),
    answer_json TEXT,
    is_correct INTEGER,
    score REAL,
    duration_ms INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_attempts_user_q ON question_attempts(user_id, question_id, created_at);
CREATE INDEX IF NOT EXISTS idx_attempts_user_time ON question_attempts(user_id, created_at);

-- 错题本
CREATE TABLE IF NOT EXISTS wrong_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    wrong_count INTEGER NOT NULL DEFAULT 0,
    correct_count INTEGER NOT NULL DEFAULT 0,
    review_count INTEGER NOT NULL DEFAULT 0,
    mastery_score REAL NOT NULL DEFAULT 0,
    interval_days REAL NOT NULL DEFAULT 0,
    difficulty_factor REAL NOT NULL DEFAULT 1,
    priority_score REAL NOT NULL DEFAULT 0,
    first_wrong_at DATETIME,
    last_wrong_at DATETIME,
    last_review_at DATETIME,
    next_review_at DATETIME,
    status TEXT NOT NULL DEFAULT 'learning',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, question_id)
);
CREATE INDEX IF NOT EXISTS idx_wrong_user_due ON wrong_questions(user_id, next_review_at);
CREATE INDEX IF NOT EXISTS idx_wrong_user_status ON wrong_questions(user_id, status);

-- 错题笔记（一用户一题一份主笔记）
CREATE TABLE IF NOT EXISTS question_notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    content TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, question_id)
);

-- 收藏
CREATE TABLE IF NOT EXISTS favorites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id INTEGER NOT NULL REFERENCES canonical_questions(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, question_id)
);

-- 复习记录
CREATE TABLE IF NOT EXISTS review_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    wrong_question_id INTEGER NOT NULL REFERENCES wrong_questions(id) ON DELETE CASCADE,
    result TEXT NOT NULL,
    is_correct INTEGER,
    interval_days REAL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_review_records_wq ON review_records(wrong_question_id);

-- 每日学习统计
CREATE TABLE IF NOT EXISTS learning_statistics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stat_date TEXT NOT NULL,
    question_count INTEGER NOT NULL DEFAULT 0,
    correct_count INTEGER NOT NULL DEFAULT 0,
    wrong_count INTEGER NOT NULL DEFAULT 0,
    review_count INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, stat_date)
);
