import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { createPracticeSession, previewPractice } from '@/api/practice'
import { Badge, Button, Card, Skeleton } from '@/components/ui'
import {
  ArrowLeft,
  BookOpen,
  Check,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Flame,
  HelpCircle,
  Layers,
  ListChecks,
  Play,
  RotateCcw,
  Search,
  Shuffle,
  SlidersHorizontal,
  Sparkles,
  Star,
  Timer,
  X,
  Zap,
} from 'lucide-react'
import type {
  Page,
  PracticeMode,
  PracticePreview,
  PracticeSessionRequest,
  QuestionBank,
  QuestionType,
} from '@/types'

const PAGE_SIZE = 12

interface ModeOptionConfig {
  value: PracticeMode
  label: string
  desc: string
  icon: typeof BookOpen
}

const modeOptions: ModeOptionConfig[] = [
  { value: 'sequential', label: '顺序刷题', desc: '按题目原始序号推进', icon: BookOpen },
  { value: 'random', label: '随机乱序', desc: '全库随机打乱抽题', icon: Shuffle },
  { value: 'unattempted', label: '未做优先', desc: '优先抽取未作答题目', icon: ListChecks },
  { value: 'wrong', label: '错题突击', desc: '重点强化历史错题薄弱点', icon: RotateCcw },
  { value: 'favorite', label: '收藏专练', desc: '专项复习已标记重点题', icon: Star },
  { value: 'due', label: '遗忘复习', desc: '基于艾宾浩斯记忆曲线', icon: Sparkles },
]

const typeOptions: Array<{ value: QuestionType | ''; label: string }> = [
  { value: '', label: '全部题型（混合）' },
  { value: 'single_choice', label: '单选题' },
  { value: 'multiple_choice', label: '多选题' },
  { value: 'true_false', label: '判断题' },
  { value: 'fill_blank', label: '填空题' },
  { value: 'short_answer', label: '简答题' },
  { value: 'material_analysis', label: '材料分析题' },
  { value: 'essay', label: '论述题' },
  { value: 'code', label: '代码题' },
]

const timeOptions: Array<{ value: number | null; label: string; sub: string }> = [
  { value: null, label: '不限时', sub: '自由练习' },
  { value: 15 * 60, label: '15 分钟', sub: '快速冲刺' },
  { value: 30 * 60, label: '30 分钟', sub: '标准自测' },
  { value: 45 * 60, label: '45 分钟', sub: '深度检测' },
  { value: 60 * 60, label: '60 分钟', sub: '全真模拟' },
]

const maxCustomTimeMinutes = 24 * 60

const questionPresets = [10, 20, 30, 50, 100]

function parseIDs(value: string | null): number[] {
  return (value || '')
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((id, index, ids) => id > 0 && ids.indexOf(id) === index)
}

