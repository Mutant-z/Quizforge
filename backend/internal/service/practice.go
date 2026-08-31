package service

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/judge"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/review"
)

// PracticeService 刷题业务：会话、判题、错题、复习调度。
type PracticeService struct {
	repo      *sqlite.Repository
	scheduler review.ReviewScheduler
}

func NewPracticeService(repo *sqlite.Repository, scheduler review.ReviewScheduler) *PracticeService {
	return &PracticeService{repo: repo, scheduler: scheduler}
}

// CreateSessionRequest 创建刷题会话。
type CreateSessionRequest struct {
	BankID           *int64  `json:"bank_id"`
	BankIDs          []int64 `json:"bank_ids"`
	SubjectID        *int64  `json:"subject_id"`
	ChapterID        *int64  `json:"chapter_id"`
	Mode             string  `json:"mode"` // sequential | random | wrong | favorite | due | unattempted
	Type             string  `json:"type"`
	Limit            int     `json:"limit"`
	TimeLimitSeconds *int    `json:"time_limit_seconds"`
}

const (
	maxPracticeLimit       = 500
	maxPracticeTimeSeconds = 24 * 60 * 60
)

// PracticePreview 练习配置预览，不创建会话。
type PracticePreview struct {
	AvailableCount int `json:"available_count"`
	SelectedCount  int `json:"selected_count"`
}

