package service

import (
	"context"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// QuestionService 题库 / 题目业务。
type QuestionService struct {
	repo *sqlite.Repository
}

func NewQuestionService(repo *sqlite.Repository) *QuestionService {
	return &QuestionService{repo: repo}
}

func (s *QuestionService) CreateBank(ctx context.Context, name, description, visibility string, userID int64) (*domain.QuestionBank, error) {
	if name == "" {
		return nil, api.InvalidRequest("题库名称不能为空")
	}
	if visibility == "" {
		visibility = "private"
	}
	return s.repo.CreateBank(ctx, name, description, visibility, userID)
}

func (s *QuestionService) DeleteBank(ctx context.Context, id int64) error {
	if _, err := s.repo.GetBank(ctx, id); err != nil {
		return api.NotFound("题库不存在")
	}
	return s.repo.DeleteBank(ctx, id)
}

func (s *QuestionService) CreateSubject(ctx context.Context, bankID int64, name string) (*domain.Subject, error) {
	if name == "" {
		return nil, api.InvalidRequest("科目名称不能为空")
	}
	if _, err := s.repo.GetBank(ctx, bankID); err != nil {
		return nil, api.NotFound("题库不存在")
	}
	return s.repo.CreateSubject(ctx, bankID, name)
}

func (s *QuestionService) CreateChapter(ctx context.Context, subjectID int64, parentID *int64, name string, level, sortOrder int) (*domain.Chapter, error) {
	if name == "" {
		return nil, api.InvalidRequest("章节名称不能为空")
	}
	if _, err := s.repo.GetSubject(ctx, subjectID); err != nil {
		return nil, api.NotFound("科目不存在")
	}
	if parentID != nil {
		if _, err := s.repo.GetChapter(ctx, *parentID); err != nil {
			return nil, api.NotFound("父章节不存在")
		}
	}
	if level == 0 {
		level = 1
	}
	return s.repo.CreateChapter(ctx, subjectID, parentID, name, level, sortOrder)
}

func (s *QuestionService) CreateQuestion(ctx context.Context, q *domain.Question) (*domain.Question, error) {
	if err := validateQuestion(q); err != nil {
		return nil, err
	}
	if q.Status == "" {
		q.Status = domain.QuestionStatusPublished
	}
	if q.Difficulty == 0 {
		q.Difficulty = 3
	}
	created, err := s.repo.CreateQuestion(ctx, q)
	if err != nil {
		return nil, api.Internal("创建题目失败", err)
	}
	_ = s.repo.BuildQuestionFTS(ctx, created)
	return created, nil
}

func (s *QuestionService) UpdateQuestion(ctx context.Context, id int64, q *domain.Question) error {
	if err := validateQuestion(q); err != nil {
		return err
	}
	if _, err := s.repo.GetQuestion(ctx, id); err != nil {
		return api.NotFound("题目不存在")
	}
	if err := s.repo.UpdateQuestion(ctx, id, q); err != nil {
		return api.Internal("更新题目失败", err)
	}
	updated, err := s.repo.GetQuestion(ctx, id)
	if err != nil {
		return nil
	}
	_ = s.repo.BuildQuestionFTS(ctx, updated)
	return nil
}

func (s *QuestionService) DeleteQuestion(ctx context.Context, id int64) error {
	if _, err := s.repo.GetQuestion(ctx, id); err != nil {
		return api.NotFound("题目不存在")
	}
	if err := s.repo.DeleteQuestion(ctx, id); err != nil {
		return api.Internal("删除题目失败", err)
	}
	return nil
}

func validateQuestion(q *domain.Question) error {
	if q.BankID == 0 {
		return api.InvalidRequest("缺少题库")
	}
	if q.Stem == "" {
		return api.InvalidRequest("题干不能为空")
	}
	switch q.Type {
	case domain.QuestionTypeSingleChoice, domain.QuestionTypeMultipleChoice, domain.QuestionTypeTrueFalse:
		if len(q.Answer) == 0 {
			return api.InvalidRequest("缺少答案")
		}
	case domain.QuestionTypeFillBlank:
		if len(q.Answer) == 0 {
			return api.InvalidRequest("缺少答案")
		}
	case "":
		return api.InvalidRequest("缺少题型")
	}
	return nil
}

var _ = fmt.Sprintf
