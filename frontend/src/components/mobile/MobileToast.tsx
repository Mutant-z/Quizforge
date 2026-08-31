import React, { useEffect, useState } from 'react'
import { AlertCircle, CheckCircle2, Info, X, AlertTriangle } from 'lucide-react'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

interface ToastMessage {
  id: string
  message: string
  type: ToastType
  duration?: number
}

let toastListeners: ((toasts: ToastMessage[]) => void)[] = []
let activeToasts: ToastMessage[] = []

function notify() {
  toastListeners.forEach((fn) => fn([...activeToasts]))
}

export function showToast(message: string, type: ToastType = 'info', duration = 2500) {
  const id = Math.random().toString(36).substring(2, 9)
  const item: ToastMessage = { id, message, type, duration }
  activeToasts = [...activeToasts.slice(-2), item] // Max 3 toasts at once
  notify()

  setTimeout(() => {
    activeToasts = activeToasts.filter((t) => t.id !== id)
    notify()
  }, duration)
}

export function MobileToastContainer() {
  const [toasts, setToasts] = useState<ToastMessage[]>([])

  useEffect(() => {
    const handler = (list: ToastMessage[]) => setToasts(list)
    toastListeners.push(handler)
    return () => {
      toastListeners = toastListeners.filter((fn) => fn !== handler)
    }
  }, [])

  if (toasts.length === 0) return null

  const icons = {
    success: <CheckCircle2 className="h-4 w-4 text-emerald-500 shrink-0" />,
    error: <AlertCircle className="h-4 w-4 text-rose-500 shrink-0" />,
    warning: <AlertTriangle className="h-4 w-4 text-amber-500 shrink-0" />,
    info: <Info className="h-4 w-4 text-sky-500 shrink-0" />,
  }

  const borderTones = {
    success: 'border-emerald-500/30 bg-surface/95 dark:bg-surface-elevated/95 text-foreground shadow-glow-success',
    error: 'border-rose-500/30 bg-surface/95 dark:bg-surface-elevated/95 text-foreground shadow-glow-danger',
    warning: 'border-amber-500/30 bg-surface/95 dark:bg-surface-elevated/95 text-foreground shadow-glow-amber',
    info: 'border-sky-500/30 bg-surface/95 dark:bg-surface-elevated/95 text-foreground shadow-glow',
  }

  return (
    <div className="fixed top-4 inset-x-0 z-[9999] flex flex-col items-center gap-2 pointer-events-none px-4">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`pointer-events-auto flex items-center gap-2.5 rounded-2xl border px-4 py-2.5 text-xs font-medium backdrop-blur-xl shadow-elevated transition-all animate-toast-in max-w-sm w-full ${
            borderTones[t.type]
          }`}
          role="alert"
        >
          {icons[t.type]}
          <span className="flex-1 truncate">{t.message}</span>
          <button
            onClick={() => {
              activeToasts = activeToasts.filter((item) => item.id !== t.id)
              notify()
            }}
            className="text-muted-foreground hover:text-foreground p-0.5"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ))}
    </div>
  )
}
