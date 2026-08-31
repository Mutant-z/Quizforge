package service

import (
	"context"
	"encoding/json"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// recoverAnswersFromCompletedUnits replays the durable output of every
// completed answer-extraction unit into the current answer projection. The
// old normalized-path key could collapse distinct subjective blocks that share
// a page and question number before relinking. The unit JSON is the raw,
// auditable model result; the active answer directory is the authority used to
// normalize its identity.
func (s *ImportService) recoverAnswersFromCompletedUnits(ctx context.Context, sessionID, runID int64) (int, error) {
	directories, err := s.loadAllAnswerDirectoryEntries(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	rows, err := s.repo.DB().QueryContext(ctx, `
		SELECT u.id,COALESCE(u.document_id,j.document_id),COALESCE(u.run_id,0),COALESCE(u.output_json,'')
		FROM import_agent_units u
		JOIN import_jobs j ON j.id=u.import_job_id
		WHERE j.session_id=? AND u.unit_type='extract_answer' AND u.status='completed'
		  AND trim(COALESCE(u.output_json,''))<>''
		ORDER BY COALESCE(u.run_id,0),u.id`, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	recovered := 0
	for rows.Next() {
		var unitID, documentID, unitRunID int64
		var raw string
		if err := rows.Scan(&unitID, &documentID, &unitRunID, &raw); err != nil {
			return recovered, err
		}
		var payload answerPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil || len(payload.Answers) == 0 {
			continue
		}
		effectiveRunID := runID
		if unitRunID > 0 {
			effectiveRunID = unitRunID
		}
		if effectiveRunID == 0 {
			effectiveRunID = unitID
		}
		normalized := make([]domain.VisionAnswer, 0, len(payload.Answers))
		for _, answer := range payload.Answers {
			entry, _, ok := normalizeVisionAnswerIdentity(answer, directories[documentID])
			if !ok {
				// Keep the completed unit untouched for audit. An answer with no
				// provable directory identity must not become a cross-topic row.
				continue
			}
			normalized = append(normalized, entry)
		}
		if len(normalized) == 0 {
			continue
		}
		if err := s.repo.UpsertAnswerRecords(ctx, sessionID, documentID, effectiveRunID, normalized); err != nil {
			return recovered, err
		}
		recovered += len(normalized)
	}
	if err := rows.Err(); err != nil {
		return recovered, err
	}
	return recovered, nil
}
