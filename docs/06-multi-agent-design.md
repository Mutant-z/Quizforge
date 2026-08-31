# 06. 多 Agent 设计

## 1. 原则

使用：

> Orchestrated Multi-Agent

而不是让 Agent 自由聊天。

所有 Agent：

- 输入明确；
- 输出结构化；
- 由 Orchestrator 控制；
- 最大步数受限；
- 能追踪模型与 Prompt。

---

## 2. Agent 列表

### Document Agent

职责：

- 判断文本型 / 扫描型；
- 辅助清洗；
- 识别结构异常。

### Structure Agent

职责：

- 题目边界；
- 跨页拼接；
- Candidate 切分。

### Extractor Agent

职责：

- 题型；
- 题干；
- 选项；
- 答案；
- 解析。

### Validator Agent

职责：

- 与原文对照；
- 答案匹配；
- 选项完整；
- 幻觉检测。

### Dedup / Merge Agent

职责：

- 疑似重复；
- 内容补全；
- 不覆盖冲突。

### Classification Agent

职责：

- 科目；
- 章节；
- 子章节；
- 知识点；
- 难度。

### Taxonomy Agent

职责：

- 章节标准化；
- 新章节建议；
- 同义目录合并。

### Conflict / Quality Agent

职责：

- 多来源冲突；
- 最终质量评分；
- 审核分流。

---

## 3. Adaptive Verification

不是所有题都双 Agent。

流程：

```text
Extractor A
↓
confidence >= 0.95
├── 是 → Validator
└── 否 → Extractor B / Strong Model
```

只有低置信度和规则冲突才升级。

---

## 4. Agent Context

每次调用必须包含：

```text
task_id
document_id
candidate_id
source_pages
raw_text
allowed_taxonomy
existing_question_candidates
schema_version
prompt_version
```

---

## 5. Prompt Version

Prompts 存储：

```text
prompts/
├── extractor/
│   ├── v1.md
│   └── v2.md
├── validator/
├── classifier/
└── conflict/
```

---

## 6. Tool Design

AI Sidebar Agent 可调用：

```text
get_current_question
search_rag
search_questions
get_question_analysis
get_wrong_history
get_user_note
search_similar_questions
get_chapter_statistics
```

题库构建 Agent 不建议使用开放式任意工具。

---

## 7. Agent Step Limit

建议：

```text
max_steps = 8
```

防止：

- 循环；
- Token 失控；
- Provider 成本失控。

---

## 8. 可观测性

记录：

```text
agent_name
model
provider
prompt_version
latency
input_tokens
output_tokens
retry_count
result_status
```

---

## 9. 用户可见状态

可以展示：

```text
正在读取当前题目
正在查询相关资料
找到 8 条相关内容
正在分析你的错题历史
正在生成回答
```

不要展示模型私有思维链。
