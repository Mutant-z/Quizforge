import React, { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Button, Spinner } from '@/components/ui'
import type { AuthResponse } from '@/types'
import { KeyRound, Lock, Sparkles, User as UserIcon } from 'lucide-react'

export default function MobileLogin() {
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()
  const location = useLocation()

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const resp = await client.post('/auth/login', { account, password })
      const data = resp.data.data as AuthResponse
      localStorage.setItem('qt_refresh_token', data.refresh_token)
      setAuth(data.access_token, data.user)
      const from = (location.state as { from?: string })?.from || '/'
      navigate(from, { replace: true })
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setLoading(false)
    }
  }

  const fillDemo = (acc: string, pwd: string) => {
    setAccount(acc)
    setPassword(pwd)
  }

  return (
    <div className="min-h-screen w-full flex flex-col justify-between bg-background p-5 pt-10 pb-8 select-none">
      {/* Top Header */}
      <div className="space-y-6 text-center">
        <div className="relative mx-auto flex h-14 w-14 items-center justify-center rounded-2xl overflow-hidden shadow-glow ring-2 ring-primary/30 bg-surface-secondary">
          <img src="/logo.jpg" alt="Logo" className="h-full w-full object-cover" />
          <span className="absolute top-1 right-1 flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary/40 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-primary"></span>
          </span>
        </div>

        <div>
          <h1 className="text-xl font-bold tracking-tight text-foreground flex items-center justify-center gap-1.5">
            题迹 QuizTrace
            <span className="rounded-md bg-primary/10 px-1.5 py-0.2 text-[10px] font-mono font-bold text-primary">
              AI
            </span>
          </h1>
          <p className="text-xs text-muted-foreground mt-1">自适应智能刷题与长期记忆系统</p>
        </div>
      </div>

      {/* Main Card */}
      <div className="my-auto w-full rounded-3xl border border-border/80 bg-surface p-6 shadow-card space-y-4">
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">
              用户名 / 邮箱
            </label>
            <div className="relative">
              <UserIcon className="absolute left-3.5 top-3 h-4 w-4 text-muted-foreground/60" />
              <input
                className="input pl-10 h-11 rounded-2xl text-sm"
                value={account}
                onChange={(e) => setAccount(e.target.value)}
                placeholder="请输入用户名或邮箱"
                required
                autoComplete="username"
              />
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">密码</label>
            <div className="relative">
              <Lock className="absolute left-3.5 top-3 h-4 w-4 text-muted-foreground/60" />
              <input
                className="input pl-10 h-11 rounded-2xl text-sm"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
                autoComplete="current-password"
              />
            </div>
          </div>

          {error && (
            <div className="rounded-2xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-xs text-destructive">
              {error}
            </div>
          )}

          <Button type="submit" loading={loading} className="w-full h-11 rounded-2xl text-sm font-bold shadow-glow">
            登录进入学习空间
          </Button>
        </form>

        {/* Demo Fast Logins */}
        <div className="pt-2 border-t border-border/60">
          <div className="flex items-center justify-between text-[11px] text-muted-foreground mb-2">
            <span>快速体验 Demo 账号：</span>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={() => fillDemo('admin', 'admin123')}
              className="flex items-center justify-center gap-1.5 rounded-xl border border-border/70 bg-surface-secondary/70 py-2 text-xs font-semibold text-foreground active:scale-95 transition-all"
            >
              <KeyRound className="h-3 w-3 text-primary" />
              <span>管理员 admin</span>
            </button>
            <button
              type="button"
              onClick={() => fillDemo('demo', 'demo123')}
              className="flex items-center justify-center gap-1.5 rounded-xl border border-border/70 bg-surface-secondary/70 py-2 text-xs font-semibold text-foreground active:scale-95 transition-all"
            >
              <Sparkles className="h-3 w-3 text-amber-500" />
              <span>学习者 demo</span>
            </button>
          </div>
        </div>
      </div>

      {/* Footer Navigation */}
      <div className="text-center pt-4">
        <p className="text-xs text-muted-foreground">
          还没有账号？{' '}
          <Link to="/register" className="font-bold text-primary underline underline-offset-4">
            立即注册
          </Link>
        </p>
      </div>
    </div>
  )
}
