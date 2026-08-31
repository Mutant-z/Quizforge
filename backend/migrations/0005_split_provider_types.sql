-- 0005_split_provider_types.sql
-- 支持模型按类型（llm / embedding / rerank）独立配置

ALTER TABLE ai_providers ADD COLUMN provider_type TEXT NOT NULL DEFAULT 'llm';
ALTER TABLE ai_providers ADD COLUMN model_name TEXT;

-- 将旧数据的模型名称迁移至 model_name
UPDATE ai_providers 
SET model_name = CASE 
    WHEN chat_model IS NOT NULL AND chat_model != '' THEN chat_model
    WHEN embedding_model IS NOT NULL AND embedding_model != '' THEN embedding_model
    WHEN rerank_model IS NOT NULL AND rerank_model != '' THEN rerank_model
    ELSE 'default'
END
WHERE model_name IS NULL;

-- 索引以加速按类型查询
CREATE INDEX IF NOT EXISTS idx_ai_providers_type_default ON ai_providers(provider_type, is_default);
