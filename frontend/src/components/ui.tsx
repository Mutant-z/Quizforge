import React, { useEffect } from 'react'
import { AlertCircle, ChevronRight, Inbox, RefreshCw, X } from 'lucide-react'

// ==========================================
// 1. Buttons & Action Elements
// ==========================================

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'outline' | 'ghost' | 'subtle' | 'danger'
  size?: 'xs' | 'sm' | 'md' | 'lg'
  loading?: boolean
  icon?: React.ReactNode
  shortcut?: string
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  icon,
  shortcut,
  className = '',
  disabled,
  children,
  ...props
}: ButtonProps) {
  const variantStyles = {
    primary: 'btn-primary',
    secondary: 'btn-secondary',
    outline: 'btn-outline',
    ghost: 'btn-ghost',
    subtle: 'btn-subtle',
    danger: 'btn-danger',
  }

  const sizeStyles = {
    xs: 'h-7 px-2.5 text-xs rounded-lg gap-1.5',
    sm: 'h-8 px-3 text-xs rounded-xl gap-1.5 font-medium',
    md: 'h-9 px-3.5 text-sm rounded-xl gap-2 font-medium',
    lg: 'h-11 px-5 text-base rounded-2xl gap-2.5 font-semibold',
  }

  return (
    <button
      className={`btn ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <Spinner className="h-3.5 w-3.5" /> : icon}
      {children}
      {shortcut && (
        <kbd className="ml-1 hidden sm:inline-block rounded-md bg-background/25 px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider text-inherit opacity-80 border border-white/10">
          {shortcut}
        </kbd>
      )}
    </button>
  )
}

export function IconButton({
  variant = 'ghost',
  size = 'md',
  loading = false,
  className = '',
  children,
  title,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'outline' | 'ghost' | 'subtle' | 'danger'
  size?: 'xs' | 'sm' | 'md' | 'lg'
  loading?: boolean
  title?: string
}) {
  const sizeStyles = {
    xs: 'h-7 w-7 rounded-lg',
    sm: 'h-8 w-8 rounded-xl',
    md: 'h-9 w-9 rounded-xl',
    lg: 'h-11 w-11 rounded-2xl',
  }
  const variantStyles = {
    primary: 'btn-primary',
    secondary: 'btn-secondary',
    outline: 'btn-outline',
    ghost: 'btn-ghost',
    subtle: 'btn-subtle',
    danger: 'btn-danger',
  }

  return (
    <button
      className={`btn p-0 shrink-0 ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      disabled={props.disabled || loading}
      title={title}
      aria-label={title}
      {...props}
    >
      {loading ? <Spinner className="h-3.5 w-3.5" /> : children}
    </button>
  )
}

// ==========================================
// 2. Cards & Containers
// ==========================================

export function Card({
  className = '',
  elevated = false,
  interactive = false,
  glass = false,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & { elevated?: boolean; interactive?: boolean; glass?: boolean }) {
  let cardStyle = 'card'
  if (glass) cardStyle = 'glass-card'
  else if (interactive) cardStyle = 'card-interactive'
  else if (elevated) cardStyle = 'card-elevated'

  return (
    <div className={`${cardStyle} ${className}`} {...props}>
      {children}
    </div>
  )
}

export function CardHeader({
  title,
  desc,
  action,
  icon,
  className = '',
}: {
  title: React.ReactNode
  desc?: React.ReactNode
  action?: React.ReactNode
  icon?: React.ReactNode
  className?: string
}) {
  return (
    <div className={`flex items-start justify-between p-5 pb-3.5 ${className}`}>
      <div className="flex items-start gap-3 min-w-0">
        {icon && (
          <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-subtle">
            {icon}
          </div>
        )}
        <div className="min-w-0">
          <h3 className="text-sm font-bold tracking-tight text-foreground truncate">{title}</h3>
          {desc && <p className="mt-0.5 text-xs text-muted-foreground leading-relaxed">{desc}</p>}
        </div>
      </div>
      {action && <div className="ml-4 shrink-0">{action}</div>}
    </div>
  )
}

export function CardContent({ className = '', children }: { className?: string; children: React.ReactNode }) {
  return <div className={`p-5 pt-0 ${className}`}>{children}</div>
}

// ==========================================
// 3. Headers & Layout Sections
// ==========================================

export function PageHeader({
  title,
  subtitle,
  tags,
  action,
  onBack,
  className = '',
}: {
  title: React.ReactNode
  subtitle?: React.ReactNode
  tags?: React.ReactNode
  action?: React.ReactNode
  onBack?: () => void
  className?: string
}) {
  return (
    <div className={`flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-border/60 ${className}`}>
      <div className="space-y-1 min-w-0">
        <div className="flex items-center gap-2.5 flex-wrap">
          {onBack && (
            <button
              onClick={onBack}
              className="mr-1 inline-flex items-center justify-center rounded-xl p-1.5 text-muted-foreground hover:bg-surface-secondary hover:text-foreground transition-colors shadow-subtle border border-border/60"
              title="返回"
            >
              <ChevronRight className="h-4 w-4 rotate-180" />
            </button>
          )}
          <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">{title}</h1>
          {tags}
        </div>
        {subtitle && <p className="text-xs text-muted-foreground leading-relaxed max-w-3xl">{subtitle}</p>}
      </div>
      {action && <div className="flex items-center gap-2.5 shrink-0 flex-wrap">{action}</div>}
    </div>
  )
}

export function SectionHeader({
  title,
  badge,
  action,
  className = '',
}: {
  title: string
  badge?: React.ReactNode
  action?: React.ReactNode
  className?: string
}) {
  return (
    <div className={`flex items-center justify-between gap-2 ${className}`}>
      <div className="flex items-center gap-2">
        <h2 className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{title}</h2>
        {badge}
      </div>
      {action}
    </div>
  )
}

// ==========================================
// 4. Badges & Indicators
// ==========================================

export function Badge({
  children,
  tone = 'neutral',
  variant = 'subtle',
  dot = false,
  className = '',
}: {
  children: React.ReactNode
  tone?: 'neutral' | 'primary' | 'success' | 'danger' | 'warning' | 'info'
  variant?: 'subtle' | 'outline' | 'solid'
  dot?: boolean
  className?: string
}) {
  const tones = {
    neutral: {
      subtle: 'bg-muted/70 text-muted-foreground border border-border/70',
      outline: 'border border-border text-muted-foreground bg-transparent',
      solid: 'bg-muted-foreground text-background',
      dotColor: 'bg-muted-foreground',
    },
    primary: {
      subtle: 'bg-primary/10 text-primary border border-primary/25',
      outline: 'border border-primary/40 text-primary bg-transparent',
      solid: 'bg-primary text-primary-foreground',
      dotColor: 'bg-primary',
    },
    success: {
      subtle: 'bg-success/10 text-success border border-success/25',
      outline: 'border border-success/40 text-success bg-transparent',
      solid: 'bg-success text-success-foreground',
      dotColor: 'bg-success',
    },
    danger: {
      subtle: 'bg-destructive/10 text-destructive border border-destructive/25',
      outline: 'border border-destructive/40 text-destructive bg-transparent',
      solid: 'bg-destructive text-destructive-foreground',
      dotColor: 'bg-destructive',
    },
    warning: {
      subtle: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/25',
      outline: 'border border-amber-500/40 text-amber-600 dark:text-amber-400 bg-transparent',
      solid: 'bg-amber-500 text-white',
      dotColor: 'bg-amber-500',
    },
    info: {
      subtle: 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border border-sky-500/25',
      outline: 'border border-sky-500/40 text-sky-600 dark:text-sky-400 bg-transparent',
      solid: 'bg-sky-500 text-white',
      dotColor: 'bg-sky-500',
    },
  }

  return (
    <span className={`badge ${tones[tone][variant]} ${className}`}>
      {dot && <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${tones[tone].dotColor} animate-pulse-subtle`} />}
      {children}
    </span>
  )
}

export function MasteryIndicator({ score, size = 'sm' }: { score: number; size?: 'sm' | 'md' }) {
  const percentage = Math.min(100, Math.max(0, Math.round(score)))
  const color =
    percentage >= 80
      ? 'bg-gradient-to-r from-emerald-500 to-emerald-400'
      : percentage >= 50
        ? 'bg-gradient-to-r from-amber-500 to-amber-400'
        : percentage > 0
          ? 'bg-gradient-to-r from-rose-500 to-rose-400'
          : 'bg-muted'
  const dotColor =
    percentage >= 80 ? 'bg-emerald-500' : percentage >= 50 ? 'bg-amber-500' : percentage > 0 ? 'bg-rose-500' : 'bg-muted'

  if (size === 'md') {
    return (
      <div className="flex items-center gap-2.5" title={`掌握度评分: ${percentage}%`}>
        <div className="h-2 w-20 sm:w-28 overflow-hidden rounded-full bg-muted/70 p-0.5 border border-border/40">
          <div
            className={`h-full rounded-full transition-all duration-700 ease-out ${color}`}
            style={{ width: `${percentage}%` }}
          />
        </div>
        <span className="font-mono text-xs text-foreground/80 font-bold">{percentage}%</span>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-1.5" title={`掌握度评分: ${percentage}%`}>
      <div className={`h-2 w-2 rounded-full ${dotColor} shadow-2xs`} />
      <span className="font-mono text-xs text-muted-foreground font-semibold">{percentage}%</span>
    </div>
  )
}

export function StatCard({
  icon: Icon,
  label,
  value,
  unit,
  sub,
  warn,
  accent = 'primary',
  className = '',
  onClick,
}: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: React.ReactNode
  unit?: string
  sub?: React.ReactNode
  warn?: boolean
  accent?: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  className?: string
  onClick?: () => void
}) {
  const accentColors = {
    primary: 'bg-primary/10 text-primary border-primary/20 shadow-glow',
    success: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20 shadow-glow-success',
    warning: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 shadow-glow-amber',
    danger: 'bg-destructive/10 text-destructive border-destructive/20 shadow-glow-danger',
    info: 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border-sky-500/20',
  }

  return (
    <div
      onClick={onClick}
      className={`group relative overflow-hidden rounded-2xl border bg-surface p-3.5 sm:p-4 shadow-card transition-all duration-200 hover:shadow-elevated hover:-translate-y-0.5 ${
        warn ? 'border-amber-500/40 bg-amber-500/[0.03]' : 'border-border/80'
      } ${onClick ? 'cursor-pointer hover:border-primary/40' : ''} ${className}`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground truncate">
          {label}
        </span>
        <div
          className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border transition-transform group-hover:scale-105 ${accentColors[accent]}`}
        >
          <Icon className="h-3.5 w-3.5" />
        </div>
      </div>

      <div className="mt-2 flex items-baseline gap-1">
        <span className="text-xl sm:text-2xl font-bold tracking-tight text-foreground font-mono">
          {value}
        </span>
        {unit && <span className="text-xs font-semibold text-muted-foreground">{unit}</span>}
      </div>

      {sub && <p className="mt-0.5 text-[11px] sm:text-xs text-muted-foreground truncate leading-relaxed">{sub}</p>}
    </div>
  )
}

// ==========================================
// 5. Feedback States (Loading, Empty, Error)
// ==========================================

export function Spinner({ className = '' }: { className?: string }) {
  return (
    <div
      className={`inline-block animate-spin rounded-full border-2 border-border/80 border-t-primary ${className}`}
      role="status"
      aria-label="加载中"
    />
  )
}

export function Progress({
  value,
  max = 1,
  className = '',
  indicatorClassName = 'bg-gradient-to-r from-primary to-primary-hover',
}: {
  value: number
  max?: number
  className?: string
  indicatorClassName?: string
}) {
  const percentage = Math.min(100, Math.max(0, (value / max) * 100))
  return (
    <div className={`h-1.5 w-full overflow-hidden rounded-full bg-muted/80 ${className}`}>
      <div
        className={`h-full rounded-full transition-all duration-300 ease-out ${indicatorClassName}`}
        style={{ width: `${percentage}%` }}
      />
    </div>
  )
}

export function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`rounded-2xl skeleton-shimmer ${className}`} />
}

