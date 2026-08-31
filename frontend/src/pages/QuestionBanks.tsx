import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Badge, Button, Card, EmptyState, Modal, PageHeader, Skeleton } from '@/components/ui'
import PracticeConfigModal, { type PracticeLaunchTarget } from '@/components/practice/PracticeConfigModal'
import { AlertTriangle, GraduationCap, Library, Play, Plus, Search, Trash2 } from 'lucide-react'
import type { QuestionBank } from '@/types'

export default function QuestionBanks() {
  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [desc, setDesc] = useState('')
  const [visibility, setVisibility] = useState<'public' | 'private'>('private')
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<QuestionBank | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [practiceTarget, setPracticeTarget] = useState<PracticeLaunchTarget | null>(null)
  const user = useAuthStore((s) => s.user)
  const navigate = useNavigate()

  const load = async () => {
    setLoading(true)
    try {
      const resp = await client.get('/question-banks', { params: { page_size: 50 } })
      setBanks(resp.data.data.items)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const create = async () => {
    if (!name.trim()) return
    setCreating(true)
    try {
      await client.post('/question-banks', { name, description: desc, visibility })
      setName('')
      setDesc('')
      setVisibility('private')
      setCreateOpen(false)
      load()
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await client.delete(`/question-banks/${deleteTarget.id}`)
      setDeleteTarget(null)
      load()
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setDeleting(false)
    }
  }

  const filtered = banks.filter(
    (b) =>
      b.name.toLowerCase().includes(search.toLowerCase()) ||
      (b.description && b.description.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <PageHeader
        title="题库空间"
        subtitle="选择专业题库进入科目与章节，开始沉浸式自适应刷题。"
        action={
          <div className="flex items-center gap-2.5">
            <div className="relative">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/70" />
              <input
                type="text"
                placeholder="搜索题库..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="input h-9 w-48 sm:w-64 pl-9 text-xs"
              />
            </div>
            <Button
              size="sm"
              onClick={() => setCreateOpen(true)}
              icon={<Plus className="h-4 w-4" />}
              className="shadow-subtle"
            >
              新建题库
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => navigate('/practice/setup')}
              icon={<Play className="h-3.5 w-3.5 fill-current" />}
            >
              自定义刷题
            </Button>
          </div>
        }
      />

      {/* Grid List */}
      {loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[...Array(6)].map((_, i) => (
            <Skeleton key={i} className="h-44 rounded-3xl" />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <Card className="p-8">
          <EmptyState
            icon={Library}
            title={search ? '没有匹配的题库' : '还没有题库'}
            description={
              search
                ? '尝试更换搜索关键词'
                : '你可以创建新题库，或者前往「PDF 导入 Pipeline」快速生成试题。'
            }
            action={
              <Button size="sm" onClick={() => setCreateOpen(true)} icon={<Plus className="h-4 w-4" />}>
                立即创建
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((b) => (
            <div
              key={b.id}
              onClick={() => navigate(`/question-bank/${b.id}`)}
              className="group relative flex flex-col justify-between rounded-3xl border border-border/80 bg-surface p-5 sm:p-6 shadow-card transition-all duration-200 hover:border-primary/40 hover:shadow-elevated hover:-translate-y-0.5 cursor-pointer"
            >
              <div>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary/10 text-primary transition-transform group-hover:scale-105 shadow-subtle">
                    <GraduationCap className="h-5 w-5" />
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Badge
                      tone={b.visibility === 'public' ? 'info' : 'neutral'}
                      variant="subtle"
                    >
                      {b.visibility === 'public' ? '公开题库' : '私有题库'}
                    </Badge>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        setDeleteTarget(b)
                      }}
                      className="flex h-7 w-7 items-center justify-center rounded-lg text-muted-foreground/60 hover:text-danger hover:bg-danger/10 opacity-0 group-hover:opacity-100 transition-all"
                      title="删除题库"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                <h3 className="mt-4 text-base font-bold tracking-tight text-foreground group-hover:text-primary transition-colors">
                  {b.name}
                </h3>
                <p className="mt-1.5 line-clamp-2 text-xs text-muted-foreground leading-relaxed">
                  {b.description || '暂无详细描述'}
                </p>
              </div>

              <div className="mt-6 flex items-center justify-between border-t border-border/60 pt-3 text-[11px] text-muted-foreground">
                <span className="font-bold text-foreground font-mono">
                  {b.question_count} <span className="font-normal text-muted-foreground">题</span>
                </span>
                <span>{b.subject_count ?? 0} 个科目</span>
                <Button
                  variant="subtle"
                  size="xs"
                  onClick={(event) => {
                    event.stopPropagation()
                    setPracticeTarget({ bankIds: [b.id], title: b.name })
                  }}
                  icon={<Play className="h-3 w-3 fill-current" />}
                >
                  刷题
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create Modal */}
      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="新建题库"
        description="创建一个全新的题库容器，后续可添加科目、章节与导入试题。"
      >
        <div className="space-y-4">
          <div>
            <label className="label mb-1.5 block">题库名称</label>
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：计算机系统基础 / Java 高级进阶"
              autoFocus
            />
          </div>
          <div>
            <label className="label mb-1.5 block">题库描述（可选）</label>
            <textarea
              className="input h-24 resize-none text-xs leading-relaxed"
              value={desc}
              onChange={(e) => setDesc(e.target.value)}
              placeholder="简要说明题库涵盖的考点与适用对象..."
            />
          </div>
          <div>
            <label className="label mb-1.5 block">可见性</label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setVisibility('private')}
                className={`flex-1 rounded-xl border p-2.5 text-xs font-medium transition-all ${
                  visibility === 'private'
                    ? 'border-primary bg-primary/10 text-primary font-semibold'
                    : 'border-border/80 bg-surface hover:bg-surface-secondary text-muted-foreground'
                }`}
              >
                私有题库
              </button>
              <button
                type="button"
                onClick={() => setVisibility('public')}
                className={`flex-1 rounded-xl border p-2.5 text-xs font-medium transition-all ${
                  visibility === 'public'
                    ? 'border-primary bg-primary/10 text-primary font-semibold'
                    : 'border-border/80 bg-surface hover:bg-surface-secondary text-muted-foreground'
                }`}
              >
                公开题库
              </button>
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button onClick={create} loading={creating} disabled={!name.trim()}>
              确认创建
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => !deleting && setDeleteTarget(null)}
        title="确认删除题库"
        description="此操作不可撤销，题库下的所有科目、章节及试题将一并被删除。"
      >
        <div className="space-y-4">
          <div className="flex items-start gap-3 rounded-2xl bg-danger/10 p-3.5 text-xs text-danger">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
            <div className="leading-relaxed">
              确定要删除题库 <span className="font-bold underline">「{deleteTarget?.name}」</span> 吗？删除后相关题目与练习历史将无法恢复。
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
            <Button variant="outline" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              取消
            </Button>
            <Button variant="danger" onClick={handleDelete} loading={deleting} icon={<Trash2 className="h-3.5 w-3.5" />}>
              确认删除
            </Button>
          </div>
        </div>
      </Modal>

      <PracticeConfigModal
        open={!!practiceTarget}
        target={practiceTarget}
        onClose={() => setPracticeTarget(null)}
      />
    </div>
  )
}
