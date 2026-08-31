package sqlite

import (
	"context"
	"database/sql"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// ImportStateRepairReport describes the non-destructive startup consistency
// check. Repairs are opt-in because an operator may want to inspect the report
// before changing session projections.
type ImportStateRepairReport struct {
	SessionsChecked       int
	StaleActiveRuns       int
	CounterMismatches     int
	StaleRunningMessages  int
	ScannedFlagsRechecked int
	RepairsApplied        int
}

// AuditImportState checks run ownership, derived session counters, stale live
// messages, and the durable scanned-PDF marker. It never deletes candidates or
// changes review_decision. Set apply=true only after reviewing the dry-run
// report.
func (r *Repository) AuditImportState(ctx context.Context, apply bool) (ImportStateRepairReport, error) {
	var report ImportStateRepairReport
	rows, err := r.db.QueryContext(ctx, `SELECT id,active_run_id,status,expected_count,unique_count,complete_count,blocking_issue_count FROM import_sessions WHERE archived_at IS NULL ORDER BY id`)
	if err != nil {
		return report, err
	}
	type sessionState struct {
		id, activeRunID            int64
		activeValid                bool
		status                     string
		expected, unique, complete int
		blocking                   int
		expectedValid              bool
	}
	states := []sessionState{}
	for rows.Next() {
		var state sessionState
		var active, expected sql.NullInt64
		if err := rows.Scan(&state.id, &active, &state.status, &expected, &state.unique, &state.complete, &state.blocking); err != nil {
			rows.Close()
			return report, err
		}
		if active.Valid {
			state.activeValid, state.activeRunID = true, active.Int64
		}
		if expected.Valid {
			state.expectedValid, state.expected = true, int(expected.Int64)
		}
		states = append(states, state)
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	if err := rows.Err(); err != nil {
		return report, err
	}

	for _, state := range states {
		report.SessionsChecked++
		activeLive := false
		if state.activeValid {
			var runStatus string
			runErr := r.db.QueryRowContext(ctx, `SELECT status FROM import_runs WHERE id=? AND session_id=?`, state.activeRunID, state.id).Scan(&runStatus)
			activeLive = runErr == nil && (runStatus == "queued" || runStatus == "running" || runStatus == "merging")
			if !activeLive {
				report.StaleActiveRuns++
				if apply {
					if _, err := r.db.ExecContext(ctx, `UPDATE import_sessions SET active_run_id=NULL,updated_at=datetime('now') WHERE id=? AND active_run_id=?`, state.id, state.activeRunID); err != nil {
						return report, err
					}
					report.RepairsApplied++
				}
			}
		}

		var unique, complete, blocking int
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status NOT IN ('session_duplicate','persisted','duplicate')`, state.id).Scan(&unique); err != nil {
			return report, err
		}
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_questions WHERE import_session_id=? AND status='validated'`, state.id).Scan(&complete); err != nil {
			return report, err
		}
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_problems WHERE session_id=? AND status='open' AND severity='blocking'`, state.id).Scan(&blocking); err != nil {
			return report, err
		}
		hasProjection := unique > 0 || complete > 0 || blocking > 0 || state.expectedValid
		matches := state.unique == unique && state.complete == complete && state.blocking == blocking && (!state.expectedValid || state.expected == unique)
		if hasProjection && !matches {
			report.CounterMismatches++
			if apply && !activeLive {
				status := domain.ImportStatusNeedsAttention
				if state.expectedValid && state.expected == unique && unique == complete && blocking == 0 {
					status = domain.ImportStatusDraftReady
				}
				if _, err := r.db.ExecContext(ctx, `UPDATE import_sessions SET status=?,unique_count=?,complete_count=?,blocking_issue_count=?,updated_at=datetime('now') WHERE id=? AND active_run_id IS NULL`, status, unique, complete, blocking, state.id); err != nil {
					return report, err
				}
				report.RepairsApplied++
			}
		}

		messageRows, err := r.db.QueryContext(ctx, `SELECT m.id,COALESCE(r.status,'') FROM import_session_messages m LEFT JOIN import_runs r ON r.id=m.run_id WHERE m.session_id=? AND m.status='running'`, state.id)
		if err != nil {
			return report, err
		}
		for messageRows.Next() {
			var messageID int64
			var runStatus string
			if err := messageRows.Scan(&messageID, &runStatus); err != nil {
				messageRows.Close()
				return report, err
			}
			if runStatus == "queued" || runStatus == "running" || runStatus == "merging" {
				continue
			}
			report.StaleRunningMessages++
			if apply {
				if _, err := r.db.ExecContext(ctx, `UPDATE import_session_messages SET status='completed' WHERE id=? AND status='running'`, messageID); err != nil {
					messageRows.Close()
					return report, err
				}
				report.RepairsApplied++
			}
		}
		if err := messageRows.Close(); err != nil {
			return report, err
		}
		if err := messageRows.Err(); err != nil {
			return report, err
		}
	}

	// Recompute this flag from durable page provenance. Successful OCR must
	// still leave a scanned document marked as scanned.
	if _, err := r.db.ExecContext(ctx, `UPDATE documents SET is_scanned=CASE WHEN (SELECT COUNT(*) FROM document_pages p WHERE p.document_id=documents.id AND (p.needs_ocr=1 OR p.text_source IN ('ocr','none') OR p.ocr_status IN ('required','failed'))) >= MAX(1,(SELECT COUNT(*) FROM document_pages p2 WHERE p2.document_id=documents.id)*0.2) THEN 1 ELSE 0 END WHERE EXISTS (SELECT 1 FROM document_pages p3 WHERE p3.document_id=documents.id)`); err != nil {
		return report, err
	}
	report.ScannedFlagsRechecked = 1
	return report, nil
}
