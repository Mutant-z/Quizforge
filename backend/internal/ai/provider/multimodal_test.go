package provider

import (
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
