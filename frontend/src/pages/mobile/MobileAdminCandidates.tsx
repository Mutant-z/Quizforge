import React, { useCallback, useEffect, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { CandidateQuestion } from '@/types/import'
import {
  Check,
  CheckCircle2,
  ChevronDown,
  FileSearch,
  Sparkles,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const statusLabels: Record<string, string> = {
  needs_review: '待人工审核',
  failed: '提取失败',
  approved: '已通过',
  rejected: '已驳回',
}

export default function MobileAdminCandidates() {
  const [candidates, setCandidates] = useState<CandidateQuestion[]>([])
  const [status, setStatus] = useState('needs_review')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [acting, setActing] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params: Record<string, unknown> = { page_size: 50 }
      if (status) params.status = status
      const resp = await client.get('/admin/candidates', { params })
      const items: CandidateQuestion[] = resp.data.data.items || []
      setCandidates(items)
      if (items.length > 0) setSelectedId(items[0].id)
      else setSelectedId(null)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => {
    load()
  }, [load])

  const approve = async (id: number) => {
    setActing(true)
    try {
      await client.post(`/admin/candidates/${id}/approve`)
      showToast('题目已成功通过审核并入库', 'success')
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setActing(false)
    }
  }

  const reject = async (id: number) => {
    setActing(true)
    try {
      await client.post(`/admin/candidates/${id}/reject`)
      showToast('已驳回该候选题目', 'info')
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setActing(false)
    }
  }

  return (
    <div className="relative min-h-full select-none">
      {/* Sticky Status Chips */}
      <div className="sticky top-0 z-20 px-4 py-2.5 bg-background/95 backdrop-blur-md border-b border-border/60">
        <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
          {Object.entries(statusLabels).map(([k, label]) => (
            <button
              key={k}
              onClick={() => setStatus(k)}
              className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
                status === k
                  ? 'bg-primary text-white shadow-subtle font-bold'
                  : 'bg-surface border border-border text-muted-foreground'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <MobilePullToRefresh onRefresh={load} className="p-4 space-y-3.5 pb-24">
        {loading ? (
          <div className="space-y-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-32 rounded-3xl" />
            ))}
          </div>
        ) : candidates.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-2">
            <CheckCircle2 className="h-10 w-10 text-emerald-500 mx-auto opacity-70" />
            <h3 className="text-sm font-bold text-foreground">当前队列为空</h3>
            <p className="text-xs text-muted-foreground">没有处于「{statusLabels[status]}」状态的候选题</p>
          </div>
        ) : (
          candidates.map((c) => {
            let extracted: { stem?: string; answer?: string[]; analysis?: string; confidence?: number } = {}
            if (c.extracted_json) {
              try {
                extracted = JSON.parse(c.extracted_json)
              } catch {
                extracted = {}
              }
            }

            return (
              <div
                key={c.id}
                className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5">
                    <span className="font-mono text-xs font-bold text-muted-foreground">#{c.id}</span>
                    <Badge tone="warning" variant="subtle" className="text-[9px]">
                      置信度 {Math.round((extracted.confidence || 0.5) * 100)}%
                    </Badge>
                  </div>
                  <span className="text-[10px] text-muted-foreground font-mono">
                    页码 P.{c.page_start}
                  </span>
                </div>

                {/* Extracted Stem */}
                <div className="text-xs text-foreground font-medium leading-relaxed markdown-body">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>
                    {extracted.stem || c.raw_text || '无提取题干'}
                  </ReactMarkdown>
                </div>

                {/* Extracted Answer */}
                {extracted.answer && (
                  <div className="rounded-xl bg-surface-secondary/70 p-2.5 text-xs">
                    <span className="font-bold text-emerald-600 dark:text-emerald-400">标准答案：</span>
                    <span className="font-mono">{Array.isArray(extracted.answer) ? extracted.answer.join(', ') : extracted.answer}</span>
                  </div>
                )}

                {/* One-tap Approve / Reject Actions for Needs Review */}
                {status === 'needs_review' && (
                  <div className="flex items-center gap-2 pt-1 border-t border-border/60">
                    <Button
                      size="xs"
                      variant="outline"
                      loading={acting}
                      onClick={() => reject(c.id)}
                      icon={<X className="h-3 w-3 text-destructive" />}
                      className="flex-1 h-8 rounded-xl text-[11px]"
                    >
                      驳回
                    </Button>
                    <Button
                      size="xs"
                      loading={acting}
                      onClick={() => approve(c.id)}
                      icon={<Check className="h-3 w-3" />}
                      className="flex-1 h-8 rounded-xl text-[11px] font-bold shadow-glow"
                    >
                      通过审核
                    </Button>
                  </div>
                )}
              </div>
            )
          })
        )}
      </MobilePullToRefresh>
    </div>
  )
}
