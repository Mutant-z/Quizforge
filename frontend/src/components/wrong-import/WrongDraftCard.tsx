import { useEffect, useState } from 'react'
import { Badge, Button, Spinner } from '@/components/ui'
import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronUp,
  Copy,
  FileQuestion,
  ImageIcon,
  Maximize2,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { wrongImportApi } from '@/api/wrongImport'
import type { WrongImportDraft } from '@/types/wrongImport'
import AuthenticatedImage from './AuthenticatedImage'

const typeLabels: Record<string, string> = {
  single_choice: '单选',
  multiple_choice: '多选',
  true_false: '判断',
  fill_blank: '填空',
  short_answer: '简答',
  material_analysis: '材料分析',
  essay: '论述',
  code: '代码',
}

const questionTypes = [
  { value: 'single_choice', label: '单选' },
  { value: 'multiple_choice', label: '多选' },
  { value: 'true_false', label: '判断' },
  { value: 'fill_blank', label: '填空' },
  { value: 'short_answer', label: '简答' },
  { value: 'material_analysis', label: '材料分析' },
  { value: 'essay', label: '论述' },
  { value: 'code', label: '代码' },
] as const
type WrongImportQuestionType = (typeof questionTypes)[number]['value']

interface Props {
  draft: WrongImportDraft
  sessionId: number
  selected: boolean
  active?: boolean
  onToggleSelect: (id: number) => void
  onOpenDetail?: (draft: WrongImportDraft) => void
  onUpdated: (draft: WrongImportDraft) => void
  onDeleted: (id: number) => void
}

