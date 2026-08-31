# QuizTrace Development Todo

## Phase 1 基础工程
- [x] backend 初始化 (go.mod, cmd/server)
- [x] config / logging / 统一 Response
- [x] migrations (SQLite, FTS5 trigram)
- [x] router + middleware (request_id, CORS, auth, rate limit)
- [x] health check

## Phase 2 用户 + 题库
- [x] users (register/login/refresh/JWT + refresh token 存储)
- [x] question_banks / subjects / chapters (树形)
- [x] canonical_questions / options / sources
- [x] knowledge_points
- [x] CRUD API + seed 数据

## Phase 3 刷题
- [x] practice_sessions
- [x] question_attempts
- [x] 本地判题 (单选/多选/判断/填空；主观题待 LLM)
- [x] 章节/随机/错题/收藏/未做/待复习模式

## Phase 4 错题 + 复习
- [x] wrong_questions 自动/手动加入
- [x] question_notes (Markdown)
- [x] ReviewScheduler 接口 + simple_v1 实现（可配置）
- [x] due review API

## Phase 5 上传 + 导入
- [x] 分片上传 (init/chunk/complete + hash 幂等 409)
- [x] documents / import_jobs
- [x] SQLite Job Queue + Worker (lease/重试)
- [x] SSE 进度推送

## Phase 6 PDF 解析
- [x] DocumentParser (文本层 + OCR_REQUIRED 检测)
- [x] 清洗 (页眉/页脚/页码/控制字符/重复行)
- [x] Candidate Builder (题目边界 + 跨页)

## Phase 7 多 Agent 提取
- [x] LLM Provider 抽象 (OpenAI/Anthropic compatible + AES-GCM 加密)
- [x] Extractor Agent (Structured Output + JSON Schema)
- [x] 本地规则校验 (选项/答案/题型)
- [x] Dedup (归一化 hash) / 冲突检测
- [x] 低置信度/无 Provider → 人工审核队列
- [x] Candidate Review API

## Phase 8 RAG
- [x] FTS5 trigram 索引
- [x] Embedding Provider + 向量存储 (BLOB)
- [x] Hybrid Search (FTS + vector + rerank fallback)

## Phase 9 AI Agent
- [x] agent_sessions / messages
- [x] 工具调用 (当前题/解析/RAG/错题/笔记/统计/相似题)
- [x] SSE Streaming + 工具状态事件 (agent.started/tool.*/agent.streaming)
- [x] 会话管理 (新建/列表/详情/删除)

## Phase 10 统计
- [x] 今日/累计做题、正确率
- [x] 章节掌握度、科目掌握度
- [x] 趋势数据 / 薄弱知识点

## Phase 11 前端
- [x] 工程初始化 (Vite + React + TS + Tailwind)
- [x] 认证页面 + 布局 (深色/浅色)
- [x] Dashboard
- [x] 题库/章节页 (章节树 + 刷题配置)
- [x] 刷题页 (即时判题 + 键盘快捷键 + AI 侧栏)
- [x] 错题本 + 笔记 + 复习
- [x] 统计页 (ECharts)
- [x] 管理端 (导入/审核/题目管理)
- [x] 设置 (模型 Provider 配置 + 测试连接)

## Phase 12 测试
- [x] go test 全绿 (auth/judge/review/parser/service-import-practice-rag)
- [x] 前端 typecheck / lint / build
- [x] GUI E2E 验收 (登录→刷题→判题→错题→复习→统计)

## Phase 13 文档
- [x] README.md
- [x] Docker Compose + Dockerfile
- [x] .env.example
- [x] PROJECT_AUDIT.md / TODO.md
