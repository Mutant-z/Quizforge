import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Progress, Skeleton, Spinner } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobileAiSheet } from '@/components/mobile/MobileAiSheet'
import { showToast } from '@/components/mobile/MobileToast'
import type { AnswerResult, PracticeSession, Question, QuestionOption, QuestionType } from '@/types'
import {
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronLeft,
  Clock3,
  Edit3,
  Flame,
  Grid,
  HelpCircle,
  Lightbulb,
  PartyPopper,
  Play,
  RotateCcw,
  Sparkles,
  Star,
  Timer,
  X,
  XCircle,
} from 'lucide-react'
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

export default function MobilePractice() {
  const { sessionId } = useParams()
  const navigate = useNavigate()
  const [session, setSession] = useState<PracticeSession | null>(null)
  const [question, setQuestion] = useState<Question | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [textAnswer, setTextAnswer] = useState('')
  const [result, setResult] = useState<AnswerResult | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [finished, setFinished] = useState(false)
  const [currentTime, setCurrentTime] = useState(() => Date.now())
  const [timeoutHandled, setTimeoutHandled] = useState(false)
  const questionStartedAtRef = useRef(Date.now())

  // Notes state
  const [note, setNote] = useState('')
  const [noteOpen, setNoteOpen] = useState(false)
  const [savingNote, setSavingNote] = useState(false)

  // AI sheet
  const [aiSheetOpen, setAiSheetOpen] = useState(false)

  // Jump grid sheet
  const [jumpOpen, setJumpOpen] = useState(false)

  // Favorite toggle
  const [isFavorite, setIsFavorite] = useState(false)

  const loadCurrent = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get(`/practice/sessions/${sessionId}/current-question`)
      const data = resp.data.data
      setSession(data.session)
      setQuestion(data.question)
      setResult(null)
      setSelected([])
      setTextAnswer('')
      setIsFavorite(data.question?.is_favorite ?? false)
      questionStartedAtRef.current = Date.now()

      if (!data.question) {
        setFinished(true)
      } else {
        const noteResp = await client.get(`/questions/${data.question.id}/note`).catch(() => null)
        if (noteResp?.data?.data?.content) setNote(noteResp.data.data.content)
        else setNote('')
      }
    } catch {
      setFinished(true)
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    loadCurrent()
  }, [loadCurrent])

  useEffect(() => {
    if (!session?.expires_at || session.status !== 'active' || finished) return
    const deadline = new Date(session.expires_at).getTime()
    const tick = () => {
      const now = Date.now()
      setCurrentTime(now)
      if (!timeoutHandled && now >= deadline) {
        setTimeoutHandled(true)
        void client
          .post(`/practice/sessions/${sessionId}/finish`)
          .then((response) => {
            setSession(response.data.data as PracticeSession)
            setFinished(true)
          })
          .catch(() => setFinished(true))
      }
    }
    tick()
    const timer = window.setInterval(tick, 1000)
    return () => window.clearInterval(timer)
  }, [session, sessionId, finished, timeoutHandled])

  const toggleOption = (key: string) => {
    if (result) return
    setSelected((prev) => {
      if (question?.type === 'multiple_choice') {
        return prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]
      }
      return [key]
    })
  }

  const submitAnswer = async () => {
    if (!question || submitting || result) return
    const answer =
      question.type === 'fill_blank' ||
      question.type === 'short_answer' ||
      question.type === 'material_analysis' ||
      question.type === 'essay' ||
      question.type === 'code'
        ? [textAnswer]
        : selected

    if (answer.length === 0 || (answer.length === 1 && !answer[0].trim())) {
      showToast('请先选择或填写答案', 'warning')
      return
    }

    setSubmitting(true)
    try {
      const resp = await client.post(`/practice/sessions/${sessionId}/answer`, {
        question_id: question.id,
        answer,
        duration_ms: Math.max(0, Date.now() - questionStartedAtRef.current),
      })
      const data = resp.data.data as AnswerResult
      setResult(data)
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
      showToast(errMsg(e), 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const nextQuestion = async () => {
    if (!session || !question) return
    try {
      const response = await client.post(`/practice/sessions/${session.id}/next`)
      const nextSession = response.data.data as PracticeSession
      setSession(nextSession)
      if (nextSession.status === 'finished' || nextSession.current_index >= nextSession.total_count) {
        setFinished(true)
        return
      }
      await loadCurrent()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const toggleFavorite = async () => {
    if (!question) return
    try {
      const next = !isFavorite
      setIsFavorite(next)
      await client.post(`/questions/${question.id}/favorite`)
      showToast(next ? '已加入收藏本' : '已取消收藏', 'info')
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const saveNote = async () => {
    if (!question) return
    setSavingNote(true)
    try {
      await client.put(`/questions/${question.id}/note`, { content: note })
      showToast('笔记保存成功', 'success')
      setNoteOpen(false)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setSavingNote(false)
    }
  }

  // Finished Practice Report Screen
  if (finished) {
    const total = session?.total_count || 1
    const correct = session?.correct_count || 0
    const wrong = session?.wrong_count || 0
    const accuracy = Math.round((correct / total) * 100)

    return (
      <div className="min-h-screen flex flex-col justify-between p-5 pt-8 bg-background">
        <div className="space-y-6 text-center">
          <div className="mx-auto flex h-20 w-20 items-center justify-center rounded-3xl bg-primary/10 text-primary shadow-glow border border-primary/20 animate-scale-in">
            {session?.status === 'expired' ? <Timer className="h-10 w-10" /> : <PartyPopper className="h-10 w-10 animate-bounce" />}
          </div>

          <div className="space-y-1">
            <h2 className="text-2xl font-bold tracking-tight text-foreground">{session?.status === 'expired' ? '本次刷题时间已到' : '本次刷题已完成！'}</h2>
            <p className="text-xs text-muted-foreground">{session?.status === 'expired' ? '已保留此前完成的题目结果' : '答题数据已同步至个人知识图谱'}</p>
          </div>

          {/* Stats summary */}
          <div className="rounded-3xl border border-border/80 bg-surface p-6 shadow-card space-y-4">
            <div className="flex items-center justify-center">
              <div className="text-center">
                <span className="text-4xl font-bold font-mono text-primary">{accuracy}%</span>
                <span className="block text-xs font-semibold text-muted-foreground mt-1">本轮综合正确率</span>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-2 border-t border-border/60 pt-4 text-center font-mono">
              <div className="rounded-xl bg-surface-secondary/70 p-2.5">
                <span className="text-lg font-bold text-foreground">{total}</span>
                <span className="block text-[10px] text-muted-foreground font-sans mt-0.5">总题数</span>
              </div>
              <div className="rounded-xl bg-emerald-500/10 p-2.5">
                <span className="text-lg font-bold text-emerald-600 dark:text-emerald-400">{correct}</span>
                <span className="block text-[10px] text-emerald-600/80 font-sans mt-0.5">正确</span>
              </div>
              <div className="rounded-xl bg-destructive/10 p-2.5">
                <span className="text-lg font-bold text-destructive">{wrong}</span>
                <span className="block text-[10px] text-destructive/80 font-sans mt-0.5">错题</span>
              </div>
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="space-y-2.5 pt-6 pb-4">
          {wrong > 0 && (
            <Button
              size="lg"
              onClick={() => navigate('/wrong-book')}
              icon={<RotateCcw className="h-4 w-4" />}
              className="w-full h-12 rounded-2xl font-bold shadow-glow"
            >
              立即复习错题 ({wrong})
            </Button>
          )}
          <Button
            variant="outline"
            size="lg"
            onClick={() => navigate(session?.bank_id ? `/question-bank/${session.bank_id}` : '/question-banks')}
            className="w-full h-12 rounded-2xl font-bold"
          >
            返回题库空间
          </Button>
        </div>
      </div>
    )
  }

  const currentIndex = (session?.current_index ?? 0) + 1
  const totalCount = session?.total_count ?? 1
  const remainingSeconds = session?.expires_at
    ? Math.max(0, Math.ceil((new Date(session.expires_at).getTime() - currentTime) / 1000))
    : null
  const remainingLabel = remainingSeconds === null
    ? null
    : `${Math.floor(remainingSeconds / 60).toString().padStart(2, '0')}:${(remainingSeconds % 60).toString().padStart(2, '0')}`
  const isOptionType =
    question?.type === 'single_choice' ||
    question?.type === 'multiple_choice' ||
    question?.type === 'true_false'

  let optionsList: QuestionOption[] = []
  if (question?.options) {
    try {
      optionsList = typeof question.options === 'string' ? JSON.parse(question.options) : question.options
    } catch {
      optionsList = []
    }
  }

  return (
    <div className="flex flex-col h-full bg-background select-none">
      <MobileAiSheet
        open={aiSheetOpen}
        onClose={() => setAiSheetOpen(false)}
        question={question}
        result={result}
        session={session}
      />

      {/* Top Header Bar */}
      <header className="sticky top-0 z-30 flex items-center justify-between px-4 py-3 border-b border-border/60 bg-surface/90 backdrop-blur-xl">
        <div className="flex items-center gap-2">
          <button
            onClick={() => navigate(session?.bank_id ? `/question-bank/${session.bank_id}` : '/question-banks')}
            className="flex h-8 w-8 items-center justify-center rounded-xl bg-surface-secondary text-foreground active:scale-95"
            aria-label="退出答题"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <div className="flex items-center gap-1.5 font-mono text-xs">
            <span className="font-bold text-foreground text-sm">{currentIndex}</span>
            <span className="text-muted-foreground">/</span>
            <span className="text-muted-foreground">{totalCount}</span>
          </div>
        </div>

        {remainingLabel && (
          <div className={`flex items-center gap-1 rounded-xl px-2.5 py-1 text-xs font-mono font-bold ${remainingSeconds !== null && remainingSeconds <= 60 ? 'bg-destructive/10 text-destructive' : 'bg-primary/10 text-primary'}`}>
            {remainingSeconds !== null && remainingSeconds <= 60 ? <Clock3 className="h-3.5 w-3.5" /> : <Timer className="h-3.5 w-3.5" />}
            {remainingLabel}
          </div>
        )}

        {/* Action icons: Star & AI Copilot */}
        <div className="flex items-center gap-2">
          <button
            onClick={toggleFavorite}
            className={`flex h-8 w-8 items-center justify-center rounded-xl transition-all active:scale-90 ${
              isFavorite
                ? 'bg-amber-500/15 text-amber-500 shadow-glow-amber'
                : 'bg-surface-secondary text-muted-foreground'
            }`}
            aria-label="收藏题目"
          >
            <Star className={`h-4 w-4 ${isFavorite ? 'fill-current' : ''}`} />
          </button>

          <button
            onClick={() => setAiSheetOpen(true)}
            className="flex items-center gap-1 rounded-xl bg-primary/10 border border-primary/25 px-2.5 py-1 text-xs font-bold text-primary active:scale-95 shadow-glow"
          >
            <Sparkles className="h-3.5 w-3.5 animate-pulse-subtle" />
            <span>AI 助教</span>
          </button>
        </div>
      </header>

      {/* Progress Bar */}
      <Progress value={currentIndex} max={totalCount} className="h-1 rounded-none" />

      {/* Scrollable Question Content Area */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 touch-pan-y pb-28 min-h-0">
        {loading ? (
          <div className="space-y-4">
            <Skeleton className="h-28 rounded-3xl" />
            <Skeleton className="h-12 rounded-2xl" />
            <Skeleton className="h-12 rounded-2xl" />
          </div>
        ) : question ? (
          <>
            {/* Question Card */}
            <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3">
              <div className="flex items-center justify-between">
                <Badge tone="primary" variant="subtle" className="text-[10px] font-bold">
                  {typeLabels[question.type] || question.type}
                </Badge>
                {question.knowledge_points && question.knowledge_points.length > 0 && (
                  <span className="rounded-md bg-surface-secondary px-2 py-0.5 text-[10px] text-muted-foreground font-medium truncate max-w-[160px]">
                    {question.knowledge_points[0]}
                  </span>
                )}
              </div>

              {/* Stem */}
              <div className="text-sm font-medium text-foreground leading-relaxed markdown-body">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{question.stem}</ReactMarkdown>
              </div>
            </div>

            {/* Choice Options */}
            {isOptionType && (
              <div className="space-y-2.5">
                {optionsList.map((opt) => {
                  const isSelected = selected.includes(opt.key)
                  const isStandard =
                    result &&
                    (Array.isArray(result.correct_answer)
                      ? result.correct_answer.includes(opt.key)
                      : result.correct_answer === opt.key)

                  let optionStyle = 'border-border/80 bg-surface text-foreground active:scale-[0.98]'
                  if (result) {
                    if (isStandard) {
                      optionStyle = 'border-emerald-500 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-bold shadow-glow-success'
                    } else if (isSelected && !result.correct) {
                      optionStyle = 'border-destructive bg-destructive/10 text-destructive font-bold'
                    } else {
                      optionStyle = 'border-border/60 bg-surface/50 text-muted-foreground opacity-60'
                    }
                  } else if (isSelected) {
                    optionStyle = 'border-primary bg-primary/10 text-primary font-bold shadow-subtle ring-1 ring-primary/40'
                  }

                  return (
                    <button
                      key={opt.key}
                      type="button"
                      disabled={!!result}
                      onClick={() => toggleOption(opt.key)}
                      className={`flex w-full items-start gap-3 rounded-2xl border p-3.5 text-left text-xs transition-all duration-150 ${optionStyle}`}
                    >
                      <div
                        className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-xl font-mono text-xs font-bold transition-transform ${
                          isSelected || isStandard
                            ? 'bg-primary text-white scale-105'
                            : 'bg-surface-secondary text-muted-foreground'
                        }`}
                      >
                        {opt.key}
                      </div>
                      <div className="flex-1 pt-0.5 leading-relaxed font-normal">{opt.content}</div>
                      {result && isStandard && <Check className="h-4 w-4 text-emerald-500 shrink-0 mt-0.5" />}
                      {result && isSelected && !result.correct && (
                        <X className="h-4 w-4 text-destructive shrink-0 mt-0.5" />
                      )}
                    </button>
                  )
                })}
              </div>
            )}

            {/* Non-choice text input for fill_blank, short_answer, essay, code */}
            {!isOptionType && (
              <div className="space-y-2">
                <textarea
                  className="input min-h-[120px] rounded-2xl p-3.5 text-xs resize-none"
                  placeholder="在此输入你的作答内容..."
                  value={textAnswer}
                  disabled={!!result}
                  onChange={(e) => setTextAnswer(e.target.value)}
                />
              </div>
            )}

            {/* Instant Answer Feedback Card */}
            {result && (
              <div
                className={`rounded-3xl border p-4 space-y-3 animate-slide-up shadow-elevated ${
                  result.correct
                    ? 'border-emerald-500/30 bg-emerald-500/[0.06]'
                    : 'border-destructive/30 bg-destructive/[0.06]'
                }`}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    {result.correct ? (
                      <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    ) : (
                      <XCircle className="h-5 w-5 text-destructive" />
                    )}
                    <span className={`text-sm font-bold ${result.correct ? 'text-emerald-500' : 'text-destructive'}`}>
                      {result.correct ? '回答正确！' : '回答错误'}
                    </span>
                  </div>
                  <button
                    onClick={() => setNoteOpen(true)}
                    className="flex items-center gap-1 text-[11px] font-bold text-muted-foreground hover:text-foreground"
                  >
                    <Edit3 className="h-3 w-3" />
                    <span>{note ? '修改笔记' : '添加笔记'}</span>
                  </button>
                </div>

                {/* Standard answer comparison */}
                <div className="rounded-2xl bg-surface p-3 space-y-1 text-xs border border-border/60">
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground font-semibold">标准答案：</span>
                    <span className="font-mono font-bold text-emerald-600 dark:text-emerald-400">
                      {Array.isArray(result.correct_answer)
                        ? result.correct_answer.join(', ')
                        : String(result.correct_answer || '无')}
                    </span>
                  </div>
                  {!result.correct && (
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-muted-foreground">你的作答：</span>
                      <span className="font-mono text-destructive">
                        {Array.isArray(result.user_answer)
                          ? result.user_answer.join(', ')
                          : String(result.user_answer || '未作答')}
                      </span>
                    </div>
                  )}
                </div>

                {/* Analysis / Explanation */}
                {(question.original_analysis || question.ai_enhanced_analysis || result.analysis) && (
                  <div className="space-y-1 pt-1">
                    <span className="text-[11px] font-bold text-foreground flex items-center gap-1">
                      <Lightbulb className="h-3 w-3 text-amber-500" />
                      <span>试题解析：</span>
                    </span>
                    <div className="rounded-2xl bg-surface p-3 text-xs text-muted-foreground leading-relaxed border border-border/60 markdown-body">
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>
                        {question.original_analysis || question.ai_enhanced_analysis || result.analysis || ''}
                      </ReactMarkdown>
                    </div>
                  </div>
                )}

                {/* AI Deep Analysis Prompt */}
                <button
                  onClick={() => setAiSheetOpen(true)}
                  className="w-full flex items-center justify-center gap-2 rounded-2xl border border-primary/20 bg-primary/10 py-2.5 text-xs font-bold text-primary active:scale-[0.98]"
                >
                  <Sparkles className="h-4 w-4" />
                  <span>让 AI 助教深入剖析考点</span>
                </button>
              </div>
            )}
          </>
        ) : null}
      </div>

      {/* Fixed Bottom Action Bar */}
      <footer className="fixed bottom-0 inset-x-0 z-30 border-t border-border/70 bg-surface/95 backdrop-blur-xl p-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)] shadow-float">
        <div className="flex items-center gap-3">
          {/* Answer Submit OR Next Question */}
          {!result ? (
            <Button
              size="lg"
              loading={submitting}
              onClick={submitAnswer}
              className="flex-1 h-12 rounded-2xl font-bold shadow-glow text-sm"
            >
              提交答案
            </Button>
          ) : (
            <Button
              size="lg"
              onClick={nextQuestion}
              icon={<ArrowRight className="h-4 w-4" />}
              className="flex-1 h-12 rounded-2xl font-bold shadow-glow text-sm"
            >
              {currentIndex >= totalCount ? '查看刷题总结' : '下一题'}
            </Button>
          )}
        </div>
      </footer>

      {/* Note Editor Bottom Sheet */}
      <MobileBottomSheet
        open={noteOpen}
        onClose={() => setNoteOpen(false)}
        title="学习笔记"
        description="记录本题的考点总结或记忆口诀"
      >
        <div className="space-y-4">
          <textarea
            className="input min-h-[140px] rounded-2xl p-3 text-xs resize-none"
            placeholder="写下你的解题思路或心得体会..."
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setNoteOpen(false)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button loading={savingNote} onClick={saveNote} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              保存笔记
            </Button>
          </div>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
