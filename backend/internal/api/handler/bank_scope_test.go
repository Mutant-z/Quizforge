package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/security"
	"github.com/quiztrace/quiztrace/internal/service"
)

func callBankList(t *testing.T, h *BankHandler, userID int64, role string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/question-banks?page_size=20", nil)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(response)
	c.Request = request
	c.Set("user_id", userID)
	c.Set("role", role)
	h.List(c)
	return response
}

func callBankGet(t *testing.T, h *BankHandler, userID int64, role string, bankID int64) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/question-banks/"+strconv.FormatInt(bankID, 10), nil)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(response)
	c.Request = request
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(bankID, 10)}}
	c.Set("user_id", userID)
	c.Set("role", role)
	h.Get(c)
	return response
}

func TestBankHandlerScopesLearnerAndAdminViews(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "bank-handler.db"), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wd, _ := os.Getwd()
	if err := database.Migrate(context.Background(), db, filepath.Join(wd, "../../../migrations")); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	hash, err := security.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	ownerA, err := repo.CreateUser(context.Background(), "handler-owner-a", "handler-owner-a@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	ownerB, err := repo.CreateUser(context.Background(), "handler-owner-b", "handler-owner-b@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	bankA, err := repo.CreateBank(context.Background(), "A题库", "", "private", ownerA.ID)
	if err != nil {
		t.Fatal(err)
	}
	bankB, err := repo.CreateBank(context.Background(), "B题库", "", "private", ownerB.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := NewBankHandler(service.NewQuestionService(repo), repo)

	learnerResponse := callBankList(t, h, ownerA.ID, "user")
	if learnerResponse.Code != http.StatusOK {
		t.Fatalf("learner list status=%d body=%s", learnerResponse.Code, learnerResponse.Body.String())
	}
	var learnerBody struct {
		Data struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(learnerResponse.Body.Bytes(), &learnerBody); err != nil {
		t.Fatal(err)
	}
	if learnerBody.Data.Total != 1 || len(learnerBody.Data.Items) != 1 || learnerBody.Data.Items[0].ID != bankA.ID {
		t.Fatalf("learner list leaked another bank: %s", learnerResponse.Body.String())
	}
	if response := callBankGet(t, h, ownerA.ID, "user", bankB.ID); response.Code != http.StatusNotFound {
		t.Fatalf("learner should not get another bank: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callBankList(t, h, ownerA.ID, "admin"); response.Code != http.StatusOK ||
		!containsBankIDs(response.Body.Bytes(), bankA.ID, bankB.ID) {
		t.Fatalf("admin should retain all-bank view: status=%d body=%s", response.Code, response.Body.String())
	}
}

func containsBankIDs(body []byte, ids ...int64) bool {
	var decoded map[string]interface{}
	if json.Unmarshal(body, &decoded) != nil {
		return false
	}
	encoded, _ := json.Marshal(decoded)
	for _, id := range ids {
		if !strings.Contains(string(encoded), `"id":`+strconv.FormatInt(id, 10)) {
			return false
		}
	}
	return true
}
