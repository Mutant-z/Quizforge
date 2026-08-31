-- vision-agent.v1.1 performance telemetry. Existing v1 rows keep their strategy.
ALTER TABLE import_runs ADD COLUMN strategy_version TEXT NOT NULL DEFAULT 'vision-agent.v1';
ALTER TABLE import_runs ADD COLUMN current_stage TEXT;
ALTER TABLE import_runs ADD COLUMN completed_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN total_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN failed_units INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_runs ADD COLUMN last_progress_at DATETIME;
ALTER TABLE import_runs ADD COLUMN eta_seconds INTEGER;
ALTER TABLE import_runs ADD COLUMN adaptive_concurrency INTEGER NOT NULL DEFAULT 1;
ALTER TABLE import_runs ADD COLUMN degraded_reason TEXT;
ALTER TABLE import_runs ADD COLUMN updated_at DATETIME;

ALTER TABLE import_agent_units ADD COLUMN queued_at DATETIME;
ALTER TABLE import_agent_units ADD COLUMN started_at DATETIME;
ALTER TABLE import_agent_units ADD COLUMN finished_at DATETIME;
ALTER TABLE import_agent_units ADD COLUMN queue_latency_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_agent_units ADD COLUMN payload_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_agent_units ADD COLUMN result_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_agent_units ADD COLUMN image_profile TEXT;

UPDATE import_runs
SET strategy_version = COALESCE((
  SELECT j.pipeline_version
  FROM import_jobs j
  WHERE j.session_id = import_runs.session_id
  ORDER BY j.id DESC LIMIT 1
), 'vision-agent.v1');
UPDATE import_runs SET updated_at = COALESCE(finished_at, started_at, created_at);

CREATE INDEX IF NOT EXISTS idx_import_agent_units_progress
ON import_agent_units(import_job_id, pipeline_version, unit_type, status);
