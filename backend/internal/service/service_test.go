package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/parser"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

// TestAuthService 注册 + 登录 + 刷新。
func TestAuthService(t *testing.T) {
	db, _ := database.Open("/tmp/qt_auth_test_" + uuidStr() + ".db", 5000)
	defer db.Close()
	database.Migrate(context.Background(), db, "/Users/mutant/Documents/project/QuizForge/backend/migrations")
	repo := sqlite.NewRepository(db)
	tm := security.NewTokenManager("test-secret", 60, 7)
	svc := NewAuthService(repo, tm, security.NewAESCrypt("k"))

	user, err := svc.Register(context.Background(), "alice", "alice@test.com", "secret123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("wrong username: %s", user.Username)
	}
	// 重复注册
	if _, err := svc.Register(context.Background(), "alice", "b@b.com", "x12345"); err == nil {
		t.Fatal("duplicate username should fail")
	}
	// 登录
	resp, err := svc.Login(context.Background(), "alice", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("tokens should be issued")
	}
	// 错误密码
	if _, err := svc.Login(context.Background(), "alice", "wrong"); err == nil {
		t.Fatal("wrong password should fail")
	}
	// 刷新
	resp2, err := svc.Refresh(context.Background(), resp.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if resp2.AccessToken == "" {
		t.Fatal("refreshed token should exist")
	}
}

// TestPracticeFlow 完整刷题流程：题库 → 刷题 → 答错 → 错题本 → 复习。
func TestPracticeFlow(t *testing.T) {
	db, _ := database.Open("/tmp/qt_practice_test_" + uuidStr() + ".db", 5000)
	defer db.Close()
	database.Migrate(context.Background(), db, "/Users/mutant/Documents/project/QuizForge/backend/migrations")
	repo := sqlite.NewRepository(db)
	scheduler := reviewScheduler()

	hash, _ := security.HashPassword("x")
	user, err := repo.CreateUser(context.Background(), "u", "u@u.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	bank, err := repo.CreateBank(context.Background(), "b", "", "private", user.ID)
	if err != nil {
		t.Fatalf("create bank: %v", err)
	}
	sub, err := repo.CreateSubject(context.Background(), bank.ID, "Java")
	if err != nil {
		t.Fatalf("create subject: %v", err)
	}
	ch, err := repo.CreateChapter(context.Background(), sub.ID, nil, "JVM", 1, 0)
	if err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	q := &domain.Question{BankID: bank.ID, SubjectID: &sub.ID, ChapterID: &ch.ID,
		Type: domain.QuestionTypeSingleChoice, Stem: "题？",
		Options: []domain.QuestionOption{{Key: "A", Content: "a"}, {Key: "B", Content: "b"}},
		Answer:  []string{"B"}, Status: domain.QuestionStatusPublished}
	created, _ := repo.CreateQuestion(context.Background(), q)
	_ = repo.BuildQuestionFTS(context.Background(), created)

	ps := NewPracticeService(repo, scheduler)
	session, err := ps.CreateSession(context.Background(), user.ID, CreateSessionRequest{
		BankID: &bank.ID, ChapterID: &ch.ID, Mode: "sequential", Limit: 10,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.TotalCount != 1 {
		t.Fatalf("want 1 question, got %d", session.TotalCount)
	}
	// 答错
	res, err := ps.Answer(context.Background(), user.ID, &session.ID, AnswerRequest{
		QuestionID: created.ID, Answer: []string{"A"}, DurationMs: 1000,
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if res.Correct {
		t.Fatal("A should be wrong")
	}
	if !res.WrongAdded {
		t.Fatal("should add to wrong book")
	}
	// 错题本
	w, err := repo.GetWrongByQuestion(context.Background(), user.ID, created.ID)
	if err != nil {
		t.Fatalf("wrong question: %v", err)
	}
	if w.WrongCount != 1 {
		t.Fatalf("wrong count should be 1, got %d", w.WrongCount)
	}
	// 复习
	r, err := scheduler.Calculate(context.Background(), reviewInputFor(w))
	if err != nil {
		t.Fatal(err)
	}
	if r.IntervalDays <= 0 {
		t.Fatal("interval should be positive")
	}
	// 笔记
	note, err := repo.UpsertNote(context.Background(), user.ID, created.ID, "# 笔记")
	if err != nil || note.Content != "# 笔记" {
		t.Fatalf("note: %v %v", note, err)
	}
	// 统计
	stats := NewStatService(repo)
	ov, err := stats.Overview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ov["today"].(map[string]int64)["question_count"] != 1 {
		t.Fatal("today stats should have 1 question")
	}
}

// TestImportPipeline 导入流水线：解析 → candidate → 提取（无 provider 标记错误）→ 幂等。
func TestImportPipeline(t *testing.T) {
	db, _ := database.Open("/tmp/qt_import_test_" + uuidStr() + ".db", 5000)
	defer db.Close()
	database.Migrate(context.Background(), db, "/Users/mutant/Documents/project/QuizForge/backend/migrations")
	repo := sqlite.NewRepository(db)

	hash, _ := security.HashPassword("x")
		user, _ := repo.CreateUser(context.Background(), "u", "u@u.com", hash)
		bank, _ := repo.CreateBank(context.Background(), "b", "", "private", user.ID)
		doc, _ := repo.CreateDocument(context.Background(), &domain.Document{
			UserID: user.ID, BankID: &bank.ID, OriginalName: "t.txt", StoredName: "t.txt",
			FilePath: "/tmp/qt_sample.txt", FileSize: 10, FileHash: "h1", MimeType: "text/plain",
		})
		imp, _ := repo.CreateImportJob(context.Background(), user.ID, doc.ID, &bank.ID)

	// 解析
	pages, err := parser.ParseDocument(context.Background(), "/tmp/qt_sample.txt", "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	cands, err := parser.BuildCandidates(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("should have candidates")
	}
	// 幂等写入
	jobID := imp.ID
	cands[0].DocumentID = doc.ID
	cands[0].ImportJobID = &jobID
	created1, err := repo.InsertCandidateIfAbsent(context.Background(), cands[0])
	if err != nil || !created1 {
		t.Fatalf("first insert should create: %v %v", created1, err)
	}
	created2, err := repo.InsertCandidateIfAbsent(context.Background(), cands[0])
	if err != nil || created2 {
		t.Fatalf("second insert should skip: %v %v", created2, err)
	}
}

// TestRAGSearch FTS 检索。
func TestRAGSearch(t *testing.T) {
	db, _ := database.Open("/tmp/qt_rag_test_" + uuidStr() + ".db", 5000)
	defer db.Close()
	database.Migrate(context.Background(), db, "/Users/mutant/Documents/project/QuizForge/backend/migrations")
	repo := sqlite.NewRepository(db)
	hash, _ := security.HashPassword("x")
	user, _ := repo.CreateUser(context.Background(), "u", "u@u.com", hash)
	bank, _ := repo.CreateBank(context.Background(), "b", "", "private", user.ID)
	q := &domain.Question{BankID: bank.ID, Type: "single_choice", Stem: "JVM 垃圾回收机制是什么？",
		Answer: []string{"A"}, Status: "published", KnowledgePoints: []string{"JVM"}}
	created, _ := repo.CreateQuestion(context.Background(), q)
	_ = repo.BuildQuestionFTS(context.Background(), created)

	docs, err := repo.SearchFTS(context.Background(), "垃圾回收", 5, &bank.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("FTS should find JVM question")
	}
	if docs[0].QuestionID != created.ID {
		t.Fatalf("wrong doc: %d vs %d", docs[0].QuestionID, created.ID)
	}
}

func uuidStr() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
