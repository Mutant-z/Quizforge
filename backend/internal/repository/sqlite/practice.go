package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// CreateSession 创建刷题会话。
func (r *Repository) CreateSession(ctx context.Context, s *domain.PracticeSession) (*domain.PracticeSession, error) {
	bankIDs := append([]int64(nil), s.BankIDs...)
	if len(bankIDs) == 0 && s.BankID != nil {
		bankIDs = []int64{*s.BankID}
	}
	s.BankIDs = bankIDs
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO practice_sessions
			(user_id, bank_id, bank_ids_json, subject_id, chapter_id, mode, time_limit_seconds, expires_at,
			 question_ids_json, current_index, total_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, s.BankID, marshalJSON(bankIDs), s.SubjectID, s.ChapterID, s.Mode,
		s.TimeLimitSeconds, fmtTime(s.ExpiresAt), marshalJSON(s.QuestionIDs), s.CurrentIndex, len(s.QuestionIDs))
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	id, _ := res.LastInsertId()
	s.ID = id
	return s, nil
}

func (r *Repository) GetSession(ctx context.Context, id int64) (*domain.PracticeSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, bank_id, bank_ids_json, subject_id, chapter_id, mode, time_limit_seconds, expires_at,
		       question_ids_json, current_index, total_count, correct_count, wrong_count, status, started_at,
		       finished_at, created_at, updated_at
		FROM practice_sessions WHERE id = ?`, id)
	var s domain.PracticeSession
	var bankID, subjectID, chapterID sql.NullInt64
	var timeLimit sql.NullInt64
	var bankIDsJSON, idsJSON, expiresAt, finishedAt sql.NullString
	var startedAt, createdAt string
	var updatedAt sql.NullString
	if err := row.Scan(&s.ID, &s.UserID, &bankID, &bankIDsJSON, &subjectID, &chapterID, &s.Mode, &timeLimit,
		&expiresAt, &idsJSON, &s.CurrentIndex, &s.TotalCount, &s.CorrectCount, &s.WrongCount, &s.Status,
		&startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if bankID.Valid {
		s.BankID = &bankID.Int64
	}
	if subjectID.Valid {
		s.SubjectID = &subjectID.Int64
	}
	if chapterID.Valid {
		s.ChapterID = &chapterID.Int64
	}
	if timeLimit.Valid {
		v := int(timeLimit.Int64)
		s.TimeLimitSeconds = &v
	}
	s.ExpiresAt = parseTime(expiresAt)
	s.BankIDs = []int64{}
	if bankIDsJSON.Valid && bankIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(bankIDsJSON.String), &s.BankIDs)
	}
	if len(s.BankIDs) == 0 && bankID.Valid {
		s.BankIDs = []int64{bankID.Int64}
	}
	s.QuestionIDs = []int64{}
	json.Unmarshal([]byte(idsJSON.String), &s.QuestionIDs)
	s.StartedAt = parseSQLiteTime(startedAt)
	s.CreatedAt = parseSQLiteTime(createdAt)
	if finishedAt.Valid && finishedAt.String != "" {
		t := parseSQLiteTime(finishedAt.String)
		s.FinishedAt = &t
	}
	_ = updatedAt
	return &s, nil
}

// UpdateSessionProgress 更新进度。
func (r *Repository) UpdateSessionProgress(ctx context.Context, sessionID int64, index, correct, wrong int, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE practice_sessions SET current_index = ?, correct_count = ?, wrong_count = ?, status = ?,
			finished_at = CASE WHEN ? = 'finished' THEN datetime('now') ELSE finished_at END,
			updated_at = datetime('now')
		WHERE id = ?`, index, correct, wrong, status, status, sessionID)
	return err
}