export default function PracticeSetup() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const initialBankIDs = useMemo(
    () => parseIDs(searchParams.get('bank_ids') || searchParams.get('bank_id')),
    [searchParams],
  )

  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [bankCatalog, setBankCatalog] = useState<Record<number, QuestionBank>>({})
  const [selectedBankIDs, setSelectedBankIDs] = useState<number[]>(initialBankIDs)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loadingBanks, setLoadingBanks] = useState(true)

  const [mode, setMode] = useState<PracticeMode>('random')
  const [type, setType] = useState<QuestionType | ''>('')
  const [limit, setLimit] = useState(20)
  const [timeLimitSeconds, setTimeLimitSeconds] = useState<number | null>(null)
  const [customTimeMinutes, setCustomTimeMinutes] = useState('90')
  const [customTimeActive, setCustomTimeActive] = useState(false)
  const [preview, setPreview] = useState<PracticePreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')

  const loadBanks = useCallback(async () => {
    setLoadingBanks(true)
    try {
      const response = await client.get('/question-banks', {
        params: { page, page_size: PAGE_SIZE, q: search.trim() || undefined },
      })
      const data = response.data.data as Page<QuestionBank>
      setBanks(data.items || [])
      setTotal(data.total || 0)
      setBankCatalog((previous) => ({
        ...previous,
        ...(data.items || []).reduce<Record<number, QuestionBank>>((catalog, bank) => {
          catalog[bank.id] = bank
          return catalog
        }, {}),
      }))
    } catch (cause) {
      setError(errMsg(cause))
    } finally {
      setLoadingBanks(false)
    }
  }, [page, search])

  useEffect(() => {
    const timer = window.setTimeout(loadBanks, search ? 180 : 0)
    return () => window.clearTimeout(timer)
  }, [loadBanks, search])

  useEffect(() => {
    if (initialBankIDs.length === 0) return
    let cancelled = false
    Promise.all(
      initialBankIDs.map(async (id) => {
        try {
          const response = await client.get(`/question-banks/${id}`)
          return response.data.data as QuestionBank
        } catch {
          return null
        }
      }),
    ).then((items) => {
      if (cancelled) return
      setBankCatalog((previous) => ({
        ...previous,
        ...items.reduce<Record<number, QuestionBank>>((catalog, bank) => {
          if (bank) catalog[bank.id] = bank
          return catalog
        }, {}),
      }))
    })
    return () => {
      cancelled = true
    }
  }, [initialBankIDs])

  const toggleBank = (id: number) => {
    setSelectedBankIDs((current) => {
      if (current.includes(id)) {
        return current.filter((item) => item !== id)
      }
      return [...current, id]
    })
  }

  const selectAllCurrentPage = () => {
    const pageIDs = banks.map((b) => b.id)
    setSelectedBankIDs((current) => {
      const merged = new Set([...current, ...pageIDs])
      return Array.from(merged)
    })
  }

  const clearAllSelected = () => {
    setSelectedBankIDs([])
  }

  const request = useMemo<PracticeSessionRequest | null>(() => {
    if (selectedBankIDs.length === 0) return null
    return {
      bank_ids: selectedBankIDs,
      mode,
      type: type || undefined,
      limit,
      time_limit_seconds: timeLimitSeconds ?? undefined,
    }
  }, [selectedBankIDs, mode, type, limit, timeLimitSeconds])

  useEffect(() => {
    if (!request) {
      setPreview(null)
      return
    }
    let cancelled = false
    setPreviewLoading(true)
    setError('')
    const timer = window.setTimeout(() => {
      previewPractice(request)
        .then((data) => {
          if (!cancelled) setPreview(data)
        })
        .catch((cause) => {
          if (!cancelled) {
            setPreview(null)
            setError(errMsg(cause))
          }
        })
        .finally(() => {
          if (!cancelled) setPreviewLoading(false)
        })
    }, 120)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [request])

  const start = useCallback(async () => {
    if (!request || !preview || preview.selected_count === 0 || starting || (customTimeActive && !customTimeMinutes.trim())) return
    setStarting(true)
    setError('')
    try {
      const session = await createPracticeSession(request)
      navigate(`/practice/${session.id}`)
    } catch (cause) {
      setError(errMsg(cause))
    } finally {
      setStarting(false)
    }
  }, [request, preview, starting, navigate, customTimeActive, customTimeMinutes])

  // 支持键盘回车快捷键启动（当不在输入框内聚焦时）
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        start()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [start])

  const totalPages = Math.ceil(total / PAGE_SIZE)
  const selectedBankItems = selectedBankIDs.map((id) => bankCatalog[id] || { id, name: `题库 #${id}`, question_count: 0 })
  const selectedModeConfig = modeOptions.find((item) => item.value === mode) || modeOptions[0]
  const selectedTypeLabel = typeOptions.find((item) => item.value === type)?.label || '全部题型'
  const isCustomTime = customTimeActive
  const selectedTimeLabel = isCustomTime
    ? `自定义 ${customTimeMinutes} 分钟`
    : timeOptions.find((item) => item.value === timeLimitSeconds)?.label || '不限时'

  const updateCustomTime = (value: string) => {
    setCustomTimeActive(true)
    setCustomTimeMinutes(value)
    const minutes = Number(value)
    if (Number.isFinite(minutes) && minutes >= 1) {
      setTimeLimitSeconds(Math.min(maxCustomTimeMinutes, Math.floor(minutes)) * 60)
    } else {
      setTimeLimitSeconds(null)
    }
  }

  const availableCount = preview?.available_count ?? 0
  const selectedCount = preview?.selected_count ?? 0

  return (
    <div className="mx-auto max-w-[1580px] h-full flex flex-col justify-between space-y-3 sm:space-y-3.5 pb-2 animate-fade-in select-none">
      {/* 顶部极简导航与状态 Header */}
      <div className="flex shrink-0 items-center justify-between gap-3 px-0.5">
        <div className="flex items-center gap-2.5 min-w-0">
          <button
            type="button"
            onClick={() => navigate('/question-banks')}
            className="group flex h-8.5 w-8.5 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-surface text-muted-foreground transition-all duration-150 hover:border-primary/40 hover:bg-surface-secondary hover:text-foreground active:scale-95 shadow-2xs cursor-pointer"
            title="返回题库空间"
          >
            <ArrowLeft className="h-4 w-4 transition-transform group-hover:-translate-x-0.5" />
          </button>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h1 className="text-lg font-bold tracking-tight text-foreground sm:text-xl truncate">自定义智能刷题</h1>
              <span className="hidden sm:inline-flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-bold text-primary border border-primary/20">
                <Zap className="h-3 w-3" /> Dashboard
              </span>
            </div>
            <p className="text-[11px] text-muted-foreground truncate hidden md:block">
              一站式配置题库范围、刷题模式与练习节奏，右侧实时联动
            </p>
          </div>
        </div>

        {/* 顶部快捷状态与重置 */}
        <div className="flex items-center gap-2">
          {selectedBankIDs.length > 0 && (
            <button
              type="button"
              onClick={clearAllSelected}
              className="inline-flex items-center gap-1 rounded-lg border border-border/70 bg-surface/80 px-2.5 py-1 text-xs text-muted-foreground transition-all hover:border-destructive/30 hover:bg-destructive/10 hover:text-destructive active:scale-95 cursor-pointer shadow-2xs"
            >
              <RotateCcw className="h-3 w-3" />
              <span>重置题库 ({selectedBankIDs.length})</span>
            </button>
          )}
        </div>
      </div>

      {/* 核心双栏 Dashboard: 左侧 73% (lg:col-span-8) + 右侧 27% (lg:col-span-4) */}
      <div className="grid grid-cols-1 gap-3.5 lg:grid-cols-12 flex-1 min-h-0 items-stretch">
        {/* ===================== 左侧主配置区 (8 Cols) ===================== */}
        <div className="lg:col-span-8 flex flex-col gap-3.5 min-w-0">
          {/* Section 1: 题库选择控制台 (Bank Selection Bar & Grid) */}
          <div className="rounded-2xl border border-border/80 bg-surface p-3.5 sm:p-4 shadow-subtle transition-all">
            {/* Top Toolbar: Title, Chips, Search & Select All */}
            <div className="flex flex-wrap items-center justify-between gap-2.5 pb-2.5 border-b border-border/50">
              <div className="flex items-center gap-2 min-w-0">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary font-bold">
                  <Layers className="h-3.5 w-3.5" />
                </div>
                <div className="flex items-center gap-2">
                  <h2 className="text-xs font-bold uppercase tracking-wider text-foreground">题库范围</h2>
                  <span className="rounded-full bg-surface-secondary px-2 py-0.5 text-[11px] font-semibold text-muted-foreground border border-border/60">
                    已选 <span className="font-mono font-bold text-primary">{selectedBankIDs.length}</span> 库
                  </span>
                </div>
              </div>

              {/* Search Box & Quick Select Buttons */}
              <div className="flex items-center gap-1.5 flex-1 max-w-sm justify-end">
                <div className="relative flex-1 min-w-[140px]">
                  <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground/60" />
                  <input
                    className="input h-8 pl-8 pr-6 text-xs bg-surface-secondary/40 focus:bg-surface border-border/70"
                    placeholder="搜索题库名称..."
                    value={search}
                    onChange={(event) => {
                      setSearch(event.target.value)
                      setPage(1)
                    }}
                  />
                  {search && (
                    <button
                      type="button"
                      onClick={() => setSearch('')}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground cursor-pointer"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                </div>
                {banks.length > 0 && (
                  <button
                    type="button"
                    onClick={selectAllCurrentPage}
                    className="shrink-0 rounded-lg border border-border/80 bg-surface px-2.5 py-1 text-[11px] font-medium text-foreground/80 transition-all hover:bg-surface-secondary hover:text-foreground hover:border-primary/40 active:scale-95 cursor-pointer shadow-2xs"
                  >
                    本页全选
                  </button>
                )}
              </div>
            </div>

            {/* Selected Banks Chips Bar (Inline, Compact, Height-constrained) */}
            {selectedBankItems.length > 0 && (
              <div className="mt-2 flex items-center gap-1.5 overflow-x-auto py-0.5 text-xs border-b border-border/40 pb-2">
                <span className="shrink-0 text-[10px] font-bold text-muted-foreground/80 uppercase">已选:</span>
                <div className="flex flex-wrap gap-1 max-h-14 overflow-y-auto">
                  {selectedBankItems.map((bank) => (
                    <span
                      key={bank.id}
                      className="inline-flex items-center gap-1 rounded-md border border-primary/25 bg-primary/10 px-1.5 py-0.5 text-[11px] font-semibold text-primary transition-all hover:bg-primary/15"
                    >
                      <span className="max-w-[130px] truncate">{bank.name}</span>
                      <button
                        type="button"
                        onClick={() => toggleBank(bank.id)}
                        className="rounded-full p-0.5 hover:bg-primary/25 text-primary/80 hover:text-primary transition-colors cursor-pointer"
                        title="取消选择"
                      >
                        <X className="h-2.5 w-2.5" />
                      </button>
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Question Bank Selectable Grid (Internal Slim Scroll, Fixed Height) */}
            <div className="mt-2.5">
              {loadingBanks ? (
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <div key={index} className="flex items-center gap-2.5 rounded-xl border border-border/50 p-2.5 bg-surface-secondary/20">
                      <Skeleton className="h-4 w-4 rounded-md" />
                      <div className="flex-1 space-y-1">
                        <Skeleton className="h-3.5 w-2/3" />
                        <Skeleton className="h-2.5 w-1/3" />
                      </div>
                    </div>
                  ))}
                </div>
              ) : banks.length === 0 ? (
                <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-5 text-center bg-surface-secondary/20">
                  <HelpCircle className="h-6 w-6 text-muted-foreground/40 mb-1" />
                  <p className="text-xs font-semibold text-foreground">未找到相关题库</p>
                  <p className="text-[10px] text-muted-foreground mt-0.5">请尝试其他关键词或前往题库空间管理</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-2 max-h-[148px] overflow-y-auto pr-1 sm:grid-cols-2">
                  {banks.map((bank) => {
                    const checked = selectedBankIDs.includes(bank.id)
                    return (
                      <div
                        key={bank.id}
                        role="button"
                        tabIndex={0}
                        onClick={() => toggleBank(bank.id)}
                        onKeyDown={(e) => {
                          if (e.key === ' ' || e.key === 'Enter') {
                            e.preventDefault()
                            toggleBank(bank.id)
                          }
                        }}
                        className={`group relative flex items-center gap-2.5 rounded-xl border p-2.5 text-left transition-all duration-150 cursor-pointer active:scale-[0.99] select-none ${
                          checked
                            ? 'border-primary/60 bg-primary/[0.08] shadow-2xs ring-1 ring-primary/30'
                            : 'border-border/70 bg-surface hover:border-primary/30 hover:bg-surface-secondary/60'
                        }`}
                      >
                        {/* Checkbox Visual */}
                        <div
                          className={`flex h-4.5 w-4.5 shrink-0 items-center justify-center rounded-md border transition-all ${
                            checked
                              ? 'border-primary bg-primary text-primary-foreground shadow-2xs'
                              : 'border-border/80 bg-surface text-transparent group-hover:border-primary/50'
                          }`}
                        >
                          <Check className="h-3 w-3 stroke-[3]" />
                        </div>

                        {/* Bank Info */}
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center justify-between gap-1">
                            <span className={`block truncate text-xs font-bold transition-colors ${checked ? 'text-primary' : 'text-foreground'}`}>
                              {bank.name}
                            </span>
                            <span className="shrink-0 font-mono text-[10px] font-bold rounded bg-surface-secondary px-1.5 py-0.2 text-muted-foreground border border-border/50">
                              {bank.question_count} 题
                            </span>
                          </div>
                          <div className="mt-0.5 flex items-center justify-between text-[10px] text-muted-foreground">
                            <span className="truncate max-w-[130px]">{bank.description || '全量题库'}</span>
                            <span className="shrink-0 font-medium">{bank.subject_count ?? 0} 科目</span>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            {/* Bottom Compact Pagination */}
            {totalPages > 1 && (
              <div className="mt-2 flex items-center justify-between border-t border-border/50 pt-2 text-[10px] text-muted-foreground">
                <span>
                  第 {page} / {totalPages} 页 (共 {total} 个题库)
                </span>
                <div className="flex gap-1">
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={page <= 1}
                    onClick={() => setPage((current) => current - 1)}
                    icon={<ChevronLeft className="h-2.5 w-2.5" />}
                    className="h-6 px-2 text-[10px]"
                  >
                    上页
                  </Button>
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={page >= totalPages}
                    onClick={() => setPage((current) => current + 1)}
                    icon={<ChevronRight className="h-2.5 w-2.5" />}
                    className="h-6 px-2 text-[10px]"
                  >
                    下页
                  </Button>
                </div>
              </div>
            )}
          </div>

          {/* Section 2: 统一刷题配置工作台 (Unified Config Grid) */}
          <div className="rounded-2xl border border-border/80 bg-surface p-3.5 sm:p-4 shadow-subtle transition-all space-y-3.5">
            {/* Header */}
            <div className="flex items-center justify-between pb-2.5 border-b border-border/50">
              <div className="flex items-center gap-2">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary font-bold">
                  <SlidersHorizontal className="h-3.5 w-3.5" />
                </div>
                <div>
                  <h2 className="text-xs font-bold uppercase tracking-wider text-foreground">练习策略与节奏</h2>
                </div>
              </div>
              <span className="text-[10px] text-muted-foreground hidden sm:inline-block">根据复习进度灵活定制</span>
            </div>

            {/* Row 1: 刷题模式 (3x2 Grid Segmented Cards with Rich Visual Hierarchy) */}
            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="text-[11px] font-bold text-foreground">刷题模式 (Practice Mode)</label>
                <span className="text-[10px] text-muted-foreground">当前：{selectedModeConfig.label}</span>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {modeOptions.map(({ value, label, desc, icon: Icon }) => {
                  const active = mode === value
                  return (
                    <div
                      key={value}
                      role="button"
                      tabIndex={0}
                      onClick={() => setMode(value)}
                      onKeyDown={(e) => {
                        if (e.key === ' ' || e.key === 'Enter') {
                          e.preventDefault()
                          setMode(value)
                        }
                      }}
                      className={`group relative flex items-start gap-2.5 rounded-xl border p-2.5 text-left transition-all duration-150 cursor-pointer active:scale-[0.98] select-none ${
                        active
                          ? 'border-primary bg-primary/[0.09] shadow-2xs ring-1 ring-primary/40'
                          : 'border-border/80 bg-surface hover:border-primary/30 hover:bg-surface-secondary/60'
                      }`}
                    >
                      <div
                        className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg transition-colors ${
                          active
                            ? 'bg-primary text-primary-foreground shadow-2xs'
                            : 'bg-surface-secondary text-muted-foreground group-hover:text-foreground'
                        }`}
                      >
                        <Icon className="h-3.5 w-3.5" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between">
                          <span className={`block text-xs font-bold transition-colors ${active ? 'text-primary' : 'text-foreground'}`}>
                            {label}
                          </span>
                          {active && (
                            <span className="h-1.5 w-1.5 rounded-full bg-primary ring-2 ring-primary/30" />
                          )}
                        </div>
                        <span className="mt-0.5 block truncate text-[10px] text-muted-foreground">
                          {desc}
                        </span>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>

            {/* Row 3: 题型 & 题目数量 (2 Columns Side-by-Side) */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {/* Question Type */}
              <div>
                <label className="text-[11px] font-bold text-foreground mb-1 block">题目类型 (Question Type)</label>
                <select
                  className="input h-9 text-xs bg-surface font-medium border-border/80 focus:border-primary cursor-pointer"
                  value={type}
                  onChange={(event) => setType(event.target.value as QuestionType | '')}
                >
                  {typeOptions.map((option) => (
                    <option key={option.value || 'all'} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-[10px] text-muted-foreground">默认混合抽取题库中的所有题型</p>
              </div>

              {/* Question Count & Presets */}
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-[11px] font-bold text-foreground">抽取数量 (Question Count)</label>
                  <span className="text-[10px] font-mono text-muted-foreground">上限 500 题</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <div className="relative w-20 shrink-0">
                    <input
                      className="input h-9 font-mono text-xs font-bold pr-6 text-center"
                      type="number"
                      min={1}
                      max={500}
                      value={limit}
                      onChange={(event) => setLimit(Math.min(500, Math.max(1, Number(event.target.value) || 1)))}
                    />
                    <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[11px] font-bold text-muted-foreground">
                      题
                    </span>
                  </div>
                  {/* Quick Presets */}
                  <div className="grid grid-cols-5 gap-1 flex-1">
                    {questionPresets.map((preset) => (
                      <button
                        key={preset}
                        type="button"
                        onClick={() => setLimit(preset)}
                        className={`h-9 rounded-lg border text-xs font-mono font-bold transition-all active:scale-95 cursor-pointer ${
                          limit === preset
                            ? 'border-primary bg-primary/15 text-primary ring-1 ring-primary/30'
                            : 'border-border/80 bg-surface text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary'
                        }`}
                      >
                        {preset}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            {/* Row 4: 练习时间限制 */}
            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="text-[11px] font-bold text-foreground">练习时间限制 (Time Limit)</label>
                <span className="text-[10px] text-muted-foreground">当前：{selectedTimeLabel}</span>
              </div>
              <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-6">
                {timeOptions.map((option) => {
                  const active = timeLimitSeconds === option.value
                  return (
                    <div
                      key={option.label}
                      role="button"
                      tabIndex={0}
                      onClick={() => {
                        setCustomTimeActive(false)
                        setTimeLimitSeconds(option.value)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === ' ' || e.key === 'Enter') {
                          e.preventDefault()
                          setCustomTimeActive(false)
                          setTimeLimitSeconds(option.value)
                        }
                      }}
                      className={`group flex flex-col items-center justify-center rounded-xl border py-2 px-1 text-center transition-all duration-150 cursor-pointer active:scale-[0.98] select-none ${
                        active
                          ? 'border-primary bg-primary/[0.09] font-bold text-primary ring-1 ring-primary/30 shadow-2xs'
                          : 'border-border/80 bg-surface text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary/70'
                      }`}
                    >
                      <div className="flex items-center gap-1 text-xs">
                        {option.value === null ? (
                          <Clock3 className={`h-3.5 w-3.5 ${active ? 'text-primary' : 'text-muted-foreground'}`} />
                        ) : (
                          <Timer className={`h-3.5 w-3.5 ${active ? 'text-primary' : 'text-muted-foreground'}`} />
                        )}
                        <span className={active ? 'text-primary font-bold' : 'text-foreground'}>{option.label}</span>
                      </div>
                      <span className="mt-0.5 text-[9px] text-muted-foreground/80">{option.sub}</span>
                    </div>
                  )
                })}
                <div
                  role="button"
                  tabIndex={0}
                  aria-pressed={isCustomTime}
                  onClick={() => updateCustomTime(customTimeMinutes)}
                  onKeyDown={(e) => {
                    if (e.key === ' ' || e.key === 'Enter') {
                      e.preventDefault()
                      updateCustomTime(customTimeMinutes)
                    }
                  }}
                  className={`group flex flex-col items-center justify-center rounded-xl border py-2 px-1 text-center transition-all duration-150 cursor-pointer active:scale-[0.98] select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 ${
                    isCustomTime
                      ? 'border-primary bg-primary/[0.09] font-bold text-primary ring-1 ring-primary/30 shadow-2xs'
                      : 'border-border/80 bg-surface text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary/70'
                  }`}
                >
                  <div className="flex items-center gap-1 text-xs">
                    <SlidersHorizontal className={`h-3.5 w-3.5 ${isCustomTime ? 'text-primary' : 'text-muted-foreground'}`} />
                    <span className={isCustomTime ? 'text-primary font-bold' : 'text-foreground'}>自定义</span>
                  </div>
                  <span className="mt-0.5 text-[9px] text-muted-foreground/80">按分钟设置</span>
                </div>
              </div>
              {isCustomTime && (
                <div className="mt-2 flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/[0.04] px-3 py-2">
                  <label htmlFor="custom-practice-time" className="shrink-0 text-[11px] font-semibold text-muted-foreground">自定义时长</label>
                  <div className="relative min-w-0 flex-1">
                    <input
                      id="custom-practice-time"
                      className="input h-8 w-full pr-12 text-center text-xs font-mono font-bold"
                      type="number"
                      min={1}
                      max={maxCustomTimeMinutes}
                      value={customTimeMinutes}
                      onChange={(event) => updateCustomTime(event.target.value)}
                      onClick={(event) => event.stopPropagation()}
                    />
                    <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground">分钟</span>
                  </div>
                  <span className="shrink-0 text-[10px] text-muted-foreground">最多 24 小时</span>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* ===================== 右侧实时练习概览与启动区 (4 Cols Full Height Sticky) ===================== */}
        <div className="lg:col-span-4 flex flex-col h-full min-w-0">
          <Card className="flex-1 flex flex-col justify-between overflow-hidden border-primary/30 shadow-elevated transition-all">
            {/* Header with Visual Accent */}
            <div className="shrink-0 relative overflow-hidden bg-gradient-to-br from-primary/15 via-primary/[0.05] to-surface p-4 border-b border-border/60">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="flex h-7.5 w-7.5 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-glow">
                    <Sparkles className="h-4 w-4" />
                  </div>
                  <div>
                    <h3 className="text-xs font-bold text-foreground tracking-tight">本次练习概览</h3>
                    <p className="text-[10px] text-muted-foreground">自适应练习 · 实时同步配置</p>
                  </div>
                </div>
                <Badge tone="primary" variant="subtle" className="font-mono text-[10px] font-bold px-2 py-0.5">
                  LIVE SYNC
                </Badge>
              </div>
            </div>

            {/* Middle Scrollable/Flexible Content */}
            <div className="flex-1 p-4 space-y-3 flex flex-col justify-start">
              {/* 核心双指标卡片 (可用题量 vs 本次计划抽取) */}
              <div className="grid grid-cols-2 gap-2.5">
                <div className="rounded-xl border border-border/80 bg-surface-secondary/30 p-2.5 transition-all">
                  <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
                    <span>题库可用</span>
                    <Layers className="h-3 w-3 opacity-60" />
                  </div>
                  <div className="mt-1 flex items-baseline gap-1">
                    {previewLoading ? (
                      <Skeleton className="h-7 w-12" />
                    ) : (
                      <span className="font-mono text-2xl font-black tracking-tight text-foreground">
                        {availableCount}
                      </span>
                    )}
                    <span className="text-[11px] text-muted-foreground font-semibold">题</span>
                  </div>
                </div>

                <div className="rounded-xl border border-primary/30 bg-primary/[0.08] p-2.5 transition-all shadow-2xs">
                  <div className="flex items-center justify-between text-[10px] font-bold text-primary uppercase">
                    <span>计划抽取</span>
                    <Flame className="h-3 w-3 text-primary" />
                  </div>
                  <div className="mt-1 flex items-baseline gap-1">
                    {previewLoading ? (
                      <Skeleton className="h-7 w-12" />
                    ) : (
                      <span className="font-mono text-2xl font-black tracking-tight text-primary">
                        {selectedCount}
                      </span>
                    )}
                    <span className="text-[11px] text-primary font-bold">题</span>
                  </div>
                </div>
              </div>

              {/* 已选题库 Badge 清单 */}
              <div className="rounded-xl border border-border/70 bg-surface-secondary/20 p-2.5 text-xs">
                <div className="flex items-center justify-between mb-1.5 text-[10px] font-bold text-muted-foreground uppercase">
                  <span>已锁定题库 ({selectedBankIDs.length})</span>
                  {selectedBankIDs.length > 0 && (
                    <span className="text-[10px] text-primary font-medium">
                      {selectedBankIDs.length === 1 ? '单库模式' : '多库聚合'}
                    </span>
                  )}
                </div>
                {selectedBankIDs.length === 0 ? (
                  <p className="text-xs text-muted-foreground/80 py-1">⚠️ 请在左侧勾选至少一个题库以开始练习</p>
                ) : (
                  <div className="flex flex-wrap gap-1 max-h-16 overflow-y-auto">
                    {selectedBankItems.map((b) => (
                      <Badge key={b.id} tone="primary" variant="subtle" className="text-[10px] py-0.5 max-w-[180px] truncate">
                        {b.name}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>

              {/* 配置清单 List */}
              <div className="space-y-2 rounded-xl border border-border/60 bg-surface/50 p-2.5 text-xs">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-[11px]">刷题模式</span>
                  <div className="flex items-center gap-1 font-bold text-foreground">
                    <selectedModeConfig.icon className="h-3 w-3 text-primary" />
                    <span>{selectedModeConfig.label}</span>
                  </div>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-[11px]">题目类型</span>
                  <span className="font-semibold text-foreground">{selectedTypeLabel}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-[11px]">时间安排</span>
                  <span className="font-semibold text-foreground">{selectedTimeLabel}</span>
                </div>
              </div>

              {/* AI 算法调度与练习策略指引 */}
              <div className="rounded-xl border border-primary/20 bg-primary/[0.03] p-2.5 text-xs space-y-1.5">
                <div className="flex items-center gap-1.5 font-bold text-primary text-[11px]">
                  <Sparkles className="h-3.5 w-3.5" />
                  <span>智能调度策略</span>
                </div>
                <p className="text-[11px] text-muted-foreground leading-relaxed">
                  {mode === 'wrong' && '优先抽取错题本中失分频次最高题目，针对薄弱点实施饱和强化。'}
                  {mode === 'unattempted' && '优先过滤已作答题目，调度全库新题快速扫盲开拓考点视野。'}
                  {mode === 'due' && '基于艾宾浩斯记忆模型，精准定位当前处于遗忘临界点的题目。'}
                  {mode === 'favorite' && '集中抽验星标重点题，加深高频考点与典型题型的解题手感。'}
                  {mode === 'sequential' && '按教材与考纲序号循序渐进，适合系统性摸底与地毯式扫题。'}
                  {mode === 'random' && '全库无规则打乱混合抽题，模拟真实考场的随机应变与综合把控。'}
                </p>
                <div className="flex items-center justify-between text-[10px] text-muted-foreground/90 pt-1 border-t border-primary/10">
                  <span>预估单题用时</span>
                  <span className="font-mono font-bold text-foreground">
                    {timeLimitSeconds && selectedCount > 0
                      ? `约 ${(timeLimitSeconds / 60 / selectedCount).toFixed(1)} 分钟 / 题`
                      : '自主掌控'}
                  </span>
                </div>
              </div>
            </div>

            {/* Bottom Action Footer (Pushed to bottom) */}
            <div className="shrink-0 p-4 pt-0 mt-auto space-y-2">
              {/* 异常或提示信息 */}
              {preview && limit > preview.available_count && preview.available_count > 0 && (
                <div className="rounded-xl border border-warning/30 bg-warning/10 p-2 text-[11px] leading-relaxed text-warning animate-slide-down">
                  ⚠️ 可用题目 ({preview.available_count} 题) 少于期望抽题量 ({limit} 题)，将按最大可用数生成。
                </div>
              )}
              {error && (
                <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-2 text-[11px] leading-relaxed text-destructive animate-slide-down">
                  {error}
                </div>
              )}

              {/* 开始刷题 Primary CTA */}
              <Button
                className="w-full h-11.5 text-sm font-bold shadow-glow hover:brightness-105 active:scale-[0.99] transition-all cursor-pointer"
                onClick={start}
                loading={starting}
                disabled={!request || !preview || preview.selected_count === 0 || previewLoading}
                icon={<Play className="h-4 w-4 fill-current" />}
              >
                {starting ? '正在生成练习会话...' : '立即开始刷题'}
              </Button>

              <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-0.5">
                <span className="flex items-center gap-1">
                  <Timer className="h-3 w-3" />
                  {timeLimitSeconds ? '倒计时自创建起计时' : '自由练习无时限'}
                </span>
                <span className="font-mono text-[10px] opacity-75">⌘ + Enter / Ctrl + Enter</span>
              </div>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}
