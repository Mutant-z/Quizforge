import client, { API_BASE, errMsg } from '@/api/client'
import type {
  WrongImportDraft,
  WrongImportImageBatch,
  WrongImportMergeCandidate,
  WrongImportMessage,
  WrongImportSession,
  WrongImportSessionDetail,
} from '@/types/wrongImport'

function authHeaders(): Record<string, string> {
  return { Authorization: `Bearer ${localStorage.getItem('qt_access_token')}` }
}

/** 后端 omitempty 会省略空数组字段，统一归一化保证前端可安全访问。 */
function normalizeDraft(d: WrongImportDraft): WrongImportDraft {
  return {
    ...d,
    options: d.options ?? [],
    answer: d.answer ?? [],
    answer_status: d.answer_status ?? ((d.answer ?? []).length > 0 ? 'provided' : d.answer_source === 'none' ? 'missing' : 'pending'),
    user_marked_no_answer: d.user_marked_no_answer ?? false,
    warnings: d.warnings ?? [],
    knowledge_points: d.knowledge_points ?? [],
    sources: d.sources ?? [],
    merge_status: d.merge_status ?? 'active',
    content_version: d.content_version ?? 1,
  }
}

export const wrongImportApi = {
  async listSessions(): Promise<WrongImportSession[]> {
    const resp = await client.get('/wrong-import/sessions')
    return (resp.data.data?.items ?? []) as WrongImportSession[]
  },

  async createSession(title?: string): Promise<WrongImportSession> {
    const resp = await client.post('/wrong-import/sessions', title ? { title } : {})
    return resp.data.data as WrongImportSession
  },

  async getSession(id: number): Promise<WrongImportSessionDetail> {
    const resp = await client.get(`/wrong-import/sessions/${id}`)
    const data = resp.data.data as WrongImportSessionDetail
    return {
      ...data,
      images: data.images ?? [],
      batches: data.batches ?? [],
      answer_fragments: data.answer_fragments ?? [],
      drafts: (data.drafts ?? []).map(normalizeDraft),
      messages: data.messages ?? [],
    }
  },

  async discardSession(id: number): Promise<void> {
    await client.delete(`/wrong-import/sessions/${id}`)
  },

  async listDrafts(sessionId: number): Promise<WrongImportDraft[]> {
    const resp = await client.get(`/wrong-import/sessions/${sessionId}/drafts`)
    return ((resp.data.data?.items ?? []) as WrongImportDraft[]).map(normalizeDraft)
  },

  async patchDraft(
    sessionId: number,
    draftId: number,
    patch: Record<string, unknown>,
  ): Promise<WrongImportDraft> {
    const resp = await client.patch(`/wrong-import/sessions/${sessionId}/drafts/${draftId}`, patch)
    return normalizeDraft(resp.data.data as WrongImportDraft)
  },

  async deleteDraft(sessionId: number, draftId: number): Promise<void> {
    await client.delete(`/wrong-import/sessions/${sessionId}/drafts/${draftId}`)
  },

  async listMessages(sessionId: number): Promise<WrongImportMessage[]> {
    const resp = await client.get(`/wrong-import/sessions/${sessionId}/messages`)
    return (resp.data.data?.items ?? []) as WrongImportMessage[]
  },

  async sendMessage(sessionId: number, content: string): Promise<WrongImportMessage> {
    const resp = await client.post(`/wrong-import/sessions/${sessionId}/messages`, { content })
    return resp.data.data as WrongImportMessage
  },

  async confirm(
    sessionId: number,
    draftIds: number[],
    targetBankId?: number,
  ): Promise<{ committed_count: number; linked_count: number; created_count: number; bank_id: number }> {
    const resp = await client.post(`/wrong-import/sessions/${sessionId}/confirm`, {
      draft_ids: draftIds,
      target_bank_id: targetBankId,
    })
    return resp.data.data
  },

  async retryExtract(sessionId: number, imageId: number): Promise<void> {
    await client.post(`/wrong-import/sessions/${sessionId}/images/${imageId}/extract`)
  },

  async finalizeBatch(sessionId: number, batchId: string, expectedCount: number): Promise<WrongImportImageBatch> {
    const resp = await client.post(`/wrong-import/sessions/${sessionId}/batches/${batchId}/finalize`, {
      expected_count: expectedCount,
    })
    return resp.data.data.batch as WrongImportImageBatch
  },

  async reconcile(sessionId: number, scope: 'batch' | 'legacy_session', batchId?: string): Promise<void> {
    await client.post(`/wrong-import/sessions/${sessionId}/reconcile`, { scope, batch_id: batchId })
  },

  async retryAnswerExtraction(sessionId: number): Promise<number> {
    const resp = await client.post(`/wrong-import/sessions/${sessionId}/answer-reconcile`)
    return Number(resp.data.data?.queued ?? 0)
  },

  async listMergeCandidates(sessionId: number, status = 'pending'): Promise<WrongImportMergeCandidate[]> {
    const resp = await client.get(`/wrong-import/sessions/${sessionId}/merge-candidates`, { params: { status } })
    return (resp.data.data?.items ?? []) as WrongImportMergeCandidate[]
  },

  async resolveMergeCandidate(sessionId: number, candidateId: number, action: 'accept' | 'reject') {
    const resp = await client.post(
      `/wrong-import/sessions/${sessionId}/merge-candidates/${candidateId}/resolve`,
      { action },
    )
    return resp.data.data
  },

  async undoMerge(sessionId: number, mergeId: number): Promise<void> {
    await client.post(`/wrong-import/sessions/${sessionId}/merges/${mergeId}/undo`)
  },

  imageFileUrl(sessionId: number, imageId: number, crop?: string): string {
    const query = crop ? `?crop=${crop}` : ''
    return `${API_BASE}/wrong-import/sessions/${sessionId}/images/${imageId}/file${query}`
  },
}

