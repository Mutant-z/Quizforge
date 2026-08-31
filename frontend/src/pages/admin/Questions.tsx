import { useCallback, useEffect, useState } from 'react'
import client, { errMsg } from '@/api/client'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  IconButton,
  Modal,
  PageHeader,
  Skeleton,
} from '@/components/ui'
import type { Question, QuestionType } from '@/types'
import {
  ChevronLeft,
  ChevronRight,
  Edit,
  FileQuestion,
  Plus,
  RefreshCw,
  Search,
  Trash2,
} from 'lucide-react'

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

export default function AdminQuestions() {
  const [list, setList] = useState<Question[]>([])
  const [loading, setLoading] = useState(true)
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [editing, setEditing] = useState<Question | null>(null)
  const [saving, setSaving] = useState(false)
  const [reindexingId, setReindexingId] = useState<number | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/admin/questions', {
        params: { q: keyword, page, page_size: 20 },
      })
      setList(resp.data.data.items)
      setTotal(resp.data.data.total)
    } finally {
      setLoading(false)
    }
  }, [page, keyword])

  useEffect(() => {
    load()
  }, [load])

  const save = async () => {
    if (!editing) return
    setSaving(true)
    try {
      if (editing.id) {
        await client.put(`/admin/questions/${editing.id}`, editing)
      } else {
        await client.post('/admin/questions', editing)
      }
      setEditing(null)
      load()
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setSaving(false)
    }
  }

  const remove = async (q: Question) => {
    if (!confirm(`确定删除题目 #${q.id}？删除后不可恢复。`)) return
    try {
      await client.delete(`/admin/questions/${q.id}`)
      load()
    } catch (e) {
      alert(errMsg(e))
    }
  }

  const reindex = async (q: Question) => {
    setReindexingId(q.id)
    try {
      await client.post(`/admin/questions/${q.id}/reindex`)
      alert(`题目 #${q.id} 已提交向量数据库重新索引`)
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setReindexingId(null)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / 20))

  return (
    <div className="space-y-6">
      <PageHeader
        title="题库题目管理"
        subtitle="浏览、检索、编辑或手动录入全量题库题目，支持向量重新索引与考点标记。"
        action={
          <div className="flex items-center gap-2.5">
            <div className="relative">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/70" />
              <input
                type="text"
                placeholder="检索题干关键词..."
                value={keyword}
                onChange={(e) => {
                  setKeyword(e.target.value)
                  setPage(1)
                }}
                className="input h-9 w-48 sm:w-64 pl-9 text-xs"
              />
            </div>
            <Button
              size="sm"
              onClick={() =>
                setEditing({
                  id: 0,
                  bank_id: 1,
                  type: 'single_choice',
                  stem: '',
                  options: [
                    { key: 'A', content: '' },
                    { key: 'B', content: '' },
                    { key: 'C', content: '' },
                    { key: 'D', content: '' },
                  ],
                  answer: ['A'],
                  difficulty: 3,
                  quality_score: 5,
                  status: 'published',
                  version: 1,
                  created_at: new Date().toISOString(),
                  updated_at: new Date().toISOString(),
                })
              }
              icon={<Plus className="h-4 w-4" />}
              className="shadow-subtle"
            >
              录入新题
            </Button>
          </div>
        }
      />

      {/* Question List */}
      <Card>
        {loading ? (
          <div className="p-6 space-y-3">
            {[...Array(5)].map((_, i) => (
              <Skeleton key={i} className="h-20 rounded-2xl" />
            ))}
          </div>
        ) : list.length === 0 ? (
          <EmptyState
            icon={FileQuestion}
            title={keyword ? '未找到相关题目' : '当前无题目'}
            description="你可以尝试清除搜索条件，或点击上方按钮录入新题目。"
          />
        ) : (
          <div className="divide-y divide-border/60">
            {list.map((q) => (
              <div
                key={q.id}
                className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 p-4 sm:p-5 transition-colors hover:bg-surface-secondary/40"
              >
                <div className="space-y-1.5 min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap text-xs">
                    <span className="font-mono font-bold text-foreground">#{q.id}</span>
                    <Badge tone="primary" variant="subtle" className="text-[10px] font-medium">
                      {typeLabels[q.type]}
                    </Badge>
                    <Badge
                      tone={
                        q.status === 'published'
                          ? 'success'
                          : q.status === 'needs_review'
                            ? 'warning'
                            : 'neutral'
                      }
                      variant="outline"
                      className="text-[10px]"
                    >
                      {q.status}
                    </Badge>
                    <span className="text-muted-foreground text-[11px] font-mono">
                      难度 {q.difficulty}/5
                    </span>
                    <span className="text-muted-foreground text-[11px]">
                      题库 #{q.bank_id}
                    </span>
                  </div>

                  <p className="line-clamp-2 text-xs text-foreground/90 font-medium leading-relaxed">
                    {q.stem}
                  </p>

                  <div className="flex items-center gap-3 text-[11px] text-muted-foreground">
                    <span>
                      答案: <b className="text-success font-mono font-bold">{q.answer.join('、')}</b>
                    </span>
                    {q.knowledge_points && q.knowledge_points.length > 0 && (
                      <span>考点: {q.knowledge_points.join(', ')}</span>
                    )}
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1.5 shrink-0 self-end sm:self-center">
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={() => setEditing({ ...q })}
                    icon={<Edit className="h-3 w-3" />}
                  >
                    编辑
                  </Button>
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={() => reindex(q)}
                    loading={reindexingId === q.id}
                    icon={<RefreshCw className="h-3 w-3" />}
                    title="重新生成知识库向量索引"
                  >
                    向量索引
                  </Button>
                  <IconButton
                    variant="ghost"
                    size="xs"
                    onClick={() => remove(q)}
                    className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                    title="删除"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </IconButton>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination Toolbar */}
        {!loading && total > 0 && (
          <div className="flex items-center justify-between border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
            <span>
              共 <b className="text-foreground font-mono">{total}</b> 道题目 · 当前第 <b className="text-foreground font-mono">{page}</b> / <span className="font-mono">{totalPages}</span> 页
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="xs"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
                icon={<ChevronLeft className="h-3.5 w-3.5" />}
              >
                上一页
              </Button>
              <Button
                variant="outline"
                size="xs"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                icon={<ChevronRight className="h-3.5 w-3.5" />}
              >
                下一页
              </Button>
            </div>
          </div>
        )}
      </Card>

      {/* Edit & Create Modal */}
      <Modal
        open={!!editing}
        onClose={() => setEditing(null)}
        title={editing?.id ? `编辑题目 #${editing.id}` : '录入新题目'}
        width="max-w-2xl"
      >
        {editing && (
          <div className="space-y-4">
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="label mb-1 block">所属题库 ID</label>
                <input
                  className="input text-xs"
                  type="number"
                  value={editing.bank_id || 1}
                  onChange={(e) => setEditing({ ...editing, bank_id: Number(e.target.value) })}
                />
              </div>
              <div>
                <label className="label mb-1 block">题目类型</label>
                <select
                  className="input text-xs"
                  value={editing.type || 'single_choice'}
                  onChange={(e) => setEditing({ ...editing, type: e.target.value as QuestionType })}
                >
                  {(Object.keys(typeLabels) as QuestionType[]).map((t) => (
                    <option key={t} value={t}>
                      {typeLabels[t]}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="label mb-1 block">难度 (1-5)</label>
                <input
                  className="input text-xs"
                  type="number"
                  min={1}
                  max={5}
                  value={editing.difficulty || 3}
                  onChange={(e) =>
                    setEditing({ ...editing, difficulty: Number(e.target.value) })
                  }
                />
              </div>
            </div>

            <div>
              <label className="label mb-1 block">题干内容 (支持 Markdown)</label>
              <textarea
                className="input h-24 resize-y text-xs font-mono p-2.5 leading-relaxed"
                value={editing.stem ?? ''}
                onChange={(e) => setEditing({ ...editing, stem: e.target.value })}
                placeholder="请输入题干描述..."
              />
            </div>

            {editing.type !== 'fill_blank' &&
            editing.type !== 'short_answer' &&
              editing.type !== 'material_analysis' &&
              editing.type !== 'essay' &&
              editing.type !== 'code' && (
                <div>
                  <label className="label mb-1 block">选项列表 JSON</label>
                  <textarea
                    className="input h-24 resize-y font-mono text-xs p-2.5 leading-relaxed"
                    value={JSON.stringify(editing.options ?? [], null, 2)}
                    onChange={(e) => {
                      try {
                        setEditing({ ...editing, options: JSON.parse(e.target.value) })
                      } catch {
                        // ignore parsing in typing
                      }
                    }}
                  />
                </div>
              )}

            <div>
              <label className="label mb-1 block">标准答案 JSON (如 ["A"] 或 ["A","B"])</label>
              <input
                className="input font-mono text-xs"
                value={JSON.stringify(editing.answer ?? [])}
                onChange={(e) => {
                  try {
                    setEditing({ ...editing, answer: JSON.parse(e.target.value) })
                  } catch {
                    // ignore
                  }
                }}
              />
            </div>

            <div>
              <label className="label mb-1 block">解析说明</label>
              <textarea
                className="input h-20 resize-y text-xs p-2.5 leading-relaxed"
                value={editing.original_analysis ?? ''}
                onChange={(e) =>
                  setEditing({ ...editing, original_analysis: e.target.value })
                }
                placeholder="解析推导过程..."
              />
            </div>

            <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
              <Button variant="outline" onClick={() => setEditing(null)}>
                取消
              </Button>
              <Button onClick={save} loading={saving}>
                保存题目
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
