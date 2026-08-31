-- Answer alignment and per-question review state.  This migration is additive;
-- raw model output and the original answer records remain untouched.
ALTER TABLE import_directory_entries ADD COLUMN canonical_question_type TEXT NOT NULL DEFAULT '';
ALTER TABLE import_directory_entries ADD COLUMN section_scope_id TEXT NOT NULL DEFAULT '';
ALTER TABLE import_directory_entries ADD COLUMN sequence_in_scope INTEGER NOT NULL DEFAULT 0;

ALTER TABLE import_answer_records ADD COLUMN canonical_question_type TEXT NOT NULL DEFAULT '';
ALTER TABLE import_answer_records ADD COLUMN section_scope_id TEXT NOT NULL DEFAULT '';
ALTER TABLE import_answer_records ADD COLUMN link_status TEXT NOT NULL DEFAULT 'unresolved';

ALTER TABLE candidate_questions ADD COLUMN canonical_question_type TEXT NOT NULL DEFAULT '';
ALTER TABLE candidate_questions ADD COLUMN section_scope_id TEXT NOT NULL DEFAULT '';
ALTER TABLE candidate_questions ADD COLUMN answer_link_status TEXT NOT NULL DEFAULT 'unresolved';
ALTER TABLE candidate_questions ADD COLUMN answer_link_method TEXT;
ALTER TABLE candidate_questions ADD COLUMN answer_link_confidence REAL;
ALTER TABLE candidate_questions ADD COLUMN review_decision TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE candidate_questions ADD COLUMN exclusion_reason TEXT;
ALTER TABLE candidate_questions ADD COLUMN reviewed_by INTEGER;
ALTER TABLE candidate_questions ADD COLUMN reviewed_at DATETIME;

CREATE TABLE candidate_answer_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    candidate_id INTEGER NOT NULL REFERENCES candidate_questions(id) ON DELETE CASCADE,
    answer_record_id INTEGER NOT NULL REFERENCES import_answer_records(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'proposed',
    method TEXT NOT NULL DEFAULT 'exact',
    confidence REAL NOT NULL DEFAULT 0,
    detail_json TEXT,
    run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(candidate_id, answer_record_id, run_id)
);

UPDATE import_directory_entries
SET canonical_question_type = CASE lower(replace(replace(question_type,'-','_'),' ',''))
    WHEN 'multi_choice' THEN 'multiple_choice'
    WHEN 'multiplechoice' THEN 'multiple_choice'
    WHEN 'judgement' THEN 'true_false'
    WHEN 'judgment' THEN 'true_false'
    WHEN 'true_or_false' THEN 'true_false'
    WHEN 'statement_or_true_false' THEN 'true_false'
    WHEN 'qa' THEN 'subjective'
    WHEN 'essay' THEN 'subjective'
    WHEN 'short_answer' THEN 'subjective'
    ELSE lower(replace(replace(question_type,'-','_'),' ','')) END;
UPDATE import_answer_records
SET canonical_question_type = CASE lower(replace(replace(question_type,'-','_'),' ',''))
    WHEN 'multi_choice' THEN 'multiple_choice'
    WHEN 'multiplechoice' THEN 'multiple_choice'
    WHEN 'judgement' THEN 'true_false'
    WHEN 'judgment' THEN 'true_false'
    WHEN 'true_or_false' THEN 'true_false'
    WHEN 'statement_or_true_false' THEN 'true_false'
    WHEN 'qa' THEN 'subjective'
    WHEN 'essay' THEN 'subjective'
    WHEN 'short_answer' THEN 'subjective'
    ELSE lower(replace(replace(question_type,'-','_'),' ','')) END;
UPDATE candidate_questions
SET canonical_question_type = CASE lower(replace(replace(COALESCE(question_type,''),'-','_'),' ',''))
    WHEN 'multi_choice' THEN 'multiple_choice'
    WHEN 'multiplechoice' THEN 'multiple_choice'
    WHEN 'judgement' THEN 'true_false'
    WHEN 'judgment' THEN 'true_false'
    WHEN 'true_or_false' THEN 'true_false'
    WHEN 'statement_or_true_false' THEN 'true_false'
    WHEN 'qa' THEN 'subjective'
    WHEN 'essay' THEN 'subjective'
    WHEN 'short_answer' THEN 'subjective'
    ELSE lower(replace(replace(COALESCE(question_type,''),'-','_'),' ','')) END;

CREATE INDEX idx_directory_scope ON import_directory_entries(session_id, section_scope_id, canonical_question_type, set_no, source_question_no);
CREATE INDEX idx_answer_scope ON import_answer_records(session_id, section_scope_id, canonical_question_type, set_no, source_question_no, link_status);
CREATE INDEX idx_candidate_scope ON candidate_questions(import_session_id, section_scope_id, canonical_question_type, set_no, source_question_no);
CREATE INDEX idx_candidate_review ON candidate_questions(import_session_id, review_decision, answer_link_status);
CREATE INDEX idx_answer_links_candidate ON candidate_answer_links(session_id, candidate_id, status);
