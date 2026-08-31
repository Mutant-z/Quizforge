-- 修复 0022 对 JSON null 的误判，并把历史“无需答案”与“答案缺失”分开。
-- answer_json 为字符串 "null" 时并不代表存在答案。
UPDATE wrong_import_drafts
SET answer_status = CASE
    WHEN json_valid(answer_json)
         AND json_type(answer_json) = 'array'
         AND json_array_length(answer_json) > 0 THEN 'provided'
    WHEN answer_source = 'none' AND user_marked_no_answer = 1 THEN 'explicit_none'
    WHEN answer_source = 'image' THEN 'unresolved'
    WHEN answer_source IN ('pending', 'none') THEN 'missing'
    ELSE 'pending'
END;
