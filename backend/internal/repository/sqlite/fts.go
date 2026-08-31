package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// BuildQuestionFTS 将题目写入 FTS 索引（question 源类型）。
func (r *Repository) BuildQuestionFTS(ctx context.Context, q *domain.Question) error {
	content := strings.Join([]string{
		q.Stem,
		joinOptions(q.Options),
		q.OriginalAnalysis,
		q.AIEnhancedAnalysis,
		strings.Join(q.KnowledgePoints, " "),
	}, "\n")
	return r.UpsertRAGDocument(ctx, "question", q.ID, nil, 0, q.BankID, q.SubjectID, q.ChapterID, "",
		q.Difficulty, q.Type, content)
}

func joinOptions(opts []domain.QuestionOption) string {
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		parts = append(parts, o.Key+". "+o.Content)
	}
	return strings.Join(parts, " | ")
}

// IndexDocumentChunk 将文档 chunk 写入 RAG 索引。
func (r *Repository) IndexDocumentChunk(ctx context.Context, docID int64, page int, bankID int64, subjectID, chapterID *int64, text string) error {
	return r.UpsertRAGDocument(ctx, "document_chunk", 0, &docID, page, bankID, subjectID, chapterID, "",
		0, "", text)
}

var _ = fmt.Sprintf