// ExpireSession 将仍处于 active 状态的会话标记为超时。条件更新保证重复请求幂等。
func (r *Repository) ExpireSession(ctx context.Context, sessionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE practice_sessions
		SET status = 'expired', finished_at = COALESCE(finished_at, datetime('now')), updated_at = datetime('now')
		WHERE id = ? AND status = 'active'`, sessionID)
	return err
}

// CreateAttempt 写入答题记录。
func (r *Repository) CreateAttempt(ctx context.Context, a *domain.QuestionAttempt) (*domain.QuestionAttempt, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO question_attempts (user_id, question_id, session_id, answer_json, is_correct, score, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.QuestionID, a.SessionID, marshalJSON(a.Answer), boolToInt(a.IsCorrect), a.Score, a.DurationMs)
	if err != nil {
		return nil, fmt.Errorf("insert attempt: %w", err)
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

// FindSessionAttempt 查询会话中某道题已经保存的首次答题记录。
// 答案接口可能因为网络重试或服务端异常被重复调用，调用方可用它实现幂等响应。
func (r *Repository) FindSessionAttempt(ctx context.Context, sessionID, questionID int64) (*domain.QuestionAttempt, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, question_id, session_id, answer_json, is_correct, score, duration_ms, created_at
		FROM question_attempts
		WHERE session_id = ? AND question_id = ?
		ORDER BY id ASC LIMIT 1`, sessionID, questionID)

	var a domain.QuestionAttempt
	var storedSessionID sql.NullInt64
	var answerJSON, createdAt string
	var isCorrect int
	if err := row.Scan(&a.ID, &a.UserID, &a.QuestionID, &storedSessionID, &answerJSON, &isCorrect, &a.Score, &a.DurationMs, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if storedSessionID.Valid {
		a.SessionID = &storedSessionID.Int64
	}
	if err := json.Unmarshal([]byte(answerJSON), &a.Answer); err != nil {
		return nil, fmt.Errorf("decode attempt answer: %w", err)
	}
	a.IsCorrect = isCorrect != 0
	a.CreatedAt = parseSQLiteTime(createdAt)
	return &a, nil
}

// UpsertDailyStat 更新每日统计。
func (r *Repository) UpsertDailyStat(ctx context.Context, userID int64, date string, correct, wrong, reviewCount, durationMs int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO learning_statistics (user_id, stat_date, question_count, correct_count, wrong_count, review_count, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, stat_date) DO UPDATE SET
			question_count = question_count + excluded.question_count,
			correct_count = correct_count + excluded.correct_count,
			wrong_count = wrong_count + excluded.wrong_count,
			review_count = review_count + excluded.review_count,
			duration_ms = duration_ms + excluded.duration_ms`,
		userID, date, correct+wrong, correct, wrong, reviewCount, durationMs)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- Wrong Questions ----

// UpsertWrongQuestion 答错自动加入错题本。
func (r *Repository) UpsertWrongQuestion(ctx context.Context, userID, questionID int64) (*domain.WrongQuestion, error) {
	// 先查存在
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM wrong_questions WHERE user_id = ? AND question_id = ?`, userID, questionID).Scan(&id)
	if err == nil {
		_, err = r.db.ExecContext(ctx, `
			UPDATE wrong_questions SET wrong_count = wrong_count + 1, last_wrong_at = datetime('now'),
				interval_days = 0, mastery_score = 0, priority_score = priority_score + 1, status = 'learning',
				updated_at = datetime('now')
			WHERE id = ?`, id)
		if err != nil {
			return nil, err
		}
		return r.GetWrongQuestion(ctx, id)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO wrong_questions (user_id, question_id, wrong_count, first_wrong_at, last_wrong_at, status)
		VALUES (?, ?, 1, datetime('now'), datetime('now'), 'learning')`, userID, questionID)
	if err != nil {
		return nil, fmt.Errorf("insert wrong question: %w", err)
	}
	id, _ = res.LastInsertId()
	return r.GetWrongQuestion(ctx, id)
}

// RecordCorrectOnWrong 错题答对记录。
func (r *Repository) RecordCorrectOnWrong(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_questions SET correct_count = correct_count + 1, updated_at = datetime('now')
		WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return err
}

func (r *Repository) GetWrongQuestion(ctx context.Context, id int64) (*domain.WrongQuestion, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, question_id, wrong_count, correct_count, review_count, mastery_score, interval_days,
		       difficulty_factor, priority_score, first_wrong_at, last_wrong_at, last_review_at, next_review_at, status, created_at, updated_at
		FROM wrong_questions WHERE id = ?`, id)
	return scanWrongQuestion(row)
}

func (r *Repository) GetWrongByQuestion(ctx context.Context, userID, questionID int64) (*domain.WrongQuestion, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, question_id, wrong_count, correct_count, review_count, mastery_score, interval_days,
		       difficulty_factor, priority_score, first_wrong_at, last_wrong_at, last_review_at, next_review_at, status, created_at, updated_at
		FROM wrong_questions WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return scanWrongQuestion(row)
}

