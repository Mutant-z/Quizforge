# 15. 部署与运维

## 1. MVP 部署

```text
ai-study-server
app.db
data/
frontend/
```

Go 可以直接提供 React Static。

---

## 2. 推荐服务器

最低：

```text
2 Core
2 GB RAM
```

推荐：

```text
2~4 Core
4 GB RAM
50 GB SSD
```

前提：

- LLM 走远程 API；
- Embedding 走远程 API；
- OCR 非本地重模型。

---

## 3. Docker

MVP 只需要一个应用容器。

不默认引入：

```text
Redis
MySQL
Qdrant
Kafka
```

---

## 4. Backup

必须备份：

```text
app.db
data/originals/
```

SQLite Backup 建议使用：

- SQLite backup API；
- 或暂停写入短时间 snapshot。

不要直接在高并发写入时随意 cp。

---

## 5. 文件清理

定时任务：

- 过期 upload chunks；
- 失败 OCR 临时图；
- 已完成 temp；
- 过期 export。

---

## 6. Database Maintenance

定期：

```text
PRAGMA optimize
ANALYZE
```

必要时：

```text
VACUUM
```

不应高频执行。

---

## 7. Observability

MVP：

- slog；
- request_id；
- import metrics；
- provider cost。

后期：

- Prometheus；
- OpenTelemetry。

---

## 8. Health

```http
GET /health/live
GET /health/ready
```

Ready 检查：

- DB；
- Migration；
- Worker。

---

## 9. Desktop

后期：

```text
React
+
Tauri 2
+
Remote Go API
```

也可以进一步：

```text
Tauri
+
Local Go Sidecar
+
Local SQLite
```

实现离线版本。
