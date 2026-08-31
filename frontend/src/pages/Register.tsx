import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import client, { errMsg } from '@/api/client'
import { Button } from '@/components/ui'
import { Lock, Mail, User as UserIcon } from 'lucide-react'

export default function Register() {
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const navigate = useNavigate()

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      await client.post('/auth/register', { username, email, password })
      navigate('/login')
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4 py-12">
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-primary/5 via-background to-background pointer-events-none" />

      <div className="relative z-10 w-full max-w-sm space-y-6">
        <div className="text-center space-y-2">
          <div className="relative mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground text-xl font-bold shadow-glow">
            <span>迹</span>
            <span className="absolute -top-0.5 -right-0.5 flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary/40 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-primary"></span>
            </span>
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">创建新账号</h1>
          <p className="text-xs text-muted-foreground">加入题迹，开启 AI 原生自适应刷题之旅</p>
        </div>

        <div className="rounded-3xl border border-border/80 bg-surface p-6 sm:p-8 shadow-card space-y-5">
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-1.5">
              <label className="label block">用户名</label>
              <div className="relative">
                <UserIcon className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/60" />
                <input
                  className="input pl-9 rounded-xl"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="至少 2 个字符"
                  required
                  minLength={2}
                  autoFocus
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="label block">电子邮箱</label>
              <div className="relative">
                <Mail className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/60" />
                <input
                  className="input pl-9 rounded-xl"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@example.com"
                  required
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="label block">登录密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground/60" />
                <input
                  className="input pl-9 rounded-xl"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="至少 6 位密码"
                  required
                  minLength={6}
                />
              </div>
            </div>

            {error && (
              <div className="rounded-2xl border border-destructive/30 bg-destructive/10 px-3.5 py-2 text-xs text-destructive">
                {error}
              </div>
            )}

            <Button className="w-full shadow-glow" size="lg" loading={loading} disabled={loading}>
              立即注册
            </Button>
          </form>

          <div className="text-center text-xs text-muted-foreground">
            已有账号？{' '}
            <Link
              to="/login"
              className="text-primary font-semibold underline underline-offset-4 hover:opacity-80"
            >
              直接登录
            </Link>
          </div>
        </div>
      </div>
    </div>
  )
}
