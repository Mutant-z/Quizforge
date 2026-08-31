import React, { useRef, useState } from 'react'
import { Spinner } from '@/components/ui'

interface MobilePullToRefreshProps {
  onRefresh: () => Promise<void> | void
  children: React.ReactNode
  className?: string
}

export function MobilePullToRefresh({ onRefresh, children, className = '' }: MobilePullToRefreshProps) {
  const [pulling, setPulling] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [pullDistance, setPullDistance] = useState(0)
  const startY = useRef(0)
  const containerRef = useRef<HTMLDivElement>(null)

  const handleTouchStart = (e: React.TouchEvent) => {
    if (containerRef.current && containerRef.current.scrollTop === 0) {
      startY.current = e.touches[0].clientY
      setPulling(true)
    }
  }

  const handleTouchMove = (e: React.TouchEvent) => {
    if (!pulling || refreshing) return
    const currentY = e.touches[0].clientY
    const diff = currentY - startY.current
    if (diff > 0 && containerRef.current && containerRef.current.scrollTop <= 0) {
      // Damping factor
      const distance = Math.min(diff * 0.45, 80)
      setPullDistance(distance)
    }
  }

  const handleTouchEnd = async () => {
    if (!pulling) return
    setPulling(false)
    if (pullDistance > 45 && !refreshing) {
      setRefreshing(true)
      try {
        await onRefresh()
      } finally {
        setRefreshing(false)
        setPullDistance(0)
      }
    } else {
      setPullDistance(0)
    }
  }

  return (
    <div
      ref={containerRef}
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
      className={`relative overflow-y-auto ${className}`}
    >
      {/* Pull Indicator */}
      {(pullDistance > 0 || refreshing) && (
        <div
          style={{ height: `${refreshing ? 40 : pullDistance}px` }}
          className="flex items-center justify-center overflow-hidden transition-all duration-150 text-xs text-muted-foreground"
        >
          {refreshing ? (
            <div className="flex items-center gap-2 font-medium">
              <Spinner className="h-4 w-4" />
              <span>正在刷新...</span>
            </div>
          ) : (
            <span className="text-[11px] font-semibold text-muted-foreground/80">
              {pullDistance > 45 ? '松开立即刷新' : '下拉刷新'}
            </span>
          )}
        </div>
      )}
      {children}
    </div>
  )
}
