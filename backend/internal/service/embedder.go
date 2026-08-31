package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/quiztrace/quiztrace/internal/ai/embedding"
	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

// Embedder 向量化服务（RAG）。
type Embedder struct {
	repo     *sqlite.Repository
	cfg      *config.Config
	registry *provider.Registry
}

func NewEmbedder(repo *sqlite.Repository, cfg *config.Config) *Embedder {
	return &Embedder{repo: repo, cfg: cfg, registry: provider.NewRegistry(repo.DB(), cryptFrom(cfg))}
}

// EmbedPending 为所有未向量化文档生成向量。
func (e *Embedder) EmbedPending(ctx context.Context, bankID int64) error {
	cfg, err := e.registry.Default(ctx, provider.TypeEmbedding)
	if err != nil || cfg == nil {
		return nil // 未配置 embedding 模型，直接跳过
	}
	if cfg.EmbeddingModel == "" && cfg.ChatModel == "" {
		return nil
	}
	modelName := cfg.EmbeddingModel
	if modelName == "" {
		modelName = cfg.ChatModel
	}
	emb, err := embedding.NewProvider(embedding.Config{
		Protocol: cfg.Protocol,
		BaseURL:  cfg.BaseURL,
		APIKey:   cfg.APIKey,
		Model:    modelName,
	})
	if err != nil {
		return err
	}
	const batchSize = 32
	offset := 0
	for {
		docs, err := e.repo.ListRAGDocumentsForEmbedding(ctx, modelName, batchSize, offset)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			break
		}
		texts := make([]string, 0, len(docs))
		for _, d := range docs {
			texts = append(texts, d.Content)
		}
		vectors, err := emb.Embed(ctx, texts)
		if err != nil {
			// 单批失败不整体中断
			offset += len(docs)
			continue
		}
		for i, d := range docs {
			if i < len(vectors) {
				_ = e.repo.UpsertEmbedding(ctx, d.ID, modelName, vectors[i])
			}
		}
		offset += len(docs)
	}
	return nil
}

// EmbedOne 单题向量化。
func (e *Embedder) EmbedOne(ctx context.Context, q *domain.Question) error {
	cfg, err := e.registry.Default(ctx, provider.TypeEmbedding)
	if err != nil || cfg == nil {
		return nil
	}
	modelName := cfg.EmbeddingModel
	if modelName == "" {
		modelName = cfg.ChatModel
	}
	if modelName == "" {
		return nil
	}
	doc, err := e.repo.GetRAGDocumentByQuestion(ctx, q.ID)
	if err != nil {
		return err
	}
	emb, err := embedding.NewProvider(embedding.Config{
		Protocol: cfg.Protocol,
		BaseURL:  cfg.BaseURL,
		APIKey:   cfg.APIKey,
		Model:    modelName,
	})
	if err != nil {
		return err
	}
	vectors, err := emb.Embed(ctx, []string{doc.Content})
	if err != nil {
		return err
	}
	if len(vectors) > 0 {
		return e.repo.UpsertEmbedding(ctx, doc.ID, modelName, vectors[0])
	}
	return nil
}

// EmbedText 直接向量化一段文本。
func (e *Embedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	cfg, err := e.registry.Default(ctx, provider.TypeEmbedding)
	if err != nil || cfg == nil {
		return nil, nil
	}
	modelName := cfg.EmbeddingModel
	if modelName == "" {
		modelName = cfg.ChatModel
	}
	if modelName == "" {
		return nil, nil
	}
	emb, err := embedding.NewProvider(embedding.Config{
		Protocol: cfg.Protocol,
		BaseURL:  cfg.BaseURL,
		APIKey:   cfg.APIKey,
		Model:    modelName,
	})
	if err != nil {
		return nil, err
	}
	vectors, err := emb.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return vectors[0], nil
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}

var _ = strings.TrimSpace
