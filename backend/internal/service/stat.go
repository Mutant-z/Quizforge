package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// StatService 学习统计。
type StatService struct {
	repo *sqlite.Repository
}

func NewStatService(repo *sqlite.Repository) *StatService {
	return &StatService{repo: repo}
}

// DailyOverview 今日 / 累计概览。
func (s *StatService) Overview(ctx context.Context, userID int64) (map[string]interface{}, error) {
	today := time.Now().UTC().Format("2006-01-02")
	out := map[string]interface{}{}

	// 今日
	var todayCount, todayCorrect, todayWrong, todayReview int64
	_ = s.repo.DB().QueryRowContext(ctx,
		`SELECT question_count, correct_count, wrong_count, review_count FROM learning_statistics WHERE user_id = ? AND stat_date = ?`,
		userID, today).Scan(&todayCount, &todayCorrect, &todayWrong, &todayReview)

	// 累计
	var totalCount, totalCorrect int64
	_ = s.repo.DB().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(question_count),0), COALESCE(SUM(correct_count),0) FROM learning_statistics WHERE user_id = ?`, userID).Scan(&totalCount, &totalCorrect)

	// 错题数 / 待复习
	var wrongTotal, dueCount int64
	_ = s.repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wrong_questions WHERE user_id = ?`, userID).Scan(&wrongTotal)
	_ = s.repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wrong_questions WHERE user_id = ? AND next_review_at IS NOT NULL AND next_review_at <= datetime('now')`, userID).Scan(&dueCount)

	// 连续学习天数
	streak := s.streakDays(ctx, userID)

	accuracy := 0.0
	if totalCount > 0 {
		accuracy = float64(totalCorrect) / float64(totalCount) * 100
	}
	todayAccuracy := 0.0
	if todayCount > 0 {
		todayAccuracy = float64(todayCorrect) / float64(todayCount) * 100
	}

	out["today"] = map[string]int64{
		"question_count": todayCount, "correct_count": todayCorrect,
		"wrong_count": todayWrong, "review_count": todayReview,
	}
	out["today_accuracy"] = round1(todayAccuracy)
	out["total_question_count"] = totalCount
	out["accuracy"] = round1(accuracy)
	out["wrong_count"] = wrongTotal
	out["due_review_count"] = dueCount
	out["streak_days"] = streak
	return out, nil
}

// streakDays 连续学习天数（按 learning_statistics 有记录的日期向前推）。
func (s *StatService) streakDays(ctx context.Context, userID int64) int {
	rows, err := s.repo.DB().QueryContext(ctx,
		`SELECT stat_date FROM learning_statistics WHERE user_id = ? ORDER BY stat_date DESC`, userID)
	if err != nil {
		return 0
	}
	defer rows.Close()
	days := map[string]bool{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			break
		}
		days[d] = true
	}
	streak := 0
	cur := time.Now().UTC()
	// 今天没学则从昨天开始算
	if !days[cur.Format("2006-01-02")] {
		cur = cur.AddDate(0, 0, -1)
	}
	for days[cur.Format("2006-01-02")] {
		streak++
		cur = cur.AddDate(0, 0, -1)
	}
	return streak
}

// Trend 最近 N 天学习趋势。
func (s *StatService) Trend(ctx context.Context, userID int64, days int) ([]map[string]interface{}, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	start := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT stat_date, question_count, correct_count, wrong_count, review_count, duration_ms
		FROM learning_statistics WHERE user_id = ? AND stat_date >= ? ORDER BY stat_date`, userID, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]map[string]int64{}
	for rows.Next() {
		var d string
		var q, cor, w, rv, dur int64
		if err := rows.Scan(&d, &q, &cor, &w, &rv, &dur); err != nil {
			break
		}
		byDate[d] = map[string]int64{"question_count": q, "correct_count": cor, "wrong_count": w, "review_count": rv, "duration_ms": dur}
	}
	out := []map[string]interface{}{}
	for i := 0; i < days; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		v, ok := byDate[d]
		if !ok {
			v = map[string]int64{}
		}
		out = append(out, map[string]interface{}{"date": d, "question_count": v["question_count"], "correct_count": v["correct_count"], "wrong_count": v["wrong_count"], "review_count": v["review_count"]})
	}
	return out, nil
}

