import React, { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Button } from '@/components/ui'
import type { AuthResponse } from '@/types'
import { Lock, Mail, User as UserIcon } from 'lucide-react'

export default function MobileRegister() {
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const resp = await client.post('/auth/register', { username, email, password })
      const data = resp.data.data as AuthResponse
      localStorage.setItem('qt_refresh_token', data.refresh_token)
      setAuth(data.access_token, data.user)
      navigate('/', { replace: true })
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen w-full flex flex-col justify-between bg-background p-5 pt-10 pb-8 select-none">
      {/* Top Header */}
      <div className="space-y-6 text-center">
        <div className="relative mx-auto flex h-14 w-14 items-center justify-center rounded-2xl overflow-hidden shadow-glow ring-2 ring-primary/30 bg-surface-secondary">
          <img src="/logo.jpg" alt="Logo" className="h-full w-full object-cover" />
        </div>

        <div>
          <h1 className="text-xl font-bold tracking-tight text-foreground">注册 QuizTrace 账号</h1>
          <p className="text-xs text-muted-foreground mt-1">开启 AI 原生自适应刷题与个性化知识图谱</p>
        </div>
      </div>

      {/* Main Form Card */}
      <div className="my-auto w-full rounded-3xl border border-border/80 bg-surface p-6 shadow-card space-y-4">
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">
              用户名
            </label>
            <div className="relative">
              <UserIcon className="absolute left-3.5 top-3 h-4 w-4 text-muted-foreground/60" />
              <input
                className="input pl-10 h-11 rounded-2xl text-sm"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="设置 3-32 位用户名"
                required
                minLength={3}
                maxLength={32}
              />
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">
              电子邮箱
            </label>
            <div className="relative">
              <Mail className="absolute left-3.5 top-3 h-4 w-4 text-muted-foreground/60" />
              <input
                className="input pl-10 h-11 rounded-2xl text-sm"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="name@example.com"
                required
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
                placeholder="至少 6 位字符"
                required
                minLength={6}
              />
            </div>
          </div>

          {error && (
            <div className="rounded-2xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-xs text-destructive">
              {error}
            </div>
          )}

          <Button type="submit" loading={loading} className="w-full h-11 rounded-2xl text-sm font-bold shadow-glow">
            立即创建账号
          </Button>
        </form>
      </div>

      {/* Footer Navigation */}
      <div className="text-center pt-4">
        <p className="text-xs text-muted-foreground">
          已有账号？{' '}
          <Link to="/login" className="font-bold text-primary underline underline-offset-4">
            直接登录
          </Link>
        </p>
      </div>
    </div>
  )
}
