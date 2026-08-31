import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { showToast } from '@/components/mobile/MobileToast'
import type { Chapter, PracticeMode, QuestionBank, QuestionType, Subject } from '@/types'
import {
  BookOpen,
  Check,
  ChevronLeft,
  ChevronRight,
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
} from 'lucide-react'

const modeOptions: Array<{ value: PracticeMode; label: string; icon: typeof BookOpen; desc: string }> = [
  { value: 'sequential', label: '顺序刷题', icon: BookOpen, desc: '按题目序号顺序练习' },
  { value: 'random', label: '随机乱序', icon: Shuffle, desc: '打破固定顺序随机抽题' },
  { value: 'unattempted', label: '未做题目', icon: ListChecks, desc: '优先抽取尚未作答题目' },
  { value: 'wrong', label: '错题强化', icon: RotateCcw, desc: '针对错题本重点突击' },
  { value: 'favorite', label: '收藏专练', icon: Star, desc: '专项复习星标重点题' },
  { value: 'due', label: '到期复习', icon: Sparkles, desc: '艾宾浩斯遗忘曲线复习' },
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

const timeOptions = [
  { value: null, label: '不限时' },
  { value: 15 * 60, label: '15 分钟' },
  { value: 30 * 60, label: '30 分钟' },
  { value: 45 * 60, label: '45 分钟' },
  { value: 60 * 60, label: '60 分钟' },
]

const maxCustomTimeMinutes = 24 * 60

export default function MobilePracticeSetup() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [banks, setBanks] = useState<QuestionBank[]>([])
  const [selectedBankIds, setSelectedBankIds] = useState<number[]>(() => {
    const p = searchParams.get('bank_id') || searchParams.get('bank_ids')
    return (p || '').split(',').map(Number).filter((id, index, ids) => id > 0 && ids.indexOf(id) === index)
  })
  const [mode, setMode] = useState<PracticeMode>('sequential')
  const [limit, setLimit] = useState(20)
  const [selectedType, setSelectedType] = useState<QuestionType | ''>('')
  const [timeLimit, setTimeLimit] = useState<number | null>(null)
  const [customTimeMinutes, setCustomTimeMinutes] = useState('90')
  const [loading, setLoading] = useState(true)
  const [starting, setStarting] = useState(false)

  // Bank selector sheet
  const [bankPickerOpen, setBankPickerOpen] = useState(false)

  const loadBanks = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/question-banks', { params: { page_size: 50 } })
      const list: QuestionBank[] = resp.data.data.items || []
      setBanks(list)
      if (selectedBankIds.length === 0 && list.length > 0) {
        setSelectedBankIds([list[0].id])
      }
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setLoading(false)
    }
  }, [selectedBankIds.length])

  useEffect(() => {
    loadBanks()
  }, [loadBanks])

  const selectedBanks = banks.filter((bank) => selectedBankIds.includes(bank.id))
  const isCustomTime = timeLimit !== null && !timeOptions.some((option) => option.value === timeLimit)
  const updateCustomTime = (value: string) => {
    setCustomTimeMinutes(value)
    const minutes = Number(value)
    setTimeLimit(Number.isFinite(minutes) && minutes >= 1 ? Math.min(maxCustomTimeMinutes, Math.floor(minutes)) * 60 : null)
  }

  const handleStart = async () => {
    if (selectedBankIds.length === 0) {
      showToast('请先选择题库', 'warning')
      return
    }
    setStarting(true)
    try {
      const payload: Record<string, unknown> = {
        bank_ids: selectedBankIds,
        mode,
        limit,
        time_limit_seconds: timeLimit,
      }
      if (selectedType) payload.type = selectedType

      const resp = await client.post('/practice/sessions', payload)
      navigate(`/practice/${resp.data.data.id}`)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="relative min-h-full select-none">
      <MobilePullToRefresh onRefresh={loadBanks} className="p-4 space-y-4 pb-28">
        {/* 1. Target Bank Selector Card */}
        <div className="space-y-1.5">
          <label className="text-xs font-bold text-foreground block">目标题库</label>
          <div
            onClick={() => setBankPickerOpen(true)}
            className="flex items-center justify-between p-3.5 rounded-2xl border border-border/80 bg-surface shadow-card active:scale-[0.98] transition-all cursor-pointer"
          >
            <div className="flex items-center gap-2.5 min-w-0">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <BookOpen className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <span className="font-bold text-xs text-foreground block truncate">
                  {selectedBanks.length > 0 ? `${selectedBanks.length} 个题库` : '选择题库...'}
                </span>
                <span className="text-[10px] text-muted-foreground font-mono">
                  {selectedBanks.length > 0
                    ? `${selectedBanks.reduce((sum, bank) => sum + bank.question_count, 0)} 题可用 · ${selectedBanks.map((bank) => bank.name).join('、')}`
                    : '点击选择'}
                </span>
              </div>
            </div>
            <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" />
          </div>
        </div>

        {/* 2. Practice Mode Selector */}
        <div className="space-y-1.5">
          <label className="text-xs font-bold text-foreground block">刷题模式</label>
          <div className="grid grid-cols-2 gap-2">
            {modeOptions.map((m) => {
              const isActive = mode === m.value
              return (
                <button
                  key={m.value}
                  type="button"
                  onClick={() => setMode(m.value)}
                  className={`flex flex-col items-start p-3 rounded-2xl border text-left transition-all active:scale-[0.98] ${
                    isActive
                      ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                      : 'border-border bg-surface text-foreground'
                  }`}
                >
                  <div className="flex items-center gap-1.5 font-bold text-xs">
                    <m.icon className="h-3.5 w-3.5" />
                    <span>{m.label}</span>
                  </div>
                  <span className="text-[10px] text-muted-foreground mt-0.5">{m.desc}</span>
                </button>
              )
            })}
          </div>
        </div>

        {/* 3. Question Count Limit */}
        <div className="space-y-1.5">
          <label className="text-xs font-bold text-foreground block">单次刷题题量</label>
          <div className="flex items-center gap-2">
            {[10, 20, 50, 100].map((num) => (
              <button
                key={num}
                type="button"
                onClick={() => setLimit(num)}
                className={`flex-1 py-2 rounded-xl border text-xs font-mono font-bold transition-all ${
                  limit === num
                    ? 'border-primary bg-primary text-white shadow-glow'
                    : 'border-border bg-surface text-muted-foreground'
                }`}
              >
                {num} 题
              </button>
            ))}
            <input
              type="number"
              min={1}
              max={500}
              value={limit}
              onChange={(event) => setLimit(Math.min(500, Math.max(1, Number(event.target.value) || 1)))}
              className="input h-9 w-full rounded-xl text-center text-xs font-mono"
              aria-label="自定义题目数量"
            />
          </div>
        </div>

        {/* 4. Question Type Filter Chips */}
        <div className="space-y-1.5">
          <label className="text-xs font-bold text-foreground block">指定题型</label>
          <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
            {typeOptions.map((t) => (
              <button
                key={t.value}
                type="button"
                onClick={() => setSelectedType(t.value)}
                className={`rounded-full px-3 py-1.5 text-xs font-semibold shrink-0 transition-all ${
                  selectedType === t.value
                    ? 'bg-primary text-white shadow-subtle font-bold'
                    : 'bg-surface border border-border text-muted-foreground'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>

        {/* 5. Time Limit Options */}
        <div className="space-y-1.5">
          <label className="text-xs font-bold text-foreground block">时间限制</label>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {timeOptions.map((t, idx) => (
              <button
                key={idx}
                type="button"
                onClick={() => setTimeLimit(t.value)}
                className={`py-2 rounded-xl border text-xs font-semibold transition-all ${
                  timeLimit === t.value
                    ? 'border-primary bg-primary text-white shadow-glow'
                    : 'border-border bg-surface text-muted-foreground'
                }`}
              >
                {t.label}
              </button>
            ))}
            <button
              type="button"
              aria-pressed={isCustomTime}
              onClick={() => updateCustomTime(customTimeMinutes)}
              className={`rounded-xl border py-2 text-xs font-semibold transition-all ${isCustomTime ? 'border-primary bg-primary text-white shadow-glow' : 'border-border bg-surface text-muted-foreground'}`}
            >
              自定义
            </button>
          </div>
          {isCustomTime && (
            <div className="mt-2 flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/[0.04] p-2">
              <label htmlFor="mobile-custom-practice-time" className="shrink-0 text-xs font-semibold text-muted-foreground">自定义时长</label>
              <input
                id="mobile-custom-practice-time"
                type="number"
                min={1}
                max={maxCustomTimeMinutes}
                value={customTimeMinutes}
                onChange={(event) => updateCustomTime(event.target.value)}
                className="input h-9 min-w-0 flex-1 rounded-xl text-center text-xs font-mono font-bold"
              />
              <span className="shrink-0 text-[10px] text-muted-foreground">分钟</span>
            </div>
          )}
        </div>
      </MobilePullToRefresh>

      {/* Sticky Bottom Action */}
      <div className="fixed bottom-0 inset-x-0 z-30 border-t border-border/70 bg-surface/95 backdrop-blur-xl p-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)] shadow-float">
        <Button
          size="lg"
          loading={starting}
          onClick={handleStart}
          icon={<Play className="h-4 w-4 fill-current" />}
          className="w-full h-12 rounded-2xl font-bold shadow-glow text-sm"
        >
          开始刷题 · {selectedBankIds.length} 个题库 · {limit} 题
        </Button>
      </div>

      {/* Bank Picker Sheet */}
      <MobileBottomSheet
        open={bankPickerOpen}
        onClose={() => setBankPickerOpen(false)}
        title="选择刷题题库"
      >
        <div className="space-y-2">
          {banks.map((b) => (
            <button
              key={b.id}
              type="button"
              onClick={() => setSelectedBankIds((previous) => previous.includes(b.id) ? previous.filter((id) => id !== b.id) : [...previous, b.id])}
              className={`w-full flex items-center justify-between p-3.5 rounded-2xl border text-xs font-semibold text-left transition-all ${
                selectedBankIds.includes(b.id)
                  ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                  : 'border-border bg-surface text-foreground'
              }`}
            >
              <div className="min-w-0 pr-2">
                <span className="font-bold block truncate">{b.name}</span>
                <span className="text-[10px] text-muted-foreground font-mono">{b.question_count} 题</span>
              </div>
              {selectedBankIds.includes(b.id) && <Check className="h-4 w-4 text-primary shrink-0" />}
            </button>
          ))}
          <div className="flex items-center gap-2 pt-2">
            <Button type="button" variant="outline" className="flex-1 rounded-xl text-xs" onClick={() => setSelectedBankIds([])}>
              清空选择
            </Button>
            <Button type="button" className="flex-1 rounded-xl text-xs" onClick={() => setBankPickerOpen(false)}>
              完成选择 ({selectedBankIds.length})
            </Button>
          </div>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
