# QuizTrace PDF 导入管线

## 1. 总览

```text
Upload（分片）→ Object Storage（本地磁盘）→ Job Queue → Document Parser → Candidate Builder
  → AI Extraction（Structured JSON）→ 校验 → 去重/冲突检测 → 入库 → FTS → Embedding
```

## 2. 分片上传

- `POST /uploads/init` → `{upload_id, chunk_size(5MB), chunk_count}`
- `POST /uploads/:id/chunks/:index`（multipart）
- `POST /uploads/:id/complete` → 合并 + SHA-256 + 幂等检查（同 hash 409）+ 创建 document + import_job + 入队

## 3. 文档解析（parser 包）

- **PDF**：逐页提取文本层（`ledongthuc/pdf`），单页失败不中断；文本密度 < 20 字符标记 `NeedsOCR`（OCR_REQUIRED，不默认全量 OCR）
- **TXT/MD**：整文件单页
- 清洗：控制字符、页码行、页眉页脚重复行、多余空格

## 4. Candidate Builder

- 题目边界正则：`^\s*\d+[\.、]` / `第N题` / `（N）` / `答案:` / `解析:`
- 跨页累积：题目开始 → 累积 → 下一题开始 → 关闭候选（记录 page_start/page_end）
- 归一化 + SHA-256 hash（幂等去重）

## 5. AI 提取

- **Structured Output**：System Prompt 定义 JSON Schema，`response_format: json_object`，禁止正则解析 Markdown
- 每次只处理一个候选（严禁整本书发给 LLM）
- 无 Provider 配置 → candidate 标记 `MODEL_ERROR` 进人工审核（不伪造）
- 提取结果字段：type/stem/options/answer/analysis/difficulty/knowledge_points/confidence

## 6. 校验与入库

- 本地规则校验：题干非空、选项 ≥ 2、答案存在、题型合法
- confidence < 0.8 → `needs_review` 审核队列
- 去重：归一化题干 hash → 已存在 → 冲突检测（答案不一致 → conflict）或绑定来源（duplicate）
- 新题 → canonical_questions + question_sources + FTS 索引

## 7. 幂等与恢复

- 同文件重复上传 → 409
- candidate 按 input_hash 幂等插入
- Job lease 超时自动重新抢占；已完成节点（pipeline_node_runs）不重跑

## 8. 扫描版 PDF

`resources/` 中的测试 PDF 均为扫描版（无文本层）→ 自动标记 OCR_REQUIRED。OCR 作为可选外部 Provider 扩展点。
