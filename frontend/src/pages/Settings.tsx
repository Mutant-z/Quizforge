import { useEffect, useState } from 'react'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Modal, PageHeader } from '@/components/ui'
import { useAuthStore } from '@/store/auth'
import type { ProviderConfig, ProviderType } from '@/types'
import {
  ArrowRight,
  Bot,
  Check,
  CheckCircle2,
  Copy,
  Cpu,
  Database,
  Layers,
  Network,
  Pencil,
  Plus,
  Save,
  Shield,
  Sparkles,
  Trash2,
  User as UserIcon,
  Zap,
} from 'lucide-react'

interface PresetConfig {
  label: string
  name: string
  protocol: string
  base_url: string
  model_name: string
  supports_vision?: boolean
  tag?: string
}

const PRESETS: Record<ProviderType, PresetConfig[]> = {
  llm: [
    {
      label: 'DeepSeek 官方',
      name: 'DeepSeek-V3 / Chat',
      protocol: 'openai',
      base_url: 'https://api.deepseek.com',
      model_name: 'deepseek-chat',
      supports_vision: false,
      tag: '高性价比',
    },
    {
      label: '硅基流动 DeepSeek',
      name: 'SiliconFlow DeepSeek-V3',
      protocol: 'openai',
      base_url: 'https://api.siliconflow.cn/v1',
      model_name: 'deepseek-ai/DeepSeek-V3',
      supports_vision: false,
      tag: '国内加速',
    },
    {
      label: 'OpenAI GPT-4o-mini',
      name: 'OpenAI GPT-4o Mini',
      protocol: 'openai',
      base_url: 'https://api.openai.com/v1',
      model_name: 'gpt-4o-mini',
      supports_vision: true,
      tag: '多模态视觉',
    },
    {
      label: 'Claude 3.5 Sonnet',
      name: 'Anthropic Claude 3.5',
      protocol: 'anthropic',
      base_url: 'https://api.anthropic.com',
      model_name: 'claude-3-5-sonnet-20241022',
      supports_vision: true,
      tag: '高精推理',
    },
    {
      label: 'Ollama 本地大模型',
      name: 'Ollama Local LLM',
      protocol: 'openai',
      base_url: 'http://localhost:11434/v1',
      model_name: 'llama3:8b',
      supports_vision: false,
      tag: '本地私有化',
    },
  ],
  embedding: [
    {
      label: '硅基流动 BGE-M3',
      name: 'SiliconFlow BGE-M3',
      protocol: 'openai',
      base_url: 'https://api.siliconflow.cn/v1',
      model_name: 'BAAI/bge-m3',
      tag: '多语言推荐',
    },
    {
      label: 'OpenAI Embed v3',
      name: 'OpenAI Text Embedding Small',
      protocol: 'openai',
      base_url: 'https://api.openai.com/v1',
      model_name: 'text-embedding-3-small',
      tag: '官方推荐',
    },
    {
      label: 'Ollama 本地向量',
      name: 'Ollama Local Embedding',
      protocol: 'ollama',
      base_url: 'http://localhost:11434',
      model_name: 'bge-m3',
      tag: '离线部署',
    },
    {
      label: 'Cohere Multilingual',
      name: 'Cohere Embed v3',
      protocol: 'cohere',
      base_url: 'https://api.cohere.ai/v1',
      model_name: 'embed-multilingual-v3.0',
      tag: '高精多语言',
    },
  ],
  rerank: [
    {
      label: '硅基流动 BGE Reranker',
      name: 'SiliconFlow BGE-Reranker-v2',
      protocol: 'http',
      base_url: 'https://api.siliconflow.cn/v1',
      model_name: 'BAAI/bge-reranker-v2-m3',
      tag: '首选精排',
    },
    {
      label: '通用大模型提示词重排',
      name: 'LLM Prompt Reranker',
      protocol: 'openai',
      base_url: 'https://api.deepseek.com',
      model_name: 'deepseek-chat',
      tag: '免专用接口',
    },
    {
      label: 'Cohere Rerank v3',
      name: 'Cohere Rerank API',
      protocol: 'http',
      base_url: 'https://api.cohere.ai/v1',
      model_name: 'rerank-multilingual-v3.0',
      tag: '高精度',
    },
  ],
}

