import React from 'react'

export interface SegmentItem<T extends string> {
  key: T
  label: React.ReactNode
  badge?: number | string
  icon?: React.ReactNode
}

export interface MobileSegmentedControlProps<T extends string> {
  items: SegmentItem<T>[]
  active: T
  onChange: (key: T) => void
  className?: string
}

export function MobileSegmentedControl<T extends string>({
  items,
  active,
  onChange,
  className = '',
}: MobileSegmentedControlProps<T>) {
  return (
    <div
      className={`flex items-center p-1 rounded-2xl bg-surface-secondary/90 border border-border/70 shadow-2xs ${className}`}
    >
      {items.map((item) => {
        const isActive = item.key === active
        return (
          <button
            key={item.key}
            type="button"
            onClick={() => onChange(item.key)}
            className={`relative flex-1 flex items-center justify-center gap-1.5 py-1.5 px-2.5 rounded-xl text-xs font-semibold transition-all duration-200 cursor-pointer ${
              isActive
                ? 'bg-surface text-foreground shadow-subtle font-bold scale-[1.01]'
                : 'text-muted-foreground hover:text-foreground hover:bg-surface/40'
            }`}
          >
            {item.icon}
            <span className="truncate">{item.label}</span>
            {item.badge !== undefined && (
              <span
                className={`rounded-full px-1.5 py-0.2 text-[9px] font-mono font-bold leading-none ${
                  isActive ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'
                }`}
              >
                {item.badge}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
