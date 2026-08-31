# QuizTrace API 文档

Base: `/api/v1` · 统一响应 `{data, error, request_id}` · 统一分页 `{items, total, page, page_size}`

## 认证

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /auth/register | 注册 {username, email, password} |
| POST | /auth/login | 登录 {account(用户名/邮箱), password} → access_token + refresh_token |
| POST | /auth/refresh | 刷新 {refresh_token} |
| GET | /users/me | 当前用户（需 Bearer） |
| PUT | /users/me | 更新资料 |
| PUT | /users/me/settings | 学习偏好 |

## 题库

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | /question-banks | 列表 / 创建（学习者仅返回本人创建的题库；管理员可查看全部） |
| GET | /question-banks/:id | 详情（含题目数，按角色校验题库归属） |
| GET/POST | /question-banks/:id/subjects | 科目列表 / 创建 |
| GET | /subjects/:subject_id/chapters | 章节树 |
| POST | /subjects/:subject_id/chapters | 创建章节 {name, parent_id?, level} |
| PUT/DELETE | /chapters/:id | 更新 / 删除章节 |
| GET | /chapters/:id/questions | 章节题目（分页） |

## 题目

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /questions/search?q=&bank_id=&subject_id=&chapter_id=&type= | 搜索 |
| GET | /questions/:id | 详情 |
| POST | /questions/:id/favorite | 收藏/取消 |
| GET/PUT/DELETE | /questions/:id/note | 笔记（Markdown） |

## 刷题

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /practice/preview | 预览当前配置的可用题数，不创建会话 |
| POST | /practice/sessions | 创建刷题会话，支持 `bank_ids[]`、题型、题数和倒计时 |
| GET | /practice/sessions/:id | 会话详情（读取时同步检查超时） |
| GET | /practice/sessions/:id/current-question | 当前题 |
| POST | /practice/sessions/:id/answer | 判题 {question_id, answer[], duration_ms} → 即时反馈 |
| POST | /practice/sessions/:id/next | 下一题 |
| POST | /practice/sessions/:id/finish | 结束 |
| POST | /practice/answer | 无会话判题（错题复习等） |

创建会话请求：

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

`bank_id` 仍兼容旧客户端，单题库请求会自动转换为 `bank_ids`。`mode` 支持
`sequential`、`random`、`unattempted`、`wrong`、`favorite`、`due`；`limit` 为
`1–500`（传 `0` 表示按最多 500 题处理）；`time_limit_seconds` 为 `null` 表示不限时，
也可以传入自定义秒数，服务端限制为 `1–86400` 秒。

会话返回 `bank_ids`、`time_limit_seconds`、`expires_at` 和 `status`。状态为
`active`、`finished` 或 `expired`；截止时间由服务端创建时计算，刷新客户端不会延长会话。

## 错题 / 复习

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /wrong-questions?subject_id=&chapter_id=&status=&due=&sort= | 错题列表 |
| GET | /wrong-questions/due | 待复习 |
| GET | /wrong-questions/summary | 统计 {total, due, mastered} |
| POST | /wrong-questions/:id/review-result | 复习 {result: again/hard/good/easy} |
| GET | /wrong-questions/questions/:id/add | 手动加入错题 |
| DELETE | /wrong-questions/questions/:id | 移除错题 |
| GET/PUT | /reviews/config | 复习调度配置 |

## 上传 / 导入

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /uploads/init | 初始化 {filename, size, mime_type, bank_id?} → upload_id + chunk_size |
| POST | /uploads/:id/chunks/:index | 上传分片（multipart chunk） |
| POST | /uploads/:id/complete | 合并 → document + import_job + 入队（同 hash 返回 409） |
| GET | /imports | 任务列表 |
| GET | /imports/:id | 详情 + 日志 |
| GET | /imports/:id/events | SSE 实时进度 |

## RAG / 搜索

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /rag/search | 混合检索 {query, bank_id?, subject_id?, chapter_id?, use_vector, top_k} |
| GET | /search/questions?q= | 题目搜索 |

## Agent

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /agent/sessions | 新建 {title, context{question_id, session_id}} |
| GET | /agent/sessions | 列表 |
| GET/DELETE | /agent/sessions/:id | 详情（含消息）/ 删除 |
| POST | /agent/sessions/:id/messages | 发消息 → SSE 流式（agent.started / tool.* / agent.streaming / agent.completed） |

## 统计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /statistics/overview | 今日/累计/正确率/错题/待复习/连续天数 |
| GET | /statistics/trend?days=30 | 每日做题趋势 |
| GET | /statistics/chapter-mastery | 章节掌握度 |
| GET | /statistics/subject-mastery | 科目掌握度 |
| GET | /statistics/wrong-trend?days=30 | 错题趋势 |
| GET | /statistics/weak-knowledge-points | 薄弱知识点 |

## 管理端（需 admin 角色）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /admin/questions | 题目管理（分页 + 筛选） |
| POST/PUT/DELETE | /admin/questions(/:id) | 题目 CRUD |
| POST | /admin/questions/:id/reindex | 重新索引 |
| GET | /admin/candidates?status= | 候选题列表 |
| GET | /admin/candidates/:id | 候选题详情（原文 + 提取结果） |
| POST | /admin/candidates/:id/approve | 审核通过 |
| POST | /admin/candidates/:id/reject | 拒绝 |
| GET | /admin/conflicts?status= | 冲突列表 |
| POST | /admin/conflicts/:id/resolve | 解决冲突 |
| GET | /admin/jobs?status= | 任务队列 |
| GET/POST/PUT/DELETE | /providers(/:id) | 模型 Provider 管理 |
| POST | /providers/test | 测试未保存的 Provider 连接（请求体包含配置与 API Key） |
| POST | /providers/:id/test | 测试已保存的 Provider 连接（服务端按当前用户读取密钥） |

## 错误码

`INVALID_REQUEST` / `UNAUTHORIZED` / `NOT_FOUND` / `CONFLICT` / `RATE_LIMITED` / `MODEL_ERROR` / `IMPORT_FAILED` / `PIPELINE_FAILED` / `VALIDATION_FAILED` / `INTERNAL_ERROR`

## 分页

`?page=1&page_size=50`（page_size ≤ 100），响应 `{items, total, page, page_size}`。
