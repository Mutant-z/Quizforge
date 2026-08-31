// Package rerank Rerank Provider 抽象（支持 通用大模型 Prompt 重排、专用 HTTP Rerank API 及 NoRerank 回退）。
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Item 待重排条目。
type Item struct {
	ID      int64   `json:"id"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// Provider 重排接口。
type Provider interface {
	Name() string
	Rerank(ctx context.Context, query string, items []Item, topK int) ([]Item, error)
}

// NoRerank 默认实现：不重排，按原有顺序直接截断。
type NoRerank struct{}

func NewNoRerank() *NoRerank { return &NoRerank{} }

func (n *NoRerank) Name() string { return "no_rerank" }

func (n *NoRerank) Rerank(ctx context.Context, query string, items []Item, topK int) ([]Item, error) {
	if topK <= 0 || topK > len(items) {
		topK = len(items)
	}
	return items[:topK], nil
}

// Config Rerank 配置。
type Config struct {
	Protocol   string // openai | llm | http | cohere
	BaseURL    string
	APIKey     string
	Model      string
	TimeoutSec int
}

// NewProvider 统一工厂方法，根据 Protocol 返回对应的 Provider（默认支持通用大模型提示词重排）。
func NewProvider(cfg Config) Provider {
	proto := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	switch proto {
	case "http", "cohere", "siliconflow", "bge":
		return NewHTTPRerankProvider(cfg)
	case "openai", "llm", "":
		return NewLLMRerankProvider(cfg)
	default:
		return NewLLMRerankProvider(cfg)
	}
}

// ====================================================================
// 1. LLMRerankProvider (基于通用大模型 Chat Completions 的 Prompt 重排)
// ====================================================================

type LLMRerankProvider struct {
	cfg    Config
	client *http.Client
}

func NewLLMRerankProvider(cfg Config) *LLMRerankProvider {
	timeout := 30 * time.Second
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}
	return &LLMRerankProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *LLMRerankProvider) Name() string {
	return "llm_rerank"
}

func (p *LLMRerankProvider) Rerank(ctx context.Context, query string, items []Item, topK int) ([]Item, error) {
	if len(items) == 0 {
		return items, nil
	}
	if topK <= 0 || topK > len(items) {
		topK = len(items)
	}

	// 构建候选文档列表
	var docListBuilder strings.Builder
	for i, it := range items {
		// 截取前 300 字符以节省 Token 并提升响应速度
		snippet := strings.TrimSpace(it.Content)
		if len([]rune(snippet)) > 300 {
			snippet = string([]rune(snippet)[:300]) + "..."
		}
		docListBuilder.WriteString(fmt.Sprintf("[%d] %s\n", i, snippet))
	}

	systemPrompt := `你是一个高精度的检索重排与语义相关度评估助手。
请根据用户的搜索查询（Query），评估各候选文档与该查询的语义相关性。
请为每个候选文档输出 0.0 到 1.0 的相关度评分（relevance_score，数值越高表示越相关）。
必须严格以 JSON 数组格式返回结果，按 relevance_score 从高到低排序，不要输出任何其他说明文字。
格式示例：
[
  {"index": 0, "relevance_score": 0.95},
  {"index": 1, "relevance_score": 0.72}
]`

	userPrompt := fmt.Sprintf("查询问题：%s\n\n候选文档列表：\n%s", query, docListBuilder.String())

	baseURL := strings.TrimSuffix(p.cfg.BaseURL, "/")
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		if strings.HasSuffix(endpoint, "/v1") {
			endpoint += "/chat/completions"
		} else {
			endpoint += "/v1/chat/completions"
		}
	}

	reqBody := map[string]interface{}{
		"model": p.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
		"max_tokens":  800,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm server returned %d: %s", resp.StatusCode, string(raw))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &chatResp); err != nil || len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("failed to parse chat completions response: %w", err)
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)

	// 解析 LLM 返回的 JSON 数组
	type scoreItem struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	}
	var scores []scoreItem

	jsonStr := extractJSONArray(content)
	if err := json.Unmarshal([]byte(jsonStr), &scores); err != nil {
		// 若偶发 JSON 格式异常，平滑降级：按原有分数顺序截取返回，避免阻断检索主流程
		return items[:topK], nil
	}

	seen := make(map[int]bool)
	out := make([]Item, 0, len(scores))
	for _, s := range scores {
		if s.Index >= 0 && s.Index < len(items) && !seen[s.Index] {
			seen[s.Index] = true
			it := items[s.Index]
			it.Score = s.RelevanceScore
			out = append(out, it)
		}
	}

	// 补充未在评分列表中的剩余条目（保持原相对顺序）
	for i, it := range items {
		if !seen[i] {
			out = append(out, it)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})

	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

var jsonArrayRegex = regexp.MustCompile(`(?s)\[\s*\{.*?\}\s*\]`)

func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 去除代码块包裹
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			end := len(lines) - 1
			if strings.HasPrefix(strings.TrimSpace(lines[end]), "```") {
				lines = lines[1:end]
			} else {
				lines = lines[1:]
			}
			s = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return s
	}
	match := jsonArrayRegex.FindString(s)
	if match != "" {
		return match
	}
	return s
}

// ====================================================================
// 2. HTTPRerankProvider (兼容 SiliconFlow, Cohere, BGE-Reranker 等专用 /v1/rerank)
// ====================================================================

type HTTPRerankProvider struct {
	cfg    Config
	client *http.Client
}

func NewHTTPRerankProvider(cfg Config) *HTTPRerankProvider {
	timeout := 30 * time.Second
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}
	return &HTTPRerankProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *HTTPRerankProvider) Name() string {
	return "http_rerank"
}

func (p *HTTPRerankProvider) Rerank(ctx context.Context, query string, items []Item, topK int) ([]Item, error) {
	if len(items) == 0 {
		return items, nil
	}
	if topK <= 0 || topK > len(items) {
		topK = len(items)
	}

	docs := make([]string, len(items))
	for i, item := range items {
		docs[i] = item.Content
	}

	reqBody := map[string]interface{}{
		"model":     p.cfg.Model,
		"query":     query,
		"documents": docs,
		"top_n":     topK,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSuffix(p.cfg.BaseURL, "/")
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/rerank") {
		if strings.HasSuffix(endpoint, "/v1") {
			endpoint += "/rerank"
		} else {
			endpoint += "/v1/rerank"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank server returned %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to parse rerank response: %w", err)
	}

	if len(result.Results) == 0 {
		return items[:topK], nil
	}

	out := make([]Item, 0, len(result.Results))
	for _, r := range result.Results {
		if r.Index >= 0 && r.Index < len(items) {
			it := items[r.Index]
			it.Score = r.RelevanceScore
			out = append(out, it)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})

	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}
