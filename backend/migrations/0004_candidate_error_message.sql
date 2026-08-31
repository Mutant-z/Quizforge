-- 0004_candidate_error_message.sql
-- 补全 candidate_questions.error_message 列（SetCandidateError 依赖）
ALTER TABLE candidate_questions ADD COLUMN error_message TEXT;
