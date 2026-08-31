import React, { useCallback, useEffect, useRef, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Progress, Skeleton, Spinner } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { ImportAgentSession, ImportAgentDocument, ImportAgentProblem } from '@/types'
import {
  AlertCircle,
  ArrowRight,
  Bot,
  CheckCircle2,
  FileSearch,
  FileText,
  Layers,
  Plus,
  Play,
  RefreshCw,
  Sparkles,
  UploadCloud,
  X,
} from 'lucide-react'

export default function MobileAdminImports() {
  const [sessions, setSessions] = useState<ImportAgentSession[]>([])
  const [selectedSessionId, setSelectedSessionId] = useState<number | null>(null)
  const [sessionDetail, setSessionDetail] = useState<{
    session: ImportAgentSession
    documents: ImportAgentDocument[]
    problems: ImportAgentProblem[]
  } | null>(null)
  const [loading, setLoading] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  const loadSessions = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/admin/import-agent/sessions', { params: { page_size: 20 } })
      const items: ImportAgentSession[] = resp.data.data.items || []
      setSessions(items)
      if (items.length > 0 && !selectedSessionId) {
        setSelectedSessionId(items[0].id)
      }
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setLoading(false)
    }
  }, [selectedSessionId])

  const loadDetail = useCallback(async (sid: number) => {
    try {
      const resp = await client.get(`/admin/import-agent/sessions/${sid}`)
      setSessionDetail(resp.data.data)
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }, [])

  useEffect(() => {
    loadSessions()
  }, [loadSessions])

  useEffect(() => {
    if (selectedSessionId) {
      loadDetail(selectedSessionId)
    }
  }, [selectedSessionId, loadDetail])

  const handleCreateSession = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newTitle.trim()) return
    try {
      const resp = await client.post('/admin/import-agent/sessions', { title: newTitle.trim() })
      showToast('导入流水线已创建', 'success')
      setNewTitle('')
      setCreateOpen(false)
      setSelectedSessionId(resp.data.data.id)
      loadSessions()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const handleUploadFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !selectedSessionId) return
    setUploading(true)
    const formData = new FormData()
    formData.append('file', file)
    try {
      await client.post(`/admin/import-agent/sessions/${selectedSessionId}/documents`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      showToast('PDF 试卷文档已上传', 'success')
      loadDetail(selectedSessionId)
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const triggerRun = async () => {
    if (!selectedSessionId) return
    try {
      await client.post(`/admin/import-agent/sessions/${selectedSessionId}/run`)
      showToast('Pipeline 流水线已启动', 'success')
      loadDetail(selectedSessionId)
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  return (
    <div className="relative min-h-full">
      <input
        ref={fileInputRef}
        type="file"
        accept=".pdf"
        className="hidden"
        onChange={handleUploadFile}
      />

      <MobilePullToRefresh onRefresh={loadSessions} className="p-4 space-y-4 pb-24 select-none">
        {/* Header & Session Selector */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <UploadCloud className="h-5 w-5 text-primary" />
            <h2 className="text-sm font-bold text-foreground">PDF 导入流水线</h2>
          </div>
          <Button size="xs" onClick={() => setCreateOpen(true)} icon={<Plus className="h-3 w-3" />}>
            新建任务
          </Button>
        </div>

        {/* Sessions Horizontal Scroll */}
        {sessions.length > 0 && (
          <div className="flex items-center gap-2 overflow-x-auto no-scrollbar py-1">
            {sessions.map((s) => (
              <button
                key={s.id}
                onClick={() => setSelectedSessionId(s.id)}
                className={`flex items-center gap-1.5 rounded-2xl px-3 py-2 text-xs font-semibold shrink-0 transition-all border ${
                  selectedSessionId === s.id
                    ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                    : 'border-border bg-surface text-muted-foreground'
                }`}
              >
                <span>#{s.id}</span>
                <span className="truncate max-w-[120px]">{s.title}</span>
              </button>
            ))}
          </div>
        )}

        {/* Selected Session Details */}
        {loading ? (
          <div className="space-y-3">
            <Skeleton className="h-28 rounded-3xl" />
            <Skeleton className="h-36 rounded-3xl" />
          </div>
        ) : sessionDetail ? (
          <div className="space-y-3.5">
            {/* Run Status Card */}
            <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-xs font-bold text-foreground truncate">{sessionDetail.session.title}</h3>
                  <span className="text-[10px] text-muted-foreground">
                    状态: {sessionDetail.session.status}
                  </span>
                </div>
                <Badge tone="primary" variant="subtle" className="text-[9px]">
                  {sessionDetail.session.status}
                </Badge>
              </div>

              <div className="flex items-center gap-2 pt-1">
                <Button
                  size="sm"
                  variant="outline"
                  loading={uploading}
                  onClick={() => fileInputRef.current?.click()}
                  icon={<UploadCloud className="h-3.5 w-3.5" />}
                  className="flex-1 h-9 rounded-xl text-xs"
                >
                  上传 PDF 题本
                </Button>
                <Button
                  size="sm"
                  onClick={triggerRun}
                  icon={<Play className="h-3.5 w-3.5 fill-current" />}
                  className="flex-1 h-9 rounded-xl text-xs font-bold shadow-glow"
                >
                  启动智能提取
                </Button>
              </div>
            </div>

            {/* Documents List */}
            <div className="space-y-2">
              <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block px-1">
                关联文档 ({sessionDetail.documents.length})
              </span>
              {sessionDetail.documents.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-border/80 bg-surface p-6 text-center text-xs text-muted-foreground">
                  尚未上传 PDF 试卷文件
                </div>
              ) : (
                sessionDetail.documents.map((doc) => (
                  <div
                    key={doc.id}
                    className="flex items-center justify-between rounded-2xl border border-border/80 bg-surface p-3 text-xs shadow-card"
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <FileText className="h-4 w-4 text-primary shrink-0" />
                      <div className="min-w-0">
                        <span className="font-semibold text-foreground truncate block">{doc.original_name}</span>
                        <span className="text-[10px] text-muted-foreground font-mono">
                          {doc.page_count ?? 0} 页 · {doc.document_role || '待分类'}
                        </span>
                      </div>
                    </div>
                    <Badge tone="info" variant="subtle" className="text-[9px]">
                      {doc.status || '已就绪'}
                    </Badge>
                  </div>
                ))
              )}
            </div>

            {/* Problems Review List */}
            {sessionDetail.problems && sessionDetail.problems.length > 0 && (
              <div className="space-y-2">
                <span className="text-[10px] font-bold uppercase tracking-wider text-amber-600 dark:text-amber-400 block px-1">
                  异常与待排查问题 ({sessionDetail.problems.length})
                </span>
                {sessionDetail.problems.map((p) => (
                  <div
                    key={p.id}
                    className="rounded-2xl border border-amber-500/30 bg-amber-500/5 p-3 text-xs space-y-1 text-foreground"
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-bold">{p.problem_type}</span>
                      <Badge tone="warning" variant="subtle" className="text-[9px]">
                        {p.severity}
                      </Badge>
                    </div>
                    <p className="text-[11px] text-muted-foreground">{p.resolution_hint || p.detail_json || '待人工排查'}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : null}
      </MobilePullToRefresh>

      {/* New Session Sheet */}
      <MobileBottomSheet open={createOpen} onClose={() => setCreateOpen(false)} title="新建 PDF 导入任务">
        <form onSubmit={handleCreateSession} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">任务标题 *</label>
            <input
              className="input h-10 rounded-xl text-xs"
              placeholder="例如：2024年期末真题卷提取"
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              required
              autoFocus
            />
          </div>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={() => setCreateOpen(false)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button type="submit" disabled={!newTitle.trim()} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              创建流水线
            </Button>
          </div>
        </form>
      </MobileBottomSheet>
    </div>
  )
}
