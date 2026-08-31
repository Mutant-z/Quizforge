import client from '@/api/client'
import type { PracticePreview, PracticeSession, PracticeSessionRequest } from '@/types'

export async function previewPractice(config: PracticeSessionRequest) {
  const response = await client.post('/practice/preview', config)
  return response.data.data as PracticePreview
}

export async function createPracticeSession(config: PracticeSessionRequest) {
  const response = await client.post('/practice/sessions', config)
  return response.data.data as PracticeSession
}
