import React, { useEffect } from 'react'
import { X } from 'lucide-react'

export interface MobileBottomSheetProps {
  open: boolean
  onClose: () => void
  title?: React.ReactNode
  description?: React.ReactNode
  children: React.ReactNode
  maxHeight?: string
  footer?: React.ReactNode
}

export function MobileBottomSheet({
  open,
  onClose,
  title,
  description,
  children,
  maxHeight = 'max-h-[88vh]',
  footer,
}: MobileBottomSheetProps) {
  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
    return () => {
      document.body.style.overflow = ''
    }
  }, [open])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 backdrop-blur-xs transition-opacity animate-in fade-in-0 duration-200"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Sheet Container */}
      <div
        className={`relative z-10 flex flex-col w-full rounded-t-3xl border-t border-border/80 bg-surface shadow-float ${maxHeight} animate-sheet-up overflow-hidden`}
        role="dialog"
        aria-modal="true"
      >
        {/* Handle Indicator Bar */}
        <div className="flex justify-center pt-2.5 pb-1">
          <div className="h-1.5 w-12 rounded-full bg-muted-foreground/30" />
        </div>

        {/* Sheet Header */}
        {(title || description) && (
          <div className="flex items-start justify-between px-5 py-3 border-b border-border/60">
            <div className="min-w-0 pr-4">
              {title && <h3 className="text-base font-bold tracking-tight text-foreground truncate">{title}</h3>}
              {description && <p className="text-xs text-muted-foreground mt-0.5 leading-relaxed line-clamp-2">{description}</p>}
            </div>
            <button
              onClick={onClose}
              className="flex h-8 w-8 items-center justify-center rounded-full bg-surface-secondary text-muted-foreground hover:text-foreground shrink-0"
              aria-label="关闭"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        )}

        {/* Scrollable Sheet Content */}
        <div className="flex-1 overflow-y-auto px-5 py-4 min-h-0 touch-pan-y">{children}</div>

        {/* Optional Fixed Bottom Footer */}
        {footer && (
          <div className="border-t border-border/60 bg-surface-secondary/40 px-5 py-3.5 pb-[calc(env(safe-area-inset-bottom)+0.75rem)]">
            {footer}
          </div>
        )}
      </div>
    </div>
  )
}
