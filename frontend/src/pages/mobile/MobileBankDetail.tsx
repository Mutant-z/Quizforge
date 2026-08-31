import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, MasteryIndicator, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { MobileSegmentedControl } from '@/components/mobile/MobileSegmentedControl'
import { showToast } from '@/components/mobile/MobileToast'
import type { Chapter, Question, QuestionBank, QuestionOption, QuestionType, Subject } from '@/types'
import {
  AlertTriangle,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Edit3,
  FileQuestion,
  Filter,
  FolderTree,
  GraduationCap,
  Layers,
  ListOrdered,
  Pencil,
  Play,
  Plus,
  RotateCcw,
  Search,
  Shuffle,
  Sparkles,
  Star,
  Trash2,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

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

const OPTION_TYPES: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false']

function defaultOptions(type: QuestionType = 'single_choice'): QuestionOption[] {
  if (type === 'true_false') {
    return [
      { key: 'A', content: '正确' },
      { key: 'B', content: '错误' },
    ]
  }
  return [
    { key: 'A', content: '' },
    { key: 'B', content: '' },
    { key: 'C', content: '' },
    { key: 'D', content: '' },
  ]
}

export default function MobileBankDetail() {
  const { id } = useParams()
  const bankID = Number(id)
  const navigate = useNavigate()

  const [activeTab, setActiveTab] = useState<'chapters' | 'questions'>('chapters')
  const [bank, setBank] = useState<QuestionBank | null>(null)
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [chapterTrees, setChapterTrees] = useState<Record<number, Chapter[]>>({})
  const [expandedSubjects, setExpandedSubjects] = useState<Record<number, boolean>>({})
  const [expandedChapters, setExpandedChapters] = useState<Record<number, boolean>>({})
  const [loading, setLoading] = useState(true)

  // Questions tab states
  const [questions, setQuestions] = useState<Question[]>([])
  const [questionTotal, setQuestionTotal] = useState(0)
  const [questionPage, setQuestionPage] = useState(1)
  const [questionSearch, setQuestionSearch] = useState('')
  const [selectedType, setSelectedType] = useState<string>('')
  const [loadingQuestions, setLoadingQuestions] = useState(false)
  const [expandedQuestionIds, setExpandedQuestionIds] = useState<Record<number, boolean>>({})

  // Practice bottom sheet modal
  const [practiceOpen, setPracticeOpen] = useState(false)
  const [practiceMode, setPracticeMode] = useState<'sequential' | 'random' | 'unattempted' | 'wrong' | 'favorite' | 'due'>('sequential')
  const [practiceTarget, setPracticeTarget] = useState<{
    type: 'bank' | 'subject' | 'chapter'
    id?: number
    title: string
  }>({ type: 'bank', id: bankID, title: '全部题库' })
  const [limit, setLimit] = useState(20)
  const [practiceType, setPracticeType] = useState<QuestionType | ''>('')
  const [timeLimit, setTimeLimit] = useState<number | null>(null)
  const [customTimeMinutes, setCustomTimeMinutes] = useState('90')
  const [startingPractice, setStartingPractice] = useState(false)

  const isCustomTime = timeLimit !== null && ![null, 15 * 60, 30 * 60, 45 * 60, 60 * 60].includes(timeLimit)
  const updateCustomTime = (value: string) => {
    setCustomTimeMinutes(value)
    const minutes = Number(value)
    setTimeLimit(Number.isFinite(minutes) && minutes >= 1 ? Math.min(24 * 60, Math.floor(minutes)) * 60 : null)
  }

  // Subject / Chapter create & delete sheets
  const [newSubjectOpen, setNewSubjectOpen] = useState(false)
  const [newSubjectName, setNewSubjectName] = useState('')
  const [newChapterTarget, setNewChapterTarget] = useState<{ subjectId: number; parentId?: number } | null>(null)
  const [newChapterName, setNewChapterName] = useState('')
  const [deleteSubjectTarget, setDeleteSubjectTarget] = useState<Subject | null>(null)
  const [deleteChapterTarget, setDeleteChapterTarget] = useState<Chapter | null>(null)
  const [deletingSubjectOrChapter, setDeletingSubjectOrChapter] = useState(false)

  // Question Create / Edit Sheet
  const [editingQuestion, setEditingQuestion] = useState<Partial<Question> | null>(null)
  const [editingIsNew, setEditingIsNew] = useState(false)
  const [editingOptions, setEditingOptions] = useState<QuestionOption[]>([])
  const [editingAnswerText, setEditingAnswerText] = useState('')
  const [savingQuestion, setSavingQuestion] = useState(false)
  const [deleteQuestionTarget, setDeleteQuestionTarget] = useState<Question | null>(null)
  const [deletingQuestion, setDeletingQuestion] = useState(false)

  // Load bank info & chapter tree
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
          try {
            const ch = await client.get(`/subjects/${s.id}/chapters`)
            trees[s.id] = ch.data.data
          } catch {
            trees[s.id] = []
          }
        })
      )
      setChapterTrees(trees)
      if (subsData.length > 0) {
        setExpandedSubjects({ [subsData[0].id]: true })
      }
    } finally {
      setLoading(false)
    }
  }, [bankID])

  useEffect(() => {
    load()
  }, [load])

  // Load questions for list tab
  const loadQuestions = useCallback(async () => {
    setLoadingQuestions(true)
    try {
      const params: Record<string, unknown> = {
        bank_id: bankID,
        page: questionPage,
        page_size: 15,
      }
      if (selectedType) params.type = selectedType
      if (questionSearch.trim()) params.q = questionSearch.trim()

      const resp = await client.get('/questions', { params })
      const data = resp.data.data
      setQuestions(data.items || [])
      setQuestionTotal(data.total || 0)
    } finally {
      setLoadingQuestions(false)
    }
  }, [bankID, questionPage, selectedType, questionSearch])

  useEffect(() => {
    if (activeTab === 'questions') {
      loadQuestions()
    }
  }, [activeTab, loadQuestions])

  const startPractice = async () => {
    setStartingPractice(true)
    try {
      const payload: Record<string, unknown> = {
        bank_ids: [bankID],
        mode: practiceMode,
        limit,
        type: practiceType,
        time_limit_seconds: timeLimit,
      }
      if (practiceTarget.type === 'subject' && practiceTarget.id) {
        payload.subject_id = practiceTarget.id
      } else if (practiceTarget.type === 'chapter' && practiceTarget.id) {
        payload.chapter_id = practiceTarget.id
      }

      const resp = await client.post('/practice/sessions', payload)
      const session = resp.data.data
      setPracticeOpen(false)
      navigate(`/practice/${session.id}`)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setStartingPractice(false)
    }
  }

  const handleCreateSubject = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newSubjectName.trim()) return
    try {
      await client.post(`/question-banks/${bankID}/subjects`, { name: newSubjectName.trim() })
      showToast('科目创建成功', 'success')
      setNewSubjectName('')
      setNewSubjectOpen(false)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const handleCreateChapter = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newChapterName.trim() || !newChapterTarget) return
    try {
      await client.post(`/subjects/${newChapterTarget.subjectId}/chapters`, {
        name: newChapterName.trim(),
        parent_id: newChapterTarget.parentId,
      })
      showToast('章节创建成功', 'success')
      setNewChapterName('')
      setNewChapterTarget(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const handleDeleteSubject = async () => {
    if (!deleteSubjectTarget) return
    setDeletingSubjectOrChapter(true)
    try {
      await client.delete(`/subjects/${deleteSubjectTarget.id}`)
      showToast('科目已删除', 'success')
      setDeleteSubjectTarget(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setDeletingSubjectOrChapter(false)
    }
  }

  const handleDeleteChapter = async () => {
    if (!deleteChapterTarget) return
    setDeletingSubjectOrChapter(true)
    try {
      await client.delete(`/chapters/${deleteChapterTarget.id}`)
      showToast('章节已删除', 'success')
      setDeleteChapterTarget(null)
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setDeletingSubjectOrChapter(false)
    }
  }

  const openCreateQuestion = () => {
    setEditingIsNew(true)
    setEditingQuestion({
      bank_id: bankID,
      type: 'single_choice',
      stem: '',
      difficulty: 3,
      knowledge_points: [],
      original_analysis: '',
    })
    setEditingOptions(defaultOptions('single_choice'))
    setEditingAnswerText('A')
  }

  const openEditQuestion = (q: Question) => {
    setEditingIsNew(false)
    setEditingQuestion({ ...q })
    setEditingOptions(q.options && q.options.length > 0 ? q.options : defaultOptions(q.type))
    setEditingAnswerText(Array.isArray(q.answer) ? q.answer.join(', ') : String(q.answer || ''))
  }

  const handleSaveQuestion = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editingQuestion || !editingQuestion.stem?.trim()) {
      showToast('题干内容不能为空', 'warning')
      return
    }

    const type = (editingQuestion.type || 'single_choice') as QuestionType
    const isOption = OPTION_TYPES.includes(type)
    const answer = isOption
      ? editingAnswerText.split(/[,，、]/).map((s) => s.trim().toUpperCase()).filter(Boolean)
      : [editingAnswerText.trim()]

    if (answer.length === 0) {
      showToast('请填写或选择标准答案', 'warning')
      return
    }

    setSavingQuestion(true)
    try {
      const payload: Record<string, unknown> = {
        bank_id: bankID,
        type,
        stem: editingQuestion.stem.trim(),
        options: isOption ? editingOptions : undefined,
        answer,
        difficulty: editingQuestion.difficulty ?? 3,
        knowledge_points: editingQuestion.knowledge_points ?? [],
        original_analysis: editingQuestion.original_analysis || '',
      }

      if (editingIsNew) {
        await client.post('/questions', payload)
        showToast('试题创建成功', 'success')
      } else if (editingQuestion.id) {
        await client.patch(`/questions/${editingQuestion.id}`, payload)
        showToast('试题更新成功', 'success')
      }

      setEditingQuestion(null)
      loadQuestions()
      load()
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setSavingQuestion(false)
    }
  }

  const handleDeleteQuestion = async () => {
    if (!deleteQuestionTarget) return
    setDeletingQuestion(true)
    try {
      await client.delete(`/questions/${deleteQuestionTarget.id}`)
      showToast('试题已删除', 'success')
      setDeleteQuestionTarget(null)
      loadQuestions()
      load()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setDeletingQuestion(false)
    }
  }

  // Recursive chapter tree renderer
  const renderChapterNode = (ch: Chapter, subjectId: number, depth = 0) => {
    const hasChildren = ch.children && ch.children.length > 0
    const isExpanded = expandedChapters[ch.id] ?? false

    return (
      <div key={ch.id} className="space-y-1.5">
        <div
          className={`flex items-center justify-between p-2.5 rounded-2xl border transition-all ${
            depth === 0 ? 'bg-surface border-border/80 shadow-2xs' : 'bg-surface-secondary/40 border-border/60 ml-3'
          }`}
        >
          <div
            onClick={() => {
              if (hasChildren) {
                setExpandedChapters((prev) => ({ ...prev, [ch.id]: !isExpanded }))
              }
            }}
            className="flex items-center gap-2 min-w-0 flex-1 cursor-pointer"
          >
            {hasChildren ? (
              <ChevronRight className={`h-4 w-4 text-muted-foreground transition-transform shrink-0 ${isExpanded ? 'rotate-90' : ''}`} />
            ) : (
              <FolderTree className="h-4 w-4 text-primary shrink-0 opacity-70" />
            )}
            <div className="min-w-0">
              <span className="text-xs font-semibold text-foreground truncate block">{ch.name}</span>
              <span className="text-[10px] text-muted-foreground font-mono">
                {ch.question_count ?? 0} 题 {ch.mastery_score !== undefined ? `· 掌握度 ${Math.round(ch.mastery_score)}%` : ''}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-1 shrink-0">
            {/* Direct practice chapter */}
            <button
              onClick={() => {
                setPracticeTarget({ type: 'chapter', id: ch.id, title: ch.name })
                setPracticeOpen(true)
              }}
              className="flex items-center gap-1 rounded-xl bg-primary/10 px-2 py-1 text-[10px] font-bold text-primary active:scale-95"
            >
              <Play className="h-3 w-3 fill-current" />
              <span>练此章</span>
            </button>

            {/* Sub-chapter add */}
            <button
              onClick={() => setNewChapterTarget({ subjectId, parentId: ch.id })}
              className="p-1 text-muted-foreground hover:text-foreground"
              title="添加子章节"
            >
              <Plus className="h-3.5 w-3.5" />
            </button>

            {/* Delete chapter */}
            <button
              onClick={() => setDeleteChapterTarget(ch)}
              className="p-1 text-muted-foreground hover:text-destructive"
              title="删除章节"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>

        {hasChildren && isExpanded && (
          <div className="space-y-1.5 pt-0.5">
            {ch.children!.map((child) => renderChapterNode(child, subjectId, depth + 1))}
          </div>
        )}
      </div>
    )
  }

  const totalPages = Math.ceil(questionTotal / 15)

  return (
    <div className="relative min-h-full select-none">
      <MobilePullToRefresh onRefresh={load} className="p-4 space-y-3.5 pb-28">
        {/* Bank Header Card */}
        {loading ? (
          <Skeleton className="h-32 rounded-3xl" />
        ) : (
          <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-2.5">
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-start gap-3 min-w-0">
                <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary shadow-subtle">
                  <GraduationCap className="h-6 w-6" />
                </div>
                <div className="min-w-0">
                  <h2 className="text-base font-bold tracking-tight text-foreground truncate">{bank?.name}</h2>
                  <p className="text-xs text-muted-foreground line-clamp-2 mt-0.5 leading-relaxed">
                    {bank?.description || '暂无详细描述'}
                  </p>
                </div>
              </div>
              <Badge tone={bank?.visibility === 'public' ? 'info' : 'neutral'} variant="subtle" className="text-[10px]">
                {bank?.visibility === 'public' ? '公开' : '私有'}
              </Badge>
            </div>

            <div className="flex items-center justify-between border-t border-border/60 pt-2 text-xs text-muted-foreground font-mono">
              <div className="flex items-center gap-2">
                <span className="font-bold text-foreground">{bank?.question_count} <span className="font-normal text-muted-foreground">题</span></span>
                <span>·</span>
                <span>{subjects.length} 科目</span>
              </div>
              <MasteryIndicator score={75} size="sm" />
            </div>
          </div>
        )}

        {/* Tab Switcher: 章节大纲 vs 题目明细 */}
        <MobileSegmentedControl
          active={activeTab}
          onChange={(val) => setActiveTab(val)}
          items={[
            { key: 'chapters', label: '章节大纲', badge: subjects.length },
            { key: 'questions', label: '题目明细', badge: questionTotal || bank?.question_count },
          ]}
        />

        {/* View 1: Chapters Outline */}
        {activeTab === 'chapters' && (
          <div className="space-y-3">
            <div className="flex items-center justify-between px-1">
              <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                学科与大纲结构
              </span>
              <button
                onClick={() => setNewSubjectOpen(true)}
                className="flex items-center gap-1 text-xs font-bold text-primary active:scale-95"
              >
                <Plus className="h-3.5 w-3.5" />
                <span>新建科目</span>
              </button>
            </div>

            {loading ? (
              <div className="space-y-2">
                {[...Array(3)].map((_, i) => (
                  <Skeleton key={i} className="h-16 rounded-2xl" />
                ))}
              </div>
            ) : subjects.length === 0 ? (
              <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-6 text-center space-y-2">
                <FolderTree className="h-8 w-8 text-muted-foreground mx-auto opacity-50" />
                <h4 className="text-xs font-bold text-foreground">暂无科目与章节</h4>
                <p className="text-[11px] text-muted-foreground">点击上方「新建科目」开始构建题库大纲</p>
              </div>
            ) : (
              subjects.map((sub) => {
                const isExpanded = expandedSubjects[sub.id] ?? false
                const trees = chapterTrees[sub.id] || []

                return (
                  <div
                    key={sub.id}
                    className="rounded-3xl border border-border/80 bg-surface p-3.5 shadow-card space-y-2.5"
                  >
                    <div className="flex items-center justify-between">
                      <div
                        onClick={() => setExpandedSubjects((prev) => ({ ...prev, [sub.id]: !isExpanded }))}
                        className="flex items-center gap-2 cursor-pointer min-w-0"
                      >
                        <ChevronRight className={`h-4 w-4 text-primary transition-transform ${isExpanded ? 'rotate-90' : ''}`} />
                        <span className="text-xs font-bold text-foreground truncate">{sub.name}</span>
                        <span className="text-[10px] font-mono text-muted-foreground">({trees.length} 章节)</span>
                      </div>

                      <div className="flex items-center gap-1">
                        <button
                          onClick={() => {
                            setPracticeTarget({ type: 'subject', id: sub.id, title: sub.name })
                            setPracticeOpen(true)
                          }}
                          className="flex items-center gap-1 rounded-xl bg-primary/10 px-2 py-1 text-[10px] font-bold text-primary active:scale-95"
                        >
                          <Play className="h-3 w-3 fill-current" />
                          <span>练此科目</span>
                        </button>
                        <button
                          onClick={() => setNewChapterTarget({ subjectId: sub.id })}
                          className="p-1 text-muted-foreground hover:text-foreground"
                          title="添加根章节"
                        >
                          <Plus className="h-3.5 w-3.5" />
                        </button>
                        <button
                          onClick={() => setDeleteSubjectTarget(sub)}
                          className="p-1 text-muted-foreground hover:text-destructive"
                          title="删除科目"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>

                    {isExpanded && (
                      <div className="space-y-2 pt-1 border-t border-border/50">
                        {trees.length === 0 ? (
                          <div className="text-[11px] text-muted-foreground py-2 text-center">暂无章节，点击加号添加</div>
                        ) : (
                          trees.map((ch) => renderChapterNode(ch, sub.id))
                        )}
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
        )}

        {/* View 2: Questions List */}
        {activeTab === 'questions' && (
          <div className="space-y-3">
            {/* Search & Type Filter Strip & Create Button */}
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
                  <input
                    type="text"
                    placeholder="搜索题干或考点..."
                    value={questionSearch}
                    onChange={(e) => setQuestionSearch(e.target.value)}
                    className="input h-9 pl-9 pr-8 text-xs rounded-2xl"
                  />
                  {questionSearch && (
                    <button onClick={() => setQuestionSearch('')} className="absolute right-2.5 top-2.5 text-muted-foreground">
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>

                <Button size="xs" onClick={openCreateQuestion} icon={<Plus className="h-3.5 w-3.5" />} className="h-9 px-3 rounded-2xl shadow-glow text-xs font-bold shrink-0">
                  新建试题
                </Button>
              </div>

              {/* Type Filter Chips */}
              <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
                <button
                  onClick={() => setSelectedType('')}
                  className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
                    !selectedType ? 'bg-primary text-white shadow-2xs font-bold' : 'bg-surface border border-border text-muted-foreground'
                  }`}
                >
                  全部
                </button>
                {Object.entries(QUESTION_TYPE_LABELS).map(([t, label]) => (
                  <button
                    key={t}
                    onClick={() => setSelectedType(t === selectedType ? '' : t)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
                      selectedType === t
                        ? 'bg-primary text-white shadow-2xs font-bold'
                        : 'bg-surface border border-border text-muted-foreground'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            {/* Questions Stream */}
            {loadingQuestions ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, i) => (
                  <Skeleton key={i} className="h-28 rounded-3xl" />
                ))}
              </div>
            ) : questions.length === 0 ? (
              <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-2">
                <FileQuestion className="h-8 w-8 text-muted-foreground mx-auto opacity-50" />
                <h4 className="text-xs font-bold text-foreground">没有找到试题</h4>
                <p className="text-[11px] text-muted-foreground">点击上方「新建试题」添加题目</p>
              </div>
            ) : (
              <div className="space-y-3">
                {questions.map((q) => {
                  const isExpanded = expandedQuestionIds[q.id] ?? false
                  const options = q.options || []

                  return (
                    <div
                      key={q.id}
                      className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-2.5"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-1.5">
                          <span className="text-[10px] font-mono font-bold text-muted-foreground">#{q.id}</span>
                          <Badge tone={QUESTION_TYPE_TONES[q.type] || 'primary'} variant="subtle" className="text-[9px]">
                            {QUESTION_TYPE_LABELS[q.type] || q.type}
                          </Badge>
                          {q.difficulty && (
                            <span className="text-[9px] font-mono text-amber-500 font-bold">
                              ★{q.difficulty}
                            </span>
                          )}
                        </div>

                        <div className="flex items-center gap-1">
                          <button
                            onClick={() => openEditQuestion(q)}
                            className="p-1 text-muted-foreground hover:text-foreground"
                            title="编辑试题"
                          >
                            <Edit3 className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => setDeleteQuestionTarget(q)}
                            className="p-1 text-muted-foreground hover:text-destructive"
                            title="删除试题"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => setExpandedQuestionIds((prev) => ({ ...prev, [q.id]: !isExpanded }))}
                            className="text-xs font-bold text-primary flex items-center gap-0.5 ml-1"
                          >
                            <span>{isExpanded ? '收起' : '答案'}</span>
                            <ChevronDown className={`h-3 w-3 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
                          </button>
                        </div>
                      </div>

                      {/* Stem */}
                      <div className="text-xs text-foreground font-medium leading-relaxed markdown-body">
                        <ReactMarkdown remarkPlugins={[remarkGfm]}>{q.stem}</ReactMarkdown>
                      </div>

                      {/* Options */}
                      {options.length > 0 && (
                        <div className="space-y-1 pt-1">
                          {options.map((opt) => (
                            <div
                              key={opt.key}
                              className="flex items-start gap-2 rounded-xl bg-surface-secondary/50 p-2 text-xs text-foreground/90"
                            >
                              <span className="font-mono font-bold text-primary shrink-0">{opt.key}.</span>
                              <span>{opt.content}</span>
                            </div>
                          ))}
                        </div>
                      )}

                      {/* Expanded Answer & Analysis */}
                      {isExpanded && (
                        <div className="rounded-xl border border-border/70 bg-surface-secondary/40 p-3 space-y-2 text-xs animate-slide-down">
                          <div className="flex items-center gap-2">
                            <span className="font-bold text-emerald-600 dark:text-emerald-400">参考答案：</span>
                            <span className="font-mono font-bold">
                              {Array.isArray(q.answer) ? q.answer.join(', ') : String(q.answer)}
                            </span>
                          </div>
                          {(q.original_analysis || q.ai_enhanced_analysis) && (
                            <div className="text-muted-foreground text-[11px] leading-relaxed border-t border-border/50 pt-2 markdown-body">
                              <ReactMarkdown remarkPlugins={[remarkGfm]}>{q.original_analysis || q.ai_enhanced_analysis || ''}</ReactMarkdown>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )
                })}

                {/* Pagination */}
                {totalPages > 1 && (
                  <div className="flex items-center justify-between pt-2">
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={questionPage <= 1}
                      onClick={() => setQuestionPage((p) => Math.max(1, p - 1))}
                      icon={<ChevronLeft className="h-3.5 w-3.5" />}
                      className="h-8 px-3 rounded-xl text-xs"
                    >
                      上一页
                    </Button>
                    <span className="text-xs font-mono text-muted-foreground font-semibold">
                      {questionPage} / {totalPages}
                    </span>
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={questionPage >= totalPages}
                      onClick={() => setQuestionPage((p) => Math.min(totalPages, p + 1))}
                      className="h-8 px-3 rounded-xl text-xs"
                    >
                      <span>下一页</span>
                      <ChevronRight className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </MobilePullToRefresh>

      {/* Sticky Bottom Big Button: Start Smart Practice */}
      <div className="fixed bottom-[calc(env(safe-area-inset-bottom)+3.6rem)] inset-x-0 z-30 px-4 pointer-events-none">
        <Button
          size="lg"
          onClick={() => {
            setPracticeTarget({ type: 'bank', id: bankID, title: bank?.name || '当前题库' })
            setPracticeOpen(true)
          }}
          icon={<Play className="h-5 w-5 fill-current" />}
          className="w-full h-12 rounded-2xl font-bold shadow-glow pointer-events-auto text-sm"
        >
          立即智能刷题
        </Button>
      </div>

      {/* Practice Configuration Bottom Sheet */}
      <MobileBottomSheet
        open={practiceOpen}
        onClose={() => setPracticeOpen(false)}
        title="开启自适应刷题"
        description={`目标范围: ${practiceTarget.title}`}
      >
        <div className="space-y-4">
          {/* Practice Mode Options */}
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">选择刷题策略</label>
            <div className="grid grid-cols-2 gap-2">
              {[
                { key: 'sequential', label: '顺序刷题', icon: ListOrdered, desc: '按章节顺序逐题练习' },
                { key: 'random', label: '随机刷题', icon: Shuffle, desc: '全局乱序智能抽题' },
                { key: 'unattempted', label: '未做题', icon: ListOrdered, desc: '优先练习尚未作答题目' },
                { key: 'wrong', label: '错题强化', icon: RotateCcw, desc: '专攻历史做错题目' },
                { key: 'favorite', label: '收藏专项', icon: Star, desc: '复习重点标记题目' },
                { key: 'due', label: '到期复习', icon: Sparkles, desc: '艾宾浩斯遗忘曲线' },
              ].map((m) => {
                const isActive = practiceMode === m.key
                return (
                  <button
                    key={m.key}
                    type="button"
                    onClick={() => setPracticeMode(m.key as typeof practiceMode)}
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

          {/* Question Limit Selection */}
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
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">时间限制</label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {[
                [null, '不限时'],
                [15 * 60, '15 分钟'],
                [30 * 60, '30 分钟'],
                [45 * 60, '45 分钟'],
                [60 * 60, '60 分钟'],
              ].map(([value, label]) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => setTimeLimit(value as number | null)}
                  className={`rounded-xl border py-2 text-xs font-semibold ${timeLimit === value ? 'border-primary bg-primary text-white shadow-glow' : 'border-border bg-surface text-muted-foreground'}`}
                >
                  {label}
                </button>
              ))}
              <button
                type="button"
                aria-pressed={isCustomTime}
                onClick={() => updateCustomTime(customTimeMinutes)}
                className={`rounded-xl border py-2 text-xs font-semibold ${isCustomTime ? 'border-primary bg-primary text-white shadow-glow' : 'border-border bg-surface text-muted-foreground'}`}
              >
                自定义
              </button>
            </div>
            {isCustomTime && (
              <div className="flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/[0.04] p-2">
                <label htmlFor="mobile-detail-custom-time" className="shrink-0 text-xs font-semibold text-muted-foreground">自定义时长</label>
                <input
                  id="mobile-detail-custom-time"
                  type="number"
                  min={1}
                  max={24 * 60}
                  value={customTimeMinutes}
                  onChange={(event) => updateCustomTime(event.target.value)}
                  className="input h-9 min-w-0 flex-1 rounded-xl text-center text-xs font-mono font-bold"
                />
                <span className="shrink-0 text-[10px] text-muted-foreground">分钟</span>
              </div>
            )}
          </div>

          {/* Launch Practice Button */}
          <div className="pt-2">
            <Button
              size="lg"
              loading={startingPractice}
              onClick={startPractice}
              icon={<Play className="h-4 w-4 fill-current" />}
              className="w-full h-11 rounded-2xl font-bold shadow-glow text-xs"
            >
              进入刷题工作台 ({limit} 题)
            </Button>
          </div>
        </div>
      </MobileBottomSheet>

      {/* New Subject Bottom Sheet */}
      <MobileBottomSheet open={newSubjectOpen} onClose={() => setNewSubjectOpen(false)} title="新建科目分类">
        <form onSubmit={handleCreateSubject} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">科目名称 *</label>
            <input
              type="text"
              placeholder="例如：高等数学 / 数据结构"
              value={newSubjectName}
              onChange={(e) => setNewSubjectName(e.target.value)}
              className="input h-10 text-xs rounded-xl"
              required
            />
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Button variant="outline" type="button" onClick={() => setNewSubjectOpen(false)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button type="submit" className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              确认创建
            </Button>
          </div>
        </form>
      </MobileBottomSheet>

      {/* New Chapter Bottom Sheet */}
      <MobileBottomSheet open={!!newChapterTarget} onClose={() => setNewChapterTarget(null)} title="新建章节大纲">
        <form onSubmit={handleCreateChapter} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">章节名称 *</label>
            <input
              type="text"
              placeholder="例如：第一章：极限与连续"
              value={newChapterName}
              onChange={(e) => setNewChapterName(e.target.value)}
              className="input h-10 text-xs rounded-xl"
              required
            />
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Button variant="outline" type="button" onClick={() => setNewChapterTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button type="submit" className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              确认创建
            </Button>
          </div>
        </form>
      </MobileBottomSheet>

      {/* Subject Delete Confirmation Sheet */}
      <MobileBottomSheet
        open={!!deleteSubjectTarget}
        onClose={() => setDeleteSubjectTarget(null)}
        title="删除科目确认"
        description={`确定要删除科目「${deleteSubjectTarget?.name}」吗？其下所属的全部章节与关联关系将被清理。`}
      >
        <div className="flex items-center gap-2 pt-2">
          <Button variant="outline" onClick={() => setDeleteSubjectTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
            取消
          </Button>
          <Button
            variant="danger"
            loading={deletingSubjectOrChapter}
            onClick={handleDeleteSubject}
            className="flex-1 h-10 rounded-xl text-xs font-bold"
          >
            确认删除
          </Button>
        </div>
      </MobileBottomSheet>

      {/* Chapter Delete Confirmation Sheet */}
      <MobileBottomSheet
        open={!!deleteChapterTarget}
        onClose={() => setDeleteChapterTarget(null)}
        title="删除章节确认"
        description={`确定要删除章节「${deleteChapterTarget?.name}」吗？`}
      >
        <div className="flex items-center gap-2 pt-2">
          <Button variant="outline" onClick={() => setDeleteChapterTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
            取消
          </Button>
          <Button
            variant="danger"
            loading={deletingSubjectOrChapter}
            onClick={handleDeleteChapter}
            className="flex-1 h-10 rounded-xl text-xs font-bold"
          >
            确认删除
          </Button>
        </div>
      </MobileBottomSheet>

      {/* Question Create / Edit Bottom Sheet */}
      <MobileBottomSheet
        open={!!editingQuestion}
        onClose={() => setEditingQuestion(null)}
        title={editingIsNew ? '新建试题' : '编辑试题'}
      >
        {editingQuestion && (
          <form onSubmit={handleSaveQuestion} className="space-y-3.5">
            {/* Question Type */}
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">题型选择</label>
              <div className="flex items-center gap-1 overflow-x-auto no-scrollbar py-0.5">
                {Object.entries(QUESTION_TYPE_LABELS).map(([t, label]) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => {
                      const newType = t as QuestionType
                      setEditingQuestion({ ...editingQuestion, type: newType })
                      if (OPTION_TYPES.includes(newType)) {
                        setEditingOptions(defaultOptions(newType))
                      }
                    }}
                    className={`px-2.5 py-1 rounded-xl text-[11px] font-semibold shrink-0 transition-all ${
                      editingQuestion.type === t
                        ? 'bg-primary text-white font-bold shadow-2xs'
                        : 'bg-surface-secondary border border-border text-muted-foreground'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            {/* Stem */}
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">题干内容 *</label>
              <textarea
                className="input min-h-[90px] rounded-xl text-xs p-2.5 resize-none"
                placeholder="输入试题题干（支持 Markdown 语法）..."
                value={editingQuestion.stem || ''}
                onChange={(e) => setEditingQuestion({ ...editingQuestion, stem: e.target.value })}
                required
              />
            </div>

            {/* Options if choice type */}
            {OPTION_TYPES.includes((editingQuestion.type || 'single_choice') as QuestionType) && (
              <div className="space-y-2">
                <label className="text-xs font-bold text-foreground block">选项内容</label>
                {editingOptions.map((opt, idx) => (
                  <div key={opt.key} className="flex items-center gap-2">
                    <span className="font-mono text-xs font-bold text-primary w-5 text-center">{opt.key}.</span>
                    <input
                      type="text"
                      className="input h-8 text-xs rounded-xl flex-1"
                      placeholder={`选项 ${opt.key} 内容`}
                      value={opt.content}
                      onChange={(e) => {
                        const newOpts = [...editingOptions]
                        newOpts[idx] = { ...opt, content: e.target.value }
                        setEditingOptions(newOpts)
                      }}
                    />
                  </div>
                ))}
              </div>
            )}

            {/* Standard Answer */}
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">标准答案 *</label>
              <input
                type="text"
                className="input h-9 text-xs rounded-xl"
                placeholder="例如：A / A, B / 正确"
                value={editingAnswerText}
                onChange={(e) => setEditingAnswerText(e.target.value)}
                required
              />
            </div>

            {/* Analysis */}
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground block">解析与思路</label>
              <textarea
                className="input min-h-[70px] rounded-xl text-xs p-2.5 resize-none"
                placeholder="填写试题解析与关键考点..."
                value={editingQuestion.original_analysis || ''}
                onChange={(e) => setEditingQuestion({ ...editingQuestion, original_analysis: e.target.value })}
              />
            </div>

            <div className="flex items-center gap-2 pt-2">
              <Button type="button" variant="outline" onClick={() => setEditingQuestion(null)} className="flex-1 h-10 rounded-xl text-xs">
                取消
              </Button>
              <Button type="submit" loading={savingQuestion} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
                保存试题
              </Button>
            </div>
          </form>
        )}
      </MobileBottomSheet>

      {/* Delete Question Confirmation Sheet */}
      <MobileBottomSheet
        open={!!deleteQuestionTarget}
        onClose={() => setDeleteQuestionTarget(null)}
        title="删除试题确认"
        description={`确定要删除试题 #${deleteQuestionTarget?.id} 吗？删除后不可恢复。`}
      >
        <div className="flex items-center gap-2 pt-2">
          <Button variant="outline" onClick={() => setDeleteQuestionTarget(null)} className="flex-1 h-10 rounded-xl text-xs">
            取消
          </Button>
          <Button
            variant="danger"
            loading={deletingQuestion}
            onClick={handleDeleteQuestion}
            className="flex-1 h-10 rounded-xl text-xs font-bold"
          >
            确认删除
          </Button>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
