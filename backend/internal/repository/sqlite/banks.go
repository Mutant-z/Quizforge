package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// ---- QuestionBank ----

func (r *Repository) CreateBank(ctx context.Context, name, description, visibility string, userID int64) (*domain.QuestionBank, error) {
	if visibility == "" {
		visibility = "private"
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO question_banks (name, description, visibility, created_by) VALUES (?, ?, ?, ?)`,
		name, description, visibility, userID)
	if err != nil {
		return nil, fmt.Errorf("insert bank: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetBank(ctx, id)
}

func (r *Repository) GetBank(ctx context.Context, id int64) (*domain.QuestionBank, error) {
	return r.getBankWhere(ctx, `b.id = ?`, id)
}

// GetBankForUser 仅返回指定用户创建的题库。管理员或内部任务需要跨用户读取时，
// 应继续显式使用 GetBank，避免把内部数据处理误绑定到请求用户。
func (r *Repository) GetBankForUser(ctx context.Context, id, userID int64) (*domain.QuestionBank, error) {
	return r.getBankWhere(ctx, `b.id = ? AND b.created_by = ?`, id, userID)
}

func (r *Repository) getBankWhere(ctx context.Context, where string, args ...interface{}) (*domain.QuestionBank, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT b.id, b.name, b.description, b.cover, b.visibility, b.status, b.question_count, b.created_by, b.created_at, b.updated_at,
		       (SELECT COUNT(*) FROM subjects s WHERE s.bank_id = b.id) AS subject_count
		FROM question_banks b WHERE `+where, args...)
	var b domain.QuestionBank
	var desc, cover sql.NullString
	var createdBy sql.NullInt64
	var createdAt, updatedAt string
	if err := row.Scan(&b.ID, &b.Name, &desc, &cover, &b.Visibility, &b.Status, &b.QuestionCount, &createdBy, &createdAt, &updatedAt, &b.SubjectCount); err != nil {
		return nil, err
	}
	b.Description = desc.String
	b.Cover = cover.String
	b.CreatedBy = createdBy.Int64
	b.CreatedAt = parseSQLiteTime(createdAt)
	b.UpdatedAt = parseSQLiteTime(updatedAt)
	return &b, nil
}

func (r *Repository) ListBanks(ctx context.Context, page, pageSize int) ([]*domain.QuestionBank, int64, error) {
	return r.ListBanksFiltered(ctx, page, pageSize, "")
}

// ListBanksFiltered 分页查询题库，并支持名称/描述搜索。
func (r *Repository) ListBanksFiltered(ctx context.Context, page, pageSize int, keyword string) ([]*domain.QuestionBank, int64, error) {
	return r.listBanksFiltered(ctx, page, pageSize, keyword, nil)
}

// ListBanksForUser 分页查询用户自己的题库。visibility 不参与普通学习者的
// 数据范围判断：题库归属由 created_by 决定，避免公开标记造成跨用户泄露。
func (r *Repository) ListBanksForUser(ctx context.Context, userID int64, page, pageSize int, keyword string) ([]*domain.QuestionBank, int64, error) {
	return r.listBanksFiltered(ctx, page, pageSize, keyword, &userID)
}

