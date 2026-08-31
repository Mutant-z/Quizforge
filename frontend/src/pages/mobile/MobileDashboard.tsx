import React, { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import client from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import type { QuestionBank, StatOverview } from '@/types'
import {
  ArrowRight,
  BookOpen,
  CheckCircle2,
  Clock,
  Compass,
  FileSearch,
  Flame,
  GraduationCap,
  ImagePlus,
  Library,
  Play,
  Plus,
  RotateCcw,
  Sparkles,
  Target,
  TrendingUp,
  UploadCloud,
  Zap,
} from 'lucide-react'

export default function MobileDashboard() {
  const user = useAuthStore((s) => s.user)
  const navigate = useNavigate()
  const [overview, setOverview] = useState<StatOverview | null>(null)
  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [weakPoints, setWeakPoints] = useState<{ knowledge_point: string; wrong_count: number }[]>([])
  const [loading, setLoading] = useState(true)

  const loadData = useCallback(async () => {
    try {
      const [ov, bk, wk] = await Promise.all([
        client.get('/statistics/overview'),
        client.get('/question-banks', { params: { page_size: 6 } }),
        client.get('/statistics/weak-knowledge-points'),
      ])
      setOverview(ov.data.data)
      setBanks(bk.data.data.items)
      setWeakPoints(wk.data.data)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  const hour = new Date().getHours()
  const greeting =
    hour < 6
      ? '夜深了'
      : hour < 11
        ? '早上好'
        : hour < 14
          ? '中午好'
          : hour < 18
            ? '下午好'
            : '晚上好'

  const activeBank = banks[0]
  const streak = overview?.streak_days ?? 0
  const dueCount = overview?.due_review_count ?? 0

  return (
    <MobilePullToRefresh onRefresh={loadData} className="p-4 space-y-4">
      {/* 1. Hero Greeting Banner */}
      <div className="relative overflow-hidden rounded-3xl border border-border/80 bg-gradient-to-br from-surface via-surface to-surface-secondary/80 p-4 shadow-card">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-full border border-primary/20 bg-primary/10 px-2.5 py-0.5 text-[10px] font-bold text-primary">
              <Sparkles className="h-3 w-3" />
              <span>AI 自适应</span>
            </span>
            {streak > 0 && (
              <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/25 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-600 dark:text-amber-400">
                <Flame className="h-3 w-3 fill-current animate-pulse-subtle" />
                <span>{streak}天打卡</span>
              </span>
            )}
          </div>
          <span className="text-[11px] font-mono text-muted-foreground">
            {new Date().toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric', weekday: 'short' })}
          </span>
        </div>

        <div className="mt-3">
          <h2 className="text-lg font-bold tracking-tight text-foreground">
            {greeting}，{user?.username || '学习者'}
          </h2>
          <p className="mt-0.5 text-xs text-muted-foreground leading-relaxed">
            {dueCount > 0 ? (
              <>
                今日有 <span className="font-bold text-amber-600 dark:text-amber-400 font-mono">{dueCount}</span> 道错题达到艾宾浩斯复习周期。
              </>
            ) : (
              '今日错题复习已清空，保持每日答题惯性！'
            )}
          </p>
        </div>

        {/* Hero 1-Tap Action */}
        <div className="mt-3.5 flex items-center gap-2">
          {activeBank ? (
            <Button
              size="sm"
              onClick={() => navigate(`/practice/setup?bank_id=${activeBank.id}`)}
              icon={<Play className="h-3.5 w-3.5 fill-current" />}
              className="flex-1 shadow-glow text-xs font-bold h-9"
            >
              继续刷题 ({activeBank.name.slice(0, 8)})
            </Button>
          ) : (
            <Button
              size="sm"
              onClick={() => navigate('/question-banks')}
              icon={<BookOpen className="h-3.5 w-3.5" />}
              className="flex-1 shadow-glow text-xs font-bold h-9"
            >
              探索题库
            </Button>
          )}

          {dueCount > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => navigate('/wrong-book')}
              icon={<RotateCcw className="h-3.5 w-3.5 text-amber-500" />}
              className="border-amber-500/40 text-amber-600 dark:text-amber-400 hover:bg-amber-500/10 text-xs font-bold h-9 shrink-0"
            >
              复习 ({dueCount})
            </Button>
          )}
        </div>
      </div>

      {/* 2. 2x2 Core Metrics Grid */}
      {loading ? (
        <div className="grid grid-cols-2 gap-2.5">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-20 rounded-2xl" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2.5">
          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>今日做题</span>
              <Target className="h-3.5 w-3.5 text-primary" />
            </div>
            <div className="mt-1.5 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">
                {overview?.today.question_count ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">题</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5 truncate">
              正确 {overview?.today.correct_count ?? 0} · 错 {overview?.today.wrong_count ?? 0}
            </div>
          </div>

          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>综合正确率</span>
              <TrendingUp className="h-3.5 w-3.5 text-emerald-500" />
            </div>
            <div className="mt-1.5 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">
                {overview?.accuracy ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">%</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5 truncate">
              累计作答 {overview?.total_question_count ?? 0} 题
            </div>
          </div>

          <div
            onClick={() => navigate('/wrong-book')}
            className={`rounded-2xl border bg-surface p-3 shadow-card active:scale-[0.98] transition-all cursor-pointer ${
              dueCount > 0 ? 'border-amber-500/40 bg-amber-500/[0.04]' : 'border-border/80'
            }`}
          >
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>待复习错题</span>
              <Clock className="h-3.5 w-3.5 text-amber-500" />
            </div>
            <div className="mt-1.5 flex items-baseline gap-1">
              <span className={`text-xl font-bold font-mono ${dueCount > 0 ? 'text-amber-500' : 'text-foreground'}`}>
                {dueCount}
              </span>
              <span className="text-[10px] text-muted-foreground">题</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5 truncate">艾宾浩斯记忆队列</div>
          </div>

          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>连续学习</span>
              <Flame className="h-3.5 w-3.5 text-rose-500" />
            </div>
            <div className="mt-1.5 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">{streak}</span>
              <span className="text-[10px] text-muted-foreground">天</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5 truncate">保持刷题好习惯</div>
          </div>
        </div>
      )}

      {/* 3. My Question Banks (Cards) */}
      <div className="space-y-2.5">
        <div className="flex items-center justify-between px-1">
          <div className="flex items-center gap-1.5">
            <Library className="h-4 w-4 text-primary" />
            <h3 className="text-xs font-bold text-foreground">我的题库</h3>
            <span className="rounded-full bg-surface-secondary px-1.5 py-0.2 text-[9px] font-mono font-bold text-muted-foreground">
              {banks.length}
            </span>
          </div>
          <Link to="/question-banks" className="text-xs font-bold text-primary inline-flex items-center gap-0.5">
            全部 <ArrowRight className="h-3 w-3" />
          </Link>
        </div>

        {banks.length === 0 && !loading ? (
          <div className="rounded-2xl border border-dashed border-border/80 bg-surface p-6 text-center space-y-2">
            <GraduationCap className="h-8 w-8 text-muted-foreground mx-auto opacity-50" />
            <p className="text-xs text-muted-foreground">暂无题库，可立即新建或导入</p>
            <Button size="xs" onClick={() => navigate('/question-banks')} icon={<Plus className="h-3 w-3" />}>
              新建题库
            </Button>
          </div>
        ) : (
          <div className="space-y-2">
            {banks.map((b) => (
              <div
                key={b.id}
                onClick={() => navigate(`/practice/setup?bank_id=${b.id}`)}
                className="flex items-center justify-between rounded-2xl border border-border/80 bg-surface p-3.5 shadow-card active:scale-[0.99] transition-all cursor-pointer"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                    <GraduationCap className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <h4 className="text-xs font-bold text-foreground truncate">{b.name}</h4>
                    <div className="flex items-center gap-2 text-[10px] text-muted-foreground mt-0.5 font-mono">
                      <span>{b.question_count} 题</span>
                      <span>·</span>
                      <span>{b.subject_count ?? 0} 科目</span>
                      <span>·</span>
                      <Badge tone={b.visibility === 'public' ? 'info' : 'neutral'} variant="subtle" className="text-[9px] py-0 px-1">
                        {b.visibility === 'public' ? '公开' : '私有'}
                      </Badge>
                    </div>
                  </div>
                </div>

                <Button
                  size="xs"
                  variant="subtle"
                  onClick={(e) => {
                    e.stopPropagation()
                    navigate(`/practice/setup?bank_id=${b.id}`)
                  }}
                  icon={<Play className="h-3 w-3 fill-current" />}
                  className="shrink-0 ml-2"
                >
                  刷题
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 4. AI Weak Knowledge Points Radar */}
      <div className="space-y-2.5">
        <div className="flex items-center justify-between px-1">
          <div className="flex items-center gap-1.5">
            <Zap className="h-4 w-4 text-amber-500" />
            <h3 className="text-xs font-bold text-foreground">薄弱考点预警</h3>
          </div>
          <Link to="/wrong-book" className="text-xs text-muted-foreground hover:text-foreground">
            错题本
          </Link>
        </div>

        <div className="rounded-2xl border border-border/80 bg-surface p-3.5 shadow-card space-y-2">
          {weakPoints.length === 0 ? (
            <div className="flex items-center gap-2.5 py-1 text-xs text-muted-foreground">
              <CheckCircle2 className="h-4 w-4 text-emerald-500 shrink-0" />
              <span>暂无高频错题考点，知识掌握稳固！</span>
            </div>
          ) : (
            weakPoints.slice(0, 3).map((item, i) => (
              <div
                key={i}
                className="flex items-center justify-between rounded-xl bg-surface-secondary/70 p-2 text-xs"
              >
                <span className="font-semibold text-foreground truncate max-w-[180px]">
                  {item.knowledge_point}
                </span>
                <span className="rounded-md bg-destructive/10 px-2 py-0.5 font-mono text-[10px] font-bold text-destructive shrink-0">
                  错 {item.wrong_count} 次
                </span>
              </div>
            ))
          )}
        </div>
      </div>

      {/* 5. Mobile Quick Tool Buttons */}
      <div className="space-y-2">
        <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block px-1">
          快捷功能
        </span>
        <div className="grid grid-cols-3 gap-2">
          <button
            onClick={() => navigate('/wrong-import')}
            className="flex flex-col items-center justify-center gap-1.5 rounded-2xl border border-border/80 bg-surface p-3 text-center active:scale-95 shadow-card transition-all"
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <ImagePlus className="h-4 w-4" />
            </div>
            <span className="text-[11px] font-bold text-foreground truncate w-full">拍照导入</span>
          </button>

          <button
            onClick={() => navigate('/admin/imports')}
            className="flex flex-col items-center justify-center gap-1.5 rounded-2xl border border-border/80 bg-surface p-3 text-center active:scale-95 shadow-card transition-all"
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-500">
              <UploadCloud className="h-4 w-4" />
            </div>
            <span className="text-[11px] font-bold text-foreground truncate w-full">PDF 导入</span>
          </button>

          <button
            onClick={() => navigate('/statistics')}
            className="flex flex-col items-center justify-center gap-1.5 rounded-2xl border border-border/80 bg-surface p-3 text-center active:scale-95 shadow-card transition-all"
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-500">
              <TrendingUp className="h-4 w-4" />
            </div>
            <span className="text-[11px] font-bold text-foreground truncate w-full">学习报告</span>
          </button>
        </div>
      </div>
    </MobilePullToRefresh>
  )
}
