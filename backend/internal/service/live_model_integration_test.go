package service

// This test is intentionally opt-in. The normal unit-test suite must remain
// deterministic and must never spend tokens or depend on a user's provider.
// Run it explicitly with QT_REAL_MODEL_TEST=1 to prove that the configured
// default LLM receives a real multimodal request.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/security"
)

func TestConfiguredVisionProviderLive(t *testing.T) {
	if os.Getenv("QT_REAL_MODEL_TEST") != "1" {
		t.Skip("real model test is opt-in; set QT_REAL_MODEL_TEST=1")
	}

	cfg := config.Load()
	dbPath := os.Getenv("QT_DATABASE_PATH")
	if dbPath == "" {
		dbPath = findLiveDatabase(t)
	}
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Clean(dbPath)
	}
	db, err := database.Open(dbPath, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	userID := int64(3)
	if raw := os.Getenv("QT_REAL_MODEL_USER_ID"); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed <= 0 {
			t.Fatalf("invalid QT_REAL_MODEL_USER_ID %q", raw)
		}
		userID = parsed
	}
	ctx := observability.WithUserID(context.Background(), userID)
	registry := provider.NewRegistry(db, security.NewAESCrypt(cfg.Auth.MasterKey))
	stored, err := registry.Default(ctx, provider.TypeLLM)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.SupportsVision {
		t.Fatalf("default provider %q is not marked as vision-capable", stored.Name)
	}
	llm, err := provider.NewFromConfig(*stored)
	if err != nil {
		t.Fatal(err)
	}

	imagePath := os.Getenv("QT_REAL_MODEL_IMAGE")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(dbPath), "page-assets", "1", "extraction", "page-0004.png")
	}
	image, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read live vision fixture %s: %v", imagePath, err)
	}

	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	started := time.Now()
	resp, err := llm.Chat(callCtx, provider.ChatRequest{
		Messages: []provider.ChatMessage{{
			Role:    "user",
			Content: `阅读所附题目页，只返回 JSON：{"ok":true,"has_question":true}`,
			Parts:   []provider.ContentPart{{Type: "image", MIMEType: "image/png", Data: image, Detail: "low"}},
		}},
		JSONMode:  true,
		MaxTokens: 128,
	})
	if err != nil {
		t.Fatalf("configured provider %s/%s multimodal request failed after %s: %v", stored.Name, stored.ChatModel, time.Since(started).Round(time.Millisecond), err)
	}
	if len(resp.Content) == 0 {
		t.Fatal("configured provider returned an empty response")
	}
	t.Logf("live model request completed provider=%s model=%s latency=%s input_tokens=%d output_tokens=%d response_bytes=%d", stored.Name, resp.Model, time.Since(started).Round(time.Millisecond), resp.InputTokens, resp.OutputTokens, len(resp.Content))
}

func findLiveDatabase(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "data", "app.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, "backend", "data", "app.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatal("live database not found; set QT_DATABASE_PATH")
	return ""
}
