# 16. 测试与质量保障

## 1. 测试类型

### Unit

- Judge；
- Review Scheduler；
- Hash；
- Normalization；
- Taxonomy Mapper；
- Provider Adapter。

### Integration

- SQLite Repository；
- Upload；
- Job Queue；
- Pipeline；
- RAG。

### E2E

- 登录；
- 章节刷题；
- 答错进入错题；
- PDF 上传；
- Import 完成；
- AI Sidebar。

---

## 2. Golden Dataset

必须建立小型标准数据集：

```text
tests/golden/
```

包含：

- 单页单选；
- 跨页单选；
- 多选；
- 判断；
- 简答；
- 无解析；
- 答案在文末；
- 重复题；
- 冲突题；
- 扫描页。

每次修改 Prompt 或 Extractor 都跑。

---

## 3. Extraction Metrics

统计：

```text
question_recall
stem_accuracy
option_accuracy
answer_accuracy
analysis_accuracy
boundary_accuracy
```

不能只看“JSON 成功率”。

---

## 4. Dedup Metrics

需要：

- Precision；
- False Merge Rate。

错误合并比漏掉重复更危险。

---

## 5. RAG Evaluation

准备固定 Query：

- 当前题解释；
- 相似题；
- 章节知识点；
- 原文追溯。

评估：

- Recall@K；
- Hit Rate；
- Citation Correctness。

---

## 6. Load Test

重点：

- 100k Questions；
- 1M Attempts；
- 100MB PDF；
- 多个 SSE；
- 大量搜索。

---

## 7. Chaos / Recovery

测试：

```text
PDF 处理到 60%
↓
Kill Process
↓
Restart
```

要求：

- 不重复题；
- Job 恢复；
- Candidate 不丢；
- 已完成 Node 不重跑。

---

## 8. Security Test

- 文件路径穿越；
- 超大请求；
- SQL Injection；
- XSS；
- Prompt Injection；
- Key 泄露；
- 越权管理接口。
