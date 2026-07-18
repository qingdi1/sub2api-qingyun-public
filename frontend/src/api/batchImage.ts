import { buildGatewayUrl } from './client'
import { isDemoSession } from './demo'

export type BatchImageStatus =
  | 'queued'
  | 'running'
  | 'indexing'
  | 'processing_results'
  | 'settling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'output_deleted'
  | string

export interface BatchImageSubmitItem {
  custom_id: string
  prompt: string
  output_count?: number
  reference_images?: BatchImageReferenceImage[]
}

export interface BatchImageReferenceImage {
  id?: string
  type?: string
  mime_type: string
  data?: string
  file_uri?: string
}

export interface BatchImageSubmitRequest {
  model: string
  task_name?: string
  parent_batch_id?: string
  provider?: '' | 'gemini_api' | 'vertex' | string
  image_size?: '1K' | '2K' | '4K' | string
  response_mime_type?: string
  aspect_ratio?: string
  items: BatchImageSubmitItem[]
  metadata?: Record<string, string>
}

export interface BatchImageJob {
  id: string
  object: string
  task_name: string
  parent_batch_id?: string | null
  status: BatchImageStatus
  model: string
  provider: string
  item_count: number
  success_count: number
  fail_count: number
  estimated_cost: number
  hold_amount: number
  actual_cost: number | null
  created_at: number
  submitted_at: number | null
  settled_at: number | null
  downloaded_at?: number | null
  output_deleted_at?: number | null
}

export interface BatchImageItem {
  batch_id?: string
  source_task_name?: string
  custom_id: string
  status: string
  prompt_preview?: string | null
  mime_type: string | null
  file_extension: string | null
  image_count: number
  error?: {
    code: string
    message: string
    source?: 'provider' | 'system' | string
  } | null
}

export interface BatchImageItemsResponse {
  object: string
  data: BatchImageItem[]
  has_more: boolean
}

export interface BatchImageJobsResponse {
  object: string
  data: BatchImageJob[]
  has_more: boolean
}

export interface BatchImageModel {
  id: string
  object: string
  provider: string
}

export interface BatchImageModelsResponse {
  object: string
  data: BatchImageModel[]
}

export interface BatchImageJobsListOptions {
  limit?: number
  cursor?: string
  status?: string
  taskName?: string
  downloaded?: '' | 'true' | 'false' | string
  from?: string
  to?: string
}

interface DemoBatchRecord {
  job: BatchImageJob
  items: BatchImageItem[]
}

// Batch-image calls target the gateway directly instead of the Axios client.
// Keep their demo equivalent entirely in module memory so an active demo
// session cannot submit a key or request to a real gateway.
const DEMO_BATCH_CREATED_AT = Math.floor(Date.now() / 1000) - 15 * 60
let demoBatchSequence = 1
let demoBatchRecords: DemoBatchRecord[] = [{
  job: {
    id: 'demo-batch-sample',
    object: 'batch',
    task_name: '演示图像任务',
    parent_batch_id: null,
    status: 'completed',
    model: 'gemini-3-pro-image-preview',
    provider: 'gemini_api',
    item_count: 2,
    success_count: 2,
    fail_count: 0,
    estimated_cost: 0.04,
    hold_amount: 0.04,
    actual_cost: 0.04,
    created_at: DEMO_BATCH_CREATED_AT,
    submitted_at: DEMO_BATCH_CREATED_AT,
    settled_at: DEMO_BATCH_CREATED_AT + 3,
  },
  items: [
    {
      batch_id: 'demo-batch-sample',
      custom_id: 'demo-sunrise',
      status: 'succeeded',
      prompt_preview: '演示：夏日海边日出',
      mime_type: 'image/png',
      file_extension: 'png',
      image_count: 1,
    },
    {
      batch_id: 'demo-batch-sample',
      custom_id: 'demo-lake',
      status: 'succeeded',
      prompt_preview: '演示：水墨湖畔',
      mime_type: 'image/png',
      file_extension: 'png',
      image_count: 1,
    },
  ],
}]