export default function Settings() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const [providers, setProviders] = useState<ProviderConfig[]>([])
  const [activeTab, setActiveTab] = useState<ProviderType>('llm')
  const [editor, setEditor] = useState<ProviderConfig | 'new' | null>(null)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string; latency?: number } | null>(null)
  const [apiKeyInput, setApiKeyInput] = useState('')
  const [savingProfile, setSavingProfile] = useState(false)
  const [profileSaved, setProfileSaved] = useState(false)
  const [bio, setBio] = useState(user?.bio ?? '')
  const [copiedId, setCopiedId] = useState<number | null>(null)
  const [cardTestingId, setCardTestingId] = useState<number | null>(null)
  const [cardTestStatus, setCardTestStatus] = useState<Record<number, { ok: boolean; latency?: number; msg: string }>>({})

  const load = async () => {
    try {
      const resp = await client.get('/providers')
      setProviders(resp.data.data)
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    load()
  }, [])

  useEffect(() => {
    if (user) setBio(user.bio ?? '')
  }, [user])

  const currentTabProviders = providers.filter(
    (p) => (p.provider_type || 'llm') === activeTab
  )

  const form: ProviderConfig =
    editor && editor !== 'new'
      ? editor
      : {
          id: 0,
          provider_type: activeTab,
          name: '',
          protocol: activeTab === 'llm' ? 'openai' : 'http',
          base_url: '',
          api_key_masked: '',
          model_name: '',
          is_default: currentTabProviders.length === 0,
          supports_vision: false,
        }
  const isNew = editor === 'new' || (editor && editor.id === 0)

  const update = (patch: Partial<ProviderConfig>) => {
    if (editor === 'new' || !editor) return
    setEditor({ ...editor, ...patch })
  }

  const handleOpenAdd = (preset?: PresetConfig) => {
    const p = preset || PRESETS[activeTab][0]
    setEditor({
      id: 0,
      provider_type: activeTab,
      name: p?.name || '',
      protocol: p?.protocol || 'openai',
      base_url: p?.base_url || '',
      api_key_masked: '',
      model_name: p?.model_name || '',
      is_default: currentTabProviders.length === 0,
      supports_vision: Boolean(p?.supports_vision),
    })
    setApiKeyInput('')
    setTestResult(null)
  }

  const applyPresetToForm = (p: PresetConfig) => {
    if (!editor || editor === 'new') return
    setEditor({
      ...editor,
      name: p.name,
      protocol: p.protocol,
      base_url: p.base_url,
      model_name: p.model_name,
      supports_vision: Boolean(p.supports_vision),
    })
  }

  const save = async () => {
    if (!editor) return
    const body = {
      provider_type: form.provider_type || activeTab,
      name: form.name || '默认 Provider',
      protocol: form.protocol || 'openai',
      base_url: form.base_url,
      api_key: apiKeyInput,
      model_name: form.model_name,
      is_default: form.is_default,
      supports_vision: form.supports_vision,
    }
    try {
      if (isNew) await client.post('/providers', body)
      else await client.put(`/providers/${form.id}`, body)
      setEditor(null)
      setApiKeyInput('')
      setTestResult(null)
      load()
    } catch (e) {
      alert(errMsg(e))
    }
  }

  const setDefault = async (id: number) => {
    try {
      await client.post(`/providers/${id}/default`)
      load()
    } catch (e) {
      alert(errMsg(e))
    }
  }

  const test = async () => {
    if (!editor) return
    if (!apiKeyInput && form.api_key_masked) {
      alert('为避免把脱敏值当作凭证，请重新输入 API Key 后执行能力测试。')
      return
    }
    setTesting(true)
    setTestResult(null)
    try {
      const resp = await client.post('/providers/test', {
        provider_type: form.provider_type || activeTab,
        name: form.name,
        protocol: form.protocol || 'openai',
        base_url: form.base_url,
        api_key: apiKeyInput,
        model_name: form.model_name,
        supports_vision: form.supports_vision,
      })
      const data = resp.data.data
      setTestResult({
        ok: true,
        message: data.reply || '连通性验证成功',
        latency: data.latency_ms,
      })
    } catch (e) {
      setTestResult({
        ok: false,
        message: errMsg(e),
      })
    } finally {
      setTesting(false)
    }
  }

  const testCardProvider = async (p: ProviderConfig) => {
    setCardTestingId(p.id)
    try {
      const resp = await client.post('/providers/test', {
        provider_type: p.provider_type,
        name: p.name,
        protocol: p.protocol,
        base_url: p.base_url,
        api_key: '',
        model_name: p.model_name,
        supports_vision: p.supports_vision,
      })
      const data = resp.data.data
      setCardTestStatus((prev) => ({
        ...prev,
        [p.id]: {
          ok: true,
          latency: data.latency_ms,
          msg: data.reply || '连接成功',
        },
      }))
    } catch (e) {
      setCardTestStatus((prev) => ({
        ...prev,
        [p.id]: {
          ok: false,
          msg: errMsg(e),
        },
      }))
    } finally {
      setCardTestingId(null)
    }
  }

  const copyEndpoint = (p: ProviderConfig) => {
    navigator.clipboard.writeText(p.base_url)
    setCopiedId(p.id)
    setTimeout(() => setCopiedId(null), 1800)
  }

  const saveProfile = async () => {
    setSavingProfile(true)
    try {
      const resp = await client.put('/users/me', { bio })
      setUser(resp.data.data)
      setProfileSaved(true)
      setTimeout(() => setProfileSaved(false), 2000)
    } catch (e) {
      alert(errMsg(e))
    } finally {
      setSavingProfile(false)
    }
  }

  const tabConfig = [
    {
      key: 'llm' as ProviderType,
      label: '通用大模型 (LLM)',
      shortLabel: 'LLM 对话推理',
      desc: '负责多 Agent 协同推理、题目结构化解析与 AI 导师实时对话',
      icon: Bot,
      accentColor: 'text-primary',
      badgeBg: 'bg-primary/10 text-primary border-primary/20',
      count: providers.filter((p) => (p.provider_type || 'llm') === 'llm').length,
    },
    {
      key: 'embedding' as ProviderType,
      label: '向量嵌入模型 (Embedding)',
      shortLabel: 'Embedding 向量',
      desc: '负责构建题库语义向量索引、相似考点关联与深度语义检索',
      icon: Database,
      accentColor: 'text-sky-500',
      badgeBg: 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border-sky-500/20',
      count: providers.filter((p) => p.provider_type === 'embedding').length,
    },
    {
      key: 'rerank' as ProviderType,
      label: '二次重排模型 (Rerank)',
      shortLabel: 'Rerank 重排',
      desc: '负责 RAG 混合检索后的高精二次语义打分与上下文精排',
      icon: Network,
      accentColor: 'text-violet-500',
      badgeBg: 'bg-violet-500/10 text-violet-600 dark:text-violet-400 border-violet-500/20',
      count: providers.filter((p) => p.provider_type === 'rerank').length,
    },
  ]

  const totalProviders = providers.length
  const defaultLlm = providers.find((p) => (p.provider_type || 'llm') === 'llm' && p.is_default)
  const defaultEmbed = providers.find((p) => p.provider_type === 'embedding' && p.is_default)
  const defaultRerank = providers.find((p) => p.provider_type === 'rerank' && p.is_default)

  return (
    <div className="space-y-6 pb-8 animate-fade-in max-w-7xl mx-auto">
      {/* 1. Modern Page Header with Dynamic RAG Architecture Status Bar */}
      <div className="space-y-4">
        <PageHeader
          title="系统与模型设置"
          subtitle="统一管理多厂商 AI 模型端点、解耦式 RAG 检索链路与个人学习偏好"
          tags={
            <div className="hidden sm:flex items-center gap-2">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-surface border border-border px-3 py-1 text-xs font-medium text-foreground shadow-2xs">
                <span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
                已就绪 {totalProviders} 个端点
              </span>
            </div>
          }
        />

        {/* High-Tech RAG Pipeline Readiness Bar */}
        <div className="rounded-2xl border border-border/80 bg-gradient-to-r from-surface via-surface-secondary/30 to-surface p-4 sm:p-5 shadow-sm">
          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div className="flex items-center gap-2.5">
              <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Layers className="h-4 w-4" />
              </div>
              <div>
                <h2 className="text-xs sm:text-sm font-bold text-foreground">解耦式 RAG 架构流水线</h2>
                <p className="text-[11px] text-muted-foreground">三大核心引擎独立路由与故障隔离</p>
              </div>
            </div>

            {/* Pipeline Stage Indicators */}
            <div className="flex flex-wrap items-center gap-2 sm:gap-3 text-xs">
              {/* Stage 1: LLM */}
              <button
                type="button"
                onClick={() => setActiveTab('llm')}
                className={`flex items-center gap-2 rounded-xl px-3 py-1.5 border transition-all cursor-pointer ${
                  activeTab === 'llm'
                    ? 'border-primary bg-primary/5 text-primary font-semibold ring-2 ring-primary/20 shadow-2xs'
                    : 'border-border/80 bg-surface hover:border-primary/40 text-foreground'
                }`}
                title="点击切换到通用大模型配置"
              >
                <Bot className="h-3.5 w-3.5 text-primary" />
                <span className="text-[11px] font-medium">1. 对话推理</span>
                {defaultLlm ? (
                  <span className="max-w-[100px] truncate font-mono text-[10px] text-emerald-600 dark:text-emerald-400 font-bold">
                    {defaultLlm.name}
                  </span>
                ) : (
                  <span className="text-[10px] text-amber-500 font-bold">未配置</span>
                )}
              </button>

              <ArrowRight className="h-3.5 w-3.5 text-muted-foreground/40 hidden sm:block" />

              {/* Stage 2: Embedding */}
              <button
                type="button"
                onClick={() => setActiveTab('embedding')}
                className={`flex items-center gap-2 rounded-xl px-3 py-1.5 border transition-all cursor-pointer ${
                  activeTab === 'embedding'
                    ? 'border-sky-500 bg-sky-500/5 text-sky-600 dark:text-sky-400 font-semibold ring-2 ring-sky-500/20 shadow-2xs'
                    : 'border-border/80 bg-surface hover:border-sky-500/40 text-foreground'
                }`}
                title="点击切换到向量嵌入模型配置"
              >
                <Database className="h-3.5 w-3.5 text-sky-500" />
                <span className="text-[11px] font-medium">2. 向量检索</span>
                {defaultEmbed ? (
                  <span className="max-w-[100px] truncate font-mono text-[10px] text-emerald-600 dark:text-emerald-400 font-bold">
                    {defaultEmbed.name}
                  </span>
                ) : (
                  <span className="text-[10px] text-muted-foreground">可选</span>
                )}
              </button>

              <ArrowRight className="h-3.5 w-3.5 text-muted-foreground/40 hidden sm:block" />

              {/* Stage 3: Rerank */}
              <button
                type="button"
                onClick={() => setActiveTab('rerank')}
                className={`flex items-center gap-2 rounded-xl px-3 py-1.5 border transition-all cursor-pointer ${
                  activeTab === 'rerank'
                    ? 'border-violet-500 bg-violet-500/5 text-violet-600 dark:text-violet-400 font-semibold ring-2 ring-violet-500/20 shadow-2xs'
                    : 'border-border/80 bg-surface hover:border-violet-500/40 text-foreground'
                }`}
                title="点击切换到二次重排模型配置"
              >
                <Network className="h-3.5 w-3.5 text-violet-500" />
                <span className="text-[11px] font-medium">3. 精准精排</span>
                {defaultRerank ? (
                  <span className="max-w-[100px] truncate font-mono text-[10px] text-emerald-600 dark:text-emerald-400 font-bold">
                    {defaultRerank.name}
                  </span>
                ) : (
                  <span className="text-[10px] text-muted-foreground">可选</span>
                )}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* 2. Main Workspace Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
        {/* Left Column (4 / 12): Clean Account Profile & Preferences Card */}
        <div className="lg:col-span-4 space-y-6">
          <div className="rounded-2xl border border-border/80 bg-surface p-5 shadow-sm space-y-5">
            {/* User Identity Header */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-primary/20 via-primary/10 to-indigo-500/20 text-primary font-bold text-base shadow-2xs border border-primary/20">
                  {user?.username?.[0]?.toUpperCase() || <UserIcon className="h-5 w-5" />}
                </div>
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm font-bold text-foreground">{user?.username ?? '学习者'}</h3>
                    {user?.role === 'admin' ? (
                      <span className="inline-flex items-center gap-1 rounded-md bg-primary/10 border border-primary/20 px-1.5 py-0.5 text-[10px] font-bold text-primary">
                        <Shield className="h-3 w-3" /> 管理员
                      </span>
                    ) : (
                      <span className="inline-flex items-center rounded-md bg-surface-secondary px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground border border-border/60">
                        学员
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground font-mono">{user?.email || '未绑定邮箱'}</p>
                </div>
              </div>
            </div>

            <div className="h-px bg-border/60" />

            {/* Study Target / Bio Input */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <label className="text-xs font-bold text-foreground flex items-center gap-1.5">
                  <Sparkles className="h-3.5 w-3.5 text-primary" />
                  个性签名 / 备考目标
                </label>
                <span className="text-[10px] text-muted-foreground">{bio.length} 字</span>
              </div>
              <textarea
                className="input min-h-[96px] resize-none text-xs leading-relaxed rounded-xl bg-surface-secondary/40 focus:bg-surface border-border/70"
                value={bio}
                onChange={(e) => setBio(e.target.value)}
                placeholder="记录你的备考目标与座右铭，例如：全力备战计算机考研 / 攻克分布式高并发..."
              />
            </div>

            {/* Profile Action Bar */}
            <div className="flex items-center justify-between pt-1">
              <div>
                {profileSaved && (
                  <span className="inline-flex items-center gap-1 text-xs font-bold text-emerald-600 dark:text-emerald-400 animate-fade-in">
                    <CheckCircle2 className="h-3.5 w-3.5" /> 资料已保存
                  </span>
                )}
              </div>
              <Button
                size="sm"
                onClick={saveProfile}
                loading={savingProfile}
                icon={<Save className="h-3.5 w-3.5" />}
              >
                保存资料
              </Button>
            </div>
          </div>

          {/* Quick System Environment Summary */}
          <div className="rounded-2xl border border-border/80 bg-surface p-4.5 shadow-sm space-y-3 text-xs">
            <div className="flex items-center justify-between text-muted-foreground pb-2 border-b border-border/60">
              <span className="font-semibold text-foreground">系统就绪环境</span>
              <span className="flex items-center gap-1 text-[11px] font-mono text-emerald-600 dark:text-emerald-400">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                在线运行中
              </span>
            </div>
            <div className="space-y-2 text-[11px]">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">视觉大模型 (Vision)</span>
                <span className="font-medium text-foreground">
                  {providers.some((p) => p.supports_vision) ? (
                    <span className="text-emerald-600 dark:text-emerald-400 font-bold">已就绪</span>
                  ) : (
                    <span className="text-amber-500 font-bold">未配置 Vision 端点</span>
                  )}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">知识库向量检索</span>
                <span className="font-medium text-foreground">
                  {defaultEmbed ? (
                    <span className="text-emerald-600 dark:text-emerald-400 font-bold">已就绪 ({defaultEmbed.name})</span>
                  ) : (
                    <span className="text-muted-foreground">已启用降级模式</span>
                  )}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">精排上下文打分</span>
                <span className="font-medium text-foreground">
                  {defaultRerank ? (
                    <span className="text-emerald-600 dark:text-emerald-400 font-bold">高精重排开启</span>
                  ) : (
                    <span className="text-muted-foreground">余弦相似度直接排序</span>
                  )}
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* Right Column (8 / 12): AI Model Engine Console */}
        <div className="lg:col-span-8 space-y-5">
          {/* Segmented Category Pill Tabs */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div className="inline-flex p-1 bg-surface-secondary/70 rounded-2xl border border-border/70 max-w-full overflow-x-auto">
              {tabConfig.map((tab) => {
                const Icon = tab.icon
                const isActive = activeTab === tab.key
                return (
                  <button
                    key={tab.key}
                    type="button"
                    onClick={() => setActiveTab(tab.key)}
                    className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-semibold transition-all whitespace-nowrap cursor-pointer select-none ${
                      isActive
                        ? 'bg-surface text-foreground shadow-sm font-bold border border-border/60'
                        : 'text-muted-foreground hover:text-foreground hover:bg-surface/50 border border-transparent'
                    }`}
                  >
                    <Icon className={`h-3.5 w-3.5 ${isActive ? tab.accentColor : ''}`} />
                    <span>{tab.label}</span>
                    <span
                      className={`rounded-full px-2 py-0.5 text-[10px] font-mono font-bold ${
                        isActive
                          ? tab.badgeBg
                          : 'bg-surface-secondary text-muted-foreground'
                      }`}
                    >
                      {tab.count}
                    </span>
                  </button>
                )
              })}
            </div>

            <Button
              size="sm"
              onClick={() => handleOpenAdd()}
              icon={<Plus className="h-4 w-4" />}
              className="shadow-sm font-semibold shrink-0"
            >
              添加 {activeTab.toUpperCase()} 端点
            </Button>
          </div>

          {/* Category Info & Quick Preset Chips */}
          <div className="rounded-2xl border border-border/70 bg-surface p-4 shadow-2xs space-y-3">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 text-xs">
              <p className="text-muted-foreground text-xs leading-relaxed">
                {tabConfig.find((t) => t.key === activeTab)?.desc}
              </p>
            </div>

            {/* Quick Vendor Presets Strip */}
            <div className="flex items-center gap-2 pt-2 border-t border-border/50 flex-wrap">
              <span className="text-[11px] font-bold text-muted-foreground flex items-center gap-1">
                <Zap className="h-3 w-3 text-amber-500" /> 常用厂商预设:
              </span>
              {PRESETS[activeTab].map((preset, idx) => (
                <button
                  key={idx}
                  type="button"
                  onClick={() => handleOpenAdd(preset)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface-secondary/40 hover:bg-surface hover:border-primary/40 px-2.5 py-1 text-xs font-medium text-foreground transition-all cursor-pointer shadow-2xs hover:-translate-y-0.5"
                  title={`快速配置 ${preset.name} (${preset.model_name})`}
                >
                  <Plus className="h-3 w-3 text-primary" />
                  <span>{preset.label}</span>
                  {preset.tag && (
                    <span className="rounded bg-primary/10 px-1 py-0.2 text-[9px] font-semibold text-primary">
                      {preset.tag}
                    </span>
                  )}
                </button>
              ))}
            </div>
          </div>

          {/* Model Provider Cards List */}
          {currentTabProviders.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border bg-surface/50 p-10 text-center space-y-4">
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-surface-secondary text-muted-foreground mx-auto border border-border/80 shadow-2xs">
                {(() => {
                  const Icon = tabConfig.find((t) => t.key === activeTab)?.icon || Cpu
                  return <Icon className="h-7 w-7 text-muted-foreground/80" />
                })()}
              </div>
              <div className="space-y-1.5 max-w-md mx-auto">
                <h3 className="text-sm font-bold text-foreground">尚未挂载 {activeTab.toUpperCase()} 模型端点</h3>
                <p className="text-xs text-muted-foreground leading-relaxed">
                  添加独立模型端点以接入 AI 推理或 RAG 知识库检索。点击下方常用厂商预设一键配置。
                </p>
              </div>
              <div className="flex justify-center gap-2 pt-2 flex-wrap">
                {PRESETS[activeTab].map((preset, idx) => (
                  <Button
                    key={idx}
                    size="sm"
                    variant="outline"
                    onClick={() => handleOpenAdd(preset)}
                    icon={<Plus className="h-3.5 w-3.5 text-primary" />}
                  >
                    {preset.label}
                  </Button>
                ))}
              </div>
            </div>
          ) : (
            <div className="space-y-3.5">
              {currentTabProviders.map((p) => {
                const cardStatus = cardTestStatus[p.id]
                const isTestingThis = cardTestingId === p.id

                return (
                  <div
                    key={p.id}
                    className={`group rounded-2xl border transition-all duration-200 bg-surface p-4.5 sm:p-5 shadow-sm hover:shadow-md ${
                      p.is_default
                        ? 'border-primary/50 ring-1 ring-primary/20'
                        : 'border-border/80 hover:border-primary/30'
                    }`}
                  >
                    {/* Top Row: Provider Identity, Badges & Actions */}
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3.5 border-b border-border/60">
                      <div className="flex items-center gap-3 flex-wrap">
                        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-surface-secondary border border-border text-foreground font-bold">
                          {p.protocol === 'anthropic' ? (
                            <span className="text-xs font-mono font-bold text-amber-600 dark:text-amber-400">CL</span>
                          ) : p.protocol === 'ollama' ? (
                            <span className="text-xs font-mono font-bold text-sky-600 dark:text-sky-400">OL</span>
                          ) : (
                            <Bot className="h-4 w-4 text-primary" />
                          )}
                        </div>

                        <div>
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="font-bold text-sm sm:text-base text-foreground">{p.name}</span>

                            <Badge
                              tone={p.protocol === 'anthropic' ? 'warning' : 'primary'}
                              variant="subtle"
                              className="text-[10px] uppercase font-mono"
                            >
                              {p.protocol}
                            </Badge>

                            {p.is_default ? (
                              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[10px] font-bold text-emerald-600 dark:text-emerald-400">
                                <Check className="h-3 w-3" /> 默认使用
                              </span>
                            ) : (
                              <button
                                type="button"
                                onClick={() => setDefault(p.id)}
                                className="text-[11px] text-muted-foreground hover:text-primary hover:underline cursor-pointer font-medium transition-colors"
                              >
                                设为默认
                              </button>
                            )}

                            {p.supports_vision && (
                              <Badge tone="info" variant="subtle" className="text-[10px]">
                                视觉 (Vision)
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* Action Buttons */}
                      <div className="flex items-center gap-1.5 self-end sm:self-center shrink-0">
                        {/* Quick Test Inline Button */}
                        <Button
                          variant="ghost"
                          size="xs"
                          loading={isTestingThis}
                          onClick={() => testCardProvider(p)}
                          icon={<Zap className="h-3 w-3 text-amber-500" />}
                          title="测试当前模型连通性与延迟"
                        >
                          测试
                        </Button>

                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => {
                            setEditor(p)
                            setApiKeyInput('')
                            setTestResult(null)
                          }}
                          icon={<Pencil className="h-3 w-3 text-muted-foreground" />}
                        >
                          编辑
                        </Button>

                        <Button
                          variant="ghost"
                          size="xs"
                          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                          onClick={async () => {
                            if (confirm(`确定删除模型端点 "${p.name}"？`)) {
                              await client.delete(`/providers/${p.id}`)
                              load()
                            }
                          }}
                          title="删除该模型端点"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>

                    {/* Metadata Grid */}
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-3.5 text-xs">
                      {/* Endpoint URL */}
                      <div className="flex items-center justify-between rounded-xl bg-surface-secondary/40 px-3 py-2 border border-border/50 group/item">
                        <div className="min-w-0 flex-1 pr-1">
                          <span className="text-[10px] text-muted-foreground font-semibold block">端点 (Base URL)</span>
                          <span className="font-mono text-xs text-foreground truncate block" title={p.base_url}>
                            {p.base_url || 'https://api.openai.com/v1'}
                          </span>
                        </div>
                        <button
                          type="button"
                          onClick={() => copyEndpoint(p)}
                          className="text-muted-foreground hover:text-primary transition-colors p-1"
                          title="复制端点 URL"
                        >
                          {copiedId === p.id ? (
                            <Check className="h-3.5 w-3.5 text-emerald-500 animate-scale" />
                          ) : (
                            <Copy className="h-3.5 w-3.5 opacity-60 group-hover/item:opacity-100" />
                          )}
                        </button>
                      </div>

                      {/* Model Identifier */}
                      <div className="rounded-xl bg-surface-secondary/40 px-3 py-2 border border-border/50 min-w-0">
                        <span className="text-[10px] text-muted-foreground font-semibold block">模型标识 (Model ID)</span>
                        <span className="font-mono text-xs font-bold text-foreground truncate block" title={p.model_name}>
                          {p.model_name}
                        </span>
                      </div>

                      {/* API Key */}
                      <div className="rounded-xl bg-surface-secondary/40 px-3 py-2 border border-border/50 min-w-0">
                        <span className="text-[10px] text-muted-foreground font-semibold block">API 密钥 (Key)</span>
                        <span className="font-mono text-xs text-muted-foreground truncate block">
                          {p.api_key_masked || '未配置密钥'}
                        </span>
                      </div>
                    </div>

                    {/* Inline Test Result Toast Banner if available */}
                    {cardStatus && (
                      <div
                        className={`mt-3 rounded-xl px-3 py-2 text-xs flex items-center justify-between animate-fade-in ${
                          cardStatus.ok
                            ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20'
                            : 'bg-destructive/10 text-destructive border border-destructive/20'
                        }`}
                      >
                        <div className="flex items-center gap-1.5 font-mono text-[11px] truncate">
                          {cardStatus.ok ? (
                            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                          ) : (
                            <Zap className="h-3.5 w-3.5 text-destructive shrink-0" />
                          )}
                          <span className="truncate">{cardStatus.msg}</span>
                        </div>
                        {cardStatus.latency && (
                          <span className="font-mono font-bold text-[10px] shrink-0 ml-2">
                            {cardStatus.latency}ms
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

      {/* 3. Provider Editor Modal */}
      <Modal
        open={!!editor}
        onClose={() => setEditor(null)}
        title={
          isNew
            ? `添加 ${form.provider_type?.toUpperCase()} 模型端点`
            : `编辑 ${form.provider_type?.toUpperCase()} 模型端点`
        }
        description={`配置独立的 ${form.provider_type?.toUpperCase()} 服务端点规范与访问凭证`}
        width="max-w-xl"
      >
        <div className="space-y-4">
          {/* Preset Quick Fill Bar */}
          <div className="rounded-xl border border-border bg-surface-secondary/40 p-3 space-y-2">
            <span className="text-[11px] font-bold text-muted-foreground flex items-center gap-1">
              <Sparkles className="h-3 w-3 text-primary" /> 点击快速填入常用厂商预设：
            </span>
            <div className="flex flex-wrap gap-1.5">
              {PRESETS[form.provider_type || activeTab].map((p, idx) => (
                <button
                  key={idx}
                  type="button"
                  onClick={() => applyPresetToForm(p)}
                  className="rounded-lg border border-border bg-surface px-2.5 py-1 text-xs font-semibold text-foreground hover:border-primary/40 hover:text-primary transition-all shadow-2xs active:scale-95 cursor-pointer"
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-foreground block">配置标识名称</label>
              <input
                className="input text-xs"
                value={form.name}
                onChange={(e) => update({ name: e.target.value })}
                placeholder="例如：DeepSeek-V3 / Chat"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-foreground block">协议规范</label>
              <select
                className="input text-xs"
                value={form.protocol}
                onChange={(e) => update({ protocol: e.target.value })}
              >
                <option value="openai">OpenAI Compatible (通用网关 / 硅基流动等)</option>
                {form.provider_type === 'llm' && (
                  <option value="anthropic">Anthropic Compatible (Claude 规范)</option>
                )}
                {form.provider_type === 'embedding' && (
                  <>
                    <option value="ollama">Ollama API (本地 /api/embed)</option>
                    <option value="huggingface">HuggingFace / TEI 规范</option>
                    <option value="cohere">Cohere Embed API (/v1/embed)</option>
                  </>
                )}
                {form.provider_type === 'rerank' && (
                  <option value="http">专用 HTTP Rerank API (SiliconFlow / Cohere 规范)</option>
                )}
              </select>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">Base URL 端点地址</label>
            <input
              className="input text-xs font-mono"
              value={form.base_url}
              onChange={(e) => update({ base_url: e.target.value })}
              placeholder="https://api.deepseek.com 或 https://api.siliconflow.cn/v1"
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">模型标识 (Model Identifier)</label>
            <input
              className="input text-xs font-mono"
              value={form.model_name}
              onChange={(e) => update({ model_name: e.target.value })}
              placeholder={
                form.provider_type === 'embedding'
                  ? 'BAAI/bge-m3 或 text-embedding-3-small'
                  : form.provider_type === 'rerank'
                  ? 'deepseek-chat 或 BAAI/bge-reranker-v2-m3'
                  : 'deepseek-chat 或 gpt-4o-mini'
              }
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-foreground block">
              API Key 访问密钥
              {form.api_key_masked && (
                <span className="text-[11px] text-muted-foreground ml-1.5 font-normal">
                  (已配置 {form.api_key_masked}，留空则保持原密钥)
                </span>
              )}
            </label>
            <input
              className="input text-xs font-mono"
              type="password"
              value={apiKeyInput}
              onChange={(e) => setApiKeyInput(e.target.value)}
              placeholder="sk-..."
            />
          </div>

          <div className="space-y-2 pt-1">
            <label className="flex items-center gap-2.5 text-xs text-foreground cursor-pointer select-none">
              <input
                type="checkbox"
                checked={form.is_default}
                onChange={(e) => update({ is_default: e.target.checked })}
                className="rounded border-border accent-primary h-4 w-4 cursor-pointer"
              />
              <span className="font-semibold">设为该分类的默认使用模型</span>
            </label>

            {form.provider_type === 'llm' && (
              <label className="flex items-start gap-2.5 rounded-xl border border-primary/20 bg-primary/[0.03] p-3 text-xs text-foreground cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={form.supports_vision}
                  onChange={(e) => update({ supports_vision: e.target.checked })}
                  className="mt-0.5 rounded border-border accent-primary h-4 w-4 cursor-pointer"
                />
                <div className="space-y-0.5">
                  <span className="block font-semibold">启用图片输入与多模态视觉 (Vision)</span>
                  <span className="block text-[11px] text-muted-foreground leading-relaxed">
                    测试连通性将发送 1×1 验证图；错题 OCR 与视觉 PDF 智能导入将使用该默认模型。
                  </span>
                </div>
              </label>
            )}
          </div>

          {/* Test Connection Bench */}
          <div className="rounded-xl bg-surface-secondary/50 p-3.5 flex flex-col gap-2.5 text-xs border border-border/70">
            <div className="flex items-center justify-between gap-3">
              <Button
                variant="outline"
                size="xs"
                onClick={test}
                loading={testing}
                icon={<Zap className="h-3 w-3 text-amber-500" />}
              >
                测试连通性
              </Button>
              {testResult && (
                <div className="flex items-center gap-1.5 text-xs">
                  {testResult.ok ? (
                    <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400 font-bold font-mono">
                      <Check className="h-3.5 w-3.5" /> 连接成功 {testResult.latency ? `(${testResult.latency}ms)` : ''}
                    </span>
                  ) : (
                    <span className="text-destructive font-bold">连接失败</span>
                  )}
                </div>
              )}
            </div>
            {testResult && (
              <div
                className={`rounded-lg p-2.5 font-mono text-[11px] break-all ${
                  testResult.ok
                    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20'
                    : 'bg-destructive/10 text-destructive border border-destructive/20'
                }`}
              >
                {testResult.message}
              </div>
            )}
          </div>

          {/* Modal Action Buttons */}
          <div className="flex justify-end gap-2 pt-3 border-t border-border/60">
            <Button variant="outline" onClick={() => setEditor(null)}>
              取消
            </Button>
            <Button onClick={save} className="shadow-sm font-bold">
              保存配置
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
