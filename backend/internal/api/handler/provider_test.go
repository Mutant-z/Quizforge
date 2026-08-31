package handler

import (
	"bytes"
	"context"
	"database/sql"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/security"
)

func setupProviderHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wd, _ := os.Getwd()
	migrationsDir := filepath.Join(wd, "../../../migrations")
	if _, err := os.Stat(migrationsDir); err != nil {
		t.Fatalf("locate migrations: %v", err)
	}
	if err := database.Migrate(context.Background(), db, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func callStoredProviderTest(t *testing.T, h *ProviderHandler, userID, providerID int64) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/providers/"+strconv.FormatInt(providerID, 10)+"/test", nil)
	request = request.WithContext(observability.WithUserID(request.Context(), userID))
	response := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(response)
	c.Request = request
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(providerID, 10)}}
	h.TestStored(c)
	return response
}

func TestTestStoredUsesOwnerKeyAndRejectsOtherUsers(t *testing.T) {
	db := setupProviderHandlerTestDB(t)
	registry := provider.NewRegistry(db, security.NewAESCrypt("provider-handler-test-master-key"))

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-owner-secret" {
			http.Error(w, "wrong authorization", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer modelServer.Close()

	ownerCtx := observability.WithUserID(context.Background(), 301)
	saved, err := registry.Save(ownerCtx, provider.TypeLLM, "owner-provider", "openai", modelServer.URL+"/v1", "sk-owner-secret", "test-model", true)
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	h := NewProviderHandler(registry)

	ownerResponse := callStoredProviderTest(t, h, 301, saved.ID)
	if ownerResponse.Code != http.StatusOK || !strings.Contains(ownerResponse.Body.String(), `"ok":true`) {
		t.Fatalf("owner test failed: status=%d body=%s", ownerResponse.Code, ownerResponse.Body.String())
	}

	otherResponse := callStoredProviderTest(t, h, 302, saved.ID)
	if otherResponse.Code != http.StatusNotFound {
		t.Fatalf("other user should not test provider: status=%d body=%s", otherResponse.Code, otherResponse.Body.String())
	}
}

func TestVisionProbeMessageContainsValidPNG(t *testing.T) {
	message := visionProbeMessage()
	if len(message.Parts) != 1 || message.Parts[0].MIMEType != "image/png" {
		t.Fatalf("unexpected probe parts: %#v", message.Parts)
	}
	decoded, err := png.Decode(bytes.NewReader(message.Parts[0].Data))
	if err != nil {
		t.Fatalf("probe image is not valid PNG: %v", err)
	}
	if decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
		t.Fatalf("unexpected probe image size: %v", decoded.Bounds())
	}
}
