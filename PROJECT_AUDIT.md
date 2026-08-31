# QuizTrace Project Audit (Phase 0)

审计日期：2026-08-25

## 1. 现状

| 项目 | 状态 |
| --- | --- |
| 代码（backend/frontend） | 不存在，仓库为空 |
| 设计文档 | 完整（`docs/` 17 份，含数据库/API/多Agent/RAG/部署） |
| 测试资源 | `resources/` 7 个大型扫描版 PDF（教资题本，共 ~2.3GB） |
| 技术决策 | 文档明确：Go + Gin + SQLite(WAL/FTS5) + React + SSE，单体模块化 |

## 2. 资源 PDF 分析

- 全部为扫描版（无文本层），页数 222–354，单文件 193–615 MB。
- 意味着：文本层解析返回空 → 必须走 `OCR_REQUIRED` 检测分支。
- 开发验证策略：程序化生成文本型 PDF 作为 golden dataset，跑通完整 pipeline；
  扫描版 PDF 验证"无文本页自动检测 + OCR_REQUIRED"路径。

## 3. 技术债 / 缺口

1. 无任何代码：需按 `docs/` 从零搭建。
2. 需决定 SQLite 驱动：选用 `modernc.org/sqlite`（纯 Go，含 FTS5，免 CGo，跨平台构建简单）。
3. 向量检索：MVP 采用接口隔离 + BLOB 存储 + 余弦相似度（万级题目量级足够），
   保留替换为 sqlite-vec / Qdrant 的接口。
4. 无测试环境：需要新建 `tests/golden/` 标准数据集。

## 4. 实施顺序（对应 TODO.md）

Phase 1 基础工程 → 2 用户/题库 → 3 刷题 → 4 错题复习 → 5 上传 → 6 解析 → 7 多 Agent
→ 8 RAG → 9 AI Agent → 10 统计 → 11 前端 → 12 测试 → 13 验收。
