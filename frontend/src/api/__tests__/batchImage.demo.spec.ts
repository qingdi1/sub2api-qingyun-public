import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

async function loadDemoBatchImageApi() {
  localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
  vi.resetModules()
  return import('../batchImage')
}

describe('batch image demo isolation', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('serves every batch-image operation locally for the demo session', async () => {
    const batchImage = await loadDemoBatchImageApi()
    const apiKey = 'sk-not-sent-anywhere'

    const models = await batchImage.listBatchImageModels(apiKey)
    const initial = await batchImage.listBatchImageJobs(apiKey)
    const created = await batchImage.submitBatchImageJob(apiKey, {
      model: models.data[0].id,
      task_name: '本地测试任务',
      items: [{ custom_id: 'local-item', prompt: '本地模拟图片' }],
    }, 'idempotency-key')
    const fetched = await batchImage.getBatchImageJob(apiKey, created.id)
    const items = await batchImage.listBatchImageItems(apiKey, created.id)
    const image = await batchImage.getBatchImageItemContent(apiKey, created.id, 'local-item')
    const archive = await batchImage.downloadBatchImageZip(apiKey, created.id)
    const cancelled = await batchImage.cancelBatchImageJob(apiKey, created.id)
    await batchImage.deleteBatchImageJobRecord(apiKey, created.id)

    expect(models.data).toHaveLength(1)
    expect(initial.data.length).toBeGreaterThan(0)
    expect(fetched.id).toBe(created.id)
    expect(items.data).toEqual([expect.objectContaining({ custom_id: 'local-item', status: 'succeeded' })])
    expect(image.type).toBe('image/svg+xml')
    expect(archive.type).toBe('application/zip')
    expect(cancelled.status).toBe('cancelled')
    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
  })

  it('preserves the gateway request path for a non-demo session', async () => {
    vi.resetModules()
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ object: 'list', data: [] }),
    } as Response)
    const { listBatchImageModels } = await import('../batchImage')

    await listBatchImageModels('sk-real-user-key')

    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      expect.stringContaining('/v1/images/batches/models'),
      expect.objectContaining({ headers: { Authorization: 'Bearer sk-real-user-key' } }),
    )
  })
})
