import React, { useCallback, useEffect, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { Question, QuestionType } from '@/types'
import {
  ChevronLeft,
  ChevronRight,
  Database,
  Edit3,
  FileQuestion,
  ListFilter,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const typeLabels: Record<QuestionType, string> = {
  single_choice: '单选题',
  multiple_choice: '多选题',
  true_false: '判断题',
  fill_blank: '填空题',
  short_answer: '简答题',
  material_analysis: '材料分析题',
  essay: '论述题',
  code: '代码题',
}

export default function MobileAdminQuestions() {
  const [list, setList] = useState<Question[]>([])
  const [loading, setLoading] = useState(true)
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)

  // Edit question sheet
  const [editing, setEditing] = useState<Question | null>(null)
  const [saving, setSaving] = useState(false)
  const [reindexingId, setReindexingId] = useState<number | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/admin/questions', {
        params: { q: keyword, page, page_size: 15 },
      })
      setList(resp.data.data.items || [])
      setTotal(resp.data.data.total || 0)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setLoading(false)
    }
  }, [page, keyword])

  useEffect(() => {
    load()
  }, [load])

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editing) return
    setSaving(true)
    try {
      if (editing.id) {
        await client.put(`/admin/questions/${editing.id}`, editing)
      } else {
        await client.post('/admin/questions', editing)
      }
      showToast('试题已保存', 'success')
      setEditing(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (q: Question) => {
    if (!window.confirm(`确定删除试题 #${q.id} 吗？`)) return
    try {
      await client.delete(`/admin/questions/${q.id}`)
      showToast('试题已删除', 'success')
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const reindex = async (q: Question) => {
    setReindexingId(q.id)
    try {
      await client.post(`/admin/questions/${q.id}/reindex`)
      showToast(`试题 #${q.id} 向量索引重建完成`, 'success')
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setReindexingId(null)
    }
  }

  const totalPages = Math.ceil(total / 15) || 1

  return (
    <div className="relative min-h-full select-none">
      {/* Search Header */}
      <div className="sticky top-0 z-20 px-4 py-2.5 bg-background/95 backdrop-blur-md border-b border-border/60">
        <div className="relative">
          <Search className="absolute left-3.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
          <input
            type="text"
            placeholder="搜索全量题库关键词..."
            value={keyword}
            onChange={(e) => {
              setKeyword(e.target.value)
              setPage(1)
            }}
            className="input h-10 pl-9 pr-8 text-xs rounded-2xl"
          />
          {keyword && (
            <button onClick={() => setKeyword('')} className="absolute right-2.5 top-2.5 text-muted-foreground">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      <MobilePullToRefresh onRefresh={load} className="p-4 space-y-3.5 pb-24">
        <div className="flex items-center justify-between px-1">
          <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
            试题列表 ({total} 题)
          </span>
          <span className="text-[10px] text-muted-foreground font-mono">
            第 {page} / {totalPages} 页
          </span>
        </div>

        {loading ? (
          <div className="space-y-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-3xl" />
            ))}
          </div>
        ) : list.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-2">
            <FileQuestion className="h-10 w-10 text-muted-foreground mx-auto opacity-50" />
            <h3 className="text-sm font-bold text-foreground">没有找到匹配试题</h3>
          </div>
        ) : (
          <div className="space-y-3">
            {list.map((q) => (
              <div
                key={q.id}
                className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-2.5"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5">
                    <span className="font-mono text-xs font-bold text-muted-foreground">#{q.id}</span>
                    <Badge tone="primary" variant="subtle" className="text-[9px]">
                      {typeLabels[q.type] || q.type}
                    </Badge>
                  </div>
                  {q.knowledge_points && q.knowledge_points.length > 0 && (
                    <span className="text-[10px] text-muted-foreground font-medium truncate max-w-[150px]">
                      {q.knowledge_points[0]}
                    </span>
                  )}
                </div>

                <div className="text-xs text-foreground font-medium leading-relaxed line-clamp-3 markdown-body">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{q.stem}</ReactMarkdown>
                </div>

                <div className="flex items-center justify-between border-t border-border/60 pt-2 text-xs">
                  <span className="text-[10px] text-muted-foreground font-mono">
                    答案: <span className="text-emerald-600 dark:text-emerald-400 font-bold">{Array.isArray(q.answer) ? q.answer.join(', ') : String(q.answer)}</span>
                  </span>

                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => reindex(q)}
                      disabled={reindexingId === q.id}
                      className="p-1.5 text-muted-foreground hover:text-primary active:scale-90"
                      title="重索向量库"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${reindexingId === q.id ? 'animate-spin' : ''}`} />
                    </button>
                    <button
                      onClick={() => setEditing(q)}
                      className="p-1.5 text-muted-foreground hover:text-foreground active:scale-90"
                      title="编辑"
                    >
                      <Edit3 className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => remove(q)}
                      className="p-1.5 text-muted-foreground hover:text-destructive active:scale-90"
                      title="删除"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination Bar */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between pt-2">
            <Button
              size="xs"
              variant="outline"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              icon={<ChevronLeft className="h-3.5 w-3.5" />}
              className="h-8 px-3 rounded-xl text-xs"
            >
              上一页
            </Button>
            <span className="text-xs font-mono text-muted-foreground font-semibold">
              {page} / {totalPages}
            </span>
            <Button
              size="xs"
              variant="outline"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              className="h-8 px-3 rounded-xl text-xs"
            >
              <span>下一页</span>
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
          </div>
        )}
      </MobilePullToRefresh>

      {/* Edit Question Bottom Sheet */}
      <MobileBottomSheet open={!!editing} onClose={() => setEditing(null)} title="编辑题目内容">
        {editing && (
          <form onSubmit={save} className="space-y-3.5">
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">题干内容 *</label>
              <textarea
                className="input min-h-[100px] rounded-xl text-xs resize-none"
                value={editing.stem}
                onChange={(e) => setEditing({ ...editing, stem: e.target.value })}
                required
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">知识点标签 (逗号分隔)</label>
              <input
                className="input h-9 rounded-xl text-xs"
                value={editing.knowledge_points ? editing.knowledge_points.join(', ') : ''}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    knowledge_points: e.target.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
                  })
                }
                placeholder="例如：快速排序, 递归"
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">解析</label>
              <textarea
                className="input min-h-[80px] rounded-xl text-xs resize-none"
                value={editing.original_analysis || editing.ai_enhanced_analysis || ''}
                onChange={(e) => setEditing({ ...editing, original_analysis: e.target.value })}
              />
            </div>

            <div className="flex items-center gap-2 pt-2">
              <Button type="button" variant="outline" onClick={() => setEditing(null)} className="flex-1 h-10 rounded-xl text-xs">
                取消
              </Button>
              <Button type="submit" loading={saving} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
                保存题目
              </Button>
            </div>
          </form>
        )}
      </MobileBottomSheet>
    </div>
  )
}
