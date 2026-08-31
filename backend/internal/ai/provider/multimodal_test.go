package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIWireMessagesMultimodal(t *testing.T) {
	wire := openAIWireMessages([]ChatMessage{{
		Role: "user", Content: "inspect",
		Parts: []ContentPart{{Type: "text", Text: "page 1"}, {Type: "image", MIMEType: "image/png", Data: []byte{1, 2, 3}, Detail: "high"}},
	}})
	parts, ok := wire[0]["content"].([]map[string]interface{})
	if !ok || len(parts) != 3 {
		t.Fatalf("unexpected content parts: %#v", wire[0]["content"])
	}
	imageURL := parts[2]["image_url"].(map[string]string)
	if !strings.HasPrefix(imageURL["url"], "data:image/png;base64,") || imageURL["detail"] != "high" {
		t.Fatalf("unexpected image payload: %#v", imageURL)
	}
}

func TestAnthropicBodyMultimodal(t *testing.T) {
	p := NewAnthropicProvider(Config{ChatModel: "vision-model", SupportsVision: true})
	body := p.buildBody(ChatRequest{Messages: []ChatMessage{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "inspect", Parts: []ContentPart{{Type: "image", MIMEType: "image/jpeg", Data: []byte{4, 5}}}},
	}}, false)
	if body["system"] != "rules" || !p.SupportsVision() {
		t.Fatalf("system or capability missing: %#v", body)
	}
	messages := body["messages"].([]map[string]interface{})
	content := messages[0]["content"].([]map[string]interface{})
	source := content[1]["source"].(map[string]string)
	if source["type"] != "base64" || source["media_type"] != "image/jpeg" || source["data"] == "" {
		t.Fatalf("unexpected source: %#v", source)
	}
}

func TestOpenAIProviderDisablesDeepSeekV4ThinkingForShortProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		thinking, ok := payload["thinking"].(map[string]interface{})
		if !ok || thinking["type"] != "disabled" {
			t.Fatalf("expected disabled thinking, payload=%s", body)
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("vision probe should not require JSON output, payload=%s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(Config{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		ChatModel:  "deepseek-v4-flash-vision-exp",
		TimeoutSec: 2,
	})
	if _, err := p.Chat(context.Background(), ChatRequest{
		Messages:        []ChatMessage{{Role: "user", Content: "请读取图片", Parts: []ContentPart{{Type: "image", MIMEType: "image/png", Data: []byte{1}}}}},
		MaxTokens:       128,
		DisableThinking: true,
	}); err != nil {
		t.Fatalf("probe request failed: %v", err)
	}
}
