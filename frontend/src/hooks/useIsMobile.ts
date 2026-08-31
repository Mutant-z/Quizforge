import { useEffect, useState } from 'react'
import { useUIStore } from '@/store/ui'

const MOBILE_BREAKPOINT = 768

export function useIsMobile(breakpoint = MOBILE_BREAKPOINT): boolean {
  const viewMode = useUIStore((s) => s.viewMode)
  const [isNarrow, setIsNarrow] = useState<boolean>(() => {
    if (typeof window === 'undefined') return false
    return window.innerWidth < breakpoint
  })

  useEffect(() => {
    if (typeof window === 'undefined') return

    const mql = window.matchMedia(`(max-width: ${breakpoint - 1}px)`)
    const onChange = () => {
      setIsNarrow(window.innerWidth < breakpoint)
    }

    mql.addEventListener('change', onChange)
    window.addEventListener('resize', onChange)
    setIsNarrow(window.innerWidth < breakpoint)

    return () => {
      mql.removeEventListener('change', onChange)
      window.removeEventListener('resize', onChange)
    }
  }, [breakpoint])

  if (viewMode === 'mobile') return true
  if (viewMode === 'desktop') return false
  return isNarrow
}
