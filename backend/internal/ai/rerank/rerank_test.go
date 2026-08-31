package rerank_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quiztrace/quiztrace/internal/ai/rerank"
)

func TestLLMRerankProvider(t *testing.T) {
	// 模拟 OpenAI 兼容的 Chat Completions 接口
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		// 返回带 Markdown 代码块包装的打分结果，测试健壮性
		mockReply := "```json\n[\n  {\"index\": 1, \"relevance_score\": 0.95},\n  {\"index\": 0, \"relevance_score\": 0.35}\n]\n```"
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": mockReply,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	provider := rerank.NewProvider(rerank.Config{
		Protocol:   "openai",
		BaseURL:    ts.URL + "/v1",
		APIKey:     "mock-key",
		Model:      "gemini-3.7-flash-high",
		TimeoutSec: 5,
	})

	items := []rerank.Item{
		{ID: 101, Content: "无关内容", Score: 0.1},
		{ID: 102, Content: "高度相关的计算机网络内容", Score: 0.2},
	}

	ranked, err := provider.Rerank(context.Background(), "计算机网络", items, 2)
	if err != nil {
		t.Fatalf("rerank failed: %v", err)
	}

	if len(ranked) != 2 {
		t.Fatalf("expected 2 items, got %d", len(ranked))
	}

	// 确认第 2 个候选（ID: 102）被排到了最前
	if ranked[0].ID != 102 {
		t.Errorf("expected top item ID 102, got %d", ranked[0].ID)
	}
	if ranked[0].Score != 0.95 {
		t.Errorf("expected top score 0.95, got %f", ranked[0].Score)
	}
}
