import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
}))

describe('demo API adapter', () => {
  let apiClient: AxiosInstance
  let networkAdapter: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    localStorage.clear()
    localStorage.setItem('auth_token', 'demo-token')
    localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
    vi.resetModules()

    const client = await import('@/api/client')
    apiClient = client.apiClient
    networkAdapter = vi.fn().mockResolvedValue({
      status: 200,
      statusText: 'OK',
      headers: {},
      config: {},
      data: { code: 0, data: { from: 'network' } },
    })
    apiClient.defaults.adapter = networkAdapter
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('answers dashboard requests locally without invoking the configured network adapter', async () => {
    const response = await apiClient.get('/usage/dashboard/stats')

    expect(response.data).toEqual(expect.objectContaining({ total_requests: 128 }))
    expect(networkAdapter).not.toHaveBeenCalled()
  })

  it('keeps API key writes in memory and never sends them to the backend', async () => {
    const { keysAPI } = await import('@/api/keys')

    const created = await keysAPI.create('temporary demo key')
    const listed = await keysAPI.list()

    expect(created.name).toBe('temporary demo key')
    expect(listed.items.some((key) => key.id === created.id)).toBe(true)
    expect(networkAdapter).not.toHaveBeenCalled()
  })

  it('does not silently route an unlisted authenticated endpoint to the backend', async () => {
    const response = await apiClient.get('/demo/unlisted-user-endpoint')

    expect(response.data).toEqual({})
    expect(networkAdapter).not.toHaveBeenCalled()
  })

  it('keeps login local after the demo session is established', async () => {
    const response = await apiClient.post('/auth/login', { email: 'demo@qingyun.local', password: 'secret' })

    expect(response.data).toEqual(expect.objectContaining({
      access_token: 'demo-token',
      user: expect.objectContaining({ id: -1, is_demo: true }),
    }))
    expect(networkAdapter).not.toHaveBeenCalled()
  })

  it('uses the backend-configured demo identity persisted at login for fixtures', async () => {
    localStorage.setItem('auth_user', JSON.stringify({
      id: -1,
      username: 'configured-demo-user',
      email: 'configured-demo@example.com',
      balance: 9999,
      balance_notify_enabled: true,
      is_demo: true,
      role: 'user',
    }))

    const response = await apiClient.get('/auth/me')

    expect(response.data).toEqual(expect.objectContaining({
      username: 'configured-demo-user',
      email: 'configured-demo@example.com',
      id: -1,
      role: 'user',
      is_demo: true,
      balance: 128.88,
      balance_notify_enabled: false,
    }))
    expect(networkAdapter).not.toHaveBeenCalled()
  })

  it('allows the initial login before a demo session exists to reach the backend once', async () => {
    localStorage.removeItem('auth_user')
    await apiClient.post('/auth/login', { email: 'demo@qingyun.local', password: 'secret' })

    expect(networkAdapter).toHaveBeenCalledTimes(1)
  })
})
