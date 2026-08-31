import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Badge, Button, Skeleton } from '@/components/ui'
import { MobileBottomSheet } from '@/components/mobile/MobileBottomSheet'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { MobileSegmentedControl } from '@/components/mobile/MobileSegmentedControl'
import { showToast } from '@/components/mobile/MobileToast'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import type { ProviderConfig, ProviderType } from '@/types'
import {
  Bot,
  Check,
  Cpu,
  Database,
  Edit3,
  LogOut,
  Monitor,
  Moon,
  Network,
  Plus,
  Radio,
  Save,
  Settings as SettingsIcon,
  ShieldCheck,
  Sparkles,
  Sun,
  Trash2,
  User as UserIcon,
  Zap,
} from 'lucide-react'

export default function MobileSettings() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const logout = useAuthStore((s) => s.logout)
  const { darkMode, toggleDark, toggleViewMode } = useUIStore()
  const navigate = useNavigate()

  const [providers, setProviders] = useState<ProviderConfig[]>([])
  const [activeTab, setActiveTab] = useState<ProviderType>('llm')
  const [loading, setLoading] = useState(true)

  // Profile Edit
  const [bio, setBio] = useState(user?.bio ?? '')
  const [bioOpen, setBioOpen] = useState(false)
  const [savingProfile, setSavingProfile] = useState(false)

  // Provider Edit Sheet
  const [editor, setEditor] = useState<ProviderConfig | 'new' | null>(null)
  const [apiKeyInput, setApiKeyInput] = useState('')
  const [savingProvider, setSavingProvider] = useState(false)
  const [testingId, setTestingId] = useState<number | null>(null)

  const loadProviders = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await client.get('/providers')
      setProviders(resp.data.data)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadProviders()
  }, [loadProviders])

  useEffect(() => {
    if (user) setBio(user.bio ?? '')
  }, [user])

  const currentTabProviders = providers.filter((p) => (p.provider_type || 'llm') === activeTab)

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
  const isNew = editor === 'new' || Boolean(editor && editor.id === 0)

  const handleOpenAdd = () => {
    let defaultPreset: Partial<ProviderConfig> = {}
    if (activeTab === 'llm') {
      defaultPreset = {
        name: 'DeepSeek-V3',
        protocol: 'openai',
        base_url: 'https://api.deepseek.com',
        model_name: 'deepseek-chat',
      }
    } else if (activeTab === 'embedding') {
      defaultPreset = {
        name: 'SiliconFlow BGE-M3',
        protocol: 'openai',
        base_url: 'https://api.siliconflow.cn/v1',
        model_name: 'BAAI/bge-m3',
      }
    } else if (activeTab === 'rerank') {
      defaultPreset = {
        name: 'LLM Reranker',
        protocol: 'openai',
        base_url: 'https://api.deepseek.com',
        model_name: 'deepseek-chat',
      }
    }

    setEditor({
      id: 0,
      provider_type: activeTab,
      name: defaultPreset.name || '',
      protocol: defaultPreset.protocol || 'openai',
      base_url: defaultPreset.base_url || '',
      api_key_masked: '',
      model_name: defaultPreset.model_name || '',
      is_default: currentTabProviders.length === 0,
      supports_vision: false,
    })
    setApiKeyInput('')
  }

  const saveProvider = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editor) return
    setSavingProvider(true)
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
      showToast('模型配置已保存', 'success')
      setEditor(null)
      setApiKeyInput('')
      loadProviders()
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setSavingProvider(false)
    }
  }

  const testProvider = async (p: ProviderConfig) => {
    setTestingId(p.id)
    try {
      const resp = await client.post(`/providers/${p.id}/test`)
      const res = resp.data.data
      if (res.ok) {
        showToast(`连通性测试成功！耗时 ${res.latency || 0}ms`, 'success')
      } else {
        showToast(`测试失败: ${res.message}`, 'error')
      }
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setTestingId(null)
    }
  }

  const deleteProvider = async (id: number) => {
    try {
      await client.delete(`/providers/${id}`)
      showToast('配置已删除', 'success')
      loadProviders()
    } catch (e) {
      showToast(errMsg(e), 'error')
    }
  }

  const saveProfile = async () => {
    setSavingProfile(true)
    try {
      const resp = await client.put('/users/me', { bio })
      setUser(resp.data.data)
      showToast('个人资料已保存', 'success')
      setBioOpen(false)
    } catch (e) {
      showToast(errMsg(e), 'error')
    } finally {
      setSavingProfile(false)
    }
  }

  return (
    <MobilePullToRefresh onRefresh={loadProviders} className="p-4 space-y-4 pb-24 select-none">
      {/* 1. User Profile Card */}
      <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-primary/20 to-primary/5 text-primary font-bold text-base border border-primary/20 shadow-subtle">
              {user?.avatar ? (
                <img src={user.avatar} alt="Avatar" className="h-full w-full rounded-2xl object-cover" />
              ) : (
                <UserIcon className="h-6 w-6" />
              )}
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <h3 className="text-sm font-bold text-foreground">{user?.username || '学习者'}</h3>
                {user?.role === 'admin' && (
                  <span className="flex items-center gap-0.5 rounded-md bg-primary/10 px-1.5 py-0.2 text-[9px] font-bold text-primary">
                    <ShieldCheck className="h-3 w-3" />
                    管理员
                  </span>
                )}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5">{user?.email || '未绑定邮箱'}</p>
            </div>
          </div>

          <button
            onClick={() => setBioOpen(true)}
            className="flex items-center gap-1 rounded-xl bg-surface-secondary px-2.5 py-1 text-xs font-semibold text-foreground active:scale-95"
          >
            <Edit3 className="h-3 w-3 text-primary" />
            <span>编辑简介</span>
          </button>
        </div>

        {user?.bio && (
          <div className="rounded-xl bg-surface-secondary/50 p-2.5 text-xs text-muted-foreground leading-relaxed border border-border/50">
            {user.bio}
          </div>
        )}
      </div>

      {/* 2. Appearance & Theme Switch */}
      <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-amber-500/10 text-amber-500">
            {darkMode ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
          </div>
          <div>
            <span className="text-xs font-bold text-foreground block">深浅外观模式</span>
            <span className="text-[10px] text-muted-foreground">当前：{darkMode ? '深色夜间模式' : '浅色明亮模式'}</span>
          </div>
        </div>

        <button
          onClick={toggleDark}
          className={`flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-bold transition-all active:scale-95 ${
            darkMode
              ? 'bg-amber-500/15 text-amber-400 border border-amber-500/30'
              : 'bg-surface-secondary text-foreground border border-border'
          }`}
        >
          {darkMode ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
          <span>{darkMode ? '切至浅色' : '切至深色'}</span>
        </button>
      </div>

      {/* 3. View Mode Switch */}
      <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Monitor className="h-4 w-4" />
          </div>
          <div>
            <span className="text-xs font-bold text-foreground block">视图模式</span>
            <span className="text-[10px] text-muted-foreground">当前：手机端触屏视图</span>
          </div>
        </div>

        <button
          onClick={toggleViewMode}
          className="flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-bold bg-primary/10 text-primary border border-primary/20 transition-all active:scale-95"
        >
          <Monitor className="h-3.5 w-3.5" />
          <span>切换至电脑端</span>
        </button>
      </div>

      {/* 3. AI Providers / Model Configurations */}
      <div className="space-y-2.5">
        <div className="flex items-center justify-between px-1">
          <div className="flex items-center gap-1.5">
            <Sparkles className="h-4 w-4 text-primary" />
            <h3 className="text-xs font-bold text-foreground">AI 引擎与模型配置</h3>
          </div>
          <button
            onClick={handleOpenAdd}
            className="flex items-center gap-1 text-xs font-bold text-primary active:scale-95"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>添加模型</span>
          </button>
        </div>

        {/* Tab Switcher */}
        <MobileSegmentedControl
          items={[
            { key: 'llm', label: '对话模型', icon: <Bot className="h-3.5 w-3.5" /> },
            { key: 'embedding', label: '向量嵌入', icon: <Database className="h-3.5 w-3.5" /> },
            { key: 'rerank', label: '重排模型', icon: <Cpu className="h-3.5 w-3.5" /> },
          ]}
          active={activeTab}
          onChange={(tab) => setActiveTab(tab)}
        />

        {/* Providers List */}
        {currentTabProviders.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-border/80 bg-surface p-6 text-center space-y-2">
            <Cpu className="h-8 w-8 text-muted-foreground mx-auto opacity-50" />
            <p className="text-xs text-muted-foreground">尚未配置此类 Provider</p>
            <Button size="xs" onClick={handleOpenAdd} icon={<Plus className="h-3 w-3" />}>
              立即添加
            </Button>
          </div>
        ) : (
          <div className="space-y-2.5">
            {currentTabProviders.map((p) => (
              <div
                key={p.id}
                className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-2.5"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <h4 className="text-xs font-bold text-foreground truncate">{p.name}</h4>
                      {p.is_default && (
                        <Badge tone="success" variant="subtle" className="text-[9px] py-0 px-1 font-bold">
                          默认
                        </Badge>
                      )}
                      {p.supports_vision && (
                        <Badge tone="info" variant="subtle" className="text-[9px] py-0 px-1 font-bold">
                          视觉
                        </Badge>
                      )}
                    </div>
                    <div className="text-[10px] text-muted-foreground font-mono mt-0.5 truncate">
                      {p.model_name || '未指定模型'} · {p.protocol}
                    </div>
                  </div>

                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => {
                        setEditor(p)
                        setApiKeyInput('')
                      }}
                      className="p-1 text-muted-foreground hover:text-foreground active:scale-90"
                      aria-label="编辑"
                    >
                      <Edit3 className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => deleteProvider(p.id)}
                      className="p-1 text-muted-foreground hover:text-destructive active:scale-90"
                      aria-label="删除"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                <div className="flex items-center justify-between border-t border-border/60 pt-2 text-xs">
                  <span className="text-[10px] text-muted-foreground font-mono truncate max-w-[170px]">
                    {p.base_url || 'https://api.openai.com'}
                  </span>
                  <Button
                    size="xs"
                    variant="outline"
                    loading={testingId === p.id}
                    onClick={() => testProvider(p)}
                    className="h-7 text-[10px] rounded-lg"
                  >
                    测试连通性
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 4. Logout Card */}
      <div className="pt-2">
        <button
          onClick={() => {
            logout()
            navigate('/login')
          }}
          className="w-full flex items-center justify-center gap-2 rounded-2xl border border-destructive/30 bg-destructive/10 py-3 text-xs font-bold text-destructive active:scale-[0.98] shadow-subtle"
        >
          <LogOut className="h-4 w-4" />
          <span>退出当前账号</span>
        </button>
      </div>

      {/* Bio Edit Sheet */}
      <MobileBottomSheet open={bioOpen} onClose={() => setBioOpen(false)} title="编辑个人简介">
        <div className="space-y-4">
          <textarea
            className="input min-h-[100px] rounded-2xl p-3 text-xs resize-none"
            placeholder="写一句你的学习目标或座右铭..."
            value={bio}
            onChange={(e) => setBio(e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setBioOpen(false)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button loading={savingProfile} onClick={saveProfile} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              保存简介
            </Button>
          </div>
        </div>
      </MobileBottomSheet>

      {/* Provider Add/Edit Sheet */}
      <MobileBottomSheet
        open={!!editor}
        onClose={() => setEditor(null)}
        title={isNew ? '添加模型配置' : '编辑模型配置'}
      >
        <form onSubmit={saveProvider} className="space-y-3.5">
          <div className="space-y-1">
            <label className="text-xs font-bold text-foreground block">配置别名 *</label>
            <input
              className="input h-9 rounded-xl text-xs"
              value={form.name}
              onChange={(e) => setEditor(editor === 'new' ? editor : { ...form, name: e.target.value })}
              placeholder="例如：DeepSeek 官方 Chat"
              required
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs font-bold text-foreground block">API Base URL *</label>
            <input
              className="input h-9 rounded-xl text-xs"
              value={form.base_url}
              onChange={(e) => setEditor(editor === 'new' ? editor : { ...form, base_url: e.target.value })}
              placeholder="https://api.deepseek.com"
              required
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs font-bold text-foreground block">
              API Key {isNew ? '*' : '(留空表示保持原 Key)'}
            </label>
            <input
              className="input h-9 rounded-xl text-xs"
              type="password"
              value={apiKeyInput}
              onChange={(e) => setApiKeyInput(e.target.value)}
              placeholder={form.api_key_masked || 'sk-••••••••'}
              required={isNew}
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs font-bold text-foreground block">模型名称 (Model ID) *</label>
            <input
              className="input h-9 rounded-xl text-xs"
              value={form.model_name}
              onChange={(e) => setEditor(editor === 'new' ? editor : { ...form, model_name: e.target.value })}
              placeholder="deepseek-chat / gpt-4o"
              required
            />
          </div>

          <div className="flex items-center justify-between pt-1">
            <label className="text-xs font-semibold text-foreground">设为默认 Provider</label>
            <input
              type="checkbox"
              checked={Boolean(form.is_default)}
              onChange={(e) => setEditor(editor === 'new' ? editor : { ...form, is_default: e.target.checked })}
              className="h-4 w-4 rounded text-primary focus:ring-primary"
            />
          </div>

          <div className="flex items-center justify-between">
            <label className="text-xs font-semibold text-foreground">支持 Vision 图像识别</label>
            <input
              type="checkbox"
              checked={Boolean(form.supports_vision)}
              onChange={(e) => setEditor(editor === 'new' ? editor : { ...form, supports_vision: e.target.checked })}
              className="h-4 w-4 rounded text-primary focus:ring-primary"
            />
          </div>

          <div className="flex items-center gap-2 pt-2">
            <Button type="button" variant="outline" onClick={() => setEditor(null)} className="flex-1 h-10 rounded-xl text-xs">
              取消
            </Button>
            <Button type="submit" loading={savingProvider} className="flex-1 h-10 rounded-xl text-xs font-bold shadow-glow">
              保存配置
            </Button>
          </div>
        </form>
      </MobileBottomSheet>
    </MobilePullToRefresh>
  )
}