func (r *Repository) listBanksFiltered(ctx context.Context, page, pageSize int, keyword string, ownerID *int64) ([]*domain.QuestionBank, int64, error) {
	whereParts := []string{"1=1"}
	countArgs := []interface{}{}
	queryArgs := []interface{}{}
	if ownerID != nil {
		whereParts = append(whereParts, "b.created_by = ?")
		countArgs = append(countArgs, *ownerID)
		queryArgs = append(queryArgs, *ownerID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		whereParts = append(whereParts, "(b.name LIKE ? OR b.description LIKE ?)")
		pattern := "%" + keyword + "%"
		countArgs = append(countArgs, pattern, pattern)
		queryArgs = append(queryArgs, pattern, pattern)
	}
	where := strings.Join(whereParts, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM question_banks b WHERE `+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.name, b.description, b.cover, b.visibility, b.status, b.question_count, b.created_by, b.created_at, b.updated_at,
		       (SELECT COUNT(*) FROM subjects s WHERE s.bank_id = b.id) AS subject_count
		FROM question_banks b WHERE `+where+`
		ORDER BY b.id DESC LIMIT ? OFFSET ?`, append(queryArgs, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	banks := []*domain.QuestionBank{}
	for rows.Next() {
		var b domain.QuestionBank
		var desc, cover sql.NullString
		var createdBy sql.NullInt64
		var createdAt, updatedAt string
		if err := rows.Scan(&b.ID, &b.Name, &desc, &cover, &b.Visibility, &b.Status, &b.QuestionCount, &createdBy, &createdAt, &updatedAt, &b.SubjectCount); err != nil {
			return nil, 0, err
		}
		b.Description = desc.String
		b.Cover = cover.String
		b.CreatedBy = createdBy.Int64
		b.CreatedAt = parseSQLiteTime(createdAt)
		b.UpdatedAt = parseSQLiteTime(updatedAt)
		banks = append(banks, &b)
	}
	return banks, total, rows.Err()
}

func (r *Repository) DeleteBank(ctx context.Context, id int64) error {
	// 开启事务，严格按照外键依赖拓扑顺序清理题库及所有关联数据
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. 查询该题库关联的所有题目 ID
	qRows, err := tx.QueryContext(ctx, `SELECT id FROM canonical_questions WHERE bank_id = ?`, id)
	if err != nil {
		return fmt.Errorf("query questions: %w", err)
	}
	var questionIDs []int64
	for qRows.Next() {
		var qID int64
		if err := qRows.Scan(&qID); err == nil {
			questionIDs = append(questionIDs, qID)
		}
	}
	qRows.Close()

	// 2. 清理题目衍生数据
	for _, qID := range questionIDs {
		// 清理错题复习记录 (review_records -> wrong_questions)
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM review_records WHERE wrong_question_id IN (
				SELECT id FROM wrong_questions WHERE question_id = ?
			)`, qID); err != nil {
			return fmt.Errorf("delete review_records: %w", err)
		}
		// 清理错题
		if _, err := tx.ExecContext(ctx, `DELETE FROM wrong_questions WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete wrong_questions: %w", err)
		}
		// 清理做题记录
		if _, err := tx.ExecContext(ctx, `DELETE FROM question_attempts WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete question_attempts: %w", err)
		}
		// 清理笔记
		if _, err := tx.ExecContext(ctx, `DELETE FROM question_notes WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete question_notes: %w", err)
		}
		// 清理收藏
		if _, err := tx.ExecContext(ctx, `DELETE FROM favorites WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete favorites: %w", err)
		}
		// 清理选项
		if _, err := tx.ExecContext(ctx, `DELETE FROM question_options WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete question_options: %w", err)
		}
		// 清理题目知识点关联
		if _, err := tx.ExecContext(ctx, `DELETE FROM question_knowledge_points WHERE question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete question_knowledge_points: %w", err)
		}
		// 清理溯源记录
		if _, err := tx.ExecContext(ctx, `DELETE FROM question_sources WHERE canonical_question_id = ?`, qID); err != nil {
			return fmt.Errorf("delete question_sources: %w", err)
		}
	}

	// 3. 清理练习会话 (practice_sessions) 及其答题记录
	if _, err := tx.ExecContext(ctx, `DELETE FROM question_attempts WHERE session_id IN (SELECT id FROM practice_sessions WHERE bank_id = ?)`, id); err != nil {
		return fmt.Errorf("delete session attempts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM practice_sessions WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("delete practice_sessions: %w", err)
	}

	// 4. 清理题目本身 (此时没有任何下级记录和章节科目外键阻碍)
	if _, err := tx.ExecContext(ctx, `DELETE FROM canonical_questions WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("delete canonical_questions: %w", err)
	}

	// 5. 清理知识点 (knowledge_points)
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_points WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("delete knowledge_points: %w", err)
	}

	// 6. 清理章节 (从底层子章节向上级级联清理)
	rows, err := tx.QueryContext(ctx, `SELECT id FROM subjects WHERE bank_id = ?`, id)
	if err != nil {
		return fmt.Errorf("query subjects: %w", err)
	}
	var subjectIDs []int64
	for rows.Next() {
		var sID int64
		if err := rows.Scan(&sID); err == nil {
			subjectIDs = append(subjectIDs, sID)
		}
	}
	rows.Close()

	for _, sID := range subjectIDs {
		// 先删子章节 (level 3, 2) 再删顶级章节 (level 1)
		if _, err := tx.ExecContext(ctx, `DELETE FROM chapters WHERE subject_id = ? AND parent_id IS NOT NULL`, sID); err != nil {
			return fmt.Errorf("delete child chapters: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chapters WHERE subject_id = ?`, sID); err != nil {
			return fmt.Errorf("delete root chapters: %w", err)
		}
	}

	// 7. 清理科目
	if _, err := tx.ExecContext(ctx, `DELETE FROM subjects WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("delete subjects: %w", err)
	}

	// 8. 解除用户的默认题库关联
	if _, err := tx.ExecContext(ctx, `UPDATE users SET default_bank_id = NULL WHERE default_bank_id = ?`, id); err != nil {
		return fmt.Errorf("clear users default_bank_id: %w", err)
	}

	// 9. 解除文档和导入任务关联
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET bank_id = NULL WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("update documents bank_id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET bank_id = NULL WHERE bank_id = ?`, id); err != nil {
		return fmt.Errorf("update import_jobs bank_id: %w", err)
	}

	// 10. 清理 RAG 相关索引文档
	_, _ = tx.ExecContext(ctx, `DELETE FROM rag_documents WHERE bank_id = ?`, id)

	// 11. 删除题库记录本身
	if _, err := tx.ExecContext(ctx, `DELETE FROM question_banks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete question_bank: %w", err)
	}

	return tx.Commit()
}

// ---- Subject ----

func (r *Repository) CreateSubject(ctx context.Context, bankID int64, name string) (*domain.Subject, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO subjects (bank_id, name) VALUES (?, ?)`, bankID, name)
	if err != nil {
		return nil, fmt.Errorf("insert subject: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetSubject(ctx, id)
}

func (r *Repository) GetSubject(ctx context.Context, id int64) (*domain.Subject, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, bank_id, name, sort_order, created_at FROM subjects WHERE id = ?`, id)
	var s domain.Subject
	var createdAt string
	if err := row.Scan(&s.ID, &s.BankID, &s.Name, &s.SortOrder, &createdAt); err != nil {
		return nil, err
	}
	s.CreatedAt = parseSQLiteTime(createdAt)
	return &s, nil
}

// GetSubjectForUser 仅返回用户自己题库下的科目。
func (r *Repository) GetSubjectForUser(ctx context.Context, id, userID int64) (*domain.Subject, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT s.id, s.bank_id, s.name, s.sort_order, s.created_at
		FROM subjects s
		JOIN question_banks b ON b.id = s.bank_id
		WHERE s.id = ? AND b.created_by = ?`, id, userID)
	var s domain.Subject
	var createdAt string
	if err := row.Scan(&s.ID, &s.BankID, &s.Name, &s.SortOrder, &createdAt); err != nil {
		return nil, err
	}
	s.CreatedAt = parseSQLiteTime(createdAt)
	return &s, nil
}

func (r *Repository) ListSubjects(ctx context.Context, bankID int64) ([]*domain.Subject, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, bank_id, name, sort_order, created_at FROM subjects WHERE bank_id = ? ORDER BY sort_order, id`, bankID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subjects := []*domain.Subject{}
	for rows.Next() {
		var s domain.Subject
		var createdAt string
		if err := rows.Scan(&s.ID, &s.BankID, &s.Name, &s.SortOrder, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt = parseSQLiteTime(createdAt)
		subjects = append(subjects, &s)
	}
	return subjects, rows.Err()
}

// ---- Chapter ----

func (r *Repository) CreateChapter(ctx context.Context, subjectID int64, parentID *int64, name string, level, sortOrder int) (*domain.Chapter, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO chapters (subject_id, parent_id, name, normalized_name, level, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
		subjectID, parentID, name, normalizeName(name), level, sortOrder)
	if err != nil {
		return nil, fmt.Errorf("insert chapter: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetChapter(ctx, id)
}

func (r *Repository) GetChapter(ctx context.Context, id int64) (*domain.Chapter, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.subject_id, c.parent_id, c.name, c.normalized_name, c.level, c.sort_order, c.status,
		       (SELECT COUNT(*) FROM canonical_questions q WHERE q.chapter_id = c.id) AS question_count
		FROM chapters c WHERE c.id = ?`, id)
	return scanChapter(row)
}

// GetChapterForUser 仅返回用户自己题库下的章节。
func (r *Repository) GetChapterForUser(ctx context.Context, id, userID int64) (*domain.Chapter, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.subject_id, c.parent_id, c.name, c.normalized_name, c.level, c.sort_order, c.status,
		       (SELECT COUNT(*) FROM canonical_questions q WHERE q.chapter_id = c.id) AS question_count
		FROM chapters c
		JOIN subjects s ON s.id = c.subject_id
		JOIN question_banks b ON b.id = s.bank_id
		WHERE c.id = ? AND b.created_by = ?`, id, userID)
	return scanChapter(row)
}

func (r *Repository) ListChapters(ctx context.Context, subjectID int64) ([]*domain.Chapter, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.subject_id, c.parent_id, c.name, c.normalized_name, c.level, c.sort_order, c.status,
		       (SELECT COUNT(*) FROM canonical_questions q WHERE q.chapter_id = c.id) AS question_count
		FROM chapters c WHERE c.subject_id = ? ORDER BY c.sort_order, c.id`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chapters := []*domain.Chapter{}
	for rows.Next() {
		c, err := scanChapter(rows)
		if err != nil {
			return nil, err
		}
		chapters = append(chapters, c)
	}
	return chapters, rows.Err()
}

// ChapterTree 返回带 children 的章节树（level1 章 → level2 节）。
func (r *Repository) ChapterTree(ctx context.Context, subjectID int64) ([]*domain.Chapter, error) {
	chapters, err := r.ListChapters(ctx, subjectID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*domain.Chapter{}
	for _, c := range chapters {
		byID[c.ID] = c
	}
	var roots []*domain.Chapter
	for _, c := range chapters {
		if c.ParentID != nil {
			if parent, ok := byID[*c.ParentID]; ok {
				parent.Children = append(parent.Children, c)
				continue
			}
		}
		roots = append(roots, c)
	}
	return roots, nil
}

func (r *Repository) UpdateChapter(ctx context.Context, id int64, name string) (*domain.Chapter, error) {
	if _, err := r.db.ExecContext(ctx, `UPDATE chapters SET name = ?, normalized_name = ? WHERE id = ?`, name, normalizeName(name), id); err != nil {
		return nil, err
	}
	return r.GetChapter(ctx, id)
}

func (r *Repository) DeleteChapter(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM chapters WHERE id = ?`, id)
	return err
}

func (r *Repository) FindChapterByNormalized(ctx context.Context, subjectID int64, normName string) (*domain.Chapter, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, subject_id, parent_id, name, normalized_name, level, sort_order, status, 0 FROM chapters WHERE subject_id = ? AND normalized_name = ? AND status='active'`, subjectID, normName)
	c, err := scanChapter(row)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func scanChapter(row interface{ Scan(...any) error }) (*domain.Chapter, error) {
	var c domain.Chapter
	var parentID sql.NullInt64
	var normName sql.NullString
	var qCount int64
	if err := row.Scan(&c.ID, &c.SubjectID, &parentID, &c.Name, &normName, &c.Level, &c.SortOrder, &c.Status, &qCount); err != nil {
		return nil, err
	}
	if parentID.Valid {
		c.ParentID = &parentID.Int64
	}
	c.NormalizedName = normName.String
	c.QuestionCount = qCount
	return &c, nil
}

func normalizeName(name string) string {
	// 章节名归一化：去空白，统一小写（用于同义章节匹配）
	var out []rune
	for _, ch := range name {
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			continue
		default:
			out = append(out, ch)
		}
	}
	return string(out)
}

// ---- KnowledgePoint ----

func (r *Repository) CreateKnowledgePoint(ctx context.Context, bankID int64, subjectID, chapterID *int64, name string) (*domain.KnowledgePoint, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO knowledge_points (bank_id, subject_id, chapter_id, name) VALUES (?, ?, ?, ?)`,
		bankID, subjectID, chapterID, name)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge point: %w", err)
	}
	id, _ := res.LastInsertId()
	kp := &domain.KnowledgePoint{ID: id, BankID: bankID, SubjectID: subjectID, ChapterID: chapterID, Name: name}
	return kp, nil
}

func (r *Repository) FindKnowledgePoint(ctx context.Context, bankID int64, name string) (*domain.KnowledgePoint, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, bank_id, subject_id, chapter_id, name, created_at FROM knowledge_points WHERE bank_id = ? AND name = ?`, bankID, name)
	var kp domain.KnowledgePoint
	var sub, chap sql.NullInt64
	var createdAt string
	if err := row.Scan(&kp.ID, &kp.BankID, &sub, &chap, &kp.Name, &createdAt); err != nil {
		return nil, err
	}
	if sub.Valid {
		kp.SubjectID = &sub.Int64
	}
	if chap.Valid {
		kp.ChapterID = &chap.Int64
	}
	kp.CreatedAt = parseSQLiteTime(createdAt)
	return &kp, nil
}

func parseSQLiteTime(s string) (t time.Time) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return parsed
		}
	}
	return
}

var _ = json.Marshal
