import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { createPracticeSession, previewPractice } from '@/api/practice'
import { Button, Modal } from '@/components/ui'
import {
  BookOpen,
  Check,
  ChevronDown,
  Clock3,
  Layers,
  ListChecks,
  Play,
  RotateCcw,
  Search,
  Shuffle,
  Sparkles,
  Star,
  Timer,
  X,
  Zap,
} from 'lucide-react'
import type { PracticeMode, PracticePreview, PracticeSessionRequest, QuestionBank, QuestionType } from '@/types'

export interface PracticeLaunchTarget {
  bankIds: number[]
  title: string
  subjectId?: number
  chapterId?: number
}

interface PracticeConfigModalProps {
  open: boolean
  target: PracticeLaunchTarget | null
  onClose: () => void
}

const modeOptions: Array<{ value: PracticeMode; label: string; icon: typeof BookOpen }> = [
  { value: 'sequential', label: '顺序刷题', icon: BookOpen },
  { value: 'random', label: '随机乱序', icon: Shuffle },
  { value: 'unattempted', label: '未做题', icon: ListChecks },
  { value: 'wrong', label: '错题优先', icon: RotateCcw },
  { value: 'favorite', label: '收藏专练', icon: Star },
  { value: 'due', label: '待复习', icon: Sparkles },
]

const typeOptions: Array<{ value: QuestionType | ''; label: string }> = [
  { value: '', label: '全部题型' },
  { value: 'single_choice', label: '单选题' },
  { value: 'multiple_choice', label: '多选题' },
  { value: 'true_false', label: '判断题' },
  { value: 'fill_blank', label: '填空题' },
  { value: 'short_answer', label: '简答题' },
  { value: 'material_analysis', label: '材料分析题' },
  { value: 'essay', label: '论述题' },
  { value: 'code', label: '代码题' },
]

const timeOptions: Array<{ value: number | null; label: string }> = [
  { value: null, label: '不限时' },
  { value: 15 * 60, label: '15 分钟' },
  { value: 30 * 60, label: '30 分钟' },
  { value: 45 * 60, label: '45 分钟' },
  { value: 60 * 60, label: '60 分钟' },
]

const maxCustomTimeMinutes = 24 * 60

const questionPresets = [10, 20, 50, 100]

