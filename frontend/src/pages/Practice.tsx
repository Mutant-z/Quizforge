import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { useUIStore } from '@/store/ui'
import { Badge, Button, EmptyState, IconButton, Progress, ShortcutKey, Spinner } from '@/components/ui'
import AiSidebar from '@/components/AiSidebar'
import {
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronLeft,
  Clock3,
  Edit3,
  PartyPopper,
  Sparkles,
  Star,
  Timer,
  X,
  XCircle,
} from 'lucide-react'
import type { AnswerResult, PracticeSession, Question, QuestionType } from '@/types'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

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

export default function Practice() {
  const { sessionId } = useParams()
  const navigate = useNavigate()
  const [session, setSession] = useState<PracticeSession | null>(null)
  const [question, setQuestion] = useState<Question | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [textAnswer, setTextAnswer] = useState('')
  const [result, setResult] = useState<AnswerResult | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [note, setNote] = useState('')
  const [noteSaved, setNoteSaved] = useState(false)
  const [noteSaving, setNoteSaving] = useState(false)
  const [currentTime, setCurrentTime] = useState(() => Date.now())
  const [timeoutHandled, setTimeoutHandled] = useState(false)
  const { aiPanelOpen, setAiPanel, aiPanelWidth, setAiPanelWidth } = useUIStore()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const questionStartedAtRef = useRef(0)
  const [isDragging, setIsDragging] = useState(false)
  const draggingRef = useRef(false)

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    draggingRef.current = true
    setIsDragging(true)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    const onMouseMove = (moveEvent: MouseEvent) => {
      if (!draggingRef.current) return
      const newWidth = window.innerWidth - moveEvent.clientX
      setAiPanelWidth(newWidth)
    }

    const onMouseUp = () => {
      draggingRef.current = false
      setIsDragging(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }

    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }, [setAiPanelWidth])

  useEffect(() => {
    return () => {
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
  }, [])

  const loadCurrent = useCallback(async () => {
    try {
      const resp = await client.get(`/practice/sessions/${sessionId}/current-question`)
      const data = resp.data.data
      setSession(data.session)
      setQuestion(data.question)
      setResult(null)
      setSelected([])
      setTextAnswer('')
      questionStartedAtRef.current = data.question ? Date.now() : 0
      if (data.question) {
        const noteResp = await client.get(`/questions/${data.question.id}/note`).catch(() => null)
        if (noteResp?.data?.data?.content) setNote(noteResp.data.data.content)
        else setNote('')
      }
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    loadCurrent()
  }, [loadCurrent])

  useEffect(() => {
    if (!session?.expires_at || session.status !== 'active') return
    const tick = () => setCurrentTime(Date.now())
    tick()
    const timer = window.setInterval(tick, 1000)
    return () => window.clearInterval(timer)
  }, [session?.expires_at, session?.status])

  const remainingSeconds =
    session?.status === 'active' && session.expires_at
      ? Math.max(0, Math.ceil((Date.parse(session.expires_at) - currentTime) / 1000))
      : null

  useEffect(() => {
    if (remainingSeconds !== 0 || !session || session.status !== 'active' || timeoutHandled) return
    setTimeoutHandled(true)
    client
      .post(`/practice/sessions/${sessionId}/finish`)
      .then(() => loadCurrent())
      .catch(() => loadCurrent())
  }, [remainingSeconds, session, sessionId, timeoutHandled, loadCurrent])

  const toggleOption = (key: string) => {
    if (result) return
    setSelected((prev) => {
      if (question?.type === 'multiple_choice') {
        return prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]
      }
      return [key]
    })
  }

  const submit = async () => {
    if (!question || submitting || result) return
    const answer =
      question.type === 'fill_blank' ||
      question.type === 'short_answer' ||
      question.type === 'material_analysis' ||
      question.type === 'essay' ||
      question.type === 'code'
        ? [textAnswer]
        : selected

    if (answer.length === 0 || (answer.length === 1 && answer[0] === '')) return
    setSubmitting(true)
    try {
      const resp = await client.post(`/practice/sessions/${sessionId}/answer`, {
        question_id: question.id,
        answer,
        duration_ms: questionStartedAtRef.current ? Math.max(0, Date.now() - questionStartedAtRef.current) : 0,
      })
      const data = resp.data.data as AnswerResult
      setResult(data)
      // Update local session stats
      setSession((s) =>
        s
          ? {
              ...s,
              correct_count: s.correct_count + (data.correct ? 1 : 0),
              wrong_count: s.wrong_count + (data.correct ? 0 : 1),
            }
          : s
      )
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setSubmitting(false)
    }
  }

  const next = async () => {
    try {
      await client.post(`/practice/sessions/${sessionId}/next`)
      await loadCurrent()
    } catch (e) {
      alert(errMsg(e))
    }
  }

  const toggleFavorite = async () => {
    if (!question) return
    await client.post(`/questions/${question.id}/favorite`)
    setQuestion({ ...question, is_favorite: !question.is_favorite })
  }

  const saveNote = async () => {
    if (!question || noteSaving) return
    setNoteSaving(true)
    try {
      await client.put(`/questions/${question.id}/note`, { content: note })
      setNoteSaved(true)
      setTimeout(() => setNoteSaved(false), 2000)
    } finally {
      setNoteSaving(false)
    }
  }

  // Keyboard navigation
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      const isInput = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA'

      if (isInput) {
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
          submit()
        }
        return
      }

      if (result) {
        if (e.key === 'Enter' || e.key === 'ArrowRight') {
          e.preventDefault()
          next()
        }
        return
      }

      const key = e.key.toUpperCase()
      if (['A', 'B', 'C', 'D', 'E', 'F'].includes(key)) {
        e.preventDefault()
        const idx = key.charCodeAt(0) - 65
        const opt = question?.options?.[idx]
        if (opt) toggleOption(opt.key)
      } else if (['1', '2', '3', '4', '5', '6'].includes(e.key)) {
        e.preventDefault()
        const opt = question?.options?.[Number(e.key) - 1]
        if (opt) toggleOption(opt.key)
      } else if (e.key === 'Enter') {
        e.preventDefault()
        submit()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [question, result, selected, submitting, textAnswer])

  if (loading) {
    return (
      <div className="flex h-96 flex-col items-center justify-center gap-3">
        <Spinner className="h-8 w-8 text-primary" />
        <span className="text-xs text-muted-foreground">正在准备刷题上下文...</span>
      </div>
    )
  }

  // Session finished state
  if ((session?.status === 'finished' || session?.status === 'expired') && !question) {
    const accuracy =
      session.total_count > 0 ? Math.round((session.correct_count / session.total_count) * 100) : 0
    const expired = session.status === 'expired'
    return (
      <div className="mx-auto max-w-lg py-12 px-4 text-center animate-scale-in">
        <div className="rounded-4xl border border-border/80 bg-surface p-8 sm:p-10 shadow-elevated space-y-6">
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-3xl bg-primary/10 text-primary shadow-glow border border-primary/20">
              {expired ? <Timer className="h-8 w-8" /> : <PartyPopper className="h-8 w-8" />}
          </div>
          <div className="space-y-1.5">
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
              {expired ? '本次刷题时间已到' : '本次刷题已圆满完成'}
            </h1>
            <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
              {expired ? '已保留你在时间结束前完成的答题结果。' : '保持每日刷题节奏，艾宾浩斯记忆模型已根据本次答题结果更新遗忘曲线。'}
            </p>
          </div>

          <div className="grid grid-cols-3 gap-3 rounded-3xl bg-surface-secondary/70 p-5 text-center border border-border/60">
            <div>
              <div className="text-2xl sm:text-3xl font-bold text-foreground font-mono">{session.total_count}</div>
              <div className="text-xs text-muted-foreground mt-0.5">总题数</div>
            </div>
            <div>
              <div className="text-2xl sm:text-3xl font-bold text-success font-mono">{session.correct_count}</div>
              <div className="text-xs text-muted-foreground mt-0.5">做对</div>
            </div>
            <div>
              <div className="text-2xl sm:text-3xl font-bold text-primary font-mono">{accuracy}%</div>
              <div className="text-xs text-muted-foreground mt-0.5">正确率</div>
            </div>
          </div>

          <div className="flex flex-col sm:flex-row justify-center gap-3 pt-2">
            <Button size="lg" onClick={() => navigate('/question-banks')} className="flex-1 shadow-glow">
              返回题库
            </Button>
            <Button variant="outline" size="lg" onClick={() => navigate('/wrong-book')} className="flex-1">
              查看错题本
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!question) {
    return (
      <div className="mx-auto max-w-md py-12 text-center animate-fade-in">
        <EmptyState
          title="当前范围已全部刷完"
          description="你已完成了当前选定范围内的所有题目，可以开始复习错题或探索新章节。"
          action={
            <Button onClick={() => navigate('/question-banks')} className="mt-4">
              返回题库
            </Button>
          }
        />
      </div>
    )
  }

  const currentIndex = session?.current_index ?? 0
  const totalCount = session?.total_count ?? 1
  const isTextType =
    question.type === 'fill_blank' ||
    question.type === 'short_answer' ||
    question.type === 'material_analysis' ||
    question.type === 'essay' ||
    question.type === 'code'

  return (
    <div className="flex h-full gap-6 items-start animate-fade-in">
      {/* Primary Quiz Workspace */}
      <div className="min-w-0 flex-1 space-y-4">
        {/* Top Minimal Focus Header */}
        <div className="flex items-center justify-between gap-4 rounded-3xl border border-border/80 bg-surface/90 backdrop-blur-md px-5 py-3 shadow-subtle">
          <div className="flex items-center gap-3 min-w-0">
            <button
              onClick={() => navigate('/question-banks')}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0"
              title="退出刷题"
            >
              <ChevronLeft className="h-4 w-4" />
              <span className="hidden sm:inline">题库</span>
            </button>
            <div className="h-4 w-px bg-border shrink-0" />
            <div className="flex items-center gap-2 min-w-0 truncate text-xs">
              <Badge tone="primary" variant="subtle" className="text-[10px] font-semibold">
                {typeLabels[question.type]}
              </Badge>
              <span className="text-muted-foreground text-[11px] font-mono">
                难度 {question.difficulty}/5
              </span>
              {question.knowledge_points?.slice(0, 2).map((kp) => (
                <Badge key={kp} tone="neutral" variant="outline" className="text-[10px] hidden md:inline-flex">
                  {kp}
                </Badge>
              ))}
            </div>
          </div>

            <div className="flex items-center gap-3 shrink-0">
            {remainingSeconds !== null && (
              <div
                className={`flex items-center gap-1.5 rounded-xl border px-2.5 py-1.5 text-xs font-mono font-bold ${
                  remainingSeconds <= 60
                    ? 'border-destructive/35 bg-destructive/10 text-destructive animate-pulse-subtle'
                    : 'border-primary/20 bg-primary/10 text-primary'
                }`}
                title="本次刷题剩余时间"
              >
                <Clock3 className="h-3.5 w-3.5" />
                {Math.floor(remainingSeconds / 60).toString().padStart(2, '0')}:{(remainingSeconds % 60).toString().padStart(2, '0')}
              </div>
            )}
            {/* Progress counter & Slim bar */}
            <div className="flex items-center gap-2.5">
              <span className="font-mono text-xs font-bold text-foreground">
                {currentIndex + 1}
                <span className="text-muted-foreground/60 font-normal"> / {totalCount}</span>
              </span>
              <div className="w-16 sm:w-28">
                <Progress value={currentIndex + 1} max={Math.max(1, totalCount)} />
              </div>
            </div>

            {/* Favorite button */}
            <IconButton
              variant="ghost"
              size="xs"
              onClick={toggleFavorite}
              title={question.is_favorite ? '取消收藏' : '收藏本题'}
              className="text-muted-foreground hover:text-amber-500 hover:scale-105 transition-transform"
            >
              <Star
                className={`h-4 w-4 transition-colors ${
                  question.is_favorite ? 'fill-amber-400 text-amber-400' : ''
                }`}
              />
            </IconButton>

            {/* AI Toggle Button */}
            <Button
              variant={aiPanelOpen ? 'subtle' : 'outline'}
              size="xs"
              onClick={() => setAiPanel(!aiPanelOpen)}
              icon={<Sparkles className="h-3.5 w-3.5 text-primary" />}
              className="text-xs shadow-subtle"
            >
              <span className="hidden sm:inline">AI 答疑</span>
            </Button>
          </div>
        </div>

        {/* Question & Options Focus Card */}
        <div className="rounded-4xl border border-border bg-surface p-6 sm:p-8 shadow-card space-y-6">
          {/* Question Stem */}
          <div className="text-[16px] sm:text-[17px] font-medium leading-relaxed text-foreground tracking-tight markdown-body select-text">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{question.stem}</ReactMarkdown>
          </div>

          {/* Option Choices */}
          {!isTextType && (
            <div className="space-y-3 pt-1">
              {question.options?.map((opt, idx) => {
                const isSelected = selected.includes(opt.key)
                const isCorrectAnswer = result ? result.correct_answer.includes(opt.key) : false
                const isWrongSelection = result && isSelected && !isCorrectAnswer
                const optionKeyLetter = String.fromCharCode(65 + idx)

                let itemClass =
                  'border-border/80 bg-surface hover:bg-surface-secondary/70 hover:border-primary/40 hover:-translate-y-0.5'
                let badgeClass = 'border-border/80 bg-surface-secondary text-muted-foreground'

                if (result) {
                  if (isCorrectAnswer) {
                    itemClass =
                      'border-success/60 bg-success/10 text-foreground font-medium ring-1 ring-success/40'
                    badgeClass = 'border-success bg-success text-white'
                  } else if (isWrongSelection) {
                    itemClass =
                      'border-destructive/60 bg-destructive/10 text-foreground ring-1 ring-destructive/40 animate-micro-shake'
                    badgeClass = 'border-destructive bg-destructive text-white'
                  } else {
                    itemClass = 'border-border/40 bg-surface opacity-50 cursor-default'
                  }
                } else if (isSelected) {
                  itemClass =
                    'border-primary bg-primary/5 text-foreground font-medium ring-1 ring-primary/40 shadow-subtle'
                  badgeClass = 'border-primary bg-primary text-primary-foreground'
                }

                return (
                  <button
                    key={opt.key}
                    type="button"
                    onClick={() => toggleOption(opt.key)}
                    disabled={!!result}
                    className={`group relative flex w-full items-start gap-4 rounded-2xl border p-4 text-left text-sm transition-all duration-150 active:scale-[0.99] disabled:active:scale-100 shadow-2xs ${itemClass}`}
                  >
                    {/* Option Key Tag / Letter */}
                    <span
                      className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-xl border text-xs font-bold font-mono tracking-tight transition-colors shadow-2xs ${badgeClass}`}
                    >
                      {opt.key || optionKeyLetter}
                    </span>

                    {/* Option Content Text */}
                    <span className="flex-1 pt-0.5 leading-relaxed text-[14px]">
                      {opt.content}
                    </span>

                    {/* Result icon indicators */}
                    {result && isCorrectAnswer && (
                      <div className="animate-check-scale shrink-0 mt-0.5 text-success">
                        <CheckCircle2 className="h-5 w-5 fill-current/10" />
                      </div>
                    )}
                    {result && isWrongSelection && (
                      <div className="shrink-0 mt-0.5 text-destructive">
                        <XCircle className="h-5 w-5 fill-current/10" />
                      </div>
                    )}
                  </button>
                )
              })}
            </div>
          )}

          {/* Text/Fill-in Answer Form */}
          {isTextType && (
            <div className="pt-1">
              <textarea
                ref={textareaRef}
                className="input min-h-[120px] resize-y font-mono text-sm leading-relaxed p-4 rounded-2xl"
                placeholder={
                  question.type === 'fill_blank'
                    ? '请输入填空答案...'
                    : '请输入您的解答分析与推导（支持 Markdown）...'
                }
                value={textAnswer}
                onChange={(e) => setTextAnswer(e.target.value)}
                disabled={!!result}
              />
            </div>
          )}

          {/* Submit Action Bar */}
          {!result && (
            <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-4 border-t border-border/60">
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="hidden sm:inline">快捷键：</span>
                <ShortcutKey keys={['A-D', '1-4']} />
                <span className="hidden sm:inline">选择</span>
                <ShortcutKey keys={['Enter']} />
                <span className="hidden sm:inline">提交</span>
              </div>

              <Button
                onClick={submit}
                disabled={submitting || (isTextType ? !textAnswer.trim() : selected.length === 0)}
                loading={submitting}
                className="w-full sm:w-auto px-6 shadow-glow"
                shortcut="Enter"
              >
                提交答案
              </Button>
            </div>
          )}

          {/* Answer Feedback & Analysis Expansion */}
          {result && (
            <div className="space-y-5 pt-4 border-t border-border/80 animate-slide-down">
              {/* Result Status Banner */}
              <div
                className={`flex items-center justify-between gap-4 rounded-3xl border p-4 sm:p-5 shadow-subtle ${
                  result.correct
                    ? 'border-success/30 bg-success/5 text-success'
                    : 'border-destructive/30 bg-destructive/5 text-destructive'
                }`}
              >
                <div className="flex items-center gap-3.5">
                  {result.correct ? (
                    <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-success/15 text-success animate-check-scale">
                      <Check className="h-5 w-5 stroke-[2.5]" />
                    </div>
                  ) : (
                    <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-destructive/15 text-destructive">
                      <X className="h-5 w-5 stroke-[2.5]" />
                    </div>
                  )}
                  <div>
                    <div className="text-sm font-bold tracking-tight">
                      {result.correct ? '回答正确' : '回答错误'}
                    </div>
                    <div className="text-xs text-muted-foreground mt-0.5">
                      正确答案：<span className="font-bold text-foreground font-mono">{result.correct_answer.join('、')}</span>
                      {result.wrong_added && (
                        <span className="ml-2 inline-flex items-center text-destructive font-semibold">
                          · 已自动加入错题本
                        </span>
                      )}
                    </div>
                  </div>
                </div>

                <Button
                  onClick={next}
                  icon={<ArrowRight className="h-4 w-4" />}
                  className="shrink-0 shadow-glow"
                  shortcut="Enter"
                >
                  下一题
                </Button>
              </div>

              {/* Comprehensive Analysis Text */}
              {result.analysis && (
                <div className="rounded-3xl border border-border/80 bg-surface-secondary/40 p-5 sm:p-6 space-y-2.5">
                  <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-muted-foreground">
                    <BookOpen className="h-4 w-4 text-primary" />
                    <span>题目官方解析</span>
                  </div>
                  <div className="text-sm leading-relaxed markdown-body select-text">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{result.analysis}</ReactMarkdown>
                  </div>
                </div>
              )}

              {/* Notion-Style Personal Learning Note Editor */}
              <div className="rounded-3xl border border-border/80 bg-surface p-5 space-y-3 shadow-card">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 text-xs font-bold text-foreground">
                    <Edit3 className="h-4 w-4 text-primary" />
                    <span>个人错因与要点备忘</span>
                  </div>
                  {noteSaved && (
                    <Badge tone="success" variant="subtle" className="text-[10px]">
                      笔记已保存
                    </Badge>
                  )}
                </div>
                <textarea
                  className="input min-h-[80px] resize-y font-mono text-xs p-3.5 leading-relaxed bg-surface-secondary/40 rounded-2xl"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="记录你对这道题的思路陷阱、直觉、记忆口诀与复习备忘..."
                />
                <div className="flex justify-end pt-0.5">
                  <Button
                    variant="outline"
                    size="xs"
                    onClick={saveNote}
                    loading={noteSaving}
                  >
                    保存笔记
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* AI Assistant Copilot Sidebar Panel with Smooth Resize & Transitions */}
      <aside
        style={{ width: aiPanelOpen ? `${aiPanelWidth}px` : '0px' }}
        className={`relative shrink-0 flex flex-col h-full bg-surface border-l ${
          aiPanelOpen ? 'border-border/80 opacity-100' : 'border-transparent opacity-0 pointer-events-none'
        } ${isDragging ? '' : 'transition-all duration-300 ease-in-out'} overflow-hidden z-20`}
      >
        {/* Draggable Resize Handle */}
        {aiPanelOpen && (
          <div
            onMouseDown={handleMouseDown}
            className="absolute left-0 top-0 bottom-0 w-2 -ml-1 cursor-col-resize hover:bg-primary/40 active:bg-primary transition-colors z-30 group flex items-center justify-center select-none"
            title="拖拽调整侧边栏宽度"
          >
            <div className="w-0.5 h-8 rounded-full bg-border group-hover:bg-primary group-active:bg-primary transition-colors shadow-glow" />
          </div>
        )}

        {/* Inner Fixed-Width Viewport for Ultra Smooth Slide & Clip Transition */}
        <div
          style={{ width: `${aiPanelWidth}px` }}
          className="h-full flex flex-col shrink-0 overflow-hidden"
        >
          <AiSidebar
            question={question}
            result={result}
            session={session}
            onClose={() => setAiPanel(false)}
          />
        </div>
      </aside>
    </div>
  )
}