// ChapterMastery 各章节掌握度（按答题正确率 + 题量加权）。
func (s *StatService) ChapterMastery(ctx context.Context, userID int64) ([]map[string]interface{}, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT q.chapter_id, c.name AS chapter_name, s.name AS subject_name, q.subject_id,
		       COUNT(a.id) AS attempts, SUM(a.is_correct) AS correct
		FROM question_attempts a
		JOIN canonical_questions q ON q.id = a.question_id
		JOIN chapters c ON c.id = q.chapter_id
		JOIN subjects s ON s.id = q.subject_id
		WHERE a.user_id = ? AND q.chapter_id IS NOT NULL
		GROUP BY q.chapter_id
		HAVING COUNT(a.id) >= 1
		ORDER BY s.sort_order, c.sort_order`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var chapterID, subjectID int64
		var chapterName, subjectName string
		var attempts, correct int64
		if err := rows.Scan(&chapterID, &chapterName, &subjectName, &subjectID, &attempts, &correct); err != nil {
			break
		}
		acc := float64(correct) / float64(attempts)
		// 简单掌握度：正确率 * 100，题量少时略低
		mastery := acc * 100
		if attempts < 5 {
			mastery = mastery * float64(attempts) / 5
		}
		out = append(out, map[string]interface{}{
			"chapter_id": chapterID, "chapter_name": chapterName,
			"subject_id": subjectID, "subject_name": subjectName,
			"attempts": attempts, "correct": correct, "accuracy": round1(acc * 100),
			"mastery_score": round1(mastery),
		})
	}
	return out, nil
}

// SubjectMastery 科目掌握度。
func (s *StatService) SubjectMastery(ctx context.Context, userID int64) ([]map[string]interface{}, error) {
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT q.subject_id, s.name AS subject_name, COUNT(a.id) AS attempts, SUM(a.is_correct) AS correct
		FROM question_attempts a
		JOIN canonical_questions q ON q.id = a.question_id
		JOIN subjects s ON s.id = q.subject_id
		WHERE a.user_id = ? AND q.subject_id IS NOT NULL
		GROUP BY q.subject_id ORDER BY s.sort_order`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var subjectID int64
		var subjectName string
		var attempts, correct int64
		if err := rows.Scan(&subjectID, &subjectName, &attempts, &correct); err != nil {
			break
		}
		acc := 0.0
		if attempts > 0 {
			acc = float64(correct) / float64(attempts) * 100
		}
		out = append(out, map[string]interface{}{
			"subject_id": subjectID, "subject_name": subjectName,
			"attempts": attempts, "correct": correct, "accuracy": round1(acc),
		})
	}
	return out, nil
}

// WrongTrend 错题趋势（最近 N 天）。
func (s *StatService) WrongTrend(ctx context.Context, userID int64, days int) ([]map[string]interface{}, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	start := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT date(created_at) AS d, COUNT(*) FROM question_attempts
		WHERE user_id = ? AND is_correct = 0 AND created_at >= ?
		GROUP BY d ORDER BY d`, userID, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]int64{}
	for rows.Next() {
		var d string
		var c int64
		if err := rows.Scan(&d, &c); err != nil {
			break
		}
		byDate[d] = c
	}
	out := []map[string]interface{}{}
	for i := 0; i < days; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		out = append(out, map[string]interface{}{"date": d, "wrong_count": byDate[d]})
	}
	return out, nil
}

// WeakKnowledgePoints 薄弱知识点（错题中知识点出现频率）。
func (s *StatService) WeakKnowledgePoints(ctx context.Context, userID int64, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT q.knowledge_points_json, w.wrong_count FROM wrong_questions w
		JOIN canonical_questions q ON q.id = w.question_id
		WHERE w.user_id = ? AND q.knowledge_points_json != '' AND q.knowledge_points_json != 'null'
		ORDER BY w.wrong_count DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var kpJSON string
		var wrongCount int
		if err := rows.Scan(&kpJSON, &wrongCount); err != nil {
			break
		}
		var kps []string
		if err := json.Unmarshal([]byte(kpJSON), &kps); err == nil {
			for _, kp := range kps {
				out = append(out, map[string]interface{}{"knowledge_point": kp, "wrong_count": wrongCount})
			}
		}
	}
	return out, nil
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
