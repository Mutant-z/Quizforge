import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import PracticeConfigModal, { type PracticeLaunchTarget } from '@/components/practice/PracticeConfigModal'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  MasteryIndicator,
  Modal,
  Skeleton,
} from '@/components/ui'
import {
  AlertTriangle,
  BookOpen,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  FileQuestion,
  Filter,
  FolderTree,
  GraduationCap,
  Layers,
  Lightbulb,
  ListChecks,
  ListOrdered,
  Pencil,
  Play,
  Plus,
  Save,
  Search,
  Sparkles,
  Star,
  Trash2,
} from 'lucide-react'
import type { Chapter, Question, QuestionBank, QuestionOption, QuestionType, Subject } from '@/types'

const QUESTION_TYPE_LABELS: Record<QuestionType, string> = {
  single_choice: '单选题',
  multiple_choice: '多选题',
  true_false: '判断题',
  fill_blank: '填空题',
  short_answer: '简答题',
  material_analysis: '材料分析',
  essay: '论述题',
  code: '代码题',
}

const QUESTION_TYPE_TONES: Record<QuestionType, 'info' | 'success' | 'warning' | 'neutral' | 'primary'> = {
  single_choice: 'info',
  multiple_choice: 'primary',
  true_false: 'neutral',
  fill_blank: 'warning',
  short_answer: 'success',
  material_analysis: 'info',
  essay: 'primary',
  code: 'warning',
}

const OPTION_QUESTION_TYPES: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false']

function isOptionQuestion(type: QuestionType) {
  return OPTION_QUESTION_TYPES.includes(type)
}

function defaultOptions(type: QuestionType = 'single_choice'): QuestionOption[] {
  if (type === 'true_false') {
    return [
      { key: 'A', content: '正确' },
      { key: 'B', content: '错误' },
    ]
  }
  return ['A', 'B', 'C', 'D'].map((key) => ({ key, content: '' }))
}

function parseAnswerText(value: string) {
  return value
    .split(/[\n,，、]/)
    .map((answer) => answer.trim())
    .filter(Boolean)
}

