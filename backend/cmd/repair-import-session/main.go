package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/database"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

// repair-import-session runs the same deterministic directory rebuild,
// identity normalization, answer relink and strict closeout used by the API.
// It never invokes a model and is safe to repeat.
func main() {
	sessionID := flag.Int64("session", 0, "import session id")
	dbPath := flag.String("db", "data/app.db", "SQLite database path")
	flag.Parse()
	if *sessionID <= 0 {
		log.Fatal("--session must be positive")
	}
	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	cfg := config.Load()
	db, err := database.Open(absDB, cfg.DB.BusyTimeoutMs)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	importer := service.NewImportService(sqlite.NewRepository(db), cfg)
	mismatches, err := importer.AuditSessionCatalogCoverage(context.Background(), *sessionID)
	if err != nil {
		log.Fatal(err)
	}
	recovered, err := importer.RecoverCatalogGapsFromIdenticalDocumentSessions(context.Background(), *sessionID, mismatches)
	if err != nil {
		log.Fatal(err)
	}
	recoveredAnswers, err := importer.RecoverAnswerGapsFromIdenticalDocumentSessions(context.Background(), *sessionID, mismatches)
	if err != nil {
		log.Fatal(err)
	}
	if err := importer.RelinkSessionAnswers(context.Background(), *sessionID, 0); err != nil {
		log.Fatal(err)
	}
	if _, err := importer.AuditSessionCatalogCoverage(context.Background(), *sessionID); err != nil {
		log.Fatal(err)
	}
	var expected, unique, complete, blocked int
	if err := db.QueryRow(`SELECT COALESCE(expected_count,0),unique_count,complete_count,blocking_issue_count FROM import_sessions WHERE id=?`, *sessionID).Scan(&expected, &unique, &complete, &blocked); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("session=%d recovered_questions=%d recovered_answers=%d expected=%d unique=%d complete=%d blocked=%d\n", *sessionID, recovered, recoveredAnswers, expected, unique, complete, blocked)
}
