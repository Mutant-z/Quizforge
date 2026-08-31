import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import client from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Badge, Button, Card, EmptyState, Skeleton } from '@/components/ui'
import PracticeConfigModal, { type PracticeLaunchTarget } from '@/components/practice/PracticeConfigModal'
import type { QuestionBank, StatOverview } from '@/types'
import {
  ArrowRight,
  BarChart3,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  Clock,
  Compass,
  Flame,
  GraduationCap,
  ImagePlus,
  Layers,
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

export default function Dashboard() {
  const user = useAuthStore((s) => s.user)
  const navigate = useNavigate()
  const [overview, setOverview] = useState<StatOverview | null>(null)
  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [weakPoints, setWeakPoints] = useState<{ knowledge_point: string; wrong_count: number }[]>([])
  const [loading, setLoading] = useState(true)
  const [practiceTarget, setPracticeTarget] = useState<PracticeLaunchTarget | null>(null)

  useEffect(() => {
    const load = async () => {
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
    }
    load()
  }, [])

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
  const todayDone = overview?.today.question_count ?? 0
  const dailyTarget = 30
  const progressPercent = Math.min(100, Math.round((todayDone / dailyTarget) * 100))

  return (
    <div className="h-full flex flex-col justify-between gap-2.5 sm:gap-3 2xl:gap-4 min-h-0 w-full overflow-hidden">
      {/* ========================================================================= */}
      {/* 1. Dashboard Hero Section (全景指令中枢与状态概览 - 字体层级全面升级) */}
      {/* ========================================================================= */}
      <div className="shrink-0 relative overflow-hidden rounded-2xl 2xl:rounded-3xl border border-border/80 bg-gradient-to-r from-primary/[0.08] via-surface to-indigo-500/[0.04] p-3.5 sm:p-4 2xl:p-5 shadow-card transition-all">
        {/* 背景氛围微光光斑 */}
        <div className="absolute -right-10 -top-10 h-44 w-44 rounded-full bg-primary/15 blur-3xl pointer-events-none" />
        <div className="absolute left-1/3 -bottom-10 h-36 w-36 rounded-full bg-indigo-500/10 blur-3xl pointer-events-none" />

        <div className="relative z-10 flex flex-col lg:flex-row lg:items-center lg:justify-between gap-3 sm:gap-4">
          {/* 左侧：问候、引擎状态与核心目标进度 */}
          <div className="space-y-1.5 min-w-0 max-w-3xl">
            {/* 顶栏徽章组 */}
            <div className="flex items-center gap-2.5 flex-wrap">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-primary/25 bg-primary/10 px-3 py-0.5 text-xs font-semibold text-primary shadow-2xs">
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary/60 opacity-75" />
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-primary" />
                </span>
                <Sparkles className="h-3.5 w-3.5 text-primary" />
                <span>AI 自适应强化学习引擎</span>
              </span>

              {streak > 0 ? (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/25 bg-amber-500/10 px-3 py-0.5 text-xs font-semibold text-amber-600 dark:text-amber-400">
                  <Flame className="h-3.5 w-3.5 fill-current animate-pulse-subtle" />
                  <span>连续学习 {streak} 天</span>
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-border/70 bg-surface-secondary/80 px-3 py-0.5 text-xs font-medium text-muted-foreground">
                  <Target className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>每日目标 {dailyTarget} 题</span>
                </span>
              )}
            </div>

            {/* 大字号问候标题 */}
            <div className="space-y-0.5">
              <h1 className="text-lg sm:text-xl 2xl:text-2xl font-black tracking-tight text-foreground flex items-center gap-2.5 flex-wrap">
                <span>{greeting}，{user?.username || '学习者'}</span>
                <span className="text-xs sm:text-sm font-medium text-muted-foreground">
                  {todayDone >= dailyTarget
                    ? '🎉 今日目标已超额达成，继续保持巅峰状态！'
                    : `今日已完成 ${todayDone}/${dailyTarget} 题，完成度 ${progressPercent}%`}
                </span>
              </h1>

              {/* 智能情境状态提示 */}
              <p className="text-xs sm:text-sm text-foreground/80 leading-relaxed line-clamp-1">
                {dueCount > 0 ? (
                  <>
                    今日有 <span className="font-bold text-amber-600 dark:text-amber-400 font-mono text-sm sm:text-base">{dueCount}</span> 道艾宾浩斯错题达到复习阈值，算法建议优先巩固高频盲区，避免知识点遗忘衰减。
                  </>
                ) : (
                  '今日暂无到期遗忘错题，所有知识点掌握稳固。建议开启新题库训练或进行专项攻坚。'
                )}
              </p>
            </div>

            {/* 今日刷题进度条 */}
            <div className="pt-0.5 max-w-md">
              <div className="flex items-center justify-between text-xs font-medium mb-1">
                <span className="text-muted-foreground">今日刷题进度</span>
                <span className="font-mono font-bold text-primary">{progressPercent}%</span>
              </div>
              <div className="h-2 w-full overflow-hidden rounded-full bg-muted/80 p-0.5 border border-border/40">
                <div
                  className="h-full rounded-full bg-gradient-to-r from-primary to-indigo-500 transition-all duration-500 ease-out shadow-glow"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
            </div>
          </div>

          {/* 右侧：双核心行动 CTA 组 */}
          <div className="flex items-center gap-2.5 shrink-0 flex-wrap sm:flex-nowrap pt-1 lg:pt-0">
            {activeBank ? (
              <Button
                size="md"
                onClick={() => setPracticeTarget({ bankIds: [activeBank.id], title: activeBank.name })}
                icon={<Play className="h-4 w-4 fill-current" />}
                className="shadow-glow font-bold text-xs sm:text-sm px-4.5 2xl:px-5 h-10 2xl:h-11 hover:scale-[1.02] active:scale-[0.98] transition-all"
              >
                继续刷题 ({activeBank.name})
              </Button>
            ) : (
              <Button
                size="md"
                onClick={() => navigate('/question-banks')}
                icon={<BookOpen className="h-4 w-4" />}
                className="shadow-glow font-bold text-xs sm:text-sm px-4.5 2xl:px-5 h-10 2xl:h-11 hover:scale-[1.02] active:scale-[0.98] transition-all"
              >
                探索题库
              </Button>
            )}

            {dueCount > 0 ? (
              <Button
                variant="outline"
                size="md"
                onClick={() => navigate('/wrong-book')}
                icon={<RotateCcw className="h-4 w-4" />}
                className="border-amber-500/40 text-amber-600 dark:text-amber-400 hover:bg-amber-500/10 font-bold text-xs sm:text-sm px-4 2xl:px-4.5 h-10 2xl:h-11 hover:scale-[1.02] active:scale-[0.98] transition-all shadow-glow-amber"
              >
                复习错题 ({dueCount})
              </Button>
            ) : (
              <Button
                variant="outline"
                size="md"
                onClick={() => navigate('/wrong-book')}
                icon={<RotateCcw className="h-4 w-4" />}
                className="text-xs sm:text-sm font-semibold px-4 2xl:px-4.5 h-10 2xl:h-11 hover:scale-[1.02] active:scale-[0.98] transition-all"
              >
                错题本
              </Button>
            )}
          </div>
        </div>
      </div>

      {/* ========================================================================= */}
      {/* 2. Unified Statistics Hub (一体化核心数据看板) */}
      {/* ========================================================================= */}
      {loading ? (
        <div className="shrink-0 rounded-2xl 2xl:rounded-3xl border border-border/80 bg-surface p-3 shadow-card">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-20 rounded-xl" />
            ))}
          </div>
        </div>
      ) : (
        <div className="shrink-0 rounded-2xl 2xl:rounded-3xl border border-border/80 bg-surface shadow-card overflow-hidden divide-y sm:divide-y-0 sm:divide-x divide-border/60 grid grid-cols-2 lg:grid-cols-4">
          {/* 指标 1：今日做题 */}
          <div className="p-3.5 2xl:p-4 flex flex-col justify-between transition-all hover:bg-surface-secondary/40 group">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">今日做题</span>
              <div className="flex h-7 w-7 2xl:h-8 2xl:w-8 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20 transition-transform group-hover:scale-105 shadow-2xs">
                <Target className="h-4 w-4" />
              </div>
            </div>
            <div className="mt-1 2xl:mt-1.5 flex items-baseline gap-1.5">
              <span className="text-2xl sm:text-3xl font-black tracking-tight text-foreground font-mono">
                {overview?.today.question_count ?? 0}
              </span>
              <span className="text-xs text-muted-foreground font-semibold">题</span>
            </div>
            <div className="mt-1.5 2xl:mt-2 flex items-center justify-between text-xs text-muted-foreground border-t border-border/40 pt-1.5 font-medium">
              <span>正确 {overview?.today.correct_count ?? 0} · 错误 {overview?.today.wrong_count ?? 0}</span>
              <span className="font-mono text-primary font-bold">{progressPercent}%</span>
            </div>
          </div>

          {/* 指标 2：综合正确率 */}
          <div className="p-3.5 2xl:p-4 flex flex-col justify-between transition-all hover:bg-surface-secondary/40 group">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">综合正确率</span>
              <div className="flex h-7 w-7 2xl:h-8 2xl:w-8 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 transition-transform group-hover:scale-105 shadow-2xs">
                <TrendingUp className="h-4 w-4" />
              </div>
            </div>
            <div className="mt-1 2xl:mt-1.5 flex items-baseline gap-1.5">
              <span className="text-2xl sm:text-3xl font-black tracking-tight text-emerald-600 dark:text-emerald-400 font-mono">
                {overview?.accuracy ?? 0}
              </span>
              <span className="text-xs text-muted-foreground font-semibold">%</span>
            </div>
            <div className="mt-1.5 2xl:mt-2 flex items-center justify-between text-xs text-muted-foreground border-t border-border/40 pt-1.5 font-medium">
              <span>累计作答 {overview?.total_question_count ?? 0} 题</span>
              <span className="text-emerald-600 dark:text-emerald-400 font-bold">稳步提升</span>
            </div>
          </div>

          {/* 指标 3：待复习错题 (可直接点击) */}
          <div
            onClick={() => navigate('/wrong-book')}
            className={`p-3.5 2xl:p-4 flex flex-col justify-between transition-all cursor-pointer group ${
              dueCount > 0
                ? 'hover:bg-amber-500/[0.04] bg-amber-500/[0.015]'
                : 'hover:bg-surface-secondary/40'
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">待复习错题</span>
              <div className="flex h-7 w-7 2xl:h-8 2xl:w-8 items-center justify-center rounded-xl bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 transition-transform group-hover:scale-105 shadow-2xs">
                <Clock className="h-4 w-4" />
              </div>
            </div>
            <div className="mt-1 2xl:mt-1.5 flex items-baseline gap-1.5">
              <span className={`text-2xl sm:text-3xl font-black tracking-tight font-mono ${dueCount > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-foreground'}`}>
                {overview?.due_review_count ?? 0}
              </span>
              <span className="text-xs text-muted-foreground font-semibold">题</span>
            </div>
            <div className="mt-1.5 2xl:mt-2 flex items-center justify-between text-xs text-muted-foreground border-t border-border/40 pt-1.5 font-medium">
              <span>艾宾浩斯复习队列</span>
              <span className="font-bold text-amber-600 dark:text-amber-400 group-hover:translate-x-0.5 transition-transform inline-flex items-center gap-1">
                立即复习 <ChevronRight className="h-3.5 w-3.5" />
              </span>
            </div>
          </div>

          {/* 指标 4：连续学习 */}
          <div className="p-3.5 2xl:p-4 flex flex-col justify-between transition-all hover:bg-surface-secondary/40 group">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">连续学习</span>
              <div className="flex h-7 w-7 2xl:h-8 2xl:w-8 items-center justify-center rounded-xl bg-rose-500/10 text-rose-500 border border-rose-500/20 transition-transform group-hover:scale-105 shadow-2xs">
                <Flame className="h-4 w-4" />
              </div>
            </div>
            <div className="mt-1 2xl:mt-1.5 flex items-baseline gap-1.5">
              <span className="text-2xl sm:text-3xl font-black tracking-tight text-foreground font-mono">
                {overview?.streak_days ?? 0}
              </span>
              <span className="text-xs text-muted-foreground font-semibold">天</span>
            </div>
            <div className="mt-1.5 2xl:mt-2 flex items-center justify-between text-xs text-muted-foreground border-t border-border/40 pt-1.5 font-medium">
              <span>保持每日学习习惯</span>
              <span className="text-rose-500 font-bold">🔥 冲刺榜首</span>
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* 3. Main Workspace Grid (8:4 比例，充满剩余高度，无溢出滚动) */}
      {/* ========================================================================= */}
      <div className="flex-1 min-h-0 grid gap-2.5 sm:gap-3 2xl:gap-4 lg:grid-cols-12 items-stretch">
        {/* ----------------------------------------------------------------------- */}
        {/* 左侧 8 列：我的题库空间 (字号与层级全面强化) */}
        {/* ----------------------------------------------------------------------- */}
        <div className="lg:col-span-8 flex flex-col h-full min-h-0 gap-1.5 2xl:gap-2">
          <div className="shrink-0 flex items-center justify-between px-1">
            <div className="flex items-center gap-2.5">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Library className="h-4 w-4" />
              </div>
              <h2 className="text-sm sm:text-base font-bold tracking-tight text-foreground">我的题库空间</h2>
              <span className="rounded-full bg-surface-secondary px-2.5 py-0.5 text-xs font-mono font-bold text-muted-foreground border border-border/60">
                {banks.length} 个题库
              </span>
            </div>
            <div className="flex items-center gap-3.5">
              {user?.role === 'admin' && (
                <Link
                  to="/admin/imports"
                  className="text-xs sm:text-sm font-semibold text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 transition-colors"
                >
                  <UploadCloud className="h-4 w-4 text-primary" /> PDF 导入
                </Link>
              )}
              <Link
                to="/question-banks"
                className="text-xs sm:text-sm font-bold text-primary hover:underline inline-flex items-center gap-1"
              >
                全部题库 <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            </div>
          </div>

          {banks.length === 0 && !loading ? (
            <Card className="flex-1 min-h-0 p-6 border-dashed border-2 rounded-2xl 2xl:rounded-3xl flex items-center justify-center">
              <EmptyState
                icon={Library}
                title="尚未创建或导入题库"
                description="你可以从 PDF、Markdown 智能导入海量试题，或者手动创建新题库开启自适应练习。"
                action={
                  <div className="flex items-center gap-3 pt-2">
                    <Button size="sm" onClick={() => navigate('/admin/imports')} icon={<UploadCloud className="h-4 w-4" />}>
                      导入 PDF 题库
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => navigate('/question-banks')} icon={<Plus className="h-4 w-4" />}>
                      新建题库
                    </Button>
                  </div>
                }
              />
            </Card>
          ) : (
            <div className="flex-1 min-h-0 grid gap-2.5 2xl:gap-3.5 sm:grid-cols-2">
              {banks.slice(0, 4).map((b) => (
                <div
                  key={b.id}
                  onClick={() => navigate(`/question-bank/${b.id}`)}
                  className="group relative flex flex-col justify-between rounded-2xl 2xl:rounded-3xl border border-border/80 bg-surface p-4 2xl:p-5 shadow-card transition-all duration-200 hover:border-primary/50 hover:shadow-card-hover hover:-translate-y-0.5 cursor-pointer h-full min-h-0"
                >
                  <div className="space-y-1.5 min-w-0">
                    {/* 卡片顶部：图标、名称与公开/私有徽章 */}
                    <div className="flex items-start justify-between gap-2.5">
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="flex h-9 w-9 2xl:h-10 2xl:w-10 shrink-0 items-center justify-center rounded-xl 2xl:rounded-2xl bg-primary/10 text-primary transition-transform group-hover:scale-105 shadow-subtle border border-primary/20">
                          <GraduationCap className="h-4.5 w-4.5 2xl:h-5 2xl:w-5" />
                        </div>
                        <div className="min-w-0">
                          <h3 className="text-sm sm:text-base font-bold text-foreground group-hover:text-primary transition-colors truncate">
                            {b.name}
                          </h3>
                          <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono mt-0.5">
                            <span className="font-semibold">{b.question_count} 题</span>
                            <span>·</span>
                            <span>{b.subject_count ?? 0} 科目</span>
                          </div>
                        </div>
                      </div>
                      <Badge tone={b.visibility === 'public' ? 'info' : 'neutral'} variant="subtle" className="text-xs shrink-0 py-0.5 px-2">
                        {b.visibility === 'public' ? '公开' : '私有'}
                      </Badge>
                    </div>

                    {/* 题库描述 */}
                    <p className="line-clamp-2 text-xs sm:text-sm text-muted-foreground leading-relaxed pt-0.5">
                      {b.description || '暂无详细描述，点击进入题库查看章节与知识点。'}
                    </p>
                  </div>

                  {/* 卡片底部操作栏 */}
                  <div className="mt-2.5 2xl:mt-3 flex items-center justify-between border-t border-border/50 pt-2 2xl:pt-2.5 text-xs">
                    <span className="text-xs font-semibold text-muted-foreground group-hover:text-foreground transition-colors inline-flex items-center gap-1">
                      查看详情 <ChevronRight className="h-3.5 w-3.5 group-hover:translate-x-0.5 transition-transform" />
                    </span>

                    <Button
                      size="sm"
                      variant="subtle"
                      onClick={(e) => {
                        e.stopPropagation()
                        setPracticeTarget({ bankIds: [b.id], title: b.name })
                      }}
                      icon={<Play className="h-3.5 w-3.5 fill-current" />}
                      className="px-3 2xl:px-3.5 py-1 font-bold text-xs h-7.5 2xl:h-8"
                    >
                      快速刷题
                    </Button>
                  </div>
                </div>
              ))}

              {/* 创建 / 导入题库 卡片 */}
              {banks.length > 0 && banks.length < 4 && (
                <div
                  onClick={() => navigate('/question-banks')}
                  className="group flex flex-col items-center justify-center rounded-2xl 2xl:rounded-3xl border-2 border-dashed border-border/80 bg-surface/40 p-4 2xl:p-5 hover:border-primary/50 hover:bg-primary/[0.02] transition-all cursor-pointer h-full min-h-0 text-center"
                >
                  <div className="flex h-9 w-9 2xl:h-10 2xl:w-10 items-center justify-center rounded-xl 2xl:rounded-2xl bg-surface-secondary text-muted-foreground group-hover:bg-primary/10 group-hover:text-primary group-hover:scale-110 transition-all mb-1.5 shadow-2xs">
                    <Plus className="h-4.5 w-4.5 2xl:h-5 2xl:w-5" />
                  </div>
                  <span className="text-sm sm:text-base font-bold text-muted-foreground group-hover:text-foreground transition-colors">
                    创建 / 导入题库
                  </span>
                  <span className="text-xs text-muted-foreground/80 mt-0.5 max-w-xs line-clamp-1">
                    支持从 PDF 智能解析或手动创建新题库
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* ----------------------------------------------------------------------- */}
        {/* 右侧 4 列：AI Learning Assistant Panel (字号全面升级) */}
        {/* ----------------------------------------------------------------------- */}
        <div className="lg:col-span-4 h-full min-h-0">
          <div className="h-full min-h-0 rounded-2xl 2xl:rounded-3xl border border-border/80 bg-surface p-4 2xl:p-5 shadow-card flex flex-col justify-between gap-2.5 2xl:gap-3.5 overflow-hidden">
            {/* 中枢 Header */}
            <div className="shrink-0 flex items-center justify-between border-b border-border/60 pb-2.5">
              <div className="flex items-center gap-2.5">
                <div className="flex h-7 w-7 2xl:h-8 2xl:w-8 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-subtle border border-primary/20">
                  <Sparkles className="h-4 w-4 animate-pulse-subtle" />
                </div>
                <div>
                  <h3 className="text-sm sm:text-base font-bold tracking-tight text-foreground leading-none">AI 助学中枢</h3>
                  <span className="text-xs text-muted-foreground mt-0.5 block">智能学情与学习加速</span>
                </div>
              </div>
              <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                <span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
                <span>诊断就绪</span>
              </span>
            </div>

            {/* 1. AI 智能学习诊断建议 */}
            <div className="rounded-xl 2xl:rounded-2xl border border-primary/20 bg-gradient-to-br from-primary/[0.08] via-primary/[0.02] to-surface p-3 2xl:p-3.5 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs sm:text-sm font-bold text-foreground flex items-center gap-1.5">
                  <Sparkles className="h-3.5 w-3.5 text-primary" />
                  智能学情建议
                </span>
                <span className="text-xs font-bold text-primary bg-primary/10 px-2 py-0.5 rounded-md">
                  实时分析
                </span>
              </div>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed line-clamp-2">
                {weakPoints.length > 0
                  ? `检测到近期在「${weakPoints[0].knowledge_point}」错频较高。建议优先开启针对性巩固或使用 AI 解析。`
                  : '近期答题正确率保持优异，建议保持每日练习节奏，定期清理遗忘曲线到期错题。'}
              </p>
              <div className="pt-0.5">
                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => navigate('/wrong-book')}
                  className="w-full text-xs font-bold h-7.5 2xl:h-8 border-primary/30 text-primary hover:bg-primary/10"
                >
                  前往错题复习本
                </Button>
              </div>
            </div>

            {/* 2. 薄弱考点雷达 */}
            <div className="space-y-1.5 min-h-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-1.5">
                  <Zap className="h-4 w-4 text-amber-500" />
                  <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground">薄弱考点雷达</h4>
                </div>
                <Link
                  to="/wrong-book"
                  className="text-xs font-bold text-muted-foreground hover:text-foreground transition-colors"
                >
                  全部错题 →
                </Link>
              </div>

              {weakPoints.length === 0 ? (
                <div className="flex items-center gap-2.5 py-2 px-3 rounded-xl bg-success/5 border border-success/15">
                  <div className="flex h-6 w-6 items-center justify-center rounded-lg bg-success/10 text-success shrink-0">
                    <CheckCircle2 className="h-4 w-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <span className="text-xs sm:text-sm font-bold text-foreground block leading-tight">考点掌握扎实</span>
                    <span className="text-xs text-muted-foreground block truncate">暂未检测到高频错题考点，继续保持！</span>
                  </div>
                </div>
              ) : (
                <div className="space-y-1.5">
                  {weakPoints.slice(0, 2).map((item, i) => (
                    <div
                      key={i}
                      onClick={() => navigate('/wrong-book')}
                      className="flex items-center justify-between rounded-lg border border-border/70 bg-surface-secondary/50 px-3 py-1.5 text-xs sm:text-sm transition-all hover:bg-surface-secondary hover:border-primary/30 cursor-pointer"
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="flex h-4 w-4 items-center justify-center rounded-full bg-muted text-xs font-mono font-bold text-muted-foreground">
                          {i + 1}
                        </span>
                        <span className="font-semibold text-foreground truncate max-w-[140px]">
                          {item.knowledge_point}
                        </span>
                      </div>
                      <Badge tone="danger" variant="subtle" className="font-mono text-xs font-bold py-0.5 px-2">
                        错 {item.wrong_count} 次
                      </Badge>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* 3. 快捷学习工具箱 (4 宫格) */}
            <div className="shrink-0 space-y-1.5 pt-1 border-t border-border/60">
              <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground block px-0.5">
                快捷学习工具
              </span>
              <div className="grid grid-cols-2 gap-2">
                <button
                  onClick={() => navigate('/question-banks')}
                  className="flex items-center gap-2.5 rounded-xl border border-border/70 bg-surface-secondary/50 p-2.5 text-left text-xs sm:text-sm font-semibold text-foreground hover:bg-surface-secondary hover:border-primary/40 transition-all active:scale-[0.98] shadow-2xs cursor-pointer group"
                >
                  <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary/10 text-primary group-hover:scale-105 transition-transform">
                    <Compass className="h-4 w-4" />
                  </div>
                  <span className="truncate">探索题库</span>
                </button>
                <button
                  onClick={() => navigate('/wrong-book')}
                  className="flex items-center gap-2.5 rounded-xl border border-border/70 bg-surface-secondary/50 p-2.5 text-left text-xs sm:text-sm font-semibold text-foreground hover:bg-surface-secondary hover:border-amber-500/40 transition-all active:scale-[0.98] shadow-2xs cursor-pointer group"
                >
                  <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-500/10 text-amber-500 group-hover:scale-105 transition-transform">
                    <RotateCcw className="h-4 w-4" />
                  </div>
                  <span className="truncate">错题复习</span>
                </button>
                <button
                  onClick={() => navigate('/wrong-import')}
                  className="flex items-center gap-2.5 rounded-xl border border-border/70 bg-surface-secondary/50 p-2.5 text-left text-xs sm:text-sm font-semibold text-foreground hover:bg-surface-secondary hover:border-sky-500/40 transition-all active:scale-[0.98] shadow-2xs cursor-pointer group"
                >
                  <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-sky-500/10 text-sky-500 group-hover:scale-105 transition-transform">
                    <ImagePlus className="h-4 w-4" />
                  </div>
                  <span className="truncate">错题导入</span>
                </button>
                <button
                  onClick={() => navigate('/statistics')}
                  className="flex items-center gap-2.5 rounded-xl border border-border/70 bg-surface-secondary/50 p-2.5 text-left text-xs sm:text-sm font-semibold text-foreground hover:bg-surface-secondary hover:border-emerald-500/40 transition-all active:scale-[0.98] shadow-2xs cursor-pointer group"
                >
                  <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-500 group-hover:scale-105 transition-transform">
                    <BarChart3 className="h-4 w-4" />
                  </div>
                  <span className="truncate">数据分析</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 刷题配置弹窗 */}
      <PracticeConfigModal
        open={!!practiceTarget}
        target={practiceTarget}
        onClose={() => setPracticeTarget(null)}
      />
    </div>
  )
}
