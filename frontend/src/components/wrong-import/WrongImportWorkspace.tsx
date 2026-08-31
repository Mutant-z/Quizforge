import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { uploadWrongImportImage, wrongImportApi } from '@/api/wrongImport'
import { createUUID } from '@/utils/uuid'
import { Badge, Button, IconButton, Modal, Spinner } from '@/components/ui'
import {
  AlertCircle,
  ArrowUp,
  Bot,
  Camera,
  Check,
  CheckCircle2,
  CheckSquare,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Edit3,
  FileQuestion,
  FileText,
  HelpCircle,
  ImagePlus,
  Images,
  Layers,
  LayoutList,
  Loader2,
  MessageSquare,
  PanelRightClose,
  PanelRightOpen,
  Paperclip,
  Plus,
  RefreshCw,
  Send,
  Sparkles,
  Terminal,
  Trash2,
  UploadCloud,
  X,
  Zap,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type {
  MessageAttachment,
  WrongImportDraft,
  WrongImportImage,
  WrongImportMergeCandidate,
  WrongImportMessage,
  WrongImportSession,
  WrongImportSessionDetail,
} from '@/types/wrongImport'
import AuthenticatedImage from './AuthenticatedImage'
import WrongDraftCard from './WrongDraftCard'
import WrongDraftDetailModal from './WrongDraftDetailModal'

interface BankOption {
  id: number
  name: string
}

type DraftFilter = 'all' | 'missing_answer' | 'ready' | 'excluded' | 'committed'

const statusTone: Record<string, 'primary' | 'success' | 'neutral'> = {
  active: 'primary',
  committed: 'success',
  archived: 'neutral',
}
const statusLabel: Record<string, string> = {
  active: '进行中',
  committed: '已入库',
  archived: '已归档',
}

const filterMeta: { key: DraftFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'missing_answer', label: '缺答案' },
  { key: 'ready', label: '就绪' },
  { key: 'excluded', label: '已排除' },
  { key: 'committed', label: '已入库' },
]

const WRONG_IMPORT_MERGE_REVIEW_THRESHOLD = 0.7
const answerExtractionIntent = /(答案|参考答案|答题卡|答案解析)/
const extractionActionIntent = /(识别|提取|读取|整理|回填|匹配|解析|看看|这里)/
const questionExtractionIntent = /(题目|错题|试题|题干|选项)/

function isAnswerExtractionRequest(content: string): boolean {
  return answerExtractionIntent.test(content) && extractionActionIntent.test(content)
}

function resolveExtractionMode(content: string): 'questions' | 'answer_key' | 'auto' {
  const asksQuestions = questionExtractionIntent.test(content) && extractionActionIntent.test(content)
  const asksAnswers = isAnswerExtractionRequest(content)
  if (asksQuestions && asksAnswers) return 'auto'
  if (asksAnswers) return 'answer_key'
  return 'questions'
}

interface ParsedExtractionReport {
  isExtractionMessage: boolean
  isNoQuestionMessage?: boolean
  isAnswerKeyMessage?: boolean
  imageName?: string
  extractedCount?: number
  missingSeqs?: number[]
  rawContent: string
}

function isAnswerBatchWaitingMessage(message: WrongImportMessage): boolean {
  return message.role === 'agent' && message.content.includes('答案片段已保存，等待同批次图片整理后再匹配题目')
}

function isAnswerBatchFinalMessage(message: WrongImportMessage): boolean {
  return message.role === 'agent' && message.content.includes('已完成答案图片整理')
}

function parseAgentExtractionMessage(content: string): ParsedExtractionReport {
  if (content.includes('没有识别到完整题目')) {
    const match = content.match(/已分析图片「(.*?)」/)
    return {
      isExtractionMessage: true,
      isNoQuestionMessage: true,
      imageName: match ? match[1] : undefined,
      rawContent: content,
    }
  }

  if (content.includes('已识别到你要读取的是答案参考图')) {
    const match = content.match(/答案参考图「(.*?)」/)
    return {
      isExtractionMessage: true,
      isAnswerKeyMessage: true,
      imageName: match ? match[1] : undefined,
      rawContent: content,
    }
  }

  const batchMatch = content.match(/已完成本批次\s*(\d+)\s*张图片的视觉切题.*?共整理出\s*(\d+)\s*道错题草稿/)
  if (batchMatch) {
    const extractedCount = parseInt(batchMatch[2], 10) || 0
    const missingSeqs: number[] = []
    const missingMatch = content.match(/其中\s*(.*?)\s*未在图片中找到答案/)
    if (missingMatch) {
      const seqMatches = missingMatch[1].matchAll(/第\s*(\d+)\s*题/g)
      for (const m of seqMatches) {
        missingSeqs.push(parseInt(m[1], 10))
      }
    }
    return {
      isExtractionMessage: true,
      extractedCount,
      missingSeqs,
      rawContent: content,
    }
  }

  const extractMatch = content.match(/已从图片「(.*?)」提取\s*(\d+)\s*道题/)
  if (!extractMatch) {
    return { isExtractionMessage: false, rawContent: content }
  }

  const imageName = extractMatch[1]
  const extractedCount = parseInt(extractMatch[2], 10) || 0

  const missingSeqs: number[] = []
  const missingMatch = content.match(/其中\s*(.*?)\s*未在图片中找到答案/)
  if (missingMatch) {
    const seqMatches = missingMatch[1].matchAll(/第\s*(\d+)\s*题/g)
    for (const m of seqMatches) {
      missingSeqs.push(parseInt(m[1], 10))
    }
  }

  return {
    isExtractionMessage: true,
    imageName,
    extractedCount,
    missingSeqs,
    rawContent: content,
  }
}

function uploadedImageAttachments(
  message: WrongImportMessage,
  images: WrongImportImage[],
  sessionId: number,
): MessageAttachment[] {
  if (message.attachments?.length) return message.attachments
  if (message.role !== 'user') return []

  let batchId = ''
  try {
    const metadata = message.metadata_json ? (JSON.parse(message.metadata_json) as { batch_id?: string }) : null
    batchId = metadata?.batch_id ?? ''
  } catch {
    // 历史消息的 metadata 可能为空或不是 JSON，继续用文件名匹配。
  }

  let relatedImages = batchId ? images.filter((image) => image.batch_id === batchId) : []
  if (!relatedImages.length) {
    const names = [...message.content.matchAll(/(?:错题图片|答案参考图)「([^」]+)」/g)].map((match) => match[1])
    relatedImages = images.filter((image) => image.original_name && names.includes(image.original_name))
  }

  // 兼容早期消息：旧数据没有保存 metadata_json，但图片记录仍保留了批次号。
  if (!relatedImages.length) {
    const countMatch = message.content.match(/上传了\s*(\d+)\s*张(?:错题图片|答案参考图)/)
    const expectedCount = countMatch ? Number(countMatch[1]) : 0
    if (expectedCount > 0) {
      const groups = new Map<string, WrongImportImage[]>()
      for (const image of images) {
        if (!image.batch_id) continue
        const group = groups.get(image.batch_id) ?? []
        group.push(image)
        groups.set(image.batch_id, group)
      }

      const messageTime = Date.parse(message.created_at)
      relatedImages = [...groups.values()]
        .filter((group) => group.length === expectedCount)
        .sort((left, right) => {
          const leftDistance = Math.abs(Date.parse(left[0].created_at) - messageTime)
          const rightDistance = Math.abs(Date.parse(right[0].created_at) - messageTime)
          return leftDistance - rightDistance
        })[0] ?? []
    }
  }

  return relatedImages.map((image) => ({
    id: image.id,
    url: wrongImportApi.imageFileUrl(sessionId, image.id),
    name: image.original_name || `图片 #${image.id}`,
    type: 'image' as const,
    size: image.size_bytes,
  }))
}

function matchFilter(draft: WrongImportDraft, filter: DraftFilter): boolean {
  if (filter === 'all') return true
  return draft.review_status === filter
}

function mergeReasonLabel(reason: string): string {
  if (reason.includes('left_truncated') && reason.includes('right_continuation')) {
    return '前一张图片末尾疑似截断，后一张从中间继续，可能是同一道题。'
  }
  if (reason.includes('option_conflict')) return '两张图片的选项内容存在冲突，需要人工核对。'
  if (reason.includes('answer_conflict')) return '两张图片识别出的答案不一致，需要人工核对。'
  if (reason.includes('new_question_marker')) return '后一张图片出现了明确的新题号，建议保持独立。'
  return '相邻图片中的题干边界存在延续迹象，请核对原图后决定。'
}

