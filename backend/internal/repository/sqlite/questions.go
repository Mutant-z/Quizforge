package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// marshalJSON 安全序列化。
func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// CreateQuestion 创建题目（含选项）。
func (r *Repository) CreateQuestion(ctx context.Context, q *domain.Question) (*domain.Question, error) {
	optionsJSON := marshalJSON(q.Options)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO canonical_questions
		(bank_id, subject_id, chapter_id, type, stem, options_json, answer_json, original_analysis, ai_enhanced_analysis, difficulty, knowledge_points_json, quality_score, status, source_hash, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		q.BankID, q.SubjectID, q.ChapterID, q.Type, q.Stem, optionsJSON, marshalJSON(q.Answer),
		q.OriginalAnalysis, q.AIEnhancedAnalysis, q.Difficulty, marshalJSON(q.KnowledgePoints),
		q.QualityScore, q.Status, q.SourceHash)
	if err != nil {
		return nil, fmt.Errorf("insert question: %w", err)
	}
	id, _ := res.LastInsertId()
	q.ID = id
	// 更新题库计数
	_, _ = r.db.ExecContext(ctx,
		`UPDATE question_banks SET question_count = (SELECT COUNT(*) FROM canonical_questions WHERE bank_id = ?), updated_at = datetime('now') WHERE id = ?`,
		q.BankID, q.BankID)
	return r.GetQuestion(ctx, id)
}

func (r *Repository) UpdateQuestion(ctx context.Context, id int64, q *domain.Question) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE canonical_questions SET
			subject_id = ?, chapter_id = ?, type = ?, stem = ?, options_json = ?, answer_json = ?,
			original_analysis = ?, ai_enhanced_analysis = ?, difficulty = ?, knowledge_points_json = ?,
			quality_score = ?, status = ?, source_hash = ?, version = version + 1,
			updated_at = datetime('now')
		WHERE id = ?`,
		q.SubjectID, q.ChapterID, q.Type, q.Stem, marshalJSON(q.Options), marshalJSON(q.Answer),
		q.OriginalAnalysis, q.AIEnhancedAnalysis, q.Difficulty, marshalJSON(q.KnowledgePoints),
		q.QualityScore, q.Status, q.SourceHash, id)
	if err != nil {
		return fmt.Errorf("update question: %w", err)
	}
	return nil
}

func (r *Repository) DeleteQuestion(ctx context.Context, id int64) error {
	// 先取 bank_id 用于更新计数
	var bankID int64
	_ = r.db.QueryRowContext(ctx, `SELECT bank_id FROM canonical_questions WHERE id = ?`, id).Scan(&bankID)
	if _, err := r.db.ExecContext(ctx, `DELETE FROM canonical_questions WHERE id = ?`, id); err != nil {
		return err
	}
	if bankID > 0 {
		_, _ = r.db.ExecContext(ctx,
			`UPDATE question_banks SET question_count = (SELECT COUNT(*) FROM canonical_questions WHERE bank_id = ?), updated_at = datetime('now') WHERE id = ?`,
			bankID, bankID)
	}
	return nil
}

func (r *Repository) GetQuestion(ctx context.Context, id int64) (*domain.Question, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, bank_id, subject_id, chapter_id, type, stem, options_json, answer_json,
		       original_analysis, ai_enhanced_analysis, difficulty, knowledge_points_json, quality_score, status, source_hash, version, created_at, updated_at
		FROM canonical_questions WHERE id = ?`, id)
	return scanQuestion(row)
}

// ListQuestions 分页 + 过滤查询题目。
// filter: bank_id / bank_ids / subject_id / chapter_id / type / status / keyword。
func (r *Repository) ListQuestions(ctx context.Context, f QuestionFilter) ([]*domain.Question, int64, error) {
	where, args := questionFilterWhere(f)
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_questions q WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := f.Page
	pageSize := f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	order := "q.id"
	if f.OrderRandom {
		order = "RANDOM()"
	}

	query := `SELECT q.id, q.bank_id, q.subject_id, q.chapter_id, q.type, q.stem, q.options_json, q.answer_json,
	       q.original_analysis, q.ai_enhanced_analysis, q.difficulty, q.knowledge_points_json, q.quality_score, q.status, q.source_hash, q.version, q.created_at, q.updated_at
	FROM canonical_questions q WHERE ` + whereSQL + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	questions := []*domain.Question{}
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			return nil, 0, err
		}
		questions = append(questions, q)
	}
	return questions, total, rows.Err()
}

