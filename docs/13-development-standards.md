# 13. 开发规范

## 1. 总体原则

- 模块化；
- 可测试；
- 显式依赖；
- 不做隐式全局状态；
- 业务规则不写在 UI；
- AI 结果必须结构化；
- 所有任务必须可追踪。

---

# 2. Git

分支：

```text
main
develop
feature/*
fix/*
refactor/*
```

Commit 推荐：

```text
feat:
fix:
refactor:
docs:
test:
chore:
```

---

# 3. Go

必须：

```text
gofmt
go vet
```

推荐：

```text
golangci-lint
```

错误必须 wrap：

```go
fmt.Errorf("parse candidate %d: %w", id, err)
```

禁止吞错。

---

# 4. TypeScript

开启 strict。

禁止大量：

```ts
any
```

API 类型统一放：

```text
src/types/
```

---

# 5. API

- RESTful；
- 版本前缀；
- request_id；
- 统一错误结构；
- 参数校验；
- pagination。

---

# 6. 数据库

禁止：

- 手动线上改表；
- 动态拼接 SQL；
- 无索引的大表扫描；
- 删除来源数据。

所有 Schema 变化必须 migration。

---

# 7. AI

每个 Prompt：

- 必须版本化；
- 必须有 Schema；
- 必须限制输出；
- 必须记录 Model；
- 必须有 Timeout；
- 必须有 Retry 上限。

---

# 8. Pipeline

每个 Node：

- 幂等；
- 可重试；
- 可观察；
- 有状态；
- 不依赖进程内临时状态作为唯一事实来源。

---

# 9. 文件

所有上传文件：

- UUID；
- Hash；
- MIME 检查；
- 大小限制；
- 路径隔离。

---

# 10. 文档

新增核心模块必须更新：

- Architecture；
- API；
- Migration；
- Config；
- Test。

---

# 11. Code Review Checklist

检查：

- 是否破坏幂等；
- 是否存在 N+1；
- 是否出现大文件 ReadAll；
- 是否泄露 Key；
- 是否漏 Transaction；
- 是否直接相信 LLM；
- 是否加入 Migration；
- 是否存在无限 Agent Loop。
