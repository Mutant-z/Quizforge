import { create } from 'zustand'
import type { User } from '@/types'

interface AuthState {
  user: User | null
  token: string | null
  setAuth: (token: string, user: User) => void
  setUser: (user: User) => void
  logout: () => void
  isAuthed: () => boolean
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  token: typeof window !== 'undefined' ? localStorage.getItem('qt_access_token') : null,
  setAuth: (token, user) => {
    localStorage.setItem('qt_access_token', token)
    set({ token, user })
  },
  setUser: (user) => set({ user }),
  logout: () => {
    localStorage.removeItem('qt_access_token')
    localStorage.removeItem('qt_refresh_token')
    set({ token: null, user: null })
  },
  isAuthed: () => !!get().token,
}))