// UpdateWrongQuestion 更新错题复习状态（由 ReviewScheduler 计算结果写入）。
func (r *Repository) UpdateWrongQuestion(ctx context.Context, w *domain.WrongQuestion) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wrong_questions SET
			correct_count = ?, review_count = ?, mastery_score = ?, interval_days = ?,
			difficulty_factor = ?, priority_score = ?, last_review_at = datetime('now'),
			next_review_at = ?, status = ?, updated_at = datetime('now')
		WHERE id = ?`,
		w.CorrectCount, w.ReviewCount, w.MasteryScore, w.IntervalDays, w.DifficultyFactor, w.PriorityScore,
		fmtTime(w.NextReviewAt), w.Status, w.ID)
	return err
}

// RemoveWrongQuestion 移除错题。
func (r *Repository) RemoveWrongQuestion(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM wrong_questions WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return err
}

// ListWrongQuestions 错题列表（带筛选 + 题目）。
func (r *Repository) ListWrongQuestions(ctx context.Context, f WrongFilter) ([]*domain.WrongQuestion, int64, error) {
	where := []string{"w.user_id = ?"}
	args := []interface{}{f.UserID}
	if f.SubjectID != nil {
		where = append(where, "q.subject_id = ?")
		args = append(args, *f.SubjectID)
	}
	if len(f.BankIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(f.BankIDs)), ",")
		where = append(where, "q.bank_id IN ("+placeholders+")")
		for _, id := range f.BankIDs {
			args = append(args, id)
		}
	}
	if f.Type != "" {
		where = append(where, "q.type = ?")
		args = append(args, f.Type)
	}
	if f.ChapterID != nil {
		where = append(where, "q.chapter_id = ?")
		args = append(args, *f.ChapterID)
	}
	if f.Status != "" {
		where = append(where, "w.status = ?")
		args = append(args, f.Status)
	}
	if f.DueOnly {
		where = append(where, "w.next_review_at IS NOT NULL AND w.next_review_at <= datetime('now')")
	}
	if f.RequireAnswer || f.RequireOptions {
		where = append(where, playableQuestionConditions("q", f.RequireAnswer, f.RequireOptions)...)
	}
	if f.MinWrongCount > 0 {
		where = append(where, "w.wrong_count >= ?")
		args = append(args, f.MinWrongCount)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wrong_questions w JOIN canonical_questions q ON q.id = w.question_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	order := "w.updated_at DESC"
	if f.SortByPriority {
		order = "w.priority_score DESC"
	} else if f.SortByDue {
		order = "w.next_review_at ASC"
	}

	query := `SELECT w.id, w.user_id, w.question_id, w.wrong_count, w.correct_count, w.review_count, w.mastery_score,
		w.interval_days, w.difficulty_factor, w.priority_score, w.first_wrong_at, w.last_wrong_at, w.last_review_at,
		w.next_review_at, w.status, w.created_at, w.updated_at
		FROM wrong_questions w JOIN canonical_questions q ON q.id = w.question_id
		WHERE ` + whereSQL + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []*domain.WrongQuestion{}
	for rows.Next() {
		w, err := scanWrongQuestion(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, w)
	}
	// 填充题目 + 笔记
	for _, w := range list {
		if q, err := r.GetQuestion(ctx, w.QuestionID); err == nil {
			w.Question = q
		}
		if note, err := r.GetNote(ctx, f.UserID, w.QuestionID); err == nil {
			w.Note = note.Content
		}
	}
	return list, total, rows.Err()
}

// ListWrongQuestionIDs 返回满足错题筛选条件的题目 ID，供刷题会话固定题目快照使用。
func (r *Repository) ListWrongQuestionIDs(ctx context.Context, f WrongFilter) ([]int64, error) {
	where := []string{"w.user_id = ?"}
	args := []interface{}{f.UserID}
	if len(f.BankIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(f.BankIDs)), ",")
		where = append(where, "q.bank_id IN ("+placeholders+")")
		for _, id := range f.BankIDs {
			args = append(args, id)
		}
	}
	if f.SubjectID != nil {
		where = append(where, "q.subject_id = ?")
		args = append(args, *f.SubjectID)
	}
	if f.ChapterID != nil {
		where = append(where, "q.chapter_id = ?")
		args = append(args, *f.ChapterID)
	}
	if f.Type != "" {
		where = append(where, "q.type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		where = append(where, "w.status = ?")
		args = append(args, f.Status)
	}
	if f.DueOnly {
		where = append(where, "w.next_review_at IS NOT NULL AND w.next_review_at <= datetime('now')")
	}
	if f.RequireAnswer || f.RequireOptions {
		where = append(where, playableQuestionConditions("q", f.RequireAnswer, f.RequireOptions)...)
	}
	if f.MinWrongCount > 0 {
		where = append(where, "w.wrong_count >= ?")
		args = append(args, f.MinWrongCount)
	}

	order := "w.updated_at DESC"
	if f.SortByPriority {
		order = "w.priority_score DESC"
	} else if f.SortByDue {
		order = "w.next_review_at ASC"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT q.id
		FROM wrong_questions w
		JOIN canonical_questions q ON q.id = w.question_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order, args...)
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

func scanWrongQuestion(row interface{ Scan(...any) error }) (*domain.WrongQuestion, error) {
	var w domain.WrongQuestion
	var firstWrong, lastWrong, lastReview, nextReview sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&w.ID, &w.UserID, &w.QuestionID, &w.WrongCount, &w.CorrectCount, &w.ReviewCount,
		&w.MasteryScore, &w.IntervalDays, &w.DifficultyFactor, &w.PriorityScore,
		&firstWrong, &lastWrong, &lastReview, &nextReview, &w.Status, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	w.FirstWrongAt = parseTime(firstWrong)
	w.LastWrongAt = parseTime(lastWrong)
	w.LastReviewAt = parseTime(lastReview)
	w.NextReviewAt = parseTime(nextReview)
	w.CreatedAt = parseSQLiteTime(createdAt)
	w.UpdatedAt = parseSQLiteTime(updatedAt)
	return &w, nil
}

// WrongFilter 错题筛选。
type WrongFilter struct {
	UserID         int64
	BankIDs        []int64
	SubjectID      *int64
	ChapterID      *int64
	Type           string
	Status         string
	DueOnly        bool
	MinWrongCount  int
	SortByPriority bool
	SortByDue      bool
	Page           int
	PageSize       int
	RequireAnswer  bool
	RequireOptions bool
}

// ---- Notes ----

func (r *Repository) GetNote(ctx context.Context, userID, questionID int64) (*domain.QuestionNote, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, question_id, content, version, created_at, updated_at
		FROM question_notes WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return scanNote(row)
}

func (r *Repository) UpsertNote(ctx context.Context, userID, questionID int64, content string) (*domain.QuestionNote, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO question_notes (user_id, question_id, content, version) VALUES (?, ?, ?, 1)
		ON CONFLICT(user_id, question_id) DO UPDATE SET
			content = excluded.content, version = version + 1, updated_at = datetime('now')`,
		userID, questionID, content)
	if err != nil {
		return nil, fmt.Errorf("upsert note: %w", err)
	}
	return r.GetNote(ctx, userID, questionID)
}