// Preview 计算当前配置下的可用题数。
func (s *PracticeService) Preview(ctx context.Context, userID int64, req CreateSessionRequest) (*PracticePreview, error) {
	ids, err := s.selectQuestionIDs(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	return &PracticePreview{
		AvailableCount: len(ids),
		SelectedCount:  minPracticeCount(len(ids), req.Limit),
	}, nil
}

// CreateSession 选择题目并创建会话。
func (s *PracticeService) CreateSession(ctx context.Context, userID int64, req CreateSessionRequest) (*domain.PracticeSession, error) {
	if err := validatePracticeRequest(req); err != nil {
		return nil, err
	}
	bankIDs, err := s.normalizeAndValidateBanks(ctx, req)
	if err != nil {
		return nil, err
	}
	questionIDs, err := s.selectQuestionIDs(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if len(questionIDs) == 0 {
		return nil, api.InvalidRequest("当前范围没有可用题目")
	}

	limit := req.Limit
	if limit == 0 {
		limit = maxPracticeLimit
	}
	if limit > len(questionIDs) {
		limit = len(questionIDs)
	}
	questionIDs = questionIDs[:limit]

	mode := req.Mode
	if mode == "" {
		mode = "sequential"
	}
	var legacyBankID *int64
	if len(bankIDs) == 1 {
		legacyBankID = &bankIDs[0]
	}
	var expiresAt *time.Time
	if req.TimeLimitSeconds != nil {
		expires := time.Now().UTC().Add(time.Duration(*req.TimeLimitSeconds) * time.Second)
		expiresAt = &expires
	}

	session := &domain.PracticeSession{
		UserID: userID, BankID: legacyBankID, BankIDs: bankIDs, SubjectID: req.SubjectID, ChapterID: req.ChapterID,
		Mode: mode, TimeLimitSeconds: req.TimeLimitSeconds, ExpiresAt: expiresAt,
		QuestionIDs: questionIDs, CurrentIndex: 0, TotalCount: len(questionIDs),
		Status: "active", StartedAt: time.Now().UTC(),
	}
	return s.repo.CreateSession(ctx, session)
}

func (s *PracticeService) selectQuestionIDs(ctx context.Context, userID int64, req CreateSessionRequest) ([]int64, error) {
	if err := validatePracticeRequest(req); err != nil {
		return nil, err
	}
	bankIDs, err := s.normalizeAndValidateBanks(ctx, req)
	if err != nil {
		return nil, err
	}
	mode := req.Mode
	if mode == "" {
		mode = "sequential"
	}
	base := sqlite.QuestionFilter{
		BankIDs: bankIDs, SubjectID: req.SubjectID, ChapterID: req.ChapterID,
		Type: req.Type, Status: domain.QuestionStatusPublished,
		OrderRandom: mode == "random", RequireAnswer: true, RequireOptions: true,
	}

	var questionIDs []int64
	switch mode {
	case "wrong", "due":
		wrongStatus := ""
		if mode == "wrong" {
			wrongStatus = "learning"
		}
		wrongFilter := sqlite.WrongFilter{
			UserID: userID, BankIDs: bankIDs, SubjectID: req.SubjectID, ChapterID: req.ChapterID,
			Type: req.Type, Status: wrongStatus, DueOnly: mode == "due",
			SortByPriority: mode == "wrong", SortByDue: mode == "due",
			RequireAnswer: true, RequireOptions: true,
		}
		questionIDs, err = s.repo.ListWrongQuestionIDs(ctx, wrongFilter)
	case "favorite":
		favoriteIDs, favErr := s.repo.ListFavoriteIDs(ctx, userID)
		if favErr != nil {
			return nil, favErr
		}
		if len(favoriteIDs) == 0 {
			return []int64{}, nil
		}
		base.IDs = favoriteIDs
		questionIDs, err = s.repo.ListQuestionIDs(ctx, base)
	case "unattempted":
		allIDs, listErr := s.repo.ListQuestionIDs(ctx, base)
		if listErr != nil {
			return nil, listErr
		}
		attemptedIDs, attemptedErr := s.attemptedIDs(ctx, userID)
		if attemptedErr != nil {
			return nil, attemptedErr
		}
		attempted := make(map[int64]struct{}, len(attemptedIDs))
		for _, id := range attemptedIDs {
			attempted[id] = struct{}{}
		}
		for _, id := range allIDs {
			if _, ok := attempted[id]; !ok {
				questionIDs = append(questionIDs, id)
			}
		}
	default:
		questionIDs, err = s.repo.ListQuestionIDs(ctx, base)
	}
	if err != nil {
		return nil, err
	}
	if mode == "random" {
		rand.Shuffle(len(questionIDs), func(i, j int) { questionIDs[i], questionIDs[j] = questionIDs[j], questionIDs[i] })
	}
	return questionIDs, nil
}

func (s *PracticeService) normalizeAndValidateBanks(ctx context.Context, req CreateSessionRequest) ([]int64, error) {
	ids := append([]int64(nil), req.BankIDs...)
	if len(ids) == 0 && req.BankID != nil {
		ids = append(ids, *req.BankID)
	}
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, api.InvalidRequest("题库范围不合法")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		bank, err := s.repo.GetBank(ctx, id)
		if err != nil || bank.Status != "active" {
			return nil, api.InvalidRequest("题库不存在或已停用")
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, nil
}

func validatePracticeRequest(req CreateSessionRequest) error {
	if req.Limit < 0 || req.Limit > maxPracticeLimit {
		return api.InvalidRequest("刷题数量必须在 0 到 500 之间")
	}
	if requestBankCount(req) > 1 && (req.SubjectID != nil || req.ChapterID != nil) {
		return api.InvalidRequest("选择多个题库时不能同时指定科目或章节")
	}
	if req.TimeLimitSeconds != nil && (*req.TimeLimitSeconds < 1 || *req.TimeLimitSeconds > maxPracticeTimeSeconds) {
		return api.InvalidRequest("刷题时间必须在 1 秒到 24 小时之间")
	}
	if req.Type != "" {
		switch req.Type {
		case domain.QuestionTypeSingleChoice, domain.QuestionTypeMultipleChoice, domain.QuestionTypeTrueFalse,
			domain.QuestionTypeFillBlank, domain.QuestionTypeShortAnswer, domain.QuestionTypeMaterialAnalysis,
			domain.QuestionTypeEssay, domain.QuestionTypeCode:
		default:
			return api.InvalidRequest("题型不合法")
		}
	}
	switch req.Mode {
	case "", "sequential", "random", "wrong", "favorite", "due", "unattempted":
		return nil
	default:
		return api.InvalidRequest("刷题模式不合法")
	}
}

func requestBankCount(req CreateSessionRequest) int {
	ids := req.BankIDs
	if len(ids) == 0 && req.BankID != nil {
		ids = []int64{*req.BankID}
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func minPracticeCount(available, requested int) int {
	if requested <= 0 {
		if available > maxPracticeLimit {
			return maxPracticeLimit
		}
		return available
	}
	if requested > available {
		return available
	}
	return requested
}

func (s *PracticeService) attemptedIDs(ctx context.Context, userID int64) ([]int64, error) {
	// 返回所有作答过的题目 ID
	rows, err := s.repo.DB().QueryContext(ctx,
		`SELECT DISTINCT question_id FROM question_attempts WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AnswerRequest 提交答案。
type AnswerRequest struct {
	QuestionID int64    `json:"question_id"`
	Answer     []string `json:"answer"`
	DurationMs int      `json:"duration_ms"`
}

// AnswerResult 判题结果（即时反馈）。
type AnswerResult struct {
	Correct       bool                 `json:"correct"`
	Score         float64              `json:"score"`
	UserAnswer    []string             `json:"user_answer"`
	CorrectAnswer []string             `json:"correct_answer"`
	Analysis      string               `json:"analysis"`
	Question      *domain.Question     `json:"question"`
	WrongAdded    bool                 `json:"wrong_added"`
	NextReviewAt  *time.Time           `json:"next_review_at,omitempty"`
	ReviewInfo    *review.ReviewResult `json:"review_info,omitempty"`
	AIExplanation string               `json:"ai_explanation,omitempty"`
}

// Answer 判题并记录。
func (s *PracticeService) Answer(ctx context.Context, userID int64, sessionID *int64, req AnswerRequest) (*AnswerResult, error) {
	var session *domain.PracticeSession
	var err error
	if sessionID != nil {
		session, err = s.getOwnedSession(ctx, userID, *sessionID)
		if err != nil {
			return nil, err
		}
		if session.Status == "expired" {
			return nil, api.Conflict("刷题时间已到，会话已结束")
		}
		if session.Status != "active" {
			return nil, api.Conflict("会话已结束，无法继续答题")
		}
		if session.CurrentIndex >= len(session.QuestionIDs) || session.QuestionIDs[session.CurrentIndex] != req.QuestionID {
			return nil, api.InvalidRequest("只能提交当前题目的答案")
		}
	}
	q, err := s.repo.GetQuestion(ctx, req.QuestionID)
	if err != nil {
		return nil, api.NotFound("题目不存在")
	}
	if !isPracticeQuestionUsable(q) {
		return nil, api.Conflict("当前题目缺少答案或选项，无法作答")
	}
	if session != nil {
		if existing, err := s.repo.FindSessionAttempt(ctx, session.ID, req.QuestionID); err != nil {
			return nil, api.Internal("读取答题记录失败", err)
		} else if existing != nil {
			// 提交请求可能在服务端已写入后才因异常断开；返回原结果，避免重试重复计分。
			return &AnswerResult{
				Correct:       existing.IsCorrect,
				Score:         existing.Score,
				UserAnswer:    existing.Answer,
				CorrectAnswer: q.Answer,
				Analysis:      firstNonEmpty(q.AIEnhancedAnalysis, q.OriginalAnalysis),
				Question:      q,
			}, nil
		}
	}

	result := judge.Judge(q, req.Answer)
	res := &AnswerResult{
		Correct:       result.IsCorrect,
		Score:         result.Score,
		UserAnswer:    req.Answer,
		CorrectAnswer: result.Correct,
		Analysis:      firstNonEmpty(q.AIEnhancedAnalysis, q.OriginalAnalysis),
		Question:      q,
	}

	attempt := &domain.QuestionAttempt{
		UserID: userID, QuestionID: req.QuestionID, SessionID: sessionID,
		Answer: req.Answer, IsCorrect: result.IsCorrect, Score: result.Score, DurationMs: req.DurationMs,
	}
	if _, err := s.repo.CreateAttempt(ctx, attempt); err != nil {
		return nil, api.Internal("记录答题失败", err)
	}
	if session != nil {
		correctCount, wrongCount := session.CorrectCount, session.WrongCount
		if result.IsCorrect {
			correctCount++
		} else {
			wrongCount++
		}
		if err := s.repo.UpdateSessionProgress(ctx, session.ID, session.CurrentIndex, correctCount, wrongCount, "active"); err != nil {
			return nil, api.Internal("更新刷题进度失败", err)
		}
	}

	date := time.Now().UTC().Format("2006-01-02")
	if result.IsCorrect {
		s.repo.UpsertDailyStat(ctx, userID, date, 1, 0, 0, req.DurationMs)
		// 错题答对
		if w, err := s.repo.GetWrongByQuestion(ctx, userID, req.QuestionID); err == nil {
			s.repo.RecordCorrectOnWrong(ctx, userID, req.QuestionID)
			_ = s.scheduleAfterReview(ctx, w, "good")
		}
	} else {
		s.repo.UpsertDailyStat(ctx, userID, date, 0, 1, 0, req.DurationMs)
		w, err := s.repo.UpsertWrongQuestion(ctx, userID, req.QuestionID)
		if err != nil {
			return res, nil
		}
		res.WrongAdded = true
		if w.NextReviewAt != nil {
			res.NextReviewAt = w.NextReviewAt
		}
	}
	return res, nil
}

// scheduleAfterReview 在答对 / 复习后更新调度。
func (s *PracticeService) scheduleAfterReview(ctx context.Context, w *domain.WrongQuestion, result string) error {
	input := review.ReviewInput{
		WrongCount:   w.WrongCount,
		CorrectCount: w.CorrectCount + 1,
		ReviewCount:  w.ReviewCount,
		MasteryScore: w.MasteryScore,
		IntervalDays: w.IntervalDays,
		Difficulty:   3,
		Result:       result,
	}
	if w.LastReviewAt != nil && !w.LastReviewAt.IsZero() {
		input.LastReviewAt = *w.LastReviewAt
	}
	r, err := s.scheduler.Calculate(ctx, input)
	if err != nil {
		return err
	}
	w.CorrectCount = input.CorrectCount
	w.ReviewCount = w.ReviewCount + 1
	w.MasteryScore = r.MasteryScore
	w.IntervalDays = r.IntervalDays
	w.PriorityScore = r.PriorityScore
	w.NextReviewAt = &r.NextReviewAt
	w.Status = r.Status
	return s.repo.UpdateWrongQuestion(ctx, w)
}

// GetCurrentQuestion 获取会话当前题。
func (s *PracticeService) GetCurrentQuestion(ctx context.Context, userID, sessionID int64) (*domain.Question, *domain.PracticeSession, error) {
	session, err := s.getOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	for session.Status == "active" && session.CurrentIndex < len(session.QuestionIDs) {
		q, err := s.repo.GetQuestion(ctx, session.QuestionIDs[session.CurrentIndex])
		if err != nil {
			return nil, session, api.NotFound("题目不存在")
		}
		if !isPracticeQuestionUsable(q) {
			// 兼容过滤规则上线前创建的旧会话：发现不完整题目时自动跳过，绝不展示空题。
			session.CurrentIndex++
			status := "active"
			if session.CurrentIndex >= len(session.QuestionIDs) {
				status = "finished"
			}
			if err := s.repo.UpdateSessionProgress(ctx, session.ID, session.CurrentIndex, session.CorrectCount, session.WrongCount, status); err != nil {
				return nil, session, err
			}
			if status == "finished" {
				finishedSession, getErr := s.repo.GetSession(ctx, session.ID)
				return nil, finishedSession, getErr
			}
			session, err = s.repo.GetSession(ctx, session.ID)
			if err != nil {
				return nil, nil, err
			}
			continue
		}
		s.fillUserFlags(ctx, userID, q)
		return q, session, nil
	}
	return nil, session, nil
}

func isPracticeQuestionUsable(q *domain.Question) bool {
	if q == nil || !hasNonBlankAnswer(q.Answer) {
		return false
	}
	if q.Type != domain.QuestionTypeSingleChoice && q.Type != domain.QuestionTypeMultipleChoice && q.Type != domain.QuestionTypeTrueFalse {
		return true
	}
	for _, option := range q.Options {
		if strings.TrimSpace(option.Key) != "" && strings.TrimSpace(option.Content) != "" {
			return true
		}
	}
	return false
}

func hasNonBlankAnswer(answers []string) bool {
	for _, answer := range answers {
		if strings.TrimSpace(answer) != "" {
			return true
		}
	}
	return false
}

// GetSession 获取当前用户的会话，并在读取时同步检查服务端截止时间。
func (s *PracticeService) GetSession(ctx context.Context, userID, sessionID int64) (*domain.PracticeSession, error) {
	return s.getOwnedSession(ctx, userID, sessionID)
}

// Next 前进到下一题。
func (s *PracticeService) Next(ctx context.Context, userID, sessionID int64) (*domain.PracticeSession, error) {
	session, err := s.getOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "active" {
		return session, nil
	}
	if session.CurrentIndex < session.TotalCount {
		session.CurrentIndex++
	}
	status := "active"
	if session.CurrentIndex >= session.TotalCount {
		status = "finished"
	}
	if err := s.repo.UpdateSessionProgress(ctx, sessionID, session.CurrentIndex, session.CorrectCount, session.WrongCount, status); err != nil {
		return nil, err
	}
	return s.repo.GetSession(ctx, sessionID)
}

// Finish 结束会话。
func (s *PracticeService) Finish(ctx context.Context, userID, sessionID int64) (*domain.PracticeSession, error) {
	session, err := s.getOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "active" {
		return session, nil
	}
	if err := s.repo.UpdateSessionProgress(ctx, sessionID, session.CurrentIndex, session.CorrectCount, session.WrongCount, "finished"); err != nil {
		return nil, err
	}
	return s.repo.GetSession(ctx, sessionID)
}

func (s *PracticeService) getOwnedSession(ctx context.Context, userID, sessionID int64) (*domain.PracticeSession, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, api.NotFound("会话不存在")
	}
	if session.UserID != userID {
		return nil, api.Unauthorized("无权访问该会话")
	}
	if session.Status == "active" && session.ExpiresAt != nil && !time.Now().UTC().Before(*session.ExpiresAt) {
		if err := s.repo.ExpireSession(ctx, sessionID); err != nil {
			return nil, err
		}
		return s.repo.GetSession(ctx, sessionID)
	}
	return session, nil
}

func (s *PracticeService) fillUserFlags(ctx context.Context, userID int64, q *domain.Question) {
	if isFav, err := s.repo.IsFavorite(ctx, userID, q.ID); err == nil {
		q.IsFav = isFav
	}
	if w, err := s.repo.GetWrongByQuestion(ctx, userID, q.ID); err == nil {
		q.IsWrong = true
		if w.NextReviewAt != nil {
			_ = w.NextReviewAt
		}
	}
	if _, err := s.repo.GetNote(ctx, userID, q.ID); err == nil {
		q.HasNote = true
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
