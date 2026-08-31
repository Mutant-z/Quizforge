# 14. 安全与模型 Provider 设计

## 1. Provider 类型

支持：

```text
OpenAI Compatible
Anthropic Compatible
```

未来：

```text
Local OpenAI Compatible
Custom Embedding
Custom Rerank
```

---

## 2. 模型角色

分开配置：

```text
Chat Model
Question Extraction Model
Validation Model
Classification Model
Conflict Model
Embedding Model
Rerank Model
Subjective Judge Model
```

---

## 3. Model Routing

可以配置：

```text
Cheap
Balanced
Strong
```

低置信度自动升级。

---

## 4. API Key

数据库不保存纯文本 Key。

使用：

```text
AES-GCM
```

加密。

Master Key 来自：

```text
环境变量
```

前端仅展示：

```text
sk-****abcd
```

---

## 5. Provider Test

保存前测试：

- Base URL；
- Key；
- Model；
- Chat；
- Embedding Dimension。

---

## 6. Prompt Injection

AI Sidebar：

- RAG 视为不可信；
- 用户输入不覆盖 System；
- 工具参数服务端二次验证。

---

## 7. File Security

限制：

```text
PDF MIME
最大文件大小
最大页数（可配置）
```

实际文件路径不可由用户控制。

---

## 8. Auth

密码：

```text
bcrypt / argon2
```

Token：

```text
Access Token
Refresh Token
```

后台接口需要角色权限。

---

## 9. Rate Limit

重点限制：

- Login；
- Agent；
- Import；
- Provider Test；
- Subjective Judge。

---

## 10. Audit

管理员高风险操作记录：

- 删除题目；
- 合并题目；
- 修改标准答案；
- 解决冲突；
- 删除来源；
- 修改知识树。
