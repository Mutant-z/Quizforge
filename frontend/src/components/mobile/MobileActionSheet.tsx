import React, { useEffect } from 'react'

export interface ActionItem {
  key: string
  label: React.ReactNode
  icon?: React.ReactNode
  danger?: boolean
  onClick: () => void
}

export interface MobileActionSheetProps {
  open: boolean
  onClose: () => void
  title?: React.ReactNode
  actions: ActionItem[]
  cancelText?: string
}

export function MobileActionSheet({
  open,
  onClose,
  title,
  actions,
  cancelText = '取消',
}: MobileActionSheetProps) {
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
      <div
        className="fixed inset-0 bg-black/50 backdrop-blur-xs transition-opacity animate-in fade-in-0 duration-200"
        onClick={onClose}
      />
      <div className="relative z-10 w-full px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] space-y-2 animate-sheet-up">
        {/* Actions Card */}
        <div className="overflow-hidden rounded-3xl border border-border/80 bg-surface/95 backdrop-blur-xl shadow-float">
          {title && (
            <div className="px-4 py-3 text-center text-xs font-semibold text-muted-foreground border-b border-border/60">
              {title}
            </div>
          )}
          <div className="divide-y divide-border/60">
            {actions.map((action) => (
              <button
                key={action.key}
                type="button"
                onClick={() => {
                  action.onClick()
                  onClose()
                }}
                className={`flex w-full items-center justify-center gap-2.5 px-4 py-3.5 text-sm font-semibold transition-colors active:bg-surface-secondary/80 ${
                  action.danger ? 'text-destructive' : 'text-foreground'
                }`}
              >
                {action.icon}
                <span>{action.label}</span>
              </button>
            ))}
          </div>
        </div>

        {/* Cancel Button */}
        <button
          type="button"
          onClick={onClose}
          className="flex w-full items-center justify-center rounded-2xl border border-border/80 bg-surface px-4 py-3 text-sm font-bold text-foreground active:bg-surface-secondary shadow-card"
        >
          {cancelText}
        </button>
      </div>
    </div>
  )
}
