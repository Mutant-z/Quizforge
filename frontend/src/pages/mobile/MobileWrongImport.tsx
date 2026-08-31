import React, { useCallback, useEffect, useRef, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { uploadWrongImportImage, wrongImportApi } from '@/api/wrongImport'
import { Badge, Button, Spinner } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { showToast } from '@/components/mobile/MobileToast'
import type {
  WrongImportDraft,
  WrongImportImage,
  WrongImportMessage,
  WrongImportSession,
  WrongImportSessionDetail,
} from '@/types/wrongImport'
import {
  AlertCircle,
  ArrowRight,
  Bot,
  Camera,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Edit3,
  EyeOff,
  FileQuestion,
  FileText,
  FolderOpen,
  HelpCircle,
  ImagePlus,
  Layers,
  ListFilter,
  Paperclip,
  Plus,
  RefreshCw,
  Send,
  Sparkles,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface BankOption {
  id: number
  name: string
}

type DraftFilter = 'all' | 'missing_answer' | 'ready' | 'excluded' | 'committed'

const filterLabels: { key: DraftFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'ready', label: '就绪' },
  { key: 'missing_answer', label: '缺答案' },
  { key: 'excluded', label: '已排除' },
  { key: 'committed', label: '已入库' },
]

export default function MobileWrongImport() {
  const [sessions, setSessions] = useState<WrongImportSession[]>([])
  const [sessionDetail, setSessionDetail] = useState<WrongImportSessionDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [messages, setMessages] = useState<WrongImportMessage[]>([])
  const [drafts, setDrafts] = useState<WrongImportDraft[]>([])
  const [images, setImages] = useState<WrongImportImage[]>([])
  const [inputText, setInputText] = useState('')
  const [sending, setSending] = useState(false)
  const [uploading, setUploading] = useState(false)

  // Draft filter & extraction mode
  const [draftFilter, setDraftFilter] = useState<DraftFilter>('all')
  const [extractionMode, setExtractionMode] = useState<'questions' | 'answer_key'>('questions')

  // Session Manager Sheet
  const [sessionSheetOpen, setSessionSheetOpen] = useState(false)
  const [newSessionTitle, setNewSessionTitle] = useState('')
  const [creatingSession, setCreatingSession] = useState(false)

  // Commit Modal Sheet
  const [commitOpen, setCommitOpen] = useState(false)
  const [banks, setBanks] = useState<BankOption[]>([])
  const [selectedBankId, setSelectedBankId] = useState<number | null>(null)
  const [committing, setCommitting] = useState(false)

  // Draft Edit Sheet
  const [editingDraft, setEditingDraft] = useState<WrongImportDraft | null>(null)
  const [editingStem, setEditingStem] = useState('')
  const [editingAnswer, setEditingAnswer] = useState('')
  const [editingAnalysis, setEditingAnalysis] = useState('')
  const [savingDraft, setSavingDraft] = useState(false)

  // Tab: 'chat' vs 'drafts'
  const [activeTab, setActiveTab] = useState<'chat' | 'drafts'>('chat')

  const fileInputRef = useRef<HTMLInputElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  // Load session list and active session
  const loadSessions = useCallback(async (preferredId?: number) => {
    setLoading(true)
    try {
      const sessionList = await wrongImportApi.listSessions()
      setSessions(sessionList)
      let targetId = preferredId || sessionList[0]?.id

      if (!targetId) {
        const newSession = await wrongImportApi.createSession('移动端错题快拍导入')
        targetId = newSession.id
        setSessions([newSession])
      }

      const detail = await wrongImportApi.getSession(targetId)
      setSessionDetail(detail)
      setMessages(detail.messages || [])
      setDrafts(detail.drafts || [])
      setImages(detail.images || [])
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSessions()
    // Load banks for commit
    client.get('/question-banks', { params: { page_size: 50 } }).then((r) => {
      const bList: BankOption[] = (r.data.data.items || []).map((b: { id: number; name: string }) => ({
        id: b.id,
        name: b.name,
      }))
      setBanks(bList)
      if (bList.length > 0) setSelectedBankId(bList[0].id)
    }).catch(() => {})
  }, [loadSessions])

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages, sending])

  // Switch session
  const handleSwitchSession = async (sessionId: number) => {
    setSessionSheetOpen(false)
    await loadSessions(sessionId)
  }

  // Create new session
  const handleCreateNewSession = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newSessionTitle.trim()) return
    setCreatingSession(true)
    try {
      const s = await wrongImportApi.createSession(newSessionTitle.trim())
      setNewSessionTitle('')
      setSessionSheetOpen(false)
      showToast('已新建导入会话', 'success')
      await loadSessions(s.id)
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setCreatingSession(false)
    }
  }

  // Handle Photo / Image Upload
  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (!files || files.length === 0 || !sessionDetail) return
    setUploading(true)
    try {
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        await uploadWrongImportImage(sessionDetail.session.id, file, undefined, {
          id: String(Date.now()),
          index: i,
          size: files.length,
          extractionMode,
        })
      }
      showToast('图片上传成功，AI 正在分析', 'success')
      const updated = await wrongImportApi.getSession(sessionDetail.session.id)
      setSessionDetail(updated)
      setImages(updated.images || [])
      setDrafts(updated.drafts || [])
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  // Send Chat message to trigger AI extraction
  const handleSendMessage = async (e?: React.FormEvent) => {
    if (e) e.preventDefault()
    const content = inputText.trim()
    if (!content) return
    if (!sessionDetail || sending) return

    setSending(true)
    setInputText('')

    try {
      const msg = await wrongImportApi.sendMessage(sessionDetail.session.id, content)
      setMessages((prev) => [...prev, msg])
      const updated = await wrongImportApi.getSession(sessionDetail.session.id)
      setSessionDetail(updated)
      setMessages(updated.messages || [])
      setDrafts(updated.drafts || [])
      showToast('AI 试题提取完成', 'success')
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setSending(false)
    }
  }

  // Edit draft open
  const openEditDraft = (draft: WrongImportDraft) => {
    setEditingDraft(draft)
    setEditingStem(draft.stem)
    setEditingAnswer(Array.isArray(draft.answer) ? draft.answer.join(', ') : String(draft.answer || ''))
    setEditingAnalysis(draft.analysis || '')
  }

  const saveDraft = async () => {
    if (!editingDraft || !sessionDetail) return
    setSavingDraft(true)
    try {
      const answerArr = editingAnswer ? editingAnswer.split(/[,，、]/).map((s) => s.trim()).filter(Boolean) : []
      await wrongImportApi.patchDraft(sessionDetail.session.id, editingDraft.id, {
        stem: editingStem,
        answer: answerArr,
        analysis: editingAnalysis,
        review_status: answerArr.length > 0 ? 'ready' : 'missing_answer',
      })
      showToast('草稿已更新', 'success')
      setEditingDraft(null)
      const updated = await wrongImportApi.getSession(sessionDetail.session.id)
      setDrafts(updated.drafts || [])
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setSavingDraft(false)
    }
  }

  const toggleExcludeDraft = async (draft: WrongImportDraft) => {
    if (!sessionDetail) return
    const nextStatus = draft.review_status === 'excluded' ? 'ready' : 'excluded'
    try {
      await wrongImportApi.patchDraft(sessionDetail.session.id, draft.id, { review_status: nextStatus })
      showToast(nextStatus === 'excluded' ? '已排除该草稿' : '已恢复该草稿', 'info')
      const updated = await wrongImportApi.getSession(sessionDetail.session.id)
      setDrafts(updated.drafts || [])
    } catch (err) {
      showToast(errMsg(err), 'error')
    }
  }

  const handleDeleteDraft = async (draftId: number) => {
    if (!sessionDetail) return
    try {
      await wrongImportApi.deleteDraft(sessionDetail.session.id, draftId)
      showToast('草稿已删除', 'success')
      const updated = await wrongImportApi.getSession(sessionDetail.session.id)
      setDrafts(updated.drafts || [])
    } catch (err) {
      showToast(errMsg(err), 'error')
    }
  }

  // Commit drafts to question bank
  const handleCommit = async () => {
    if (!sessionDetail || !selectedBankId) return
    const readyDrafts = drafts.filter((d) => d.review_status === 'ready' || (!d.review_status && d.answer?.length > 0))
    if (readyDrafts.length === 0) {
      showToast('没有可入库的就绪草稿题目', 'warning')
      return
    }

    setCommitting(true)
    try {
      await wrongImportApi.confirm(
        sessionDetail.session.id,
        readyDrafts.map((d) => d.id),
        selectedBankId
      )
      showToast(`成功将 ${readyDrafts.length} 道错题存入题库！`, 'success')
      setCommitOpen(false)
      loadSessions(sessionDetail.session.id)
    } catch (err) {
      showToast(errMsg(err), 'error')
    } finally {
      setCommitting(false)
    }
  }

  const readyCount = drafts.filter((d) => d.review_status === 'ready' || (d.review_status !== 'excluded' && d.review_status !== 'committed' && d.answer?.length > 0)).length

  // Filtered drafts
  const filteredDrafts = drafts.filter((d) => {
    if (draftFilter === 'all') return true
    if (draftFilter === 'ready') return d.review_status === 'ready' || (d.review_status !== 'excluded' && d.review_status !== 'committed' && d.answer?.length > 0)
    if (draftFilter === 'missing_answer') return d.review_status === 'missing_answer' || (!d.answer || d.answer.length === 0)
    if (draftFilter === 'excluded') return d.review_status === 'excluded'
    if (draftFilter === 'committed') return d.review_status === 'committed'
    return true
  })

  return (
    <div className="flex flex-col h-full bg-background select-none">
      {/* Hidden file input */}
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        onChange={handleFileChange}
      />

      {/* Top Segmented Tabs & Session trigger */}
      <div className="sticky top-0 z-20 flex items-center justify-between px-3.5 py-2 bg-surface/90 backdrop-blur-md border-b border-border/60 gap-2">
        <div className="flex items-center gap-1 rounded-2xl bg-surface-secondary p-1 border border-border/70">
          <button
            onClick={() => setActiveTab('chat')}
            className={`py-1 px-3 rounded-xl text-xs font-semibold transition-all ${
              activeTab === 'chat' ? 'bg-surface text-foreground font-bold shadow-2xs' : 'text-muted-foreground'
            }`}
          >
            AI 拍照识别
          </button>
          <button
            onClick={() => setActiveTab('drafts')}
            className={`flex items-center gap-1 py-1 px-3 rounded-xl text-xs font-semibold transition-all ${
              activeTab === 'drafts' ? 'bg-surface text-foreground font-bold shadow-2xs' : 'text-muted-foreground'
            }`}
          >
            <span>草稿箱</span>
            {drafts.length > 0 && (
              <span className="rounded-full bg-primary/10 px-1.5 py-0.2 text-[9px] font-mono text-primary font-bold">
                {drafts.length}
              </span>
            )}
          </button>
        </div>

        <div className="flex items-center gap-1.5">
          {/* Session Switcher Button */}
          <button
            onClick={() => setSessionSheetOpen(true)}
            className="flex items-center gap-1 rounded-xl bg-surface-secondary px-2 py-1 text-[11px] font-bold text-muted-foreground hover:text-foreground border border-border/60"
            title="切换导入会话"
          >
            <FolderOpen className="h-3 w-3" />
            <span className="max-w-[70px] truncate">{sessionDetail?.session.title || '会话'}</span>
          </button>

          {readyCount > 0 && (
            <Button
              size="xs"
              onClick={() => setCommitOpen(true)}
              icon={<CheckCircle2 className="h-3 w-3" />}
              className="shadow-glow"
            >
              入库 ({readyCount})
            </Button>
          )}
        </div>
      </div>

      {/* Main Viewport */}
      {activeTab === 'chat' ? (
        <div className="flex-1 flex flex-col min-h-0">
          {/* Messages Stream */}
          <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-3.5 touch-pan-y min-h-0 text-xs">
            {/* Greeting card */}
            <div className="rounded-3xl border border-primary/20 bg-primary/5 p-4 space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 text-primary font-bold text-xs">
                  <Sparkles className="h-4 w-4" />
                  <span>智能错题图片提取工作台</span>
                </div>
                <div className="flex items-center gap-1 rounded-full bg-surface p-0.5 border border-border">
                  <button
                    onClick={() => setExtractionMode('questions')}
                    className={`px-2 py-0.5 rounded-full text-[10px] font-bold transition-all ${
                      extractionMode === 'questions' ? 'bg-primary text-white' : 'text-muted-foreground'
                    }`}
                  >
                    拍错题
                  </button>
                  <button
                    onClick={() => setExtractionMode('answer_key')}
                    className={`px-2 py-0.5 rounded-full text-[10px] font-bold transition-all ${
                      extractionMode === 'answer_key' ? 'bg-primary text-white' : 'text-muted-foreground'
                    }`}
                  >
                    拍答案
                  </button>
                </div>
              </div>
              <p className="text-muted-foreground leading-relaxed text-[11px]">
                支持拍摄考卷、平板错题截图。模式：{extractionMode === 'questions' ? '【提取试题与解析】' : '【提取答题卡并回填答案】'}。
              </p>
            </div>

            {/* Uploaded images preview gallery */}
            {images.length > 0 && (
              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block px-1">
                  已附加图片 ({images.length})
                </span>
                <div className="flex items-center gap-2 overflow-x-auto no-scrollbar py-1">
                  {images.map((img) => (
                    <div
                      key={img.id}
                      className="relative h-16 w-16 shrink-0 rounded-2xl overflow-hidden border border-border bg-surface-secondary shadow-2xs"
                    >
                      <img
                        src={`/api/v1/wrong-import/sessions/${sessionDetail?.session.id}/images/${img.id}/file`}
                        alt="Upload"
                        className="h-full w-full object-cover"
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Chat Messages */}
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}
              >
                {msg.role === 'user' ? (
                  <div className="max-w-[85%] rounded-2xl bg-gradient-to-r from-primary to-primary-hover px-3.5 py-2.5 text-white shadow-subtle">
                    <p className="leading-relaxed whitespace-pre-wrap">{msg.content}</p>
                  </div>
                ) : (
                  <div className="max-w-[95%] rounded-2xl border border-border/80 bg-surface p-3.5 shadow-subtle markdown-body">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                  </div>
                )}
              </div>
            ))}

            {sending && (
              <div className="flex items-center gap-2 text-muted-foreground text-xs py-1">
                <Spinner className="h-3.5 w-3.5" />
                <span>AI 视觉模型正在提取试题与解析...</span>
              </div>
            )}
          </div>

          {/* Bottom Chat / Upload Bar */}
          <div className="border-t border-border/60 bg-surface p-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)]">
            <form onSubmit={handleSendMessage} className="flex items-center gap-2">
              {/* Photo Upload Trigger */}
              <button
                type="button"
                disabled={uploading || sending}
                onClick={() => fileInputRef.current?.click()}
                className="flex h-10 w-10 items-center justify-center rounded-2xl bg-surface-secondary border border-border/70 text-foreground active:scale-95 shrink-0"
                aria-label="上传照片"
              >
                {uploading ? <Spinner className="h-4 w-4" /> : <Camera className="h-4 w-4 text-primary" />}
              </button>

              <input
                type="text"
                value={inputText}
                onChange={(e) => setInputText(e.target.value)}
                placeholder="发送识别指令（例如：请识别上述试题）..."
                disabled={sending}
                className="input flex-1 h-10 text-xs rounded-2xl"
              />

              <button
                type="submit"
                disabled={sending || !inputText.trim()}
                className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary text-white shadow-glow disabled:opacity-40 active:scale-95 shrink-0"
              >
                <Send className="h-4 w-4" />
              </button>
            </form>
          </div>
        </div>
      ) : (
        /* Tab: Drafts List */
        <div className="flex-1 overflow-y-auto p-4 space-y-3 touch-pan-y pb-24 min-h-0">
          {/* Drafts Filter Chips */}
          <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
            {filterLabels.map((f) => (
              <button
                key={f.key}
                onClick={() => setDraftFilter(f.key)}
                className={`rounded-full px-3 py-1 text-[11px] font-semibold shrink-0 transition-all ${
                  draftFilter === f.key
                    ? 'bg-primary text-white shadow-2xs font-bold'
                    : 'bg-surface border border-border text-muted-foreground'
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>

          {filteredDrafts.length === 0 ? (
            <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-8 text-center space-y-2">
              <FileQuestion className="h-10 w-10 text-muted-foreground mx-auto opacity-50" />
              <h3 className="text-sm font-bold text-foreground">暂无符合条件的草稿</h3>
              <p className="text-xs text-muted-foreground">切换到「AI 拍照识别」拍摄错题即可生成草稿</p>
            </div>
          ) : (
            filteredDrafts.map((d) => {
              const hasAnswer = d.answer && d.answer.length > 0

              return (
                <div
                  key={d.id}
                  className={`rounded-3xl border p-4 shadow-card space-y-2.5 transition-all ${
                    d.review_status === 'excluded'
                      ? 'border-border/60 bg-surface/50 opacity-60'
                      : 'border-border/80 bg-surface'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-1.5">
                      <Badge
                        tone={
                          d.review_status === 'committed'
                            ? 'success'
                            : d.review_status === 'excluded'
                              ? 'neutral'
                              : hasAnswer
                                ? 'primary'
                                : 'warning'
                        }
                        variant="subtle"
                        className="text-[9px]"
                      >
                        {d.review_status === 'committed'
                          ? '已入库'
                          : d.review_status === 'excluded'
                            ? '已排除'
                            : hasAnswer
                              ? '就绪'
                              : '缺少答案'}
                      </Badge>
                      <span className="font-mono text-[10px] text-muted-foreground">序号 #{d.seq_no || d.id}</span>
                    </div>

                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => toggleExcludeDraft(d)}
                        className="p-1 text-muted-foreground hover:text-foreground"
                        title={d.review_status === 'excluded' ? '恢复草稿' : '排除此题'}
                      >
                        <EyeOff className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => openEditDraft(d)}
                        className="p-1 text-muted-foreground hover:text-foreground"
                        title="编辑"
                      >
                        <Edit3 className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => handleDeleteDraft(d.id)}
                        className="p-1 text-muted-foreground hover:text-destructive"
                        title="删除"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>

                  <div className="text-xs text-foreground font-medium line-clamp-3 markdown-body">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{d.stem}</ReactMarkdown>
                  </div>

                  <div className="flex items-center justify-between border-t border-border/60 pt-2 text-[11px]">
                    <span className="text-muted-foreground">
                      答案: <span className="font-mono font-bold text-emerald-600 dark:text-emerald-400">{hasAnswer ? d.answer.join(', ') : '待回填'}</span>
                    </span>
                    {!hasAnswer && (
                      <button
                        onClick={() => openEditDraft(d)}
                        className="text-primary font-bold text-[11px] underline"
                      >
                        补全答案
                      </button>
                    )}
                  </div>
                </div>
              )
            })
          )}
        </div>
      )}

      {/* Session Switcher Sheet */}
      <MobileBottomSheet
        open={sessionSheetOpen}
        onClose={() => setSessionSheetOpen(false)}
        title="导入任务会话"
      >
        <div className="space-y-4">
          <form onSubmit={handleCreateNewSession} className="flex items-center gap-2">
            <input
              type="text"
              placeholder="新会话名称..."
              value={newSessionTitle}
              onChange={(e) => setNewSessionTitle(e.target.value)}
              className="input h-9 text-xs rounded-xl flex-1"
            />
            <Button size="xs" type="submit" loading={creatingSession} icon={<Plus className="h-3.5 w-3.5" />}>
              新建
            </Button>
          </form>

          <div className="space-y-2 max-h-60 overflow-y-auto">
            {sessions.map((s) => (
              <button
                key={s.id}
                onClick={() => handleSwitchSession(s.id)}
                className={`w-full flex items-center justify-between p-3 rounded-2xl border text-xs font-semibold text-left transition-all ${
                  sessionDetail?.session.id === s.id
                    ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                    : 'border-border bg-surface text-foreground'
                }`}
              >
                <div className="min-w-0 pr-2">
                  <span className="font-bold truncate block">{s.title}</span>
                  <span className="text-[10px] text-muted-foreground font-mono">
                    {s.draft_count} 题草稿 · {new Date(s.created_at).toLocaleDateString()}
                  </span>
                </div>
                {sessionDetail?.session.id === s.id && <Check className="h-4 w-4 text-primary shrink-0" />}
              </button>
            ))}
          </div>
        </div>
      </MobileBottomSheet>

      {/* Batch Commit Bottom Sheet */}
      <MobileBottomSheet
        open={commitOpen}
        onClose={() => setCommitOpen(false)}
        title="批量入库至题库"
        description={`将当前 ${readyCount} 道已就绪错题写入指定题库空间`}
      >
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">选择目标题库 *</label>
            <div className="space-y-2">
              {banks.map((b) => (
                <button
                  key={b.id}
                  type="button"
                  onClick={() => setSelectedBankId(b.id)}
                  className={`w-full flex items-center justify-between p-3 rounded-2xl border text-xs font-semibold text-left transition-all ${
                    selectedBankId === b.id
                      ? 'border-primary bg-primary/10 text-primary shadow-subtle'
                      : 'border-border bg-surface text-foreground'
                  }`}
                >
                  <span className="truncate">{b.name}</span>
                  {selectedBankId === b.id && <Check className="h-4 w-4 text-primary shrink-0" />}
                </button>
              ))}
            </div>
          </div>

          <div className="pt-2 flex items-center gap-2">
            <Button variant="outline" onClick={() => setCommitOpen(false)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button
              loading={committing}
              onClick={handleCommit}
              className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow"
            >
              确认入库
            </Button>
          </div>
        </div>
      </MobileBottomSheet>

      {/* Single Draft Edit Bottom Sheet */}
      <MobileBottomSheet
        open={!!editingDraft}
        onClose={() => setEditingDraft(null)}
        title="编辑草稿题目"
      >
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">题干内容 *</label>
            <textarea
              className="input min-h-[100px] rounded-xl text-xs resize-none"
              value={editingStem}
              onChange={(e) => setEditingStem(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">标准答案</label>
            <input
              className="input h-10 rounded-xl text-xs"
              placeholder="例如：A 或 线性表"
              value={editingAnswer}
              onChange={(e) => setEditingAnswer(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">解析内容</label>
            <textarea
              className="input min-h-[80px] rounded-xl text-xs resize-none"
              value={editingAnalysis}
              onChange={(e) => setEditingAnalysis(e.target.value)}
            />
          </div>

          <div className="flex items-center gap-2 pt-2">
            <Button variant="outline" onClick={() => setEditingDraft(null)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button loading={savingDraft} onClick={saveDraft} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              保存更改
            </Button>
          </div>
        </div>
      </MobileBottomSheet>
    </div>
  )
}
