import React from 'react'
import { useIsMobile } from '@/hooks/useIsMobile'
import Layout from '@/components/Layout'
import { MobileLayout } from '@/components/mobile/MobileLayout'

export default function ResponsiveLayout() {
  const isMobile = useIsMobile()
  return isMobile ? <MobileLayout /> : <Layout />
}
