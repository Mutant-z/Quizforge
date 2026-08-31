# QuizTrace 部署与运维

## 1. 本地开发

```bash
# 后端（Go 1.24+）
cd backend && go run ./cmd/server
# 前端（Node 20+ / pnpm 11）
cd frontend && pnpm install && pnpm dev
```

## 2. Docker

```bash
docker compose up -d
```

- `api`：Go 服务（:8080）
- `worker`：后台任务（导入/向量化）
- `web`：Nginx 托管前端（:5173，/api 反代到 api）
- `postgres/redis/qdrant`：预留基础设施（MVP 未使用，SQLite/本地文件已满足）

## 3. 环境变量

见 `backend/.env.example`。生产必须修改：

```text
QT_JWT_SECRET         # 长随机串
QT_API_KEY_MASTER_KEY # Provider Key 加密主密钥
```

## 4. 数据目录

```text
data/
├── app.db           # SQLite（WAL）
├── originals/       # 原始上传文件（备份重点）
├── uploads/         # 分片临时目录（任务完成后清理）
└── processed/       # 处理中间产物
```

## 5. 备份

- 定期备份 `app.db` + `originals/`
- SQLite 建议用 `sqlite3 app.db ".backup backup.db"`（非直接 cp，避免 WAL 不一致）

## 6. 健康检查

```text
GET /health/live    # 存活
GET /health/ready   # DB + 迁移就绪
```
