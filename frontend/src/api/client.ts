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
    const message = d?.error?.message
    const detail = d?.error?.detail
    if (message && detail && detail !== message) return `${message}：${detail}`
    if (message) return message
    if (detail) return detail
    return e.message
  }
  return String(e)
}

/**
 * 用于原生 fetch 流式接口的鉴权请求。
 * axios 拦截器无法覆盖 fetch，因此这里复用同一套 access/refresh token 逻辑。
 */
export async function authFetch(input: Parameters<typeof fetch>[0], init: Parameters<typeof fetch>[1] = {}): Promise<Response> {
  const request = () => {
    const headers = new Headers(init.headers)
    const token = localStorage.getItem('qt_access_token')
    if (token) headers.set('Authorization', `Bearer ${token}`)
    return fetch(input, { ...init, headers })
  }

  let response = await request()
  if (response.status === 401 && localStorage.getItem('qt_refresh_token')) {
    try {
      await refreshAccessToken()
      response = await request()
    } catch {
      // refreshAccessToken 已清理失效凭据，保留原始 401 供调用方展示。
    }
  }
  return response
}

/** 读取统一 API 错误响应，兼容流式接口在建立 SSE 前返回的 JSON 错误。 */
export async function responseErrorMessage(response: Response, fallback = '请求失败'): Promise<string> {
  try {
    const payload = (await response.json()) as ApiResponse<unknown>
    const message = payload.error?.message
    const detail = payload.error?.detail
    if (message && detail && detail !== message) return `${message}：${detail}`
    if (message) return message
    if (detail) return detail
  } catch {
    // Some proxies return an HTML/plain-text error page.
  }
  return `${fallback}（HTTP ${response.status}）`
}

export default client
