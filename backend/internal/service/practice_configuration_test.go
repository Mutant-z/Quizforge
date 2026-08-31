package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

type practiceFixture struct {
	db    *sql.DB
	repo  *sqlite.Repository
	user  *domain.User
	banks []*domain.QuestionBank
	qs    []*domain.Question
	svc   *PracticeService
}

func newPracticeFixture(t *testing.T) *practiceFixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "practice.db"), 5000)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, "/Users/mutant/Documents/project/QuizForge/backend/migrations"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	hash, err := security.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	user, err := repo.CreateUser(ctx, "practice-user", "practice@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &practiceFixture{db: db, repo: repo, user: user, svc: NewPracticeService(repo, reviewScheduler())}
	for _, name := range []string{"基础题库", "进阶题库"} {
		bank, err := repo.CreateBank(ctx, name, "", "private", user.ID)
		if err != nil {
			t.Fatal(err)
		}
		fixture.banks = append(fixture.banks, bank)
	}
	for i, bank := range fixture.banks {
		count := 3
		if i == 1 {
			count = 2
		}
		for n := 0; n < count; n++ {
			questionType := domain.QuestionTypeSingleChoice
			if i == 1 && n == 1 {
				questionType = domain.QuestionTypeTrueFalse
			}
			question, err := repo.CreateQuestion(ctx, &domain.Question{
				BankID: bank.ID, Type: questionType, Stem: nameForPracticeQuestion(bank.ID, n),
				Options: []domain.QuestionOption{{Key: "A", Content: "正确"}, {Key: "B", Content: "错误"}},
				Answer:  []string{"A"}, Status: domain.QuestionStatusPublished,
			})
			if err != nil {
				t.Fatal(err)
			}
			fixture.qs = append(fixture.qs, question)
		}
	}
	return fixture
}

func nameForPracticeQuestion(bankID int64, index int) string {
	if bankID%2 == 0 {
		return "进阶题库题目"
	}
	return "基础题库题目"
}

func (f *practiceFixture) bankIDs() []int64 {
	return []int64{f.banks[0].ID, f.banks[1].ID}
}

func TestPracticeConfigurationMergesBanksAndAppliesFilters(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	all, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: f.bankIDs(), Mode: "sequential", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if all.AvailableCount != 5 || all.SelectedCount != 5 {
		t.Fatalf("merged preview = %+v, want 5 available and selected", all)
	}

	session, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{BankIDs: f.bankIDs(), Mode: "random", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.BankIDs) != 2 || session.TotalCount != 3 {
		t.Fatalf("merged session = %+v", session)
	}
	for _, id := range session.QuestionIDs {
		question, err := f.repo.GetQuestion(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if question.BankID != f.banks[0].ID && question.BankID != f.banks[1].ID {
			t.Fatalf("question %d escaped selected banks", id)
		}
	}

	typed, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: f.bankIDs(), Mode: "sequential", Type: string(domain.QuestionTypeTrueFalse), Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if typed.AvailableCount != 1 || typed.SelectedCount != 1 {
		t.Fatalf("type preview = %+v, want one true/false question", typed)
	}

	if err := f.repo.AddFavorite(ctx, f.user.ID, f.qs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.AddFavorite(ctx, f.user.ID, f.qs[3].ID); err != nil {
		t.Fatal(err)
	}
	favorite, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[1].ID}, Mode: "favorite", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if favorite.AvailableCount != 1 {
		t.Fatalf("favorite scope = %+v, want one question from bank 2", favorite)
	}
	duplicateBankScope, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[0].ID, f.banks[0].ID}, Mode: "sequential", Limit: 20})
	if err != nil {
		t.Fatalf("duplicate bank IDs should collapse to one bank: %v", err)
	}
	if duplicateBankScope.AvailableCount != 3 {
		t.Fatalf("deduplicated bank scope = %+v, want three questions", duplicateBankScope)
	}

	if _, err := f.repo.UpsertWrongQuestion(ctx, f.user.ID, f.qs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.UpsertWrongQuestion(ctx, f.user.ID, f.qs[3].ID); err != nil {
		t.Fatal(err)
	}
	wrong, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[0].ID}, Mode: "wrong", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if wrong.AvailableCount != 1 {
		t.Fatalf("wrong scope = %+v, want one question from bank 1", wrong)
	}

	if _, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: f.bankIDs(), SubjectID: ptr64(1), Mode: "random", Limit: 20}); err == nil {
		t.Fatal("multiple banks with subject scope should be rejected")
	}
}

func TestPracticeRejectsAnotherUsersBank(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	hash, err := security.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.repo.CreateUser(ctx, "practice-other", "practice-other@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	foreignBank, err := f.repo.CreateBank(ctx, "他人的题库", "", "private", other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{foreignBank.ID}, Limit: 10}); err == nil {
		t.Fatal("practice preview should reject another user's bank")
	}
}