export default function WrongImportWorkspace() {
  const [sessions, setSessions] = useState<WrongImportSession[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [detail, setDetail] = useState<WrongImportSessionDetail | null>(null)
  const [banks, setBanks] = useState<BankOption[]>([])
  const [bankId, setBankId] = useState<number | ''>('')
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [draftFilter, setDraftFilter] = useState<DraftFilter>('all')
  const [input, setInput] = useState('')
  const [pendingFiles, setPendingFiles] = useState<File[]>([])
  const [uploadProgress, setUploadProgress] = useState<Record<string, number>>({})
  const [isDraggingOver, setIsDraggingOver] = useState(false)
  const [isSessionListOpen, setIsSessionListOpen] = useState(true)
  const [isDraftListOpen, setIsDraftListOpen] = useState(true)
  const [isPipelineExpanded, setIsPipelineExpanded] = useState(false)
  const [sessionToDelete, setSessionToDelete] = useState<WrongImportSession | null>(null)
  const [activeDraftModalId, setActiveDraftModalId] = useState<number | null>(null)
  const [activeDraftId, setActiveDraftId] = useState<number | null>(null)
  const [mergeCandidates, setMergeCandidates] = useState<WrongImportMergeCandidate[]>([])

  const filePreviews = useMemo(
    () => new Map(pendingFiles.map((file) => [file, URL.createObjectURL(file)])),
    [pendingFiles],
  )
  useEffect(() => {
    const urls = filePreviews
    return () => urls.forEach((url) => URL.revokeObjectURL(url))
  }, [filePreviews])

  const [busy, setBusy] = useState('')
  const [streamState, setStreamState] = useState<'connecting' | 'connected' | 'reconnecting'>('connecting')

  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const lastEventId = useRef(0)
  const streamGeneration = useRef(0)
  const streamRef = useRef<AbortController | null>(null)

  const session = detail?.session
  const drafts = useMemo(() => detail?.drafts ?? [], [detail])
  const messages = useMemo(() => {
    const items = detail?.messages ?? []
    // 兼容旧数据：如果中间状态消息后面已有最终整理结果，只展示最终结果。
    // 新流程不会再写入这条中间消息，但这样可以让历史会话也保持一条 Agent 回复。
    return items.filter(
      (message, index) =>
        !isAnswerBatchWaitingMessage(message) || items.slice(index + 1).every((item) => !isAnswerBatchFinalMessage(item)),
    )
  }, [detail])
  const processingImageCount = useMemo(
    () => (detail?.images ?? []).filter((image) => image.status === 'pending' || image.status === 'processing').length,
    [detail],
  )
  const agentResponding =
    busy === 'upload' || busy === 'message' || busy === 'answer-reconcile' || busy === 'reconcile' || processingImageCount > 0
  const missingCount = drafts.filter((d) => d.review_status === 'missing_answer').length
  const readyCount = drafts.filter((d) => d.review_status === 'ready').length
  const excludedCount = drafts.filter((d) => d.review_status === 'excluded').length
  const committedCount = drafts.filter((d) => d.review_status === 'committed').length
  const pendingMerges = mergeCandidates.filter(
    (candidate) => candidate.status === 'pending' && candidate.score >= WRONG_IMPORT_MERGE_REVIEW_THRESHOLD,
  )
  const acceptedMerges = mergeCandidates.filter((candidate) => candidate.status === 'accepted')
  const completedImageCount = (detail?.images ?? []).filter((image) => image.status === 'done' || image.status === 'failed').length
  const canReconcile = (detail?.images ?? []).length >= 2 && completedImageCount === (detail?.images ?? []).length
  const reconcilingBatch = (detail?.batches ?? []).find((batch) => batch.status === 'sealed' || batch.status === 'reconciling')
  const latestBatch = detail?.batches?.[detail.batches.length - 1]
  const answerImageCount = (detail?.images ?? []).filter((image) => image.extraction_mode === 'answer_key').length
  const unresolvedAnswerFragments = (detail?.answer_fragments ?? []).filter((fragment) => fragment.status === 'unresolved')

  const [activeImageId, setActiveImageId] = useState<number | null>(null)
  const [imageQueueFilter, setImageQueueFilter] = useState<'all' | 'has_drafts' | 'empty' | 'processing'>('all')

  const selectedReadyIds = drafts
    .filter((d) => d.review_status === 'ready' && selectedIds.has(d.id))
    .map((d) => d.id)

  const filteredDrafts = useMemo(() => {
    return drafts.filter((d) => {
      if (!matchFilter(d, draftFilter)) return false
      if (activeImageId !== null) {
        const matchesImage = d.sources?.some((s) => s.image_id === activeImageId)
        if (!matchesImage) return false
      }
      return true
    })
  }, [drafts, draftFilter, activeImageId])

  const activeImage = useMemo(() => {
    if (!activeImageId || !detail?.images) return null
    return detail.images.find((img) => img.id === activeImageId) || null
  }, [activeImageId, detail])

  const activeModalDraft = useMemo(() => {
    if (activeDraftModalId === null) return null
    return drafts.find((d) => d.id === activeDraftModalId) || null
  }, [activeDraftModalId, drafts])

  const loadSessions = useCallback(async () => {
    try {
      setSessions(await wrongImportApi.listSessions())
    } catch {
      // 列表加载失败不阻塞会话内操作
    }
  }, [])

  const loadDetail = useCallback(async (id: number) => {
    try {
      const d = await wrongImportApi.getSession(id)
      setDetail(d)
      setMergeCandidates(await wrongImportApi.listMergeCandidates(id, '').catch(() => []))
      setSelectedIds((prev) => {
        const next = new Set(prev)
        for (const draft of d.drafts) {
          if (draft.review_status === 'ready') next.add(draft.id)
          else next.delete(draft.id)
        }
        return next
      })
      if (d.session.target_bank_id) setBankId(d.session.target_bank_id)
    } catch {
      // 会话可能已被删除
      setDetail(null)
    }
  }, [])

  // 初始化：恢复最近会话（无会话则创建）+ 题库列表
  useEffect(() => {
    ;(async () => {
      try {
        const list = await wrongImportApi.listSessions()
        setSessions(list)
        const latest = list.find((s) => s.status === 'active') ?? list[0]
        if (latest) {
          setSelectedId(latest.id)
        } else {
          const created = await wrongImportApi.createSession()
          setSessions((prev) => [created, ...prev])
          setSelectedId(created.id)
        }
      } catch {
        // 首次加载失败时保持空态
      }
    })()
    client
      .get('/question-banks', { params: { page_size: 100 } })
      .then((resp: { data: { data?: { items?: BankOption[] } } }) => setBanks(resp.data.data?.items ?? []))
      .catch(() => undefined)
  }, [])

  useEffect(() => {
    if (!selectedId) return
    void loadDetail(selectedId)
  }, [selectedId, loadDetail])

  useEffect(() => {
    setActiveDraftId(null)
    setActiveDraftModalId(null)
  }, [selectedId])

  const createSession = useCallback(async () => {
    setBusy('create')
    try {
      const created = await wrongImportApi.createSession()
      setSessions((prev) => [created, ...prev])
      setSelectedId(created.id)
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setBusy('')
    }
  }, [])

  const executeDeleteSession = useCallback(async () => {
    if (!sessionToDelete) return
    const target = sessionToDelete
    setBusy(`delete-${target.id}`)
    try {
      await wrongImportApi.discardSession(target.id)
      setSessions((prev) => prev.filter((s) => s.id !== target.id))
      setSessionToDelete(null)
      if (selectedId === target.id) {
        setSelectedId(null)
        setDetail(null)
        const rest = sessions.filter((s) => s.id !== target.id)
        const next = rest.find((s) => s.status === 'active') ?? rest[0]
        if (next) setSelectedId(next.id)
      }
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setBusy('')
    }
  }, [sessionToDelete, selectedId, sessions])

  // 事件流：快照恢复 + 实时订阅 + 断线重连
  useEffect(() => {
    if (!selectedId) return
    const generation = ++streamGeneration.current
    streamRef.current?.abort()
    const controller = new AbortController()
    streamRef.current = controller
    let retry = 0
    let hydrated = false
    let refreshTimer: number | undefined

    const scheduleRefresh = (immediate = false) => {
      if (refreshTimer) window.clearTimeout(refreshTimer)
      refreshTimer = window.setTimeout(() => void loadDetail(selectedId), immediate ? 0 : 400)
    }

    const wait = (delay: number) => new Promise((resolve) => window.setTimeout(resolve, delay))
    const authFetch = async (url: string) =>
      fetch(url, {
        cache: 'no-store',
        headers: { Authorization: `Bearer ${localStorage.getItem('qt_access_token') || ''}` },
        signal: controller.signal,
      })

    const read = async () => {
      while (!controller.signal.aborted) {
        try {
          setStreamState(retry ? 'reconnecting' : 'connecting')
          if (!hydrated) {
            const snapshot = await authFetch(
              `/api/v1/wrong-import/sessions/${selectedId}/events?after_event_id=0&snapshot=1`,
            )
            if (snapshot.ok && snapshot.body) {
              const raw = await snapshot.text()
              for (const line of raw.split('\n')) {
                if (!line.trim()) continue
                try {
                  const event = JSON.parse(line) as { event_id?: number }
                  if (event.event_id && event.event_id > lastEventId.current) lastEventId.current = event.event_id
                } catch {
                  // 忽略不完整行
                }
              }
            }
            hydrated = true
          }
          const response = await authFetch(
            `/api/v1/wrong-import/sessions/${selectedId}/events?after_event_id=${lastEventId.current}`,
          )
          if (!response.ok || !response.body) throw new Error(`event stream HTTP ${response.status}`)
          setStreamState('connected')
          retry = 0
          const reader = response.body.getReader()
          const decoder = new TextDecoder()
          let buffer = ''
          while (!controller.signal.aborted) {
            const { done, value } = await reader.read()
            if (done) throw new Error('event stream closed')
            buffer += decoder.decode(value, { stream: true })
            const lines = buffer.split('\n')
            buffer = lines.pop() || ''
            for (const line of lines) {
              if (!line.trim()) continue
              try {
                const event = JSON.parse(line) as { event_id?: number; type?: string }
                if (event.event_id && event.event_id > lastEventId.current) lastEventId.current = event.event_id
                if (event.type && event.type !== 'heartbeat') scheduleRefresh(event.type === 'commit.completed')
              } catch {
                // 忽略不完整行
              }
            }
          }
        } catch {
          if (controller.signal.aborted) return
          setStreamState('reconnecting')
          retry += 1
          await wait(Math.min(5000, 500 * 2 ** Math.min(retry, 4)))
        }
      }
    }
    void read()
    return () => {
      if (streamGeneration.current === generation) streamGeneration.current += 1
      controller.abort()
      if (refreshTimer) window.clearTimeout(refreshTimer)
    }
  }, [selectedId, loadDetail])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length, busy])

  const uploadFiles = useCallback(
    async (files: File[], extractionMode: 'questions' | 'answer_key' | 'auto' = 'questions', instruction = '') => {
      if (!files.length || !selectedId) return
      setBusy('upload')
      try {
        const batchId = createUUID()
        for (const [index, file] of files.entries()) {
          await uploadWrongImportImage(selectedId, file, (sent, total) =>
            setUploadProgress((current) => ({ ...current, [file.name]: Math.round((sent / total) * 100) })),
            { id: batchId, index, size: files.length, extractionMode, instruction },
          )
        }
        await wrongImportApi.finalizeBatch(selectedId, batchId, files.length)
        setPendingFiles((current) => current.filter((f) => !files.includes(f)))
        setUploadProgress({})
        await loadDetail(selectedId)
      } catch (e) {
        alert(errMsg(e))
      } finally {
        setBusy('')
      }
    },
    [selectedId, loadDetail],
  )

  const checkCrossImageQuestions = useCallback(async () => {
    if (!selectedId) return
    setBusy('reconcile')
    try {
      await wrongImportApi.reconcile(selectedId, 'legacy_session')
      await loadDetail(selectedId)
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setBusy('')
    }
  }, [selectedId, loadDetail])

  const retryAnswerExtraction = useCallback(async () => {
    if (!selectedId) return
    setBusy('answer-reconcile')
    try {
      await wrongImportApi.retryAnswerExtraction(selectedId)
      await loadDetail(selectedId)
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setBusy('')
    }
  }, [selectedId, loadDetail])

  const resolveMergeCandidate = useCallback(
    async (candidate: WrongImportMergeCandidate, action: 'accept' | 'reject') => {
      if (!selectedId) return
      setBusy(`merge-${candidate.id}`)
      try {
        await wrongImportApi.resolveMergeCandidate(selectedId, candidate.id, action)
        await loadDetail(selectedId)
      } catch (e) {
        alert(errMsg(e))
      } finally {
        setBusy('')
      }
    },
    [selectedId, loadDetail],
  )

  const undoMerge = useCallback(
    async (candidate: WrongImportMergeCandidate) => {
      if (!selectedId) return
      setBusy(`undo-${candidate.id}`)
      try {
        await wrongImportApi.undoMerge(selectedId, candidate.id)
        await loadDetail(selectedId)
      } catch (e) {
        alert(errMsg(e))
      } finally {
        setBusy('')
      }
    },
    [selectedId, loadDetail],
  )

  const sendMessage = useCallback(
    async (content: string) => {
      if (!selectedId || !content.trim()) return
      setBusy('message')
      try {
        await wrongImportApi.sendMessage(selectedId, content)
        await loadDetail(selectedId)
      } catch (e) {
        alert(errMsg(e))
        await loadDetail(selectedId)
      } finally {
        setBusy('')
      }
    },
    [selectedId, loadDetail],
  )

  const submitComposer = async () => {
    if (busy || !selectedId) return
    const content = input.trim()
    const files = [...pendingFiles]
    if (!content && !files.length) return

    // 1. 构造乐观预览附件与聚合消息
    const localAttachments = files.map((file) => ({
      url: URL.createObjectURL(file),
      name: file.name,
      type: 'image' as const,
      size: file.size,
    }))

    // 立即清空输入与待传队列
    setInput('')
    setPendingFiles([])

    // 2. 插入本地乐观消息（图文聚合显示）
    const optimisticMessage = {
      id: `temp-${Date.now()}`,
      session_id: selectedId,
      role: 'user' as const,
      content,
      attachments: localAttachments,
      created_at: new Date().toISOString(),
    }
    setDetail((prev) => (prev ? { ...prev, messages: [...prev.messages, optimisticMessage] } : prev))

    // 3. 执行多图上传与后端联动
    if (files.length) {
      await uploadFiles(files, resolveExtractionMode(content), content)
      return
    }
    await sendMessage(content)
  }

  const handlePaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData.items
    const imageFiles: File[] = []
    for (let i = 0; i < items.length; i++) {
      if (items[i].type.startsWith('image/')) {
        const file = items[i].getAsFile()
        if (file) imageFiles.push(file)
      }
    }
    if (imageFiles.length > 0) {
      e.preventDefault()
      setPendingFiles((prev) => [...prev, ...imageFiles])
    }
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(true)
  }

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)
    const files = Array.from(e.dataTransfer.files).filter((f) => f.type.startsWith('image/'))
    if (files.length > 0) {
      setPendingFiles((prev) => [...prev, ...files])
    }
  }

  const confirmImport = async () => {
    if (!selectedId || !selectedReadyIds.length) {
      alert('没有勾选可入库的题目（缺答案的题需先补齐）')
      return
    }
    setBusy('confirm')
    try {
      await wrongImportApi.confirm(selectedId, selectedReadyIds, bankId || undefined)
      await loadDetail(selectedId)
      await loadSessions()
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="flex h-full w-full overflow-hidden bg-background">
      {/* ==================== 1. 左栏：会话列表 ==================== */}
      <aside
        className={`flex flex-col border-r border-border/60 bg-surface-secondary/20 transition-all duration-200 ease-natural shrink-0 z-10 ${
          isSessionListOpen ? 'w-64 min-w-[240px]' : 'w-14 min-w-[56px]'
        }`}
      >
        {/* Session Header */}
        <div className="flex h-14 items-center justify-between border-b border-border/60 px-3.5 bg-surface/70 backdrop-blur-xs">
          {isSessionListOpen ? (
            <div className="flex items-center gap-2.5 min-w-0">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-subtle border border-primary/20">
                <Bot className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <h1 className="text-xs font-bold tracking-tight text-foreground truncate">错题导入</h1>
                <p className="text-[10px] text-muted-foreground truncate">智能提取 · 答题校对</p>
              </div>
            </div>
          ) : (
            <div className="mx-auto flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Bot className="h-4 w-4" />
            </div>
          )}

          <IconButton
            size="xs"
            variant="ghost"
            onClick={() => setIsSessionListOpen(!isSessionListOpen)}
            className="text-muted-foreground hover:text-foreground hidden sm:flex shrink-0"
            title={isSessionListOpen ? '收起会话列表' : '展开会话列表'}
          >
            {isSessionListOpen ? <ChevronLeft className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </IconButton>
        </div>

        {/* New Session Action CTA */}
        {isSessionListOpen ? (
          <div className="p-2.5 pb-1 border-b border-border/40">
            <Button
              size="xs"
              variant="primary"
              onClick={() => void createSession()}
              loading={busy === 'create'}
              icon={<Plus className="h-3.5 w-3.5" />}
              className="w-full justify-center shadow-subtle text-xs h-8 rounded-xl font-semibold"
              title="创建新导入会话"
            >
              新建导入会话
            </Button>
          </div>
        ) : (
          <div className="p-2 flex justify-center border-b border-border/40">
            <IconButton
              size="xs"
              variant="primary"
              onClick={() => void createSession()}
              loading={busy === 'create'}
              title="创建新导入会话"
            >
              <Plus className="h-3.5 w-3.5" />
            </IconButton>
          </div>
        )}

        {/* Sessions list */}
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
          {sessions.map((s, index) => {
            const isSelected = selectedId === s.id
            if (!isSessionListOpen) {
              return (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => {
                    setSelectedId(s.id)
                    if (selectedId === s.id) void loadDetail(s.id)
                  }}
                  title={`${s.title} #${s.id} (${s.ready_count}/${s.draft_count} 就绪)`}
                  className={`relative flex h-10 w-full items-center justify-center rounded-xl font-mono text-xs font-bold transition-all ${
                    isSelected
                      ? 'bg-primary text-primary-foreground shadow-subtle'
                      : 'bg-surface/80 text-muted-foreground hover:bg-surface hover:text-foreground'
                  }`}
                >
                  {String(index + 1).padStart(2, '0')}
                  {s.status === 'active' && !isSelected && (
                    <span className="absolute right-1 top-1 h-2 w-2 rounded-full bg-primary" />
                  )}
                </button>
              )
            }

            return (
              <div
                key={s.id}
                className={`group relative flex w-full items-center rounded-xl border transition-all duration-150 ${
                  isSelected
                    ? 'border-primary/40 bg-primary/[0.08] shadow-xs text-foreground'
                    : 'border-transparent bg-surface/70 hover:bg-surface hover:border-border/70 text-foreground/80'
                }`}
              >
                <button
                  type="button"
                  onClick={() => {
                    setSelectedId(s.id)
                    if (selectedId === s.id) void loadDetail(s.id)
                  }}
                  className="flex min-w-0 flex-1 items-center gap-2.5 p-2.5 pr-8 text-left"
                >
                  <span
                    className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg font-mono text-[11px] font-bold transition-colors ${
                      isSelected
                        ? 'bg-primary text-primary-foreground shadow-xs'
                        : 'bg-surface-secondary text-muted-foreground group-hover:text-foreground'
                    }`}
                  >
                    {String(index + 1).padStart(2, '0')}
                  </span>
                  <div className="min-w-0 flex-1">
                    <span
                      className={`block truncate text-xs font-semibold leading-tight ${
                        isSelected ? 'text-primary font-bold' : 'text-foreground'
                      }`}
                    >
                      {s.title} #{s.id}
                    </span>
                    <div className="mt-1 flex items-center justify-between text-[10px]">
                      <span
                        className={`inline-flex items-center gap-1 font-medium ${
                          s.status === 'active'
                            ? 'text-primary'
                            : s.status === 'committed'
                              ? 'text-emerald-600 dark:text-emerald-400'
                              : 'text-muted-foreground'
                        }`}
                      >
                        <span
                          className={`h-1.5 w-1.5 rounded-full ${
                            s.status === 'active'
                              ? 'bg-primary animate-pulse'
                              : s.status === 'committed'
                                ? 'bg-emerald-500'
                                : 'bg-muted-foreground'
                          }`}
                        />
                        {statusLabel[s.status] || s.status}
                      </span>
                      <span className="font-mono text-muted-foreground font-medium">
                        {s.ready_count}/{s.draft_count} 就绪
                      </span>
                    </div>
                  </div>
                </button>

                <IconButton
                  type="button"
                  size="xs"
                  title="删除会话"
                  variant="ghost"
                  onClick={(e) => {
                    e.stopPropagation()
                    setSessionToDelete(s)
                  }}
                  className={`absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:bg-destructive/10 hover:text-destructive ${
                    isSelected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
                  }`}
                >
                  <Trash2 className="h-3 w-3" />
                </IconButton>

                {isSelected && (
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 h-5 w-1 rounded-r-full bg-primary" />
                )}
              </div>
            )
          })}

          {sessions.length === 0 && (
            <div className="space-y-2 px-3 py-10 text-center">
              <div className="mx-auto flex h-9 w-9 items-center justify-center rounded-xl bg-surface-secondary text-muted-foreground">
                <MessageSquare className="h-4 w-4" />
              </div>
              <h4 className="text-xs font-bold text-foreground">暂无会话</h4>
              <p className="text-[10px] text-muted-foreground">点击上方「新会话」开始导入错题。</p>
            </div>
          )}
        </div>

        {/* Footer actions */}
        <div className="border-t border-border/60 bg-surface/50 p-2">
          <Button
            variant="ghost"
            size="xs"
            className="w-full text-[11px] text-muted-foreground hover:text-foreground h-7"
            onClick={() => void loadSessions()}
            icon={<RefreshCw className="h-3 w-3" />}
          >
            {isSessionListOpen ? '刷新会话列表' : ''}
          </Button>
        </div>
      </aside>

      {/* ==================== 2. 中栏：Agent 对话主区域 ==================== */}
      <main className="flex flex-1 min-w-0 flex-col bg-surface overflow-hidden relative">
        {/* Chat Header */}
        <header className="flex h-14 shrink-0 items-center justify-between gap-2 border-b border-border/60 bg-surface/80 px-3 sm:px-5 backdrop-blur-md z-10">
          <div className="min-w-0 flex items-center gap-2">
            <h2 className="truncate text-xs sm:text-sm font-bold tracking-tight text-foreground max-w-[140px] sm:max-w-[220px]">
              {session ? `${session.title} #${session.id}` : '请选择或创建导入会话'}
            </h2>
            {session && (
              <Badge tone={statusTone[session.status] || 'neutral'} variant="subtle" className="text-[10px] px-1.5 py-0 shrink-0">
                {statusLabel[session.status] || session.status}
              </Badge>
            )}
            <span
              className={`inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded-full shrink-0 ${
                streamState === 'connected'
                  ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                  : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
              }`}
              title={streamState === 'connected' ? '实时状态已连接' : '正在重新连接…'}
            >
              <span
                className={`h-1.5 w-1.5 rounded-full shrink-0 ${
                  streamState === 'connected' ? 'bg-emerald-500 animate-pulse' : 'bg-amber-500 animate-ping'
                }`}
              />
              <span className="hidden md:inline">
                {streamState === 'connected' ? '已连接' : streamState === 'reconnecting' ? '恢复中' : '连接中'}
              </span>
            </span>
          </div>

          {/* Quick Stat Pill Capsules & Drawer Toggle */}
          {session && (
            <div className="flex items-center gap-1.5 shrink-0">
              <div className="hidden sm:flex items-center gap-1 rounded-full bg-surface-secondary/70 p-0.5 border border-border/60 text-xs">
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-surface font-medium text-foreground text-[11px] shadow-2xs">
                  <Layers className="h-3 w-3 text-muted-foreground" />
                  <span className="font-mono font-bold">{drafts.length}</span>
                  <span className="text-muted-foreground text-[10px]">草稿</span>
                </span>
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-medium text-[11px]">
                  <Check className="h-3 w-3" />
                  <span className="font-mono font-bold">{readyCount}</span>
                  <span className="text-[10px]">就绪</span>
                </span>
                {missingCount > 0 && (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 font-medium text-[11px]">
                    <AlertCircle className="h-3 w-3" />
                    <span className="font-mono font-bold">{missingCount}</span>
                    <span className="text-[10px]">待补</span>
                  </span>
                )}
              </div>

              {/* Right Drafts Panel Toggle Button */}
              <button
                type="button"
                onClick={() => setIsDraftListOpen(!isDraftListOpen)}
                className={`inline-flex items-center gap-1.5 rounded-xl px-2.5 py-1 text-xs font-semibold transition-all duration-200 cursor-pointer shadow-2xs border shrink-0 ${
                  isDraftListOpen
                    ? 'bg-surface-secondary text-foreground border-border hover:bg-surface'
                    : 'bg-primary/10 text-primary border-primary/30 shadow-glow'
                }`}
                title={isDraftListOpen ? '收起题目草稿工作区' : '展开题目草稿工作区'}
              >
                {isDraftListOpen ? <PanelRightClose className="h-3.5 w-3.5 shrink-0" /> : <PanelRightOpen className="h-3.5 w-3.5 text-primary shrink-0" />}
                <span className="hidden sm:inline whitespace-nowrap">{isDraftListOpen ? '收起草稿' : '核对草稿'}</span>
                <span className="font-mono text-[10px] opacity-80 shrink-0">({drafts.length})</span>
              </button>
            </div>
          )}
        </header>

        {/* ==================== 顶部固定区域：错题视觉提取 Agent（连续交互流式仪表盘） ==================== */}
        {(() => {
          const totalImages = (detail?.images ?? []).length
          const activeImages = (detail?.images ?? []).filter(
            (img) => img.status === 'pending' || img.status === 'processing',
          )
          const isExtracting = activeImages.length > 0 || busy === 'upload'
          const completedImages = (detail?.images ?? []).filter((img) => img.status === 'done')
          const failedImages = (detail?.images ?? []).filter((img) => img.status === 'failed')
          const progressPercent = totalImages > 0 ? Math.round(((completedImages.length + failedImages.length) / totalImages) * 100) : 0

          const imagesWithDrafts = (detail?.images ?? []).filter((img) => img.draft_count > 0)
          const emptyImages = (detail?.images ?? []).filter((img) => img.draft_count === 0 && img.status === 'done')

          const displayedQueueImages = (detail?.images ?? []).filter((img) => {
            if (imageQueueFilter === 'has_drafts') return img.draft_count > 0
            if (imageQueueFilter === 'empty') return img.draft_count === 0 && img.status === 'done'
            if (imageQueueFilter === 'processing') return img.status === 'processing' || img.status === 'pending'
            return true
          })

          if (!isExtracting && completedImages.length === 0 && failedImages.length === 0) return null

          return (
            <div className="border-b border-border/60 bg-surface/95 backdrop-blur-md px-3.5 sm:px-5 py-2.5 shrink-0 z-10 shadow-2xs transition-all animate-fade-in">
              <div className="w-full space-y-2">
                {/* 连续一体化状态主卡片 */}
                <div
                  className={`relative overflow-hidden rounded-2xl border transition-all duration-200 shadow-card ${
                    isExtracting
                      ? 'border-primary/40 bg-gradient-to-r from-primary/[0.08] via-surface to-surface ring-1 ring-primary/20'
                      : failedImages.length > 0
                        ? 'border-amber-500/40 bg-surface'
                        : 'border-border/80 bg-gradient-to-r from-surface via-surface to-emerald-500/[0.03]'
                  }`}
                >
                  <div className="p-3 space-y-2.5">
                    {/* 第一层：Agent 身份状态 + 右侧统计 & 折叠按钮 */}
                    <div className="flex items-center justify-between gap-2.5">
                      {/* 左侧：Agent 身份与状态指示 */}
                      <div className="flex items-center gap-2.5 min-w-0">
                        <div
                          className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-xl transition-transform ${
                            isExtracting
                              ? 'bg-gradient-to-br from-primary via-primary to-indigo-600 text-white shadow-glow animate-pulse'
                              : 'bg-surface-secondary text-primary border border-border/80 shadow-2xs'
                          }`}
                        >
                          <Sparkles className="h-4 w-4" />
                        </div>
                        <div className="min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-xs sm:text-sm font-bold tracking-tight text-foreground whitespace-nowrap">
                              错题视觉提取 Agent
                            </span>
                            <span
                              className={`inline-flex items-center gap-1.5 text-[10px] sm:text-[11px] font-semibold px-2 py-0.5 rounded-full whitespace-nowrap ${
                                isExtracting
                                  ? 'bg-primary/15 text-primary border border-primary/25 shadow-glow'
                                  : failedImages.length > 0
                                    ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'
                                    : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                              }`}
                            >
                              <span
                                className={`h-1.5 w-1.5 rounded-full shrink-0 ${
                                  isExtracting
                                    ? 'bg-primary animate-ping'
                                    : failedImages.length > 0
                                      ? 'bg-amber-500'
                                      : 'bg-emerald-500'
                                }`}
                              />
                              <span>
                                {isExtracting
                                  ? `多模态视觉切题中 (${completedImages.length}/${totalImages})`
                                  : failedImages.length > 0
                                    ? '部分提取异常'
                                    : `全部提取完成 (${totalImages} 图)`}
                              </span>
                            </span>
                          </div>
                        </div>
                      </div>

                      {/* 右侧：统计与明细折叠展开开关 */}
                      <div className="flex items-center gap-2 shrink-0">
                        <div className="hidden xs:flex items-center gap-1 font-mono text-xs text-muted-foreground bg-surface-secondary/70 px-2.5 py-1 rounded-xl border border-border/60 whitespace-nowrap">
                          <Images className="h-3.5 w-3.5 text-primary shrink-0" />
                          <span className="font-bold text-foreground">{completedImages.length}</span>
                          <span>/</span>
                          <span>{totalImages} 图</span>
                        </div>

                        {totalImages > 0 && (
                          <button
                            type="button"
                            onClick={() => setIsPipelineExpanded(!isPipelineExpanded)}
                            className={`inline-flex items-center gap-1 text-xs font-semibold px-2.5 py-1 rounded-xl border transition-all shadow-2xs cursor-pointer active:scale-95 whitespace-nowrap ${
                              isPipelineExpanded
                                ? 'bg-primary text-primary-foreground border-primary shadow-subtle'
                                : 'bg-surface text-foreground border-border/80 hover:bg-surface-secondary hover:border-primary/40'
                            }`}
                          >
                            <span>{isPipelineExpanded ? '收起流水线' : '查看流水线'}</span>
                            <ChevronDown
                              className={`h-3.5 w-3.5 transition-transform duration-200 shrink-0 ${
                                isPipelineExpanded ? 'rotate-180' : 'text-muted-foreground'
                              }`}
                            />
                          </button>
                        )}
                      </div>
                    </div>

                    {/* 第二层：连续阶段交互 Step 流 (水平滚动条，永不挤压折行) */}
                    <div className="flex items-center justify-between gap-2 pt-1 border-t border-border/40">
                      <div className="flex items-center gap-1.5 sm:gap-2 overflow-x-auto no-scrollbar py-0.5 flex-1 min-w-0">
                        <button
                          type="button"
                          onClick={() => fileInputRef.current?.click()}
                          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-xl text-xs font-medium border transition-all cursor-pointer hover:scale-105 active:scale-95 whitespace-nowrap shrink-0 ${
                            busy === 'upload'
                              ? 'border-primary/40 bg-primary/10 text-primary shadow-glow ring-1 ring-primary/20'
                              : 'border-border/60 bg-surface hover:bg-surface-secondary text-foreground/80'
                          }`}
                          title="点击添加更多错题图片"
                        >
                          {busy === 'upload' ? (
                            <Spinner className="h-3 w-3 text-primary shrink-0" />
                          ) : (
                            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                          )}
                          <span>1. 分片上传</span>
                          <span className="font-mono text-[10px] text-muted-foreground">({totalImages}图)</span>
                        </button>

                        <ChevronRight className="h-3 w-3 text-muted-foreground/40 shrink-0" />

                        <button
                          type="button"
                          onClick={() => setIsPipelineExpanded(!isPipelineExpanded)}
                          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-xl text-xs font-medium border transition-all cursor-pointer hover:scale-105 active:scale-95 whitespace-nowrap shrink-0 ${
                            isExtracting && busy !== 'upload'
                              ? 'border-primary/40 bg-primary/10 text-primary ring-1 ring-primary/20 shadow-glow'
                              : isPipelineExpanded
                                ? 'border-primary/50 bg-primary/5 text-primary'
                                : completedImages.length > 0
                                  ? 'border-border/60 bg-surface hover:bg-surface-secondary text-foreground/80'
                                  : 'border-border/40 bg-surface/40 text-muted-foreground/60'
                          }`}
                          title="点击展开/收起图片处理队列明细"
                        >
                          {isExtracting && busy !== 'upload' ? (
                            <Loader2 className="h-3.5 w-3.5 text-primary animate-spin shrink-0" />
                          ) : completedImages.length > 0 ? (
                            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                          ) : (
                            <span className="h-2 w-2 rounded-full bg-muted-foreground/40 shrink-0" />
                          )}
                          <span>2. 视觉切题</span>
                          <span className="font-mono text-[10px] text-muted-foreground">({completedImages.length}完)</span>
                        </button>

                        <ChevronRight className="h-3 w-3 text-muted-foreground/40 shrink-0" />

                        <button
                          type="button"
                          onClick={() => {
                            setIsDraftListOpen(true)
                            setDraftFilter('all')
                            setActiveImageId(null)
                          }}
                          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-xl text-xs font-medium border transition-all cursor-pointer hover:scale-105 active:scale-95 whitespace-nowrap shrink-0 ${
                            completedImages.length > 0 && !isExtracting
                              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-bold hover:bg-emerald-500/15'
                              : 'border-border/40 bg-surface/40 text-muted-foreground/60'
                          }`}
                          title="点击在右侧打开全部题目草稿"
                        >
                          {completedImages.length > 0 && !isExtracting ? (
                            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                          ) : (
                            <span className="h-2 w-2 rounded-full bg-muted-foreground/40 shrink-0" />
                          )}
                          <span>3. 草稿就绪</span>
                          <span className="font-mono text-[10px]">({drafts.length}题)</span>
                        </button>
                      </div>

                      <div className="hidden lg:block text-[11px] text-muted-foreground truncate shrink-0 pl-2">
                        {isExtracting
                          ? 'AI Agent 正在并行切题…'
                          : `共切分 ${drafts.length} 道草稿 (${readyCount} 就绪 · ${missingCount} 待补)`}
                      </div>
                    </div>
                  </div>

                  {/* 细长平滑进度条 */}
                  {isExtracting && (
                    <div className="h-1 w-full bg-muted/60 overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-primary via-indigo-500 to-emerald-400 transition-all duration-500 ease-out animate-pulse"
                        style={{ width: `${Math.max(15, progressPercent)}%` }}
                      />
                    </div>
                  )}
                </div>

                {/* 展开的图片处理流水线详情：连续交互多列矩阵与双向联动过滤 */}
                {isPipelineExpanded && totalImages > 0 && (
                  <div className="rounded-2xl border border-border/80 bg-surface p-3 space-y-2.5 shadow-card animate-slide-down">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 px-1 text-[11px] font-bold text-muted-foreground">
                      <div className="flex items-center gap-2">
                        <Camera className="h-3.5 w-3.5 text-primary" />
                        <span className="text-foreground font-bold">图片队列与切题联动</span>
                        <span className="text-[10px] text-muted-foreground font-normal">
                          (点击任意图片卡片，可直接在右侧草稿区聚焦核对该图切出的题目)
                        </span>
                      </div>

                      {/* 队列内快捷筛选胶囊 */}
                      <div className="flex items-center gap-1 shrink-0">
                        <button
                          type="button"
                          onClick={() => setImageQueueFilter('all')}
                          className={`px-2 py-0.5 rounded-lg text-[10px] font-medium transition-colors ${
                            imageQueueFilter === 'all'
                              ? 'bg-primary text-primary-foreground font-bold'
                              : 'bg-surface-secondary text-muted-foreground hover:text-foreground'
                          }`}
                        >
                          全部 ({totalImages})
                        </button>
                        <button
                          type="button"
                          onClick={() => setImageQueueFilter('has_drafts')}
                          className={`px-2 py-0.5 rounded-lg text-[10px] font-medium transition-colors ${
                            imageQueueFilter === 'has_drafts'
                              ? 'bg-emerald-600 text-white font-bold'
                              : 'bg-surface-secondary text-muted-foreground hover:text-foreground'
                          }`}
                        >
                          已切题 ({imagesWithDrafts.length})
                        </button>
                        {emptyImages.length > 0 && (
                          <button
                            type="button"
                            onClick={() => setImageQueueFilter('empty')}
                            className={`px-2 py-0.5 rounded-lg text-[10px] font-medium transition-colors ${
                              imageQueueFilter === 'empty'
                                ? 'bg-neutral-600 text-white font-bold'
                                : 'bg-surface-secondary text-muted-foreground hover:text-foreground'
                            }`}
                          >
                            未检出 ({emptyImages.length})
                          </button>
                        )}
                        {activeImageId !== null && (
                          <button
                            type="button"
                            onClick={() => setActiveImageId(null)}
                            className="px-2 py-0.5 rounded-lg text-[10px] font-bold text-primary hover:bg-primary/10 border border-primary/30 transition-colors ml-1"
                          >
                            重置聚焦 ×
                          </button>
                        )}
                      </div>
                    </div>

                    {/* 响应式多列交互网格 */}
                    <div className="max-h-[240px] overflow-y-auto pr-1 overscroll-contain">
                      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-2">
                        {[...displayedQueueImages].reverse().map((img, idx) => {
                          const isFocused = activeImageId === img.id
                          const imageSeq = totalImages - idx

                          return (
                            <div
                              key={img.id}
                              onClick={() => {
                                setActiveImageId(isFocused ? null : img.id)
                                setIsDraftListOpen(true)
                              }}
                              className={`group relative flex items-center justify-between gap-2 text-xs p-2.5 rounded-xl border transition-all duration-150 cursor-pointer shadow-2xs select-none ${
                                isFocused
                                  ? 'border-primary bg-primary/[0.08] ring-2 ring-primary/30 shadow-glow scale-[1.02]'
                                  : 'bg-surface-secondary/40 border-border/60 hover:border-primary/40 hover:bg-surface hover:scale-[1.01]'
                              }`}
                              title={
                                img.draft_count > 0
                                  ? `点击在右侧工作区高亮核对图片「${img.original_name}」切出的 ${img.draft_count} 道题`
                                  : `图片「${img.original_name}」未检出完整题目`
                              }
                            >
                              <div className="flex items-center gap-2 min-w-0 flex-1">
                                <div
                                  className={`flex h-6.5 w-6.5 shrink-0 items-center justify-center rounded-lg text-[11px] font-mono font-bold transition-transform group-hover:scale-105 ${
                                    isFocused
                                      ? 'bg-primary text-primary-foreground shadow-xs'
                                      : 'bg-surface border border-border/70 text-muted-foreground'
                                  }`}
                                >
                                  {imageSeq < 10 ? `0${imageSeq}` : imageSeq}
                                </div>
                                <div className="min-w-0 flex-1">
                                  <div className="truncate font-semibold text-foreground text-xs">
                                    {img.original_name || `图片 #${img.id}`}
                                  </div>
                                </div>
                              </div>

                              <div className="shrink-0">
                                {img.status === 'processing' ? (
                                  <span className="inline-flex items-center gap-1 text-[10px] text-primary font-mono font-bold bg-primary/10 px-1.5 py-0.5 rounded-md border border-primary/20">
                                    <Spinner className="h-2 w-2" />
                                    解析中
                                  </span>
                                ) : img.status === 'pending' ? (
                                  <span className="text-[10px] text-muted-foreground font-mono bg-surface px-1.5 py-0.5 rounded-md border border-border/60">
                                    排队
                                  </span>
                                ) : img.status === 'done' ? (
                                  <span
                                    className={`inline-flex items-center gap-1 text-[10px] font-mono font-bold px-1.5 py-0.5 rounded-md border ${
                                      isFocused
                                        ? 'bg-primary text-primary-foreground border-primary'
                                        : img.draft_count > 0
                                          ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                                          : 'bg-muted/60 text-muted-foreground border-border/60'
                                    }`}
                                  >
                                    {isFocused ? (
                                      '聚焦中 ✓'
                                    ) : img.draft_count > 0 ? (
                                      <>
                                        <Check className="h-2.5 w-2.5" />
                                        已切分 {img.draft_count} 题
                                      </>
                                    ) : (
                                      '未检出题目'
                                    )}
                                  </span>
                                ) : (
                                  <span className="text-[10px] text-destructive font-mono font-bold bg-destructive/10 px-1.5 py-0.5 rounded-md border border-destructive/20">
                                    失败
                                  </span>
                                )}
                              </div>
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )
        })()}

        {/* Message Stream Viewport */}
        <div
          className="flex-1 min-h-0 overflow-y-auto px-4 sm:px-6 py-5"
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <div className="w-full max-w-6xl mx-auto space-y-4">
            {/* 对话消息流列表 */}
            <div className="space-y-4">
              {messages.map((message) => {
                const isUser = message.role === 'user'
                const messageAttachments = isUser
                  ? uploadedImageAttachments(message, detail?.images ?? [], selectedId ?? 0)
                  : []
                if (message.role === 'system') {
                  return (
                    <div key={message.id} className="flex justify-center animate-message-enter">
                      <div className="rounded-full border border-border/60 bg-surface-secondary/80 px-3.5 py-1 text-[11px] text-muted-foreground font-medium shadow-2xs">
                        {message.content}
                      </div>
                    </div>
                  )
                }

                if (isUser) {
                  return (
                    <article
                      key={message.id}
                      className="flex items-start justify-end gap-3 animate-message-enter"
                    >
                      <div className="flex max-w-[85%] sm:max-w-[75%] flex-col items-end gap-2">
                        {/* 1. 多图片缩略图区域 (横向网格卡片排列) */}
                        {messageAttachments.length > 0 && (
                          <div className="flex flex-wrap justify-end gap-2.5">
                            {messageAttachments.map((att, idx) => (
                              <div
                                key={idx}
                                className="group relative w-20 h-20 sm:w-24 sm:h-24 rounded-2xl overflow-hidden border border-border/80 bg-surface shadow-subtle hover:shadow-md transition-all duration-200 cursor-pointer"
                                onClick={() => window.open(att.url, '_blank')}
                                title={att.name || `参考图片 ${idx + 1}`}
                              >
                                <AuthenticatedImage
                                  src={att.url}
                                  alt={att.name || `参考图片 ${idx + 1}`}
                                  className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                                />
                                <div className="absolute inset-0 bg-black/0 group-hover:bg-black/10 transition-colors" />
                              </div>
                            ))}
                          </div>
                        )}

                        {/* 2. 文本消息气泡 (浅色柔和圆角卡片) */}
                        {message.content && (
                          <div className="rounded-2xl rounded-tr-xs border border-border/70 bg-surface-secondary/85 hover:bg-surface-secondary text-foreground px-4 py-3 text-xs sm:text-[13px] leading-relaxed shadow-subtle select-text whitespace-pre-wrap">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
                          </div>
                        )}

                        <span className="px-1 font-mono text-[10px] text-muted-foreground/60">
                          {message.created_at &&
                            new Date(message.created_at).toLocaleTimeString([], {
                              hour: '2-digit',
                              minute: '2-digit',
                            })}
                        </span>
                      </div>
                    </article>
                  )
                }

                // ==================== Agent 智能结构化消息卡片 ====================
                const report = parseAgentExtractionMessage(message.content)

                return (
                  <article
                    key={message.id}
                    className="flex items-start gap-3 animate-message-enter"
                  >
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl border border-primary/20 bg-gradient-to-br from-primary/15 to-primary/5 text-primary shadow-xs mt-0.5">
                      <Bot className="h-4 w-4" />
                    </div>

                    <div className="w-full flex-1 space-y-2">
                      {report.isExtractionMessage ? (
                        <div className="overflow-hidden rounded-2xl border border-border/80 bg-surface shadow-card transition-all duration-200 hover:border-primary/40">
                          {/* 卡片头部：图片名 + 提取成功标签 */}
                          <div className="flex items-center justify-between border-b border-border/50 bg-gradient-to-r from-primary/[0.04] via-surface to-surface px-4 py-3">
                            <div className="flex items-center gap-2.5 min-w-0">
                              <div className="flex h-7 w-7 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20 shadow-2xs shrink-0">
                                <Sparkles className="h-3.5 w-3.5" />
                              </div>
                              <div className="min-w-0">
                                <div className="flex items-center gap-2 flex-wrap">
                                  <span className="text-xs font-bold text-foreground">
                                    {report.isNoQuestionMessage
                                      ? '未检测到完整题目'
                                      : report.isAnswerKeyMessage
                                        ? '答案参考图匹配回填中'
                                        : report.imageName
                                          ? '错题提取就绪'
                                          : '批量切题提取完成'}
                                  </span>
                                  {report.imageName && (
                                    <span
                                      className="font-mono text-[11px] text-muted-foreground truncate max-w-[280px] bg-surface-secondary/70 px-2 py-0.5 rounded-md border border-border/40"
                                      title={report.imageName}
                                    >
                                      {report.imageName}
                                    </span>
                                  )}
                                </div>
                              </div>
                            </div>

                            <div className="flex items-center gap-2 shrink-0">
                              {report.extractedCount !== undefined && (
                                <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-mono font-bold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 shadow-2xs">
                                  <Check className="h-3 w-3" />
                                  已切分 {report.extractedCount} 题
                                </span>
                              )}
                              <span className="font-mono text-[10px] text-muted-foreground/70 hidden sm:inline">
                                {message.created_at &&
                                  new Date(message.created_at).toLocaleTimeString([], {
                                    hour: '2-digit',
                                    minute: '2-digit',
                                  })}
                              </span>
                            </div>
                          </div>

                          {/* 缺答案题目警示与快捷补填区域 */}
                          {report.missingSeqs && report.missingSeqs.length > 0 && (
                            <div className="p-4 space-y-3 bg-amber-500/[0.03] border-b border-amber-500/15">
                              <div className="flex items-center justify-between">
                                <div className="flex items-center gap-1.5 text-xs font-bold text-amber-600 dark:text-amber-400">
                                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                                  <span>{report.missingSeqs.length} 道题未在图片中检测到答案：</span>
                                </div>
                                <span className="text-[11px] text-muted-foreground hidden sm:inline">
                                  点击题号快速填答
                                </span>
                              </div>

                              {/* 题号胶囊芯片列表 */}
                              <div className="flex flex-wrap items-center gap-1.5 max-h-28 overflow-y-auto pr-1">
                                {report.missingSeqs.map((num) => (
                                  <button
                                    key={num}
                                    type="button"
                                    onClick={() => {
                                      setInput((prev) => (prev ? `${prev} ${num}.` : `${num}.`))
                                      textareaRef.current?.focus()
                                    }}
                                    className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-amber-500/30 bg-surface hover:bg-amber-500/10 hover:border-amber-500/50 text-xs font-mono font-bold text-foreground hover:text-amber-600 dark:hover:text-amber-400 transition-all shadow-2xs hover:scale-105 active:scale-95 cursor-pointer"
                                    title={`点击快速在输入框填答第 ${num} 题`}
                                  >
                                    <span>#{num}</span>
                                    <span className="text-[10px] font-normal text-amber-600/80 dark:text-amber-400/80">待填</span>
                                  </button>
                                ))}
                              </div>

                              {/* 快捷操作动作条 */}
                              <div className="flex flex-wrap items-center gap-2 pt-1 border-t border-border/40">
                                <button
                                  type="button"
                                  onClick={() => {
                                    const template = report.missingSeqs!.slice(0, 10).map((n) => `${n}. `).join('')
                                    setInput(template)
                                    textareaRef.current?.focus()
                                  }}
                                  className="inline-flex items-center gap-1.5 px-3 py-1 rounded-xl bg-surface border border-border/80 hover:border-primary/40 hover:bg-primary/5 text-xs font-semibold text-foreground transition-all shadow-2xs hover:scale-105 active:scale-95 cursor-pointer"
                                >
                                  <Edit3 className="h-3 w-3 text-primary" />
                                  <span>快速填答模版 (1. 2. ...)</span>
                                </button>

                                <button
                                  type="button"
                                  onClick={() => {
                                    const rangeText = `第 ${report.missingSeqs![0]} 题至第 ${report.missingSeqs![report.missingSeqs!.length - 1]} 题无需答案`
                                    setInput(rangeText)
                                    textareaRef.current?.focus()
                                  }}
                                  className="inline-flex items-center gap-1.5 px-3 py-1 rounded-xl bg-surface border border-border/80 hover:border-muted-foreground/60 text-xs font-semibold text-muted-foreground hover:text-foreground transition-all shadow-2xs hover:scale-105 active:scale-95 cursor-pointer"
                                >
                                  <CheckSquare className="h-3 w-3" />
                                  <span>全部标记为无需答案</span>
                                </button>

                                <button
                                  type="button"
                                  onClick={() => {
                                    const matchImg = detail?.images?.find((img) => img.original_name === report.imageName)
                                    if (matchImg) {
                                      setActiveImageId(matchImg.id)
                                    }
                                    setDraftFilter('missing_answer')
                                    setIsDraftListOpen(true)
                                  }}
                                  className="inline-flex items-center gap-1.5 px-3 py-1 rounded-xl bg-surface border border-border/80 hover:border-primary/40 text-xs font-semibold text-primary transition-all shadow-2xs hover:scale-105 active:scale-95 cursor-pointer ml-auto"
                                >
                                  <span>在右侧核对缺答案草稿 ›</span>
                                </button>
                              </div>
                            </div>
                          )}

                          {/* 提示文案底部条 */}
                          <div className="px-4 py-2.5 bg-surface-secondary/20 flex items-center justify-between text-[11px] text-muted-foreground">
                            <span className="flex items-center gap-1.5">
                              <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
                              <span>草稿已在右侧工作区列出，可逐题核对与修改，确认无误后批量入库。</span>
                            </span>
                            <button
                              type="button"
                              onClick={() => {
                                const matchImg = detail?.images?.find((img) => img.original_name === report.imageName)
                                if (matchImg) {
                                  setActiveImageId(matchImg.id)
                                }
                                setIsDraftListOpen(true)
                              }}
                              className="text-primary font-semibold hover:underline cursor-pointer ml-2"
                            >
                              核对本图切题 ›
                            </button>
                          </div>
                        </div>
                      ) : (
                        <div className="rounded-2xl rounded-tl-xs border border-border/70 bg-surface p-4 text-xs sm:text-[13px] leading-relaxed text-foreground shadow-card space-y-2">
                          <div className="flex items-center justify-between gap-4 pb-1 border-b border-border/40">
                            <span className="text-xs font-bold text-foreground flex items-center gap-1.5">
                              <span>导入 Agent</span>
                              <span className="h-1.5 w-1.5 rounded-full bg-primary/60" />
                            </span>
                            <span className="font-mono text-[10px] text-muted-foreground">
                              {message.created_at &&
                                new Date(message.created_at).toLocaleTimeString([], {
                                  hour: '2-digit',
                                  minute: '2-digit',
                                })}
                            </span>
                          </div>
                          <div className="markdown-body select-text leading-relaxed text-foreground/90">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
                          </div>
                        </div>
                      )}
                    </div>
                  </article>
                )
              })}
            </div>

            {/* AI Live Thinking & Processing Card */}
            {agentResponding && (
              <div className="flex items-start gap-3 animate-message-enter mt-4" role="status" aria-live="polite">
                <div className="relative flex h-8 w-8 shrink-0 items-center justify-center rounded-xl border border-primary/30 bg-primary/10 text-primary shadow-xs">
                  <span className="absolute inset-0 rounded-xl bg-primary/15 animate-ping" />
                  <Sparkles className="relative h-4 w-4 animate-spin" />
                </div>
                <div className="relative w-fit max-w-[90%] overflow-hidden rounded-2xl rounded-tl-xs border border-primary/25 bg-primary/[0.045] px-4 py-3 text-xs text-foreground shadow-subtle ring-1 ring-primary/10">
                  <div className="pointer-events-none absolute inset-y-0 -left-1/3 w-1/3 bg-gradient-to-r from-transparent via-primary/10 to-transparent animate-shimmer" />
                  <div className="relative space-y-1.5">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-primary">导入 Agent</span>
                      <span className="inline-flex items-center gap-1 text-[10px] font-medium text-primary/75">
                        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-pulse" />
                        正在回复
                      </span>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-foreground/80">
                        {busy === 'upload'
                          ? '正在上传图片并智能提取题目…'
                          : busy === 'message'
                            ? '正在分析回答并校对草稿答案…'
                            : busy === 'answer-reconcile'
                              ? '正在识别答案并匹配右侧题目…'
                              : busy === 'reconcile'
                                ? '正在检查跨图题目…'
                                : `正在整理 ${processingImageCount} 张图片并生成识别结果…`}
                      </span>
                      <div className="flex items-center gap-1 pt-0.5" aria-label="Agent 正在回复">
                        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-typing-1" />
                        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-typing-2" />
                        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-typing-3" />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Empty State when no messages */}
            {detail && messages.length === 0 && (
              <div className="space-y-6 py-12 text-center animate-message-enter">
                <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-3xl bg-gradient-to-b from-primary/15 to-primary/5 text-primary border border-primary/20 shadow-glow">
                  <ImagePlus className="h-8 w-8" />
                </div>
                <div className="mx-auto max-w-md space-y-2">
                  <h3 className="text-base font-bold tracking-tight text-foreground">上传错题图片开始智能导入</h3>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    支持多张试卷/错题截图，一张图可自动切分多道题。Agent 会自动识别题目与答案，右侧可直接核对修改，确认后批量入库。
                  </p>
                </div>

                {/* Quick action helper chips */}
                <div className="flex flex-wrap items-center justify-center gap-2 max-w-lg mx-auto pt-2">
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    className="inline-flex items-center gap-1.5 rounded-full border border-border/80 bg-surface-secondary/70 hover:bg-surface hover:border-primary/40 px-3.5 py-1.5 text-xs text-foreground font-medium transition-all shadow-subtle"
                  >
                    <ImagePlus className="h-3.5 w-3.5 text-primary" />
                    <span>选择错题图片</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setInput('请问支持哪些答题格式？')
                      textareaRef.current?.focus()
                    }}
                    className="inline-flex items-center gap-1.5 rounded-full border border-border/80 bg-surface-secondary/70 hover:bg-surface hover:border-primary/40 px-3.5 py-1.5 text-xs text-muted-foreground hover:text-foreground transition-all shadow-subtle"
                  >
                    <HelpCircle className="h-3.5 w-3.5 text-muted-foreground" />
                    <span>查看补充答案格式说明</span>
                  </button>
                </div>
              </div>
            )}

            {!detail && (
              <div className="py-24 text-center text-xs text-muted-foreground flex flex-col items-center justify-center gap-2">
                <Spinner className="h-5 w-5 text-primary" />
                <span>正在加载会话数据…</span>
              </div>
            )}

            <div ref={messagesEndRef} />
          </div>
        </div>

        {/* ==================== 底部现代 AI Agent 输入区域 ==================== */}
        <footer
          className="border-t border-border/60 bg-surface/95 backdrop-blur-md p-3.5 sm:p-4 shrink-0"
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <div className="w-full max-w-5xl mx-auto space-y-2.5">
            {/* Pending Files Preview Carousel */}
            {pendingFiles.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 p-2 rounded-xl bg-surface-secondary/50 border border-border/60 animate-slide-down">
                {pendingFiles.map((file) => (
                  <div
                    key={`${file.name}-${file.size}`}
                    title={file.name}
                    className="group/file relative h-16 w-16 shrink-0 overflow-hidden rounded-xl border border-border/80 shadow-subtle bg-surface"
                  >
                    <img
                      src={filePreviews.get(file)}
                      alt={file.name}
                      draggable={false}
                      className="h-full w-full object-cover transition-transform duration-200 group-hover/file:scale-105"
                    />
                    {uploadProgress[file.name] !== undefined && (
                      <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/60 text-[10px] font-bold text-white backdrop-blur-2xs">
                        <Spinner className="h-3 w-3 mb-1" />
                        {uploadProgress[file.name]}%
                      </div>
                    )}
                    <button
                      type="button"
                      title="移除此图片"
                      onClick={() => setPendingFiles((items) => items.filter((item) => item !== file))}
                      className="absolute right-1 top-1 flex h-4 w-4 items-center justify-center rounded-full bg-black/70 text-white transition-all hover:bg-destructive hover:scale-110"
                    >
                      <X className="h-2.5 w-2.5" />
                    </button>
                  </div>
                ))}
                <div className="flex flex-col justify-center pl-1 text-[11px] text-muted-foreground">
                  <span className="font-medium text-foreground">已选择 {pendingFiles.length} 张错题截图</span>
                  <span className="text-[10px]">按 Enter 或点击发送按钮开始提取题目</span>
                </div>
              </div>
            )}

            {/* Smart Suggestion Chips above composer if missing answers exist */}
            {missingCount > 0 && pendingFiles.length === 0 && (
              <div className="flex items-center gap-1.5 overflow-x-auto pb-0.5 no-scrollbar text-xs animate-fade-in">
                <span className="text-[11px] font-semibold text-muted-foreground shrink-0 flex items-center gap-1">
                  <Zap className="h-3 w-3 text-amber-500" />
                  <span>快捷回复：</span>
                </span>
                <button
                  type="button"
                  onClick={() => {
                    setInput('1.B 2.对 3.ABC')
                    textareaRef.current?.focus()
                  }}
                  className="px-2.5 py-0.5 rounded-lg border border-border/70 bg-surface hover:bg-surface-secondary hover:border-primary/40 text-[11px] font-mono text-muted-foreground hover:text-foreground transition-colors shrink-0 shadow-2xs"
                >
                  「1.B 2.对 3.ABC」
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setInput('本批题目均为主观题无需答案')
                    textareaRef.current?.focus()
                  }}
                  className="px-2.5 py-0.5 rounded-lg border border-border/70 bg-surface hover:bg-surface-secondary hover:border-primary/40 text-[11px] text-muted-foreground hover:text-foreground transition-colors shrink-0 shadow-2xs"
                >
                  「主观题无需答案」
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setDraftFilter('missing_answer')
                    setIsDraftListOpen(true)
                  }}
                  className="px-2.5 py-0.5 rounded-lg border border-amber-500/30 bg-amber-500/5 hover:bg-amber-500/10 text-[11px] text-amber-600 dark:text-amber-400 font-medium transition-colors shrink-0 shadow-2xs ml-auto"
                >
                  在右侧查看 {missingCount} 道待补答题目 ›
                </button>
              </div>
            )}

            {/* Composer Box (Linear / Claude style) */}
            <div
              className={`group relative rounded-2xl border bg-surface-secondary/40 focus-within:bg-surface focus-within:border-primary/60 focus-within:ring-4 focus-within:ring-primary/10 transition-all duration-200 shadow-subtle ${
                isDraggingOver ? 'border-primary border-dashed bg-primary/5 ring-4 ring-primary/10' : 'border-border/80'
              }`}
            >
              {/* Textarea */}
              <textarea
                ref={textareaRef}
                rows={1}
                value={input}
                onPaste={handlePaste}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                    e.preventDefault()
                    void submitComposer()
                  }
                }}
                placeholder={
                  missingCount > 0
                    ? `发现 ${missingCount} 道题缺答案，输入如「1.B 2.对」快速补齐，或上传更多图片…`
                    : pendingFiles.length > 0
                      ? '可输入补充说明（可选），回车开始识别…'
                      : '上传错题图片、补充题目答案，或向导入助手提问…'
                }
                disabled={busy === 'message' || busy === 'upload'}
                className="max-h-[140px] min-h-[48px] w-full resize-none bg-transparent px-3.5 pt-3 pb-2 text-xs sm:text-[13px] text-foreground placeholder:text-muted-foreground/60 focus:outline-none leading-relaxed"
              />

              {/* Action Toolbar */}
              <div className="flex items-center justify-between px-2.5 pb-2 pt-0.5">
                <div className="flex items-center gap-1.5 min-w-0">
                  <button
                    type="button"
                    title="添加错题图片（支持 PNG / JPG，可多选、拖拽或剪贴板粘贴）"
                    onClick={() => fileInputRef.current?.click()}
                    disabled={busy === 'upload'}
                    className="flex h-7 items-center gap-1.5 rounded-lg px-2.5 text-xs text-muted-foreground hover:bg-surface hover:text-primary transition-all disabled:opacity-50 border border-transparent hover:border-border/60 hover:shadow-2xs shrink-0 whitespace-nowrap cursor-pointer"
                  >
                    <ImagePlus className="h-3.5 w-3.5 text-primary" />
                    <span className="hidden sm:inline text-[11px] font-semibold">添加图片</span>
                  </button>
                </div>

                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    size="xs"
                    variant="primary"
                    onClick={() => void submitComposer()}
                    disabled={Boolean(busy) || (!input.trim() && pendingFiles.length === 0)}
                    loading={busy === 'upload' || busy === 'message'}
                    icon={<Send className="h-3.5 w-3.5" />}
                    className="h-7 px-3.5 rounded-lg text-xs font-semibold shadow-subtle active:scale-95 whitespace-nowrap"
                  >
                    <span>发送</span>
                  </Button>
                </div>
              </div>
            </div>

            {/* Hidden File Input */}
            <input
              ref={fileInputRef}
              type="file"
              accept="image/png,image/jpeg"
              multiple
              className="hidden"
              onChange={(e) => {
                const files = Array.from(e.target.files ?? []).filter((f) => f.type.startsWith('image/'))
                if (files.length) setPendingFiles((current) => [...current, ...files])
                if (fileInputRef.current) fileInputRef.current.value = ''
              }}
            />

            {/* Bottom microhint */}
            <div className="flex items-center justify-between px-1 text-[10px] text-muted-foreground/60">
              <span className="whitespace-nowrap">Enter 发送，Shift + Enter 换行</span>
              <span className="hidden sm:inline whitespace-nowrap">支持直接拖拽文件或粘贴剪贴板截图</span>
            </div>
          </div>
        </footer>
      </main>

      {/* ==================== 3. 右栏：题目草稿工作区 ==================== */}
      <aside
        className={`flex flex-col shrink-0 border-l border-border/60 bg-surface-secondary/15 overflow-hidden z-10 transition-all duration-300 ease-natural ${
          isDraftListOpen
            ? 'w-[380px] sm:w-[400px] lg:w-[420px] xl:w-[450px] 2xl:w-[480px] opacity-100'
            : 'w-0 border-l-0 opacity-0 pointer-events-none'
        }`}
      >
        {/* Drafts Header */}
        <div className="border-b border-border/60 bg-surface/70 px-4 py-3 backdrop-blur-xs">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="flex h-6 w-6 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Images className="h-3.5 w-3.5" />
              </div>
              <h3 className="text-xs font-bold text-foreground">题目草稿核对</h3>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {canReconcile && session?.status === 'active' && (
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => void checkCrossImageQuestions()}
                  disabled={Boolean(busy)}
                  loading={busy === 'reconcile'}
                  icon={<RefreshCw className="h-3 w-3" />}
                  className="h-7 rounded-lg px-2 text-[10px] font-semibold text-primary hover:bg-primary/10"
                  title="扫描当前会话相邻图片，识别被截断的同一道题"
                >
                  检查跨图题目
                </Button>
              )}
              {answerImageCount > 0 && session?.status === 'active' && (
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => void retryAnswerExtraction()}
                  disabled={Boolean(busy)}
                  loading={busy === 'answer-reconcile'}
                  icon={<RefreshCw className="h-3 w-3" />}
                  className="h-7 rounded-lg px-2 text-[10px] font-semibold text-primary hover:bg-primary/10"
                  title="重新识别当前会话中的答案图片，并合并连续答案片段"
                >
                  重新匹配答案
                </Button>
              )}
              <span className="font-mono text-[11px] font-bold text-muted-foreground bg-surface-secondary px-2 py-0.5 rounded-full border border-border/60 whitespace-nowrap">
                共 {drafts.length} 道
              </span>
            </div>
          </div>

          {reconcilingBatch && (
            <div className="mt-2 flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-2.5 py-1.5 text-[10px] font-medium text-primary">
              <Loader2 className="h-3 w-3 animate-spin" />
              {reconcilingBatch.status === 'reconciling' ? '正在检查跨图题目…' : '等待图片识别完成后检查跨图题目…'}
            </div>
          )}
          {!reconcilingBatch && latestBatch?.status === 'done' && (
            <div className="mt-2 flex items-center gap-2 rounded-lg border border-emerald-500/20 bg-emerald-500/5 px-2.5 py-1.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-300">
              <CheckCircle2 className="h-3 w-3" />
              已完成跨图检查
            </div>
          )}
          {unresolvedAnswerFragments.length > 0 && (
            <div className="mt-2 flex items-center gap-2 rounded-lg border border-amber-500/25 bg-amber-500/5 px-2.5 py-1.5 text-[10px] font-medium text-amber-700 dark:text-amber-300">
              <AlertCircle className="h-3 w-3 shrink-0" />
              有 {unresolvedAnswerFragments.length} 个答案片段未匹配到题目，已保留原文；可点击“重新匹配答案”重试。
            </div>
          )}

          {/* Segmented Pill Filters */}
          <div className="mt-2.5 flex items-center gap-1.5 overflow-x-auto pb-0.5 no-scrollbar">
            {filterMeta.map((f) => {
              const count =
                f.key === 'all'
                  ? drafts.length
                  : f.key === 'missing_answer'
                    ? missingCount
                    : f.key === 'ready'
                      ? readyCount
                      : f.key === 'excluded'
                        ? excludedCount
                        : committedCount
              const isActive = draftFilter === f.key
              return (
                <button
                  key={f.key}
                  type="button"
                  onClick={() => setDraftFilter(f.key)}
                  className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] font-medium transition-all duration-150 shrink-0 ${
                    isActive
                      ? 'bg-primary text-primary-foreground shadow-subtle'
                      : 'bg-surface hover:bg-surface-secondary text-muted-foreground hover:text-foreground border border-border/60'
                  }`}
                >
                  <span>{f.label}</span>
                  <span
                    className={`font-mono text-[10px] rounded-full px-1.5 py-0 ${
                      isActive
                        ? 'bg-primary-foreground/20 text-primary-foreground'
                        : f.key === 'missing_answer' && count > 0
                          ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400 font-bold'
                          : 'bg-surface-secondary text-muted-foreground'
                    }`}
                  >
                    {count}
                  </span>
                </button>
              )
            })}
          </div>

          {/* Active Image Focus Indicator Banner */}
          {activeImage && (
            <div className="mt-2.5 flex items-center justify-between gap-2 rounded-xl border border-primary/30 bg-primary/[0.08] px-3 py-1.5 text-xs text-primary shadow-2xs animate-slide-down">
              <div className="flex items-center gap-1.5 min-w-0">
                <Camera className="h-3.5 w-3.5 shrink-0" />
                <span className="truncate font-semibold text-[11px]">
                  正在核对「{activeImage.original_name || `图片 #${activeImage.id}`}」切出的 {filteredDrafts.length} 道题
                </span>
              </div>
              <button
                type="button"
                onClick={() => setActiveImageId(null)}
                className="text-[10px] font-bold underline hover:opacity-80 shrink-0 cursor-pointer text-primary"
              >
                查看全部 ×
              </button>
            </div>
          )}
        </div>

        {/* Drafts Cards List */}
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3.5">
          {pendingMerges.length > 0 && (
            <div className="space-y-2 rounded-2xl border border-amber-500/30 bg-amber-500/[0.04] p-3 animate-slide-down">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-xs font-bold text-foreground">
                  <AlertCircle className="h-4 w-4 text-amber-500" />
                  <span>发现 {pendingMerges.length} 个可能的跨图题目</span>
                </div>
                <span className="text-[10px] text-muted-foreground">请确认是否合并</span>
              </div>
              {pendingMerges.map((candidate) => {
                const left = drafts.find((draft) => draft.id === candidate.left_draft_id)
                const right = drafts.find((draft) => draft.id === candidate.right_draft_id)
                return (
                  <div key={candidate.id} className="rounded-xl border border-amber-500/20 bg-surface px-2.5 py-2.5 space-y-2">
                    <p className="text-[11px] leading-relaxed text-foreground/90">
                      第 {left?.seq_no ?? candidate.left_draft_id} 题和第 {right?.seq_no ?? candidate.right_draft_id} 题可能属于同一道题
                    </p>
                    <p className="text-[10px] leading-relaxed text-muted-foreground">{mergeReasonLabel(candidate.reason_code)}</p>
                    <div className="flex items-center justify-end gap-1.5">
                      <Button
                        size="xs"
                        variant="ghost"
                        onClick={() => void resolveMergeCandidate(candidate, 'reject')}
                        disabled={Boolean(busy)}
                        className="h-6 px-2 text-[10px]"
                      >
                        保持独立
                      </Button>
                      <Button
                        size="xs"
                        variant="primary"
                        onClick={() => void resolveMergeCandidate(candidate, 'accept')}
                        disabled={Boolean(busy)}
                        loading={busy === `merge-${candidate.id}`}
                        className="h-6 px-2 text-[10px]"
                      >
                        合并
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}

          {acceptedMerges.length > 0 && (
            <div className="space-y-2 rounded-2xl border border-emerald-500/25 bg-emerald-500/[0.04] p-3 animate-slide-down">
              {acceptedMerges.slice(0, 3).map((candidate) => {
                const left = drafts.find((draft) => draft.id === candidate.left_draft_id)
                return (
                  <div key={candidate.id} className="flex items-center justify-between gap-2 text-[10px] text-emerald-700 dark:text-emerald-300">
                    <span className="flex min-w-0 items-center gap-1.5">
                      <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
                      <span className="truncate">第 {left?.seq_no ?? candidate.left_draft_id} 题已合并跨图内容</span>
                    </span>
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => void undoMerge(candidate)}
                      disabled={Boolean(busy)}
                      loading={busy === `undo-${candidate.id}`}
                      className="h-6 shrink-0 px-2 text-[10px] text-emerald-700 dark:text-emerald-300"
                    >
                      撤销合并
                    </Button>
                  </div>
                )
              })}
            </div>
          )}

          {filteredDrafts.length === 0 ? (
            <div className="space-y-3 px-4 py-16 text-center animate-message-enter">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl border border-border/80 bg-surface text-muted-foreground shadow-subtle">
                <Images className="h-6 w-6" />
              </div>
              <h4 className="text-xs font-bold text-foreground">
                {drafts.length === 0 ? '暂无题目草稿' : '该分类下暂无题目'}
              </h4>
              <p className="text-[11px] leading-relaxed text-muted-foreground max-w-xs mx-auto">
                {drafts.length === 0
                  ? '上传错题图片后，提取出的题目草稿将在此列出，可逐题核对修改后再确认入库。'
                  : '可切换上方筛选标签查看其他题目。'}
              </p>
            </div>
          ) : (
            filteredDrafts.map((draft) => (
              <WrongDraftCard
                key={draft.id}
                draft={draft}
                sessionId={selectedId ?? 0}
                selected={selectedIds.has(draft.id)}
                active={activeDraftId === draft.id}
                onToggleSelect={(id) =>
                  setSelectedIds((prev) => {
                    const next = new Set(prev)
                    if (next.has(id)) next.delete(id)
                    else next.add(id)
                    return next
                  })
                }
                onOpenDetail={(targetDraft) => {
                  setActiveDraftId(targetDraft.id)
                  setActiveDraftModalId(targetDraft.id)
                }}
                onUpdated={(updated) =>
                  setDetail((d) =>
                    d ? { ...d, drafts: d.drafts.map((x) => (x.id === updated.id ? updated : x)) } : d,
                  )
                }
                onDeleted={(id) => {
                  setDetail((d) => (d ? { ...d, drafts: d.drafts.filter((x) => x.id !== id) } : d))
                  if (selectedId) void loadDetail(selectedId)
                }}
              />
            ))
          )}
        </div>

        {/* Sticky Action Footer */}
        <div className="space-y-2.5 border-t border-border/60 bg-surface p-3.5 shadow-elevated">
          <div className="flex items-center gap-2">
            <select
              className="input h-8 min-w-0 flex-1 rounded-xl py-0 pl-2.5 pr-6 text-xs bg-surface-secondary/40 border-border/60"
              value={bankId}
              onChange={(e) => setBankId(e.target.value ? Number(e.target.value) : '')}
              title="选择入库目标题库"
            >
              <option value="">我的错题本（默认）</option>
              {banks
                .filter((b) => b.name !== '我的错题本')
                .map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.name}
                  </option>
                ))}
            </select>
            <Button
              size="sm"
              variant="primary"
              onClick={() => void confirmImport()}
              disabled={Boolean(busy) || selectedReadyIds.length === 0}
              loading={busy === 'confirm'}
              icon={<CheckCircle2 className="h-3.5 w-3.5" />}
              title={missingCount > 0 ? '缺答案的题需先补齐后方可入库' : ''}
              className="h-8 px-3.5 rounded-xl font-bold shadow-subtle shrink-0"
            >
              确认入库 ({selectedReadyIds.length})
            </Button>
          </div>

          {session?.status === 'committed' && (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300 animate-slide-down">
              <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-500" />
              <span>本批错题已全部处理入库，可点击左侧「新会话」继续导入。</span>
            </div>
          )}
        </div>
      </aside>

      {/* ==================== 删除会话确认弹窗 ==================== */}
      <Modal
        open={Boolean(sessionToDelete)}
        onClose={() => !busy && setSessionToDelete(null)}
        title="删除导入会话"
        description="请确认是否要删除此导入会话及其关联的所有提取数据。"
        width="max-w-md"
      >
        {sessionToDelete && (
          <div className="space-y-4">
            {/* 会话信息卡片 */}
            <div className="rounded-2xl border border-border/80 bg-surface-secondary/40 p-4 space-y-2">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                  <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-surface border border-border/60 text-primary shadow-2xs">
                    <Sparkles className="h-3.5 w-3.5" />
                  </div>
                  <span className="text-xs font-bold text-foreground truncate">
                    {sessionToDelete.title || `会话 #${sessionToDelete.id}`}
                  </span>
                </div>
                <Badge tone={statusTone[sessionToDelete.status] || 'neutral'} variant="subtle" className="text-[10px]">
                  {statusLabel[sessionToDelete.status] || sessionToDelete.status}
                </Badge>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono pl-0.5">
                <span>共 {sessionToDelete.draft_count} 道草稿</span>
                <span>•</span>
                <span className="text-emerald-600 dark:text-emerald-400">{sessionToDelete.ready_count} 道就绪</span>
                {sessionToDelete.missing_answer_count > 0 && (
                  <>
                    <span>•</span>
                    <span className="text-amber-600 dark:text-amber-400">{sessionToDelete.missing_answer_count} 道缺答案</span>
                  </>
                )}
              </div>
            </div>

            {/* 警告说明 */}
            <div className="flex items-start gap-3 rounded-2xl border border-destructive/25 bg-destructive/[0.04] p-3.5 text-xs text-foreground">
              <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-destructive/10 text-destructive mt-0.5 shadow-2xs">
                <AlertCircle className="h-4 w-4" />
              </div>
              <div className="space-y-1">
                <p className="font-semibold text-destructive">此操作不可撤销</p>
                <p className="text-[11px] text-muted-foreground leading-relaxed">
                  删除后，当前会话中的所有待切分图片、识别草稿与对话记录将被归档移除。已确认入库到题库中的题目不会受到影响。
                </p>
              </div>
            </div>

            {/* 底部按钮栏 */}
            <div className="flex items-center justify-end gap-2.5 pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSessionToDelete(null)}
                disabled={Boolean(busy)}
                className="h-9 px-4 rounded-xl text-xs font-medium"
              >
                取消
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => void executeDeleteSession()}
                loading={busy === `delete-${sessionToDelete.id}`}
                icon={<Trash2 className="h-3.5 w-3.5" />}
                className="h-9 px-4 rounded-xl text-xs font-semibold shadow-subtle"
              >
                确认删除会话
              </Button>
            </div>
          </div>
        )}
      </Modal>

      {/* 题目详情弹窗 */}
      <WrongDraftDetailModal
          draft={activeModalDraft}
        allDrafts={filteredDrafts}
        sessionId={selectedId ?? 0}
        isOpen={activeDraftModalId !== null && Boolean(activeModalDraft)}
        onClose={() => setActiveDraftModalId(null)}
        onSelectDraft={(targetDraft) => {
          setActiveDraftId(targetDraft.id)
          setActiveDraftModalId(targetDraft.id)
        }}
        onUpdated={(updated) =>
          setDetail((d) =>
            d ? { ...d, drafts: d.drafts.map((x) => (x.id === updated.id ? updated : x)) } : d,
          )
        }
        onDeleted={(id) => {
          setDetail((d) => (d ? { ...d, drafts: d.drafts.filter((x) => x.id !== id) } : d))
          setActiveDraftId((current) => (current === id ? null : current))
          setActiveDraftModalId(null)
          if (selectedId) void loadDetail(selectedId)
        }}
      />
    </div>
  )
}
