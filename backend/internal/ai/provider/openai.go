// Package provider LLM Provider 抽象（docs/14）。
package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatMessage 对话消息。
type ChatMessage struct {
	Role    string        `json:"role"`
	Content string        `json:"content,omitempty"`
	Parts   []ContentPart `json:"-"`
}

// ContentPart is a provider-neutral multimodal message part. Data is never
// marshalled directly so request logs cannot accidentally persist images.
type ContentPart struct {
	Type     string `json:"type"` // text | image
	Text     string `json:"text,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Data     []byte `json:"-"`
	Detail   string `json:"detail,omitempty"`
}

// ChatRequest 请求。
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	// JSONMode 请求结构化输出
	JSONMode bool `json:"-"`
	// DisableThinking 用于短响应探测，当前仅对 DeepSeek V4 发送兼容参数。
	DisableThinking bool `json:"-"`
}

// ChatResponse 响应。
type ChatResponse struct {
	Content      string
	Model        string
	InputTokens  int
	OutputTokens int
}

// StreamEvent 流式事件。
type StreamEvent struct {
	Type    string // token | done | error
	Content string
	Error   error
}

// LLMProvider 聊天接口。
type LLMProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	Name() string
	Model() string
	SupportsVision() bool
}

// Config Provider 配置。
type Config struct {
	ID             int64
	Name           string
	Protocol       string // openai | anthropic
	BaseURL        string
	APIKey         string
	ChatModel      string
	EmbeddingModel string
	RerankModel    string
	TimeoutSec     int
	SupportsVision bool
}

// OpenAIProvider OpenAI Compatible 实现。
type OpenAIProvider struct {
	cfg    Config
	client *http.Client
}

func NewOpenAIProvider(cfg Config) *OpenAIProvider {
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 120
	}
	return &OpenAIProvider{cfg: cfg, client: &http.Client{Timeout: time.Duration(timeout) * time.Second}}
}

func (p *OpenAIProvider) Name() string         { return p.cfg.Name }
func (p *OpenAIProvider) Model() string        { return p.cfg.ChatModel }
func (p *OpenAIProvider) SupportsVision() bool { return p.cfg.SupportsVision }

func openAIWireMessages(messages []ChatMessage) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		wire := map[string]interface{}{"role": message.Role}
		if len(message.Parts) == 0 {
			wire["content"] = message.Content
			out = append(out, wire)
			continue
		}
		parts := make([]map[string]interface{}, 0, len(message.Parts)+1)
		if strings.TrimSpace(message.Content) != "" {
			parts = append(parts, map[string]interface{}{"type": "text", "text": message.Content})
		}
		for _, part := range message.Parts {
			switch part.Type {
			case "image":
				mimeType := part.MIMEType
				if mimeType == "" {
					mimeType = "image/png"
				}
				detail := part.Detail
				if detail == "" {
					detail = "high"
				}
				parts = append(parts, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url":    "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(part.Data),
						"detail": detail,
					},
				})
			default:
				if strings.TrimSpace(part.Text) != "" {
					parts = append(parts, map[string]interface{}{"type": "text", "text": part.Text})
				}
			}
		}
		wire["content"] = parts
		out = append(out, wire)
	}
	return out
}

func (p *OpenAIProvider) baseURL() string {
	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if req.Model == "" {
		req.Model = p.cfg.ChatModel
	}
	body := map[string]interface{}{
		"model":       req.Model,
		"messages":    openAIWireMessages(req.Messages),
		"temperature": req.Temperature,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	if req.DisableThinking && isDeepSeekV4Model(req.Model) {
		body["thinking"] = map[string]string{"type": "disabled"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL(), strings.NewReader(string(payload)))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return ChatResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("llm status %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("llm response parse: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("llm empty choices")
	}
	return ChatResponse{
		Content:      parsed.Choices[0].Message.Content,
		Model:        req.Model,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
	}, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		req.Model = p.cfg.ChatModel
	}
	body := map[string]interface{}{
		"model":       req.Model,
		"messages":    openAIWireMessages(req.Messages),
		"temperature": req.Temperature,
		"stream":      true,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	if req.DisableThinking && isDeepSeekV4Model(req.Model) {
		body["thinking"] = map[string]string{"type": "disabled"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL(), strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llm stream status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := newSSEReader(resp.Body)
		for {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Type: "error", Error: ctx.Err()}
				return
			default:
			}
			event, data, ok := reader.Next()
			if !ok {
				ch <- StreamEvent{Type: "done"}
				return
			}
			switch event {
			case "data":
				if strings.TrimSpace(data) == "[DONE]" {
					ch <- StreamEvent{Type: "done"}
					return
				}
				var parsed struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				if err := json.Unmarshal([]byte(data), &parsed); err != nil {
					continue
				}
				if len(parsed.Choices) > 0 {
					ch <- StreamEvent{Type: "token", Content: parsed.Choices[0].Delta.Content}
				}
			}
		}
	}()
	return ch, nil
}

func isDeepSeekV4Model(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-v4")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
