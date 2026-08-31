package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/parser"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
)

// cryptFrom 由配置构造 AES 工具（解密 Provider Key）。
func cryptFrom(cfg *config.Config) *security.AESCrypt {
	return security.NewAESCrypt(cfg.Auth.MasterKey)
}

// Persister 校验 + 去重 + 入库。
type Persister struct {
	repo *sqlite.Repository
	cfg  *config.Config
}

func NewPersister(repo *sqlite.Repository, cfg *config.Config) *Persister {
	return &Persister{repo: repo, cfg: cfg}
}

// Persist 将 candidate 提取结果写入规范题库（幂等 + 去重 + 冲突检测）。
func (p *Persister) Persist(ctx context.Context, cand *parser.Candidate, doc *domain.Document, bankID *int64, subjectID *int64) error {
	if cand == nil || doc == nil || bankID == nil {
		return fmt.Errorf("persist: nil input (cand=%v doc=%v bankID=%v)", cand == nil, doc == nil, bankID == nil)
	}
	var extracted ExtractedQuestion
	if err := json.Unmarshal([]byte(cand.ExtractedJSON), &extracted); err != nil {
		_ = p.repo.SetCandidateError(ctx, cand.ID, "QUESTION_JSON_INVALID", err.Error())
		return err
	}
	// 校验（本地规则，docs/04 §5）
	if issues := validateExtracted(&extracted); len(issues) > 0 {
		_ = p.repo.SetCandidateError(ctx, cand.ID, "VALIDATION_FAILED", strings.Join(issues, "; "))
		return fmt.Errorf("validation failed: %s", strings.Join(issues, "; "))
	}

	// 低置信度 → 人工审核队列
	if extracted.Confidence < 0.8 {
		_ = p.repo.SetCandidateStatus(ctx, cand.ID, "needs_review")
		return nil
	}

	// 去重：精确 hash（source_hash 唯一）→ 已存在则标记 duplicate
	normStem := parser.NormalizeText(extracted.Stem)
	hash := hashString(normStem)
	existingID, err := p.repo.FindQuestionBySourceHash(ctx, hash)
	if err == nil && existingID > 0 {
		// 答案冲突检测
		if conflict := p.detectAnswerConflict(ctx, existingID, &extracted); conflict {
			_ = p.repo.SetCandidateStatus(ctx, cand.ID, "conflict")
			_ = p.repo.CreateConflict(ctx, existingID, "answer", cand.RawText, extractedJSONAnswer(&extracted), "")
			return nil
		}
		// 完全重复：绑定来源
		_ = p.repo.CreateQuestionSource(ctx, existingID, cand, doc, strings.Join(extracted.Answer, ","), extracted.Analysis)
		_ = p.repo.SetCandidateStatus(ctx, cand.ID, "duplicate")
		return nil
	}

	// 新题入库
	q := &domain.Question{
		BankID:           *bankID,
		SubjectID:        subjectID,
		Type:             mapType(extracted.Type),
		Stem:             extracted.Stem,
		Options:          toOptions(extracted.Options),
		Answer:           extracted.Answer,
		OriginalAnalysis: extracted.Analysis,
		Difficulty:       extracted.Difficulty,
		KnowledgePoints:  extracted.KnowledgePoints,
		QualityScore:     extracted.Confidence,
		Status:           domain.QuestionStatusPublished,
		SourceHash:       hash,
	}
	if q.Difficulty <= 0 {
		q.Difficulty = 3
	}
	created, err := p.repo.CreateQuestion(ctx, q)
	if err != nil {
		_ = p.repo.SetCandidateError(ctx, cand.ID, "PIPELINE_FAILED", err.Error())
		return err
	}
	_ = p.repo.CreateQuestionSource(ctx, created.ID, cand, doc, strings.Join(extracted.Answer, ","), extracted.Analysis)
	_ = p.repo.SetCandidateStatus(ctx, cand.ID, "persisted")
	// FTS 索引
	_ = p.repo.BuildQuestionFTS(ctx, created)
	return nil
}

// validateExtracted 本地规则校验。
func validateExtracted(e *ExtractedQuestion) []string {
	var issues []string
	if strings.TrimSpace(e.Stem) == "" {
		issues = append(issues, "题干为空")
	}
	switch e.Type {
	case "single_choice", "multiple_choice":
		if len(e.Options) < 2 {
			issues = append(issues, "选项不足")
		}
		if len(e.Answer) == 0 {
			issues = append(issues, "缺少答案")
		}
	case "true_false":
		if len(e.Answer) == 0 {
			issues = append(issues, "缺少答案")
		}
	case "fill_blank":
		if len(e.Answer) == 0 {
			issues = append(issues, "缺少答案")
		}
	case "short_answer", "essay":
	default:
		issues = append(issues, "题型未知: "+e.Type)
	}
	return issues
}

func (p *Persister) detectAnswerConflict(ctx context.Context, existingID int64, e *ExtractedQuestion) bool {
	existing, err := p.repo.GetQuestion(ctx, existingID)
	if err != nil {
		return false
	}
	old := normalizeAnswerSet(existing.Answer)
	neu := normalizeAnswerSet(e.Answer)
	if len(old) == 0 || len(neu) == 0 {
		return false
	}
	if len(old) != len(neu) {
		return true
	}
	for i := range old {
		if old[i] != neu[i] {
			return true
		}
	}
	return false
}

func mapType(t string) string {
	switch t {
	case "single_choice":
		return domain.QuestionTypeSingleChoice
	case "multiple_choice":
		return domain.QuestionTypeMultipleChoice
	case "true_false":
		return domain.QuestionTypeTrueFalse
	case "fill_blank":
		return domain.QuestionTypeFillBlank
	case "short_answer":
		return domain.QuestionTypeShortAnswer
	case "essay":
		return domain.QuestionTypeEssay
	case "code":
		return domain.QuestionTypeCode
	default:
		return domain.QuestionTypeSingleChoice
	}
}

func toOptions(opts []struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}) []domain.QuestionOption {
	out := make([]domain.QuestionOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, domain.QuestionOption{Key: o.Key, Content: o.Content})
	}
	return out
}

func extractedJSONAnswer(e *ExtractedQuestion) string {
	return strings.Join(e.Answer, ",")
}

func normalizeAnswerSet(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, normalizeAlpha(it))
	}
	sortStrings(out)
	return out
}

func normalizeAlpha(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func hashString(s string) string {
	return hashHex(s)
}