export function EmptyState({
  icon: Icon = Inbox,
  title = '暂无数据',
  description,
  action,
  className = '',
}: {
  icon?: React.ComponentType<{ className?: string }>
  title?: string
  description?: string
  action?: React.ReactNode
  className?: string
}) {
  return (
    <div className={`flex flex-col items-center justify-center py-12 px-4 text-center ${className}`}>
      <div className="flex h-14 w-14 items-center justify-center rounded-3xl bg-surface-secondary/80 border border-border/80 text-muted-foreground/70 shadow-subtle">
        <Icon className="h-6 w-6" />
      </div>
      <h3 className="mt-3.5 text-sm font-bold tracking-tight text-foreground">{title}</h3>
      {description && <p className="mt-1 text-xs text-muted-foreground max-w-sm leading-relaxed">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function ErrorState({
  title = '发生错误',
  message = '加载数据时遇到问题，请检查网络连接后重试。',
  onRetry,
  className = '',
}: {
  title?: string
  message?: string
  onRetry?: () => void
  className?: string
}) {
  return (
    <div className={`flex flex-col items-center justify-center py-10 px-4 text-center rounded-3xl border border-destructive/25 bg-destructive/5 ${className}`}>
      <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-destructive/10 text-destructive">
        <AlertCircle className="h-5 w-5" />
      </div>
      <h3 className="mt-3 text-sm font-bold tracking-tight text-foreground">{title}</h3>
      <p className="mt-1 text-xs text-muted-foreground max-w-sm leading-relaxed">{message}</p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry} className="mt-4" icon={<RefreshCw className="h-3.5 w-3.5" />}>
          重试
        </Button>
      )}
    </div>
  )
}

