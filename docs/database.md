# QuizTrace 数据库设计

SQLite（WAL / foreign_keys / busy_timeout），migration 位于 `backend/migrations/`（版本化，每文件只执行一次）。

## 核心表

| 表 | 说明 |
| --- | --- |
| users / refresh_tokens | 用户 + JWT 刷新令牌（hash 存储） |
| question_banks / subjects / chapters | 题库层级（章节树：level 1 章 / 2 节 / 3 小节） |
| canonical_questions / question_options | 规范题（多来源合并目标） |
| question_sources | 来源题（document + 页码 + 原文 + hash） |
| knowledge_points / question_knowledge_points | 知识点 |
| practice_sessions / question_attempts | 刷题会话 / 答题记录 |
| wrong_questions | 错题（错误次数/掌握度/复习调度） |
| question_notes | 错题笔记（Markdown，一用户一题一份） |
| favorites | 收藏 |
| review_records | 复习记录 |
| learning_statistics | 每日学习统计 |
| documents / document_pages | 上传文档 / 分页文本 |
| import_jobs / import_task_logs | 导入任务 / 日志 |
| candidate_questions | 候选题（AI 提取前） |
| jobs | 任务队列（lease/retry） |
| pipeline_runs / pipeline_node_runs | 流水线审计（幂等） |
| rag_documents / rag_embeddings / rag_fts | RAG 文档 / 向量 / FTS5 索引 |
| ai_providers | 模型 Provider（Key AES-GCM 加密） |
| agent_sessions / agent_messages | AI 侧栏会话 |
| review_configs | 复习调度配置 |
| quality_alerts / question_conflicts | 质量告警 / 题目冲突 |

## 关键索引

```text
canonical_questions(bank_id, subject_id, chapter_id)
question_attempts(user_id, question_id, created_at)
wrong_questions(user_id, next_review_at)
question_sources(canonical_question_id)
candidate_questions(document_id, status)
jobs(status, type)
documents(file_hash) UNIQUE  ← 幂等上传
rag_documents(source_type, content_hash) UNIQUE  ← 幂等索引
```

## FTS5

`rag_fts` 虚拟表使用 trigram tokenizer（支持中文子串检索）：

```text
content, source_type, bank_id, subject_id, chapter_id, question_id
```

## 幂等约束

- `documents.file_hash` UNIQUE：同文件重复上传 → 409
- `candidate_questions(document_id, input_hash)`：同内容候选不重复插入
- `rag_documents(source_type, content_hash)`：同内容文档不重复索引
- `pipeline_node_runs(input_hash, node_name)`：同输入同版本节点不重跑
