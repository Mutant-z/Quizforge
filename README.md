# 题迹 QuizTrace

**AI 原生智能题库 · RAG 知识检索 · 错题记忆 · 自适应复习 · 大型题库自动化构建平台**

QuizTrace 是一个完整的 AI 智能刷题系统：上传大型 PDF 题库，多 Agent 自动提取题目、校验、去重、分类入库，构建 RAG 知识索引；用户按题库/科目/章节刷题，即时判题，错题自动进入错题本，基于自适应复习算法调度，配合 AI 侧栏 Agent 讲解与检索。

## 功能总览

- **题库体系**：题库 → 科目 → 章节（树形）→ 题目，支持单选/多选/判断/填空/简答/编程题
- **刷题**：顺序/随机/错题/收藏/未做/待复习模式，即时判题 + 解析 + 知识点
- **错题本**：自动记录错误次数、掌握度、复习调度（SimpleV1 可插拔算法），Markdown 个人笔记
- **大型 PDF 导入**：分片上传（断点续传 + Hash 幂等）→ 后台异步处理 → SSE 实时进度
- **多 Agent 提取**：文档解析 → 题目边界识别 → LLM 结构化提取 → 校验 → 去重/合并 → 分类 → 人工审核队列
- **RAG**：FTS5（trigram）+ Embedding 向量混合检索，可选 Rerank，Metadata 过滤
- **AI 侧栏 Agent**：基于当前题目上下文，工具调用（当前题/解析/RAG/错题/笔记/统计/相似题），SSE 流式输出，任务状态事件
- **学习统计**：今日/累计做题、正确率、错题趋势、章节掌握度、薄弱知识点、ECharts 可视化
- **管理端**：题目 CRUD、候选审核、冲突解决、任务队列、重新索引、模型 Provider 配置

## 技术栈

| 层 | 技术 |
| --- | --- |
| 前端 | React 18 · TypeScript · Vite · Tailwind CSS · Zustand · React Router · ECharts |
| 后端 | Go 1.24 · Gin · SQLAlchemy 风格分层（Handler/Service/Repository/Domain） |
| 数据库 | SQLite（WAL · FTS5 trigram）· 向量 BLOB + 余弦相似度（接口隔离，可换 sqlite-vec/Qdrant） |
| AI | OpenAI Compatible / Anthropic Compatible Provider 抽象 · AES-GCM Key 加密存储 |
| 任务 | SQLite Job Queue + 后台 Worker（Lease/重试/恢复） |
| 实时 | SSE（导入进度 / Agent 流式） |

## 快速开始

### 方式一：本地开发

```bash
# 0. PDF 视觉导入依赖（macOS）
brew install poppler

# 1. 后端（Go 1.24+）
cd backend
cp .env.example .env
go run ./cmd/server
# → http://localhost:8080  (API + /api/v1)

# 2. 前端（Node 20+ / pnpm）
cd frontend
pnpm install
pnpm dev
# → http://localhost:5173
```

视觉 PDF 导入需要 Poppler 的 `pdftoppm`。服务会自动查找 PATH、Apple Silicon Homebrew 的
`/opt/homebrew/bin/pdftoppm` 和 Intel Homebrew 的 `/usr/local/bin/pdftoppm`；其他位置可通过
`QT_PDFTOPPM_PATH` 指定。

新上传 PDF 默认使用 `vision-agent.v1.1`：Catalog Agent 将页面画像与目录识别合并为一次
视觉调用，Catalog/提取图分别使用 JPEG v2 缓存，题目与答案页窗并行执行。模型并发会根据
延迟、429 和超时自动调整；相关阈值见 `backend/.env.example`。历史 `vision-agent.v1` 任务
仍按原策略恢复，不会自动重跑或覆盖审计记录。

### 方式二：Docker

```bash
docker compose up -d
# api: http://localhost:8080
# web: http://localhost:5173
```

