import { create } from 'zustand'

export type ViewMode = 'auto' | 'mobile' | 'desktop'

interface UIState {
  sidebarOpen: boolean
  darkMode: boolean
  aiPanelOpen: boolean
  aiPanelWidth: number
  viewMode: ViewMode
  toggleSidebar: () => void
  toggleDark: () => void
  setAiPanel: (open: boolean) => void
  setAiPanelWidth: (w: number) => void
  setViewMode: (mode: ViewMode) => void
  toggleViewMode: () => void
}

const prefersDark = typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches
const savedAiWidth = typeof window !== 'undefined' ? parseInt(localStorage.getItem('qt_ai_panel_width') || '420', 10) : 420
const savedViewMode = (typeof window !== 'undefined' ? localStorage.getItem('qt_view_mode') : null) as ViewMode | null

export const useUIStore = create<UIState>((set, get) => ({
  sidebarOpen: true,
  darkMode: localStorage.getItem('qt_dark') ? localStorage.getItem('qt_dark') === '1' : prefersDark,
  aiPanelOpen: false,
  aiPanelWidth: Number.isNaN(savedAiWidth) ? 420 : Math.min(Math.max(savedAiWidth, 320), 800),
  viewMode: savedViewMode || 'auto',
  toggleSidebar: () => set({ sidebarOpen: !get().sidebarOpen }),
  toggleDark: () => {
    const next = !get().darkMode
    localStorage.setItem('qt_dark', next ? '1' : '0')
    set({ darkMode: next })
  },
  setAiPanel: (open) => set({ aiPanelOpen: open }),
  setAiPanelWidth: (w) => {
    const clamped = Math.min(Math.max(w, 320), Math.min(typeof window !== 'undefined' ? window.innerWidth * 0.75 : 800, 800))
    localStorage.setItem('qt_ai_panel_width', String(clamped))
    set({ aiPanelWidth: clamped })
  },
  setViewMode: (mode) => {
    localStorage.setItem('qt_view_mode', mode)
    set({ viewMode: mode })
  },
  toggleViewMode: () => {
    const current = get().viewMode
    let next: ViewMode = 'mobile'
    if (current === 'mobile') next = 'desktop'
    else if (current === 'desktop') next = 'mobile'
    else {
      // auto
      const isNarrow = typeof window !== 'undefined' && window.innerWidth < 768
      next = isNarrow ? 'desktop' : 'mobile'
    }
    localStorage.setItem('qt_view_mode', next)
    set({ viewMode: next })
  },
}))
