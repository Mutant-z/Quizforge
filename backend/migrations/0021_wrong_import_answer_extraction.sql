-- 错题导入支持答案参考图：答案图只回填已有草稿，不创建新题。

ALTER TABLE wrong_import_images ADD COLUMN extraction_mode TEXT NOT NULL DEFAULT 'questions';
CREATE INDEX IF NOT EXISTS idx_wi_images_extraction_mode
    ON wrong_import_images(session_id, extraction_mode, status);