// ==========================================
// 6. Modal / Dialog
// ==========================================

export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  width = 'max-w-lg',
}: {
  open: boolean
  onClose: () => void
  title: React.ReactNode
  description?: React.ReactNode
  children: React.ReactNode
  width?: string
}) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 overflow-y-auto">
      {/* Backdrop with modern blur */}
      <div
        className="fixed inset-0 bg-black/40 dark:bg-black/65 backdrop-blur-xs transition-opacity animate-in fade-in-0 duration-200"
        onClick={onClose}
        aria-hidden="true"
      />
      {/* Dialog container with scale-in animation */}
      <div
        className={`relative w-full ${width} rounded-3xl border border-border/80 bg-surface shadow-float transition-all my-6 z-10 animate-scale-in flex flex-col max-h-[calc(100vh-3rem)] overflow-hidden`}
        role="dialog"
        aria-modal="true"
      >
        <div className="flex items-start justify-between border-b border-border/60 p-4 sm:p-5 pb-3.5 bg-surface/80 shrink-0">
          <div className="space-y-0.5 min-w-0 pr-3">
            <h3 className="text-sm sm:text-base font-bold tracking-tight text-foreground truncate">{title}</h3>
            {description && <p className="text-xs text-muted-foreground leading-relaxed truncate">{description}</p>}
          </div>
          <IconButton
            variant="ghost"
            size="xs"
            onClick={onClose}
            aria-label="关闭"
            className="text-muted-foreground hover:text-foreground shrink-0 rounded-lg hover:bg-surface-secondary"
          >
            <X className="h-4 w-4" />
          </IconButton>
        </div>
        <div className="p-4 sm:p-5 overflow-y-auto">{children}</div>
      </div>
    </div>
  )
}

