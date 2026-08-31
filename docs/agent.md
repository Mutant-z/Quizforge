# QuizTrace Agent 设计

## 1. 定位

AI 侧栏 Agent 是刷题页右侧的对话助手，自动携带当前题目上下文（题干/选项/答案/解析/用户作答/错题历史），无需用户复制题目。

## 2. 工具

| 工具 | 说明 |
| --- | --- |
| get_current_question | 获取当前题目（无参数，用会话上下文） |
| get_question_analysis | 题目解析（question_id） |
| search_rag | RAG 混合检索（query, chapter_id 可选） |
| get_wrong_history | 用户错题历史（question_id 可选） |
| get_user_note | 用户笔记（question_id） |
| get_learning_statistics | 学习统计 |
| search_similar_questions | 相似题（排除当前题） |

## 3. 执行循环

```text
User
 → Agent（LLM，max_steps=8）
 → 解析工具调用（JSON）
 → 执行工具（真实数据）
 → 结果回注
 → Agent（最终回答）
 → SSE 流式输出
```

## 4. SSE 事件

```text
agent.started     "正在分析你的问题..."
tool.started      "正在读取当前题目..." / "正在检索相关资料..." / "正在检查你的错题记录..."
tool.completed    {name, result, latency_ms}
agent.streaming   {delta} 流式 token
agent.completed
agent.failed      "AI 服务暂时不可用"
```

前端只展示可公开任务状态，不暴露模型隐藏思维链。

## 5. 记忆

- **短期**：会话内最近 20 条消息
- **长期**：错题历史 / 学习统计 / 用户笔记（按需工具查询），不把全部历史塞入 Prompt

## 6. 会话

`agent_sessions`（标题 + context_json）+ `agent_messages`（角色 + 内容 + 工具调用记录），支持新建/列表/详情/删除。

## 7. Prompt 版本

`prompts/` 目录按 Agent 类型版本化（extractor v1 等），每次调用记录 model / provider / prompt_version / token 用量。
