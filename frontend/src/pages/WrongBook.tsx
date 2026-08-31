import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  MasteryIndicator,
  Modal,
  PageHeader,
  Skeleton,
} from '@/components/ui'
import {
  AlertCircle,
  BookOpen,
  Calendar,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Clock,
  Edit3,
  Eye,
  EyeOff,
  Filter,
  Flame,
  HelpCircle,
  Lightbulb,
  ListFilter,
  Play,
  RotateCcw,
  Search,
  Sparkles,
  Star,
  Tag,
  TrendingUp,
  Zap,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { QuestionType, WrongQuestion } from '@/types'

const QUESTION_TYPE_LABELS: Record<string, string> = {
  single_choice: '单选题',
  multiple_choice: '多选题',
  true_false: '判断题',
  fill_blank: '填空题',
  short_answer: '简答题',
  material_analysis: '材料分析',
  essay: '论述题',
  code: '代码题',
}

const QUESTION_TYPE_TONES: Record<string, 'info' | 'success' | 'warning' | 'neutral' | 'primary'> = {
  single_choice: 'info',
  multiple_choice: 'primary',
  true_false: 'neutral',
  fill_blank: 'warning',
  short_answer: 'success',
  material_analysis: 'info',
  essay: 'primary',
  code: 'warning',
}

// 辅助安全格式化答案（兼容 string / array / null）
function formatAnswerDisplay(rawAnswer: unknown): string {
  if (!rawAnswer) return '暂无标准答案'
  if (Array.isArray(rawAnswer)) {
    return rawAnswer.filter(Boolean).join('、') || '暂无标准答案'
  }
  if (typeof rawAnswer === 'string') {
    return rawAnswer.trim() || '暂无标准答案'
  }
  return String(rawAnswer)
}

// 辅助安全提取知识点列表
function safeKnowledgePoints(kp: unknown): string[] {
  if (Array.isArray(kp)) return kp.filter((k) => typeof k === 'string' && k.trim())
  if (typeof kp === 'string') {
    try {
      const parsed = JSON.parse(kp)
      if (Array.isArray(parsed)) return parsed.filter((k) => typeof k === 'string' && k.trim())
    } catch {
      return kp.split(/[,，、]/).map((s) => s.trim()).filter(Boolean)
    }
  }
  return []
}

export default function WrongBook() {
  const [list, setList] = useState<WrongQuestion[]>([])
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [keyword, setKeyword] = useState('')
  const [dueOnly, setDueOnly] = useState(false)
  const [summary, setSummary] = useState({ total: 0, due: 0, mastered: 0 })
  const [reviewing, setReviewing] = useState<number | null>(null)
  const [noteDrafts, setNoteDrafts] = useState<Record<number, string>>({})
  const [savedFlag, setSavedFlag] = useState<Record<number, boolean>>({})

  // 答案折叠/自测控制：记录当前展开解析的错题 ID
  const [revealedIds, setRevealedIds] = useState<Record<number, boolean>>({})
  const [practiceModalOpen, setPracticeModalOpen] = useState(false)
  const [practiceLimit, setPracticeLimit] = useState(20)
  const [startingPractice, setStartingPractice] = useState(false)

  const navigate = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params: Record<string, unknown> = { page_size: 100 }
      if (status) params.status = status
      if (dueOnly) params.due = 'true'
      const [listResp, sumResp] = await Promise.all([
        client.get('/wrong-questions', { params }),
        client.get('/wrong-questions/summary'),
      ])
      setList(listResp.data?.data?.items || [])
      setSummary(sumResp.data?.data || { total: 0, due: 0, mastered: 0 })
    } catch (e) {
      console.error('加载错题列表失败:', e)
    } finally {
      setLoading(false)
    }
  }, [status, dueOnly])

  useEffect(() => {
    load()
  }, [load])

  // 前端多维过滤（关键字、题型）
  const filteredList = useMemo(() => {
    return list.filter((item) => {
      if (!item) return false
      const q = item.question
      if (typeFilter && q?.type !== typeFilter) {
        return false
      }
      if (keyword.trim()) {
        const kw = keyword.trim().toLowerCase()
        const matchStem = q?.stem?.toLowerCase().includes(kw)
        const matchNote = item.note?.toLowerCase().includes(kw)
        const kpList = safeKnowledgePoints(q?.knowledge_points)
        const matchKP = kpList.some((k) => k.toLowerCase().includes(kw))
        if (!matchStem && !matchNote && !matchKP) return false
      }
      return true
    })
  }, [list, typeFilter, keyword])

  const toggleReveal = (id: number) => {
    setRevealedIds((prev) => ({
      ...prev,
      [id]: !prev[id],
    }))
  }

  const review = async (w: WrongQuestion, result: 'again' | 'hard' | 'good' | 'easy') => {
    setReviewing(w.id)
    try {
      await client.post(`/wrong-questions/${w.id}/review-result`, { result })
      await load()
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setReviewing(null)
    }
  }

  const saveNote = async (w: WrongQuestion) => {
    const content = noteDrafts[w.id] ?? w.note ?? ''
    await client.put(`/questions/${w.question_id}/note`, { content })
    setSavedFlag((f) => ({ ...f, [w.id]: true }))
    setTimeout(() => setSavedFlag((f) => ({ ...f, [w.id]: false })), 1500)
    load()
  }

  const startDuePractice = async () => {
    setStartingPractice(true)
    try {
      const resp = await client.post('/practice/sessions', {
        mode: 'wrong',
        limit: practiceLimit,
      })
      navigate(`/practice/${resp.data.data.id}`)
    } catch (e) {
      alert(errMsg(e))
      setStartingPractice(false)
    }
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* 顶部标题与一键复习入口 */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-gradient-to-r from-surface via-surface to-surface-secondary/70 p-6 sm:p-7 rounded-4xl border border-border/80 shadow-card">
        <div className="space-y-1.5">
          <div className="flex items-center gap-2.5">
            <h1 className="text-2xl font-bold tracking-tight text-foreground">错题复习本</h1>
            <span className="flex items-center gap-1 text-xs px-2.5 py-0.5 rounded-full bg-primary/10 text-primary font-medium border border-primary/20">
              <Sparkles className="h-3 w-3" />
              艾宾浩斯智能间隔调度
            </span>
          </div>
          <p className="text-xs sm:text-sm text-muted-foreground max-w-2xl leading-relaxed">
            基于记忆遗忘曲线自适应计算复习周期，自动锁定薄弱考点，攻坚错因盲区。
          </p>
        </div>

        <div className="flex items-center gap-3 shrink-0">
          <Button
            onClick={() => setPracticeModalOpen(true)}
            icon={<Play className="h-4 w-4 fill-current" />}
            className="shadow-glow font-bold cursor-pointer"
          >
            开始错题专项演练
          </Button>
        </div>
      </div>

      {/* 统计指标卡片组 */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        {/* 卡片 1: 总错题 */}
        <div className="relative overflow-hidden rounded-3xl border border-border/80 bg-surface p-5 shadow-card transition-all hover:border-primary/40 group">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-muted-foreground">总错题收录</span>
            <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-primary/10 text-primary group-hover:scale-110 transition-transform">
              <BookOpen className="h-4 w-4" />
            </div>
          </div>
          <div className="mt-3 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold font-mono text-foreground tracking-tight">
              {summary.total}
            </span>
            <span className="text-xs text-muted-foreground font-medium">道题目</span>
          </div>
          <div className="mt-2 text-[11px] text-muted-foreground flex items-center gap-1">
            <span>📚 累计自动收录薄弱错题</span>
          </div>
        </div>

        {/* 卡片 2: 今日待复习 */}
        <div
          onClick={() => setDueOnly(!dueOnly)}
          className={`relative overflow-hidden rounded-3xl border p-5 shadow-card cursor-pointer transition-all group ${
            dueOnly
              ? 'border-warning bg-warning/5 ring-2 ring-warning/30'
              : 'border-border/80 bg-surface hover:border-warning/50'
          }`}
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-foreground flex items-center gap-1.5">
              <span>今日待复习</span>
              {summary.due > 0 && (
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-warning opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-warning"></span>
                </span>
              )}
            </span>
            <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-warning/10 text-warning group-hover:scale-110 transition-transform">
              <Clock className="h-4 w-4" />
            </div>
          </div>
          <div className="mt-3 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold font-mono text-warning tracking-tight">
              {summary.due}
            </span>
            <span className="text-xs text-muted-foreground font-medium">道待复现</span>
          </div>
          <div className="mt-2 text-[11px] text-muted-foreground flex items-center justify-between">
            <span>⏰ 艾宾浩斯临界阈值到达</span>
            <span className="text-primary font-medium hover:underline text-[10px]">
              {dueOnly ? '查看全部 ✕' : '点击筛选 →'}
            </span>
          </div>
        </div>

        {/* 卡片 3: 已稳固掌握 */}
        <div className="relative overflow-hidden rounded-3xl border border-border/80 bg-surface p-5 shadow-card transition-all hover:border-success/40 group">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-muted-foreground">已稳固掌握</span>
            <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-success/10 text-success group-hover:scale-110 transition-transform">
              <CheckCircle2 className="h-4 w-4" />
            </div>
          </div>
          <div className="mt-3 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold font-mono text-success tracking-tight">
              {summary.mastered}
            </span>
            <span className="text-xs text-muted-foreground font-medium">道已攻克</span>
          </div>
          <div className="mt-2 text-[11px] text-muted-foreground flex items-center gap-1">
            <span>🎉 长期记忆维持稳定高分</span>
          </div>
        </div>
      </div>

      {/* 筛选与搜索工具栏 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 rounded-3xl border border-border/80 bg-surface p-4 shadow-card">
        {/* 搜索框 */}
        <div className="relative flex-1 min-w-[240px]">
          <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            type="text"
            placeholder="搜索题干关键词、考点知识点、错题备忘笔记..."
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            className="input pl-9 w-full text-xs"
          />
        </div>

        {/* 过滤选项 */}
        <div className="flex items-center gap-2 flex-wrap">
          {/* 状态快速筛选 */}
          <div className="flex items-center bg-surface-secondary p-1 rounded-2xl border border-border/60 text-xs">
            <button
              onClick={() => setStatus('')}
              className={`px-3 py-1 rounded-xl font-medium transition-all cursor-pointer ${
                status === '' ? 'bg-surface text-foreground font-bold shadow-2xs' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              全部
            </button>
            <button
              onClick={() => setStatus('learning')}
              className={`px-3 py-1 rounded-xl font-medium transition-all cursor-pointer ${
                status === 'learning' ? 'bg-warning/20 text-warning font-bold shadow-2xs' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              学习中
            </button>
            <button
              onClick={() => setStatus('reviewing')}
              className={`px-3 py-1 rounded-xl font-medium transition-all cursor-pointer ${
                status === 'reviewing' ? 'bg-primary/20 text-primary font-bold shadow-2xs' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              复习中
            </button>
            <button
              onClick={() => setStatus('mastered')}
              className={`px-3 py-1 rounded-xl font-medium transition-all cursor-pointer ${
                status === 'mastered' ? 'bg-success/20 text-success font-bold shadow-2xs' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              已掌握
            </button>
          </div>

          {/* 题型下拉 */}
          <div className="flex items-center gap-1.5 bg-surface-secondary px-3 py-1.5 rounded-2xl border border-border/60 text-xs">
            <Filter className="h-3.5 w-3.5 text-muted-foreground" />
            <select
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              className="bg-transparent border-none text-xs text-foreground focus:outline-none cursor-pointer"
            >
              <option value="">全部题型</option>
              {Object.entries(QUESTION_TYPE_LABELS).map(([val, label]) => (
                <option key={val} value={val}>
                  {label}
                </option>
              ))}
            </select>
          </div>

          {/* 今日到期快速 Toggle */}
          <button
            onClick={() => setDueOnly(!dueOnly)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-2xl border text-xs font-medium transition-all cursor-pointer ${
              dueOnly
                ? 'bg-warning/15 border-warning/40 text-warning font-bold'
                : 'bg-surface-secondary border-border/60 text-muted-foreground hover:text-foreground'
            }`}
          >
            <Clock className="h-3.5 w-3.5" />
            <span>仅今日到期</span>
          </button>
        </div>
      </div>

      {/* 错题记录列表 */}
      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-44 rounded-4xl" />
          <Skeleton className="h-44 rounded-4xl" />
          <Skeleton className="h-44 rounded-4xl" />
        </div>
      ) : filteredList.length === 0 ? (
        <Card className="p-10">
          <EmptyState
            icon={CheckCircle2}
            title={dueOnly ? '今日复习任务已全部达标！' : '没有匹配的错题记录'}
            description={
              dueOnly
                ? '太棒了！今天到期的错题已全部消化完成，保持节奏，再接再厉。'
                : '当前筛选条件下暂无错题。在日常练习中做错的题目会自动收录至此。'
            }
            action={
              dueOnly ? (
                <Button size="sm" variant="outline" onClick={() => setDueOnly(false)}>
                  查看全部错题
                </Button>
              ) : keyword || typeFilter || status ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setKeyword('')
                    setTypeFilter('')
                    setStatus('')
                    setDueOnly(false)
                  }}
                >
                  重置筛选条件
                </Button>
              ) : (
                <Button size="sm" onClick={() => navigate('/question-banks')} className="shadow-glow">
                  去题库刷题
                </Button>
              )
            }
          />
        </Card>
      ) : (
        <div className="space-y-5">
          <div className="flex items-center justify-between px-2 text-xs text-muted-foreground">
            <span>
              当前展示 <span className="font-mono text-foreground font-bold">{filteredList.length}</span> 道错题
            </span>
            <span className="text-[11px]">💡 建议：自测回想后再展开标准答案进行效果评估</span>
          </div>

          {filteredList.map((w, idx) => {
            const isRevealed = !!revealedIds[w.id]
            const q = w.question
            const qType = (q?.type ?? 'single_choice') as QuestionType
            const kpList = safeKnowledgePoints(q?.knowledge_points)
            const answerDisplay = formatAnswerDisplay(q?.answer)

            return (
              <div
                key={w.id}
                className="rounded-4xl border border-border/80 bg-surface shadow-card p-6 sm:p-7 space-y-5 transition-all hover:border-primary/40"
              >
                {/* 顶部状态栏 */}
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 pb-4">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-surface-secondary font-mono text-xs font-bold text-muted-foreground">
                      {idx + 1}
                    </span>

                    {qType && (
                      <Badge
                        tone={QUESTION_TYPE_TONES[qType] || 'neutral'}
                        variant="subtle"
                        className="text-[10px]"
                      >
                        {QUESTION_TYPE_LABELS[qType] || qType}
                      </Badge>
                    )}

                    <Badge tone="danger" variant="subtle" className="text-[11px] font-mono font-bold">
                      错 {w.wrong_count} 次
                    </Badge>
                    <Badge tone="neutral" variant="outline" className="text-[11px] font-mono">
                      对 {w.correct_count} 次
                    </Badge>

                    <Badge
                      tone={
                        w.status === 'mastered'
                          ? 'success'
                          : w.status === 'learning'
                            ? 'warning'
                            : 'info'
                      }
                      variant="subtle"
                      className="text-[11px] font-bold"
                    >
                      {w.status === 'mastered'
                        ? '已掌握'
                        : w.status === 'learning'
                          ? '学习中'
                          : '复习中'}
                    </Badge>

                    {kpList.map((kp) => (
                      <span
                        key={kp}
                        className="text-[10px] bg-primary/10 text-primary px-2 py-0.5 rounded-md font-medium"
                      >
                        #{kp}
                      </span>
                    ))}
                  </div>

                  <div className="flex items-center gap-4 text-xs text-muted-foreground">
                    <MasteryIndicator score={w.mastery_score} size="md" />
                    {w.next_review_at && (
                      <div className="flex items-center gap-1.5 font-mono text-[11px] bg-surface-secondary px-2.5 py-1 rounded-xl border border-border/60">
                        <Calendar className="h-3.5 w-3.5 text-primary" />
                        <span>复习: {new Date(w.next_review_at).toLocaleDateString()}</span>
                      </div>
                    )}
                  </div>
                </div>

                {/* 题干内容 */}
                <div className="text-[15px] sm:text-[16px] font-medium leading-relaxed text-foreground select-text whitespace-pre-wrap">
                  {q?.stem || '（暂无题干内容）'}
                </div>

                {/* 选择题选项列表渲染 */}
                {Array.isArray(q?.options) && q.options.length > 0 && (
                  <div className="space-y-2 pt-1">
                    <div className="grid grid-cols-1 gap-2">
                      {q.options.map((opt) => {
                        let isCorrect = false
                        if (isRevealed && q?.answer) {
                          if (Array.isArray(q.answer)) {
                            isCorrect = q.answer.includes(opt.key)
                          } else if (typeof q.answer === 'string') {
                            isCorrect = (q.answer as string).includes(opt.key)
                          }
                        }

                        return (
                          <div
                            key={opt.key}
                            className={`flex items-start gap-3 p-3.5 rounded-2xl border transition-all text-xs ${
                              isCorrect
                                ? 'bg-success/10 border-success/40 text-foreground font-medium shadow-2xs'
                                : 'bg-surface-secondary/40 border-border/70 text-foreground'
                            }`}
                          >
                            <span
                              className={`flex h-5 w-5 items-center justify-center rounded-lg text-xs font-mono font-bold shrink-0 ${
                                isCorrect
                                  ? 'bg-success text-white'
                                  : 'bg-surface text-muted-foreground border border-border/80'
                              }`}
                            >
                              {opt.key}
                            </span>
                            <span className="flex-1 leading-relaxed">{opt.content}</span>
                            {isCorrect && (
                              <span className="flex items-center gap-1 text-[11px] text-success font-bold shrink-0">
                                <CheckCircle2 className="h-3.5 w-3.5" />
                                正确答案
                              </span>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}

                {/* 答案与解析折叠卡片 */}
                <div className="rounded-3xl border border-border/70 bg-surface-secondary/30 overflow-hidden transition-all">
                  <div
                    onClick={() => toggleReveal(w.id)}
                    className="flex items-center justify-between p-3.5 px-4 cursor-pointer select-none hover:bg-surface-secondary/60 transition-colors"
                  >
                    <div className="flex items-center gap-2 text-xs font-bold text-foreground">
                      {isRevealed ? (
                        <EyeOff className="h-4 w-4 text-primary" />
                      ) : (
                        <Eye className="h-4 w-4 text-primary" />
                      )}
                      <span>{isRevealed ? '收起标准答案与深度解析' : '展开标准答案与深度解析 (自我回想后点击)'}</span>
                    </div>

                    <div className="flex items-center gap-2">
                      {!isRevealed && (
                        <span className="text-[11px] text-muted-foreground bg-surface px-2 py-0.5 rounded-md border border-border">
                          点击揭晓
                        </span>
                      )}
                      {isRevealed ? (
                        <ChevronUp className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <ChevronDown className="h-4 w-4 text-muted-foreground" />
                      )}
                    </div>
                  </div>

                  {isRevealed && (
                    <div className="p-4 sm:p-5 border-t border-border/60 bg-surface/80 space-y-3.5 text-xs">
                      {/* 标准答案 */}
                      <div className="space-y-1.5">
                        <span className="font-bold text-foreground block">参考答案 / 标准答案：</span>
                        <div className="p-3 bg-success/5 border border-success/20 rounded-2xl text-foreground font-medium leading-relaxed whitespace-pre-wrap select-text">
                          {answerDisplay}
                        </div>
                      </div>

                      {/* 题目原始解析 */}
                      {q?.original_analysis && (
                        <div className="space-y-1.5">
                          <div className="flex items-center gap-1.5 text-muted-foreground font-semibold">
                            <Lightbulb className="h-3.5 w-3.5 text-warning" />
                            <span>核心解析：</span>
                          </div>
                          <div className="p-3.5 bg-surface rounded-2xl border border-border/70 text-foreground leading-relaxed whitespace-pre-wrap select-text">
                            {q.original_analysis}
                          </div>
                        </div>
                      )}

                      {/* AI 增强解析 */}
                      {q?.ai_enhanced_analysis && (
                        <div className="space-y-1.5">
                          <div className="flex items-center gap-1.5 text-primary font-bold">
                            <Sparkles className="h-3.5 w-3.5" />
                            <span>AI 归因与考点延伸：</span>
                          </div>
                          <div className="p-3.5 bg-primary/5 rounded-2xl border border-primary/20 text-foreground leading-relaxed whitespace-pre-wrap select-text">
                            {q.ai_enhanced_analysis}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {/* 记忆状态评估评估区 (Anki/FSRS 风格) */}
                <div className="rounded-3xl border border-border/70 bg-surface-secondary/20 p-4 space-y-3">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-bold text-foreground flex items-center gap-1.5">
                      <TrendingUp className="h-3.5 w-3.5 text-primary" />
                      评估本次记忆掌握程度
                    </span>
                    <span className="text-[11px] text-muted-foreground">
                      系统将依据评级自适应调度下一次复习时间
                    </span>
                  </div>

                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
                    {/* 重来 */}
                    <button
                      type="button"
                      disabled={reviewing === w.id}
                      onClick={() => review(w, 'again')}
                      className="group flex flex-col items-center justify-center p-2.5 rounded-2xl border border-destructive/30 bg-destructive/5 hover:bg-destructive/15 text-destructive transition-all active:scale-95 cursor-pointer"
                    >
                      <span className="text-xs font-bold">重来 (Again)</span>
                      <span className="text-[10px] text-destructive/70 mt-0.5 font-mono">&lt; 10 分钟</span>
                    </button>

                    {/* 困难 */}
                    <button
                      type="button"
                      disabled={reviewing === w.id}
                      onClick={() => review(w, 'hard')}
                      className="group flex flex-col items-center justify-center p-2.5 rounded-2xl border border-warning/30 bg-warning/5 hover:bg-warning/15 text-warning transition-all active:scale-95 cursor-pointer"
                    >
                      <span className="text-xs font-bold">困难 (Hard)</span>
                      <span className="text-[10px] text-warning/70 mt-0.5 font-mono">1 天后</span>
                    </button>

                    {/* 良好 */}
                    <button
                      type="button"
                      disabled={reviewing === w.id}
                      onClick={() => review(w, 'good')}
                      className="group flex flex-col items-center justify-center p-2.5 rounded-2xl border border-primary/30 bg-primary/5 hover:bg-primary/15 text-primary transition-all active:scale-95 cursor-pointer"
                    >
                      <span className="text-xs font-bold">良好 (Good)</span>
                      <span className="text-[10px] text-primary/70 mt-0.5 font-mono">3 天后</span>
                    </button>

                    {/* 简单 */}
                    <button
                      type="button"
                      disabled={reviewing === w.id}
                      onClick={() => review(w, 'easy')}
                      className="group flex flex-col items-center justify-center p-2.5 rounded-2xl border border-success/30 bg-success/5 hover:bg-success/15 text-success transition-all active:scale-95 cursor-pointer"
                    >
                      <span className="text-xs font-bold">简单 (Easy)</span>
                      <span className="text-[10px] text-success/70 mt-0.5 font-mono">7 天后</span>
                    </button>
                  </div>
                </div>

                {/* 错题原因与便签式备忘 */}
                <div className="rounded-3xl border border-border/70 bg-surface p-4 space-y-2.5 shadow-subtle">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-bold text-foreground flex items-center gap-1.5">
                      <Edit3 className="h-3.5 w-3.5 text-primary" />
                      错因归纳与速记备忘
                    </span>
                    {savedFlag[w.id] && (
                      <span className="flex items-center gap-1 text-[11px] text-success font-bold animate-fade-in">
                        <CheckCircle2 className="h-3 w-3" />
                        已同步保存
                      </span>
                    )}
                  </div>
                  <textarea
                    className="input min-h-[64px] resize-y font-mono text-xs p-3 leading-relaxed bg-surface-secondary/40 rounded-2xl border-border/70 focus:border-primary/50"
                    value={noteDrafts[w.id] ?? w.note ?? ''}
                    onChange={(e) =>
                      setNoteDrafts((d) => ({ ...d, [w.id]: e.target.value }))
                    }
                    placeholder="例如：概念混淆 / 审题遗漏关键词 / 考点陷阱在第三个条件..."
                  />
                  <div className="flex justify-end pt-0.5">
                    <Button variant="subtle" size="xs" onClick={() => saveNote(w)}>
                      保存备忘
                    </Button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* 错题专项演练弹窗 */}
      <Modal
        open={practiceModalOpen}
        onClose={() => setPracticeModalOpen(false)}
        title="开启错题专项复练"
      >
        <div className="space-y-4">
          <p className="text-xs text-muted-foreground leading-relaxed">
            系统将优先调度临近记忆遗忘曲线临界点的错题，进行强化自测。
          </p>

          <div className="space-y-2">
            <label className="text-xs font-bold text-foreground">每次演练题量</label>
            <div className="grid grid-cols-4 gap-2">
              {[10, 20, 30, 50].map((num) => (
                <button
                  key={num}
                  type="button"
                  onClick={() => setPracticeLimit(num)}
                  className={`py-2 rounded-xl text-xs font-mono font-bold border transition-all cursor-pointer ${
                    practiceLimit === num
                      ? 'bg-primary text-primary-foreground border-primary shadow-xs'
                      : 'bg-surface-secondary border-border/60 text-foreground hover:border-primary/40'
                  }`}
                >
                  {num} 题
                </button>
              ))}
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setPracticeModalOpen(false)}>
              取消
            </Button>
            <Button
              size="sm"
              onClick={startDuePractice}
              loading={startingPractice}
              className="shadow-glow font-bold"
            >
              立即生成练习卷
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
