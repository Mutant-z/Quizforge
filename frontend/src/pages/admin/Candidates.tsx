import { useCallback, useEffect, useState } from 'react'
import client from '@/api/client'
import { Badge, Button, Card, EmptyState, PageHeader, Skeleton } from '@/components/ui'
import type { CandidateQuestion } from '@/types/import'
import {
  Check,
  CheckCircle2,
  FileSearch,
  Sparkles,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const statusLabels: Record<string, string> = {
  pending: '待处理',
  extracted: '已提取',
  persisted: '已入库',
  duplicate: '重复跳过',
  conflict: '题目冲突',
  needs_review: '待人工审核',
  failed: '提取失败',
  approved: '已通过审核',
  rejected: '已驳回拒绝',
}

export default function AdminCandidates() {
  const [candidates, setCandidates] = useState<CandidateQuestion[]>([])
  const [status, setStatus] = useState('needs_review')
  const [selected, setSelected] = useState<CandidateQuestion | null>(null)
  const [loading, setLoading] = useState(true)
  const [acting, setActing] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params: Record<string, unknown> = { page_size: 50 }
      if (status) params.status = status
      const resp = await client.get('/admin/candidates', { params })
      const items: CandidateQuestion[] = resp.data.data.items
      setCandidates(items)
      // Auto select first candidate in list if not selected
      if (items.length > 0) {
        setSelected((prev) => (prev ? items.find((it) => it.id === prev.id) || items[0] : items[0]))
      } else {
        setSelected(null)
      }
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
      load()
    } finally {
      setActing(false)
    }
  }

  const reject = async (id: number) => {
    setActing(true)
    try {
      await client.post(`/admin/candidates/${id}/reject`)
      load()
    } finally {
      setActing(false)
    }
  }

  let extracted: {
    stem?: string
    type?: string
    answer?: string[]
    analysis?: string
    confidence?: number
    options?: { key: string; content: string }[]
  } | null = null

  if (selected?.extracted_json) {
    try {
      extracted = JSON.parse(selected.extracted_json)
    } catch {
      extracted = null
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="候选题目人工审核"
        subtitle="AI 在 PDF 解析中遇到低置信度、公式复杂或格式歧义的题目时会自动进入此审核队列。支持双栏对比与快速核准。"
        action={
          <div className="flex items-center gap-2">
            <select
              className="input h-9 w-auto text-xs py-1 pl-3 pr-8 rounded-xl"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setSelected(null)
              }}
            >
              <option value="needs_review">待人工审核 (Needs Review)</option>
              <option value="extracted">提取候选 (Extracted)</option>
              <option value="failed">提取失败 (Failed)</option>
              <option value="duplicate">重复项 (Duplicate)</option>
              <option value="conflict">冲突项 (Conflict)</option>
            </select>
          </div>
        }
      />

      {loading ? (
        <div className="grid gap-4 lg:grid-cols-12">
          <div className="lg:col-span-5 space-y-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-3xl" />
            ))}
          </div>
          <div className="lg:col-span-7">
            <Skeleton className="h-96 rounded-3xl" />
          </div>
        </div>
      ) : candidates.length === 0 ? (
        <Card className="p-10">
          <EmptyState
            icon={CheckCircle2}
            title="当前筛选下无候选题目"
            description="所有导入题目均已处理完毕或无需要审核的低置信度题。"
          />
        </Card>
      ) : (
        /* Split View Review Layout */
        <div className="grid gap-6 lg:grid-cols-12 items-start">
          {/* Left Column: Candidates List */}
          <div className="lg:col-span-5 space-y-2.5 max-h-[calc(100vh-200px)] overflow-y-auto pr-1">
            {candidates.map((cand) => {
              const isCurrent = selected?.id === cand.id
              const conf = cand.confidence ? Math.round(cand.confidence * 100) : null
              const isLowConf = conf !== null && conf < 70

              return (
                <div
                  key={cand.id}
                  onClick={() => setSelected(cand)}
                  className={`cursor-pointer rounded-3xl border p-4 transition-all ${
                    isCurrent
                      ? 'border-primary bg-primary/5 shadow-subtle ring-1 ring-primary/30'
                      : 'border-border/80 bg-surface hover:bg-surface-secondary hover:border-primary/30'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Badge
                        tone={
                          cand.status === 'failed'
                            ? 'danger'
                            : cand.status === 'needs_review'
                              ? 'warning'
                              : 'info'
                        }
                        variant="subtle"
                        className="text-[10px] font-medium"
                      >
                        {statusLabels[cand.status] ?? cand.status}
                      </Badge>
                      <span className="font-mono text-xs font-bold text-foreground">
                        #{cand.id}
                      </span>
                    </div>

                    <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                      <span>P{cand.page_start}-{cand.page_end}</span>
                      {conf !== null && (
                        <span
                          className={`font-mono text-[10px] font-bold px-1.5 py-0.2 rounded-md ${
                            isLowConf
                              ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
                              : 'bg-success/10 text-success'
                          }`}
                        >
                          {conf}%
                        </span>
                      )}
                    </div>
                  </div>

                  <p className="mt-2 line-clamp-2 text-xs text-muted-foreground leading-relaxed">
                    {cand.raw_text.slice(0, 140)}
                  </p>
                </div>
              )
            })}
          </div>

          {/* Right Column: Split Diff & Decision Editor */}
          <div className="lg:col-span-7 rounded-3xl border border-border/80 bg-surface p-6 shadow-card space-y-5 sticky top-4">
            {selected ? (
              <>
                <div className="flex items-center justify-between border-b border-border/60 pb-3">
                  <div>
                    <h3 className="text-sm font-bold tracking-tight text-foreground">
                      候选题 #{selected.id}
                    </h3>
                    <span className="text-xs text-muted-foreground">
                      来源文档第 {selected.page_start} - {selected.page_end} 页 · 任务 #{selected.import_job_id}
                    </span>
                  </div>

                  {/* Decision Action Buttons */}
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => approve(selected.id)}
                      loading={acting}
                      icon={<Check className="h-3.5 w-3.5" />}
                      className="shadow-subtle"
                    >
                      核准入库
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => reject(selected.id)}
                      loading={acting}
                      icon={<X className="h-3.5 w-3.5" />}
                    >
                      驳回废弃
                    </Button>
                  </div>
                </div>

                {/* Split comparison */}
                <div className="grid gap-4 sm:grid-cols-2">
                  {/* Left: Original raw text */}
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between text-xs font-bold text-muted-foreground">
                      <span className="flex items-center gap-1.5">
                        <FileSearch className="h-3.5 w-3.5 text-primary" />
                        PDF 原始扫描片段
                      </span>
                    </div>
                    <pre className="max-h-96 overflow-y-auto whitespace-pre-wrap rounded-2xl border border-border/70 bg-surface-secondary/60 p-3.5 font-mono text-[11px] leading-relaxed text-muted-foreground select-text">
                      {selected.raw_text}
                    </pre>
                  </div>

                  {/* Right: AI Parsed Structured Question */}
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between text-xs font-bold text-muted-foreground">
                      <span className="flex items-center gap-1.5">
                        <Sparkles className="h-3.5 w-3.5 text-primary" />
                        AI 提取与对齐结果
                      </span>
                    </div>

                    {extracted ? (
                      <div className="max-h-96 overflow-y-auto rounded-2xl border border-border/70 bg-surface p-3.5 text-xs space-y-3">
                        <div>
                          <span className="font-bold text-muted-foreground text-[10px] uppercase">
                            题型 / 置信度
                          </span>
                          <div className="flex items-center gap-2 mt-0.5">
                            <Badge tone="primary" variant="subtle">
                              {extracted.type || 'single_choice'}
                            </Badge>
                            {extracted.confidence && (
                              <span className="font-mono text-[11px] text-muted-foreground font-semibold">
                                置信评分: {Math.round(extracted.confidence * 100)}%
                              </span>
                            )}
                          </div>
                        </div>

                        <div>
                          <span className="font-bold text-muted-foreground text-[10px] uppercase">
                            提取题干
                          </span>
                          <div className="mt-1 font-medium leading-relaxed markdown-body">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>
                              {extracted.stem || '未提取到题干'}
                            </ReactMarkdown>
                          </div>
                        </div>

                        {extracted.options && extracted.options.length > 0 && (
                          <div>
                            <span className="font-bold text-muted-foreground text-[10px] uppercase">
                              提取选项
                            </span>
                            <div className="mt-1 space-y-1">
                              {extracted.options.map((opt) => (
                                <div
                                  key={opt.key}
                                  className="flex items-center gap-2 rounded-xl bg-surface-secondary/60 px-2.5 py-1 text-[11px]"
                                >
                                  <span className="font-bold text-primary font-mono">{opt.key}.</span>
                                  <span>{opt.content}</span>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        <div>
                          <span className="font-bold text-muted-foreground text-[10px] uppercase">
                            识别答案
                          </span>
                          <div className="mt-0.5 font-mono font-bold text-success">
                            {extracted.answer?.join('、') || '暂无答案'}
                          </div>
                        </div>

                        {extracted.analysis && (
                          <div>
                            <span className="font-bold text-muted-foreground text-[10px] uppercase">
                              解析说明
                            </span>
                            <div className="mt-0.5 text-muted-foreground leading-relaxed markdown-body">
                              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                                {extracted.analysis}
                              </ReactMarkdown>
                            </div>
                          </div>
                        )}
                      </div>
                    ) : (
                      <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-4 text-center text-xs text-destructive">
                        未提取到有效结构化数据 {selected.error_code && `(${selected.error_code})`}
                      </div>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className="py-20 text-center text-xs text-muted-foreground">
                请在左侧列表中点击选择候选题进行审核与对比
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
