# 08. 错题本与智能复习设计

## 1. 错题加入

答错：

```text
不存在 → 创建
已存在 → wrong_count + 1
```

手动添加也允许。

---

## 2. 数据字段

```text
wrong_count
correct_count
review_count
mastery_score
interval_days
difficulty_factor
priority_score
last_wrong_at
last_review_at
next_review_at
status
```

---

## 3. ReviewScheduler 接口

必须可插拔：

```go
type ReviewScheduler interface {
    Calculate(ctx context.Context, input ReviewInput) (ReviewResult, error)
}
```

未来支持：

- 简单艾宾浩斯；
- SM-2；
- FSRS；
- 自定义。

---

## 4. 默认 MVP 策略

错误后：

```text
10 分钟
```

连续正确：

```text
1 天
3 天
7 天
15 天
30 天
```

再次错误：

- 降低 mastery；
- 减少 interval；
- 提高 priority。

---

## 5. 优先级

可配置：

```text
priority =
wrong_weight * wrong_score
+
overdue_weight * overdue_score
+
difficulty_weight * difficulty
+
forgetting_weight * forgetting
-
mastery_weight * mastery
```

参数不能硬编码。

---

## 6. 笔记

```text
question_notes
```

一用户一题一份主笔记。

后期可以支持历史版本。

---

## 7. 错题复习入口

提供：

```text
今日待复习
全部错题
高频错题
按科目
按章节
已掌握
```

---

## 8. AI 错因分析

输入：

- 最近错题；
- 用户答案；
- 正确答案；
- 知识点；
- 时间；
- 错误次数。

输出：

- 概念混淆；
- 记忆错误；
- 粗心；
- 边界条件；
- 理解不足。

AI 错因属于辅助判断，不直接修改题目正确答案。
