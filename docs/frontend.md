# QuizTrace 前端文档

## 1. 技术栈

React 18 · TypeScript (strict) · Vite 6 · Tailwind CSS · Zustand（UI 状态）· React Router 6 · Axios · ECharts · react-markdown

## 2. 页面

| 路由 | 说明 |
| --- | --- |
| /login /register | 认证 |
| / | Dashboard（今日学习/待复习/薄弱点/最近题库） |
| /question-banks | 题库列表 |
| /question-bank/:id | 题库详情（科目 → 章节树 → 刷题配置） |
| /practice/setup | 多题库刷题设置（搜索、多选、题型、题数、模式、倒计时） |
| /practice/:sessionId | 刷题（即时判题 + AI 侧栏） |
| /wrong-book | 错题本（筛选/复习/笔记） |
| /statistics | 学习统计（ECharts） |
| /admin/imports | PDF 导入（拖拽 + 分片上传 + SSE 进度） |
| /admin/candidates | 候选题审核 |
| /admin/questions | 题目管理 |
| /settings | 模型 Provider 配置 + 个人资料 |

## 3. 关键实现

- **Axios 拦截器**：自动附加 Bearer token，401 时用 refresh token 静默刷新
- **Zustand**：auth（token/user）、ui（侧栏/深色模式/AI 面板）
- **AI 侧栏**：SSE fetch 流式解析，工具状态事件渲染（agent.started / tool.started / agent.streaming）
- **刷题配置**：单题库入口打开快捷配置；多题库或高级配置进入 `/practice/setup`，预览接口实时显示可用题数；时间支持预设和 1–1440 分钟自定义
- **刷题会话**：服务端保存题目快照和 `expires_at`，前端显示倒计时，剩余 60 秒进入警告态，超时保留已完成结果
- **刷题快捷键**：1-6 / A-F 选择，Enter 提交，← → 切题
- **深色/浅色**：CSS 变量 + `class` 切换，跟随系统默认

## 4. 质量

```bash
pnpm typecheck   # tsc --noEmit
pnpm lint        # eslint（react-hooks 等）
pnpm build       # tsc -b && vite build
```

## 5. 目录

```text
src/
├── api/client.ts       # Axios 封装
├── components/         # Layout / AiSidebar / ui
├── pages/              # 业务页面 + admin/
├── store/              # Zustand
└── types/              # API 类型（与后端对齐）
```
