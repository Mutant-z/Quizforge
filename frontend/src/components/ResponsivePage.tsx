import React from 'react'
import { useIsMobile } from '@/hooks/useIsMobile'

interface ResponsivePageProps {
  desktop: React.ReactNode
  mobile: React.ReactNode
}

export function ResponsivePage({ desktop, mobile }: ResponsivePageProps) {
  const isMobile = useIsMobile()
  return <>{isMobile ? mobile : desktop}</>
}
