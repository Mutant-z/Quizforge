import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import client, { API_BASE, authFetch as authenticatedFetch, errMsg, refreshAccessToken, responseErrorMessage } from '@/api/client'
import { Badge, Button, EmptyState, IconButton, Progress, Spinner } from '@/components/ui'
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  FileSearch,
  FileText,
  HelpCircle,
  Layers,
  MessageSquare,
  Paperclip,
  Plus,
  RefreshCw,
  Send,
  Sparkles,
  Trash2,
  UploadCloud,
  Wrench,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type {
  ImportAgentDocument,
  ImportAgentEvent,
  ImportAgentMessage,
  ImportAgentProblem,
  ImportAgentRun,
  ImportAgentSession,
  VisionDraftCandidate,
} from '@/types'

type SessionDetail = {
  session: ImportAgentSession
  documents: ImportAgentDocument[]
  problems: ImportAgentProblem[]
  active_run?: ImportAgentRun
  draft_stats?: {
    total_count: number
    included_count: number
    excluded_count: number
    pending_review_count: number
    unresolved_answer_count: number
    needs_review_count: number
    open_problem_count: number
    open_blocking_problem_count: number
  }
}
type WorkspaceTab = 'draft' | 'documents' | 'problems' | 'units'

const statusLabel: Record<string, string> = {
  empty: '等待文件',
  receiving_files: '接收文件',
  analyzing: '分析文档',
  extracting: '提取中',
  draft_ready: '草稿待确认',
  needs_attention: '需要处理',
  needs_model_configuration: '需要视觉模型',
  committing: '提交中',
  embedding: '生成索引',
  completed: '已完成',
  cancelled: '已取消',
}

const statusTone: Record<string, 'neutral' | 'primary' | 'success' | 'danger' | 'warning' | 'info'> = {
  empty: 'neutral',
  receiving_files: 'info',
  analyzing: 'info',
  extracting: 'primary',
  draft_ready: 'warning',
  needs_attention: 'danger',
  needs_model_configuration: 'warning',
  committing: 'primary',
  embedding: 'info',
  completed: 'success',
  cancelled: 'neutral',
}

const roleLabel: Record<string, string> = {
  question_book: '题本',
  answer_book: '答案册',
  explanation_book: '解析册',
  mixed: '题目与答案混合',
  supplement: '补充卷',
  unknown: '待确认',
}

const stageLabel: Record<string, string> = {
  receiving_files: '附件已入队',
  analyzing: '启动分析',
  model_preflight: '检查视觉模型',
  document_inspection: '读取物理页',
  rendering: '生成页面图像',
  profiling: '识别文档结构',
  profile: '文档画像',
  directory_building: '建立全局目录',
  directory: '目录识别',
  catalog: '页面画像与目录识别',
  parallel_extraction: '题目与答案并行提取',
  review_question: '高分辨率复核',
  extracting_questions: '提取题目',
  extract_question: '题目页窗识别',
  extracting_answers: '提取答案与解析',
  extract_answer: '答案页窗识别',
  linking: '对齐题目与答案',
  validating: '完整性校验',
  draft_ready: '草稿已生成',
  needs_attention: '需要处理',
  needs_model_configuration: '需要配置视觉模型',
}

const messageTypeConfig: Record<string, { label: string; tone: 'primary' | 'neutral' | 'success' | 'warning' | 'danger' | 'info' }> = {
  text: { label: '文本对话', tone: 'neutral' },
  agent_run: { label: '执行单元', tone: 'primary' },
  clarification: { label: '需要确认', tone: 'warning' },
  action_card: { label: '对齐操作', tone: 'info' },
  summary: { label: '阶段汇总', tone: 'success' },
  error: { label: '异常提示', tone: 'danger' },
}

function parseMetadata(raw?: string): Record<string, unknown> {
  if (!raw) return {}
  try {
    return JSON.parse(raw)
  } catch {
    return {}
  }
}

function DocumentPage({ sessionId, documentId, page, sourceSpanId }: { sessionId: number; documentId: number; page: number; sourceSpanId?: number }) {
  const [src, setSrc] = useState('')
  const [visible, setVisible] = useState(false)
  const [loading, setLoading] = useState(false)
  const frameRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const node = frameRef.current
    if (!node) return
    if (!('IntersectionObserver' in window)) {
      setVisible(true)
      return
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setVisible(true)
          observer.disconnect()
        }
      },
      { rootMargin: '600px 0px' },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!visible || page <= 0) return
    let active = true
    let url = ''
    setLoading(true)
    client
	  .get(`/import-agent/sessions/${sessionId}/documents/${documentId}/pages/${page}`, { responseType: 'blob', params: sourceSpanId ? { source_span_id: sourceSpanId } : undefined })
      .then((response) => {
        if (!active) return
        url = URL.createObjectURL(response.data)
        setSrc(url)
      })
      .catch(() => undefined)
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
      if (url) URL.revokeObjectURL(url)
    }
	}, [visible, sessionId, documentId, page, sourceSpanId])

  if (!visible || loading) {
    return (
      <div ref={frameRef} className="flex h-32 w-24 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-surface-secondary text-[10px] text-muted-foreground shadow-subtle">
        {visible ? <Spinner className="h-4 w-4 text-primary" /> : '滚动加载'}
      </div>
    )
  }

  return src ? (
    <img
      src={src}
      alt={`PDF 第 ${page} 页`}
      loading="lazy"
      className="h-32 w-24 shrink-0 rounded-xl border border-border/80 object-cover shadow-subtle hover:scale-105 transition-transform cursor-pointer"
	  onClick={() => window.open(src, '_blank', 'noopener,noreferrer')}
    />
  ) : (
    <div className="flex h-32 w-24 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-surface-secondary text-[10px] font-mono text-muted-foreground shadow-subtle">
      第 {page} 页
    </div>
  )
}

function formatETA(seconds?: number) {
  if (!seconds || seconds < 1) return '计算中'
  if (seconds < 60) return `约 ${seconds} 秒`
  return `约 ${Math.ceil(seconds / 60)} 分钟`
}

function elapsedSeconds(timestamp: string | undefined, now: number) {
  if (!timestamp) return 0
  return Math.max(0, Math.floor((now - new Date(timestamp).getTime()) / 1000))
}

