-- Immutable question origins and page-level evidence used by targeted repair.
-- This migration is additive: candidate_sources and the legacy page columns
-- remain available as a compatibility projection during the dual-write phase.
CREATE TABLE source_question_origins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    origin_uid TEXT NOT NULL,
    session_id INTEGER NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    candidate_id INTEGER REFERENCES candidate_questions(id) ON DELETE SET NULL,
    canonical_question_id INTEGER REFERENCES canonical_questions(id) ON DELETE SET NULL,
    question_document_id INTEGER REFERENCES documents(id) ON DELETE SET NULL,
    attachment_revision INTEGER NOT NULL DEFAULT 0,
    document_file_hash TEXT NOT NULL DEFAULT '',
    source_identity TEXT NOT NULL DEFAULT '',
    identity_version INTEGER NOT NULL DEFAULT 1,
    original_question_label TEXT NOT NULL DEFAULT '',
    original_question_no INTEGER NOT NULL DEFAULT 0,
    sequence_in_scope INTEGER NOT NULL DEFAULT 0,
    section_path_json TEXT NOT NULL DEFAULT '[]',
    section_scope_id TEXT NOT NULL DEFAULT '',
    set_no INTEGER NOT NULL DEFAULT 0,
    raw_question_type TEXT NOT NULL DEFAULT '',
    canonical_question_type TEXT NOT NULL DEFAULT '',
    identity_confidence TEXT NOT NULL DEFAULT 'derived',
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_id, origin_uid)
);

CREATE UNIQUE INDEX idx_source_origins_candidate
    ON source_question_origins(session_id, candidate_id) WHERE candidate_id IS NOT NULL;
CREATE INDEX idx_source_origins_canonical
    ON source_question_origins(canonical_question_id);

CREATE TABLE source_evidence_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    origin_id INTEGER NOT NULL REFERENCES source_question_origins(id) ON DELETE CASCADE,
    candidate_id INTEGER REFERENCES candidate_questions(id) ON DELETE SET NULL,
    canonical_question_id INTEGER REFERENCES canonical_questions(id) ON DELETE SET NULL,
    source_role TEXT NOT NULL CHECK(source_role IN ('question','answer','explanation')),
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    attachment_revision INTEGER NOT NULL DEFAULT 0,
    document_file_hash TEXT NOT NULL DEFAULT '',
    page_start INTEGER NOT NULL,
    page_end INTEGER NOT NULL,
    regions_json TEXT NOT NULL DEFAULT '[]',
    marker_top INTEGER,
    marker_bottom INTEGER,
    anchor_text TEXT NOT NULL DEFAULT '',
    page_asset_render_version TEXT NOT NULL DEFAULT '',
    page_asset_sha256 TEXT NOT NULL DEFAULT '',
    directory_entry_id INTEGER REFERENCES import_directory_entries(id) ON DELETE SET NULL,
    agent_unit_id INTEGER REFERENCES import_agent_units(id) ON DELETE SET NULL,
    agent_run_id INTEGER REFERENCES import_runs(id) ON DELETE SET NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    extraction_confidence REAL NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','superseded','invalid')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    CHECK(page_start > 0 AND page_end >= page_start)
);

CREATE UNIQUE INDEX idx_source_spans_active_identity
    ON source_evidence_spans(origin_id,source_role,document_id,page_start,page_end,content_hash)
    WHERE status='active';
CREATE INDEX idx_source_spans_origin_role ON source_evidence_spans(origin_id,source_role,status);
CREATE INDEX idx_source_spans_document_page ON source_evidence_spans(document_id,page_start,page_end,status);
CREATE INDEX idx_source_spans_candidate ON source_evidence_spans(candidate_id,status);
CREATE INDEX idx_source_spans_canonical ON source_evidence_spans(canonical_question_id,status);

ALTER TABLE import_runs ADD COLUMN result_status TEXT NOT NULL DEFAULT 'unchanged';
ALTER TABLE import_runs ADD COLUMN model_success_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN candidate_updated_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN resolved_candidate_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN remaining_blocked_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN result_summary_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE import_problems ADD COLUMN origin_id INTEGER REFERENCES source_question_origins(id) ON DELETE SET NULL;
ALTER TABLE import_problems ADD COLUMN source_span_id INTEGER REFERENCES source_evidence_spans(id) ON DELETE SET NULL;
ALTER TABLE import_problems ADD COLUMN resolution_hint TEXT NOT NULL DEFAULT 'manual_review';
ALTER TABLE import_problems ADD COLUMN resolved_at DATETIME;

