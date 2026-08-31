// Package embedding 提供多协议向量嵌入（Embedding）Provider 抽象与实现。
// 支持 OpenAI 兼容规范、Ollama 本地接口、HuggingFace/TEI (Text Embeddings Inference) 及 Cohere 规范。
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider 向量化通用接口。
type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Name() string
	Model() string
}

// Config 向量化 Provider 配置。
type Config struct {
	Protocol string // openai | ollama | huggingface | tei | cohere
	BaseURL  string
	APIKey   string
	Model    string
	Timeout  time.Duration
}

// NewProvider 根据协议工厂方法构造对应的 Embedding Provider。
func NewProvider(cfg Config) (Provider, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	proto := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	switch proto {
	case "ollama":
		return NewOllamaEmbeddingProvider(cfg), nil
	case "huggingface", "tei":
		return NewTEIEmbeddingProvider(cfg), nil
	case "cohere":
		return NewCohereEmbeddingProvider(cfg), nil
	case "openai", "":
		return NewOpenAIEmbeddingProvider(cfg), nil
	default:
		// 默认回退为 OpenAI 兼容协议
		return NewOpenAIEmbeddingProvider(cfg), nil
	}
}

// ==========================================
// 1. OpenAI Compatible Embedding Provider
// ==========================================

type OpenAIEmbeddingProvider struct {
	cfg    Config
	client *http.Client
}

func NewOpenAIEmbeddingProvider(cfg Config) *OpenAIEmbeddingProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIEmbeddingProvider{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

func (p *OpenAIEmbeddingProvider) Name() string  { return "openai_embedding" }
func (p *OpenAIEmbeddingProvider) Model() string { return p.cfg.Model }

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	const batchSize = 64
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := p.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (p *OpenAIEmbeddingProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]interface{}{
		"model": p.cfg.Model,
		"input": texts,
	}
	payload, _ := json.Marshal(body)
	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/embeddings") {
		if strings.HasSuffix(url, "/v1") {
			url += "/embeddings"
		} else {
			url += "/v1/embeddings"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedding request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embedding status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("openai embedding parse: %w", err)
	}
	out := make([][]float32, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		out = append(out, d.Embedding)
	}
	return out, nil
}

// ==========================================
// 2. Ollama Embedding Provider (/api/embed or /api/embeddings)
// ==========================================

type OllamaEmbeddingProvider struct {
	cfg    Config
	client *http.Client
}

func NewOllamaEmbeddingProvider(cfg Config) *OllamaEmbeddingProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OllamaEmbeddingProvider{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

func (p *OllamaEmbeddingProvider) Name() string  { return "ollama_embedding" }
func (p *OllamaEmbeddingProvider) Model() string { return p.cfg.Model }

func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	// 优先尝试 Ollama 现代批量接口 /api/embed
	out, err := p.embedBatchModern(ctx, texts)
	if err == nil && len(out) == len(texts) {
		return out, nil
	}

	// 回退至 /api/embeddings 单条遍历
	legacyOut := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := p.embedSingleLegacy(ctx, text)
		if err != nil {
			return nil, err
		}
		legacyOut = append(legacyOut, vec)
	}
	return legacyOut, nil
}

func (p *OllamaEmbeddingProvider) embedBatchModern(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]interface{}{
		"model": p.cfg.Model,
		"input": texts,
	}
	payload, _ := json.Marshal(body)
	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/api/embed") {
		url += "/api/embed"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama /api/embed status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var parsed struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned empty embeddings")
	}
	return parsed.Embeddings, nil
}

func (p *OllamaEmbeddingProvider) embedSingleLegacy(ctx context.Context, text string) ([]float32, error) {
	body := map[string]interface{}{
		"model":  p.cfg.Model,
		"prompt": text,
	}
	payload, _ := json.Marshal(body)
	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/api/embeddings") {
		url += "/api/embeddings"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama legacy embedding: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama /api/embeddings status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var parsed struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ollama legacy parse: %w", err)
	}
	return parsed.Embedding, nil
}

// ==========================================
// 3. HuggingFace / TEI (Text Embeddings Inference) Provider
// ==========================================

type TEIEmbeddingProvider struct {
	cfg    Config
	client *http.Client
}

func NewTEIEmbeddingProvider(cfg Config) *TEIEmbeddingProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &TEIEmbeddingProvider{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

func (p *TEIEmbeddingProvider) Name() string  { return "tei_embedding" }
func (p *TEIEmbeddingProvider) Model() string { return p.cfg.Model }

func (p *TEIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	body := map[string]interface{}{
		"inputs":    texts,
		"normalize": true,
		"truncate":  true,
	}
	payload, _ := json.Marshal(body)

	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/embed") && !strings.Contains(url, "api-inference.huggingface.co") {
		url += "/embed"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei embedding request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tei embedding status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	// TEI / HuggingFace 返回直接的 2D 浮点数组 [[...], [...]]
	var vectors [][]float32
	if err := json.Unmarshal(raw, &vectors); err == nil && len(vectors) > 0 {
		return vectors, nil
	}

	// 单文本情况可能返回 1D 数组
	var singleVec []float32
	if err := json.Unmarshal(raw, &singleVec); err == nil && len(singleVec) > 0 {
		return [][]float32{singleVec}, nil
	}

	return nil, fmt.Errorf("tei embedding parse error: %s", truncate(string(raw), 150))
}

// ==========================================
// 4. Cohere Embed API Provider
// ==========================================

type CohereEmbeddingProvider struct {
	cfg    Config
	client *http.Client
}

func NewCohereEmbeddingProvider(cfg Config) *CohereEmbeddingProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &CohereEmbeddingProvider{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

func (p *CohereEmbeddingProvider) Name() string  { return "cohere_embedding" }
func (p *CohereEmbeddingProvider) Model() string { return p.cfg.Model }

func (p *CohereEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	model := p.cfg.Model
	if model == "" {
		model = "embed-multilingual-v3.0"
	}
	body := map[string]interface{}{
		"model":      model,
		"texts":      texts,
		"input_type": "search_document",
	}
	payload, _ := json.Marshal(body)

	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/v1/embed") && !strings.HasSuffix(url, "/embed") {
		if strings.HasSuffix(url, "/v1") {
			url += "/embed"
		} else {
			url += "/v1/embed"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cohere embedding request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cohere embedding status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	// Cohere 响应格式：{"embeddings": [[...], ...]} 或 {"embeddings": {"float": [[...], ...]}}
	var parsed struct {
		Embeddings json.RawMessage `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("cohere parse: %w", err)
	}

	var matrix [][]float32
	if err := json.Unmarshal(parsed.Embeddings, &matrix); err == nil && len(matrix) > 0 {
		return matrix, nil
	}

	var nested struct {
		Float [][]float32 `json:"float"`
	}
	if err := json.Unmarshal(parsed.Embeddings, &nested); err == nil && len(nested.Float) > 0 {
		return nested.Float, nil
	}

	return nil, fmt.Errorf("cohere embedding unexpected format: %s", truncate(string(raw), 150))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
