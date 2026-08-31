package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/domain"
)

func TestUpsertAnswerRecordsRestoresScopeAndIdentity(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "answers.db"), 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE import_answer_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		document_id INTEGER NOT NULL,
		run_id INTEGER,
		normalized_section_path TEXT NOT NULL DEFAULT '',
		section_path_json TEXT NOT NULL DEFAULT '[]',
		set_no INTEGER NOT NULL DEFAULT 0,
		question_type TEXT NOT NULL DEFAULT '',
		canonical_question_type TEXT NOT NULL DEFAULT '',
		section_scope_id TEXT NOT NULL DEFAULT '',
		match_scope_key TEXT NOT NULL DEFAULT '',
		source_question_no INTEGER NOT NULL,
		answer_json TEXT NOT NULL,
		analysis TEXT,
		page_start INTEGER NOT NULL,
		page_end INTEGER NOT NULL,
		source_kind TEXT NOT NULL DEFAULT 'answer',
		link_status TEXT NOT NULL DEFAULT 'unresolved',
		source_identity TEXT NOT NULL DEFAULT '',
		UNIQUE(session_id, document_id, normalized_section_path, set_no, question_type, source_question_no, source_kind, page_start)
	)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	answer := domain.VisionAnswer{
		SectionScopeID:        "依法执教与教师违法(侵权)行为",
		CanonicalQuestionType: "single_choice",
		SourceQuestionNo:      1,
		Answer:                []string{"C"},
		SourcePageStart:       134,
		SourcePageEnd:         134,
	}
	if err := repo.UpsertAnswerRecords(context.Background(), 8, 1, 12, []domain.VisionAnswer{answer}); err != nil {
		t.Fatal(err)
	}
	var normalized, section, questionType, canonical, scope, identity string
	if err := db.QueryRow(`SELECT normalized_section_path,section_path_json,question_type,canonical_question_type,section_scope_id,source_identity FROM import_answer_records`).Scan(&normalized, &section, &questionType, &canonical, &scope, &identity); err != nil {
		t.Fatal(err)
	}
	if normalized == "" || scope == "" || questionType != "single_choice" || canonical != "single_choice" || !strings.Contains(section, "依法执教") || !strings.Contains(identity, "single_choice") {
		t.Fatalf("answer identity was not persisted safely: normalized=%q section=%q type=%q canonical=%q scope=%q identity=%q", normalized, section, questionType, canonical, scope, identity)
	}
	if err := repo.UpsertAnswerRecords(context.Background(), 8, 1, 13, []domain.VisionAnswer{answer}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM import_answer_records`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("repeated upsert should remain idempotent, got %d rows", count)
	}
}
