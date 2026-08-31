-- 0020_wrong_import_cross_image_reconcile.sql
-- 错题图片跨图边界识别、来源追踪与可撤销合并。

CREATE TABLE IF NOT EXISTS wrong_import_image_batches (
    id TEXT PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    expected_count INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'uploading', -- uploading | sealed | reconciling | done | failed
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    sealed_at DATETIME,
    completed_at DATETIME,
    UNIQUE(session_id, id)
);
CREATE INDEX IF NOT EXISTS idx_wi_batches_session ON wrong_import_image_batches(session_id, status);

ALTER TABLE wrong_import_images ADD COLUMN batch_id TEXT;
ALTER TABLE wrong_import_images ADD COLUMN batch_index INTEGER;
CREATE INDEX IF NOT EXISTS idx_wi_images_batch ON wrong_import_images(session_id, batch_id, batch_index);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wi_images_batch_order
    ON wrong_import_images(session_id, batch_id, batch_index)
    WHERE batch_id IS NOT NULL AND batch_index IS NOT NULL;

ALTER TABLE wrong_import_drafts ADD COLUMN boundary_json TEXT;
ALTER TABLE wrong_import_drafts ADD COLUMN merge_status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE wrong_import_drafts ADD COLUMN merged_into_draft_id INTEGER REFERENCES wrong_import_drafts(id) ON DELETE SET NULL;
ALTER TABLE wrong_import_drafts ADD COLUMN content_version INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_wi_drafts_merge ON wrong_import_drafts(session_id, merge_status, review_status);

CREATE TABLE IF NOT EXISTS wrong_import_draft_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    draft_id INTEGER NOT NULL REFERENCES wrong_import_drafts(id) ON DELETE CASCADE,
    image_id INTEGER NOT NULL REFERENCES wrong_import_images(id) ON DELETE CASCADE,
    source_order INTEGER NOT NULL DEFAULT 0,
    fragment_role TEXT NOT NULL DEFAULT 'source', -- source | continuation
    region_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(draft_id, image_id)
);
CREATE INDEX IF NOT EXISTS idx_wi_draft_sources_draft ON wrong_import_draft_sources(draft_id, source_order);
CREATE INDEX IF NOT EXISTS idx_wi_draft_sources_image ON wrong_import_draft_sources(image_id, draft_id);

CREATE TABLE IF NOT EXISTS wrong_import_merge_candidates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES wrong_import_sessions(id) ON DELETE CASCADE,
    batch_id TEXT,
    left_draft_id INTEGER NOT NULL REFERENCES wrong_import_drafts(id) ON DELETE CASCADE,
    right_draft_id INTEGER NOT NULL REFERENCES wrong_import_drafts(id) ON DELETE CASCADE,
    score REAL NOT NULL DEFAULT 0,
    reason_code TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL DEFAULT 'review', -- merge | review | separate
    status TEXT NOT NULL DEFAULT 'pending', -- pending | accepted | rejected | undone
    before_json TEXT,
    after_json TEXT,
    target_version_after INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_id, left_draft_id, right_draft_id, status)
);
CREATE INDEX IF NOT EXISTS idx_wi_merge_candidates_session ON wrong_import_merge_candidates(session_id, status, id);

-- 为迁移前已经存在的草稿补齐单图来源。后续新草稿由应用层同步写入。
INSERT OR IGNORE INTO wrong_import_draft_sources (draft_id, image_id, source_order, fragment_role, region_json)
SELECT id, image_id, 0, 'source', region_json
FROM wrong_import_drafts;