func TestPracticeExcludesQuestionsWithoutAnswersOrOptions(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	bankID := f.banks[0].ID

	noAnswer, err := f.repo.CreateQuestion(ctx, &domain.Question{
		BankID: bankID, Type: domain.QuestionTypeSingleChoice, Stem: "缺少答案",
		Options: []domain.QuestionOption{{Key: "A", Content: "选项"}}, Answer: []string{}, Status: domain.QuestionStatusPublished,
	})
	if err != nil {
		t.Fatal(err)
	}
	noOptions, err := f.repo.CreateQuestion(ctx, &domain.Question{
		BankID: bankID, Type: domain.QuestionTypeSingleChoice, Stem: "缺少选项",
		Options: []domain.QuestionOption{}, Answer: []string{"A"}, Status: domain.QuestionStatusPublished,
	})
	if err != nil {
		t.Fatal(err)
	}
	blankAnswer, err := f.repo.CreateQuestion(ctx, &domain.Question{
		BankID: bankID, Type: domain.QuestionTypeFillBlank, Stem: "答案为空白",
		Answer: []string{"  "}, Status: domain.QuestionStatusPublished,
	})
	if err != nil {
		t.Fatal(err)
	}
	malformed, err := f.repo.CreateQuestion(ctx, &domain.Question{
		BankID: bankID, Type: domain.QuestionTypeSingleChoice, Stem: "答案和选项格式损坏",
		Options: []domain.QuestionOption{{Key: "A", Content: "选项"}}, Answer: []string{"A"}, Status: domain.QuestionStatusPublished,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx,
		`UPDATE canonical_questions SET answer_json = ?, options_json = ? WHERE id = ?`, "not-json", "not-json", malformed.ID,
	); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"sequential", "random", "unattempted"} {
		preview, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{
			BankIDs: []int64{bankID}, Mode: mode, Limit: 500,
		})
		if err != nil {
			t.Fatalf("mode %s preview failed: %v", mode, err)
		}
		if preview.AvailableCount != 3 {
			t.Fatalf("mode %s included incomplete questions: %+v", mode, preview)
		}
	}

	if err := f.repo.AddFavorite(ctx, f.user.ID, noAnswer.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.AddFavorite(ctx, f.user.ID, noOptions.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.AddFavorite(ctx, f.user.ID, f.qs[0].ID); err != nil {
		t.Fatal(err)
	}
	favorite, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{
		BankIDs: []int64{bankID}, Mode: "favorite", Limit: 500,
	})
	if err != nil || favorite.AvailableCount != 1 {
		t.Fatalf("favorite mode included incomplete questions: preview=%+v err=%v", favorite, err)
	}

	for _, questionID := range []int64{noAnswer.ID, noOptions.ID, blankAnswer.ID, malformed.ID, f.qs[0].ID} {
		if _, err := f.repo.UpsertWrongQuestion(ctx, f.user.ID, questionID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE wrong_questions SET next_review_at = datetime('now', '-1 minute') WHERE user_id = ?`, f.user.ID); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"wrong", "due"} {
		preview, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{
			BankIDs: []int64{bankID}, Mode: mode, Limit: 500,
		})
		if err != nil || preview.AvailableCount != 1 {
			t.Fatalf("mode %s included incomplete questions: preview=%+v err=%v", mode, preview, err)
		}
	}

	// 兼容过滤规则上线前已经创建的会话：旧快照中的不完整题目也不能展示。
	legacySession, err := f.repo.CreateSession(ctx, &domain.PracticeSession{
		UserID: f.user.ID, BankID: &bankID, BankIDs: []int64{bankID}, Mode: "sequential",
		QuestionIDs: []int64{noAnswer.ID, f.qs[0].ID}, Status: "active", StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	question, updatedSession, err := f.svc.GetCurrentQuestion(ctx, f.user.ID, legacySession.ID)
	if err != nil || question == nil || question.ID != f.qs[0].ID || updatedSession.CurrentIndex != 1 {
		t.Fatalf("legacy session exposed an incomplete question: question=%+v session=%+v err=%v", question, updatedSession, err)
	}
}

func TestPracticeUnattemptedAndSessionLifecycle(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	if _, err := f.repo.CreateAttempt(ctx, &domain.QuestionAttempt{UserID: f.user.ID, QuestionID: f.qs[0].ID, Answer: []string{"A"}, IsCorrect: true}); err != nil {
		t.Fatal(err)
	}
	unattempted, err := f.svc.Preview(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[0].ID}, Mode: "unattempted", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if unattempted.AvailableCount != 2 {
		t.Fatalf("unattempted preview = %+v, want two questions", unattempted)
	}

	session, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[0].ID}, Mode: "sequential", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{QuestionID: f.qs[2].ID, Answer: []string{"A"}}); err == nil {
		t.Fatal("answering a non-current question should be rejected")
	}
	if _, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{QuestionID: session.QuestionIDs[0], Answer: []string{"A"}}); err != nil {
		t.Fatal(err)
	}
	progress, err := f.svc.Next(ctx, f.user.ID, session.ID)
	if err != nil || progress.Status != "active" || progress.CurrentIndex != 1 {
		t.Fatalf("after first next = %+v, err=%v", progress, err)
	}
	if _, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{QuestionID: session.QuestionIDs[1], Answer: []string{"A"}}); err != nil {
		t.Fatal(err)
	}
	finished, err := f.svc.Next(ctx, f.user.ID, session.ID)
	if err != nil || finished.Status != "finished" || finished.CurrentIndex != 2 {
		t.Fatalf("after final next = %+v, err=%v", finished, err)
	}
	question, current, err := f.svc.GetCurrentQuestion(ctx, f.user.ID, session.ID)
	if err != nil || question != nil || current.Status != "finished" {
		t.Fatalf("finished current question = %v, session=%+v, err=%v", question, current, err)
	}
}

func TestPracticeAnswerCorrectsWrongQuestionWithoutPreviousReview(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	questionID := f.qs[0].ID

	// 错题可能刚刚加入错题本，还没有 last_review_at；答对时不应因
	// 复习调度读取空时间而触发 panic。
	if _, err := f.repo.UpsertWrongQuestion(ctx, f.user.ID, questionID); err != nil {
		t.Fatal(err)
	}
	session, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{
		BankIDs: []int64{f.banks[0].ID}, Mode: "sequential", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{
		QuestionID: questionID,
		Answer:     []string{"A"},
	})
	if err != nil {
		t.Fatalf("answering a newly-added wrong question should succeed: %v", err)
	}
	if !result.Correct {
		t.Fatal("expected the answer to be correct")
	}

	updated, err := f.repo.GetWrongByQuestion(ctx, f.user.ID, questionID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CorrectCount != 1 || updated.NextReviewAt == nil {
		t.Fatalf("wrong question was not rescheduled: %+v", updated)
	}

	// 重试同一提交只返回已保存结果，不应重复创建答题记录或累计统计。
	retry, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{
		QuestionID: questionID,
		Answer:     []string{"A"},
	})
	if err != nil || !retry.Correct {
		t.Fatalf("retrying an answered question should return the saved result: result=%+v err=%v", retry, err)
	}
	var attemptCount int
	if err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM question_attempts WHERE session_id = ? AND question_id = ?`, session.ID, questionID,
	).Scan(&attemptCount); err != nil {
		t.Fatal(err)
	}
	if attemptCount != 1 {
		t.Fatalf("retry created duplicate attempts: got %d, want 1", attemptCount)
	}
}

func TestPracticeSessionExpiresOnServerDeadline(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	seconds := 60
	session, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{BankIDs: []int64{f.banks[0].ID}, Mode: "sequential", Limit: 1, TimeLimitSeconds: &seconds})
	if err != nil {
		t.Fatal(err)
	}
	if session.ExpiresAt == nil || session.TimeLimitSeconds == nil || *session.TimeLimitSeconds != seconds {
		t.Fatalf("timer fields were not persisted: %+v", session)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE practice_sessions SET expires_at = datetime('now', '-1 second') WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	question, expired, err := f.svc.GetCurrentQuestion(ctx, f.user.ID, session.ID)
	if err != nil || question != nil || expired.Status != "expired" {
		t.Fatalf("expired session = question=%v session=%+v err=%v", question, expired, err)
	}
	if _, err := f.svc.Answer(ctx, f.user.ID, &session.ID, AnswerRequest{QuestionID: session.QuestionIDs[0], Answer: []string{"A"}}); err == nil {
		t.Fatal("expired session should reject answers")
	}
}

func TestPracticeSessionAcceptsCustomTimeAndRejectsTooLong(t *testing.T) {
	f := newPracticeFixture(t)
	ctx := context.Background()
	customSeconds := 137 * 60
	session, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{
		BankIDs: []int64{f.banks[0].ID}, Mode: "sequential", Limit: 1, TimeLimitSeconds: &customSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.TimeLimitSeconds == nil || *session.TimeLimitSeconds != customSeconds || session.ExpiresAt == nil {
		t.Fatalf("custom duration was not persisted: %+v", session)
	}

	tooLong := maxPracticeTimeSeconds + 1
	if _, err := f.svc.CreateSession(ctx, f.user.ID, CreateSessionRequest{
		BankIDs: []int64{f.banks[0].ID}, Mode: "sequential", Limit: 1, TimeLimitSeconds: &tooLong,
	}); err == nil {
		t.Fatal("durations longer than 24 hours should be rejected")
	}
}

func ptr64(value int64) *int64 { return &value }
