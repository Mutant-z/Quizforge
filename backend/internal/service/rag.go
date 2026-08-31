package service

import (
	"context"
	"strings"

	"github.com/quiztrace/quiztrace/internal/ai/embedding"
	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/ai/rerank"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// RAGService 混合检索（docs/07 §4）。
type RAGService struct {
	repo     *sqlite.Repository
	cfg      *config.Config
	registry *provider.Registry
	embedder *Embedder
}

func NewRAGService(repo *sqlite.Repository, cfg *config.Config) *RAGService {
	return &RAGService{
		repo:     repo,
		cfg:      cfg,
		registry: provider.NewRegistry(repo.DB(), cryptFrom(cfg)),
		embedder: NewEmbedder(repo, cfg),
	}
}

// SearchRequest 检索请求。
type SearchRequest struct {
	Query      string `json:"query" binding:"required"`
	BankID     *int64 `json:"bank_id"`
	SubjectID  *int64 `json:"subject_id"`
	ChapterID  *int64 `json:"chapter_id"`
	SourceType string `json:"source_type"`
	TopK       int    `json:"top_k"`
	UseVector  bool   `json:"use_vector"`
	UserID     int64  `json:"-"` // 普通用户检索范围，由认证上下文注入
}

// SearchResult 检索结果。
type SearchResult struct {
	Items []sqlite.RAGDocument `json:"items"`
	Total int                  `json:"total"`
}

// Search 混合检索：FTS + Vector → 合并 → 可选 rerank。
func (s *RAGService) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	topK := req.TopK
	if topK <= 0 || topK > 20 {
		topK = s.cfg.RAG.FinalTopK
	}

	// 1. FTS 关键词检索（trigram）
	var ftsItems []sqlite.RAGDocument
	var err error
	if req.UserID > 0 {
		ftsItems, err = s.repo.SearchFTSForUser(ctx, req.Query, s.cfg.RAG.FTSTopK, req.BankID, req.SubjectID, req.ChapterID, req.UserID)
	} else {
		ftsItems, err = s.repo.SearchFTS(ctx, req.Query, s.cfg.RAG.FTSTopK, req.BankID, req.SubjectID, req.ChapterID)
	}
	if err != nil {
		return nil, err
	}

	// 2. 向量检索（可选，无 embedding 配置时安全跳过）
	vectorItems := []sqlite.RAGDocument{}
	if req.UseVector {
		if vec, err := s.embedder.EmbedText(ctx, req.Query); err == nil && vec != nil {
			var items []sqlite.RAGDocument
			if req.UserID > 0 {
				items, err = s.repo.SearchVectorForUser(ctx, vec, s.cfg.RAG.VectorTopK, req.BankID, req.SubjectID, req.ChapterID, req.UserID)
			} else {
				items, err = s.repo.SearchVector(ctx, vec, s.cfg.RAG.VectorTopK, req.BankID, req.SubjectID, req.ChapterID)
			}
			if err == nil {
				vectorItems = items
			}
		}
	}

	// 3. Merge（按 id 去重，分数取最大）
	merged := mergeResults(ftsItems, vectorItems)

	// 4. 获取独立配置的 Rerank Provider 进行重排
	reranker := s.getReranker(ctx)

	items := make([]rerank.Item, 0, len(merged))
	for _, d := range merged {
		items = append(items, rerank.Item{ID: d.ID, Content: d.Content, Score: d.Score})
	}
	ranked, err := reranker.Rerank(ctx, req.Query, items, topK)
	if err != nil {
		// Rerank 服务异常时优雅降级为原始顺序
		ranked, _ = rerank.NewNoRerank().Rerank(ctx, req.Query, items, topK)
	}

	byID := map[int64]sqlite.RAGDocument{}
	for _, d := range merged {
		byID[d.ID] = d
	}
	out := make([]sqlite.RAGDocument, 0, len(ranked))
	for _, r := range ranked {
		if d, ok := byID[r.ID]; ok {
			d.Score = r.Score
			out = append(out, d)
		}
	}
	return &SearchResult{Items: out, Total: len(out)}, nil
}

func (s *RAGService) getReranker(ctx context.Context) rerank.Provider {
	cfg, err := s.registry.Default(ctx, provider.TypeRerank)
	if err == nil && cfg != nil && cfg.BaseURL != "" {
		return rerank.NewProvider(rerank.Config{
			Protocol:   cfg.Protocol,
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.RerankModel,
			TimeoutSec: 15,
		})
	}

	// 自动平滑回退：若未单独配置 Rerank 模型，自动借用当前用户的通用大模型作为 Prompt Reranker
	llmCfg, errLLM := s.registry.Default(ctx, provider.TypeLLM)
	if errLLM == nil && llmCfg != nil && llmCfg.BaseURL != "" {
		return rerank.NewLLMRerankProvider(rerank.Config{
			Protocol:   llmCfg.Protocol,
			BaseURL:    llmCfg.BaseURL,
			APIKey:     llmCfg.APIKey,
			Model:      llmCfg.ChatModel,
			TimeoutSec: 15,
		})
	}

	return rerank.NewNoRerank()
}

func mergeResults(groups ...[]sqlite.RAGDocument) []sqlite.RAGDocument {
	byID := map[int64]sqlite.RAGDocument{}
	order := []int64{}
	for _, group := range groups {
		for _, d := range group {
			if _, ok := byID[d.ID]; !ok {
				order = append(order, d.ID)
			}
			if d.Score > byID[d.ID].Score {
				byID[d.ID] = d
			}
		}
	}
	out := make([]sqlite.RAGDocument, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// BuildContext 将检索结果构建为 Prompt 上下文（限制长度）。
func BuildContext(docs []sqlite.RAGDocument, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 6000
	}
	var b strings.Builder
	for _, d := range docs {
		head := d.SourceType
		if d.QuestionID > 0 {
			head += " question:" + itoa64(d.QuestionID)
		}
		if d.DocumentID > 0 {
			head += " document:" + itoa64(d.DocumentID) + "/page:" + itoa64(int64(d.DocumentPage))
		}
		b.WriteString("【" + head + "】\n")
		content := d.Content
		if len(content) > maxChars {
			content = content[:maxChars]
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

var _ = embedding.NewOpenAIEmbeddingProvider
