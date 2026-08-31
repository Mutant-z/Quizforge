import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, MasteryIndicator, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { QuestionType, WrongQuestion } from '@/types'
import {
  AlertCircle,
  BookOpen,
  BookX,
  CheckCircle2,
  ChevronDown,
  Clock,
  Edit3,
  Filter,
  Flame,
  Lightbulb,
  Play,
  RotateCcw,
  Search,
  Sparkles,
  Star,
  Trash2,
  X,
  Zap,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const QUESTION_TYPE_LABELS: Record<string, string> = {
  single_choice: '单选',
  multiple_choice: '多选',
  true_false: '判断',
  fill_blank: '填空',
  short_answer: '简答',
  material_analysis: '材料',
  essay: '论述',
  code: '代码',
}

export default function MobileWrongBook() {
  const [list, setList] = useState<WrongQuestion[]>([])
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [keyword, setKeyword] = useState('')
  const [dueOnly, setDueOnly] = useState(false)
  const [summary, setSummary] = useState({ total: 0, due: 0, mastered: 0 })
  const [expandedIds, setExpandedIds] = useState<Record<number, boolean>>({})
  const [reviewingId, setReviewingId] = useState<number | null>(null)

  // Delete confirm sheet
  const [deleteTarget, setDeleteTarget] = useState<WrongQuestion | null>(null)
  const [deleting, setDeleting] = useState(false)

  // Note Sheet
  const [activeNoteTarget, setActiveNoteTarget] = useState<WrongQuestion | null>(null)
  const [noteDraft, setNoteDraft] = useState('')
  const [savingNote, setSavingNote] = useState(false)
  const navigate = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params: Record<string, unknown> = { page_size: 100 }
      if (status) params.status = status
      if (dueOnly) params.due = 'true'

      const [listResp, sumResp] = await Promise.all([
        client.get('/wrong-questions', { params }),
        client.get('/wrong-questions/summary'),
      ])
      setList(listResp.data?.data?.items || [])
      setSummary(sumResp.data?.data || { total: 0, due: 0, mastered: 0 })
    } catch (e) {
      console.error('加载错题列表失败:', e)
    } finally {
      setLoading(false)
    }
  }, [status, dueOnly])

  useEffect(() => {
    load()
  }, [load])

  // Filter list by type and keyword
  const filteredList = list.filter((item) => {
    if (!item) return false
    const q = item.question
    if (typeFilter && q?.type !== typeFilter) return false
    if (keyword.trim()) {
      const kw = keyword.trim().toLowerCase()
      const matchStem = q?.stem?.toLowerCase().includes(kw)
      const matchNote = item.note?.toLowerCase().includes(kw)
      const matchKP = (q?.knowledge_points || []).some((k) => k.toLowerCase().includes(kw))
      if (!matchStem && !matchNote && !matchKP) return false
    }
    return true
  })

  const startDuePractice = async () => {
    try {
      const resp = await client.post('/practice/sessions', {
        mode: dueOnly ? 'due' : 'wrong',
        limit: 20,
      })
      navigate(`/practice/${resp.data.data.id}`)
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const handleReviewRating = async (item: WrongQuestion, result: 'again' | 'hard' | 'good' | 'easy') => {
    setReviewingId(item.id)
    try {
      await client.post(`/wrong-questions/${item.id}/review-result`, { result })
      const ratingLabels = { again: '已重置复习周期', hard: '已缩短复习间隔', good: '已推进下一周期', easy: '已大幅延展记忆期' }
      showToast(`复习评分完成：${ratingLabels[result]}`, 'success')
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setReviewingId(null)
    }
  }

  const handleDeleteWrong = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await client.delete(`/wrong-questions/${deleteTarget.id}`)
      showToast('已从错题本中移除', 'success')
      setDeleteTarget(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setDeleting(false)
    }
  }

  const saveNote = async () => {
    if (!activeNoteTarget) return
    setSavingNote(true)
    try {
      await client.put(`/questions/${activeNoteTarget.question_id}/note`, { content: noteDraft })
      showToast('笔记保存成功', 'success')
      setActiveNoteTarget(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setSavingNote(false)
    }
  }

  return (
    <div className="relative min-h-full">
      {/* Sticky Search & Due toggle */}
      <div className="sticky top-0 z-20 px-4 py-2.5 bg-background/95 backdrop-blur-md border-b border-border/60 space-y-2">
        <div className="relative">
          <Search className="absolute left-3.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
          <input
            type="text"
            placeholder="搜索错题题干、考点或笔记..."
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            className="input h-10 pl-9 pr-8 text-xs rounded-2xl"
          />
          {keyword && (
            <button onClick={() => setKeyword('')} className="absolute right-2.5 top-2.5 text-muted-foreground">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        {/* Filter Pills */}
        <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
          <button
            onClick={() => setDueOnly(!dueOnly)}
            className={`flex items-center gap-1 rounded-full px-3 py-1 text-[11px] font-bold shrink-0 transition-all ${
              dueOnly
                ? 'bg-amber-500 text-white shadow-glow-amber'
                : 'bg-surface border border-amber-500/40 text-amber-600 dark:text-amber-400'
            }`}
          >
            <Clock className="h-3 w-3" />
            <span>到期复习 ({summary.due})</span>
          </button>

          <button
            onClick={() => setStatus(status === 'mastered' ? '' : 'mastered')}
            className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
              status === 'mastered'
                ? 'bg-emerald-500 text-white shadow-glow-success'
                : 'bg-surface border border-border text-muted-foreground'
            }`}
          >
            已掌握 ({summary.mastered})
          </button>

          <button
            onClick={() => setStatus(status === 'wrong' ? '' : 'wrong')}
            className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
              status === 'wrong'
                ? 'bg-destructive text-white shadow-glow-danger'
                : 'bg-surface border border-border text-muted-foreground'
            }`}
          >
            未攻克
          </button>
        </div>
      </div>

      <MobilePullToRefresh onRefresh={load} className="p-4 space-y-3.5 pb-24">
        {/* Ebbinghaus Review Hero Card */}
        <div className="rounded-3xl border border-amber-500/30 bg-gradient-to-br from-amber-500/[0.08] via-surface to-surface p-4 shadow-card space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-amber-500/15 text-amber-600 dark:text-amber-400 shadow-glow-amber">
                <Sparkles className="h-4 w-4" />
              </div>
              <div>
                <h3 className="text-xs font-bold text-foreground">艾宾浩斯记忆队列</h3>
                <span className="text-[10px] text-muted-foreground">根据遗忘曲线自适应衰减</span>
              </div>
            </div>
            <span className="font-mono text-xs font-bold text-amber-600 dark:text-amber-400">
              {summary.due} 题需巩固
            </span>
          </div>

          <div className="flex items-center gap-2 pt-0.5">
            <Button
              size="sm"
              disabled={summary.due === 0 && list.length === 0}
              onClick={startDuePractice}
              icon={<RotateCcw className="h-3.5 w-3.5" />}
              className="flex-1 h-9 rounded-xl font-bold text-xs bg-amber-500 hover:bg-amber-600 text-white shadow-glow-amber border-none"
            >
              开启错题特训 ({summary.due > 0 ? summary.due : list.length})
            </Button>
          </div>
        </div>

        {/* Question Cards */}
        {loading ? (
          <div className="space-y-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-32 rounded-3xl" />
            ))}
          </div>
        ) : filteredList.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-2">
            <CheckCircle2 className="h-10 w-10 text-emerald-500 mx-auto opacity-70" />
            <h3 className="text-sm font-bold text-foreground">错题本已清空！</h3>
            <p className="text-xs text-muted-foreground">当前筛选条件下没有待攻克的错题，继续保持！</p>
          </div>
        ) : (
          <div className="space-y-3">
            {filteredList.map((item) => {
              const isExpanded = expandedIds[item.id] ?? false
              const q = item.question

              return (
                <div
                  key={item.id}
                  className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-1.5">
                      <Badge tone="danger" variant="subtle" className="text-[9px] font-mono font-bold">
                        错 {item.wrong_count} 次
                      </Badge>
                      <Badge tone="neutral" variant="subtle" className="text-[9px]">
                        {QUESTION_TYPE_LABELS[q?.type || ''] || q?.type || '题目'}
                      </Badge>
                      {item.next_review_at && new Date(item.next_review_at) <= new Date() && (
                        <span className="flex items-center gap-0.5 text-[9px] font-bold text-amber-600 dark:text-amber-400 bg-amber-500/10 px-1.5 py-0.2 rounded-md">
                          <Clock className="h-2.5 w-2.5" />
                          已到期
                        </span>
                      )}
                    </div>

                    <MasteryIndicator score={item.mastery_score ?? 0} size="sm" />
                  </div>

                  {/* Stem */}
                  <div className="text-xs text-foreground font-medium leading-relaxed markdown-body">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{q?.stem || '试题题干加载中...'}</ReactMarkdown>
                  </div>

                  {/* Expand Answer Button */}
                  <div className="flex items-center justify-between border-t border-border/60 pt-2.5 text-xs">
                    <button
                      onClick={() => setExpandedIds((prev) => ({ ...prev, [item.id]: !isExpanded }))}
                      className="flex items-center gap-1 font-bold text-primary"
                    >
                      <span>{isExpanded ? '收起解析' : '查看答案与解析'}</span>
                      <ChevronDown className={`h-3.5 w-3.5 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
                    </button>

                    <div className="flex items-center gap-1.5">
                      <button
                        onClick={() => {
                          setActiveNoteTarget(item)
                          setNoteDraft(item.note || '')
                        }}
                        className="text-muted-foreground hover:text-foreground p-1"
                        aria-label="编辑笔记"
                        title="反思笔记"
                      >
                        <Edit3 className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => setDeleteTarget(item)}
                        className="text-muted-foreground hover:text-destructive p-1"
                        aria-label="移除错题"
                        title="移出错题本"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>

                  {/* Expanded Content & Spaced Repetition Ratings */}
                  {isExpanded && (
                    <div className="rounded-2xl border border-border/70 bg-surface-secondary/50 p-3.5 space-y-3 text-xs animate-slide-down">
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-emerald-600 dark:text-emerald-400">参考答案：</span>
                        <span className="font-mono font-bold">
                          {Array.isArray(q?.answer) ? q.answer.join(', ') : String(q?.answer || '无')}
                        </span>
                      </div>

                      {(q?.original_analysis || q?.ai_enhanced_analysis) && (
                        <div className="text-muted-foreground text-[11px] leading-relaxed border-t border-border/50 pt-2 markdown-body">
                          <ReactMarkdown remarkPlugins={[remarkGfm]}>{q.original_analysis || q.ai_enhanced_analysis || ''}</ReactMarkdown>
                        </div>
                      )}

                      {item.note && (
                        <div className="rounded-xl bg-primary/5 p-2 text-[11px] text-primary border border-primary/20">
                          <span className="font-bold">我的笔记：</span> {item.note}
                        </div>
                      )}

                      {/* 4-Level Ebbinghaus Mastery Rating Buttons */}
                      <div className="border-t border-border/60 pt-2 space-y-1.5">
                        <span className="text-[10px] font-bold text-muted-foreground block">
                          自测记忆反馈（艾宾浩斯调度）：
                        </span>
                        <div className="grid grid-cols-4 gap-1.5">
                          <button
                            disabled={reviewingId === item.id}
                            onClick={() => handleReviewRating(item, 'again')}
                            className="py-1.5 rounded-xl border border-destructive/30 bg-destructive/10 text-destructive text-[11px] font-bold active:scale-95 transition-all"
                          >
                            重来
                          </button>
                          <button
                            disabled={reviewingId === item.id}
                            onClick={() => handleReviewRating(item, 'hard')}
                            className="py-1.5 rounded-xl border border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-[11px] font-bold active:scale-95 transition-all"
                          >
                            困难
                          </button>
                          <button
                            disabled={reviewingId === item.id}
                            onClick={() => handleReviewRating(item, 'good')}
                            className="py-1.5 rounded-xl border border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400 text-[11px] font-bold active:scale-95 transition-all"
                          >
                            良好
                          </button>
                          <button
                            disabled={reviewingId === item.id}
                            onClick={() => handleReviewRating(item, 'easy')}
                            className="py-1.5 rounded-xl border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[11px] font-bold active:scale-95 transition-all"
                          >
                            简单
                          </button>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </MobilePullToRefresh>

      {/* Note Editor Sheet */}
      <MobileBottomSheet
        open={!!activeNoteTarget}
        onClose={() => setActiveNoteTarget(null)}
        title="错题反思笔记"
      >
        <div className="space-y-4">
          <textarea
            className="input min-h-[140px] rounded-2xl p-3 text-xs resize-none"
            placeholder="总结这道题的错误原因、盲区概念或记忆口诀..."
            value={noteDraft}
            onChange={(e) => setNoteDraft(e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setActiveNoteTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button loading={savingNote} onClick={saveNote} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              保存笔记
            </Button>
          </div>
        </div>
      </MobileBottomSheet>

      {/* Delete Confirmation Sheet */}
      <MobileBottomSheet
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="移出错题本确认"
        description="确定要将这道错题从错题本中移除吗？移除后将不再纳入艾宾浩斯复习提醒。"
      >
        <div className="flex items-center gap-2 pt-2">
          <Button variant="outline" onClick={() => setDeleteTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
            取消
          </Button>
          <Button
            variant="danger"
            loading={deleting}
            onClick={handleDeleteWrong}
            className="flex-1 h-10 rounded-xl text-xs font-bold"
          >
            确认移除
          </Button>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
