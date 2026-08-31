# 12. 前端开发文档

## 1. 技术栈

```text
React
TypeScript
Vite
Tailwind CSS
shadcn/ui
TanStack Query
Zustand
React Router
```

---

## 2. 页面结构

```text
/login
/dashboard
/question-banks
/question-bank/:id
/practice/:sessionId
/wrong-book
/favorites
/statistics
/admin/imports
/admin/imports/:id
/admin/questions
/admin/conflicts
/settings/models
/settings/review
```

---

## 3. 桌面布局

刷题页：

```text
┌─────────────────────────────────────────────────────┐
│ 顶部导航 / 章节 / 进度                              │
├──────────────────────────────────┬──────────────────┤
│                                  │ AI Agent         │
│  Question                        │                  │
│                                  │ Conversation     │
│  Options                         │                  │
│                                  │                  │
│  Submit                          │                  │
│                                  │                  │
│  Analysis                        │                  │
├──────────────────────────────────┴──────────────────┤
│ Previous                               Next        │
└─────────────────────────────────────────────────────┘
```

---

## 4. AI Sidebar

支持：

- 收起；
- 展开；
- Resize；
- 当前题自动上下文；
- 快捷 Prompt；
- SSE Streaming；
- Tool Status。

---

## 5. 移动端

AI 改成：

```text
Floating Button
↓
Bottom Sheet
```

题目区域保持主焦点。

---

## 6. 状态管理

TanStack Query：

- Server State；
- Questions；
- Sessions；
- Import Jobs；
- Providers。

Zustand：

- UI Layout；
- Sidebar；
- Local Practice State；
- Draft Note。

---

## 7. 大型列表

题库后台需要：

- 分页；
- 虚拟滚动；
- 筛选；
- 延迟搜索。

严禁一次加载全部题目。

---

## 8. Import Page

展示：

```text
文件名
文件大小
状态
阶段
进度
页数
Candidate
新增
重复
冲突
失败
```

SSE 实时刷新。

---

## 9. Candidate Review

左右布局：

```text
左：
原始 PDF / 原文 / 页码

右：
结构化题目表单
```

管理员可以：

- 修改；
- 通过；
- 重试；
- 删除。

---

## 10. Conflict Review

对比：

```text
Source A
Source B
Canonical
```

高亮：

- 答案；
- 解析；
- 题干差异。

---

## 11. Design Principle

减少大量小卡片。

优先：

- 大块内容；
- 清晰层级；
- 低视觉噪声；
- 题目阅读体验；
- AI 辅助而不是抢占主体。

---

## 12. Tauri

前端必须避免过度依赖浏览器专属 API。

封装：

```text
FileService
StorageService
PlatformService
```

便于后期：

```text
Web
↓
Tauri 2
```
