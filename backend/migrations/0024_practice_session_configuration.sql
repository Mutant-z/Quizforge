-- 0024_practice_session_configuration.sql
-- 多题库刷题会话、限时会话与进度更新时间。

ALTER TABLE practice_sessions ADD COLUMN bank_ids_json TEXT;
ALTER TABLE practice_sessions ADD COLUMN time_limit_seconds INTEGER;
ALTER TABLE practice_sessions ADD COLUMN expires_at DATETIME;
ALTER TABLE practice_sessions ADD COLUMN updated_at DATETIME;

UPDATE practice_sessions
SET bank_ids_json = CASE
    WHEN bank_id IS NULL THEN '[]'
    ELSE '[' || bank_id || ']'
END
WHERE bank_ids_json IS NULL OR bank_ids_json = '';

UPDATE practice_sessions
SET updated_at = COALESCE(created_at, datetime('now'))
WHERE updated_at IS NULL OR updated_at = '';

CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON practice_sessions(status, expires_at);
