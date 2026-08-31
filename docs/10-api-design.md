# 10. API 设计

## 1. 基础

```text
/api/v1
```

统一 JSON：

```json
{
  "data": {},
  "error": null,
  "request_id": "..."
}
```

---

# 2. Auth

```http
POST /auth/register
POST /auth/login
POST /auth/refresh
GET  /users/me
```

---

# 3. Question Bank

```http
GET /question-banks
GET /question-banks/:id
GET /question-banks/:id/subjects
GET /subjects/:id/chapters
GET /chapters/:id/questions
```

---

# 4. Practice

```http
POST /practice/preview
POST /practice/sessions
GET  /practice/sessions/:id
GET  /practice/sessions/:id/current-question
POST /practice/sessions/:id/answer
POST /practice/sessions/:id/next
POST /practice/sessions/:id/finish
```

`POST /practice/preview` 与创建会话使用相同的题库、科目、章节、题型和模式筛选，
仅返回 `{available_count, selected_count}`，不写入会话。

创建请求示例：

```json
{
  "bank_ids": [1, 2],
  "subject_id": null,
  "chapter_id": null,
  "mode": "random",
  "type": "",
  "limit": 20,
  "time_limit_seconds": 1800
}
```

旧版 `bank_id` 仍可使用。多题库会合并题目后统一抽题；多题库请求不能携带科目或章节。
`time_limit_seconds` 为 `null` 表示不限时，自定义时长可传入 `1–86400` 秒，前端按分钟输入后换算。
会话状态为 `active`、`finished` 或 `expired`，`expires_at` 由服务端固定。

Answer Request：

```json
{
  "question_id": 1001,
  "answer": ["B"],
  "duration_ms": 12500
}
```

---

# 5. Wrong Book

```http
GET    /wrong-questions
GET    /wrong-questions/due
POST   /questions/:id/wrong-book
DELETE /questions/:id/wrong-book
POST   /wrong-questions/:id/review-result
```

---

# 6. Notes

```http
GET /questions/:id/note
PUT /questions/:id/note
```

---

# 7. Upload

```http
POST /uploads/init
PUT  /uploads/:id/chunks/:index
POST /uploads/:id/complete
```

---

# 8. Import

```http
POST /imports
GET  /imports
GET  /imports/:id
GET  /imports/:id/events
POST /imports/:id/cancel
POST /imports/:id/retry
```

---

# 9. Candidate Review

```http
GET  /admin/candidates
GET  /admin/candidates/:id
POST /admin/candidates/:id/approve
POST /admin/candidates/:id/retry
POST /admin/candidates/:id/reject
```

---

# 10. Conflict

```http
GET  /admin/conflicts
GET  /admin/conflicts/:id
POST /admin/conflicts/:id/resolve
```

---

# 11. Question Admin

```http
POST   /admin/questions
PUT    /admin/questions/:id
DELETE /admin/questions/:id
POST   /admin/questions/:id/reindex
```

---

# 12. Agent

```http
POST /agent/sessions
GET  /agent/sessions/:id
POST /agent/sessions/:id/messages
GET  /agent/sessions/:id/stream
```

---

# 13. Search

```http
GET /search/questions?q=
POST /rag/search
```

---

# 14. Providers

```http
GET    /providers
POST   /providers
PUT    /providers/:id
DELETE /providers/:id
POST   /providers/:id/test
```

---

# 15. Error Codes

统一：

```text
INVALID_REQUEST
UNAUTHORIZED
NOT_FOUND
CONFLICT
RATE_LIMITED
MODEL_ERROR
IMPORT_FAILED
PIPELINE_FAILED
VALIDATION_FAILED
```

---

# 16. Pagination

```text
?page=1&page_size=50
```

最大：

```text
page_size <= 100
```

严禁一次返回整个大型题库。
