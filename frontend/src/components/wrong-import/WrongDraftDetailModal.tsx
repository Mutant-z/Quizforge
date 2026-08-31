import { useEffect, useRef, useState } from 'react'
import { Badge, Button, Spinner } from '@/components/ui'
import {
  AlertTriangle,
  Check,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Copy,
  FileQuestion,
  FileText,
  ImageIcon,
  Layers,
  Maximize2,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'
import { wrongImportApi } from '@/api/wrongImport'
import type { WrongImportDraft } from '@/types/wrongImport'
import AuthenticatedImage from './AuthenticatedImage'

const typeLabels: Record<string, string> = {
  single_choice: '单选题',
  multiple_choice: '多选题',
  true_false: '判断题',
  fill_blank: '填空题',
  short_answer: '简答题',
  material_analysis: '材料分析题',
  essay: '论述题',
  code: '代码题',
}

const questionTypes = [
  { value: 'single_choice', label: '单选题' },
  { value: 'multiple_choice', label: '多选题' },
  { value: 'true_false', label: '判断题' },
  { value: 'fill_blank', label: '填空题' },
  { value: 'short_answer', label: '简答题' },
  { value: 'material_analysis', label: '材料分析' },
  { value: 'essay', label: '论述题' },
  { value: 'code', label: '代码题' },
] as const

type WrongImportQuestionType = (typeof questionTypes)[number]['value']

interface WrongDraftDetailModalProps {
  draft: WrongImportDraft | null
  allDrafts: WrongImportDraft[]
  sessionId: number
  isOpen: boolean
  onClose: () => void
  onSelectDraft: (draft: WrongImportDraft) => void
  onUpdated: (draft: WrongImportDraft) => void
  onDeleted: (id: number) => void
}

function regionParam(region: WrongImportDraft['region']): string | undefined {
  if (!region) return undefined
  const r = region
  return `${r.left},${r.top},${r.right},${r.bottom}`
}

export default function WrongDraftDetailModal({
  draft,
  allDrafts,
  sessionId,
  isOpen,
  onClose,
  onSelectDraft,
  onUpdated,
  onDeleted,
}: WrongDraftDetailModalProps) {
  const [stem, setStem] = useState(draft?.stem ?? '')
  const [analysis, setAnalysis] = useState(draft?.analysis ?? '')
  const [options, setOptions] = useState(draft?.options ?? [])
  const [saving, setSaving] = useState(false)
  const [sourceIndex, setSourceIndex] = useState(0)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [showImagePanel, setShowImagePanel] = useState(true)

  useEffect(() => {
    if (!draft) return
    setStem(draft.stem)
    setAnalysis(draft.analysis ?? '')
    setOptions(draft.options ?? [])
    setSourceIndex(0)
    setConfirmDelete(false)
  }, [draft])

  // 快捷键支持：ESC 关闭，左/右方向键切换题目
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        event.target instanceof HTMLInputElement ||
        event.target instanceof HTMLTextAreaElement ||
        event.target instanceof HTMLSelectElement
      ) {
        return
      }

      if (event.key === 'Escape') {
        onClose()
      } else if (event.key === 'ArrowLeft') {
        const idx = allDrafts.findIndex((d) => d.id === draft?.id)
        if (idx > 0) onSelectDraft(allDrafts[idx - 1])
      } else if (event.key === 'ArrowRight') {
        const idx = allDrafts.findIndex((d) => d.id === draft?.id)
        if (idx >= 0 && idx < allDrafts.length - 1) onSelectDraft(allDrafts[idx + 1])
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, onClose, allDrafts, draft, onSelectDraft])

  if (!draft || !isOpen) return null

  const sources =
    draft.sources && draft.sources.length > 0
      ? draft.sources
      : [{ image_id: draft.image_id, region: draft.region }]
  const currentSource = sources[Math.min(sourceIndex, sources.length - 1)]
  const hasImage = sources.length > 0 && (currentSource?.image_id ?? 0) > 0

  // Current draft index for navigation
  const currentIndex = allDrafts.findIndex((d) => d.id === draft.id)
  const hasPrev = currentIndex > 0
  const hasNext = currentIndex >= 0 && currentIndex < allDrafts.length - 1

  const committed = draft.review_status === 'committed'
  const excluded = draft.review_status === 'excluded'
  const missingAnswer = draft.review_status === 'missing_answer'
  const isReady = draft.review_status === 'ready'

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
    if (questionType === draft.question_type || saving) return
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
    if (hasNext) {
      onSelectDraft(allDrafts[currentIndex + 1])
    } else if (hasPrev) {
      onSelectDraft(allDrafts[currentIndex - 1])
    } else {
      onClose()
    }
  }

  // Handle Option edits
  const handleOptionContentChange = (index: number, newContent: string) => {
    const updated = [...options]
    updated[index] = { ...updated[index], content: newContent }
    setOptions(updated)
  }

  const saveOptionContent = () => {
    if (JSON.stringify(options) !== JSON.stringify(draft.options)) {
      void patch({ options })
    }
  }

  const handleAddOption = () => {
    const nextKey = String.fromCharCode(65 + options.length) // A, B, C, D, E...
    const updated = [...options, { key: nextKey, content: '' }]
    setOptions(updated)
    void patch({ options: updated })
  }

  const handleDeleteOption = (index: number) => {
    const targetKey = options[index].key
    const filtered = options.filter((_, i) => i !== index)
    const newAnswer = draft.answer.filter((k) => k !== targetKey)
    setOptions(filtered)
    void patch({ options: filtered, answer: newAnswer })
  }

  const handlePrev = () => {
    if (hasPrev) onSelectDraft(allDrafts[currentIndex - 1])
  }

  const handleNext = () => {
    if (hasNext) onSelectDraft(allDrafts[currentIndex + 1])
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 md:p-6 overflow-hidden">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 dark:bg-black/75 backdrop-blur-sm transition-opacity animate-in fade-in-0 duration-200"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog container */}
      <div
        className={`relative w-full ${
          hasImage && showImagePanel ? 'max-w-6xl' : 'max-w-3xl'
        } h-[92vh] max-h-[880px] flex flex-col rounded-2xl sm:rounded-3xl border border-border/80 bg-surface shadow-2xl transition-all my-auto z-10 overflow-hidden animate-in zoom-in-95 duration-150`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="wrong-draft-detail-title"
      >
        {/* 顶部 Header */}
        <div className="flex items-center justify-between border-b border-border/70 bg-surface/90 backdrop-blur-md px-4 sm:px-6 py-3.5 shrink-0 z-20">
          <div className="flex items-center gap-2.5 min-w-0 flex-wrap">
            {/* 序号 Pill */}
            <div className="flex h-7 items-center justify-center gap-1 rounded-xl bg-primary/10 text-primary font-mono text-xs font-bold px-2.5 border border-primary/20 shadow-xs">
              <span>#{draft.seq_no}</span>
            </div>

            <h2 id="wrong-draft-detail-title" className="text-sm sm:text-base font-bold text-foreground tracking-tight">
              题目详情与核对
            </h2>

            {/* 题型选择器（支持自由切换修正） */}
            <div className="inline-flex items-center rounded-xl border border-primary/20 bg-primary/5 p-0.5 shadow-2xs hover:border-primary/40 transition-colors">
              <select
                aria-label="修正题型"
                value={draft.question_type}
                onChange={(event) =>
                  changeQuestionType(event.target.value as WrongImportQuestionType)
                }
                disabled={saving}
                className="h-6.5 rounded-lg border-0 bg-transparent px-2.5 text-xs font-semibold text-primary outline-none cursor-pointer focus:ring-0"
              >
                {questionTypes.map((item) => (
                  <option key={item.value} value={item.value} className="text-foreground bg-surface">
                    {item.label}
                  </option>
                ))}
              </select>
            </div>

            {/* 状态徽章 */}
            {committed ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 border border-emerald-500/30 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-3 w-3 text-emerald-500" />
                已入库
              </span>
            ) : excluded ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-muted/40 border border-border px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
                已排除
              </span>
            ) : missingAnswer ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 border border-amber-500/30 px-2.5 py-0.5 text-xs font-semibold text-amber-600 dark:text-amber-400">
                <span className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-pulse" />
                缺答案
              </span>
            ) : isReady ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 border border-emerald-500/30 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                就绪
              </span>
            ) : (
              <Badge tone="info" variant="subtle" className="text-xs px-2.5 py-0.5 rounded-full">
                识别中
              </Badge>
            )}

            {draft.duplicate_of_draft_id && (
              <Badge tone="info" variant="outline" className="text-xs px-2 py-0.5 rounded-lg">
                <Copy className="h-3 w-3 mr-1" />
                疑似与 #{draft.duplicate_of_draft_id} 重复
              </Badge>
            )}
          </div>

          {/* 顶部右侧功能操作 */}
          <div className="flex items-center gap-2 shrink-0">
            {saving && (
              <div className="flex items-center gap-1.5 text-xs text-primary font-medium bg-primary/5 px-2.5 py-1 rounded-lg animate-in fade-in-0">
                <Spinner className="h-3.5 w-3.5 text-primary" />
                <span>保存中…</span>
              </div>
            )}

            {hasImage && (
              <button
                type="button"
                onClick={() => setShowImagePanel((prev) => !prev)}
                className={`hidden md:flex items-center gap-1 px-2.5 py-1 text-xs font-medium rounded-xl border transition-all cursor-pointer ${
                  showImagePanel
                    ? 'bg-primary/10 border-primary/30 text-primary'
                    : 'bg-surface text-muted-foreground border-border/60 hover:bg-surface-secondary hover:text-foreground'
                }`}
                title={showImagePanel ? '折叠左侧原图切片' : '展开原图切片对照'}
              >
                {showImagePanel ? (
                  <>
                    <PanelLeftClose className="h-3.5 w-3.5" />
                    <span>原图对照</span>
                  </>
                ) : (
                  <>
                    <PanelLeftOpen className="h-3.5 w-3.5" />
                    <span>查看原图</span>
                  </>
                )}
              </button>
            )}

            <button
              type="button"
              onClick={toggleExclude}
              className={`rounded-xl px-2.5 py-1 text-xs font-medium transition-all border cursor-pointer ${
                excluded
                  ? 'bg-primary/10 text-primary border-primary/30'
                  : 'text-muted-foreground hover:bg-surface-secondary hover:text-foreground border-border/60'
              }`}
              title={excluded ? '恢复此题' : '排除此题（不入库）'}
            >
              {excluded ? '恢复此题' : '排除'}
            </button>
            <button
              type="button"
              onClick={remove}
              className={`flex h-7.5 items-center gap-1 px-2.5 rounded-xl text-xs font-medium transition-all cursor-pointer ${
                confirmDelete
                  ? 'bg-destructive text-destructive-foreground shadow-xs'
                  : 'text-muted-foreground border border-border/60 hover:bg-destructive/10 hover:text-destructive hover:border-destructive/30'
              }`}
              title={confirmDelete ? '再次点击确认删除' : '删除草稿'}
            >
              <Trash2 className="h-3.5 w-3.5" />
              <span>{confirmDelete ? '确认删除' : '删除'}</span>
            </button>

            <button
              type="button"
              onClick={onClose}
              className="flex h-7.5 w-7.5 items-center justify-center rounded-xl text-muted-foreground hover:bg-surface-secondary hover:text-foreground transition-colors ml-1 cursor-pointer border border-transparent hover:border-border/60"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* 弹窗主体内容区（支持有原图时的左右分栏） */}
        <div className="flex-1 min-h-0 flex flex-col md:flex-row overflow-hidden bg-background/40 divide-y md:divide-y-0 md:divide-x divide-border/60">
          {/* 左侧：来源原图切片对照面板（在有原图且开启时显示） */}
          {hasImage && showImagePanel && (
            <div className="w-full md:w-5/12 lg:w-1/2 flex flex-col shrink-0 bg-surface/40 overflow-hidden max-h-[35vh] md:max-h-full">
              <div className="flex items-center justify-between border-b border-border/60 bg-surface-secondary/40 px-4 py-2.5 text-xs font-semibold text-foreground shrink-0">
                <div className="flex items-center gap-1.5">
                  <ImageIcon className="h-4 w-4 text-primary" />
                  <span>来源原图切片</span>
                  {sources.length > 1 && (
                    <span className="text-[11px] text-muted-foreground font-normal">
                      ({sourceIndex + 1}/{sources.length})
                    </span>
                  )}
                </div>
                {sources.length > 1 && (
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      disabled={sourceIndex === 0}
                      onClick={() => setSourceIndex((i) => Math.max(0, i - 1))}
                      className="rounded-lg px-2 py-0.5 text-xs font-medium text-muted-foreground hover:bg-surface hover:text-foreground disabled:opacity-30 border border-border/40 cursor-pointer"
                    >
                      上一张
                    </button>
                    <button
                      type="button"
                      disabled={sourceIndex >= sources.length - 1}
                      onClick={() => setSourceIndex((i) => Math.min(sources.length - 1, i + 1))}
                      className="rounded-lg px-2 py-0.5 text-xs font-medium text-muted-foreground hover:bg-surface hover:text-foreground disabled:opacity-30 border border-border/40 cursor-pointer"
                    >
                      下一张
                    </button>
                  </div>
                )}
              </div>

              <div className="flex-1 min-h-0 p-4 flex items-center justify-center overflow-auto bg-surface-secondary/15">
                <div className="relative group max-h-full max-w-full flex items-center justify-center">
                  <AuthenticatedImage
                    src={wrongImportApi.imageFileUrl(
                      sessionId,
                      currentSource?.image_id ?? draft.image_id,
                      regionParam(currentSource?.region),
                    )}
                    alt={`第 ${draft.seq_no} 题来源切图`}
                    className="max-h-[280px] md:max-h-[640px] w-auto max-w-full object-contain rounded-xl border border-border/60 shadow-subtle bg-surface"
                  />
                  <a
                    href={wrongImportApi.imageFileUrl(
                      sessionId,
                      currentSource?.image_id ?? draft.image_id,
                      regionParam(currentSource?.region),
                    )}
                    target="_blank"
                    rel="noreferrer"
                    className="absolute bottom-2.5 right-2.5 opacity-0 group-hover:opacity-100 transition-opacity bg-black/70 hover:bg-black/90 text-white rounded-lg px-2.5 py-1 text-[11px] font-medium flex items-center gap-1 backdrop-blur-xs shadow-md"
                  >
                    <Maximize2 className="h-3 w-3" />
                    <span>查看大图</span>
                  </a>
                </div>
              </div>
            </div>
          )}

          {/* 右侧（或全宽）：题目数据编辑区域 */}
          <div className="flex-1 min-h-0 overflow-y-auto p-4 sm:p-6 space-y-5.5">
            {/* Warnings 警告条 */}
            {draft.warnings && draft.warnings.length > 0 && !committed && (
              <div className="flex items-start gap-2.5 rounded-2xl bg-amber-500/10 border border-amber-500/25 px-4 py-3 shadow-2xs animate-in fade-in-50">
                <AlertTriangle className="h-4.5 w-4.5 text-amber-500 mt-0.5 shrink-0" />
                <div className="text-xs text-amber-800 dark:text-amber-300 leading-relaxed font-medium">
                  {draft.warnings.join('；')}
                </div>
              </div>
            )}

            {/* 1. 题干内容卡片 */}
            <div className="rounded-2xl border border-border/70 bg-surface p-4 sm:p-4.5 shadow-2xs space-y-2.5 transition-all focus-within:border-primary/50 focus-within:ring-2 focus-within:ring-primary/10">
              <div className="flex items-center justify-between">
                <label className="text-xs font-bold text-foreground flex items-center gap-1.5">
                  <FileQuestion className="h-4 w-4 text-primary" />
                  <span>题干内容</span>
                  <span className="text-[11px] font-normal text-muted-foreground">（修改后自动保存）</span>
                </label>
                <span className="text-[11px] font-mono text-muted-foreground bg-surface-secondary/60 px-2 py-0.5 rounded-md border border-border/40">
                  {stem.length} 字
                </span>
              </div>
              <textarea
                value={stem}
                onChange={(e) => setStem(e.target.value)}
                onBlur={saveStem}
                rows={4}
                placeholder="输入或修改题干内容..."
                className="w-full resize-y rounded-xl border border-border/60 bg-surface-secondary/25 p-3 text-xs sm:text-sm leading-relaxed text-foreground placeholder:text-muted-foreground/50 transition-all focus:border-primary/50 focus:bg-surface focus:outline-none focus:ring-1 focus:ring-primary/20"
              />
            </div>

            {/* 2. 选项与正确答案设置（客观选择/判断题） */}
            {(options.length > 0 || ['single_choice', 'multiple_choice', 'true_false'].includes(draft.question_type)) && (
              <div className="rounded-2xl border border-border/70 bg-surface p-4 sm:p-4.5 shadow-2xs space-y-3 transition-all">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-bold text-foreground flex items-center gap-1.5">
                    <Layers className="h-4 w-4 text-primary" />
                    <span>选项与正确答案设置</span>
                    <span className="text-[11px] font-normal text-muted-foreground">
                      （点击选项字母设为正确答案）
                    </span>
                  </label>
                  {['single_choice', 'multiple_choice'].includes(draft.question_type) && (
                    <button
                      type="button"
                      onClick={handleAddOption}
                      className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary-hover font-medium px-2 py-0.5 rounded-lg hover:bg-primary/5 transition-colors cursor-pointer border border-primary/20"
                    >
                      <Plus className="h-3.5 w-3.5" />
                      <span>添加选项</span>
                    </button>
                  )}
                </div>

                <div className="space-y-2">
                  {options.map((opt, index) => {
                    const isSingle = draft.question_type === 'single_choice'
                    const isMulti = draft.question_type === 'multiple_choice'
                    const isTrueFalse = draft.question_type === 'true_false'
                    const isSelected = draft.answer.includes(opt.key)
                    const canClick = isSingle || isMulti || isTrueFalse

                    const toggleAnswer = () => {
                      if (!canClick) return
                      if (isSingle || isTrueFalse) {
                        setAnswer(isSelected ? [] : [opt.key])
                      } else if (isMulti) {
                        const next = isSelected
                          ? draft.answer.filter((a) => a !== opt.key)
                          : [...draft.answer, opt.key].sort()
                        setAnswer(next)
                      }
                    }

                    return (
                      <div
                        key={opt.key || index}
                        className={`group flex items-center gap-2.5 rounded-xl p-2 sm:p-2.5 transition-all border ${
                          isSelected
                            ? 'border-primary/40 bg-primary/[0.07] ring-1 ring-primary/20 shadow-2xs'
                            : 'border-border/50 bg-surface-secondary/30 hover:border-border hover:bg-surface-secondary/50'
                        }`}
                      >
                        {/* 点击设置正确答案按钮 */}
                        <button
                          type="button"
                          onClick={toggleAnswer}
                          disabled={!canClick}
                          className={`flex h-7.5 w-7.5 shrink-0 items-center justify-center font-mono font-bold text-xs transition-all cursor-pointer ${
                            isSingle || isTrueFalse ? 'rounded-full' : 'rounded-lg'
                          } ${
                            isSelected
                              ? 'bg-primary text-primary-foreground shadow-xs scale-105'
                              : 'bg-surface text-foreground/80 border border-border/70 hover:border-primary/50 hover:text-primary hover:shadow-2xs'
                          }`}
                          title={isSelected ? '已选为正确答案（点击取消）' : '点击设为正确答案'}
                        >
                          {opt.key}
                          {isSelected && <Check className="h-3 w-3 ml-0.5 stroke-[3]" />}
                        </button>

                        {/* 选项文本编辑框 */}
                        <input
                          type="text"
                          value={opt.content}
                          onChange={(e) => handleOptionContentChange(index, e.target.value)}
                          onBlur={saveOptionContent}
                          placeholder={`输入选项 ${opt.key} 内容...`}
                          className="flex-1 bg-transparent border-0 px-2 py-1 text-xs sm:text-sm text-foreground focus:outline-none focus:bg-surface focus:ring-1 focus:ring-primary/30 rounded-lg placeholder:text-muted-foreground/40"
                        />

                        {['single_choice', 'multiple_choice'].includes(draft.question_type) && options.length > 2 && (
                          <button
                            type="button"
                            onClick={() => handleDeleteOption(index)}
                            className="opacity-0 group-hover:opacity-100 p-1 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-all rounded-md cursor-pointer"
                            title="删除该选项"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* 3. 正确答案快捷设置卡片 */}
            <div className="rounded-2xl border border-border/70 bg-surface p-4 sm:p-4.5 shadow-2xs space-y-2.5">
              <div className="flex items-center justify-between text-xs">
                <label className="font-bold text-foreground flex items-center gap-1.5">
                  <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                  <span>正确答案</span>
                  {draft.question_type === 'single_choice' && (
                    <span className="text-[11px] font-normal text-muted-foreground">（单选题·圆形）</span>
                  )}
                  {draft.question_type === 'multiple_choice' && (
                    <span className="text-[11px] font-normal text-muted-foreground">（多选题·方形）</span>
                  )}
                </label>
                {missingAnswer ? (
                  <span className="text-[11px] text-amber-600 dark:text-amber-400 font-medium flex items-center gap-1">
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-pulse" />
                    当前缺答案，请直接输入或选择
                  </span>
                ) : (
                  <span className="text-[11px] text-emerald-600 dark:text-emerald-400 font-medium flex items-center gap-1">
                    <Check className="h-3.5 w-3.5 stroke-[2.5]" />
                    已就绪（支持直接修改并保存）
                  </span>
                )}
              </div>
              <div className="p-3 sm:p-4 rounded-xl bg-surface-secondary/25 border border-border/50">
                <ModalAnswerEditor
                  draft={draft}
                  onSetAnswer={setAnswer}
                  onMarkNoAnswer={markNoAnswer}
                />
              </div>
            </div>

            {/* 4. 解析编辑卡片 */}
            <div className="rounded-2xl border border-border/70 bg-surface p-4 sm:p-4.5 shadow-2xs space-y-2.5 transition-all focus-within:border-primary/50 focus-within:ring-2 focus-within:ring-primary/10">
              <div className="flex items-center justify-between">
                <label className="text-xs font-bold text-foreground flex items-center gap-1.5">
                  <FileText className="h-4 w-4 text-primary" />
                  <span>解析内容</span>
                  <span className="text-[11px] font-normal text-muted-foreground">（可选补充）</span>
                </label>
                <span className="text-[11px] font-mono text-muted-foreground bg-surface-secondary/60 px-2 py-0.5 rounded-md border border-border/40">
                  {analysis.length} 字
                </span>
              </div>
              <textarea
                value={analysis}
                onChange={(e) => setAnalysis(e.target.value)}
                onBlur={saveAnalysis}
                rows={3}
                placeholder="输入或完善题目解析（解题思路、核心考点等）..."
                className="w-full resize-y rounded-xl border border-border/60 bg-surface-secondary/25 p-3 text-xs sm:text-sm leading-relaxed text-foreground placeholder:text-muted-foreground/50 transition-all focus:border-primary/50 focus:bg-surface focus:outline-none focus:ring-1 focus:ring-primary/20"
              />
            </div>
          </div>
        </div>

        {/* 弹窗底部操作与导航栏 */}
        <div className="flex items-center justify-between border-t border-border/70 bg-surface px-4 sm:px-6 py-3 shrink-0 z-20">
          {/* 上一题 / 下一题 快捷切换 */}
          <div className="flex items-center gap-1.5 sm:gap-2">
            <Button
              size="xs"
              variant="secondary"
              onClick={handlePrev}
              disabled={!hasPrev}
              icon={<ChevronLeft className="h-3.5 w-3.5" />}
              className="h-8.5 px-3 rounded-xl disabled:opacity-30 border border-border/60 shadow-2xs"
            >
              <span className="hidden sm:inline">上一题</span>
            </Button>
            <div className="flex items-center justify-center font-mono text-xs text-muted-foreground font-semibold px-2 sm:px-3 h-8.5 rounded-xl bg-surface-secondary/50 border border-border/40">
              <span className="text-foreground">{currentIndex >= 0 ? currentIndex + 1 : 1}</span>
              <span className="mx-1 text-muted-foreground/60">/</span>
              <span>{allDrafts.length}</span>
            </div>
            <Button
              size="xs"
              variant="secondary"
              onClick={handleNext}
              disabled={!hasNext}
              className="h-8.5 px-3 rounded-xl disabled:opacity-30 border border-border/60 shadow-2xs flex items-center gap-1"
            >
              <span className="hidden sm:inline">下一题</span>
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
            <span className="text-[11px] text-muted-foreground/60 ml-1 hidden lg:inline font-mono">
              (← / → 翻题)
            </span>
          </div>

          {/* 右侧关闭 / 置信度与完成 */}
          <div className="flex items-center gap-3">
            {draft.confidence > 0 && (
              <span className="font-mono text-xs text-muted-foreground items-center gap-1 hidden md:flex bg-surface-secondary/40 px-2.5 py-1 rounded-lg border border-border/40">
                <Sparkles className="h-3 w-3 text-primary" />
                <span>置信度 {(draft.confidence * 100).toFixed(0)}%</span>
              </span>
            )}
            <Button
              size="sm"
              variant="primary"
              onClick={onClose}
              className="h-8.5 px-5 rounded-xl font-semibold shadow-xs"
            >
              完成
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function ModalAnswerEditor({
  draft,
  onSetAnswer,
  onMarkNoAnswer,
}: {
  draft: WrongImportDraft
  onSetAnswer: (answer: string[]) => void
  onMarkNoAnswer: () => void
}) {
  const singleBtnBase =
    'h-8.5 w-8.5 rounded-full border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer flex items-center justify-center p-0'
  const multiBtnBase =
    'h-8.5 w-8.5 rounded-xl border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer flex items-center justify-center p-0'
  const trueFalseBtnBase =
    'h-8.5 px-4 rounded-full border text-xs font-bold font-mono transition-all duration-150 active:scale-95 disabled:opacity-60 disabled:pointer-events-none cursor-pointer flex items-center justify-center'

  const activeBtn =
    'border-primary bg-primary text-primary-foreground shadow-xs scale-105 ring-2 ring-primary/20'
  const idleBtn =
    'border-border/70 bg-surface text-foreground/80 hover:border-primary/40 hover:bg-primary/5 hover:text-primary shadow-2xs'

  const [fillText, setFillText] = useState(draft.answer.join('、'))
  useEffect(() => setFillText(draft.answer.join('、')), [draft.id, draft.answer])

  // 主观题文本本地状态与失焦保存
  const [localAnswerText, setLocalAnswerText] = useState(draft.answer[0] ?? '')
  useEffect(() => {
    setLocalAnswerText(draft.answer[0] ?? '')
  }, [draft.id, draft.answer])

  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // 自动调整高度
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      const newHeight = Math.max(160, Math.min(textareaRef.current.scrollHeight + 4, 450))
      textareaRef.current.style.height = `${newHeight}px`
    }
  }, [localAnswerText, draft.id])

  const handleSaveSubjectiveAnswer = () => {
    const trimmed = localAnswerText.trim()
    const orig = (draft.answer[0] ?? '').trim()
    if (trimmed !== orig) {
      onSetAnswer(trimmed ? [localAnswerText] : [])
    }
  }

  if (draft.question_type === 'single_choice') {
    const defaultOptions =
      draft.options.length > 0
        ? draft.options
        : [
            { key: 'A', content: '' },
            { key: 'B', content: '' },
            { key: 'C', content: '' },
            { key: 'D', content: '' },
          ]
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {defaultOptions.map((opt) => {
          const active = draft.answer[0] === opt.key
          return (
            <button
              key={opt.key}
              type="button"
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
        : [
            { key: 'A', content: '' },
            { key: 'B', content: '' },
            { key: 'C', content: '' },
            { key: 'D', content: '' },
          ]
    const toggle = (k: string) => {
      const cur = new Set(draft.answer)
      if (cur.has(k)) cur.delete(k)
      else cur.add(k)
      onSetAnswer(Array.from(cur).sort())
    }
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {defaultOptions.map((opt) => {
          const active = draft.answer.includes(opt.key)
          return (
            <button
              key={opt.key}
              type="button"
              onClick={() => toggle(opt.key)}
              className={`${multiBtnBase} ${active ? activeBtn : idleBtn}`}
              title={`多选 ${opt.key}${active ? '（已选）' : ''}`}
            >
              {opt.key}
            </button>
          )
        })}
        {draft.answer.length > 0 && (
          <span className="font-mono text-xs font-bold text-primary ml-2 bg-primary/10 px-2 py-0.5 rounded-md border border-primary/20">
            已选：{draft.answer.join('')}
          </span>
        )}
      </div>
    )
  }

  if (draft.question_type === 'true_false') {
    const isTrue = draft.answer[0] === '对' || draft.answer[0] === 'T'
    const isFalse = draft.answer[0] === '错' || draft.answer[0] === 'F'
    return (
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => onSetAnswer(isTrue ? [] : ['对'])}
          className={`${trueFalseBtnBase} ${isTrue ? activeBtn : idleBtn}`}
        >
          对 / 正确
        </button>
        <button
          type="button"
          onClick={() => onSetAnswer(isFalse ? [] : ['错'])}
          className={`${trueFalseBtnBase} ${isFalse ? activeBtn : idleBtn}`}
        >
          错 / 错误
        </button>
      </div>
    )
  }

  if (draft.question_type === 'fill_blank') {
    return (
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={fillText}
          onChange={(e) => setFillText(e.target.value)}
          onBlur={() => {
            const arr = fillText
              .split(/[、,，\n]/)
              .map((s) => s.trim())
              .filter(Boolean)
            if (JSON.stringify(arr) !== JSON.stringify(draft.answer)) onSetAnswer(arr)
          }}
          placeholder="输入填空答案（多个空用逗号或顿号分隔）..."
          className="flex-1 rounded-xl border border-border/60 bg-surface px-3 py-1.5 text-xs text-foreground placeholder:text-muted-foreground/50 focus:border-primary/50 focus:outline-none"
        />
        <Button
          size="xs"
          variant="secondary"
          onClick={() => {
            const arr = fillText
              .split(/[、,，\n]/)
              .map((s) => s.trim())
              .filter(Boolean)
            onSetAnswer(arr)
          }}
          className="h-8 px-3 rounded-xl cursor-pointer"
        >
          保存
        </Button>
      </div>
    )
  }

  // 主观题（简答 / 材料分析 / 论述 / 代码）
  const isNoAnswer = draft.answer_source === 'none'

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2">
          <span className="text-xs font-semibold text-foreground">参考答案 / 答题要点</span>
          {!isNoAnswer && localAnswerText && (
            <span className="text-[11px] font-mono text-muted-foreground bg-surface px-2 py-0.5 rounded-md border border-border/40">
              {localAnswerText.length} 字
            </span>
          )}
        </div>
        {!isNoAnswer ? (
          <button
            type="button"
            onClick={onMarkNoAnswer}
            className="rounded-xl border border-border/70 bg-surface px-3 py-1 text-xs font-medium text-muted-foreground hover:border-primary/40 hover:text-primary transition-all shadow-2xs cursor-pointer"
          >
            主观题无需标准答案
          </button>
        ) : (
          <button
            type="button"
            onClick={() => onSetAnswer([''])}
            className="rounded-xl border border-primary/30 bg-primary/10 px-3 py-1 text-xs font-medium text-primary hover:bg-primary/20 transition-all cursor-pointer"
          >
            已免标准答案（点击恢复输入）
          </button>
        )}
      </div>

      {!isNoAnswer && (
        <div className="space-y-2">
          <textarea
            ref={textareaRef}
            value={localAnswerText}
            onChange={(e) => setLocalAnswerText(e.target.value)}
            onBlur={handleSaveSubjectiveAnswer}
            placeholder="输入参考答案 / 核心答题要点与采分点（输入完成后点击外部或按 Tab 自动保存）..."
            style={{ minHeight: '160px' }}
            className="w-full resize-y rounded-xl border border-border/60 bg-surface p-3.5 text-xs sm:text-sm leading-relaxed text-foreground placeholder:text-muted-foreground/50 focus:border-primary/50 focus:bg-surface focus:outline-none focus:ring-1 focus:ring-primary/20 transition-all shadow-2xs"
          />
          {localAnswerText.trim() !== (draft.answer[0] ?? '').trim() && (
            <div className="flex justify-end">
              <Button
                size="xs"
                variant="primary"
                onClick={handleSaveSubjectiveAnswer}
                className="h-7 px-3 rounded-lg text-xs"
              >
                保存答案修改
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