func (r *Repository) DeleteNote(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM question_notes WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return err
}

func scanNote(row *sql.Row) (*domain.QuestionNote, error) {
	var n domain.QuestionNote
	var createdAt, updatedAt string
	if err := row.Scan(&n.ID, &n.UserID, &n.QuestionID, &n.Content, &n.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	n.CreatedAt = parseSQLiteTime(createdAt)
	n.UpdatedAt = parseSQLiteTime(updatedAt)
	return &n, nil
}

// ---- Favorites ----

func (r *Repository) AddFavorite(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO favorites (user_id, question_id) VALUES (?, ?)`, userID, questionID)
	return err
}

func (r *Repository) RemoveFavorite(ctx context.Context, userID, questionID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND question_id = ?`, userID, questionID)
	return err
}

func (r *Repository) IsFavorite(ctx context.Context, userID, questionID int64) (bool, error) {
	var c int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM favorites WHERE user_id = ? AND question_id = ?`, userID, questionID).Scan(&c)
	return c > 0, err
}

// ListFavoriteIDs 收藏题目 ID 列表。
func (r *Repository) ListFavoriteIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT question_id FROM favorites WHERE user_id = ? ORDER BY id DESC`, userID)
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

// RecordReview 复习记录。
func (r *Repository) RecordReview(ctx context.Context, userID, wrongQuestionID int64, result string, isCorrect bool, intervalDays float64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO review_records (user_id, wrong_question_id, result, is_correct, interval_days)
		VALUES (?, ?, ?, ?, ?)`, userID, wrongQuestionID, result, boolToInt(isCorrect), intervalDays)
	return err
}