function ToolTrace({
  events,
  documents,
  runId,
  activeRun,
  terminalStatus,
  onCancel,
  cancelling = false,
}: {
  events: ImportAgentEvent[]
  documents: ImportAgentDocument[]
  runId?: number
  activeRun?: ImportAgentRun
  terminalStatus?: string
  onCancel?: () => void
  cancelling?: boolean
}) {
  const [open, setOpen] = useState(true)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  // A trace is owned by exactly one run. Session-level events (for example
  // attachment_received) must not borrow the active run's progress state.
  const scopedEvents = events.filter((event) => runId ? event.run_id === runId : !event.run_id)
  const visibleEvents = scopedEvents.slice(-300)
  if (!visibleEvents.length) return null
  const latest = visibleEvents[visibleEvents.length - 1]
  const docName = (id?: number) => documents.find((item) => item.document_id === id)?.original_name
  // Events are durable history, not proof that a run is currently executing.
  // After a refresh the latest event can be an answer_delta/tool event from a
  // completed run (or a direct relink that has no run_id). Only a persisted
  // queued/running/merging run may keep the trace in its live state.
  const runActive = Boolean(activeRun && ['queued', 'running', 'merging'].includes(activeRun.status))
  const eventBelongsToActiveRun = Boolean(activeRun && runId === activeRun.id && latest.run_id === activeRun.id)
  const cancelled = activeRun?.status === 'cancelled' || terminalStatus === 'cancelled'
  const failed = activeRun?.status === 'failed' || terminalStatus === 'failed' || (latest.type === 'error' && (!activeRun || eventBelongsToActiveRun))
  const terminal = !runActive || latest.type === 'run_completed' || (latest.type === 'error' && (!activeRun || eventBelongsToActiveRun))
	let outcome: Record<string, number> = {}
	try { outcome = JSON.parse([...visibleEvents].reverse().find((event) => event.type === 'run_completed')?.payload_json || '{}') } catch { outcome = {} }
	const remainingBlocked = activeRun?.remaining_blocked_count ?? outcome.remaining_blocked_count
	const resultStatus = activeRun?.result_status || (remainingBlocked === 0 ? 'resolved' : remainingBlocked != null ? 'partial' : undefined)
	const outcomePartial = terminal && resultStatus && resultStatus !== 'resolved'
  const persistedTotal = activeRun?.total_units || 0
  const persistedCompleted = activeRun?.completed_units || 0
  const progress = persistedTotal > 0
    ? Math.min(1, persistedCompleted / persistedTotal)
    : latest.total && latest.total > 0 ? Math.min(1, (latest.current || 0) / latest.total) : 0
  const stage = activeRun?.current_stage || latest.stage || ''
  const current = persistedTotal > 0 ? persistedCompleted : latest.current || 0
  const total = persistedTotal > 0 ? persistedTotal : latest.total || 0
  const runningByWindow = new Map<string, ImportAgentEvent>()
  for (const event of visibleEvents) {
    if (!event.context_page_start) continue
    const key = `${event.document_id || 0}:${event.context_page_start}:${event.context_page_end || event.context_page_start}`
    if (event.type === 'tool_started' || event.type === 'tool_retrying') runningByWindow.set(key, event)
    if (event.type === 'tool_completed' || event.type === 'error') runningByWindow.delete(key)
  }
  const runningWindows = [...runningByWindow.values()].slice(-3)

  return (
	<div className={`mt-3 overflow-hidden rounded-2xl border bg-surface shadow-subtle ${outcomePartial ? 'border-warning/60' : terminal ? 'border-border/80' : 'border-primary/40 ring-1 ring-primary/10'}`}>
      <button
        type="button"
        className="flex w-full items-center justify-between p-3.5 text-left text-xs bg-surface-secondary/40 hover:bg-surface-secondary transition-colors"
        onClick={() => setOpen((value) => !value)}
      >
        <span className="flex items-center gap-2 font-bold text-foreground">
          {open ? <ChevronDown className="h-3.5 w-3.5 text-primary" /> : <ChevronRight className="h-3.5 w-3.5 text-primary" />}
          {!terminal && <span className="relative flex h-2 w-2"><span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" /><span className="relative inline-flex h-2 w-2 rounded-full bg-primary" /></span>}
		  {cancelled ? 'Agent 已取消' : failed ? 'Agent 执行失败' : terminal ? outcomePartial ? '执行完成，仍有问题' : resultStatus === 'resolved' ? '问题已解决' : 'Agent 执行完成' : 'Agent 正在工作'}{total > 0 ? ` · ${current}/${total}` : ''}
        </span>
        <span className="max-w-[55%] truncate text-[11px] text-muted-foreground font-medium">
          {latest.summary || stageLabel[latest.stage || ''] || latest.type}
        </span>
      </button>

      <div className="border-t border-border/60 bg-surface-secondary/20 px-3.5 py-1.5 text-[10px] text-muted-foreground font-mono">
        {runId ? `run_id=${runId}` : 'session_event'} · event_id={latest.event_id}
      </div>
	  {terminal && (activeRun?.model_success_count != null || outcome.model_success_count != null) && (
		<div className="grid border-t border-border bg-white text-[10px] sm:grid-cols-2">
		  <div className="border-b border-border p-3 sm:border-b-0 sm:border-r">
			<div className="mb-2 font-semibold text-foreground">执行</div>
			<div className="grid grid-cols-3 gap-3">
			  {[['计划', outcome.planned_candidates ?? total], ['模型响应', activeRun?.model_success_count ?? outcome.model_success_count ?? 0], ['调用失败', activeRun?.failed_units ?? 0]].map(([label, value]) => <div key={String(label)}><span className="block text-muted-foreground">{label}</span><b className="font-mono text-sm tabular-nums">{value}</b></div>)}
			</div>
		  </div>
		  <div className={`p-3 ${Number(remainingBlocked) > 0 ? 'bg-warning/10' : ''}`}>
			<div className="mb-2 font-semibold text-foreground">结果</div>
			<div className="grid grid-cols-3 gap-3">
			  {[['实际修复', activeRun?.candidate_updated_count ?? outcome.candidate_updated_count ?? 0], ['解除阻断', activeRun?.resolved_candidate_count ?? outcome.resolved_candidate_count ?? 0], ['仍阻断', remainingBlocked ?? 0]].map(([label, value]) => <div key={String(label)}><span className="block text-muted-foreground">{label}</span><b className="font-mono text-sm tabular-nums">{value}</b></div>)}
			</div>
		  </div>
		</div>
	  )}

      {!terminal && (
        <div className="border-t border-border/60 bg-surface px-3.5 py-2.5">
          <div className="mb-1.5 flex items-center justify-between gap-3 text-[10px]">
            <span className="flex items-center gap-1.5 font-semibold text-primary">
              <RefreshCw className="h-3 w-3 animate-spin" />
              {stageLabel[stage] || latest.agent_role || '准备下一阶段'}
            </span>
            <span className="flex items-center gap-2">
              {total ? <span className="font-mono text-muted-foreground">{current} / {total}</span> : <span className="text-muted-foreground">执行中</span>}
              {onCancel && (
                <Button
                  type="button"
                  size="xs"
                  variant="danger"
                  loading={cancelling}
                  onClick={onCancel}
                >
                  取消任务
                </Button>
              )}
            </span>
          </div>
          <Progress value={progress} />
          {activeRun && (
            <div className="mt-2 grid grid-cols-2 gap-2 text-[10px] text-muted-foreground sm:grid-cols-4">
              <span>并发 <b className="font-mono text-foreground">{activeRun.adaptive_concurrency || 1}</b></span>
              <span>预计剩余 <b className="text-foreground">{formatETA(activeRun.eta_seconds)}</b></span>
              <span>最近进展 <b className="font-mono text-foreground">{elapsedSeconds(activeRun.last_progress_at, now)}s 前</b></span>
              <span>整体 <b className="font-mono text-foreground">{Math.round((activeRun.overall_progress || 0) * 100)}%</b> · 失败 <b className={activeRun.failed_units ? 'text-destructive' : 'text-foreground'}>{activeRun.failed_units}</b></span>
            </div>
          )}
          {activeRun?.degraded_reason && (
            <div className="mt-2 rounded-lg border border-warning/30 bg-warning/10 px-2.5 py-2 text-[10px] text-warning-foreground">
              {activeRun.degraded_reason}
            </div>
          )}
          {runningWindows.length > 0 && (
            <div className="mt-2 space-y-1 border-t border-border/60 pt-2 text-[10px] text-muted-foreground">
              {runningWindows.map((event) => (
                <div key={`${event.document_id}:${event.context_page_start}`} className="flex justify-between gap-3">
                  <span className="truncate">{event.agent_role} · 物理页 {event.context_page_start}–{event.context_page_end}</span>
                  <span className="shrink-0 font-mono">{elapsedSeconds(event.timestamp, now)}s</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {open && (
        <div className="max-h-64 divide-y divide-border/60 overflow-y-auto border-t border-border/70 p-2 space-y-1 bg-surface-secondary/20">
          {visibleEvents.map((event) => (
            <div key={event.event_id} className="grid grid-cols-[100px_1fr] gap-3 px-3 py-2 text-[11px] rounded-xl hover:bg-surface-secondary/60 transition-colors">
              <span className="font-mono font-bold text-primary truncate">
                {event.agent_role || event.stage || event.type}
              </span>
              <div className="min-w-0">
                <div className="flex items-start gap-2 text-foreground font-medium">
                  <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${event.type === 'error' ? 'bg-destructive' : event.type === 'tool_completed' || event.type === 'run_completed' ? 'bg-success' : 'bg-primary'}`} />
                  <span>{event.summary || event.type}</span>
                </div>
                {event.document_id && (
                  <div className="mt-0.5 text-muted-foreground text-[10px]">
                    {docName(event.document_id) || `文档 ${event.document_id}`}
                    {event.context_page_start ? ` · 页 ${event.context_page_start}–${event.context_page_end}` : ''}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const liveRunStatusLabel: Record<string, string> = {
  queued: '任务已排队',
  running: 'Agent 正在工作',
  merging: '正在合并执行结果',
}

function formatDuration(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return seconds % 60 ? `${minutes}m ${seconds % 60}s` : `${minutes}m`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

// Pinned live window for the run that is currently executing. The right-side
// 运行 tab owns the durable per-run history; this card answers “agent 现在在
// 干什么” inside the conversation itself, including for upload-triggered runs
// that have not produced an agent_run message yet.
function LiveRunMonitor({
  run,
  events,
  documents,
  onCancel,
  cancelling = false,
  onOpenUnits,
}: {
  run: ImportAgentRun
  events: ImportAgentEvent[]
  documents: ImportAgentDocument[]
  onCancel?: () => void
  cancelling?: boolean
  onOpenUnits?: () => void
}) {
  const [open, setOpen] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const scopedEvents = events.filter((event) => event.run_id === run.id)
  const visibleEvents = scopedEvents.slice(-150)
  const latest = visibleEvents[visibleEvents.length - 1]
  const stage = run.current_stage || latest?.stage || ''
  const total = run.total_units
  const current = run.completed_units
  const progress = total > 0 ? Math.min(1, current / total) : Math.max(0, Math.min(1, run.overall_progress))
  const docName = (id?: number) => documents.find((item) => item.document_id === id)?.original_name
  const cancellingRun = cancelling || run.cancel_requested
  const startedAt = run.started_at
  const elapsed = startedAt ? elapsedSeconds(startedAt, now) : 0

  const runningByWindow = new Map<string, ImportAgentEvent>()
  for (const event of visibleEvents) {
    if (!event.context_page_start) continue
    const key = `${event.document_id || 0}:${event.context_page_start}:${event.context_page_end || event.context_page_start}`
    if (event.type === 'tool_started' || event.type === 'tool_retrying') runningByWindow.set(key, event)
    if (event.type === 'tool_completed' || event.type === 'error') runningByWindow.delete(key)
  }
  const runningWindows = [...runningByWindow.values()].slice(-3)

  return (
    <section
      aria-live="polite"
      className="animate-slide-down overflow-hidden rounded-2xl border border-primary/40 bg-surface shadow-card ring-1 ring-primary/10"
    >
      <div className="flex items-center gap-2.5 border-b border-primary/10 bg-primary/5 px-4 py-2.5">
        <span className="relative flex h-2 w-2 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
        </span>
        <span className="shrink-0 text-xs font-bold text-foreground">
          {liveRunStatusLabel[run.status] || 'Agent 正在工作'}
        </span>
        <span className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground">
          {stageLabel[stage] || latest?.summary || '准备下一阶段'}
        </span>
        <span className="shrink-0 font-mono text-[11px] font-bold tabular-nums text-primary">
          {total > 0 ? `${current}/${total} · ${Math.round(progress * 100)}%` : run.status}
        </span>
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          title={open ? '收起实时事件' : '展开实时事件'}
          className="shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
        >
          {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </button>
      </div>

      <div className="px-4 py-3">
        <div className="mb-1.5 flex items-center justify-between gap-3 text-[10px]">
          <span className="flex items-center gap-1.5 font-semibold text-primary">
            <RefreshCw className="h-3 w-3 animate-spin" />
            {stageLabel[stage] || latest?.agent_role || '准备下一阶段'}
          </span>
          <span className="flex items-center gap-2">
            {startedAt && <span className="font-mono text-muted-foreground">已运行 {formatDuration(elapsed)}</span>}
            {onCancel && (
              <Button type="button" size="xs" variant="danger" loading={cancellingRun} onClick={onCancel}>
                {run.cancel_requested ? '取消中…' : '取消任务'}
              </Button>
            )}
          </span>
        </div>
        <Progress value={progress} />
        <div className="mt-2 grid grid-cols-2 gap-2 text-[10px] text-muted-foreground sm:grid-cols-4">
          <span>并发 <b className="font-mono text-foreground">{run.adaptive_concurrency || 1}</b></span>
          <span>预计剩余 <b className="text-foreground">{formatETA(run.eta_seconds)}</b></span>
          <span>最近进展 <b className="font-mono text-foreground">{run.last_progress_at ? `${elapsedSeconds(run.last_progress_at, now)}s 前` : '—'}</b></span>
          <span>失败 <b className={run.failed_units ? 'text-destructive' : 'text-foreground'}>{run.failed_units}</b></span>
        </div>
        {run.degraded_reason && (
          <div className="mt-2 rounded-lg border border-warning/30 bg-warning/10 px-2.5 py-2 text-[10px] text-warning-foreground">
            {run.degraded_reason}
          </div>
        )}
        {runningWindows.length > 0 && (
          <div className="mt-2 space-y-1 border-t border-border/60 pt-2 text-[10px] text-muted-foreground">
            {runningWindows.map((event) => (
              <div key={`${event.document_id}:${event.context_page_start}`} className="flex justify-between gap-3">
                <span className="truncate">{event.agent_role} · 物理页 {event.context_page_start}–{event.context_page_end}</span>
                <span className="shrink-0 font-mono">{elapsedSeconds(event.timestamp, now)}s</span>
              </div>
            ))}
          </div>
        )}
        {open && (
          <div className="mt-2 max-h-44 space-y-0.5 overflow-y-auto border-t border-border/60 pt-2">
            {[...visibleEvents].reverse().slice(0, 40).map((event) => (
              <div key={event.event_id} className="flex items-start gap-2 text-[10px] leading-relaxed">
                <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${event.type === 'error' ? 'bg-destructive' : event.type === 'tool_completed' ? 'bg-success' : 'bg-primary'}`} />
                <span className="shrink-0 font-mono font-bold text-primary">{event.agent_role || event.stage || event.type}</span>
                <span className="min-w-0 flex-1 truncate text-muted-foreground">
                  {event.summary || event.type}
                  {event.document_id ? ` · ${docName(event.document_id) || `文档 ${event.document_id}`}${event.context_page_start ? ` 页 ${event.context_page_start}–${event.context_page_end}` : ''}` : ''}
                </span>
              </div>
            ))}
            {!visibleEvents.length && <p className="text-[10px] text-muted-foreground">尚未收到该运行的实时事件。</p>}
          </div>
        )}
      </div>

      {onOpenUnits && (
        <button
          type="button"
          onClick={onOpenUnits}
          className="flex w-full items-center justify-center gap-1.5 border-t border-primary/10 bg-surface-secondary/30 px-4 py-2 text-[10px] font-bold text-primary transition-colors hover:bg-primary/5"
        >
          <Layers className="h-3 w-3" />
          查看完整运行轨迹{scopedEvents.length ? `（${scopedEvents.length} 条事件）` : ''}
        </button>
      )}
    </section>
  )
}

export default function AdminImports() {
  const [sessions, setSessions] = useState<ImportAgentSession[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [detail, setDetail] = useState<SessionDetail | null>(null)
  const [messages, setMessages] = useState<ImportAgentMessage[]>([])
  const [events, setEvents] = useState<ImportAgentEvent[]>([])
  const [draft, setDraft] = useState<VisionDraftCandidate[]>([])
  const [draftTotal, setDraftTotal] = useState(0)
  const [draftLoading, setDraftLoading] = useState(false)
  const [draftLoadError, setDraftLoadError] = useState('')
  const [draftFilter, setDraftFilter] = useState<'all' | 'pending' | 'include' | 'exclude' | 'missing_answer' | 'conflict'>('all')
  const [selectedCandidates, setSelectedCandidates] = useState<number[]>([])
  const [tab, setTab] = useState<WorkspaceTab>('draft')
  const [input, setInput] = useState('')
  const [pendingFiles, setPendingFiles] = useState<File[]>([])
  const [uploadState, setUploadState] = useState<Record<string, number>>({})
  const [preview, setPreview] = useState<Record<string, unknown> | null>(null)
  const [busy, setBusy] = useState('')
  const [traceByRun, setTraceByRun] = useState<Record<number, ImportAgentEvent[]>>({})
  const [streamState, setStreamState] = useState<'connecting' | 'connected' | 'reconnecting'>('connecting')
  const fileRef = useRef<HTMLInputElement>(null)
  const streamRef = useRef<AbortController | null>(null)
  const lastEventId = useRef(0)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const isComposingRef = useRef(false)
  const draftLoadSequence = useRef(0)
  const draftSessionRef = useRef<number | null>(null)
  const draftFullyLoadedRef = useRef(false)
  const loadedDraftVersionRef = useRef<number | null>(null)
  const sessionRefreshInFlight = useRef<Promise<unknown> | null>(null)
  const sessionRefreshID = useRef<number | null>(null)
  const refreshAgain = useRef(false)
  const refreshAgainIncludeList = useRef(false)
  const streamGeneration = useRef(0)

  const loadSessions = useCallback(async () => {
    const response = await client.get('/import-agent/sessions')
    const items: ImportAgentSession[] = response.data.data.items
    setSessions(items)
    setSelectedId((current) => current || items[0]?.id || null)
  }, [])

  const loadSession = useCallback(async (id: number) => {
    const sequence = ++draftLoadSequence.current
    const hadCompleteDraft = draftSessionRef.current === id && draftFullyLoadedRef.current
    if (draftSessionRef.current !== id) {
      draftSessionRef.current = id
      draftFullyLoadedRef.current = false
      loadedDraftVersionRef.current = null
      setDraft([])
      setDraftTotal(0)
    }
    setDraftLoadError('')
    try {
      // Summary and messages are cheap state reads. The draft pages are loaded
      // only on first entry, after a draft_version change, or after an
      // incomplete previous load; live progress events must not fan out into
      // another request for every page of a 1k+ question draft.
      const [summary, messageResponse] = await Promise.all([
        client.get(`/import-agent/sessions/${id}`),
        client.get(`/import-agent/sessions/${id}/messages`),
      ])
      if (sequence !== draftLoadSequence.current) return
      const nextDetail = summary.data.data as SessionDetail
      setDetail(nextDetail)
      setMessages(messageResponse.data.data.items)
      const serverDraftVersion = Number(nextDetail.session?.draft_version || 0)
      const shouldReloadDraft = !hadCompleteDraft || loadedDraftVersionRef.current !== serverDraftVersion
      if (!shouldReloadDraft) {
        setDraftLoading(false)
        return
      }
      setDraftLoading(true)
      const draftResponse = await client.get(`/import-agent/sessions/${id}/draft`, { params: { page: 1, page_size: 200 } })
      if (sequence !== draftLoadSequence.current) return
      const firstPage = draftResponse.data.data
      const firstItems = (firstPage.items || []) as VisionDraftCandidate[]
      const apiTotal = Number(firstPage.total || 0)
      const expectedTotal = Number(nextDetail.session?.expected_count || 0)
      // A stale backend may omit total or report only the first batch. The
      // persisted directory expectation is the safer lower bound, so the UI
      // keeps fetching until all expected candidates are present instead of
      // silently stopping at 100/200 items.
      const total = Math.max(apiTotal, expectedTotal, firstItems.length)
      const pageSize = Math.max(1, Number(firstPage.page_size || firstItems.length || 200))
      const pageCount = Math.ceil(total / pageSize)
      setDraftTotal(total)

      // On first entry, render page 1 while the remaining pages load. During a
      // live refresh, keep the previous complete draft on screen; replacing it
      // with page 1 made the pending count jump from 48 to the 3 rows that
      // happened to be pending on that page.
      if (!hadCompleteDraft || pageCount <= 1) setDraft(firstItems)
      if (pageCount <= 1) {
        draftFullyLoadedRef.current = true
        loadedDraftVersionRef.current = serverDraftVersion
        return
      }

      const fetchPage = async (page: number) => {
        let lastError: unknown
        for (let attempt = 0; attempt < 2; attempt += 1) {
          try {
            const response = await client.get(`/import-agent/sessions/${id}/draft`, { params: { page, page_size: pageSize } })
            return (response.data.data.items || []) as VisionDraftCandidate[]
          } catch (error) {
            lastError = error
            if (attempt === 0) await new Promise((resolve) => window.setTimeout(resolve, 300))
          }
        }
        throw lastError || new Error(`draft page ${page} failed`)
      }

      const pageNumbers = Array.from({ length: pageCount - 1 }, (_, index) => index + 2)
      const results = await Promise.allSettled(pageNumbers.map((page) => fetchPage(page)))
      if (sequence !== draftLoadSequence.current) return
      const byID = new Map<number, VisionDraftCandidate>()
      firstItems.forEach((item) => byID.set(item.id, item))
      const failedPages: number[] = []
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') {
          result.value.forEach((item) => byID.set(item.id, item))
        } else {
          failedPages.push(pageNumbers[index])
        }
      })
      const allItems = Array.from(byID.values()).sort((left, right) => {
        const pageDiff = (left.question_page_start || 0) - (right.question_page_start || 0)
        if (pageDiff) return pageDiff
        // A page may finish a numbered block and start another one whose
        // printed numbers restart at 1.  Use the directory's physical
        // vertical anchor so q33--38 are rendered before the following
        // multiple-choice q1--4 instead of appearing to be missing.
        const leftMarker = left.source_marker_top || 0
        const rightMarker = right.source_marker_top || 0
        if (leftMarker && rightMarker && leftMarker !== rightMarker) return leftMarker - rightMarker
        if (leftMarker !== rightMarker) return leftMarker ? -1 : 1
        const questionDiff = (left.source_question_no || 0) - (right.source_question_no || 0)
        return questionDiff || left.id - right.id
      })
      const completeLoad = failedPages.length === 0 && allItems.length >= total
      // A failed background refresh must not erase a complete draft that the
      // user was already reviewing. Initial loads still expose the partial
      // result together with the retry notice.
      if (completeLoad || !hadCompleteDraft) setDraft(allItems)
      draftFullyLoadedRef.current = completeLoad || hadCompleteDraft
      if (completeLoad) loadedDraftVersionRef.current = serverDraftVersion
      if (!completeLoad) {
        setDraftLoadError(`草稿加载不完整：已加载 ${allItems.length}/${total} 题（失败页 ${failedPages.join('、') || '未知'}），请点击刷新重试。`)
      }
    } catch (error) {
      if (sequence === draftLoadSequence.current) setDraftLoadError(`草稿加载失败：${errMsg(error)}`)
    } finally {
      if (sequence === draftLoadSequence.current) setDraftLoading(false)
    }
  }, [])

  const refresh = useCallback(async () => {
    await loadSessions()
    if (selectedId) await loadSession(selectedId)
  }, [loadSession, loadSessions, selectedId])

  // Collapse event bursts into one session refresh. A later event requests one
  // follow-up refresh after the current pagination request has settled.
  const refreshSelectedSession = useCallback((id: number, includeList = false) => {
    if (sessionRefreshInFlight.current && sessionRefreshID.current === id) {
      refreshAgain.current = true
      refreshAgainIncludeList.current = refreshAgainIncludeList.current || includeList
      return sessionRefreshInFlight.current
    }
    refreshAgain.current = false
    refreshAgainIncludeList.current = false
    sessionRefreshID.current = id
    const request = includeList
      ? Promise.all([loadSession(id), loadSessions()])
      : loadSession(id)
    sessionRefreshInFlight.current = request
    const settle = () => {
      if (sessionRefreshInFlight.current !== request) return
      sessionRefreshInFlight.current = null
      sessionRefreshID.current = null
      if (refreshAgain.current) {
        refreshAgain.current = false
        const followUpIncludeList = refreshAgainIncludeList.current || includeList
        refreshAgainIncludeList.current = false
        void refreshSelectedSession(id, followUpIncludeList)
      }
    }
    void request.then(settle, settle)
    return request
  }, [loadSession, loadSessions])

  useEffect(() => {
    loadSessions().catch(() => undefined)
  }, [loadSessions])

  // A repair can finish in another tab, after a backend restart, or while the
  // browser was backgrounded and the event stream was disconnected. Refresh
  // the selected session when the page becomes visible again so the old
  // draft/progress projection is not left on screen until a full reload.
  useEffect(() => {
    if (!selectedId) return
    let refreshing = false
    const refreshVisibleSession = () => {
      if (document.visibilityState !== 'visible' || refreshing || busy) return
      refreshing = true
      void refreshSelectedSession(selectedId, true).finally(() => {
        refreshing = false
      })
    }
    window.addEventListener('focus', refreshVisibleSession)
    document.addEventListener('visibilitychange', refreshVisibleSession)
    return () => {
      window.removeEventListener('focus', refreshVisibleSession)
      document.removeEventListener('visibilitychange', refreshVisibleSession)
    }
  }, [busy, refreshSelectedSession, selectedId])

  useEffect(() => {
    if (selectedId) {
      setEvents([])
      setTraceByRun({})
      lastEventId.current = 0
      refreshSelectedSession(selectedId, true).catch(() => undefined)
    }
  }, [selectedId, refreshSelectedSession])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  useEffect(() => {
    if (!selectedId) return
    const generation = ++streamGeneration.current
    streamRef.current?.abort()
    const controller = new AbortController()
    streamRef.current = controller
    const wait = (delay: number) => new Promise((resolve) => window.setTimeout(resolve, delay))
    let refreshTimer: number | undefined
    const scheduleDataRefresh = (immediate = false) => {
      if (refreshTimer) window.clearTimeout(refreshTimer)
      refreshTimer = window.setTimeout(() => {
        void refreshSelectedSession(selectedId, true)
      }, immediate ? 0 : 350)
    }

    const authFetch = async (url: string) => {
      const init = {
        cache: 'no-store' as const,
        headers: { Authorization: `Bearer ${localStorage.getItem('qt_access_token') || ''}` },
        signal: controller.signal,
      }
      let response = await fetch(url, init)
      if (response.status === 401) {
        await refreshAccessToken()
        response = await fetch(url, {
          ...init,
          headers: { Authorization: `Bearer ${localStorage.getItem('qt_access_token') || ''}` },
        })
      }
      return response
    }

    const parseEvents = (raw: string) => {
      const parsed: ImportAgentEvent[] = []
      for (const line of raw.split('\n')) {
        if (!line.trim()) continue
        try {
          const event = JSON.parse(line) as ImportAgentEvent
          // Heartbeats and protocol markers do not belong in a tool trace.
          if (event.event_id) parsed.push(event)
        } catch {
          // A malformed historical line must not prevent the live stream from
          // attaching. The server persists only valid JSON events, so this is
          // intentionally a best-effort UI guard.
        }
      }
      return parsed
    }

    const hydrateSnapshot = async () => {
      // The endpoint is finite but intentionally capped at 500 rows. Walk the
      // durable history with the same cursor used by the live stream so a
      // large old run is hydrated without replaying those rows as new work.
      const snapshot: ImportAgentEvent[] = []
      let cursor = 0
      for (let page = 0; page < 100; page += 1) {
        const response = await authFetch(
          `${API_BASE}/import-agent/sessions/${selectedId}/events?after_event_id=${cursor}&snapshot=1`,
        )
        if (!response.ok || !response.body) throw new Error(`event snapshot HTTP ${response.status}`)
        const batch = parseEvents(await response.text()).sort((left, right) => left.event_id - right.event_id)
        if (!batch.length) break
        snapshot.push(...batch)
        const nextCursor = batch.reduce((max, event) => Math.max(max, event.event_id), cursor)
        if (nextCursor <= cursor || batch.length < 500) break
        cursor = nextCursor
      }
      const byID = new Map<number, ImportAgentEvent>()
      const byRun: Record<number, ImportAgentEvent[]> = {}
      for (const event of snapshot) {
        byID.set(event.event_id, event)
        if (event.run_id) {
          byRun[event.run_id] = [...(byRun[event.run_id] || []), event]
        }
      }
      const hydrated = [...byID.values()].sort((left, right) => left.event_id - right.event_id)
      lastEventId.current = hydrated.reduce((max, event) => Math.max(max, event.event_id), 0)
      if (generation !== streamGeneration.current) return
      setEvents(hydrated.slice(-500))
      const boundedByRun: Record<number, ImportAgentEvent[]> = {}
      Object.entries(byRun).forEach(([runID, runEvents]) => {
        boundedByRun[Number(runID)] = runEvents.slice(-300)
      })
      setTraceByRun(boundedByRun)
    }

    const appendLiveEvent = (event: ImportAgentEvent) => {
      if (!event.event_id) return
      lastEventId.current = Math.max(lastEventId.current, event.event_id)
      if (generation !== streamGeneration.current) return
      setEvents((current) => [...current.filter((item) => item.event_id !== event.event_id), event].sort((left, right) => left.event_id - right.event_id).slice(-500))
      if (event.run_id) {
        setTraceByRun((current) => ({
          ...current,
          [event.run_id!]: [
            ...(current[event.run_id!] || []).filter((item) => item.event_id !== event.event_id),
            event,
          ].sort((left, right) => left.event_id - right.event_id).slice(-300),
        }))
      }
      if (
        [
          'run_started',
          'problem_created',
          'preview_ready',
          'run_completed',
          'clarification_required',
          'error',
        ].includes(event.type)
      ) {
        scheduleDataRefresh(event.type === 'run_completed' || event.type === 'error')
      }
    }

    const read = async () => {
      let retry = 0
      let hydrated = false
      while (!controller.signal.aborted) {
        try {
          setStreamState(retry ? 'reconnecting' : 'connecting')
          if (!hydrated) {
            await hydrateSnapshot()
            hydrated = true
          }
          const response = await authFetch(
            `${API_BASE}/import-agent/sessions/${selectedId}/events?after_event_id=${lastEventId.current}`,
          )
          if (!response.ok || !response.body) throw new Error(`event stream HTTP ${response.status}`)
          setStreamState('connected')
          retry = 0
          const reader = response.body.getReader()
          const decoder = new TextDecoder()
          let buffer = ''
          while (!controller.signal.aborted) {
            const { done, value } = await reader.read()
            if (done) throw new Error('event stream closed')
            buffer += decoder.decode(value, { stream: true })
            const lines = buffer.split('\n')
            buffer = lines.pop() || ''
            for (const line of lines) {
              if (!line.trim()) continue
              const event: ImportAgentEvent = JSON.parse(line)
              appendLiveEvent(event)
            }
          }
        } catch (error) {
          if (controller.signal.aborted) return
          console.warn('Import Agent event stream reconnecting', error)
          setStreamState('reconnecting')
          retry += 1
          await wait(Math.min(5000, 500 * 2 ** Math.min(retry, 4)))
        }
      }
    }
    void read()
    return () => {
      if (streamGeneration.current === generation) streamGeneration.current += 1
      controller.abort()
      if (refreshTimer) window.clearTimeout(refreshTimer)
    }
  }, [selectedId, refreshSelectedSession])

  const createSession = async () => {
    setBusy('create')
    try {
      const response = await client.post('/import-agent/sessions', {})
      await loadSessions()
      setSelectedId(response.data.data.id)
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const deleteSession = async (session: ImportAgentSession) => {
    const title = session.title || '未命名导入会话'
    const confirmed = window.confirm(`确认删除对话“${title}”？删除后该对话会从列表中移除。`)
    if (!confirmed) return

    setBusy(`delete-${session.id}`)
    try {
      // Cancel the persisted backend run first. The DELETE endpoint also does
      // this atomically, while this call keeps the UI responsive with older
      // backend processes during a rolling local restart.
      if (session.active_run_id) {
        try {
          await client.post(`/import-agent/sessions/${session.id}/runs/${session.active_run_id}/cancel`)
        } catch (error) {
          console.warn('Unable to cancel import run before archive; archive will retry server-side', error)
        }
      }
      if (selectedId === session.id) {
        streamRef.current?.abort()
        streamRef.current = null
        setSelectedId(null)
        setDetail(null)
        setMessages([])
        setEvents([])
        setDraft([])
        setDraftTotal(0)
        lastEventId.current = 0
      }
      await client.delete(`/import-agent/sessions/${session.id}`)
      await loadSessions()
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const cancelActiveRun = async (runID: number) => {
    if (!selectedId) return
    setBusy('cancel-run')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/runs/${runID}/cancel`)
      await refreshSelectedSession(selectedId, true)
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const ensureSession = async (): Promise<number> => {
    if (selectedId) return selectedId
    const response = await client.post('/import-agent/sessions', {})
    await loadSessions()
    setSelectedId(response.data.data.id)
    return response.data.data.id
  }

  const uploadFiles = async (files: File[] = pendingFiles, deferProcessing = false): Promise<number | null> => {
    if (!files.length) return selectedId
    setBusy('upload')
    try {
      const sessionId = await ensureSession()
      for (const file of files) {
        const init = await client.post(`/import-agent/sessions/${sessionId}/uploads/init`, {
          filename: file.name,
          size: file.size,
          mime_type: file.type || 'application/pdf',
          session_id: sessionId,
        })
        const { upload_id, chunk_size, chunk_count } = init.data.data
        for (let index = 0; index < chunk_count; index++) {
          const form = new FormData()
          form.append(
            'chunk',
            file.slice(index * chunk_size, Math.min((index + 1) * chunk_size, file.size)),
            `chunk_${index}`
          )
          await client.post(`/import-agent/sessions/${sessionId}/uploads/${upload_id}/chunks/${index}`, form, {
            timeout: 120000,
          })
          setUploadState((current) => ({ ...current, [file.name]: (index + 1) / chunk_count }))
        }
        await client.post(`/import-agent/sessions/${sessionId}/uploads/${upload_id}/complete`, {
          filename: file.name,
          mime_type: file.type || 'application/pdf',
          total_size: file.size,
          chunk_count,
          session_id: sessionId,
          defer_processing: deferProcessing,
        })
      }
      setPendingFiles((current) => current.filter((file) => !files.includes(file)))
      setUploadState({})
      await refreshSelectedSession(sessionId, true)
      return sessionId
    } catch (error) {
      alert(errMsg(error))
      return null
    } finally {
      setBusy('')
    }
  }

  const sendMessage = async (contentOverride?: string, sessionIdOverride?: number) => {
    const content = (contentOverride ?? input).trim()
    const sessionId = sessionIdOverride ?? selectedId
    if (!sessionId || !content) return
    setBusy('message')
    try {
      // Uploading a file starts a background run and may update the session
      // while the user is composing the next instruction. Read the current
      // draft version immediately before sending to avoid a stale-version
      // request being accepted without a visible assistant reply.
      const summary = await client.get(`/import-agent/sessions/${sessionId}`)
      const currentDetail = summary.data.data as SessionDetail
      const response = await authenticatedFetch(`${API_BASE}/import-agent/sessions/${sessionId}/messages/stream`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('qt_access_token') || ''}`,
        },
        body: JSON.stringify({
          content,
          expected_draft_version: currentDetail.session.draft_version,
          last_event_id: lastEventId.current,
        }),
      })
      if (!response.ok) throw new Error(await responseErrorMessage(response, '消息发送失败'))
      const raw = await response.text()
      const streamError = raw
        .split('\n')
        .map((line) => {
          try {
            return JSON.parse(line) as { type?: string; error?: string }
          } catch {
            return null
          }
        })
        .find((event) => event?.type === 'error')
      // The server has durably accepted the message even when the requested
      // operation fails, so clear only after receiving its response.
      setInput((current) => (current.trim() === content ? '' : current))
      await refreshSelectedSession(sessionId, true)
      // MessageStream persists operation failures as assistant messages. Do
      // not show a second browser alert for the same failure.
      if (streamError?.error) return
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const submitComposer = async () => {
    if (busy) return
    const content = input.trim()
    const files = [...pendingFiles]
    if (!content && !files.length) return

    if (files.length) {
      const sessionId = await uploadFiles(files, Boolean(content))
      if (sessionId && content) await sendMessage(content, sessionId)
      return
    }
    await sendMessage(content)
  }

  const runPreview = async () => {
    if (!selectedId) return
    setBusy('preview')
    try {
      const response = await client.post(`/import-agent/sessions/${selectedId}/preview`)
      setPreview(response.data.data)
      await refreshSelectedSession(selectedId, true)
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const relinkAnswers = async () => {
    if (!selectedId || !detail) return
    setBusy('relink')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/relink-answers`, {
        expected_draft_version: detail.session.draft_version,
      })
      await refreshSelectedSession(selectedId, true)
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const failedAnswerRange = useMemo(() => {
    const ranges = (detail?.problems || [])
      .filter((problem) => problem.problem_type === 'IMPORT_UNIT_FAILED')
      .map((problem) => {
        const metadata = parseMetadata(problem.detail_json)
        return {
          start: problem.page_start || 0,
          end: problem.page_end || problem.page_start || 0,
          unitType: String(metadata.unit_type || ''),
        }
      })
      .filter((range) => range.unitType === 'extract_answer' && range.start > 0 && range.end >= range.start)
    if (!ranges.length) return null
    return {
      start: Math.min(...ranges.map((range) => range.start)),
      end: Math.max(...ranges.map((range) => range.end)),
    }
  }, [detail?.problems])

  const retryMissingAnswerPages = async () => {
    if (!selectedId || !detail) return
    if (!failedAnswerRange) return
    const range = failedAnswerRange
    setBusy('retry-answer')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/retry-answer-pages`, {
        expected_draft_version: detail.session.draft_version,
        page_start: range.start,
        page_end: range.end,
      })
      await refreshSelectedSession(selectedId, true)
    } catch (error) { alert(errMsg(error)) } finally { setBusy('') }
  }

  const reextractPendingCandidates = async (explicitCandidateIDs?: number[]) => {
    if (!selectedId || !detail || pendingReviewCount <= 0) return
	const selectedPending = (explicitCandidateIDs?.length ? explicitCandidateIDs : selectedCandidates).filter((id) =>
      filteredDraft.some((candidate) => candidate.id === id && (candidate.review_decision || 'pending') === 'pending'),
    )
    setBusy('reextract-pending')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/candidates/reextract`, {
        expected_draft_version: detail.session.draft_version,
        ...(selectedPending.length ? { candidate_ids: selectedPending } : {}),
        scope: 'pending',
		target: 'auto',
      })
      setSelectedCandidates([])
      await refreshSelectedSession(selectedId, true)
    } catch (error) { alert(errMsg(error)) } finally { setBusy('') }
  }

  const commit = async () => {
    if (!selectedId || !detail || !preview) return
    setBusy('commit')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/commit`, {
        expected_version: detail.session.draft_version,
        preview_hash: preview.preview_hash,
      })
      await refresh()
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const confirmAction = async (actionId: number) => {
    if (!selectedId || !detail) return
    setBusy(`action-${actionId}`)
    try {
      await client.post(`/import-agent/sessions/${selectedId}/actions/${actionId}/confirm`, {
        expected_version: detail.session.draft_version,
      })
      await refreshSelectedSession(selectedId, true)
    } catch (error) {
      alert(errMsg(error))
    } finally {
      setBusy('')
    }
  }

  const reviewCandidate = async (candidate: VisionDraftCandidate, decision: 'include' | 'exclude' | 'pending') => {
    if (!selectedId || !detail) return
    let reason = candidate.exclusion_reason || ''
    if (decision === 'exclude') {
      reason = window.prompt('请输入排除原因（必填）', reason) || ''
      if (!reason.trim()) return
    }
    setBusy(`review-${candidate.id}`)
    try {
      await client.patch(`/import-agent/sessions/${selectedId}/candidates/${candidate.id}/review`, {
        expected_draft_version: detail.session.draft_version,
        review_decision: decision,
        exclusion_reason: reason,
      })
      await refreshSelectedSession(selectedId, true)
    } catch (error) { alert(errMsg(error)) } finally { setBusy('') }
  }

  const batchReview = async (decision: 'include' | 'exclude' | 'pending') => {
    if (!selectedId || !detail || !selectedCandidates.length) return
    let reason = ''
    if (decision === 'exclude') {
      reason = window.prompt('请输入批量排除原因（必填）') || ''
      if (!reason.trim()) return
    }
    setBusy('batch-review')
    try {
      await client.post(`/import-agent/sessions/${selectedId}/candidates/review/batch`, {
        expected_draft_version: detail.session.draft_version,
        candidate_ids: selectedCandidates,
        review_decision: decision,
        exclusion_reason: reason,
      })
      setSelectedCandidates([])
      await refreshSelectedSession(selectedId, true)
    } catch (error) { alert(errMsg(error)) } finally { setBusy('') }
  }

  const filteredDraft = useMemo(() => draft.filter((candidate) => {
    if (draftFilter === 'pending') return (candidate.review_decision || 'pending') === 'pending'
    if (draftFilter === 'include') return candidate.review_decision === 'include'
    if (draftFilter === 'exclude') return candidate.review_decision === 'exclude'
    if (draftFilter === 'missing_answer') {
      const type = candidate.canonical_question_type || candidate.question_type
      return ['single_choice', 'multiple_choice', 'true_false'].includes(type) && candidate.answer_link_status !== 'confirmed'
    }
    if (draftFilter === 'conflict') return candidate.answer_resolution === 'ANSWER_CONFLICT' || candidate.validation_issues?.some((issue) => String(issue.code || issue.problem_type || '').includes('CONFLICT'))
    return true
  }), [draft, draftFilter])

  const currentSession = detail?.session
  const draftStats = detail?.draft_stats
  // Upload-triggered runs never produce a chat message while executing, so the
  // conversation column pins a live monitor for any queued/running/merging run.
  const activeLiveRun = useMemo(() => {
    const run = detail?.active_run
    return run && ['queued', 'running', 'merging'].includes(run.status) ? run : undefined
  }, [detail?.active_run])
  const pendingReviewCount = draftStats?.pending_review_count
    ?? draft.filter((candidate) => (candidate.review_decision || 'pending') === 'pending').length
  const includedCount = draftStats?.included_count
    ?? draft.filter((candidate) => candidate.review_decision === 'include').length
  const openProblemCount = draftStats?.open_problem_count
    ?? detail?.problems.filter((problem) => problem.status === 'open').length
    ?? 0
  const traceMessageByRun = useMemo(() => {
    const owners: Record<number, number> = {}
    for (const message of messages) {
      if (message.run_id && message.message_type === 'agent_run' && !owners[message.run_id]) owners[message.run_id] = message.id
    }
    return owners
  }, [messages])

  return (
    <div className="h-[calc(100vh-6.5rem)] min-h-[660px] w-full rounded-3xl border border-border/80 bg-surface shadow-card overflow-hidden transition-all">
      <div className="grid h-full xl:grid-cols-[280px_minmax(420px,1fr)_420px]">
        {/* ========================================================================= */}
        {/* Left Column: Import Sessions List */}
        {/* ========================================================================= */}
        <aside className="flex min-h-0 flex-col border-r border-border/80 bg-surface-secondary/40">
          <div className="flex items-center justify-between border-b border-border/70 p-4 bg-surface/70 backdrop-blur-xs">
            <div className="flex items-center gap-2 min-w-0">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Bot className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <h1 className="text-sm font-bold tracking-tight text-foreground">导入 Agent</h1>
                <p className="text-[10px] text-muted-foreground truncate">多文档统一草稿对齐</p>
              </div>
            </div>
            <Button
              size="xs"
              onClick={() => void createSession()}
              loading={busy === 'create'}
              icon={<Plus className="h-3.5 w-3.5" />}
              className="shadow-subtle"
            >
              新会话
            </Button>
          </div>

          {/* Session List */}
          <div className="min-h-0 flex-1 overflow-y-auto p-2 space-y-1.5">
            {sessions.map((session, index) => {
              const isSelected = selectedId === session.id
              const tone = statusTone[session.status] || 'neutral'

              return (
                <div
                  key={session.id}
                  className={`group relative flex w-full items-center gap-3 rounded-2xl border transition-all ${
                    isSelected
                      ? 'border-primary/40 bg-primary/5 shadow-subtle ring-1 ring-primary/30'
                      : 'border-border/60 bg-surface hover:bg-surface-secondary hover:border-primary/30'
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => setSelectedId(session.id)}
                    className="flex min-w-0 flex-1 items-center gap-3 p-3.5 pr-10 text-left"
                  >
                    <span
                      className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-xl font-mono text-xs font-bold transition-colors ${
                        isSelected ? 'bg-primary text-primary-foreground shadow-subtle' : 'bg-surface-secondary text-muted-foreground'
                      }`}
                    >
                      {String(index + 1).padStart(2, '0')}
                    </span>

                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-1">
                        <span className="truncate text-xs font-bold text-foreground group-hover:text-primary transition-colors">
                          {session.title || '未命名导入会话'}
                        </span>
                      </div>

                      <div className="mt-1.5 flex items-center justify-between text-[10px]">
                        <Badge tone={tone} variant="subtle" className="text-[9px] px-1.5 py-0 font-medium">
                          {statusLabel[session.status] || session.status}
                        </Badge>
                        <span className="font-mono text-muted-foreground font-medium">
                          完整 {session.complete_count}/{session.expected_count ?? '校验中'}
                        </span>
                      </div>
                    </div>
                  </button>

                  <IconButton
                    type="button"
                    size="sm"
                    title="删除会话"
                    variant="ghost"
                    loading={busy === `delete-${session.id}`}
                    onClick={() => void deleteSession(session)}
                    className={`absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:bg-destructive/10 hover:text-destructive focus-visible:opacity-100 ${isSelected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </IconButton>

                  {isSelected && (
                    <span className="absolute left-0 top-1/2 -translate-y-1/2 h-6 w-1 rounded-r-full bg-primary" />
                  )}
                </div>
              )
            })}

            {sessions.length === 0 && (
              <div className="py-12 px-4 text-center space-y-2">
                <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-2xl bg-surface-secondary border border-border/80 text-muted-foreground">
                  <MessageSquare className="h-5 w-5" />
                </div>
                <h4 className="text-xs font-bold text-foreground">暂无导入会话</h4>
                <p className="text-[11px] text-muted-foreground">点击上方「新会话」按钮开始上传题本与答案册。</p>
              </div>
            )}
          </div>

          <div className="border-t border-border/70 p-3 bg-surface/70">
            <Button
              variant="ghost"
              size="xs"
              className="w-full text-xs text-muted-foreground hover:text-foreground"
              onClick={() => void refresh()}
              icon={<RefreshCw className="h-3.5 w-3.5" />}
            >
              刷新会话列表
            </Button>
          </div>
        </aside>

        {/* ========================================================================= */}
        {/* Middle Column: Interactive Agent Chat & Workflow */}
        {/* ========================================================================= */}
        <main className="flex min-h-0 flex-col border-r border-border/80 bg-surface">
          {/* Header */}
          <header className="flex min-h-[64px] items-center justify-between border-b border-border/70 px-5 bg-surface-secondary/30 backdrop-blur-xs">
            <div className="min-w-0">
              <div className="flex items-center gap-2.5 flex-wrap">
                <h2 className="font-bold text-sm tracking-tight text-foreground truncate max-w-md">
                  {currentSession?.title || '请选择或创建导入会话'}
                </h2>
                {currentSession && (
                  <Badge tone={statusTone[currentSession.status] || 'neutral'} variant="subtle" className="text-[10px]">
                    {statusLabel[currentSession.status] || currentSession.status}
                  </Badge>
                )}
                {currentSession && (
                  <span className={`flex items-center gap-1 text-[10px] ${streamState === 'connected' ? 'text-success' : 'text-warning'}`}>
                    <span className={`h-1.5 w-1.5 rounded-full ${streamState === 'connected' ? 'bg-success' : 'bg-warning animate-pulse'}`} />
                    {streamState === 'connected' ? '实时状态已连接' : streamState === 'reconnecting' ? '正在恢复实时状态' : '正在连接实时状态'}
                  </span>
                )}
              </div>
              {currentSession && (
                <div className="mt-0.5 text-[11px] text-muted-foreground font-mono">
                  附件版本 {currentSession.attachment_revision} · 草稿版本 {currentSession.draft_version}
                </div>
              )}
            </div>

            {currentSession && (
              <div className="grid grid-cols-3 gap-px rounded-2xl border border-border/80 bg-border/60 text-center text-[10px] overflow-hidden shadow-subtle">
                <div className="bg-surface px-3 py-1.5">
                  <b className="block font-mono text-xs font-bold text-foreground">
                    {currentSession.expected_count ?? '校验中'}
                  </b>
                  <span className="text-muted-foreground">预期</span>
                </div>
                <div className="bg-surface px-3 py-1.5">
                  <b className="block font-mono text-xs font-bold text-success">
                    {currentSession.complete_count}
                  </b>
                  <span className="text-muted-foreground">完整</span>
                </div>
                <div className="bg-surface px-3 py-1.5" title={`${draftStats?.open_blocking_problem_count ?? currentSession.blocking_issue_count} 个阻断问题`}>
                  <b
                    className={`block font-mono text-xs font-bold ${
                      pendingReviewCount > 0 ? 'text-destructive' : 'text-muted-foreground'
                    }`}
                  >
                    {pendingReviewCount}
                  </b>
                  <span className="text-muted-foreground">待审核</span>
                </div>
              </div>
            )}
          </header>

          {/* Chat Messages Log */}
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            <div className="mx-auto max-w-3xl space-y-4">
              {messages.map((message) => {
                const metadata = parseMetadata(message.metadata_json)
                const actionId = Number(metadata.action_id || 0)
                const isUser = message.role === 'user'
                const typeConfig = messageTypeConfig[message.message_type] || {
                  label: message.message_type,
                  tone: 'neutral' as const,
                }

                return (
                  <article
                    key={message.id}
                    className={`flex gap-3 items-start ${isUser ? 'justify-end' : ''}`}
                  >
                    {!isUser && (
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20 shadow-xs">
                        <Bot className="h-4 w-4" />
                      </div>
                    )}

                    {isUser ? (
                      /* User Message Bubble: Sleek rounded bubble with crisp white typography */
                      <div className="flex flex-col items-end max-w-[80%] sm:max-w-[72%]">
                        <div className="rounded-2xl rounded-tr-xs bg-primary px-4 py-2.5 text-xs text-white shadow-subtle leading-relaxed selection:bg-white/30 selection:text-white">
                          <div className="markdown-body-user select-text whitespace-pre-wrap font-medium">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
                          </div>
                        </div>
                        <span className="mt-1 px-1 text-[10px] font-mono text-muted-foreground/60">
                          {new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                        </span>
                      </div>
                    ) : (
                      /* Assistant Message Card: Clean surface card with clear status tags and typography */
                      <div className="flex-1 max-w-[88%] rounded-2xl rounded-tl-xs border border-border/80 bg-surface p-4 text-xs text-foreground shadow-card leading-relaxed">
                        <div className="mb-2 flex items-center justify-between gap-4">
                          <div className="flex items-center gap-2">
                            <span className="font-semibold text-foreground text-xs">Import Agent</span>
                            {message.message_type && message.message_type !== 'text' && (
                              <Badge tone={typeConfig.tone} variant="subtle" className="text-[9px] px-1.5 py-0 font-medium">
                                {typeConfig.label}
                              </Badge>
                            )}
                          </div>
                          <span className="font-mono text-[10px] text-muted-foreground">
                            {new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                          </span>
                        </div>

                        <div className="markdown-body text-xs text-foreground/90 select-text leading-relaxed">
                          <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
                        </div>

                        {message.run_id && traceMessageByRun[message.run_id] === message.id && (
                          <div className="mt-3">
                            <ToolTrace
                              events={traceByRun[message.run_id] || []}
                              documents={detail?.documents || []}
                              activeRun={detail?.active_run?.id === message.run_id ? detail.active_run : undefined}
                              terminalStatus={message.status}
                              onCancel={detail?.active_run?.id === message.run_id ? () => void cancelActiveRun(message.run_id!) : undefined}
                              cancelling={busy === 'cancel-run'}
                            />
                          </div>
                        )}

                        {message.message_type === 'clarification' && Array.isArray(metadata.documents) && (
                          <div className="mt-3 flex flex-wrap gap-2 pt-2.5 border-t border-border/60">
                            {(metadata.documents as ImportAgentDocument[]).map((doc) => (
                              <Button
                                key={doc.document_id}
                                size="xs"
                                variant="outline"
                                onClick={() => setInput(`重新识别文档“${doc.original_name}”的指定页码`)}
                                icon={<FileText className="h-3 w-3 text-primary" />}
                              >
                                {doc.original_name}
                              </Button>
                            ))}
                          </div>
                        )}

                        {message.message_type === 'action_card' && actionId > 0 && (
                          <div className="mt-3 pt-2.5 border-t border-border/60 flex justify-end">
                            <Button
                              size="xs"
                              loading={busy === `action-${actionId}`}
                              onClick={() => void confirmAction(actionId)}
                              icon={<Sparkles className="h-3 w-3" />}
                            >
                              确认执行对齐
                            </Button>
                          </div>
                        )}
                      </div>
                    )}
                  </article>
                )
              })}

              {!selectedId && (
                <div className="py-20 text-center space-y-3">
                  <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-3xl bg-primary/10 text-primary shadow-glow">
                    <UploadCloud className="h-7 w-7" />
                  </div>
                  <div className="space-y-1 max-w-sm mx-auto">
                    <h3 className="text-sm font-bold tracking-tight text-foreground">
                      选择或新建会话开始导入
                    </h3>
                    <p className="text-xs text-muted-foreground leading-relaxed">
                      支持在同一会话中同时上传题本、答案册与解析册，AI Agent 会自动进行跨文档对齐并增量生成结构化试题。
                    </p>
                  </div>
                </div>
              )}

              <div ref={messagesEndRef} />
            </div>
          </div>

          {/* Live run monitor: pinned interactive window for the executing run */}
          {activeLiveRun && (
            <div className="border-t border-border/80 bg-surface-secondary/30 px-5 py-3">
              <div className="mx-auto max-w-3xl">
                <LiveRunMonitor
                  run={activeLiveRun}
                  events={events}
                  documents={detail?.documents || []}
                  onCancel={() => void cancelActiveRun(activeLiveRun.id)}
                  cancelling={busy === 'cancel-run'}
                  onOpenUnits={() => setTab('units')}
                />
              </div>
            </div>
          )}

          {/* Footer Input Area */}
          <footer className="border-t border-border/80 bg-surface p-4">
            <div className="mx-auto max-w-3xl space-y-2.5">
              {/* Pending files tray */}
              {pendingFiles.length > 0 && (
                <div className="flex flex-wrap gap-2 animate-slide-down">
                  {pendingFiles.map((file) => (
                    <div
                      key={`${file.name}-${file.size}`}
                      className="min-w-56 rounded-2xl border border-border/80 bg-surface-secondary/70 p-2.5 text-xs shadow-subtle"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-1.5 truncate">
                          <FileText className="h-3.5 w-3.5 text-primary shrink-0" />
                          <span className="truncate font-semibold text-foreground">{file.name}</span>
                        </div>
                        <button
                          type="button"
                          onClick={() => setPendingFiles((items) => items.filter((item) => item !== file))}
                          className="rounded-lg p-0.5 text-muted-foreground hover:bg-surface hover:text-foreground transition-colors"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                      {uploadState[file.name] !== undefined && (
                        <div className="mt-2 space-y-1">
                          <div className="flex justify-between text-[10px] text-muted-foreground font-mono">
                            <span>上传进度</span>
                            <span>{Math.round((uploadState[file.name] || 0) * 100)}%</span>
                          </div>
                          <Progress value={uploadState[file.name]} />
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}

              {/* Input Box */}
              <div className="relative flex flex-col rounded-2xl border border-border/80 bg-surface-secondary/40 focus-within:border-primary/60 focus-within:ring-2 focus-within:ring-primary/20 transition-all">
                <input
                  ref={fileRef}
                  className="hidden"
                  type="file"
                  multiple
                  accept=".pdf"
                  onChange={(event) =>
                    setPendingFiles((current) => [...current, ...Array.from(event.target.files || [])])
                  }
                />

                <textarea
                  className="w-full resize-none bg-transparent px-3.5 py-3 text-xs text-foreground placeholder:text-muted-foreground/60 focus:outline-none min-h-[44px] max-h-[140px] leading-relaxed"
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  onCompositionStart={() => {
                    isComposingRef.current = true
                  }}
                  onCompositionEnd={() => {
                    isComposingRef.current = false
                  }}
                  onKeyDown={(event) => {
                    if (event.nativeEvent.isComposing || isComposingRef.current || event.keyCode === 229) {
                      return
                    }
                    if (event.key === 'Enter' && !event.shiftKey) {
                      event.preventDefault()
                      void submitComposer()
                    }
                  }}
                  placeholder="上传 PDF，或要求 Agent 补题、重识别页码、对齐答案… (Enter 发送)"
                />

                <div className="flex items-center justify-between px-3 pb-2.5 pt-1 border-t border-border/40">
                  <div className="flex items-center gap-1.5">
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => fileRef.current?.click()}
                      icon={<Paperclip className="h-3.5 w-3.5 text-primary" />}
                      className="text-xs text-muted-foreground hover:text-foreground"
                    >
                      添加 PDF 文件
                    </Button>
                    <span className="text-[10px] text-muted-foreground/60 hidden sm:inline">
                      Shift + Enter 换行
                    </span>
                  </div>

                  {pendingFiles.length > 0 ? (
                    <Button
                      size="xs"
                      loading={busy === 'upload' || busy === 'message'}
                      onClick={() => void submitComposer()}
                      icon={input.trim() ? <Send className="h-3.5 w-3.5" /> : <UploadCloud className="h-3.5 w-3.5" />}
                      className="shadow-glow"
                    >
                      {input.trim() ? `上传并发送 (${pendingFiles.length})` : `上传并解析 (${pendingFiles.length})`}
                    </Button>
                  ) : (
                    <Button
                      size="xs"
                      disabled={!input.trim() || !selectedId}
                      loading={busy === 'message'}
                      onClick={() => void submitComposer()}
                      icon={<Send className="h-3.5 w-3.5" />}
                      className="shadow-subtle"
                    >
                      发送
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </footer>
        </main>

        {/* ========================================================================= */}
        {/* Right Column: Inspection & Verification Workbench */}
        {/* ========================================================================= */}
        <aside className="flex min-h-0 flex-col bg-surface">
          {/* Tabs Navigation Header */}
          <div className="grid grid-cols-4 border-b border-border/80 bg-surface-secondary/30">
            {(
              [
                ['draft', '草稿', draftTotal > draft.length ? `${draft.length}/${draftTotal}` : draftTotal || draft.length],
                ['documents', '附件', detail?.documents.length ?? 0],
                ['problems', '问题', openProblemCount],
                ['units', '运行', events.length],
              ] as const
            ).map(([key, label, count]) => {
              const isActive = tab === key
              return (
                <button
                  key={key}
                  type="button"
                  onClick={() => setTab(key)}
                  className={`relative flex items-center justify-center gap-1 py-3 text-xs font-bold transition-colors ${
                    isActive
                      ? 'text-primary bg-surface font-bold'
                      : 'text-muted-foreground hover:text-foreground hover:bg-surface-secondary/60'
                  }`}
                >
                  <span>{label}</span>
                  {count !== 0 && (
                    <span
                      className={`rounded-full px-1.5 py-0.2 text-[9px] font-mono font-medium ${
                        isActive ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'
                      }`}
                    >
                      {count}
                    </span>
                  )}
                  {isActive && (
                    <span className="absolute inset-x-0 bottom-0 h-0.5 bg-primary rounded-full" />
                  )}
                </button>
              )
            })}
          </div>

          {/* Tab Content Container */}
          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            {/* Tab: Draft Candidates */}
            {tab === 'draft' && (
              <div className="space-y-3">
                {(draftLoading || draftLoadError) && (
                  <div className={`flex items-center gap-2 rounded-xl border px-3 py-2 text-[11px] ${draftLoadError ? 'border-warning/50 bg-warning/10 text-warning-foreground' : 'border-primary/20 bg-primary/5 text-primary'}`}>
                    {draftLoading && <Spinner className="h-3.5 w-3.5" />}
                    <span className="flex-1">
                      {draftLoadError || `正在加载完整草稿：已加载 ${draft.length}/${draftTotal || '…'} 题`}
                    </span>
                    {draftLoadError && <Button size="xs" variant="outline" onClick={() => selectedId && void refreshSelectedSession(selectedId, true)} loading={draftLoading}>刷新草稿</Button>}
                  </div>
                )}
                <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center gap-1.5 border-b border-border/70 bg-surface/95 px-1 pb-3 backdrop-blur-sm">
                  {(['all', 'pending', 'include', 'exclude', 'missing_answer', 'conflict'] as const).map((filter) => (
                    <button key={filter} type="button" onClick={() => setDraftFilter(filter)} className={`rounded-full border px-2 py-1 text-[10px] font-bold transition-colors ${draftFilter === filter ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:border-primary/40'}`}>
                      {{ all: '全部', pending: '待审核', include: '纳入', exclude: '排除', missing_answer: '缺答案', conflict: '答案冲突' }[filter]}
                    </button>
                  ))}
                  <span className="ml-auto text-[10px] font-mono text-muted-foreground">共 {draftStats?.total_count ?? (draftTotal || draft.length)} 题 · 当前已加载 {draft.length} · 纳入 {includedCount} · 待审核 {pendingReviewCount}</span>
                </div>
                {draftFilter === 'pending' && pendingReviewCount > 0 && (
                  <div className="flex items-center justify-between gap-2 rounded-xl border border-primary/20 bg-primary/5 px-3 py-2">
                    <span className="text-[10px] text-primary">只复核待审核候选题，不重新扫描整份 PDF。</span>
                    <Button
                      size="xs"
                      loading={busy === 'reextract-pending'}
                      onClick={() => void reextractPendingCandidates()}
                      icon={<RefreshCw className="h-3 w-3" />}
                    >
					  智能精准修复
                    </Button>
                  </div>
                )}
                {selectedCandidates.length > 0 && (
                  <div className="flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/5 p-2">
                    <span className="text-[10px] font-bold text-primary">已选 {selectedCandidates.length} 题</span>
                    <Button size="xs" onClick={() => void batchReview('include')} loading={busy === 'batch-review'}>纳入入库</Button>
                    <Button size="xs" variant="outline" onClick={() => void batchReview('exclude')} loading={busy === 'batch-review'}>排除</Button>
                    <button type="button" className="ml-auto text-[10px] text-muted-foreground" onClick={() => setSelectedCandidates([])}>清除</button>
                  </div>
                )}
                {filteredDraft.map((candidate) => {
                  const answerKeys = candidate.question.answer || []
                  const isSelected = selectedCandidates.includes(candidate.id)
                  const review = candidate.review_decision || 'pending'
                  const confirmed = candidate.answer_link_status === 'confirmed'
                  const questionType = candidate.canonical_question_type || candidate.question_type
                  const requiresChoiceAnswer = ['single_choice', 'multiple_choice', 'true_false'].includes(questionType)
                  const hasAnswerSource = (candidate.answer_sources?.length || 0) > 0 || candidate.answer_page_start > 0
				  const questionSpan = candidate.source_spans?.find((span) => span.source_role === 'question' && span.status === 'active')
				  const answerSpan = candidate.source_spans?.find((span) => (span.source_role === 'answer' || span.source_role === 'explanation') && span.status === 'active')
                  const answerResolution = candidate.answer_resolution || (!hasAnswerSource && requiresChoiceAnswer ? 'ANSWER_MISSING' : 'ANSWER_UNRESOLVED')
                  const answerLabel = answerResolution === 'ANSWER_CONFLICT'
                    ? '答案冲突'
                    : answerResolution === 'ANSWER_SOURCE_IDENTITY_INVALID'
                      ? '答案身份待复核'
                      : answerResolution === 'ANSWER_MISSING'
                        ? '未找到参考答案'
                        : requiresChoiceAnswer
                          ? '未对齐'
                          : hasAnswerSource
                            ? '参考答案待复核'
                            : '未找到参考答案'
                  const answerDetail = answerResolution === 'ANSWER_CONFLICT'
                    ? '阻断：同一题存在不同答案值，请人工确认'
                    : answerResolution === 'ANSWER_SOURCE_IDENTITY_INVALID'
                      ? '阻断：答案记录存在，但章节/题号身份无法唯一匹配'
                      : answerResolution === 'ANSWER_MISSING'
                        ? '未发现对应答案记录；可定向重识别答案页'
                        : requiresChoiceAnswer
                          ? '阻断：答案来源未唯一匹配'
                          : hasAnswerSource
                            ? '参考答案来源存在，但尚未唯一对齐'
                            : '未发现对应参考答案来源；可要求 Agent 重识别答案页'
                  return (
                  <article
                    key={candidate.id}
                    className={`rounded-2xl border bg-surface p-4 shadow-subtle space-y-2.5 transition-all hover:border-primary/30 ${review === 'pending' ? 'border-warning/40' : 'border-border/80'}`}
                  >
                    <div className="flex items-center justify-between text-xs">
                      <div className="flex items-center gap-2">
                        <input type="checkbox" checked={isSelected} onChange={(event) => setSelectedCandidates((current) => event.target.checked ? [...current, candidate.id] : current.filter((id) => id !== candidate.id))} />
                        <b className="font-mono text-sm text-foreground">第 {candidate.source_question_no} 题</b>
                      </div>
                      <Badge tone={review === 'include' ? 'success' : review === 'exclude' ? 'neutral' : candidate.validation_issues?.length ? 'danger' : 'warning'} variant="subtle" className="text-[10px]">{review === 'include' ? '纳入' : review === 'exclude' ? '已排除' : '待审核'}</Badge>
                    </div>

                    {candidate.question.passage && <p className="rounded-xl border-l-2 border-primary/40 bg-primary/5 px-3 py-2 text-[11px] leading-relaxed text-muted-foreground whitespace-pre-wrap">{candidate.question.passage}</p>}
                    <p className="text-xs text-foreground/90 font-medium leading-relaxed select-text whitespace-pre-wrap">{candidate.question.stem || '题干为空'}</p>

                    {!!candidate.question.options?.length && (
                      <div className="grid gap-1.5 rounded-xl border border-border/60 bg-surface-secondary/20 p-2">
                        {candidate.question.options.map((option) => {
                          const correct = answerKeys.includes(option.key)
                          return <div key={option.key} className={`flex gap-2 rounded-lg px-2 py-1.5 text-[11px] leading-relaxed ${correct && confirmed ? 'bg-success/10 text-success ring-1 ring-success/30' : 'text-foreground/80'}`}><b className="font-mono">{option.key}.</b><span>{option.content}</span>{correct && confirmed && <span className="ml-auto shrink-0 text-[9px] font-bold">正确答案</span>}</div>
                        })}
                      </div>
                    )}

                    <div className="flex items-center gap-2 text-[10px] text-muted-foreground flex-wrap">
                      <Badge tone="neutral" variant="outline" className="text-[9px]">
                        {candidate.question_type}
                      </Badge>
                      {candidate.section_path && (
                        <span className="truncate">{candidate.section_path.join(' / ')}</span>
                      )}
                    </div>

					<details className="border-y border-border bg-white">
					  <summary className="flex cursor-pointer list-none items-center justify-between py-2 text-[10px] font-semibold text-[#002FA7]">
						<span>题目来源 · {candidate.provenance_complete ? '证据完整' : '证据待补'}</span>
						<span className="font-mono font-normal text-muted-foreground">{candidate.origin_uid ? candidate.origin_uid.slice(-14) : '未建立来源 ID'}</span>
					  </summary>
					  <div className="grid border-t border-border sm:grid-cols-2">
						{(candidate.source_spans || []).filter((span) => span.status === 'active').map((span) => (
						  <div key={span.id} className="grid grid-cols-[52px_1fr] gap-3 border-b border-r border-border p-3">
							<div className="font-mono text-[#002FA7]"><span className="block text-[9px] uppercase">{span.source_role}</span><b className="text-2xl leading-none tabular-nums">{span.page_start}</b></div>
							<div className="min-w-0 text-[10px] leading-relaxed text-muted-foreground">
							  <b className="block truncate text-foreground">{span.document_name || `文档 #${span.document_id}`}</b>
							  <span>物理页 {span.page_start}{span.page_end !== span.page_start ? `–${span.page_end}` : ''} · 附件 v{span.attachment_revision}</span>
							  <span className="block truncate font-mono">hash {span.content_hash.slice(0, 12)}</span>
							</div>
						  </div>
						))}
						{!(candidate.source_spans || []).length && <div className="col-span-2 p-3 text-[10px] text-warning-foreground">暂无可定位来源，精准修复将返回 SOURCE_SPAN_MISSING。</div>}
					  </div>
					  <div className="border-t border-border py-2 text-[10px] text-muted-foreground">原文题号 {candidate.original_question_label || candidate.source_question_no} · 章节序号 {candidate.sequence_in_scope || '—'} · 内部来源 {candidate.origin_uid || '—'}</div>
					</details>

                    <div className="mt-3 flex items-center gap-3 pt-2 border-t border-border/50">
                      {candidate.document_id && (
                          <DocumentPage
                          sessionId={selectedId!}
                          documentId={candidate.document_id}
                          page={candidate.question_page_start}
						  sourceSpanId={questionSpan?.id}
                        />
                      )}
                      <div className="text-xs space-y-1 min-w-0 flex-1">
                        <div>
                          <span className="text-muted-foreground">答案：</span>
                          <b className={confirmed ? 'text-success font-mono font-bold' : requiresChoiceAnswer ? 'text-destructive font-mono font-bold' : 'text-warning font-mono font-bold'}>
                            {confirmed ? (answerKeys.join(', ') || (requiresChoiceAnswer ? '解析无明确答案' : '参考答案/解析已提取')) : answerLabel}
                          </b>
                        </div>
                        <div className="text-[10px] text-muted-foreground">{confirmed ? `来源 ${candidate.answer_page_start || '—'}–${candidate.answer_page_end || '—'} · ${candidate.answer_link_method || 'canonical'} · ${Math.round((candidate.answer_link_confidence || 0) * 100)}%` : answerDetail}</div>
                        <div className="text-[11px] text-muted-foreground font-mono">
                          置信评分: {Math.round((candidate.visual_confidence || 0) * 100)}%
                        </div>
                      </div>
                    </div>
					{candidate.answer_page_start > 0 && (candidate.answer_document_id || candidate.document_id) && <div className="flex items-center gap-2 text-[10px] text-muted-foreground"><DocumentPage sessionId={selectedId!} documentId={candidate.answer_document_id || candidate.document_id!} page={candidate.answer_page_start} sourceSpanId={answerSpan?.id} /><span>答案页缩略图</span></div>}
                    {candidate.question.analysis && <details className="rounded-xl border border-border/50 px-3 py-2 text-[11px]"><summary className="cursor-pointer font-bold text-muted-foreground">展开解析</summary><p className="mt-2 whitespace-pre-wrap leading-relaxed text-foreground/80">{candidate.question.analysis}</p></details>}
                    <div className="flex items-center justify-end gap-1.5 border-t border-border/50 pt-2">
					  {review === 'pending' && <Button size="xs" variant="outline" onClick={() => void reextractPendingCandidates([candidate.id])} loading={busy === 'reextract-pending'}>智能精准修复</Button>}
                      <Button size="xs" variant={review === 'include' ? 'primary' : 'outline'} onClick={() => void reviewCandidate(candidate, 'include')} loading={busy === `review-${candidate.id}`}>纳入</Button>
                      <Button size="xs" variant={review === 'exclude' ? 'secondary' : 'outline'} onClick={() => void reviewCandidate(candidate, 'exclude')} loading={busy === `review-${candidate.id}`}>排除</Button>
                      {review !== 'pending' && <Button size="xs" variant="ghost" onClick={() => void reviewCandidate(candidate, 'pending')} loading={busy === `review-${candidate.id}`}>待定</Button>}
                    </div>
                  </article>
                  )
                })}

                {filteredDraft.length === 0 && (
                  <div className="py-16 text-center space-y-2">
                    <FileSearch className="mx-auto h-8 w-8 text-muted-foreground/60" />
                    <p className="text-xs text-muted-foreground">
                      上传文档后，结构化试题草稿会在此处增量生成。
                    </p>
                  </div>
                )}
              </div>
            )}

            {/* Tab: Documents */}
            {tab === 'documents' && (
              <div className="space-y-3">
                {detail?.documents.map((doc) => (
                  <article
                    key={doc.id}
                    className="rounded-2xl border border-border/80 bg-surface p-4 shadow-subtle space-y-2.5"
                  >
                    <div className="flex items-start gap-3">
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                        <FileText className="h-5 w-5" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <b className="block truncate text-xs font-bold text-foreground">
                          {doc.original_name}
                        </b>
                        <div className="mt-0.5 text-[10px] text-muted-foreground font-mono">
                          {doc.page_count || '?'} 页 · 附件版本 {doc.attachment_revision}
                        </div>
                        <div className={`mt-1 text-[10px] font-mono ${doc.ocr_failed_count ? 'text-destructive' : doc.is_scanned ? 'text-warning-foreground' : 'text-muted-foreground'}`}>
                          {doc.is_scanned ? `扫描版 · OCR ${doc.ocr_completed_count || 0}/${doc.page_count || '?'} 页` : '文本层 PDF'}
                          {!!doc.ocr_failed_count && ` · ${doc.ocr_failed_count} 页 OCR 失败`}
                        </div>
                        <div className="mt-2.5">
                          <label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block mb-1">
                            文档角色归类
                          </label>
                          <select
                            className="input h-8 text-[11px] py-0 pl-2.5 pr-7 rounded-xl"
                            value={doc.document_role}
                            onChange={async (event) => {
                              await client.patch(
                                `/import-agent/sessions/${selectedId}/documents/${doc.document_id}/role`,
                                { document_role: event.target.value }
                              )
                              await refreshSelectedSession(selectedId!, true)
                            }}
                          >
                            {Object.entries(roleLabel).map(([value, label]) => (
                              <option key={value} value={value}>
                                {label}
                              </option>
                            ))}
                          </select>
                        </div>
                        <div className="mt-1.5 text-[10px] text-muted-foreground font-mono">
                          Agent 识别置信度: {Math.round(doc.role_confidence * 100)}%
                        </div>
                      </div>
                    </div>
                  </article>
                ))}

                {!detail?.documents.length && (
                  <p className="py-16 text-center text-xs text-muted-foreground">暂无上传文档附件</p>
                )}
              </div>
            )}

            {/* Tab: Problems */}
            {tab === 'problems' && (
              <div className="space-y-2.5">
                {detail?.problems.map((problem) => (
                  <article
                    key={problem.id}
                    className="rounded-2xl border border-destructive/30 bg-destructive/5 p-3.5 space-y-1"
                  >
                    <div className="flex items-center gap-2 text-xs font-bold text-destructive">
                      <AlertCircle className="h-4 w-4 shrink-0" />
                      <span>{problem.problem_type}</span>
                    </div>
                    <div className="text-[11px] text-muted-foreground font-mono">
                      {problem.document_id ? `文档 #${problem.document_id}` : '全局校验'}
                      {problem.page_start ? ` · 页码 ${problem.page_start}–${problem.page_end}` : ''}
					  {problem.candidate_id ? ` · 候选题 #${problem.candidate_id}` : ''}
                    </div>
					<div className="text-[10px] text-muted-foreground">推荐动作：{{ retry_question: '重识别题目来源页', retry_answer: '重识别答案/解析来源页', normalize_identity: '先执行答案身份归一化', manual_review: '人工复核来源' }[problem.resolution_hint || 'manual_review'] || '人工复核来源'}</div>
                  </article>
                ))}

                {!detail?.problems.length && (
                  <div className="py-16 text-center space-y-2">
                    <CheckCircle2 className="mx-auto h-8 w-8 text-success" />
                    <p className="text-xs font-bold text-foreground">当前没有阻断问题</p>
                    <p className="text-[11px] text-muted-foreground">所有题目均通过版面与答案完整性校验。</p>
                  </div>
                )}
              </div>
            )}

            {/* Tab: Trace */}
            {tab === 'units' && (
              <div className="space-y-3">
                {Object.entries(traceByRun)
                  .sort(([left], [right]) => Number(right) - Number(left))
                  .map(([runID, runEvents]) => {
                    const id = Number(runID)
                    const isActive = detail?.active_run?.id === id
                    return (
                      <ToolTrace
                        key={runID}
                        runId={id}
                        events={runEvents}
                        documents={detail?.documents || []}
                        activeRun={isActive ? detail?.active_run : undefined}
                        terminalStatus={messages.find((message) => message.run_id === id)?.status}
                        onCancel={isActive ? () => void cancelActiveRun(id) : undefined}
                        cancelling={busy === 'cancel-run'}
                      />
                    )
                  })}
                {!Object.keys(traceByRun).length && (
                  <p className="py-12 text-center text-xs text-muted-foreground">当前没有带运行 ID 的执行轨迹。</p>
                )}
              </div>
            )}
          </div>

          {/* Bottom Preflight & Commit Toolbar */}
          {selectedId && (
            <div className="border-t border-border/80 bg-surface-secondary/30 p-3.5 space-y-2.5">
              <div className="grid grid-cols-2 gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  loading={busy === 'relink'}
                  onClick={() => void relinkAnswers()}
                  icon={<RefreshCw className="h-3.5 w-3.5 text-primary" />}
                >
                  重新对齐答案
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!failedAnswerRange}
                  loading={busy === 'retry-answer'}
                  onClick={() => void retryMissingAnswerPages()}
                  icon={<RefreshCw className="h-3.5 w-3.5 text-warning" />}
                >
                  {failedAnswerRange ? `重试答案页 ${failedAnswerRange.start}–${failedAnswerRange.end}` : '重试失败答案页'}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  loading={busy === 'preview'}
                  onClick={() => void runPreview()}
                  icon={<FileSearch className="h-3.5 w-3.5 text-primary" />}
                >
                  重新预检
                </Button>
                <Button
                  size="sm"
                  disabled={!preview?.can_commit}
                  loading={busy === 'commit'}
                  onClick={() => void commit()}
                  icon={<Sparkles className="h-3.5 w-3.5" />}
                  className="shadow-glow"
                >
                  提交入库
                </Button>
              </div>

              {preview && (
                <div
                  className={`rounded-2xl border p-2.5 text-[11px] font-medium leading-relaxed ${
                    preview.can_commit
                      ? 'border-success/30 bg-success/5 text-success'
                      : 'border-destructive/30 bg-destructive/5 text-destructive'
                  }`}
                >
                  {preview.can_commit
                    ? '✓ 预检通过，数据结构与答案已就绪，可一键提交题库。'
                    : `✕ 预检未通过：${String(preview.pending_review_count || 0)} 题待审核，${String(preview.unlinked_answer_count || 0)} 题答案未对齐，${String(preview.blocking_issue_count || 0)} 个阻断问题需要修正。`}
                </div>
              )}
            </div>
          )}
        </aside>
      </div>
    </div>
  )
}
