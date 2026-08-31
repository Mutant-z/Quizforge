# 17. 开发路线与 TODO List

## Phase 0：基础工程

- [ ] 初始化 backend
- [ ] 初始化 frontend
- [ ] 配置 lint
- [ ] 配置 migration
- [ ] 配置 config
- [ ] 配置 logging
- [ ] 定义统一 API Response

---

## Phase 1：题库基础

- [ ] users
- [ ] question_banks
- [ ] subjects
- [ ] chapters
- [ ] canonical_questions
- [ ] question_options
- [ ] question_sources
- [ ] CRUD API
- [ ] 管理端基础页面

验收：

```text
可以人工录入题目并按章节查看。
```

---

## Phase 2：刷题

- [ ] practice_sessions
- [ ] question_attempts
- [ ] 单选判题
- [ ] 多选判题
- [ ] 判断题
- [ ] 章节刷题
- [ ] 顺序
- [ ] 随机
- [ ] 未做题
- [ ] 前端刷题页面

验收：

```text
用户可以完成一整个章节刷题。
```

---

## Phase 3：错题

- [ ] wrong_questions
- [ ] 自动加入
- [ ] 手动加入
- [ ] 笔记
- [ ] due review
- [ ] ReviewScheduler Interface
- [ ] 默认调度算法

验收：

```text
答错立即进入错题，之后可以再次复习。
```

---

## Phase 4：PDF Upload

- [ ] upload init
- [ ] chunk upload
- [ ] upload complete
- [ ] hash
- [ ] document table
- [ ] import_jobs
- [ ] SSE progress

---

## Phase 5：PDF Parser

- [ ] Text PDF
- [ ] Page Extraction
- [ ] Cleaner
- [ ] Candidate Builder
- [ ] Cross-page
- [ ] Error Recovery
- [ ] Golden Dataset

---

## Phase 6：Multi-Agent Extraction

- [ ] Provider Abstraction
- [ ] Extractor Agent
- [ ] JSON Schema
- [ ] Validator Agent
- [ ] Adaptive Verification
- [ ] Prompt Version
- [ ] Token Usage

---

## Phase 7：增量题库

- [ ] exact hash
- [ ] normalized hash
- [ ] embedding dedup
- [ ] question_sources
- [ ] Canonical Merge
- [ ] Conflict Queue
- [ ] Classification Agent
- [ ] Taxonomy Agent
- [ ] Quality Score

---

## Phase 8：RAG

- [ ] FTS5
- [ ] sqlite-vec
- [ ] Embedding Provider
- [ ] Hybrid Search
- [ ] Metadata Filter
- [ ] Similar Questions
- [ ] Reindex

---

## Phase 9：AI Sidebar

- [ ] Agent Session
- [ ] SSE Chat
- [ ] get_current_question
- [ ] search_rag
- [ ] get_wrong_history
- [ ] get_user_note
- [ ] search_similar_questions
- [ ] Tool Status UI
- [ ] Quick Actions

---

## Phase 10：统计与质量系统

- [ ] 今日刷题
- [ ] 正确率
- [ ] 章节正确率
- [ ] 薄弱知识点
- [ ] Quality Alert
- [ ] User Feedback
- [ ] Question Audit

---

## Phase 11：桌面版

- [ ] Tauri 初始化
- [ ] Platform Adapter
- [ ] Remote Server Mode
- [ ] Local Mode 评估
- [ ] Windows build
- [ ] macOS build
- [ ] Linux build

---

# 最终验收标准

系统至少可以完整完成：

```text
上传大型 PDF
↓
解析为 Candidate
↓
多 Agent 提取
↓
去重 / 分类
↓
题目进入数据库
↓
创建向量索引
↓
用户按章节刷题
↓
即时判题
↓
错题本
↓
复习
↓
AI Agent 基于当前题和 RAG 讲解
```
