import { useEffect, useRef, useState } from 'react'
import client, { authFetch, responseErrorMessage } from '@/api/client'
import { Badge, Button, IconButton, Spinner } from '@/components/ui'
import {
  Bot,
  ChevronDown,
  Database,
  HelpCircle,
  Lightbulb,
  PanelRightClose,
  RotateCcw,
  Send,
  Sparkles,
  Square,
  Wrench,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { AnswerResult, PracticeSession, Question } from '@/types'

interface ToolStep {
  id: string
  name: string
  status: 'running' | 'completed' | 'failed'
  message?: string
  result?: string
  duration?: string
}

interface ChatItem {
  role: 'user' | 'assistant' | 'status'
  content: string
  toolSteps?: ToolStep[]
  streaming?: boolean
}

const questionQuickActions = [
  { label: '详细讲解这道题', icon: Lightbulb },
  { label: '分析各个选项差异', icon: HelpCircle },
  { label: '为什么我的答案错了？', icon: RotateCcw },
  { label: '检索相关知识点', icon: Database },
  { label: '出类似题目训练', icon: Sparkles },
]

const globalQuickActions = [
  { label: '如何制定高效刷题计划？', icon: Lightbulb },
  { label: '帮我检索易错知识点', icon: Database },
  { label: '分析常见考点与考法', icon: HelpCircle },
  { label: '生成专项模拟题训练', icon: Sparkles },
]

export default function AiSidebar({
  question,
  result,
  session,
  onClose,
}: {
  question: Question | null
  result: AnswerResult | null
  session: PracticeSession | null
  onClose: () => void
}) {
  const [sessionId, setSessionId] = useState<number | null>(null)
  const [messages, setMessages] = useState<ChatItem[]>([])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [expandedTools, setExpandedTools] = useState<Record<string, boolean>>({})
  const scrollRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)
  const isComposingRef = useRef(false)

  // Initialize session with current question context
  useEffect(() => {
    let cancelled = false
    const init = async () => {
      try {
        const resp = await client.post('/agent/sessions', {
          title: question ? question.stem.slice(0, 24) : 'AI 学习会话',
          context: question ? { question_id: question.id, session_id: session?.id } : undefined,
        })
        if (!cancelled) setSessionId(resp.data.data.id)
      } catch {
        if (!cancelled) {
          setMessages([
            {
              role: 'status',
              content: 'AI 引擎未就绪。请在「设置 → 模型配置」中配置 Provider API Key 后重试。',
            },
          ])
        }
      }
    }
    init()
    return () => {
      cancelled = true
    }
  }, [question, session?.id])

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, busy])

  const toggleToolExpand = (stepId: string) => {
    setExpandedTools((prev) => ({ ...prev, [stepId]: !prev[stepId] }))
  }

  const stopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }
    setBusy(false)
    setMessages((prev) => {
      const copy = [...prev]
      if (copy.length > 0) {
        const last = copy[copy.length - 1]
        copy[copy.length - 1] = { ...last, streaming: false }
      }
      return copy
    })
  }

  const send = async (text?: string) => {
    const content = (text ?? input).trim()
    if (!content || busy || !sessionId) return
    setInput('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }

    setMessages((prev) => [...prev, { role: 'user', content }])
    setBusy(true)

    const newItem: ChatItem = { role: 'assistant', content: '', streaming: true, toolSteps: [] }
    setMessages((prev) => [...prev, newItem])

    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      const resp = await authFetch(`/api/v1/agent/sessions/${sessionId}/messages`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ message: content }),
        signal: abortController.signal,
      })

      if (!resp.ok) {
        throw new Error(await responseErrorMessage(resp, 'AI 消息发送失败'))
      }

      const reader = resp.body?.getReader()
      if (!reader) throw new Error('AI 流式响应为空，请检查局域网代理配置')

      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const events = buffer.split('\n\n')
        buffer = events.pop() ?? ''
        for (const evt of events) {
          for (const line of evt.split('\n')) {
            if (!line.startsWith('data:')) continue
            try {
              const data = JSON.parse(line.slice(5))
              handleEvent(data)
            } catch {
              // ignore partial json
            }
          }
        }
      }

      // Finish streaming
      setMessages((prev) => {
        const copy = [...prev]
        if (copy.length > 0) {
          const last = copy[copy.length - 1]
          copy[copy.length - 1] = { ...last, streaming: false }
        }
        return copy
      })
    } catch (err: unknown) {
      if ((err as Error)?.name !== 'AbortError') {
        setMessages((prev) => [
          ...prev,
          { role: 'status', content: err instanceof Error ? err.message : '与 AI 服务连接中断，请检查网络或模型配置后重试。' },
        ])
      }
    } finally {
      setBusy(false)
      abortControllerRef.current = null
    }
  }

  function handleEvent(data: {
    type: string
    message?: string
    data?: { delta?: string; done?: boolean; name?: string; result?: string; step_id?: string }
  }) {
    switch (data.type) {
      case 'agent.started':
        break
      case 'tool.started': {
        const toolName = data.data?.name ?? '检索知识库'
        const stepId = data.data?.step_id ?? `tool-${Date.now()}`
        setMessages((prev) => {
          const copy = [...prev]
          const last = copy[copy.length - 1]
          if (last?.role === 'assistant') {
            const steps = last.toolSteps ?? []
            copy[copy.length - 1] = {
              ...last,
              toolSteps: [
                ...steps,
                { id: stepId, name: toolName, status: 'running', message: data.message ?? '正在调用检索工具...' },
              ],
            }
          }
          return copy
        })
        break
      }
      case 'tool.completed': {
        setMessages((prev) => {
          const copy = [...prev]
          const last = copy[copy.length - 1]
          if (last?.role === 'assistant' && last.toolSteps?.length) {
            const steps = [...last.toolSteps]
            const lastStep = steps[steps.length - 1]
            steps[steps.length - 1] = {
              ...lastStep,
              status: 'completed',
              message: data.message ?? '执行完成',
              result: data.data?.result,
            }
            copy[copy.length - 1] = { ...last, toolSteps: steps }
          }
          return copy
        })
        break
      }
      case 'agent.streaming': {
        const delta = data.data?.delta
        if (delta) {
          setMessages((prev) => {
            const copy = [...prev]
            const last = copy[copy.length - 1]
            if (last && last.role === 'assistant') {
              copy[copy.length - 1] = { ...last, content: last.content + delta }
            } else {
              copy.push({ role: 'assistant', content: delta, streaming: true, toolSteps: [] })
            }
            return copy
          })
        }
        break
      }
      case 'agent.failed':
        setMessages((prev) => [
          ...prev,
          { role: 'status', content: data.message ?? 'AI 处理遇到问题，请重试。' },
        ])
        break
      case 'agent.completed':
        setMessages((prev) => {
          const copy = [...prev]
          const last = copy[copy.length - 1]
          if (last) copy[copy.length - 1] = { ...last, streaming: false }
          return copy
        })
        break
    }
  }

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing || isComposingRef.current || e.keyCode === 229) {
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const handleTextareaChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    // Auto grow textarea smoothly
    e.target.style.height = 'auto'
    e.target.style.height = `${Math.min(e.target.scrollHeight, 140)}px`
  }

  return (
    <div className="flex h-full w-full flex-col bg-surface overflow-hidden transition-all duration-200">
      {/* Header */}
      <div className="flex h-14 items-center justify-between border-b border-border/70 px-4 bg-surface-secondary/40 backdrop-blur-sm">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="flex h-7 w-7 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Sparkles className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="text-xs font-bold text-foreground">AI 伴学 Copilot</span>
              <span className="flex h-1.5 w-1.5 rounded-full bg-success" />
            </div>
            <span className="text-[10px] text-muted-foreground block truncate font-mono">
              {question ? `题目 #${question.id} 上下文已挂载` : '学习助手已就绪'}
            </span>
          </div>
        </div>

        <IconButton
          variant="ghost"
          size="sm"
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground"
          title="收起助手"
        >
          <PanelRightClose className="h-4 w-4" />
        </IconButton>
      </div>

      {/* Context info banner */}
      {question && (
        <div className="flex items-center justify-between border-b border-border/50 bg-primary/5 px-3.5 py-1.5 text-[11px] text-muted-foreground">
          <div className="flex items-center gap-1.5 truncate">
            <Badge tone="primary" variant="subtle" className="text-[10px] px-1.5 py-0 font-medium">
              当前题干
            </Badge>
            <span className="truncate font-mono text-[11px]">
              {question.stem.replace(/[`*#]/g, '').slice(0, 28)}...
            </span>
          </div>
          {result && (
            <Badge tone={result.correct ? 'success' : 'danger'} variant="subtle" className="text-[10px] px-1.5 py-0 shrink-0 font-mono">
              {result.correct ? '已做对' : '已答错'}
            </Badge>
          )}
        </div>
      )}

      {/* Quick Actions Scrollable */}
      <div className="flex gap-1.5 overflow-x-auto border-b border-border/50 bg-surface-secondary/20 p-2.5 scrollbar-none">
        {(question ? questionQuickActions : globalQuickActions).map((action, i) => {
          const Icon = action.icon
          return (
            <button
              key={i}
              onClick={() => send(action.label)}
              disabled={busy}
              className="inline-flex items-center gap-1.5 shrink-0 rounded-full border border-border/80 bg-surface px-3 py-1 text-[11px] font-medium text-muted-foreground transition-all hover:border-primary/40 hover:bg-surface-secondary hover:text-foreground active:scale-[0.98] disabled:opacity-50 shadow-2xs"
            >
              <Icon className="h-3 w-3 text-primary" />
              <span>{action.label}</span>
            </button>
          )
        })}
      </div>

      {/* Chat Messages */}
      <div ref={scrollRef} className="flex-1 space-y-4 overflow-y-auto p-4">
        {messages.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center py-10 px-4">
            <div className="flex h-14 w-14 items-center justify-center rounded-3xl bg-primary/10 border border-primary/20 text-primary shadow-glow">
              <Bot className="h-7 w-7" />
            </div>
            <div className="space-y-1.5 max-w-xs">
              <h4 className="text-xs font-bold tracking-tight text-foreground">我是你的 AI 专属答疑助教</h4>
              <p className="text-[11px] text-muted-foreground leading-relaxed">
                {question
                  ? '无需手动复制题目，我已经读取了当前题目的完整题干、选项与解析。点击上方快捷动作，或直接输入你的疑问。'
                  : '随时向我提问关于题库、知识点体系、学习备考计划或题目解析的问题。'}
              </p>
            </div>
          </div>
        )}

        {messages.map((m, i) => (
          <div key={i} className="space-y-2 animate-message-enter">
            {/* User message bubble */}
            {m.role === 'user' && (
              <div className="flex justify-end">
                <div className="max-w-[88%] rounded-2xl rounded-tr-xs bg-gradient-to-r from-primary to-primary-hover px-4 py-2.5 text-xs font-medium leading-relaxed text-white shadow-subtle whitespace-pre-wrap select-text selection:bg-white/30 selection:text-white">
                  {m.content}
                </div>
              </div>
            )}

            {/* Assistant message */}
            {m.role === 'assistant' && (
              <div className="space-y-2 text-foreground">
                {/* Collapsible Tool steps rendering */}
                {m.toolSteps && m.toolSteps.length > 0 && (
                  <div className="space-y-1.5 my-2">
                    {m.toolSteps.map((step) => {
                      const isExpanded = !!expandedTools[step.id]
                      return (
                        <div
                          key={step.id}
                          className="rounded-2xl border border-border/70 bg-surface-secondary/50 text-xs overflow-hidden shadow-2xs"
                        >
                          <button
                            onClick={() => toggleToolExpand(step.id)}
                            className="flex w-full items-center justify-between px-3 py-2 text-left text-muted-foreground hover:text-foreground transition-colors"
                          >
                            <div className="flex items-center gap-2 min-w-0">
                              {step.status === 'running' ? (
                                <Spinner className="h-3 w-3 text-primary" />
                              ) : step.status === 'failed' ? (
                                <span className="h-2 w-2 rounded-full bg-destructive" />
                              ) : (
                                <span className="h-2 w-2 rounded-full bg-success" />
                              )}
                              <span className="font-semibold text-[11px] text-foreground/90 truncate">
                                {step.name}
                              </span>
                              {step.message && (
                                <span className="text-[10px] text-muted-foreground truncate">
                                  {step.message}
                                </span>
                              )}
                            </div>
                            {step.result && (
                              <ChevronDown
                                className={`h-3 w-3 text-muted-foreground transition-transform ${
                                  isExpanded ? 'rotate-180' : ''
                                }`}
                              />
                            )}
                          </button>
                          {isExpanded && step.result && (
                            <div className="border-t border-border/50 bg-background/60 p-3 font-mono text-[10px] text-muted-foreground overflow-x-auto max-h-40 select-text">
                              {step.result}
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}

                {/* Assistant Markdown Content */}
                {m.content ? (
                  <div className="rounded-2xl border border-border/70 bg-surface p-4 text-xs leading-relaxed text-foreground shadow-card">
                    <div className="markdown-body select-text">
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>{m.content}</ReactMarkdown>
                      {m.streaming && (
                        <span className="inline-block h-3.5 w-1.5 ml-1 animate-pulse bg-primary align-middle rounded-xs" />
                      )}
                    </div>
                  </div>
                ) : (
                  m.streaming && (
                    <div className="flex items-center gap-2 text-xs text-muted-foreground py-2 px-1">
                      <Spinner className="h-3.5 w-3.5 text-primary" />
                      <span>正在分析题目上下文与知识点...</span>
                    </div>
                  )
                )}
              </div>
            )}

            {/* System Status message */}
            {m.role === 'status' && (
              <div className="flex items-center gap-2 rounded-2xl border border-border/60 bg-surface-secondary/80 px-3.5 py-2.5 text-xs text-muted-foreground">
                <Wrench className="h-4 w-4 text-primary shrink-0" />
                <span className="flex-1 text-[11px] leading-relaxed">{m.content}</span>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Input Area */}
      <div className="border-t border-border/80 bg-surface p-3 sm:p-4">
        <div className="relative flex flex-col rounded-2xl border border-border bg-surface-secondary/50 focus-within:border-primary/60 focus-within:ring-2 focus-within:ring-primary/20 transition-all shadow-2xs">
          <textarea
            ref={textareaRef}
            rows={1}
            value={input}
            onChange={handleTextareaChange}
            onCompositionStart={() => {
              isComposingRef.current = true
            }}
            onCompositionEnd={() => {
              isComposingRef.current = false
            }}
            onKeyDown={handleInputKeyDown}
            placeholder="向 AI 助教提问... (Enter 发送, Shift+Enter 换行)"
            disabled={busy}
            className="w-full resize-none bg-transparent px-3.5 py-3 text-xs text-foreground placeholder:text-muted-foreground/60 focus:outline-none min-h-[44px] max-h-[140px]"
          />
          <div className="flex items-center justify-between px-3 pb-2.5">
            <span className="text-[10px] text-muted-foreground/60 hidden sm:inline">
              Shift + Enter 换行
            </span>
            {busy ? (
              <Button
                size="xs"
                variant="danger"
                onClick={stopGeneration}
                icon={<Square className="h-3 w-3 fill-current" />}
                className="ml-auto"
              >
                停止生成
              </Button>
            ) : (
              <Button
                size="xs"
                variant="primary"
                onClick={() => send()}
                disabled={!input.trim()}
                icon={<Send className="h-3 w-3" />}
                className="ml-auto shadow-subtle"
              >
                发送
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
