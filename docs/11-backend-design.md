# 11. Go 后端开发文档

## 1. 推荐结构

```text
backend/
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── api/
│   │   ├── handler/
│   │   ├── middleware/
│   │   └── router/
│   ├── domain/
│   ├── service/
│   ├── repository/
│   │   └── sqlite/
│   ├── ai/
│   │   ├── provider/
│   │   ├── embedding/
│   │   └── rerank/
│   ├── parser/
│   ├── pipeline/
│   ├── rag/
│   ├── worker/
│   ├── review/
│   ├── security/
│   └── observability/
├── migrations/
├── prompts/
├── data/
└── go.mod
```

---

## 2. Handler 规范

Handler 只做：

- 参数解析；
- 鉴权上下文；
- 调用 Service；
- Response Mapping。

不能在 Handler 中：

- 写 SQL；
- 调 LLM；
- 实现业务判题。

---

## 3. Service

负责：

- 事务边界；
- 业务规则；
- Domain 协调。

---

## 4. Repository

所有 SQL 集中 Repository。

必须：

- 参数化；
- Context；
- Transaction；
- 明确 Scan。

---

## 5. Context

所有 Provider 调用：

```go
ctx context.Context
```

支持：

- timeout；
- cancel；
- request tracing。

---

## 6. Worker

Worker 从 SQLite Job Queue claim Job。

需要：

```text
lease_until
worker_id
attempt
```

防止多个 Worker 重复执行。

---

## 7. SSE

统一事件结构：

```json
{
  "type": "tool_start",
  "request_id": "...",
  "timestamp": "...",
  "payload": {}
}
```

---

## 8. Provider

```go
type LLMProvider interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}
```

Embedding：

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

---

## 9. Question Judge

客观题必须本地判题。

### 单选

标准化后 equality。

### 多选

集合相等。

### 判断

统一 bool。

### 填空

MVP 可做 normalization + alternative answers。

主观题再使用 LLM Judge。

---

## 10. 日志

使用：

```go
log/slog
```

字段：

```text
request_id
user_id
module
operation
duration_ms
error_code
```

禁止打印 API Key。

---

## 11. 配置

使用：

```text
config.yaml
环境变量
```

敏感值优先环境变量。

---

## 12. Graceful Shutdown

必须关闭：

- HTTP；
- SSE；
- DB；
- Worker；
- 当前 Job lease。

---

## 13. 性能

SQLite：

```text
WAL
busy_timeout
foreign_keys=ON
```

不要建立过大的连接池。

SQLite 建议小连接数。