/** 分片上传一张图片（与题库导入相同的 init → chunks → complete 协议）。 */
export async function uploadWrongImportImage(
  sessionId: number,
  file: File,
  onProgress?: (sent: number, total: number) => void,
  batch?: { id: string; index: number; size: number; extractionMode?: 'questions' | 'answer_key' | 'auto'; instruction?: string },
): Promise<void> {
  const initResp = await client.post(`/wrong-import/sessions/${sessionId}/images/init`, {
    filename: file.name,
    size: file.size,
    mime_type: file.type,
    ...(batch
      ? {
          batch_id: batch.id,
          batch_index: batch.index,
          batch_size: batch.size,
          extraction_mode: batch.extractionMode ?? 'questions',
        }
      : {}),
  })
  const { upload_id: uploadId, chunk_size: chunkSize, chunk_count: chunkCount } = initResp.data.data

  for (let index = 0; index < chunkCount; index++) {
    const start = index * chunkSize
    const blob = file.slice(start, Math.min(start + chunkSize, file.size))
    const form = new FormData()
    form.append('chunk', blob, file.name)
    await client.post(
      `/wrong-import/sessions/${sessionId}/images/${uploadId}/chunks/${index}`,
      form,
      { headers: { 'Content-Type': 'multipart/form-data' } },
    )
    onProgress?.(index + 1, chunkCount)
  }

  await client.post(`/wrong-import/sessions/${sessionId}/images/${uploadId}/complete`, {
    filename: file.name,
    mime_type: file.type || guessImageMime(file.name),
    total_size: file.size,
    chunk_count: chunkCount,
    ...(batch
      ? {
          batch_id: batch.id,
          batch_index: batch.index,
          batch_size: batch.size,
          extraction_mode: batch.extractionMode ?? 'questions',
          ...(batch.instruction ? { instruction: batch.instruction } : {}),
        }
      : {}),
  })
}

function guessImageMime(name: string): string {
  const ext = name.toLowerCase().split('.').pop()
  if (ext === 'png') return 'image/png'
  return 'image/jpeg'
}

export { errMsg, authHeaders }