// ==========================================
// 7. Tabs & Segmented Controls
// ==========================================

export function SegmentedControl<T extends string | number | null>({
  options,
  value,
  onChange,
  className = '',
}: {
  options: { value: T; label: React.ReactNode; icon?: React.ReactNode }[]
  value: T
  onChange: (value: T) => void
  className?: string
}) {
  return (
    <div className={`inline-flex items-center gap-1 rounded-2xl bg-surface-secondary/80 p-1 border border-border/70 ${className}`}>
      {options.map((option, idx) => {
        const isSelected = value === option.value
        return (
          <button
            key={idx}
            type="button"
            onClick={() => onChange(option.value)}
            className={`inline-flex items-center justify-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-semibold transition-all duration-150 active:scale-[0.98] ${
              isSelected
                ? 'bg-surface text-primary shadow-subtle border border-border/60 font-bold ring-1 ring-primary/20'
                : 'text-muted-foreground hover:text-foreground hover:bg-surface/50 border border-transparent'
            }`}
          >
            {option.icon}
            <span>{option.label}</span>
          </button>
        )
      })}
    </div>
  )
}

export function Tabs<T extends string>({
  tabs,
  active,
  onChange,
  className = '',
}: {
  tabs: { key: T; label: React.ReactNode; count?: number; icon?: React.ReactNode }[]
  active: T
  onChange: (key: T) => void
  className?: string
}) {
  return (
    <div className={`flex items-center gap-1 border-b border-border/80 ${className}`}>
      {tabs.map((tab) => {
        const isActive = active === tab.key
        return (
          <button
            key={tab.key}
            onClick={() => onChange(tab.key)}
            className={`group relative flex items-center gap-2 px-3.5 py-2.5 text-xs font-medium transition-all ${
              isActive
                ? 'text-foreground font-bold'
                : 'text-muted-foreground hover:text-foreground hover:bg-surface-secondary/60 rounded-t-xl'
            }`}
          >
            {tab.icon}
            <span>{tab.label}</span>
            {tab.count !== undefined && (
              <span
                className={`rounded-full px-1.5 py-0.2 text-[10px] font-mono font-semibold ${
                  isActive ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'
                }`}
              >
                {tab.count}
              </span>
            )}
            {isActive && (
              <span className="absolute inset-x-0 bottom-0 h-0.5 bg-primary rounded-full transition-all shadow-glow-sm" />
            )}
          </button>
        )
      })}
    </div>
  )
}

// ==========================================
// 8. Shortcuts & Tooltips
// ==========================================

export function ShortcutKey({ keys }: { keys: string[] }) {
  return (
    <div className="inline-flex items-center gap-1">
      {keys.map((k, i) => (
        <kbd
          key={i}
          className="inline-flex h-5 items-center justify-center rounded-lg border border-border/80 bg-surface-secondary px-1.5 font-mono text-[10px] font-semibold text-muted-foreground shadow-subtle"
        >
          {k}
        </kbd>
      ))}
    </div>
  )
}

