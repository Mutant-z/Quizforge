package provider

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider 实现 Anthropic Messages API。
type AnthropicProvider struct {
	cfg    Config
	client *http.Client
}

func NewAnthropicProvider(cfg Config) *AnthropicProvider {
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 120
	}
	return &AnthropicProvider{cfg: cfg, client: &http.Client{Timeout: time.Duration(timeout) * time.Second}}
}

func (p *AnthropicProvider) Name() string         { return p.cfg.Name }
func (p *AnthropicProvider) Model() string        { return p.cfg.ChatModel }
func (p *AnthropicProvider) SupportsVision() bool { return p.cfg.SupportsVision }

func (p *AnthropicProvider) baseURL() string {
	base := strings.TrimSuffix(p.cfg.BaseURL, "/")
	if strings.HasSuffix(base, "/messages") {
		return base
	}
	return base + "/v1/messages"
}

func (p *AnthropicProvider) buildBody(req ChatRequest, stream bool) map[string]interface{} {
	var system string
	messages := []map[string]interface{}{}
	for _, m := range req.Messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		if len(m.Parts) == 0 {
			messages = append(messages, map[string]interface{}{"role": m.Role, "content": m.Content})
			continue
		}
		content := make([]map[string]interface{}, 0, len(m.Parts)+1)
		if strings.TrimSpace(m.Content) != "" {
			content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
		}
		for _, part := range m.Parts {
			if part.Type == "image" {
				mimeType := part.MIMEType
				if mimeType == "" {
					mimeType = "image/png"
				}
				content = append(content, map[string]interface{}{
					"type": "image",
					"source": map[string]string{
						"type": "base64", "media_type": mimeType,
						"data": base64.StdEncoding.EncodeToString(part.Data),
					},
				})
			} else if strings.TrimSpace(part.Text) != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": part.Text})
			}
		}
		messages = append(messages, map[string]interface{}{"role": m.Role, "content": content})
	}
	body := map[string]interface{}{
		"model":      p.cfg.ChatModel,
		"messages":   messages,
		"max_tokens": req.MaxTokens,
	}
	if req.MaxTokens <= 0 {
		body["max_tokens"] = 2048
	}
	if system != "" {
		body["system"] = system
	}
	if stream {
		body["stream"] = true
	}
	return body
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body := p.buildBody(req, false)
	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL(), strings.NewReader(string(payload)))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return ChatResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("anthropic response parse: %w", err)
	}
	var text strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	return ChatResponse{
		Content:      text.String(),
		Model:        p.cfg.ChatModel,
		InputTokens:  parsed.Usage.InputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
	}, nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	body := p.buildBody(req, true)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL(), strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("anthropic stream status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			var evt struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				continue
			}
			switch evt.Type {
			case "content_block_delta":
				if evt.Delta.Type == "text_delta" && evt.Delta.Text != "" {
					select {
					case ch <- StreamEvent{Type: "token", Content: evt.Delta.Text}:
					case <-ctx.Done():
						return
					}
				}
			case "message_stop", "message_delta":
				// continue
			}
		}
		ch <- StreamEvent{Type: "done"}
	}()
	return ch, nil
}
