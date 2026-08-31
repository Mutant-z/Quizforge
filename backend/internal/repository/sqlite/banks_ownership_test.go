package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/security"
)

func TestBankOwnershipScopesRelatedResources(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "banks.db"), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "/Users/mutant/Documents/project/QuizForge/backend/migrations"); err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	hash, err := security.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	ownerA, err := repo.CreateUser(ctx, "bank-owner-a", "bank-owner-a@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	ownerB, err := repo.CreateUser(ctx, "bank-owner-b", "bank-owner-b@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	bankA, err := repo.CreateBank(ctx, "A 的题库", "", "private", ownerA.ID)
	if err != nil {
		t.Fatal(err)
	}
	bankB, err := repo.CreateBank(ctx, "B 的题库", "", "private", ownerB.ID)
	if err != nil {
		t.Fatal(err)
	}

	visible, total, err := repo.ListBanksForUser(ctx, ownerA.ID, 1, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(visible) != 1 || visible[0].ID != bankA.ID {
		t.Fatalf("owner A saw banks outside its scope: total=%d banks=%+v", total, visible)
	}
	if _, err := repo.GetBankForUser(ctx, bankB.ID, ownerA.ID); err == nil {
		t.Fatal("owner A should not read owner B's bank")
	}

	subjectB, err := repo.CreateSubject(ctx, bankB.ID, "B 科目")
	if err != nil {
		t.Fatal(err)
	}
	chapterB, err := repo.CreateChapter(ctx, subjectB.ID, nil, "B 章节", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetSubjectForUser(ctx, subjectB.ID, ownerA.ID); err == nil {
		t.Fatal("owner A should not read owner B's subject")
	}
	if _, err := repo.GetChapterForUser(ctx, chapterB.ID, ownerA.ID); err == nil {
		t.Fatal("owner A should not read owner B's chapter")
	}
}