### 方式三：服务管理脚本

```bash
./scripts/start.sh              # 启动前后端（默认）
./scripts/start.sh backend      # 只启动后端
./scripts/start.sh frontend     # 只启动前端
./scripts/stop.sh               # 停止前后端
./scripts/restart.sh            # 重启前后端
./scripts/logs.sh               # 查看前后端最近 50 行日志
./scripts/logs.sh -f            # 实时跟踪
./scripts/logs.sh backend       # 只看后端
./scripts/logs.sh clear         # 清空日志
```

日志统一写入 `logs/backend.log` 与 `logs/frontend.log`。

## 默认账号

| 角色 | 用户名 | 密码 |
| --- | --- | --- |
| 管理员 | `admin` | `admin123` |
| 普通用户 | `demo` | `user123` |

首次启动自动 seed 演示数据（计算机基础题库 + 6 道示例题）。

## AI 配置

在「设置 → 模型 Provider」中配置（支持 OpenAI Compatible / Anthropic Compatible）：

```text
Base URL:      https://api.deepseek.com  (或其他兼容端点)
API Key:       sk-...
对话模型:       deepseek-chat
Embedding 模型: (可选) text-embedding-ada-002 等
```

- **未配置 Provider 时**：PDF 提取进入人工审核队列（不伪造 AI 结果），AI 侧栏提示配置。
- **API Key** 使用 AES-GCM 加密存储（`QT_API_KEY_MASTER_KEY`），前端仅显示掩码。
- 重复上传同一文件（相同 hash）返回 409 冲突，杜绝静默重复入库。

## 测试

```bash
# 后端
cd backend && go test ./...
# 前端
cd frontend && pnpm typecheck && pnpm lint && pnpm build

# 可选：真实视觉 Provider 冒烟测试（会产生一次模型请求并消耗 Token）
cd backend && QT_REAL_MODEL_TEST=1 \
  QT_DATABASE_PATH=/absolute/path/to/backend/data/app.db \
  QT_REAL_MODEL_USER_ID=3 \
  go test ./internal/service -run TestConfiguredVisionProviderLive -count=1 -v
```

默认 `go test ./...` 只运行确定性单元测试，不会调用用户配置的模型，也不会产生外部请求或费用。
只有显式设置 `QT_REAL_MODEL_TEST=1` 才会读取当前用户的默认视觉 Provider，并用一张页面图片验证真实的多模态请求。

## 目录结构

```text
backend/
├── cmd/server/          # 入口 + Worker + Seed
├── internal/
│   ├── ai/              # Provider / Embedding / Rerank 抽象
│   ├── api/             # Handler / Middleware / Router / 统一响应
│   ├── config/          # 环境变量配置
│   ├── database/        # SQLite + Migration
│   ├── domain/          # 领域模型
│   ├── judge/           # 本地确定性判题
│   ├── parser/          # PDF/文本解析 + 候选题构建
│   ├── repository/sqlite/ # 数据访问层
│   ├── review/          # 复习调度（可插拔）
│   ├── security/        # JWT / bcrypt / AES-GCM
│   ├── service/         # 业务层（Auth/Question/Practice/Import/Agent/RAG/Stat）
│   └── observability/   # 结构化日志
├── migrations/          # SQLite 迁移
└── prompts/             # Agent Prompt 版本（extractor v1）
frontend/
└── src/
    ├── api/             # Axios 封装（token 刷新）
    ├── components/      # 布局 / AI 侧栏 / UI
    ├── pages/           # 登录/题库/刷题/错题/统计/管理/设置
    ├── store/           # Zustand
    └── types/           # API 类型
docs/                    # 设计文档（原始）
```

## 文档

- `PROJECT_AUDIT.md` — Phase 0 项目审计
- `TODO.md` — 开发任务清单
- `docs/` — 原始设计文档（架构/数据库/API/多Agent/RAG/部署）