function cloneDemoJob(job: BatchImageJob): BatchImageJob {
  return { ...job }
}

function cloneDemoItem(item: BatchImageItem): BatchImageItem {
  return {
    ...item,
    error: item.error ? { ...item.error } : item.error,
  }
}

function demoBatchNotFoundError(batchId: string): Error {
  const error = new Error(`Demo batch image job not found: ${batchId}`) as Error & { code?: string }
  error.code = 'DEMO_BATCH_NOT_FOUND'
  return error
}

function demoBatchRecord(batchId: string): DemoBatchRecord {
  const record = demoBatchRecords.find((item) => item.job.id === batchId)
  if (!record) throw demoBatchNotFoundError(batchId)
  return record
}

function demoUnixTime(): number {
  return Math.floor(Date.now() / 1000)
}

function demoImageSvg(label: string): Blob {
  const escapedLabel = label.replace(/[&<>"']/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[character] || character))
  const content = `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="768" viewBox="0 0 1024 768"><rect width="1024" height="768" fill="#0f766e"/><rect x="48" y="48" width="928" height="672" rx="32" fill="#ecfeff"/><circle cx="792" cy="214" r="88" fill="#fbbf24"/><path d="M0 612 238 394l176 150 150-113 460 181v156H0z" fill="#14b8a6"/><path d="M0 666 280 510l202 156 196-110 346 130v82H0z" fill="#0d9488"/><text x="512" y="104" text-anchor="middle" font-family="sans-serif" font-size="28" fill="#134e4a">本地演示图像</text><text x="512" y="704" text-anchor="middle" font-family="sans-serif" font-size="24" fill="#134e4a">${escapedLabel}</text></svg>`
  return new Blob([content], { type: 'image/svg+xml' })
}

function submitDemoBatchImageJob(payload: BatchImageSubmitRequest): BatchImageJob {
  const now = demoUnixTime()
  const id = `demo-batch-${now}-${demoBatchSequence++}`
  const items = (payload.items || []).map((item, index): BatchImageItem => ({
    batch_id: id,
    custom_id: item.custom_id || `demo-item-${index + 1}`,
    status: 'succeeded',
    prompt_preview: item.prompt || '本地演示图像',
    mime_type: payload.response_mime_type || 'image/png',
    file_extension: 'png',
    image_count: Math.max(1, Math.min(4, Number(item.output_count) || 1)),
  }))
  const itemCount = items.length
  const actualCost = Number((itemCount * 0.02).toFixed(4))
  const job: BatchImageJob = {
    id,
    object: 'batch',
    task_name: payload.task_name || '本地演示图像任务',
    parent_batch_id: payload.parent_batch_id || null,
    status: 'completed',
    model: payload.model || 'gemini-3-pro-image-preview',
    provider: payload.provider || 'gemini_api',
    item_count: itemCount,
    success_count: itemCount,
    fail_count: 0,
    estimated_cost: actualCost,
    hold_amount: actualCost,
    actual_cost: actualCost,
    created_at: now,
    submitted_at: now,
    settled_at: now,
  }
  demoBatchRecords = [{ job, items }, ...demoBatchRecords]
  return cloneDemoJob(job)
}

function listDemoBatchImageJobs(options: number | BatchImageJobsListOptions): BatchImageJobsResponse {
  const normalized = typeof options === 'number' ? { limit: options } : options
  const limit = Math.max(1, Number(normalized.limit) || 20)
  let records = [...demoBatchRecords]
  if (normalized.status) records = records.filter((record) => record.job.status === normalized.status)
  if (normalized.taskName) records = records.filter((record) => record.job.task_name.includes(normalized.taskName!))
  if (normalized.downloaded === 'true') records = records.filter((record) => Boolean(record.job.downloaded_at))
  if (normalized.downloaded === 'false') records = records.filter((record) => !record.job.downloaded_at)
  return {
    object: 'list',
    data: records.slice(0, limit).map((record) => cloneDemoJob(record.job)),
    has_more: records.length > limit,
  }
}

async function parseBatchImageError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    const message = body?.error?.message || body?.message || response.statusText
    const error = new Error(message)
    ;(error as any).code = body?.error?.code || response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`)
    ;(error as any).code = response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

export async function submitBatchImageJob(
  apiKey: string,
  payload: BatchImageSubmitRequest,
  idempotencyKey: string,
): Promise<BatchImageJob> {
  if (isDemoSession()) return submitDemoBatchImageJob(payload)
  const response = await fetch(buildGatewayUrl('/v1/images/batches'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function getBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  if (isDemoSession()) return cloneDemoJob(demoBatchRecord(batchId).job)
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageJobs(apiKey: string, options: number | BatchImageJobsListOptions = 20): Promise<BatchImageJobsResponse> {
  if (isDemoSession()) return listDemoBatchImageJobs(options)
  const params = new URLSearchParams()
  if (typeof options === 'number') {
    params.set('limit', String(options))
  } else {
    params.set('limit', String(options.limit || 20))
    if (options.cursor) params.set('cursor', options.cursor)
    if (options.status) params.set('status', options.status)
    if (options.taskName) params.set('task_name', options.taskName)
    if (options.downloaded) params.set('downloaded', options.downloaded)
    if (options.from) params.set('from', options.from)
    if (options.to) params.set('to', options.to)
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches?${params.toString()}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageModels(apiKey: string): Promise<BatchImageModelsResponse> {
  if (isDemoSession()) {
    return {
      object: 'list',
      data: [{ id: 'gemini-3-pro-image-preview', object: 'model', provider: 'gemini_api' }],
    }
  }
  const response = await fetch(buildGatewayUrl('/v1/images/batches/models'), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageItems(
  apiKey: string,
  batchId: string,
  status = '',
): Promise<BatchImageItemsResponse> {
  if (isDemoSession()) {
    const items = demoBatchRecord(batchId).items
      .filter((item) => !status || item.status === status)
      .map((item) => cloneDemoItem(item))
    return { object: 'list', data: items, has_more: false }
  }
  const query = status ? `?status=${encodeURIComponent(status)}` : ''
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/items${query}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function cancelBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  if (isDemoSession()) {
    const record = demoBatchRecord(batchId)
    record.job.status = 'cancelled'
    record.job.settled_at = demoUnixTime()
    return cloneDemoJob(record.job)
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/cancel`), {
    method: 'POST',
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function downloadBatchImageZip(apiKey: string, batchId: string): Promise<Blob> {
  if (isDemoSession()) {
    const record = demoBatchRecord(batchId)
    return new Blob([
      `This is a local-only demonstration export for ${record.job.task_name}. No gateway request was made.`,
    ], { type: 'application/zip' })
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/download`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.blob()
}

export async function getBatchImageItemContent(apiKey: string, batchId: string, customId: string, imageIndex = 0): Promise<Blob> {
  if (isDemoSession()) {
    const record = demoBatchRecord(batchId)
    const item = record.items.find((candidate) => candidate.custom_id === customId)
    if (!item || imageIndex < 0 || imageIndex >= item.image_count) {
      throw demoBatchNotFoundError(`${batchId}/${customId}/${imageIndex}`)
    }
    return demoImageSvg(item.prompt_preview || item.custom_id)
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/items/${encodeURIComponent(customId)}/content?image_index=${encodeURIComponent(String(imageIndex))}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.blob()
}

export async function deleteBatchImageJobRecord(apiKey: string, batchId: string): Promise<void> {
  if (isDemoSession()) {
    demoBatchRecord(batchId)
    demoBatchRecords = demoBatchRecords.filter((record) => record.job.id !== batchId)
    return
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}`), {
    method: 'DELETE',
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
}

export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
