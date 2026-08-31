package provider_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/security"
)

func setupTestDB(t *testing.T) (*sql.DB, *provider.Registry) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// 查找 migrations 路径
	wd, _ := os.Getwd()
	migrationsDir := filepath.Join(wd, "../../../migrations")
	if _, err := os.Stat(migrationsDir); err != nil {
		migrationsDir = "/Users/mutant/Documents/project/QuizForge/backend/migrations"
	}

	if err := database.Migrate(context.Background(), db, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	crypt := security.NewAESCrypt("0123456789abcdef0123456789abcdef")

	reg := provider.NewRegistry(db, crypt)
	return db, reg
}

func TestUserScopedProviders(t *testing.T) {
	db, reg := setupTestDB(t)
	defer db.Close()

	ctxUser1 := observability.WithUserID(context.Background(), 101)
	ctxUser2 := observability.WithUserID(context.Background(), 102)

	// 1. 用户 101 创建自己的 LLM Provider
	p1, err := reg.Save(ctxUser1, provider.TypeLLM, "User1-OpenAI", "openai", "http://user1.api/v1", "sk-user1-key", "gpt-4o", true)
	if err != nil {
		t.Fatalf("user1 save provider: %v", err)
	}
	if p1.UserID != 101 {
		t.Errorf("expected user_id 101, got %d", p1.UserID)
	}

	// 2. 用户 102 创建自己的 LLM Provider
	p2, err := reg.Save(ctxUser2, provider.TypeLLM, "User2-Gemini", "openai", "http://user2.api/v1", "sk-user2-key", "gemini-3.7-flash", true)
	if err != nil {
		t.Fatalf("user2 save provider: %v", err)
	}
	if p2.UserID != 102 {
		t.Errorf("expected user_id 102, got %d", p2.UserID)
	}

	// 3. 隔离验证：用户 101 List 只能看到自己的
	list1, err := reg.List(ctxUser1, provider.TypeLLM)
	if err != nil {
		t.Fatalf("user1 list: %v", err)
	}
	if len(list1) != 1 || list1[0].ID != p1.ID {
		t.Errorf("user1 should only see p1, got %v", list1)
	}

	// 用户 102 List 只能看到自己的
	list2, err := reg.List(ctxUser2, provider.TypeLLM)
	if err != nil {
		t.Fatalf("user2 list: %v", err)
	}
	if len(list2) != 1 || list2[0].ID != p2.ID {
		t.Errorf("user2 should only see p2, got %v", list2)
	}

	// 4. Default 验证：用户 101 获取默认配置得到 user1 的模型
	cfg1, err := reg.Default(ctxUser1, provider.TypeLLM)
	if err != nil {
		t.Fatalf("user1 default: %v", err)
	}
	if cfg1.ChatModel != "gpt-4o" || cfg1.APIKey != "sk-user1-key" {
		t.Errorf("user1 default mismatch: got %+v", cfg1)
	}

	// 用户 102 获取默认配置得到 user2 的模型
	cfg2, err := reg.Default(ctxUser2, provider.TypeLLM)
	if err != nil {
		t.Fatalf("user2 default: %v", err)
	}
	if cfg2.ChatModel != "gemini-3.7-flash" || cfg2.APIKey != "sk-user2-key" {
		t.Errorf("user2 default mismatch: got %+v", cfg2)
	}

	// 5. 权限验证：用户 102 尝试删除用户 101 的 Provider 应被拒绝
	if err := reg.Delete(ctxUser2, p1.ID); err == nil {
		t.Errorf("user2 should not be allowed to delete user1's provider")
	}

	// 用户 101 正常删除自己的 Provider
	if err := reg.Delete(ctxUser1, p1.ID); err != nil {
		t.Fatalf("user1 delete own provider: %v", err)
	}

	listAfterDelete, _ := reg.List(ctxUser1, provider.TypeLLM)
	if len(listAfterDelete) != 0 {
		t.Errorf("user1 list should be empty after deletion")
	}
}

func TestGlobalFallback(t *testing.T) {
	db, reg := setupTestDB(t)
	defer db.Close()

	ctxGlobal := observability.WithUserID(context.Background(), 0)
	ctxNewUser := observability.WithUserID(context.Background(), 999)

	// 系统全局管理员（user_id = 0）配置默认模型
	_, err := reg.Save(ctxGlobal, provider.TypeLLM, "Global-Fallback-Model", "openai", "http://global.api/v1", "sk-global-key", "global-model-v1", true)
	if err != nil {
		t.Fatalf("save global provider: %v", err)
	}

	// 新用户 999 未配置模型，获取 Default 时自动回退到系统全局配置
	cfg, err := reg.Default(ctxNewUser, provider.TypeLLM)
	if err != nil {
		t.Fatalf("new user fallback: %v", err)
	}
	if cfg.ChatModel != "global-model-v1" || cfg.APIKey != "sk-global-key" {
		t.Errorf("expected fallback to global model, got %+v", cfg)
	}
}

func TestResolveProviderIsUserScopedAndDecryptsKeyServerSide(t *testing.T) {
	db, reg := setupTestDB(t)
	defer db.Close()

	ctxUser1 := observability.WithUserID(context.Background(), 201)
	ctxUser2 := observability.WithUserID(context.Background(), 202)
	ctxSystem := observability.WithUserID(context.Background(), 0)

	saved, err := reg.Save(ctxUser1, provider.TypeLLM, "User1", "openai", "http://127.0.0.1:1/v1", "sk-secret", "test-model", true)
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}

	resolved, err := reg.Resolve(ctxUser1, saved.ID)
	if err != nil {
		t.Fatalf("resolve own provider: %v", err)
	}
	if resolved.APIKey != "sk-secret" || resolved.APIKeyMasked != "sk-****cret" {
		t.Fatalf("resolved provider key mismatch: masked=%q key=%q", resolved.APIKeyMasked, resolved.APIKey)
	}

	if _, err := reg.Resolve(ctxUser2, saved.ID); err == nil {
		t.Fatal("a user must not resolve another user's provider key")
	}
	if resolvedSystem, err := reg.Resolve(ctxSystem, saved.ID); err != nil || resolvedSystem.APIKey != "sk-secret" {
		t.Fatalf("system context should resolve provider: resolved=%+v err=%v", resolvedSystem, err)
	}
}
