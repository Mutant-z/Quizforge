import axios, { AxiosError } from 'axios'
import type { ApiResponse } from '@/types'

export const API_BASE = '/api/v1'

const client = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
})

let refreshPromise: Promise<string> | null = null

export function refreshAccessToken(): Promise<string> {
  if (refreshPromise) return refreshPromise
  refreshPromise = (async () => {
    const refresh = localStorage.getItem('qt_refresh_token')
    if (!refresh) throw new Error('refresh token is missing')
    const resp = await axios.post(`${API_BASE}/auth/refresh`, { refresh_token: refresh })
    const data = resp.data as ApiResponse<{ access_token: string; refresh_token: string }>
    localStorage.setItem('qt_access_token', data.data.access_token)
    localStorage.setItem('qt_refresh_token', data.data.refresh_token)
    return data.data.access_token
  })().catch((error) => {
    localStorage.removeItem('qt_access_token')
    localStorage.removeItem('qt_refresh_token')
    throw error
  }).finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('qt_access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

client.interceptors.response.use(
  (resp) => resp,
  async (error: AxiosError<ApiResponse<unknown>>) => {
    const original = error.config
    // 401 时尝试用 refresh token 刷新一次
	const retryable = original as (typeof original & { _authRetried?: boolean })
    if (error.response?.status === 401 && original && !original.url?.includes('/auth/') && !retryable._authRetried) {
	  retryable._authRetried = true
	  if (localStorage.getItem('qt_refresh_token')) {
        try {
		  const accessToken = await refreshAccessToken()
          original.headers = original.headers || {}
		  original.headers.Authorization = `Bearer ${accessToken}`
          return client(original)
		} catch { /* refreshAccessToken already clears invalid credentials */ }
      }
    }
    return Promise.reject(error)
  }
)

export function errMsg(e: unknown): string {
  if (axios.isAxiosError(e)) {
    const d = e.response?.data as ApiResponse<unknown> | undefined
    if (d?.error?.message) return d.error.message
    if (d?.error?.detail) return d.error.detail
    return e.message
  }
  return String(e)
}

export default client