export default function PracticeConfigModal({ open, target, onClose }: PracticeConfigModalProps) {
  const navigate = useNavigate()
  const [mode, setMode] = useState<PracticeMode>('sequential')
  const [type, setType] = useState<QuestionType | ''>('')
  const [limit, setLimit] = useState(20)
  const [timeLimitSeconds, setTimeLimitSeconds] = useState<number | null>(null)
  const [customTimeMinutes, setCustomTimeMinutes] = useState('90')
  const [customTimeActive, setCustomTimeActive] = useState(false)
  const [selectedBankIDs, setSelectedBankIDs] = useState<number[]>([])
  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [bankSearch, setBankSearch] = useState('')
  const [bankPickerOpen, setBankPickerOpen] = useState(false)
  const [banksLoading, setBanksLoading] = useState(false)
  const [preview, setPreview] = useState<PracticePreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')

  const targetKey = useMemo(
    () => (target ? `${target.bankIds.join(',')}:${target.subjectId ?? ''}:${target.chapterId ?? ''}` : ''),
    [target],
  )
  const targetBankIDs = useMemo(
    () => targetKey.split(':')[0].split(',').map(Number).filter((id) => id > 0),
    [targetKey],
  )

  useEffect(() => {
    if (!open || !targetKey) return
    setMode('sequential')
    setType('')
    setLimit(20)
    setTimeLimitSeconds(null)
    setCustomTimeMinutes('90')
    setCustomTimeActive(false)
    setSelectedBankIDs(targetBankIDs)
    setBankSearch('')
    setBankPickerOpen(false)
    setPreview(null)
    setError('')
  }, [open, targetKey, targetBankIDs])

  useEffect(() => {
    if (!open || !bankPickerOpen) return
    let cancelled = false
    setBanksLoading(true)
    client
      .get('/question-banks', { params: { page: 1, page_size: 100 } })
      .then((response) => {
        if (!cancelled) setBanks((response.data.data.items || []) as QuestionBank[])
      })
      .catch((cause) => {
        if (!cancelled) setError(errMsg(cause))
      })
      .finally(() => {
        if (!cancelled) setBanksLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, bankPickerOpen])

  const selectedBankNames = useMemo(() => {
    const names = selectedBankIDs.map((id) => banks.find((bank) => bank.id === id)?.name)
    return names.map((name, index) => name || (targetBankIDs[index] === selectedBankIDs[index] ? target?.title : `题库 #${selectedBankIDs[index]}`))
  }, [banks, selectedBankIDs, target, targetBankIDs])

  const visibleBanks = useMemo(() => {
    const keyword = bankSearch.trim().toLowerCase()
    if (!keyword) return banks
    return banks.filter((bank) => `${bank.name} ${bank.description || ''}`.toLowerCase().includes(keyword))
  }, [banks, bankSearch])

  const isOriginalSingleBank = selectedBankIDs.length === 1 && targetBankIDs.length === 1 && selectedBankIDs[0] === targetBankIDs[0]

  const request = useMemo<PracticeSessionRequest | null>(() => {
    if (!target || selectedBankIDs.length === 0) return null
    return {
      bank_ids: selectedBankIDs,
      ...(isOriginalSingleBank && target.subjectId ? { subject_id: target.subjectId } : {}),
      ...(isOriginalSingleBank && target.chapterId ? { chapter_id: target.chapterId } : {}),
      mode,
      type,
      limit,
      time_limit_seconds: timeLimitSeconds,
    }
  }, [target, selectedBankIDs, isOriginalSingleBank, mode, type, limit, timeLimitSeconds])

  useEffect(() => {
    if (!open || !request || request.bank_ids.length === 0) return
    const timer = window.setTimeout(() => {
      setPreviewLoading(true)
      previewPractice(request)
        .then(setPreview)
        .catch((cause) => setError(errMsg(cause)))
        .finally(() => setPreviewLoading(false))
    }, 200)
    return () => window.clearTimeout(timer)
  }, [open, request])

  const start = async () => {
    if (!request || request.bank_ids.length === 0 || starting || (customTimeActive && !customTimeMinutes.trim())) return
    setStarting(true)
    setError('')
    try {
      const session = await createPracticeSession(request)
      onClose()
      navigate(`/practice/${session.id}`)
    } catch (cause) {
      setError(errMsg(cause))
    } finally {
      setStarting(false)
    }
  }

  const openAdvanced = () => {
    if (!target || selectedBankIDs.length === 0) return
    const params = new URLSearchParams()
    params.set('bank_ids', selectedBankIDs.join(','))
    if (isOriginalSingleBank && target.subjectId) params.set('subject_id', String(target.subjectId))
    if (isOriginalSingleBank && target.chapterId) params.set('chapter_id', String(target.chapterId))
    onClose()
    navigate(`/practice/setup?${params.toString()}`)
  }

  const selectedCount = preview?.selected_count ?? 0
  const isCustomTime = customTimeActive
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

  return (
    <Modal
      open={open}
      onClose={() => !starting && onClose()}
      title={
        <div className="flex items-center gap-2">
          <div className="flex h-6 w-6 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Zap className="h-3.5 w-3.5" />
          </div>
          <span>智能刷题设置</span>
        </div>
      }
      description="定制本次自适应练习参数，算法将动态优化抽题与答题反馈"
      width="max-w-lg"
    >
      <div className="space-y-3.5">
        {/* 1. Compact Scope Bar with Dropdown Toggle */}
        {target && (
          <div className="rounded-xl border border-border/70 bg-surface-secondary/40 p-2.5 transition-all">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <span className="text-[11px] font-semibold text-muted-foreground shrink-0">题库范围</span>
                <div className="flex items-center gap-1.5 min-w-0 truncate">
                  <div className="flex items-center gap-1 rounded-md bg-surface px-2 py-0.5 border border-border/60 text-xs font-semibold text-foreground truncate">
                    {selectedBankIDs.length > 1 ? (
                      <>
                        <Layers className="h-3 w-3 text-primary shrink-0" />
                        <span className="truncate">{selectedBankIDs.length} 个题库</span>
                      </>
                    ) : (
                      <>
                        <BookOpen className="h-3 w-3 text-primary shrink-0" />
                        <span className="truncate">{selectedBankNames[0] || target.title}</span>
                      </>
                    )}
                  </div>
                </div>
              </div>

              <button
                type="button"
                onClick={() => setBankPickerOpen((current) => !current)}
                className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-primary hover:bg-primary/10 transition-colors shrink-0 cursor-pointer"
              >
                <span>{bankPickerOpen ? '收起' : '切换 / 多选题库'}</span>
                <ChevronDown className={`h-3 w-3 transition-transform duration-200 ${bankPickerOpen ? 'rotate-180' : ''}`} />
              </button>
            </div>

            {/* Collapsible Bank Picker */}
            {bankPickerOpen && (
              <div className="mt-2.5 pt-2.5 border-t border-border/60 space-y-2 animate-slide-down">
                <div className="flex items-center justify-between gap-2">
                  {/* Search Box */}
                  <div className="relative flex-1">
                    <Search className="absolute left-2.5 top-2 h-3.5 w-3.5 text-muted-foreground/70" />
                    <input
                      className="input h-7.5 w-full pl-8 pr-7 text-xs bg-surface"
                      placeholder="搜索题库..."
                      value={bankSearch}
                      onChange={(event) => setBankSearch(event.target.value)}
                      autoFocus
                    />
                    {bankSearch && (
                      <button
                        type="button"
                        onClick={() => setBankSearch('')}
                        className="absolute right-2 top-2 text-muted-foreground hover:text-foreground"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0 text-[11px]">
                    <button
                      type="button"
                      className="font-medium text-primary hover:underline"
                      onClick={() => setSelectedBankIDs((current) => Array.from(new Set([...current, ...visibleBanks.map((b) => b.id)])))}
                    >
                      全选
                    </button>
                    <span className="text-border">|</span>
                    <button
                      type="button"
                      className="font-medium text-muted-foreground hover:text-foreground"
                      onClick={() => setSelectedBankIDs([])}
                    >
                      清空
                    </button>
                  </div>
                </div>

                {/* Bank Items List */}
                <div className="max-h-36 overflow-y-auto rounded-lg border border-border/70 bg-surface divide-y divide-border/40">
                  {banksLoading ? (
                    <div className="p-3 text-center text-xs text-muted-foreground">正在加载可用题库…</div>
                  ) : visibleBanks.length === 0 ? (
                    <div className="p-3 text-center text-xs text-muted-foreground">未找到匹配题库</div>
                  ) : (
                    visibleBanks.map((bank) => {
                      const checked = selectedBankIDs.includes(bank.id)
                      return (
                        <div
                          key={bank.id}
                          onClick={() => setSelectedBankIDs((current) => checked ? current.filter((id) => id !== bank.id) : [...current, bank.id])}
                          className={`flex items-center gap-2.5 px-2.5 py-1.5 text-xs transition-colors cursor-pointer ${
                            checked ? 'bg-primary/[0.08]' : 'hover:bg-surface-secondary/60'
                          }`}
                        >
                          <span className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border transition-all ${
                            checked ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-surface text-transparent'
                          }`}>
                            <Check className="h-2.5 w-2.5 stroke-[3]" />
                          </span>
                          <span className="min-w-0 flex-1 truncate font-medium text-foreground">{bank.name}</span>
                          <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                            {bank.question_count} 题
                          </span>
                        </div>
                      )
                    })
                  )}
                </div>
              </div>
            )}
          </div>
        )}

        {/* 2. Practice Mode Selection (Compact 3x2 Grid) */}
        <div>
          <label className="text-[11px] font-semibold text-muted-foreground mb-1.5 block">刷题模式</label>
          <div className="grid grid-cols-3 gap-1.5">
            {modeOptions.map(({ value, label, icon: Icon }) => {
              const selectedMode = mode === value
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => setMode(value)}
                  className={`flex items-center justify-center gap-1.5 rounded-xl border py-2 px-2 text-xs transition-all duration-150 cursor-pointer active:scale-[0.98] ${
                    selectedMode
                      ? 'border-primary bg-primary/10 text-primary font-bold shadow-subtle ring-1 ring-primary/25'
                      : 'border-border/70 bg-surface text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary/70 hover:text-foreground'
                  }`}
                >
                  <Icon className={`h-3.5 w-3.5 shrink-0 ${selectedMode ? 'text-primary' : 'text-muted-foreground'}`} />
                  <span className="truncate">{label}</span>
                </button>
              )
            })}
          </div>
        </div>

        {/* 3. Question Type & Question Count (Compact 2-Column Row) */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="text-[11px] font-semibold text-muted-foreground mb-1.5 block">题目类型</label>
            <select
              className="input text-xs h-8 bg-surface font-medium cursor-pointer w-full"
              value={type}
              onChange={(event) => setType(event.target.value as QuestionType | '')}
            >
              {typeOptions.map((option) => (
                <option key={option.value || 'all'} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-[11px] font-semibold text-muted-foreground mb-1.5 block">抽题数量</label>
            <div className="flex items-center gap-1.5">
              <input
                className="input w-14 font-mono text-xs h-8 bg-surface font-bold text-center px-1 shrink-0"
                type="number"
                min={1}
                max={500}
                value={limit}
                onChange={(event) => setLimit(Math.min(500, Math.max(1, Number(event.target.value) || 1)))}
              />
              <div className="grid grid-cols-4 gap-1 flex-1">
                {questionPresets.map((preset) => (
                  <button
                    key={preset}
                    type="button"
                    onClick={() => setLimit(preset)}
                    className={`h-8 rounded-lg border text-[11px] font-mono font-semibold transition-all duration-150 active:scale-95 flex items-center justify-center ${
                      limit === preset
                        ? 'border-primary/40 bg-primary/10 text-primary ring-1 ring-primary/20 font-bold'
                        : 'border-border/70 text-muted-foreground hover:border-primary/30 hover:text-foreground'
                    }`}
                  >
                    {preset}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* 4. Time Limit Selector (Sleek Segmented Controls) */}
        <div>
          <label className="text-[11px] font-semibold text-muted-foreground mb-1.5 block">刷题时长</label>
          <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-6">
            {timeOptions.map((option) => {
              const selectedTime = timeLimitSeconds === option.value
              return (
                <button
                  key={option.label}
                  type="button"
                  onClick={() => {
                    setCustomTimeActive(false)
                    setTimeLimitSeconds(option.value)
                  }}
                  className={`flex items-center justify-center gap-1 rounded-xl border py-1.5 px-1 text-xs transition-all duration-150 cursor-pointer active:scale-[0.98] ${
                    selectedTime
                      ? 'border-primary bg-primary/10 font-bold text-primary ring-1 ring-primary/25 shadow-subtle'
                      : 'border-border/70 text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary/70 hover:text-foreground'
                  }`}
                >
                  {option.value === null ? <Clock3 className="h-3 w-3 shrink-0" /> : <Timer className="h-3 w-3 shrink-0" />}
                  <span className="truncate">{option.label}</span>
                </button>
              )
            })}
            <button
              type="button"
              aria-pressed={isCustomTime}
              onClick={() => updateCustomTime(customTimeMinutes)}
              className={`flex flex-col items-center justify-center gap-0.5 rounded-xl border px-2 py-2.5 text-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 ${
                isCustomTime
                  ? 'border-primary bg-primary/10 font-bold text-primary ring-1 ring-primary/30'
                  : 'border-border/80 text-muted-foreground hover:border-primary/30 hover:bg-surface-secondary'
              }`}
            >
              <span className="flex items-center gap-1"><Timer className="h-3.5 w-3.5" />自定义</span>
              <span className="text-[9px] text-muted-foreground">按分钟设置</span>
            </button>
          </div>
          {isCustomTime && (
            <div className="mt-2 flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/[0.04] px-3 py-2">
              <label htmlFor="quick-custom-practice-time" className="shrink-0 text-[11px] font-semibold text-muted-foreground">自定义时长</label>
              <div className="relative min-w-0 flex-1">
                <input
                  id="quick-custom-practice-time"
                  className="input h-8 w-full pr-12 text-center text-xs font-mono font-bold"
                  type="number"
                  min={1}
                  max={maxCustomTimeMinutes}
                  value={customTimeMinutes}
                  onChange={(event) => updateCustomTime(event.target.value)}
                  aria-label="自定义刷题时长（分钟）"
                />
                <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground">分钟</span>
              </div>
              <span className="shrink-0 text-[10px] text-muted-foreground">最多 24 小时</span>
            </div>
          )}
        </div>

        {error && (
          <div className="rounded-xl border border-destructive/25 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {error}
          </div>
        )}

        {/* 5. Streamlined Action Footer with Integrated Live Preview */}
        <div className="flex items-center justify-between gap-3 border-t border-border/60 pt-3">
          <div className="space-y-0.5 min-w-0">
            <div className="flex items-center gap-1.5 text-xs">
              <span className="text-muted-foreground">预计抽取:</span>
              <span className="font-mono text-xs font-bold text-primary">
                {previewLoading ? '…' : `${selectedCount} 题`}
              </span>
              {preview && preview.available_count > 0 && (
                <span className="text-[11px] text-muted-foreground font-mono">
                  / 可用 {preview.available_count} 题
                </span>
              )}
              {preview && limit > preview.available_count && preview.available_count > 0 && (
                <span className="rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                  按最大量
                </span>
              )}
            </div>
            <div>
              <button
                type="button"
                onClick={openAdvanced}
                disabled={starting || selectedBankIDs.length === 0}
                className="text-[11px] font-medium text-muted-foreground hover:text-primary transition-colors cursor-pointer inline-flex items-center gap-0.5"
              >
                <span>高级自定义设置</span>
                <span className="text-[10px]">↗</span>
              </button>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <Button variant="outline" size="sm" onClick={onClose} disabled={starting} className="text-xs h-8 px-3">
              取消
            </Button>
            <Button
              size="sm"
              onClick={start}
              loading={starting}
              disabled={!preview || selectedCount === 0 || previewLoading || (customTimeActive && !customTimeMinutes.trim())}
              icon={<Play className="h-3 w-3 fill-current" />}
              className="shadow-glow font-bold text-xs h-8 px-4"
            >
              开始刷题
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  )
}
