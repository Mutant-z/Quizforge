# 09. 数据库设计

## 1. 总体原则

SQLite：

- WAL；
- Foreign Key；
- Transaction；
- Index；
- FTS5；
- sqlite-vec。

---

## 2. 核心表

```text
users
question_banks
subjects
chapters
knowledge_points
canonical_questions
question_options
question_sources
question_knowledge_points
practice_sessions
question_attempts
wrong_questions
question_notes
favorites
documents
document_pages
candidate_questions
pipeline_runs
pipeline_node_runs
import_jobs
jobs
rag_documents
rag_embeddings
ai_providers
agent_sessions
agent_messages
review_configs
quality_alerts
question_conflicts
```

---

## 3. canonical_questions

```sql
CREATE TABLE canonical_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bank_id INTEGER NOT NULL,
    subject_id INTEGER,
    chapter_id INTEGER,
    type TEXT NOT NULL,
    stem TEXT NOT NULL,
    answer_json TEXT,
    original_analysis TEXT,
    ai_enhanced_analysis TEXT,
    difficulty REAL,
    quality_score REAL,
    status TEXT NOT NULL DEFAULT 'draft',
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

---

## 4. question_sources

```sql
CREATE TABLE question_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_question_id INTEGER,
    document_id INTEGER NOT NULL,
    candidate_id INTEGER,
    page_start INTEGER,
    page_end INTEGER,
    raw_text TEXT NOT NULL,
    original_answer TEXT,
    original_analysis TEXT,
    source_hash TEXT,
    created_at DATETIME NOT NULL
);
```

---

## 5. question_options

```sql
CREATE TABLE question_options (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_id INTEGER NOT NULL,
    option_key TEXT NOT NULL,
    content TEXT NOT NULL,
    sort_order INTEGER NOT NULL
);
```

---

## 6. chapters

```sql
CREATE TABLE chapters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_id INTEGER NOT NULL,
    parent_id INTEGER,
    name TEXT NOT NULL,
    normalized_name TEXT,
    level INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active'
);
```

---

## 7. question_attempts

```sql
CREATE TABLE question_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    question_id INTEGER NOT NULL,
    session_id INTEGER,
    answer_json TEXT,
    is_correct INTEGER,
    score REAL,
    duration_ms INTEGER,
    created_at DATETIME NOT NULL
);
```

---

## 8. wrong_questions

```sql
CREATE TABLE wrong_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    question_id INTEGER NOT NULL,
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
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(user_id, question_id)
);
```

---

## 9. candidate_questions

```sql
CREATE TABLE candidate_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL,
    page_start INTEGER,
    page_end INTEGER,
    raw_text TEXT NOT NULL,
    normalized_text TEXT,
    input_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    confidence REAL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_code TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

---

## 10. pipeline_node_runs

```sql
CREATE TABLE pipeline_node_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    candidate_id INTEGER,
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
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

---

## 11. 关键索引

必须创建：

```text
canonical_questions(bank_id, subject_id, chapter_id)
question_attempts(user_id, question_id, created_at)
wrong_questions(user_id, next_review_at)
question_sources(canonical_question_id)
candidate_questions(document_id, status)
jobs(status, type)
```

---

## 12. FTS5

建立 Question Search 虚拟表：

```text
stem
analysis
knowledge_points
```

---

## 13. Migration

严禁应用启动时随意修改 Schema。

使用：

```text
migrations/
0001_init.sql
0002_add_question_source.sql
...
```

每个 migration 只执行一次。
