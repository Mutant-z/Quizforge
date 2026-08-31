import { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Button } from '@/components/ui'
import type { AuthResponse } from '@/types'
import { KeyRound, Lock, User as UserIcon } from 'lucide-react'

export default function Login() {
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
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4 py-12">
      {/* Subtle background glow */}
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-primary/5 via-background to-background pointer-events-none" />

      <div className="relative z-10 w-full max-w-sm space-y-6">
        {/* Brand header */}
        <div className="text-center space-y-3">
          <div className="relative mx-auto flex h-16 w-16 items-center justify-center rounded-2xl overflow-hidden shadow-glow ring-2 ring-primary/30 bg-surface-secondary">
            <img src="/logo.jpg" alt="QuizTrace Logo" className="h-full w-full object-cover" />
            <span className="absolute top-1 right-1 flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary/40 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-primary"></span>
            </span>
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">题迹 QuizTrace</h1>
            <p className="text-xs text-muted-foreground mt-1">AI 原生自适应刷题与长期记忆平台</p>
          </div>
        </div>

        {/* Login Card */}
        <div className="rounded-3xl border border-border/80 bg-surface p-6 sm:p-8 shadow-card space-y-5">
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-1.5">
              <label className="label block">用户名 / 邮箱</label>
              <div className="relative">
                <UserIcon className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/60" />
                <input
                  className="input pl-9 rounded-xl"
                  value={account}
                  onChange={(e) => setAccount(e.target.value)}
                  placeholder="admin 或 demo"
                  required
                  autoFocus
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="label block">密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/60" />
                <input
                  className="input pl-9 rounded-xl"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  required
                />
              </div>
            </div>

            {error && (
              <div className="rounded-2xl border border-destructive/30 bg-destructive/10 px-3.5 py-2 text-xs text-destructive">
                {error}
              </div>
            )}

            <Button className="w-full shadow-glow" size="lg" loading={loading} disabled={loading}>
              登录系统
            </Button>
          </form>

          {/* Demo account quick fill */}
          <div className="rounded-2xl border border-border/60 bg-surface-secondary/50 p-3.5 text-xs space-y-2">
            <div className="text-[11px] font-bold text-muted-foreground flex items-center gap-1.5">
              <KeyRound className="h-3.5 w-3.5 text-primary" />
              <span>快速体验演示账号</span>
            </div>
            <div className="flex gap-2 pt-0.5">
              <button
                type="button"
                onClick={() => fillDemo('admin', 'admin123')}
                className="flex-1 rounded-xl border border-border bg-surface px-2.5 py-1.5 text-[11px] font-semibold text-foreground hover:bg-surface-secondary transition-all shadow-subtle active:scale-[0.98]"
              >
                管理员 (admin)
              </button>
              <button
                type="button"
                onClick={() => fillDemo('demo', 'user123')}
                className="flex-1 rounded-xl border border-border bg-surface px-2.5 py-1.5 text-[11px] font-semibold text-foreground hover:bg-surface-secondary transition-all shadow-subtle active:scale-[0.98]"
              >
                普通用户 (demo)
              </button>
            </div>
          </div>

          <div className="text-center text-xs text-muted-foreground">
            尚未拥有账号？{' '}
            <Link
              to="/register"
              className="text-primary font-semibold underline underline-offset-4 hover:opacity-80"
            >
              创建新账号
            </Link>
          </div>
        </div>
      </div>
    </div>
  )
}
