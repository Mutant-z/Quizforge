-- 错题导入材料分析题型与答案续图状态。
ALTER TABLE wrong_import_drafts ADD COLUMN answer_status TEXT NOT NULL DEFAULT 'missing';
ALTER TABLE wrong_import_drafts ADD COLUMN user_marked_no_answer INTEGER NOT NULL DEFAULT 0;

UPDATE wrong_import_drafts
SET answer_status = CASE
    WHEN answer_json IS NOT NULL AND length(trim(answer_json)) > 2 AND answer_json != '[]' THEN 'provided'
    WHEN answer_source = 'image' THEN 'unresolved'
    WHEN answer_source = 'none' THEN 'missing'
    ELSE 'pending'
END
WHERE answer_status = 'missing';

CREATE INDEX IF NOT EXISTS idx_wi_drafts_answer_status
    ON wrong_import_drafts(session_id, answer_status, review_status);

-- 每一条答案图片识别结果先落盘，再进行跨图拼接和题目匹配。
-- payload_json 保留模型原始结构，失败后无需用户重新上传图片。
CREATE TABLE IF NOT EXISTS wrong_import_answer_fragments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    image_id INTEGER NOT NULL REFERENCES wrong_import_images(id) ON DELETE CASCADE,
    batch_id TEXT,
    batch_index INTEGER,
    fragment_index INTEGER NOT NULL DEFAULT 0,
    section_title TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL DEFAULT 0,
    question_hint TEXT NOT NULL DEFAULT '',
    answer_json TEXT NOT NULL DEFAULT '[]',
    analysis TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    starts_mid_answer INTEGER NOT NULL DEFAULT 0,
    ends_mid_answer INTEGER NOT NULL DEFAULT 0,
    fragment_role TEXT NOT NULL DEFAULT 'source',
    status TEXT NOT NULL DEFAULT 'unresolved',
    matched_draft_id INTEGER REFERENCES wrong_import_drafts(id) ON DELETE SET NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wi_answer_fragments_batch
    ON wrong_import_answer_fragments(session_id, batch_id, batch_index, fragment_index);
CREATE INDEX IF NOT EXISTS idx_wi_answer_fragments_status
    ON wrong_import_answer_fragments(session_id, status, id);
