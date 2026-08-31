-- 0006_user_scoped_providers.sql
-- 支持每个用户独立配置属于自己的 AI 模型（LLM / Embedding / Rerank），并与系统默认配置隔离

ALTER TABLE ai_providers ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;

-- 索引以加速用户级别按类型及默认状态查询
CREATE INDEX IF NOT EXISTS idx_ai_providers_user_type_default ON ai_providers(user_id, provider_type, is_default);