ALTER TABLE candidate_revision_history ADD COLUMN provenance_before_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE candidate_revision_history ADD COLUMN provenance_after_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE candidate_revision_history ADD COLUMN validation_before_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE candidate_revision_history ADD COLUMN validation_after_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE candidate_revision_history ADD COLUMN problems_before_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE candidate_revision_history ADD COLUMN problems_after_json TEXT NOT NULL DEFAULT '[]';

CREATE INDEX idx_import_problems_origin ON import_problems(origin_id,status);
CREATE INDEX idx_import_problems_span ON import_problems(source_span_id,status);

CREATE TRIGGER trg_import_problem_provenance_after_insert
AFTER INSERT ON import_problems
BEGIN
  UPDATE import_problems
  SET origin_id=COALESCE(NEW.origin_id,(SELECT o.id FROM source_question_origins o WHERE o.session_id=NEW.session_id AND o.candidate_id=NEW.candidate_id LIMIT 1)),
      resolution_hint=CASE
        WHEN NEW.problem_type IN ('QUESTION_INVALID','QUESTION_MISSING','OPTIONS_MISSING') THEN 'retry_question'
        WHEN NEW.problem_type IN ('ANSWER_MISSING','ANSWER_UNRESOLVED','ANSWER_CONFLICT') THEN 'retry_answer'
        WHEN NEW.problem_type='ANSWER_SOURCE_IDENTITY_INVALID' THEN 'normalize_identity'
        ELSE 'manual_review' END
  WHERE id=NEW.id;
END;

CREATE TRIGGER trg_question_source_provenance_after_insert
AFTER INSERT ON question_sources
WHEN NEW.candidate_id IS NOT NULL
BEGIN
  UPDATE source_question_origins SET canonical_question_id=NEW.canonical_question_id,updated_at=datetime('now')
  WHERE candidate_id=NEW.candidate_id;
  UPDATE source_evidence_spans SET canonical_question_id=NEW.canonical_question_id,updated_at=datetime('now')
  WHERE candidate_id=NEW.candidate_id;
END;

-- Idempotent historical backfill. The UID is persisted once and is never
-- recomputed by recognition or repair workers.
INSERT OR IGNORE INTO source_question_origins(
    origin_uid,session_id,candidate_id,question_document_id,attachment_revision,
    document_file_hash,source_identity,original_question_label,original_question_no,
    sequence_in_scope,section_path_json,section_scope_id,set_no,raw_question_type,
    canonical_question_type,identity_confidence,status
)
SELECT
    'qf-origin:' || c.import_session_id || ':' || c.id,
    c.import_session_id,c.id,c.document_id,COALESCE(sd.attachment_revision,0),
    COALESCE(d.file_hash,''),COALESCE(c.source_identity,''),
    CASE WHEN c.source_question_no>0 THEN CAST(c.source_question_no AS TEXT) ELSE '' END,
    c.source_question_no,COALESCE(de.sequence_in_scope,c.source_question_no,0),
    COALESCE(c.section_path_json,'[]'),COALESCE(c.section_scope_id,''),c.set_no,
    COALESCE(c.question_type,''),COALESCE(c.canonical_question_type,''),'derived','active'
FROM candidate_questions c
LEFT JOIN import_session_documents sd ON sd.session_id=c.import_session_id AND sd.document_id=c.document_id
LEFT JOIN documents d ON d.id=c.document_id
LEFT JOIN import_directory_entries de ON de.session_id=c.import_session_id
    AND de.document_id=c.document_id AND de.source_identity=c.source_identity
    AND de.content_role='question' AND de.status='active'
WHERE c.import_session_id IS NOT NULL;

INSERT OR IGNORE INTO source_evidence_spans(
    origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,
    page_start,page_end,regions_json,marker_top,marker_bottom,anchor_text,
    directory_entry_id,agent_unit_id,agent_run_id,content_hash,extraction_confidence,status
)
SELECT o.id,c.id,'question',c.document_id,o.attachment_revision,o.document_file_hash,
    COALESCE(c.question_page_start,c.page_start),COALESCE(c.question_page_end,c.page_end),
    CASE WHEN COALESCE(de.region_json,'')='' THEN '[]' ELSE '[' || de.region_json || ']' END,
    de.marker_top,de.marker_bottom,COALESCE(de.anchor,''),de.id,de.unit_id,c.import_run_id,
    COALESCE(c.source_identity,'') || ':question:' || COALESCE(c.question_page_start,c.page_start) || ':' || COALESCE(c.question_page_end,c.page_end),
    COALESCE(c.visual_confidence,c.confidence,0),'active'
