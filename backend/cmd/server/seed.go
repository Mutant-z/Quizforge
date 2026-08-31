package main

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

// seedIfEmpty 首次启动注入演示数据（用户 + Java 题库）。
func seedIfEmpty(ctx context.Context, repo *sqlite.Repository, logger *slog.Logger) error {
	var userCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	hash, err := security.HashPassword("admin123")
	if err != nil {
		return err
	}
	admin, err := repo.CreateUser(ctx, "admin", "admin@quiztrace.local", hash)
	if err != nil {
		return err
	}
	// 提升为管理员
	if _, err := repo.DB().ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = ?`, admin.ID); err != nil {
		return err
	}

		// 题库
		bank, err := repo.CreateBank(ctx, "计算机基础", "Java / 数据库 / 操作系统 / 计算机网络 / RAG", "public", admin.ID)
	if err != nil {
		return err
	}
	java, err := repo.CreateSubject(ctx, bank.ID, "Java")
	if err != nil {
		return err
	}
	// 章节
	jvm, err := repo.CreateChapter(ctx, java.ID, nil, "JVM", 1, 0)
	if err != nil {
		return err
	}
	memory, err := repo.CreateChapter(ctx, java.ID, &jvm.ID, "内存结构", 2, 0)
	if err != nil {
		return err
	}
	gc, err := repo.CreateChapter(ctx, java.ID, &jvm.ID, "垃圾回收", 2, 1)
	if err != nil {
		return err
	}
	concurrent, err := repo.CreateChapter(ctx, java.ID, nil, "并发编程", 1, 1)
	if err != nil {
		return err
	}

	// 示例题目
	seedQuestions := []*domain.Question{
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &jvm.ID,
			Type: domain.QuestionTypeSingleChoice,
			Stem: "JVM 中负责运行时数据区内存分配与回收的是？",
			Options: []domain.QuestionOption{
				{Key: "A", Content: "类加载器"}, {Key: "B", Content: "执行引擎"},
				{Key: "C", Content: "垃圾收集器"}, {Key: "D", Content: "本地方法接口"},
			},
			Answer: []string{"C"}, OriginalAnalysis: "垃圾收集器（GC）负责堆内存的自动回收。",
			Difficulty: 2, KnowledgePoints: []string{"JVM", "垃圾回收"}, QualityScore: 0.99,
		},
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &memory.ID,
			Type: domain.QuestionTypeSingleChoice,
			Stem: "以下哪个区域在 Java 8 之后被移除？",
			Options: []domain.QuestionOption{
				{Key: "A", Content: "堆"}, {Key: "B", Content: "方法区"},
				{Key: "C", Content: "永久代"}, {Key: "D", Content: "虚拟机栈"},
			},
			Answer: []string{"C"}, OriginalAnalysis: "Java 8 用元空间（Metaspace）取代永久代。",
			Difficulty: 3, KnowledgePoints: []string{"JVM", "内存结构"}, QualityScore: 0.99,
		},
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &gc.ID,
			Type: domain.QuestionTypeMultipleChoice,
			Stem: "下列属于垃圾回收算法的是？（多选）",
			Options: []domain.QuestionOption{
				{Key: "A", Content: "标记-清除"}, {Key: "B", Content: "复制算法"},
				{Key: "C", Content: "标记-整理"}, {Key: "D", Content: "LRU 缓存"},
			},
			Answer: []string{"A", "B", "C"}, OriginalAnalysis: "LRU 是缓存淘汰策略，不是 GC 算法。",
			Difficulty: 3, KnowledgePoints: []string{"垃圾回收"}, QualityScore: 0.99,
		},
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &concurrent.ID,
			Type: domain.QuestionTypeTrueFalse,
			Stem: "volatile 关键字可以保证原子性。",
			Options: []domain.QuestionOption{
				{Key: "A", Content: "正确"}, {Key: "B", Content: "错误"},
			},
			Answer: []string{"B"}, OriginalAnalysis: "volatile 保证可见性与有序性，但不保证原子性。",
			Difficulty: 3, KnowledgePoints: []string{"并发", "volatile"}, QualityScore: 0.99,
		},
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &concurrent.ID,
			Type: domain.QuestionTypeFillBlank,
			Stem: "Java 中 synchronized 关键字基于________实现（填入一种锁机制）。",
			Answer: []string{"Monitor", "监视器锁", "monitor"},
			OriginalAnalysis: "synchronized 依赖 Monitor（监视器锁）实现。",
			Difficulty: 3, KnowledgePoints: []string{"并发", "synchronized"}, QualityScore: 0.99,
		},
		{
			BankID: bank.ID, SubjectID: &java.ID, ChapterID: &concurrent.ID,
			Type: domain.QuestionTypeShortAnswer,
			Stem: "简述 ThreadLocal 的原理及其内存泄漏风险。",
			Answer: []string{"ThreadLocal 为每个线程维护独立的变量副本；通过 ThreadLocalMap 存储。内存泄漏风险：ThreadLocalMap 的 Entry 继承 WeakReference，key 可能被回收而 value 仍被强引用。使用后应调用 remove() 清理。"},
			OriginalAnalysis: "ThreadLocal 以线程为键隔离变量，注意 remove 防止泄漏。",
			Difficulty: 4, KnowledgePoints: []string{"并发", "ThreadLocal"}, QualityScore: 0.99,
		},
	}

	for _, q := range seedQuestions {
		q.Status = domain.QuestionStatusPublished
		if _, err := repo.CreateQuestion(ctx, q); err != nil {
			logger.Error("seed question failed", "err", err)
			continue
		}
		_ = repo.BuildQuestionFTS(ctx, q)
	}

	// 用户示例
	userHash, _ := security.HashPassword("user123")
	if _, err := repo.CreateUser(ctx, "demo", "demo@quiztrace.local", userHash); err != nil {
		return err
	}

	logger.Info("seeded demo data", "bank_id", bank.ID, "admin", admin.Username)
	return nil
}

var _ = sql.ErrNoRows
