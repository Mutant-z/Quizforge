import React, { useEffect, useRef, useState } from 'react'
import client from '@/api/client'
import { Badge, Spinner } from '@/components/ui'
import {
  Bot,
  ChevronDown,
  Database,
  HelpCircle,
  Lightbulb,
  RotateCcw,
  Send,
  Sparkles,
  Square,
  Wrench,
  X,
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

export function MobileAiSheet({
  open,
  onClose,
  question = null,
  result = null,
  session = null,
}: {
  open: boolean
  onClose: () => void
  question?: Question | null
  result?: AnswerResult | null
  session?: PracticeSession | null
}) {
  const [sessionId, setSessionId] = useState<number | null>(null)
  const [messages, setMessages] = useState<ChatItem[]>([])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [expandedTools, setExpandedTools] = useState<Record<string, boolean>>({})
  const scrollRef = useRef<HTMLDivElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    const init = async () => {
      try {
        const resp = await client.post('/agent/sessions', {
          title: question ? question.stem.slice(0, 20) : '移动端 AI 学习会话',
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
  }, [open, question, session?.id])

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages, busy])

  const send = async (textToSend?: string) => {
    const query = (textToSend || input).trim()
    if (!query || busy || !sessionId) return

    setInput('')
    setMessages((prev) => [...prev, { role: 'user', content: query }])
    setBusy(true)

    const token = localStorage.getItem('qt_access_token')
    const abort = new AbortController()
    abortControllerRef.current = abort

    try {
      const resp = await fetch(`/api/v1/agent/sessions/${sessionId}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          message: query,
          context: {
            question_id: question?.id,
            practice_session_id: session?.id,
            is_correct: result?.correct,
            user_answer: result?.user_answer,
            standard_answer: result?.correct_answer,
          },
        }),
        signal: abort.signal,
      })

      if (!resp.ok) {
        throw new Error(`HTTP ${resp.status}`)
      }

      setMessages((prev) => [...prev, { role: 'assistant', content: '', toolSteps: [], streaming: true }])

      const reader = resp.body?.getReader()
      const decoder = new TextDecoder()
      let accumulated = ''
      const currentTools: ToolStep[] = []

      if (reader) {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          const chunk = decoder.decode(value, { stream: true })
          const lines = chunk.split('\n')

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const dataStr = line.slice(6).trim()
              if (dataStr === '[DONE]') continue
              try {
                const parsed = JSON.parse(dataStr)
                if (parsed.type === 'delta' || parsed.type === 'content') {
                  accumulated += parsed.content || ''
                  setMessages((prev) => {
                    const next = [...prev]
                    const last = next[next.length - 1]
                    if (last && last.role === 'assistant') {
                      last.content = accumulated
                    }
                    return next
                  })
                } else if (parsed.type === 'tool_start') {
                  currentTools.push({
                    id: parsed.id || String(Date.now()),
                    name: parsed.name || '工具调用',
                    status: 'running',
                    message: parsed.message,
                  })
                  setMessages((prev) => {
                    const next = [...prev]
                    const last = next[next.length - 1]
                    if (last && last.role === 'assistant') {
                      last.toolSteps = [...currentTools]
                    }
                    return next
                  })
                } else if (parsed.type === 'tool_end') {
                  const step = currentTools.find((s) => s.id === parsed.id || s.name === parsed.name)
                  if (step) {
                    step.status = parsed.status === 'error' ? 'failed' : 'completed'
                    step.result = parsed.result
                  }
                  setMessages((prev) => {
                    const next = [...prev]
                    const last = next[next.length - 1]
                    if (last && last.role === 'assistant') {
                      last.toolSteps = [...currentTools]
                    }
                    return next
                  })
                }
              } catch {
                // partial json line
              }
            }
          }
        }
      }

      setMessages((prev) => {
        const next = [...prev]
        const last = next[next.length - 1]
        if (last && last.role === 'assistant') {
          last.streaming = false
        }
        return next
      })
    } catch (e: unknown) {
      if ((e as Error).name !== 'AbortError') {
        setMessages((prev) => [
          ...prev,
          { role: 'status', content: `AI 生成失败: ${e instanceof Error ? e.message : '未知异常'}` },
        ])
      }
    } finally {
      setBusy(false)
      abortControllerRef.current = null
    }
  }

  const cancel = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
      setBusy(false)
    }
  }

  if (!open) return null

  const quickActions = question ? questionQuickActions : globalQuickActions

  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end">
      <div
        className="fixed inset-0 bg-black/50 backdrop-blur-xs transition-opacity animate-in fade-in-0 duration-200"
        onClick={onClose}
      />
      <div className="relative z-10 flex flex-col h-[85vh] w-full rounded-t-3xl border-t border-border/80 bg-surface shadow-float animate-sheet-up overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-border/60 bg-surface">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Sparkles className="h-4 w-4 animate-pulse-subtle" />
            </div>
            <div>
              <h3 className="text-xs font-bold text-foreground flex items-center gap-1.5">
                AI 学习助教
                <span className="rounded bg-primary/10 px-1 py-0.2 text-[9px] font-mono text-primary">Copilot</span>
              </h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="flex h-7 w-7 items-center justify-center rounded-full bg-surface-secondary text-muted-foreground hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-3.5 touch-pan-y min-h-0 text-xs">
          {messages.length === 0 && (
            <div className="space-y-3 pt-2">
              <div className="rounded-2xl border border-primary/20 bg-primary/5 p-3.5 space-y-2">
                <div className="flex items-center gap-2 text-primary font-bold">
                  <Bot className="h-4 w-4" />
                  <span>你好！我是你的自适应学习助教</span>
                </div>
                <p className="text-muted-foreground leading-relaxed">
                  随时向我提问考点细节、解题思路或选项辨析，我会通过知识库与向量检索为你提供深入解析。
                </p>
              </div>

              {/* Quick Actions */}
              <div className="space-y-1.5 pt-1">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block px-1">
                  快捷探索
                </span>
                <div className="grid grid-cols-1 gap-1.5">
                  {quickActions.map((qa, idx) => (
                    <button
                      key={idx}
                      onClick={() => send(qa.label)}
                      className="flex items-center gap-2 rounded-xl border border-border/70 bg-surface-secondary/60 px-3 py-2 text-left text-xs font-medium text-foreground hover:bg-surface-secondary active:scale-[0.98] transition-all"
                    >
                      <qa.icon className="h-3.5 w-3.5 text-primary shrink-0" />
                      <span className="truncate">{qa.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}

          {messages.map((msg, i) => (
            <div key={i} className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}>
              {msg.role === 'user' ? (
                <div className="max-w-[85%] rounded-2xl bg-gradient-to-r from-primary to-primary-hover px-3.5 py-2.5 text-white shadow-subtle">
                  <p className="leading-relaxed whitespace-pre-wrap">{msg.content}</p>
                </div>
              ) : msg.role === 'assistant' ? (
                <div className="max-w-[95%] space-y-2">
                  {/* Tool steps */}
                  {msg.toolSteps && msg.toolSteps.length > 0 && (
                    <div className="space-y-1">
                      {msg.toolSteps.map((step) => (
                        <div
                          key={step.id}
                          className="rounded-xl border border-border/60 bg-surface-secondary/70 p-2 text-[11px]"
                        >
                          <div
                            className="flex items-center justify-between cursor-pointer"
                            onClick={() =>
                              setExpandedTools((prev) => ({ ...prev, [step.id]: !prev[step.id] }))
                            }
                          >
                            <div className="flex items-center gap-1.5 font-semibold">
                              <Wrench className="h-3 w-3 text-primary" />
                              <span>{step.name}</span>
                              {step.status === 'running' && <Spinner className="h-2.5 w-2.5 ml-1" />}
                            </div>
                            <div className="flex items-center gap-1">
                              <Badge
                                tone={
                                  step.status === 'completed'
                                    ? 'success'
                                    : step.status === 'failed'
                                      ? 'danger'
                                      : 'primary'
                                }
                                variant="subtle"
                                className="text-[9px] py-0 px-1"
                              >
                                {step.status === 'completed'
                                  ? '完成'
                                  : step.status === 'failed'
                                    ? '失败'
                                    : '调用中'}
                              </Badge>
                              {step.result && (
                                <ChevronDown
                                  className={`h-3 w-3 text-muted-foreground transition-transform ${
                                    expandedTools[step.id] ? 'rotate-180' : ''
                                  }`}
                                />
                              )}
                            </div>
                          </div>
                          {expandedTools[step.id] && step.result && (
                            <pre className="mt-1.5 max-h-32 overflow-y-auto rounded-lg bg-surface p-2 font-mono text-[10px] text-muted-foreground whitespace-pre-wrap">
                              {step.result}
                            </pre>
                          )}
                        </div>
                      ))}
                    </div>
                  )}

                  {/* Assistant Text */}
                  <div className="rounded-2xl border border-border/80 bg-surface-secondary/50 p-3.5 shadow-subtle markdown-body">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content || '思考中...'}</ReactMarkdown>
                  </div>
                </div>
              ) : (
                <div className="w-full text-center py-1">
                  <span className="inline-block rounded-lg bg-destructive/10 px-2.5 py-1 text-[11px] font-medium text-destructive">
                    {msg.content}
                  </span>
                </div>
              )}
            </div>
          ))}

          {busy && (
            <div className="flex items-center gap-2 text-muted-foreground text-xs py-1">
              <Spinner className="h-3 w-3" />
              <span>AI 助教正在分析...</span>
            </div>
          )}
        </div>

        {/* Input Bar */}
        <div className="border-t border-border/60 bg-surface p-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)]">
          <form
            onSubmit={(e) => {
              e.preventDefault()
              send()
            }}
            className="flex items-center gap-2"
          >
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="向 AI 助教提问..."
              disabled={busy && !abortControllerRef.current}
              className="input flex-1 h-10 text-xs rounded-2xl"
            />
            {busy ? (
              <button
                type="button"
                onClick={cancel}
                className="flex h-10 w-10 items-center justify-center rounded-2xl bg-destructive/10 text-destructive border border-destructive/20 active:scale-95 shrink-0"
              >
                <Square className="h-4 w-4 fill-current" />
              </button>
            ) : (
              <button
                type="submit"
                disabled={!input.trim()}
                className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary text-white shadow-glow disabled:opacity-40 active:scale-95 shrink-0"
              >
                <Send className="h-4 w-4" />
              </button>
            )}
          </form>
        </div>
      </div>
    </div>
  )
}