export default function WrongDraftCard({
  draft,
  sessionId,
  selected,
  active = false,
  onToggleSelect,
  onOpenDetail,
  onUpdated,
  onDeleted,
}: Props) {
  const [stem, setStem] = useState(draft.stem)
  const [analysis, setAnalysis] = useState(draft.analysis ?? '')
  const [saving, setSaving] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [showRegion, setShowRegion] = useState(false)
  const [sourceIndex, setSourceIndex] = useState(0)

  const sources = draft.sources && draft.sources.length > 0 ? draft.sources : [{ image_id: draft.image_id, region: draft.region }]
  const currentSource = sources[Math.min(sourceIndex, sources.length - 1)]

  useEffect(() => {
    setStem(draft.stem)
    setAnalysis(draft.analysis ?? '')
    setSourceIndex(0)
  }, [draft.id, draft.stem, draft.analysis, draft.sources?.length])

  const committed = draft.review_status === 'committed'
  const excluded = draft.review_status === 'excluded'
  const missingAnswer = draft.review_status === 'missing_answer'
  const isReady = draft.review_status === 'ready'
  const selectable = isReady && !committed && !excluded

  const handleCardClick = (event: React.MouseEvent<HTMLDivElement>) => {
    if (!onOpenDetail) return

    // 保留题卡内原有的直接编辑、选答案和操作按钮行为。
    const target = event.target as HTMLElement
    if (target.closest('button, input, textarea, select, a, [role="button"]')) return
    onOpenDetail(draft)
  }

  const patch = async (body: Record<string, unknown>) => {
    setSaving(true)
    try {
      const updated = await wrongImportApi.patchDraft(sessionId, draft.id, body)
      onUpdated(updated)
    } finally {
      setSaving(false)
    }
  }

  const saveStem = () => {
    if (stem.trim() && stem.trim() !== draft.stem) void patch({ stem: stem.trim() })
  }

  const saveAnalysis = () => {
    if (analysis !== (draft.analysis ?? '')) void patch({ analysis })
  }

  const setAnswer = (answer: string[]) => void patch({ answer })
  const markNoAnswer = () => void patch({ mark_no_answer: true })

  const changeQuestionType = (questionType: WrongImportQuestionType) => {
    if (committed || questionType === draft.question_type || saving) return

    // 单选题不能携带多个答案。清空后让用户重新选择，避免静默保留一个任意选项。
    const answer = questionType === 'single_choice' && draft.answer.length > 1 ? [] : draft.answer
    const body: Record<string, unknown> = { question_type: questionType }
    if (answer !== draft.answer) body.answer = answer
    void patch(body)
  }

  const toggleExclude = () => void patch({ excluded: !excluded })

  const remove = async () => {
    if (!confirmDelete) {
      setConfirmDelete(true)
      setTimeout(() => setConfirmDelete(false), 2500)
      return
    }
    await wrongImportApi.deleteDraft(sessionId, draft.id)
    onDeleted(draft.id)
  }

  const statusBadge = committed ? (
    <Badge tone="neutral" variant="subtle" className="text-[10px] px-2 py-0.5">
      已入库
    </Badge>
  ) : excluded ? (
    <Badge tone="neutral" variant="outline" className="text-[10px] px-2 py-0.5">
      已排除
    </Badge>
  ) : missingAnswer ? (
    <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 border border-amber-500/30 px-2 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
      <span className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-pulse" />
      缺答案
    </span>
  ) : isReady ? (
    <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 border border-emerald-500/30 px-2 py-0.5 text-[10px] font-semibold text-emerald-600 dark:text-emerald-400">
      <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
      就绪
    </span>
  ) : (
    <Badge tone="info" variant="subtle" className="text-[10px] px-2 py-0.5">
      识别中
    </Badge>
  )

  return (
    <div
      onClick={handleCardClick}
      title={onOpenDetail ? '点击题卡空白区域查看并编辑详情' : undefined}
      data-active={active || undefined}
      className={`group relative rounded-2xl border p-3.5 space-y-3 shadow-subtle transition-all duration-200 animate-message-enter ${
        missingAnswer
          ? 'border-amber-500/35 bg-gradient-to-b from-amber-500/[0.03] to-surface hover:border-amber-500/60 hover:shadow-card'
          : isReady
            ? 'border-emerald-500/30 bg-gradient-to-b from-emerald-500/[0.03] to-surface hover:border-emerald-500/50 hover:shadow-card'
            : committed
              ? 'border-border/50 bg-surface-secondary/20 opacity-80'
              : 'border-border/70 bg-surface hover:border-border hover:shadow-card'
      } ${excluded ? 'opacity-50 border-dashed' : ''} ${onOpenDetail ? 'cursor-pointer' : ''} ${
        active ? 'border-primary/75 bg-primary/[0.06] shadow-card ring-2 ring-primary/20' : ''
      }`}
    >
      {active && (
        <span
          aria-hidden="true"
          className="absolute inset-y-3 left-0.5 w-1 rounded-full bg-primary shadow-glow"
        />
      )}
      {/* Header row */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0 flex-wrap">
          {selectable ? (
            <button
              type="button"
              onClick={() => onToggleSelect(draft.id)}
              className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-md border transition-all duration-150 ${
                selected
                  ? 'bg-primary border-primary text-primary-foreground shadow-xs scale-105'
                  : 'border-border bg-surface hover:border-primary/60'
              }`}
              title="勾选后随「确认入库」提交"
            >
              {selected && <Check className="h-3 w-3 stroke-[3]" />}
            </button>
          ) : (
            <span className="h-4 w-4 shrink-0" />
          )}
          <button
            type="button"
            onClick={() => onOpenDetail?.(draft)}
            className="flex items-center gap-1 font-mono text-xs font-bold text-foreground/80 hover:text-primary transition-colors shrink-0 group/seq cursor-pointer"
            title="点击弹窗查看与编辑详情"
          >
            <span>#{draft.seq_no}</span>
            <Maximize2 className="h-3 w-3 opacity-0 group-hover/seq:opacity-100 transition-opacity text-primary" />
          </button>
          {!committed ? (
            <div
              role="group"
              aria-label="修正题型"
              className="inline-flex shrink-0 items-center rounded-lg border border-primary/20 bg-primary/5 p-0.5"
            >
              <select
                aria-label="修正题型"
                value={draft.question_type}
                onChange={(event) => changeQuestionType(event.target.value as WrongImportQuestionType)}
                disabled={saving}
                className="h-6 rounded-md border-0 bg-transparent px-1.5 text-[10px] font-semibold text-primary outline-none"
              >
                {questionTypes.map((item) => (
                  <option key={item.value} value={item.value}>{item.label}</option>
                ))}
              </select>
            </div>
          ) : (
            <Badge tone="primary" variant="subtle" className="text-[10px] px-2 py-0 font-medium shrink-0">
              {typeLabels[draft.question_type] ?? draft.question_type}
            </Badge>
          )}
          {statusBadge}
          {draft.duplicate_of_draft_id && (
            <Badge tone="info" variant="outline" className="text-[10px] px-1.5 py-0">
              <Copy className="h-2.5 w-2.5 mr-0.5" />
              疑似与 #{draft.duplicate_of_draft_id} 重复
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {saving && <Spinner className="h-3 w-3 text-primary" />}
          <button
            type="button"
            onClick={() => onOpenDetail?.(draft)}
            className="flex h-6 w-6 items-center justify-center rounded-lg text-muted-foreground hover:bg-surface-secondary hover:text-primary transition-colors cursor-pointer"
            title="弹窗查看与编辑详情"
          >
            <Maximize2 className="h-3.5 w-3.5" />
          </button>
          {sources.length > 0 && currentSource?.image_id > 0 && (
            <button
              type="button"
              onClick={() => setShowRegion((v) => !v)}
              className={`flex h-6 w-6 items-center justify-center rounded-lg text-muted-foreground hover:bg-surface-secondary hover:text-primary transition-colors ${
                showRegion ? 'bg-primary/10 text-primary' : ''
              }`}
              title={showRegion ? '收起原图区域' : '查看原图区域'}
            >
              <ImageIcon className="h-3.5 w-3.5" />
            </button>
          )}
          {!committed && (
            <>
              <button
                type="button"
                onClick={toggleExclude}
                className="rounded-lg px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground hover:bg-surface-secondary hover:text-foreground transition-colors"
                title={excluded ? '恢复此题' : '排除此题（不入库）'}
              >
                {excluded ? '恢复' : '排除'}
              </button>
              <button
                type="button"
                onClick={remove}
                className={`flex h-6 w-6 items-center justify-center rounded-lg transition-colors ${
                  confirmDelete
                    ? 'bg-destructive/10 text-destructive'
                    : 'text-muted-foreground hover:bg-destructive/10 hover:text-destructive'
                }`}
                title={confirmDelete ? '再次点击确认删除' : '删除草稿'}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </>
          )}
        </div>
      </div>

      {/* Warnings Banner */}
      {draft.warnings && draft.warnings.length > 0 && !committed && (
        <div className="flex items-start gap-2 rounded-xl bg-amber-500/8 border border-amber-500/20 px-2.5 py-2">
          <AlertTriangle className="h-3.5 w-3.5 text-amber-500 mt-0.5 shrink-0" />
          <span className="text-[11px] text-amber-700 dark:text-amber-300 leading-relaxed font-medium">
            {draft.warnings.join('；')}
          </span>
        </div>
      )}

      {/* Region preview */}
      {showRegion && (
        <div className="overflow-hidden rounded-xl border border-border/70 bg-surface-secondary/30 shadow-subtle animate-slide-down">
          <AuthenticatedImage
            src={wrongImportApi.imageFileUrl(sessionId, currentSource?.image_id ?? draft.image_id, regionParam(currentSource?.region))}
            alt={`第 ${draft.seq_no} 题来源图片 ${sourceIndex + 1}`}
            className="w-full object-contain max-h-64"
          />
          {sources.length > 1 && (
            <div className="flex items-center justify-between border-t border-border/60 bg-surface px-2.5 py-1.5 text-[10px] text-muted-foreground">
              <span>来源图片 {sourceIndex + 1}/{sources.length}</span>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  disabled={sourceIndex === 0}
                  onClick={() => setSourceIndex((index) => Math.max(0, index - 1))}
                  className="rounded-md px-1.5 py-0.5 hover:bg-surface-secondary disabled:opacity-40"
                >
                  上一张
                </button>
                <button
                  type="button"
                  disabled={sourceIndex >= sources.length - 1}
                  onClick={() => setSourceIndex((index) => Math.min(sources.length - 1, index + 1))}
                  className="rounded-md px-1.5 py-0.5 hover:bg-surface-secondary disabled:opacity-40"
                >
                  下一张
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Question Stem */}
      <div className="space-y-1">
        {!committed ? (
          <textarea
            value={stem}
            onChange={(e) => setStem(e.target.value)}
            onBlur={saveStem}
            rows={Math.min(5, Math.max(2, Math.ceil(stem.length / 32)))}
            placeholder="输入或修改题干内容..."
            className="w-full resize-y rounded-xl border border-border/50 bg-surface-secondary/40 px-3 py-2 text-xs leading-relaxed text-foreground placeholder:text-muted-foreground/50 transition-all hover:bg-surface-secondary/70 focus:border-primary/50 focus:bg-surface focus:outline-none focus:ring-2 focus:ring-primary/10"
          />
        ) : (
          <p className="text-xs leading-relaxed text-foreground/90 select-text bg-surface-secondary/30 rounded-xl p-2.5">
            {draft.stem}
          </p>
        )}
      </div>

      {/* Options List with Direct Click-to-Select */}
      {draft.options.length > 0 && (
        <div className="space-y-1.5 rounded-xl bg-surface-secondary/30 p-2 border border-border/40">
          {draft.options.map((opt) => {
            const isSingle = draft.question_type === 'single_choice'
            const isMulti = draft.question_type === 'multiple_choice'
            const isTrueFalse = draft.question_type === 'true_false'
            const isSelected = draft.answer.includes(opt.key)
            const canClick = !committed && (isSingle || isMulti || isTrueFalse)

            const handleClick = () => {
              if (!canClick) return
              if (isSingle || isTrueFalse) {
                // 单选/判断：点击已选中项则取消，否则选中该项
                setAnswer(isSelected ? [] : [opt.key])
              } else if (isMulti) {
                // 多选：切换选中
                const next = isSelected
                  ? draft.answer.filter((a) => a !== opt.key)
                  : [...draft.answer, opt.key].sort()
                setAnswer(next)
              }
            }

            return (
              <div
                key={opt.key}
                onClick={handleClick}
                role={canClick ? 'button' : undefined}
                tabIndex={canClick ? 0 : undefined}
                className={`flex items-start gap-2.5 rounded-xl p-2 text-xs leading-relaxed transition-all duration-150 ${
                  canClick ? 'cursor-pointer select-none' : ''
                } ${
                  isSelected
                    ? 'border border-primary/50 bg-primary/10 text-primary shadow-2xs'
                    : canClick
                      ? 'border border-transparent hover:border-border/80 hover:bg-surface/80 text-foreground/90'
                      : 'border border-transparent text-foreground/85'
                }`}
              >
                <span
                  className={`flex h-5 w-5 shrink-0 items-center justify-center font-mono font-bold text-[10px] transition-all ${
                    isSingle || isTrueFalse ? 'rounded-full' : 'rounded-md'
                  } ${
                    isSelected
                      ? 'bg-primary text-primary-foreground shadow-2xs scale-105'
                      : 'bg-surface text-foreground/75 border border-border/70'
                  }`}
                >
                  {opt.key}
                </span>
                <span className="flex-1 pt-0.5 text-[11px] sm:text-xs leading-normal">
                  {opt.content}
                </span>
                {isSelected && (
                  <span className="shrink-0 flex items-center gap-1 font-bold text-[10px] text-primary pt-0.5">
                    <Check className="h-3 w-3 stroke-[2.5]" />
                    <span>正确</span>
                  </span>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Answer Editor */}
      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-[11px]">
          <span className="font-semibold text-foreground/70 flex items-center gap-1">
            <span>{draft.options.length > 0 ? '快捷答案选择 / 补充' : '正确答案'}</span>
            {draft.question_type === 'single_choice' && (
              <span className="text-[10px] text-muted-foreground/80 font-normal">（单选·圆形）</span>
            )}
            {draft.question_type === 'multiple_choice' && (
              <span className="text-[10px] text-muted-foreground/80 font-normal">（多选·方形）</span>
            )}
          </span>
          {missingAnswer ? (
            <span className="text-[10px] text-amber-600 dark:text-amber-400 font-medium animate-pulse">
              点击选项或下方按钮选择答案
            </span>
          ) : (
            <span className="text-[10px] text-emerald-600 dark:text-emerald-400 font-medium">
              已就绪（点击可更改）
            </span>
          )}
        </div>
        <AnswerEditor draft={draft} disabled={committed} onSetAnswer={setAnswer} onMarkNoAnswer={markNoAnswer} />
      </div>

      {/* Analysis */}
      {draft.analysis || !committed ? (
        <div className="space-y-1">
          {committed ? (
            draft.analysis && (
              <div className="rounded-xl bg-surface-secondary/50 p-2.5 text-xs text-muted-foreground leading-relaxed select-text border border-border/40">
                <span className="font-bold text-foreground/80">解析：</span>
                {draft.analysis}
              </div>
            )
          ) : (
            <textarea
              value={analysis}
              onChange={(e) => setAnalysis(e.target.value)}
              onBlur={saveAnalysis}
              rows={Math.min(3, Math.max(2, Math.ceil(analysis.length / 32)))}
              placeholder="添加解析（图片中未识别出解析时可手动补充）..."
              className="w-full resize-y rounded-xl border border-border/50 bg-surface-secondary/30 px-3 py-2 text-[11px] leading-relaxed text-foreground placeholder:text-muted-foreground/50 transition-all hover:bg-surface-secondary/60 focus:border-primary/50 focus:bg-surface focus:outline-none focus:ring-2 focus:ring-primary/10"
            />
          )}
        </div>
      ) : null}

      {/* Meta Footer */}
      <div className="flex items-center justify-between pt-1 border-t border-border/40 text-[10px] text-muted-foreground">
        <span className="flex items-center gap-1">
          <span>来源{sources.length > 1 ? `（${sources.length} 张图）` : ''}：</span>
          <span className="font-medium text-foreground/70">
            {draft.answer_source === 'image'
              ? '图片识别'
              : draft.answer_source === 'user'
                ? '对话补充'
                : draft.answer_source === 'none'
                  ? '无需答案'
                  : '待补充'}
          </span>
        </span>
        {draft.confidence > 0 && (
          <span className="font-mono flex items-center gap-1">
            <Sparkles className="h-2.5 w-2.5 text-primary" />
            置信度 {(draft.confidence * 100).toFixed(0)}%
          </span>
        )}
      </div>
    </div>
  )
}

function regionParam(region: WrongImportDraft['region']): string | undefined {
  if (!region) return undefined
  const r = region
  return `${r.left},${r.top},${r.right},${r.bottom}`
}

function AnswerEditor({
  draft,
  disabled,
  onSetAnswer,
  onMarkNoAnswer,
}: {
  draft: WrongImportDraft
  disabled: boolean
  onSetAnswer: (answer: string[]) => void
  onMarkNoAnswer: () => void
}) {
  const singleBtnBase =
    'flex h-7 w-7 items-center justify-center rounded-full border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer p-0'
  const multiBtnBase =
    'flex h-7 w-7 items-center justify-center rounded-lg border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer p-0'
  const trueFalseBtnBase =
    'flex h-7 px-3 items-center justify-center rounded-full border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer'

  const activeBtn =
    'border-primary bg-primary text-primary-foreground shadow-subtle scale-105 ring-2 ring-primary/20'
  const idleBtn =
    'border-border/80 bg-surface text-muted-foreground hover:border-primary/40 hover:bg-primary/5 hover:text-primary shadow-2xs'

  const [fillText, setFillText] = useState(draft.answer.join('、'))
  useEffect(() => setFillText(draft.answer.join('、')), [draft.id, draft.answer])

  if (draft.question_type === 'single_choice') {
    const defaultOptions =
      draft.options.length > 0
        ? draft.options
        : [{ key: 'A', content: '' }, { key: 'B', content: '' }, { key: 'C', content: '' }, { key: 'D', content: '' }]
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {defaultOptions.map((opt) => {
          const active = draft.answer[0] === opt.key
          return (
            <button
              key={opt.key}
              type="button"
              disabled={disabled}
              onClick={() => onSetAnswer(active ? [] : [opt.key])}
              className={`${singleBtnBase} ${active ? activeBtn : idleBtn}`}
              title={`单选 ${opt.key}${active ? '（已选）' : ''}`}
            >
              {opt.key}
            </button>
          )
        })}
      </div>
    )
  }

  if (draft.question_type === 'multiple_choice') {
    const defaultOptions =
      draft.options.length > 0
        ? draft.options
        : [{ key: 'A', content: '' }, { key: 'B', content: '' }, { key: 'C', content: '' }, { key: 'D', content: '' }]
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {defaultOptions.map((opt) => {
          const active = draft.answer.includes(opt.key)
          return (
            <button
              key={opt.key}
              type="button"
              disabled={disabled}
              onClick={() =>
                onSetAnswer(
                  active ? draft.answer.filter((a) => a !== opt.key) : [...draft.answer, opt.key].sort(),
                )
              }
              className={`${multiBtnBase} ${active ? activeBtn : idleBtn}`}
              title={`多选 ${opt.key}${active ? '（已选）' : ''}`}
            >
              {opt.key}
            </button>
          )
        })}
        {draft.answer.length > 0 && (
          <span className="font-mono text-[11px] font-bold text-primary bg-primary/10 px-2 py-0.5 rounded-md border border-primary/20">
            已选：{draft.answer.join('')}
          </span>
        )}
      </div>
    )
  }

  if (draft.question_type === 'true_false') {
    return (
      <div className="flex items-center gap-2">
        {['对', '错'].map((v) => {
          const active = draft.answer[0] === v
          return (
            <button
              key={v}
              type="button"
              disabled={disabled}
              onClick={() => onSetAnswer(active ? [] : [v])}
              className={`${trueFalseBtnBase} ${active ? activeBtn : idleBtn}`}
            >
              {v}
            </button>
          )
        })}
      </div>
    )
  }

  if (
    draft.question_type === 'short_answer' ||
    draft.question_type === 'material_analysis' ||
    draft.question_type === 'essay' ||
    draft.question_type === 'code'
  ) {
    const answerItems = draft.answer.map((item) => item.trim()).filter(Boolean)
    const hasAnswer = answerItems.length > 0
    const noneNeeded = (draft.answer_status === 'explicit_none' || (draft.answer_source === 'none' && draft.user_marked_no_answer)) && !hasAnswer
    return (
      <div className="space-y-2">
        {noneNeeded ? (
          <Badge tone="neutral" variant="outline" className="text-[10px] px-2 py-0.5">
            主观题 · 无需标准答案
          </Badge>
        ) : hasAnswer ? (
          <div className="max-h-44 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-emerald-500/20 bg-emerald-500/[0.04] px-2.5 py-2 text-xs leading-relaxed text-emerald-700 dark:text-emerald-300 select-text">
            {answerItems.join('\n\n')}
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">（暂无答案）</span>
        )}
        {!disabled && (
          <Button
            size="xs"
            variant={noneNeeded ? 'subtle' : 'outline'}
            onClick={() => (noneNeeded ? onSetAnswer([]) : onMarkNoAnswer())}
            title={noneNeeded ? '当前已标记为无需答案' : hasAnswer ? '清除当前答案并标记为无需答案' : '标记为无需答案'}
            icon={<FileQuestion className="h-3 w-3" />}
          >
            {noneNeeded ? '已标记无需答案' : hasAnswer ? '清除答案' : '标记无需答案'}
          </Button>
        )}
      </div>
    )
  }

  // fill_blank：失焦保存，避免逐键请求
  return (
    <input
      className="input h-8 text-xs rounded-xl bg-surface-secondary/40 border-border/60 focus:bg-surface"
      value={fillText}
      placeholder="填写标准答案，多个空请用「、」分隔"
      disabled={disabled}
      onChange={(e) => setFillText(e.target.value)}
      onBlur={() => {
        const text = fillText.trim()
        const next = text
          ? text
              .split(/[、,，]/)
              .map((s) => s.trim())
              .filter(Boolean)
          : []
        if (JSON.stringify(next) !== JSON.stringify(draft.answer)) onSetAnswer(next)
      }}
    />
  )
}