FROM candidate_questions c
JOIN source_question_origins o ON o.session_id=c.import_session_id AND o.candidate_id=c.id
LEFT JOIN import_directory_entries de ON de.session_id=c.import_session_id
    AND de.document_id=c.document_id AND de.source_identity=c.source_identity
    AND de.content_role='question' AND de.status='active'
WHERE COALESCE(c.question_page_start,c.page_start,0)>0
  AND COALESCE(c.question_page_end,c.page_end,0)>=COALESCE(c.question_page_start,c.page_start,0);

INSERT OR IGNORE INTO source_evidence_spans(
    origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,
    page_start,page_end,regions_json,directory_entry_id,content_hash,extraction_confidence,status
)
SELECT o.id,cs.candidate_id,CASE WHEN cs.source_role='explanation' THEN 'explanation' ELSE 'answer' END,
    cs.document_id,COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,''),cs.page_start,cs.page_end,
    '[]',cs.directory_entry_id,
    COALESCE(cs.source_json,'') || ':' || cs.source_role || ':' || cs.page_start || ':' || cs.page_end,
    0.5,'active'
FROM candidate_sources cs
JOIN source_question_origins o ON o.session_id=cs.session_id AND o.candidate_id=cs.candidate_id
LEFT JOIN import_session_documents sd ON sd.session_id=cs.session_id AND sd.document_id=cs.document_id
LEFT JOIN documents d ON d.id=cs.document_id
WHERE cs.source_role IN ('answer','explanation') AND cs.page_start>0 AND cs.page_end>=cs.page_start;

-- Preserve still-unlinked answer locations as candidate evidence. Multiple
-- possible records are intentionally retained: targeted repair may compare
-- them, while strict closeout refuses to pick one without unique identity.
INSERT OR IGNORE INTO source_evidence_spans(
    origin_id,candidate_id,source_role,document_id,attachment_revision,document_file_hash,
    page_start,page_end,regions_json,agent_run_id,content_hash,extraction_confidence,status
)
SELECT o.id,c.id,CASE WHEN ar.source_kind='explanation' THEN 'explanation' ELSE 'answer' END,
    ar.document_id,COALESCE(sd.attachment_revision,0),COALESCE(d.file_hash,''),
    ar.page_start,ar.page_end,'[]',ar.run_id,'answer-record:' || ar.id,0.5,'active'
FROM candidate_questions c
JOIN source_question_origins o ON o.session_id=c.import_session_id AND o.candidate_id=c.id
JOIN import_answer_records ar ON ar.session_id=c.import_session_id
    AND ar.set_no=c.set_no AND ar.source_question_no=c.source_question_no
    AND ar.canonical_question_type=c.canonical_question_type
LEFT JOIN import_session_documents sd ON sd.session_id=c.import_session_id AND sd.document_id=ar.document_id
LEFT JOIN documents d ON d.id=ar.document_id
WHERE ar.page_start>0 AND ar.page_end>=ar.page_start;

UPDATE import_problems
SET origin_id=(SELECT o.id FROM source_question_origins o
               WHERE o.session_id=import_problems.session_id
                 AND o.candidate_id=import_problems.candidate_id LIMIT 1),
    resolution_hint=CASE
      WHEN problem_type IN ('QUESTION_INVALID','QUESTION_MISSING','OPTIONS_MISSING') THEN 'retry_question'
      WHEN problem_type IN ('ANSWER_MISSING','ANSWER_UNRESOLVED','ANSWER_CONFLICT') THEN 'retry_answer'
      WHEN problem_type='ANSWER_SOURCE_IDENTITY_INVALID' THEN 'normalize_identity'
      ELSE 'manual_review' END
WHERE candidate_id IS NOT NULL;

UPDATE source_question_origins
SET canonical_question_id=(SELECT qs.canonical_question_id FROM question_sources qs
                           WHERE qs.candidate_id=source_question_origins.candidate_id
                           ORDER BY qs.id DESC LIMIT 1)
WHERE candidate_id IN (SELECT candidate_id FROM question_sources WHERE candidate_id IS NOT NULL);
UPDATE source_evidence_spans
SET canonical_question_id=(SELECT o.canonical_question_id FROM source_question_origins o WHERE o.id=source_evidence_spans.origin_id)
WHERE origin_id IN (SELECT id FROM source_question_origins WHERE canonical_question_id IS NOT NULL);