// ListQuestionIDs 返回满足筛选条件的题目 ID。刷题会话只需要先固定题目快照，
// 使用该方法可以避免把完整题目内容和分页上限带入抽题流程。
func (r *Repository) ListQuestionIDs(ctx context.Context, f QuestionFilter) ([]int64, error) {
	where, args := questionFilterWhere(f)
	query := `SELECT q.id FROM canonical_questions q WHERE ` + strings.Join(where, " AND ")
	if f.OrderRandom {
		query += " ORDER BY RANDOM()"
	} else {
		query += " ORDER BY q.id"
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
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

func questionFilterWhere(f QuestionFilter) ([]string, []interface{}) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.RequireAnswer || f.RequireOptions {
		for _, condition := range playableQuestionConditions("q", f.RequireAnswer, f.RequireOptions) {
			where = append(where, condition)
		}
	}
	if f.BankID != nil {
		where = append(where, "q.bank_id = ?")
		args = append(args, *f.BankID)
	}
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
		where = append(where, "q.status = ?")
		args = append(args, f.Status)
	}
	if f.Keyword != "" {
		where = append(where, "q.stem LIKE ?")
		args = append(args, "%"+f.Keyword+"%")
	}
	if len(f.IDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(f.IDs)), ",")
		where = append(where, "q.id IN ("+placeholders+")")
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	return where, args
}

// QuestionFilter 题目查询过滤。
type QuestionFilter struct {
	BankID         *int64
	BankIDs        []int64
	SubjectID      *int64
	ChapterID      *int64
	Type           string
	Status         string
	Keyword        string
	IDs            []int64
	Page           int
	PageSize       int
	OrderRandom    bool
	UserID         int64 // 用户相关字段填充（收藏/错题/笔记）
	RequireAnswer  bool  // 仅返回包含至少一个非空答案的题目
	RequireOptions bool  // 选择题还必须包含可展示的选项
}

// playableQuestionConditions 返回刷题时必须满足的题目完整性条件。
// 题库管理仍可查询不完整题目，便于用户修复数据；只有刷题抽题会启用这些条件。
func playableQuestionConditions(alias string, requireAnswer, requireOptions bool) []string {
	conditions := []string{}
	if requireAnswer {
		answerJSON := fmt.Sprintf(`CASE
			WHEN json_valid(COALESCE(%s.answer_json, '[]')) = 1 THEN
				CASE WHEN json_type(%s.answer_json) = 'array' THEN %s.answer_json ELSE '[]' END
			ELSE '[]'
		END`, alias, alias, alias)
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
				SELECT 1 FROM json_each(%s) AS answer_item
				WHERE trim(CAST(answer_item.value AS TEXT)) <> ''
			)`, answerJSON))
	}
	if requireOptions {
		optionsJSON := fmt.Sprintf(`CASE
			WHEN json_valid(COALESCE(%s.options_json, '[]')) = 1 THEN
				CASE WHEN json_type(%s.options_json) = 'array' THEN %s.options_json ELSE '[]' END
			ELSE '[]'
		END`, alias, alias, alias)
		conditions = append(conditions, fmt.Sprintf(`
			(%s.type NOT IN ('single_choice', 'multiple_choice', 'true_false') OR EXISTS (
					SELECT 1 FROM json_each(%s) AS option_item
					WHERE trim(COALESCE(json_extract(option_item.value, '$.key'), '')) <> ''
					  AND trim(COALESCE(json_extract(option_item.value, '$.content'), '')) <> ''
				))`, alias, optionsJSON))
	}
	return conditions
}

// CountQuestions 统计题目数。
func (r *Repository) CountQuestions(ctx context.Context, bankID, subjectID, chapterID *int64) (int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if bankID != nil {
		where = append(where, "bank_id = ?")
		args = append(args, *bankID)
	}
	if subjectID != nil {
		where = append(where, "subject_id = ?")
		args = append(args, *subjectID)
	}
	if chapterID != nil {
		where = append(where, "chapter_id = ?")
		args = append(args, *chapterID)
	}
	var c int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_questions WHERE `+strings.Join(where, " AND "), args...).Scan(&c)
	return c, err
}

func scanQuestion(row interface{ Scan(...any) error }) (*domain.Question, error) {
	var q domain.Question
	var subjectID, chapterID sql.NullInt64
	var optionsJSON, answerJSON, kpJSON, origAnalysis, aiAnalysis sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&q.ID, &q.BankID, &subjectID, &chapterID, &q.Type, &q.Stem, &optionsJSON, &answerJSON,
		&origAnalysis, &aiAnalysis, &q.Difficulty, &kpJSON, &q.QualityScore, &q.Status, &q.SourceHash, &q.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if subjectID.Valid {
		q.SubjectID = &subjectID.Int64
	}
	if chapterID.Valid {
		q.ChapterID = &chapterID.Int64
	}
	q.Options = []domain.QuestionOption{}
	json.Unmarshal([]byte(optionsJSON.String), &q.Options)
	q.Answer = []string{}
	json.Unmarshal([]byte(answerJSON.String), &q.Answer)
	q.KnowledgePoints = []string{}
	json.Unmarshal([]byte(kpJSON.String), &q.KnowledgePoints)
	q.OriginalAnalysis = origAnalysis.String
	q.AIEnhancedAnalysis = aiAnalysis.String
	q.CreatedAt = parseSQLiteTime(createdAt)
	q.UpdatedAt = parseSQLiteTime(updatedAt)
	return &q, nil
}
