# QuizTrace 架构文档

## 1. 总体架构

Modular Monolith + Async Job Pipeline（docs/03 设计落地）：

```text
React Web (Vite) ──HTTP──▶ Go API (Gin)
                              │
        ┌─────────────────────┼──────────────────────┐
        ▼                     ▼                      ▼
  Auth/User           Question/Practice         Import/Agent/RAG
        │                     │                      │
        ▼                     ▼                      ▼
  ┌──────────────────────────────────────────────────────┐
  │                    SQLite (WAL + FTS5)               │
  │  users/banks/subjects/chapters/questions/attempts/   │
  │  wrong/notes/imports/candidates/jobs/rag/agent       │
  └──────────────────────────────────────────────────────┘
        ▲
        │
  Background Worker (SQLite Job Queue)
  ├── import_pdf    : 解析 → candidate → AI 提取 → 入库
  ├── embedding     : RAG 向量化
  └── reindex_question : 单题重建索引
```

## 2. 分层

```text
Handler (参数/鉴权/响应)
  ↓
Service (事务/业务规则)
  ↓
Repository Interface → SQLite Implementation
  ↓
Domain (领域模型)
```

AI 通过接口隔离：`LLMProvider` / `EmbeddingProvider` / `RerankProvider` / `ReviewScheduler`。

## 3. 关键设计决策

| 决策 | 说明 |
| --- | --- |
| SQLite 而非 PostgreSQL | 单机部署优先，FTS5 trigram 支持中文检索，向量 BLOB 存储 |
| 本地判题 | 客观题（单选/多选/判断/填空）确定性本地判题，不依赖 LLM |
| 幂等导入 | file_hash 唯一 + candidate input_hash 唯一 + pipeline node input_hash 幂等 |
| 不伪造 AI | 未配置 Provider 时 candidate 标记 MODEL_ERROR 进审核队列 |
| SSE 而非 WebSocket | 统一用于导入进度 / Agent 流式 / 工具状态 |
| Key 加密 | AI Provider API Key 用 AES-GCM（Master Key 来自环境变量）加密存储 |