export default function BankDetail() {
  const { id } = useParams()
  const bankID = Number(id)
  const navigate = useNavigate()

  // Tab 状态: 'questions' (题目明细) | 'chapters' (章节大纲)
  const [activeTab, setActiveTab] = useState<'questions' | 'chapters'>('questions')

  const [bank, setBank] = useState<QuestionBank | null>(null)
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [chapterTrees, setChapterTrees] = useState<Record<number, Chapter[]>>({})
  const [loading, setLoading] = useState(true)

  // 题目列表相关状态
  const [questions, setQuestions] = useState<Question[]>([])
  const [questionTotal, setQuestionTotal] = useState(0)
  const [questionPage, setQuestionPage] = useState(1)
  const [questionPageSize] = useState(10)
  const [questionSearch, setQuestionSearch] = useState('')
  const [selectedType, setSelectedType] = useState<string>('')
  const [selectedSubjectId, setSelectedSubjectId] = useState<string>('')
  const [loadingQuestions, setLoadingQuestions] = useState(false)
  const [questionError, setQuestionError] = useState('')
  const [expandedQuestionIds, setExpandedQuestionIds] = useState<Record<number, boolean>>({})
  const [editingQuestion, setEditingQuestion] = useState<Question | null>(null)
  const [editingAnswerText, setEditingAnswerText] = useState('')
  const [editingError, setEditingError] = useState('')
  const [savingQuestion, setSavingQuestion] = useState(false)

  const [practiceTarget, setPracticeTarget] = useState<PracticeLaunchTarget | null>(null)

  // Creation modals
  const [newSubjectOpen, setNewSubjectOpen] = useState(false)
  const [newSubjectName, setNewSubjectName] = useState('')
  const [newChapterTarget, setNewChapterTarget] = useState<{
    subjectId: number
    parentId?: number
    level: number
  } | null>(null)
  const [newChapterName, setNewChapterName] = useState('')
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  // 加载题库及科目数据
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [bk, subs] = await Promise.all([
        client.get(`/question-banks/${bankID}`),
        client.get(`/question-banks/${bankID}/subjects`),
      ])
      setBank(bk.data.data)
      const subsData: Subject[] = subs.data.data
      setSubjects(subsData)
      const trees: Record<number, Chapter[]> = {}
      await Promise.all(
        subsData.map(async (s) => {
          const r = await client.get(`/subjects/${s.id}/chapters`)
          trees[s.id] = r.data.data
        })
      )
      setChapterTrees(trees)
    } finally {
      setLoading(false)
    }
  }, [bankID])

  useEffect(() => {
    load()
  }, [load])

  // 加载题目列表
  const loadQuestions = useCallback(async () => {
    if (!bankID) return
    setLoadingQuestions(true)
    setQuestionError('')
    try {
      const params: Record<string, unknown> = {
        bank_id: bankID,
        page: questionPage,
        page_size: questionPageSize,
      }
      if (questionSearch.trim()) params.q = questionSearch.trim()
      if (selectedType) params.type = selectedType
      if (selectedSubjectId) params.subject_id = Number(selectedSubjectId)

      // The user-facing question query endpoint is /questions/search.
      // /questions itself has no GET route (only /questions/:id and /search).
      const res = await client.get('/questions/search', { params })
      const data = res.data.data
      setQuestions(data?.items || [])
      setQuestionTotal(data?.total || 0)
    } catch (e) {
      console.error('加载题目失败:', e)
      setQuestions([])
      setQuestionTotal(0)
      setQuestionError(errMsg(e))
    } finally {
      setLoadingQuestions(false)
    }
  }, [bankID, questionPage, questionPageSize, questionSearch, selectedType, selectedSubjectId])

  useEffect(() => {
    loadQuestions()
  }, [loadQuestions])

  const openQuestionEditor = (question: Question) => {
    setEditingQuestion({
      ...question,
      options: question.options?.length ? question.options.map((option) => ({ ...option })) : defaultOptions(question.type),
    })
    setEditingAnswerText(question.answer?.join('\n') ?? '')
    setEditingError('')
  }

  const changeEditingType = (type: QuestionType) => {
    if (!editingQuestion) return
    const options = isOptionQuestion(type)
      ? editingQuestion.options?.length
        ? editingQuestion.options
        : defaultOptions(type)
      : editingQuestion.options
    const answer = isOptionQuestion(type)
      ? type === 'single_choice'
        ? [editingQuestion.answer?.[0] || options?.[0]?.key || 'A']
        : editingQuestion.answer
      : editingQuestion.answer
    setEditingQuestion({ ...editingQuestion, type, options, answer })
    if (!isOptionQuestion(type)) setEditingAnswerText(answer?.join('\n') ?? '')
  }

  const toggleChoiceAnswer = (key: string) => {
    if (!editingQuestion) return
    const answer =
      editingQuestion.type === 'single_choice'
        ? [key]
        : editingQuestion.answer?.includes(key)
          ? editingQuestion.answer.filter((item) => item !== key)
          : [...(editingQuestion.answer ?? []), key]
    setEditingQuestion({ ...editingQuestion, answer })
  }

  const updateOption = (index: number, field: keyof QuestionOption, value: string) => {
    if (!editingQuestion) return
    const options = [...(editingQuestion.options ?? [])]
    const previousKey = options[index]?.key
    options[index] = { ...options[index], [field]: value }
    const answer =
      field === 'key'
        ? editingQuestion.answer?.map((item) => (item === previousKey ? value : item))
        : editingQuestion.answer
    setEditingQuestion({ ...editingQuestion, options, answer })
  }

  const addOption = () => {
    if (!editingQuestion) return
    const options = editingQuestion.options ?? []
    const key = String.fromCharCode(65 + options.length)
    setEditingQuestion({
      ...editingQuestion,
      options: [...options, { key, content: '' }],
    })
  }

  const removeOption = (index: number) => {
    if (!editingQuestion) return
    const removedKey = editingQuestion.options?.[index]?.key
    const options = (editingQuestion.options ?? []).filter((_, optionIndex) => optionIndex !== index)
    const answer = editingQuestion.answer?.filter((item) => item !== removedKey)
    setEditingQuestion({ ...editingQuestion, options, answer })
  }

  const saveQuestion = async () => {
    if (!editingQuestion) return
    const stem = editingQuestion.stem.trim()
    const answer = isOptionQuestion(editingQuestion.type)
      ? editingQuestion.answer ?? []
      : parseAnswerText(editingAnswerText)
    const options = isOptionQuestion(editingQuestion.type)
      ? (editingQuestion.options ?? []).filter((option) => option.key.trim() || option.content.trim())
      : []

    if (!stem) {
      setEditingError('题干不能为空')
      return
    }
    if (!answer.length && ['single_choice', 'multiple_choice', 'true_false', 'fill_blank'].includes(editingQuestion.type)) {
      setEditingError('请填写或选择标准答案')
      return
    }
    if (isOptionQuestion(editingQuestion.type) && options.some((option) => !option.key.trim() || !option.content.trim())) {
      setEditingError('每个选项都需要填写选项标识和内容')
      return
    }

    setSavingQuestion(true)
    setEditingError('')
    try {
      await client.put(`/questions/${editingQuestion.id}`, {
        ...editingQuestion,
        stem,
        answer,
        options,
        original_analysis: editingQuestion.original_analysis?.trim() ?? '',
      })
      await loadQuestions()
      setEditingQuestion(null)
    } catch (e) {
      setEditingError(errMsg(e))
    } finally {
      setSavingQuestion(false)
    }
  }

  const toggleExpand = (qId: number) => {
    setExpandedQuestionIds((prev) => ({
      ...prev,
      [qId]: !prev[qId],
    }))
  }

  const addSubject = async () => {
    if (!newSubjectName.trim()) return
    await client.post(`/question-banks/${bankID}/subjects`, { name: newSubjectName })
    setNewSubjectOpen(false)
    setNewSubjectName('')
    load()
  }

  const addChapter = async () => {
    if (!newChapterTarget || !newChapterName.trim()) return
    await client.post(`/subjects/${newChapterTarget.subjectId}/chapters`, {
      name: newChapterName,
      parent_id: newChapterTarget.parentId,
      level: newChapterTarget.level,
    })
    setNewChapterTarget(null)
    setNewChapterName('')
    load()
  }

  const handleDeleteBank = async () => {
    setDeleting(true)
    try {
      await client.delete(`/question-banks/${bankID}`)
      navigate('/question-banks')
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setDeleting(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-28 rounded-3xl" />
        <Skeleton className="h-80 rounded-3xl" />
      </div>
    )
  }

  const totalPages = Math.ceil(questionTotal / questionPageSize) || 1

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Bank Header Card */}
      <div className="rounded-4xl border border-border/80 bg-gradient-to-br from-surface via-surface to-surface-secondary/70 p-6 sm:p-8 shadow-card space-y-5">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-5">
          <div className="flex items-start gap-4">
            <button
              onClick={() => navigate('/question-banks')}
              className="mt-1 flex h-9 w-9 items-center justify-center rounded-2xl border border-border bg-surface-secondary text-muted-foreground hover:text-foreground transition-colors shrink-0 shadow-subtle hover:scale-105"
              title="返回题库列表"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>

            <div className="space-y-1.5 min-w-0">
              <div className="flex items-center gap-2.5 flex-wrap">
                <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">{bank?.name}</h1>
                <Badge tone={bank?.visibility === 'public' ? 'info' : 'neutral'} variant="subtle">
                  {bank?.visibility === 'public' ? '公开题库' : '私有题库'}
                </Badge>
              </div>
              <p className="text-xs sm:text-sm text-muted-foreground max-w-2xl leading-relaxed">
                {bank?.description || '暂无详细描述'}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 flex-wrap">
            <Button
              onClick={() =>
                setPracticeTarget({
                  bankIds: [bankID],
                  title: bank?.name ?? '全题库刷题',
                })
              }
              icon={<Play className="h-4 w-4 fill-current" />}
              className="shadow-glow"
            >
              刷全库题目
            </Button>
            <Button
              variant="outline"
              onClick={() => setNewSubjectOpen(true)}
              icon={<Plus className="h-4 w-4" />}
            >
              添加科目
            </Button>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(true)}
              icon={<Trash2 className="h-4 w-4 text-destructive" />}
              className="hover:text-destructive hover:border-destructive/40"
              title="删除题库"
            >
              删除题库
            </Button>
          </div>
        </div>

        <div className="flex items-center gap-6 border-t border-border/60 pt-4 text-xs text-muted-foreground">
          <div className="flex items-center gap-2 font-bold text-foreground">
            <GraduationCap className="h-4 w-4 text-primary" />
            <span className="font-mono text-sm">{bank?.question_count ?? questionTotal}</span>
            <span className="font-normal text-muted-foreground">道试题</span>
          </div>
          <div className="flex items-center gap-2">
            <Layers className="h-4 w-4 text-muted-foreground" />
            <span className="font-mono font-semibold text-foreground">{subjects.length}</span>
            <span>个科目分类</span>
          </div>
        </div>
      </div>

      {/* Tabs Switcher */}
      <div className="flex items-center justify-between border-b border-border">
        <div className="flex gap-2">
          <button
            onClick={() => setActiveTab('questions')}
            className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
              activeTab === 'questions'
                ? 'border-primary text-primary font-bold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            <ListOrdered className="h-4 w-4" />
            <span>试题明细</span>
            <Badge tone={activeTab === 'questions' ? 'primary' : 'neutral'} variant="subtle" className="text-[11px] font-mono">
              {questionTotal}
            </Badge>
          </button>

          <button
            onClick={() => setActiveTab('chapters')}
            className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
              activeTab === 'chapters'
                ? 'border-primary text-primary font-bold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            <FolderTree className="h-4 w-4" />
            <span>章节大纲</span>
            <Badge tone={activeTab === 'chapters' ? 'primary' : 'neutral'} variant="subtle" className="text-[11px] font-mono">
              {subjects.length}
            </Badge>
          </button>
        </div>
      </div>

      {/* TAB 1: 题目明细与详情展示 */}
      {activeTab === 'questions' && (
        <div className="space-y-4">
          {/* 筛选与搜索工具栏 */}
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 bg-surface p-4 rounded-3xl border border-border/80 shadow-2xs">
            <div className="relative flex-1">
              <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <input
                type="text"
                placeholder="搜索题目内容、题干、知识点..."
                value={questionSearch}
                onChange={(e) => {
                  setQuestionSearch(e.target.value)
                  setQuestionPage(1)
                }}
                className="input pl-9 w-full text-xs"
              />
            </div>

            <div className="flex items-center gap-2">
              <div className="flex items-center gap-1.5 bg-surface-secondary px-3 py-1.5 rounded-2xl border border-border/60 text-xs">
                <Filter className="h-3.5 w-3.5 text-muted-foreground" />
                <select
                  value={selectedType}
                  onChange={(e) => {
                    setSelectedType(e.target.value)
                    setQuestionPage(1)
                  }}
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

              {subjects.length > 0 && (
                <div className="flex items-center gap-1.5 bg-surface-secondary px-3 py-1.5 rounded-2xl border border-border/60 text-xs">
                  <FolderTree className="h-3.5 w-3.5 text-muted-foreground" />
                  <select
                    value={selectedSubjectId}
                    onChange={(e) => {
                      setSelectedSubjectId(e.target.value)
                      setQuestionPage(1)
                    }}
                    className="bg-transparent border-none text-xs text-foreground focus:outline-none cursor-pointer max-w-[140px] truncate"
                  >
                    <option value="">全部科目</option>
                    {subjects.map((sub) => (
                      <option key={sub.id} value={sub.id}>
                        {sub.name}
                      </option>
                    ))}
                  </select>
                </div>
              )}
            </div>
          </div>

          {/* 题目列表区域 */}
          {loadingQuestions ? (
            <div className="space-y-3">
              <Skeleton className="h-24 rounded-2xl" />
              <Skeleton className="h-24 rounded-2xl" />
              <Skeleton className="h-24 rounded-2xl" />
            </div>
          ) : questionError ? (
            <Card className="p-8">
              <EmptyState
                icon={AlertTriangle}
                title="题目加载失败"
                description={questionError}
                action={
                  <Button size="sm" variant="outline" onClick={loadQuestions}>
                    重新加载
                  </Button>
                }
              />
            </Card>
          ) : questions.length === 0 ? (
            <Card className="p-8">
              <EmptyState
                icon={FileQuestion}
                title="未找到匹配的题目"
                description={questionSearch || selectedType || selectedSubjectId ? '没有符合当前筛选条件的试题，请尝试清除筛选。' : '该题库下暂无试题。'}
                action={
                  (questionSearch || selectedType || selectedSubjectId) ? (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setQuestionSearch('')
                        setSelectedType('')
                        setSelectedSubjectId('')
                        setQuestionPage(1)
                      }}
                    >
                      清空筛选条件
                    </Button>
                  ) : undefined
                }
              />
            </Card>
          ) : (
            <div className="space-y-3.5">
              {questions.map((q, idx) => {
                const isExpanded = !!expandedQuestionIds[q.id]
                const indexNum = (questionPage - 1) * questionPageSize + idx + 1
                return (
                  <div
                    key={q.id}
                    className="rounded-3xl border border-border/80 bg-surface shadow-card overflow-hidden transition-all hover:border-primary/40"
                  >
                    {/* 题目卡片头部简要 */}
                    <div
                      onClick={() => toggleExpand(q.id)}
                      className="p-5 cursor-pointer flex items-start justify-between gap-4 select-none hover:bg-surface-secondary/30 transition-colors"
                    >
                      <div className="flex items-start gap-3 min-w-0 flex-1">
                        <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-surface-secondary font-mono text-xs font-bold text-muted-foreground shrink-0 mt-0.5">
                          {indexNum}
                        </span>

                        <div className="space-y-1.5 flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <Badge
                              tone={QUESTION_TYPE_TONES[q.type] || 'neutral'}
                              variant="subtle"
                              className="text-[10px]"
                            >
                              {QUESTION_TYPE_LABELS[q.type] || q.type}
                            </Badge>

                            {q.difficulty > 0 && (
                              <div className="flex items-center gap-0.5 text-warning">
                                {Array.from({ length: Math.min(q.difficulty, 5) }).map((_, i) => (
                                  <Star key={i} className="h-3 w-3 fill-current" />
                                ))}
                              </div>
                            )}

                            {q.knowledge_points && q.knowledge_points.length > 0 && (
                              <div className="flex items-center gap-1">
                                {q.knowledge_points.map((kp, kIdx) => (
                                  <span
                                    key={kIdx}
                                    className="text-[10px] bg-primary/10 text-primary px-2 py-0.5 rounded-md font-medium"
                                  >
                                    #{kp}
                                  </span>
                                ))}
                              </div>
                            )}
                          </div>

                          <div className="text-sm font-semibold text-foreground line-clamp-2 leading-relaxed">
                            {q.stem}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        <span className="text-xs text-muted-foreground hidden sm:inline">
                          {isExpanded ? '收起详情' : '查看详情'}
                        </span>
                        <button
                          type="button"
                          className="flex h-7 w-7 items-center justify-center rounded-xl bg-surface-secondary text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                        </button>
                      </div>
                    </div>

                    {/* 题目展开详细信息 */}
                    {isExpanded && (
                      <div className="border-t border-primary/15 bg-gradient-to-br from-primary/[0.035] via-surface to-success/[0.025] p-5 text-xs sm:p-6">
                        <div className="flex flex-col gap-3 border-b border-border/60 pb-4 sm:flex-row sm:items-center sm:justify-between">
                          <div className="flex items-center gap-3">
                            <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-primary/10 text-primary ring-1 ring-primary/15">
                              <FileQuestion className="h-4 w-4" />
                            </div>
                            <div>
                              <div className="flex items-center gap-2 text-sm font-bold text-foreground">
                                <span>题目详情</span>
                                {q.type === 'multiple_choice' && (
                                  <Badge tone="primary" variant="subtle" className="text-[10px]">
                                    多项答案
                                  </Badge>
                                )}
                              </div>
                              <div className="mt-0.5 text-[11px] text-muted-foreground">查看题干、选项与解析</div>
                            </div>
                          </div>
                          <div className="flex items-center gap-2 self-end sm:self-auto">
                            {q.answer && q.answer.length > 0 && (
                              <Badge tone="success" variant="subtle" className="px-2.5 py-1 text-[10px]">
                                标准答案：{q.answer.join('、')}
                              </Badge>
                            )}
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => openQuestionEditor(q)}
                              icon={<Pencil className="h-3.5 w-3.5" />}
                            >
                              修改题目
                            </Button>
                          </div>
                        </div>

                        <div className="mt-5 space-y-5">
                          {/* 完整题干 */}
                          <div className="space-y-2">
                            <div className="flex items-center gap-2 text-muted-foreground font-semibold">
                              <span className="h-1.5 w-1.5 rounded-full bg-primary" />
                              <span>题干完整内容</span>
                            </div>
                            <div className="rounded-2xl border border-border/80 bg-surface p-4 text-sm leading-relaxed text-foreground shadow-2xs whitespace-pre-wrap">
                              {q.stem}
                            </div>
                          </div>

                          {/* 选项列表（针对选择题） */}
                          {q.options && q.options.length > 0 && (
                            <div className="space-y-2.5">
                              <div className="flex items-center justify-between gap-3">
                                <div className="flex items-center gap-2 text-muted-foreground font-semibold">
                                  <ListChecks className="h-4 w-4 text-primary" />
                                  <span>试题选项</span>
                                </div>
                                {q.type === 'multiple_choice' && (
                                  <span className="text-[11px] text-muted-foreground">可选择多个答案</span>
                                )}
                              </div>
                              <div className="grid grid-cols-1 gap-2.5 md:grid-cols-2">
                                {q.options.map((opt) => {
                                  const isCorrect = q.answer?.includes(opt.key)
                                  return (
                                    <div
                                      key={opt.key}
                                      className={`group flex min-h-14 items-start gap-3 rounded-2xl border p-3.5 transition-all duration-200 ${
                                        isCorrect
                                          ? 'border-success/35 bg-gradient-to-r from-success/12 to-success/5 text-foreground shadow-[0_4px_16px_rgba(22,163,74,0.08)]'
                                          : 'border-border/80 bg-surface/90 text-muted-foreground hover:border-primary/25 hover:bg-surface hover:shadow-2xs'
                                      }`}
                                    >
                                      <span
                                        className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-xl font-mono text-xs font-bold transition-colors ${
                                          isCorrect
                                            ? 'bg-success text-white shadow-[0_3px_8px_rgba(22,163,74,0.22)]'
                                            : 'border border-border/70 bg-surface-secondary/80 text-foreground/70 group-hover:border-primary/25 group-hover:text-primary'
                                        }`}
                                      >
                                        {opt.key}
                                      </span>
                                      <span className={`flex-1 pt-1 leading-relaxed ${isCorrect ? 'font-semibold' : ''}`}>
                                        {opt.content}
                                      </span>
                                      {isCorrect && (
                                        <span className="flex shrink-0 items-center gap-1 rounded-lg bg-success/10 px-2 py-1 text-[10px] font-bold text-success">
                                          <CheckCircle2 className="h-3.5 w-3.5" />
                                          已选答案
                                        </span>
                                      )}
                                    </div>
                                  )
                                })}
                              </div>
                            </div>
                          )}

                          {/* 非选择题的正确答案展示 */}
                          {(!q.options || q.options.length === 0) && q.answer && q.answer.length > 0 && (
                            <div className="space-y-2">
                              <div className="flex items-center gap-2 text-muted-foreground font-semibold">
                                <span className="h-1.5 w-1.5 rounded-full bg-success" />
                                <span>参考答案</span>
                              </div>
                              <div className="rounded-2xl border border-success/30 bg-success/10 p-3.5 font-mono font-bold leading-relaxed text-success">
                                {q.answer.join('、')}
                              </div>
                            </div>
                          )}

                          {/* 解析部分 */}
                          {(q.original_analysis || q.ai_enhanced_analysis) && (
                            <div className="space-y-2.5 border-t border-border/60 pt-4">
                              <div className="flex items-center gap-2 text-muted-foreground font-semibold">
                                <Lightbulb className="h-4 w-4 text-warning" />
                                <span>试题解析</span>
                              </div>

                              {q.original_analysis && (
                                <div className="rounded-2xl border border-border/80 bg-surface p-4 leading-relaxed text-foreground shadow-2xs whitespace-pre-wrap">
                                  {q.original_analysis}
                                </div>
                              )}

                              {q.ai_enhanced_analysis && (
                                <div className="space-y-1 rounded-2xl border border-primary/20 bg-primary/5 p-4 leading-relaxed text-foreground whitespace-pre-wrap">
                                  <div className="flex items-center gap-1.5 text-[11px] font-bold text-primary">
                                    <Sparkles className="h-3.5 w-3.5" />
                                    AI 增强深度解析
                                  </div>
                                  <div>{q.ai_enhanced_analysis}</div>
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}

              {/* 分页控制 */}
              {totalPages > 1 && (
                <div className="rounded-3xl border border-primary/20 bg-gradient-to-r from-primary/[0.07] via-surface to-surface p-3.5 sm:p-4 shadow-subtle">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-glow">
                        <span className="font-mono text-base font-bold">{questionPage}</span>
                      </div>
                      <div>
                        <div className="flex items-baseline gap-1.5 text-sm font-bold text-foreground">
                          <span>第 {questionPage} 页</span>
                          <span className="font-mono text-xs font-medium text-muted-foreground">/ {totalPages}</span>
                        </div>
                        <div className="mt-0.5 text-[11px] text-muted-foreground">
                          共 <span className="font-mono font-semibold text-foreground">{questionTotal}</span> 道题目
                        </div>
                      </div>
                      <div className="hidden h-1.5 w-24 overflow-hidden rounded-full bg-primary/10 sm:block">
                        <div
                          className="h-full rounded-full bg-primary transition-all duration-300"
                          style={{ width: `${(questionPage / totalPages) * 100}%` }}
                        />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-2 sm:flex sm:items-center">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={questionPage <= 1}
                      onClick={() => setQuestionPage((p) => Math.max(1, p - 1))}
                      icon={<ChevronLeft className="h-4 w-4" />}
                      className="min-w-[96px] border-primary/25 text-primary hover:border-primary/50 hover:bg-primary/10"
                    >
                      上一页
                    </Button>
                    <Button
                      size="sm"
                      disabled={questionPage >= totalPages}
                      onClick={() => setQuestionPage((p) => Math.min(totalPages, p + 1))}
                      icon={<ChevronRight className="h-4 w-4" />}
                      className="min-w-[96px] shadow-glow"
                    >
                      下一页
                    </Button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* TAB 2: 科目与章节大纲结构 */}
      {activeTab === 'chapters' && (
        <div>
          {subjects.length === 0 ? (
            <Card className="p-8">
              <EmptyState
                icon={FolderTree}
                title="该题库暂无科目"
                description="创建科目后，可以在科目下添加各层级章节与试题，便于体系化备考。"
                action={
                  <Button size="sm" onClick={() => setNewSubjectOpen(true)}>
                    立即添加科目
                  </Button>
                }
              />
            </Card>
          ) : (
            <div className="space-y-4">
              {subjects.map((sub) => {
                const chapters = chapterTrees[sub.id] ?? []
                return (
                  <div
                    key={sub.id}
                    className="rounded-3xl border border-border/80 bg-surface shadow-card overflow-hidden transition-all hover:border-border"
                  >
                    {/* Subject Header Row */}
                    <div className="flex items-center justify-between border-b border-border/60 bg-surface-secondary/40 px-5 py-3.5">
                      <div className="flex items-center gap-2.5 min-w-0">
                        <FolderTree className="h-4 w-4 text-primary shrink-0" />
                        <span className="font-bold text-sm text-foreground truncate">
                          {sub.name}
                        </span>
                        <span className="text-xs text-muted-foreground font-mono">
                          ({chapters.length} 章节)
                        </span>
                      </div>

                      <div className="flex items-center gap-2">
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() =>
                            setPracticeTarget({
                              bankIds: [bankID],
                              subjectId: sub.id,
                              title: `科目: ${sub.name}`,
                            })
                          }
                          icon={<Play className="h-3 w-3" />}
                        >
                          刷本科目
                        </Button>
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => setNewChapterTarget({ subjectId: sub.id, level: 1 })}
                          icon={<Plus className="h-3 w-3" />}
                        >
                          添加章节
                        </Button>
                      </div>
                    </div>

                    {/* Chapter List */}
                    <div className="p-4">
                      {chapters.length === 0 ? (
                        <div className="py-6 text-center text-xs text-muted-foreground">
                          暂无章节，点击右上角「添加章节」
                        </div>
                      ) : (
                        <ChapterTreeList
                          chapters={chapters}
                          onPractice={(ch) =>
                            setPracticeTarget({
                              bankIds: [bankID],
                              chapterId: ch.id,
                              title: `章节: ${ch.name}`,
                            })
                          }
                          onAddChild={(ch) =>
                            setNewChapterTarget({
                              subjectId: sub.id,
                              parentId: ch.id,
                              level: ch.level + 1,
                            })
                          }
                        />
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}

      {/* Question Edit Modal */}
      <Modal
        open={!!editingQuestion}
        onClose={() => !savingQuestion && setEditingQuestion(null)}
        title={editingQuestion ? `修改题目 #${editingQuestion.id}` : '修改题目'}
        description="修改后会立即同步到当前题库，并保留原有题目分类。"
        width="max-w-3xl"
      >
        {editingQuestion && (
          <div className="max-h-[72vh] space-y-5 overflow-y-auto pr-1">
            <div className="grid gap-3 sm:grid-cols-[1fr_180px]">
              <div className="rounded-2xl border border-primary/20 bg-primary/[0.04] p-3.5">
                <div className="text-[11px] font-medium text-muted-foreground">正在编辑</div>
                <div className="mt-1 text-sm font-bold text-foreground line-clamp-2">{editingQuestion.stem || '未填写题干'}</div>
              </div>
              <div>
                <label className="label mb-1.5 block">题目类型</label>
                <select
                  className="input text-xs"
                  value={editingQuestion.type}
                  onChange={(e) => changeEditingType(e.target.value as QuestionType)}
                >
                  {(Object.keys(QUESTION_TYPE_LABELS) as QuestionType[]).map((type) => (
                    <option key={type} value={type}>
                      {QUESTION_TYPE_LABELS[type]}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div>
              <label className="label mb-1.5 block">题干内容</label>
              <textarea
                className="input min-h-28 resize-y text-sm leading-relaxed"
                value={editingQuestion.stem}
                onChange={(e) => setEditingQuestion({ ...editingQuestion, stem: e.target.value })}
                placeholder="请输入题干内容"
                autoFocus
              />
            </div>

            {isOptionQuestion(editingQuestion.type) && (
              <div className="space-y-2.5 rounded-2xl border border-border/80 bg-surface-secondary/30 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-xs font-bold text-foreground">选项设置</div>
                    <div className="mt-0.5 text-[11px] text-muted-foreground">可直接修改选项内容，并在下方勾选标准答案。</div>
                  </div>
                  <Button size="xs" variant="outline" onClick={addOption} icon={<Plus className="h-3.5 w-3.5" />}>
                    添加选项
                  </Button>
                </div>

                <div className="space-y-2">
                  {(editingQuestion.options ?? []).map((option, index) => (
                    <div key={`${option.key}-${index}`} className="flex items-center gap-2">
                      <input
                        className="input w-14 shrink-0 px-2 text-center font-mono text-xs font-bold"
                        value={option.key}
                        onChange={(e) => updateOption(index, 'key', e.target.value)}
                        aria-label={`第 ${index + 1} 个选项标识`}
                      />
                      <input
                        className="input min-w-0 flex-1 text-xs"
                        value={option.content}
                        onChange={(e) => updateOption(index, 'content', e.target.value)}
                        placeholder={`请输入选项 ${option.key || index + 1} 的内容`}
                        aria-label={`选项 ${option.key || index + 1} 内容`}
                      />
                      <Button
                        size="xs"
                        variant="ghost"
                        onClick={() => removeOption(index)}
                        className="shrink-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                        title="删除选项"
                      >
                        删除
                      </Button>
                    </div>
                  ))}
                </div>

                <div className="border-t border-border/60 pt-3">
                  <div className="mb-2 text-[11px] font-semibold text-muted-foreground">标准答案</div>
                  <div className="flex flex-wrap gap-2">
                    {(editingQuestion.options ?? []).map((option) => (
                      <label
                        key={`answer-${option.key}`}
                        className={`flex cursor-pointer items-center gap-2 rounded-xl border px-3 py-2 text-xs font-semibold transition-colors ${
                          editingQuestion.answer?.includes(option.key)
                            ? 'border-success/40 bg-success/10 text-success'
                            : 'border-border/80 bg-surface text-muted-foreground hover:border-primary/30 hover:text-foreground'
                        }`}
                      >
                        <input
                          type={editingQuestion.type === 'single_choice' ? 'radio' : 'checkbox'}
                          name={`answer-${editingQuestion.id}`}
                          checked={editingQuestion.answer?.includes(option.key) ?? false}
                          onChange={() => toggleChoiceAnswer(option.key)}
                          className="accent-success"
                        />
                        {option.key || '未命名'}
                      </label>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {!isOptionQuestion(editingQuestion.type) && (
              <div>
                <label className="label mb-1.5 block">标准答案</label>
                <textarea
                  className="input min-h-20 resize-y font-mono text-xs leading-relaxed"
                  value={editingAnswerText}
                  onChange={(e) => setEditingAnswerText(e.target.value)}
                  placeholder="请输入参考答案；多个答案可用换行、逗号或顿号分隔"
                />
              </div>
            )}

            <div>
              <div className="mb-1.5 flex items-center justify-between gap-3">
                <label className="label">试题解析</label>
                <span className="text-[11px] text-muted-foreground">可补充解题思路、判断依据或知识点说明</span>
              </div>
              <textarea
                className="input min-h-28 resize-y text-xs leading-relaxed"
                value={editingQuestion.original_analysis ?? ''}
                onChange={(e) => setEditingQuestion({ ...editingQuestion, original_analysis: e.target.value })}
                placeholder="补充这道题的解析内容（可选）"
              />
            </div>

            {editingError && (
              <div className="rounded-xl border border-destructive/25 bg-destructive/5 px-3.5 py-2.5 text-xs text-destructive">
                {editingError}
              </div>
            )}

            <div className="flex flex-col-reverse gap-2 border-t border-border/60 pt-3 sm:flex-row sm:justify-end">
              <Button variant="outline" onClick={() => setEditingQuestion(null)} disabled={savingQuestion}>
                取消
              </Button>
              <Button onClick={saveQuestion} loading={savingQuestion} icon={<Save className="h-3.5 w-3.5" />}>
                保存修改
              </Button>
            </div>
          </div>
        )}
      </Modal>

      <PracticeConfigModal
        open={!!practiceTarget}
        target={practiceTarget}
        onClose={() => setPracticeTarget(null)}
      />

      {/* New Subject Modal */}
      <Modal
        open={newSubjectOpen}
        onClose={() => setNewSubjectOpen(false)}
        title="新建科目"
        description="科目是知识库的一级分类，如「操作系统」、「计算机网络」"
      >
        <div className="space-y-4">
          <input
            className="input"
            value={newSubjectName}
            onChange={(e) => setNewSubjectName(e.target.value)}
            placeholder="请输入科目名称"
            autoFocus
          />
          <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
            <Button variant="outline" onClick={() => setNewSubjectOpen(false)}>
              取消
            </Button>
            <Button onClick={addSubject} disabled={!newSubjectName.trim()}>
              确认创建
            </Button>
          </div>
        </div>
      </Modal>

      {/* New Chapter Modal */}
      <Modal
        open={!!newChapterTarget}
        onClose={() => setNewChapterTarget(null)}
        title="新建章节"
        description="章节用于划分具体的考点与知识结构"
      >
        <div className="space-y-4">
          <input
            className="input"
            value={newChapterName}
            onChange={(e) => setNewChapterName(e.target.value)}
            placeholder="例如：类加载机制与双亲委派"
            autoFocus
          />
          <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
            <Button variant="outline" onClick={() => setNewChapterTarget(null)}>
              取消
            </Button>
            <Button onClick={addChapter} disabled={!newChapterName.trim()}>
              确认创建
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={deleteOpen}
        onClose={() => !deleting && setDeleteOpen(false)}
        title="确认删除题库"
        description="此操作不可撤销，题库下的所有科目、章节及试题将一并被删除。"
      >
        <div className="space-y-4">
          <div className="flex items-start gap-3 rounded-2xl bg-destructive/10 p-4 text-xs text-destructive border border-destructive/20">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
            <div className="leading-relaxed">
              确定要删除题库 <span className="font-bold underline">「{bank?.name}」</span> 吗？删除后相关科目、章节及练习历史将无法恢复。
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2 border-t border-border/60">
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleting}>
              取消
            </Button>
            <Button variant="danger" onClick={handleDeleteBank} loading={deleting} icon={<Trash2 className="h-3.5 w-3.5" />}>
              确认删除
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

function ChapterTreeList({
  chapters,
  onPractice,
  onAddChild,
}: {
  chapters: Chapter[]
  onPractice: (ch: Chapter) => void
  onAddChild: (ch: Chapter) => void
}) {
  return (
    <div className="space-y-2">
      {chapters.map((ch) => (
        <div key={ch.id} className="space-y-2">
          <div className="group flex items-center justify-between rounded-2xl border border-border/70 bg-surface px-4 py-3 text-xs transition-all hover:bg-surface-secondary/70 hover:border-primary/40 shadow-2xs">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              {ch.level > 1 ? (
                <ChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              ) : (
                <BookOpen className="h-4 w-4 text-primary shrink-0" />
              )}
              <span className="font-bold text-foreground truncate">{ch.name}</span>
              {ch.question_count !== undefined && (
                <Badge tone="neutral" variant="subtle" className="text-[10px] font-mono shrink-0 font-semibold">
                  {ch.question_count} 题
                </Badge>
              )}
              {ch.mastery_score !== undefined && ch.mastery_score > 0 && (
                <div className="hidden sm:block">
                  <MasteryIndicator score={ch.mastery_score} size="sm" />
                </div>
              )}
            </div>

            <div className="flex items-center gap-1.5 opacity-90 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity">
              <Button
                variant="subtle"
                size="xs"
                onClick={() => onPractice(ch)}
                icon={<Play className="h-3 w-3 fill-current" />}
              >
                刷题
              </Button>
              {ch.level < 3 && (
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={() => onAddChild(ch)}
                  icon={<Plus className="h-3 w-3" />}
                >
                  子章节
                </Button>
              )}
            </div>
          </div>

          {ch.children && ch.children.length > 0 && (
            <div className="ml-4 pl-3.5 border-l-2 border-border/70 space-y-2">
              <ChapterTreeList
                chapters={ch.children}
                onPractice={onPractice}
                onAddChild={onAddChild}
              />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
