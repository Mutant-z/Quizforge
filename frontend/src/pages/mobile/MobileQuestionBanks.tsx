import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { QuestionBank } from '@/types'
import {
  AlertTriangle,
  GraduationCap,
  Library,
  MoreVertical,
  Play,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react'

export default function MobileQuestionBanks() {
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
  const navigate = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/question-banks', { params: { page_size: 50 } })
      setBanks(resp.data.data.items)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setCreating(true)
    try {
      await client.post('/question-banks', { name, description: desc, visibility })
      showToast('题库创建成功', 'success')
      setName('')
      setDesc('')
      setVisibility('private')
      setCreateOpen(false)
      load()
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await client.delete(`/question-banks/${deleteTarget.id}`)
      showToast('题库已成功删除', 'success')
      setDeleteTarget(null)
      load()
    } catch (err) {
      showToast(errMsg(err), 'error')
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
    <div className="relative min-h-full">
      {/* Sticky Search Bar */}
      <div className="sticky top-0 z-20 px-4 py-2.5 bg-background/95 backdrop-blur-md border-b border-border/60">
        <div className="relative">
          <Search className="absolute left-3.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
          <input
            type="text"
            placeholder="搜索题库名称或简介..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input h-10 pl-9 pr-8 text-xs rounded-2xl"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              className="absolute right-2.5 top-2.5 text-muted-foreground p-0.5"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      <MobilePullToRefresh onRefresh={load} className="p-4 space-y-3 pb-24">
        {loading ? (
          <div className="space-y-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-3xl" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-3">
            <Library className="h-10 w-10 text-muted-foreground mx-auto opacity-50" />
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-foreground">{search ? '无匹配题库' : '还没有题库'}</h3>
              <p className="text-xs text-muted-foreground">
                {search ? '换个搜索关键词试试' : '点击下方按钮快速创建题库'}
              </p>
            </div>
            <Button size="sm" onClick={() => setCreateOpen(true)} icon={<Plus className="h-4 w-4" />}>
              立即创建
            </Button>
          </div>
        ) : (
          filtered.map((b) => (
            <div
              key={b.id}
              onClick={() => navigate(`/practice/setup?bank_id=${b.id}`)}
              className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card active:scale-[0.99] transition-all cursor-pointer space-y-3"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-start gap-3 min-w-0">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                    <GraduationCap className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <h3 className="text-sm font-bold text-foreground truncate">{b.name}</h3>
                    <p className="text-xs text-muted-foreground line-clamp-2 mt-0.5 leading-relaxed">
                      {b.description || '暂无详细描述'}
                    </p>
                  </div>
                </div>

                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    setDeleteTarget(b)
                  }}
                  className="flex h-7 w-7 items-center justify-center rounded-xl text-muted-foreground hover:text-destructive active:bg-destructive/10"
                  aria-label="删除题库"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>

              <div className="flex items-center justify-between border-t border-border/60 pt-2.5 text-xs text-muted-foreground font-mono">
                <div className="flex items-center gap-2">
                  <span className="font-bold text-foreground">{b.question_count} <span className="font-normal text-muted-foreground">题</span></span>
                  <span>·</span>
                  <span>{b.subject_count ?? 0} 科目</span>
                  <span>·</span>
                  <Badge tone={b.visibility === 'public' ? 'info' : 'neutral'} variant="subtle" className="text-[9px] py-0 px-1.5 font-sans">
                    {b.visibility === 'public' ? '公开' : '私有'}
                  </Badge>
                </div>

                <Button
                  size="xs"
                  variant="subtle"
                  onClick={(e) => {
                    e.stopPropagation()
                    navigate(`/practice/setup?bank_id=${b.id}`)
                  }}
                  icon={<Play className="h-3 w-3 fill-current" />}
                >
                  开始刷题
                </Button>
              </div>
            </div>
          ))
        )}
      </MobilePullToRefresh>

      {/* Floating Action Button (FAB) for Create */}
      <button
        onClick={() => setCreateOpen(true)}
        className="fixed bottom-[calc(env(safe-area-inset-bottom)+4.5rem)] right-5 z-40 flex h-13 w-13 items-center justify-center rounded-full bg-gradient-to-r from-primary to-primary-hover text-white shadow-float active:scale-95 transition-transform"
        aria-label="新建题库"
      >
        <Plus className="h-6 w-6" />
      </button>

      {/* Create Bank Bottom Sheet */}
      <MobileBottomSheet
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="新建题库"
        description="创建专属题库空间，建立学科章节并录入试题。"
      >
        <form onSubmit={create} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">题库名称 *</label>
            <input
              className="input rounded-xl text-xs h-10"
              placeholder="例如：2025 计算机考研 408"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoFocus
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">题库简介</label>
            <textarea
              className="input rounded-xl text-xs h-20 resize-none"
              placeholder="描述题库适用考试、考点范围及来源..."
              value={desc}
              onChange={(e) => setDesc(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">公开权限</label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setVisibility('private')}
                className={`py-2 px-3 rounded-xl border text-xs font-semibold transition-all ${
                  visibility === 'private'
                    ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                    : 'border-border bg-surface text-muted-foreground'
                }`}
              >
                私有题库 (仅自己可见)
              </button>
              <button
                type="button"
                onClick={() => setVisibility('public')}
                className={`py-2 px-3 rounded-xl border text-xs font-semibold transition-all ${
                  visibility === 'public'
                    ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                    : 'border-border bg-surface text-muted-foreground'
                }`}
              >
                公开题库 (全站可见)
              </button>
            </div>
          </div>

          <div className="pt-2 flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setCreateOpen(false)}
              className="flex-1 h-10 rounded-xl text-xs"
            >
              取消
            </Button>
            <Button
              type="submit"
              loading={creating}
              disabled={!name.trim()}
              className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow"
            >
              确认创建
            </Button>
          </div>
        </form>
      </MobileBottomSheet>

      {/* Delete Bank Confirmation Bottom Sheet */}
      <MobileBottomSheet
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="删除题库确认"
      >
        <div className="space-y-4">
          <div className="flex items-start gap-3 rounded-2xl border border-destructive/20 bg-destructive/10 p-3.5 text-xs text-destructive">
            <AlertTriangle className="h-5 w-5 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold block">此操作不可撤销！</span>
              <span>
                确定要删除题库「{deleteTarget?.name}」吗？题库下所有科目、章节和题目将被永久清除。
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={() => setDeleteTarget(null)}
              className="flex-1 h-10 rounded-xl text-xs"
            >
              取消
            </Button>
            <Button
              variant="danger"
              loading={deleting}
              onClick={handleDelete}
              className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow-danger"
            >
              彻底删除
            </Button>
          </div>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
