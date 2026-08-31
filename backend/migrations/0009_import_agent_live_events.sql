-- Repair V2 sessions created before terminal model/pipeline failures were
-- propagated from document jobs to the conversation-level run.

INSERT INTO import_session_messages(session_id,run_id,role,message_type,content,status)
SELECT s.id,s.active_run_id,'assistant','agent_run',
       CASE
         WHEN EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status='needs_model_configuration')
           THEN '视觉模型尚未配置，当前识别已停止。请配置支持图片输入的默认模型后重试。'
         ELSE '视觉识别发生错误，当前运行已停止。请查看执行轨迹后定向重试。'
       END,
       'failed'
FROM import_sessions s
WHERE s.active_run_id IS NOT NULL
  AND s.status IN ('receiving_files','analyzing','extracting')
  AND EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status IN ('needs_model_configuration','needs_attention','failed'))
  AND NOT EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status NOT IN ('draft_ready','needs_attention','needs_model_configuration','failed','cancelled'))
  AND NOT EXISTS(SELECT 1 FROM import_session_messages m WHERE m.session_id=s.id AND m.run_id=s.active_run_id AND m.role='assistant' AND m.message_type='agent_run');

INSERT INTO import_events(session_id,run_id,message_id,event_type,stage,agent_role,summary)
SELECT s.id,s.active_run_id,
       (SELECT m.id FROM import_session_messages m WHERE m.session_id=s.id AND m.run_id=s.active_run_id AND m.role='assistant' AND m.message_type='agent_run' ORDER BY m.id LIMIT 1),
       'error',
       CASE
         WHEN EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status='needs_model_configuration') THEN 'needs_model_configuration'
         ELSE 'needs_attention'
       END,
       'ImportCoordinator',
       CASE
         WHEN EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status='needs_model_configuration')
           THEN '默认模型不支持图片输入或尚未配置，视觉识别已暂停'
         ELSE '视觉识别失败，当前运行已停止'
       END
FROM import_sessions s
WHERE s.active_run_id IS NOT NULL
  AND s.status IN ('receiving_files','analyzing','extracting')
  AND EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status IN ('needs_model_configuration','needs_attention','failed'))
  AND NOT EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status NOT IN ('draft_ready','needs_attention','needs_model_configuration','failed','cancelled'));

UPDATE import_session_messages
SET status='failed',
    content=CASE
      WHEN EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=import_session_messages.session_id AND j.status='needs_model_configuration')
        THEN '视觉模型尚未配置，当前识别已停止。请配置支持图片输入的默认模型后重试。'
      ELSE '视觉识别发生错误，当前运行已停止。请查看执行轨迹后定向重试。'
    END
WHERE message_type='agent_run'
  AND status='running'
  AND EXISTS(
    SELECT 1 FROM import_sessions s
    WHERE s.id=import_session_messages.session_id
      AND s.active_run_id=import_session_messages.run_id
      AND s.status IN ('receiving_files','analyzing','extracting')
      AND EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status IN ('needs_model_configuration','needs_attention','failed'))
      AND NOT EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status NOT IN ('draft_ready','needs_attention','needs_model_configuration','failed','cancelled'))
  );

UPDATE import_runs
SET status='failed',
    error_code='VISION_PIPELINE_TERMINATED',
    error_message='文档任务已终止，运行状态已由实时事件迁移修复',
    finished_at=datetime('now')
WHERE id IN (
  SELECT s.active_run_id FROM import_sessions s
  WHERE s.active_run_id IS NOT NULL
    AND s.status IN ('receiving_files','analyzing','extracting')
    AND EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status IN ('needs_model_configuration','needs_attention','failed'))
    AND NOT EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=s.id AND j.status NOT IN ('draft_ready','needs_attention','needs_model_configuration','failed','cancelled'))
);

UPDATE import_sessions
SET status=CASE
      WHEN EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=import_sessions.id AND j.status='needs_model_configuration')
        THEN 'needs_model_configuration'
      ELSE 'needs_attention'
    END,
    active_run_id=NULL,
    blocking_issue_count=MAX(blocking_issue_count,1),
    updated_at=datetime('now')
WHERE active_run_id IS NOT NULL
  AND status IN ('receiving_files','analyzing','extracting')
  AND EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=import_sessions.id AND j.status IN ('needs_model_configuration','needs_attention','failed'))
  AND NOT EXISTS(SELECT 1 FROM import_jobs j WHERE j.session_id=import_sessions.id AND j.status NOT IN ('draft_ready','needs_attention','needs_model_configuration','failed','cancelled'));
