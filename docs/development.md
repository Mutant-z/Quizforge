# QuizTrace 开发指南

## 环境要求

- Go 1.24+
- Node 20+ / pnpm 11
- SQLite（现代 C 标准库自带）
- Poppler（视觉 PDF 导入使用 `pdftoppm`；macOS：`brew install poppler`）

## 常用命令

```bash
# 后端
cd backend
go build ./...        # 编译
go vet ./...          # 静态检查
go test ./...         # 测试
go run ./cmd/server   # 启动（:8080）

# 前端
cd frontend
pnpm dev              # 开发（:5173，代理 /api → :8080）
pnpm typecheck        # 类型检查
pnpm lint             # ESLint
pnpm build            # 构建
```

## 新增数据库迁移

1. 在 `backend/migrations/` 新建 `NNNN_name.sql`
2. 重启服务自动应用（schema_migrations 记录已执行）
3. 严禁手改线上 Schema

## 新增 API

1. Domain 模型（`internal/domain/`）
2. Repository（`internal/repository/sqlite/`）
3. Service 业务层（`internal/service/`）
4. Handler（`internal/api/handler/`）
5. 路由注册（`internal/api/router/`）

## 新增 Agent 工具

1. `internal/service/agent.go` 的 `executeTool` 增加 case
2. `agentSystemPrompt` 补充工具说明
3. `toolStartMessage` / `toolDoneMessage` 增加用户可见状态文案

## 测试规范

- 单元测试：judge / review / parser / security（纯逻辑，无 DB）
- 集成测试：service 包（临时 SQLite 文件，迁移后测试）
- 前端：typecheck + lint + build 必须全绿
- 修改 Prompt 或 Extractor 后运行 `go test ./internal/...`
