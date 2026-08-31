# 05. 大型 PDF 导入与解析方案

## 1. 目标

支持：

- 100MB+；
- 500MB+；
- 上千页；
- 文本 PDF；
- 扫描 PDF；
- 跨页题目；
- 混排解析；
- 服务中断恢复。

---

## 2. 上传流程

推荐：

```text
POST /uploads/init
POST /uploads/:id/chunk
POST /uploads/:id/complete
```

Chunk 默认：

```text
5 MB
```

支持：

- SHA-256；
- 分片序号；
- 重传；
- 合并校验。

---

## 3. 文件保存

```text
data/uploads/{upload_id}/chunks/
data/originals/{document_uuid}.pdf
```

客户端文件名仅作为显示字段。

禁止直接作为实际路径。

---

## 4. Import Job

上传完成后创建：

```text
import_job
```

保存：

```text
document_id
status
total_pages
processed_pages
candidate_count
parsed_count
failed_count
new_count
duplicate_count
conflict_count
```

---

## 5. 文本提取策略

优先：

```text
PDF Text Layer
```

如果页面文字密度过低：

```text
OCR_REQUIRED
```

OCR 第一版可以是可选外部 Provider。

---

## 6. 内存控制

禁止：

```go
os.ReadFile("500MB.pdf")
```

禁止整本 PDF 文本一次保存在内存。

必须：

- Streaming；
- 分页；
- 临时文件；
- Worker 并发限制。

---

## 7. 文本清洗

处理：

- 页眉；
- 页脚；
- 页码；
- 重复标题；
- 连字符；
- 无意义空格；
- 控制字符；
- PDF 乱码；
- 连续重复段落。

---

## 8. 题目边界检测

第一层使用规则：

```regex
^\s*\d+[\.\、]
^\s*第\s*\d+\s*题
^\s*[（(]\d+[）)]
^\s*答案[:：]
^\s*解析[:：]
```

第二层 LLM 只处理边界不明确内容。

---

## 9. 跨页处理

不要：

```text
Page = Chunk
```

采用文本流 Candidate Builder：

```text
识别题目开始
→ 累积内容
→ 识别下一题开始
→ Close Candidate
```

支持：

```text
start_page
end_page
```

---

## 10. Candidate Question

保存：

```text
candidate_questions

id
document_id
page_start
page_end
raw_text
normalized_text
input_hash
status
```

---

## 11. LLM Extraction

每次输入一个或少量 Candidate。

禁止一次输入整本 PDF。

返回严格 Structured JSON。

---

## 12. 自适应模型升级

```text
Cheap Model
↓
confidence >= threshold
├── 是 → Validator
└── 否 → Strong Model
```

---

## 13. 任务恢复

服务重启后：

```text
running
```

且 lease 超时的 Job：

```text
requeue
```

Candidate 已成功阶段不重跑。

---

## 14. 进度推送

SSE：

```text
event: import_progress
data: {
  "stage": "extracting",
  "progress": 0.63,
  "processed_pages": 630,
  "total_pages": 1000
}
```

---

## 15. 并发建议

低配置服务器：

```text
PDF Parser Workers = 1
LLM Extraction Workers = 2
Validation Workers = 2
Embedding Workers = 1~2
```

必须通过配置控制。

---

## 16. 临时文件清理

Job 完成后清理：

- 上传分片；
- OCR 图片；
- 中间缓存。

保留：

- 原始 PDF；
- 结构化原文；
- Source Question；
- Pipeline Audit。
