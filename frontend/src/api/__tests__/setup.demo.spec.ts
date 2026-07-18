import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const setupClient = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))
const axiosCreate = vi.hoisted(() => vi.fn(() => setupClient))

vi.mock('axios', () => ({
  default: { create: axiosCreate },
}))

describe('setup API demo isolation', () => {
  beforeEach(() => {
    localStorage.clear()
    setupClient.get.mockReset()
    setupClient.post.mockReset()
    axiosCreate.mockClear()
  })

  afterEach(() => {
    vi.resetModules()
  })

  it('answers setup status locally and does not call the standalone Axios client', async () => {
    localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
    const setup = await import('../setup')

    const status = await setup.getSetupStatus()
    await setup.testDatabase({ host: 'localhost', port: 5432, user: 'demo', password: 'demo', dbname: 'demo', sslmode: 'disable' })
    await setup.testRedis({ host: 'localhost', port: 6379, password: '', db: 0, enable_tls: false })
    const install = await setup.install({
      database: { host: 'localhost', port: 5432, user: 'demo', password: 'demo', dbname: 'demo', sslmode: 'disable' },
      redis: { host: 'localhost', port: 6379, password: '', db: 0, enable_tls: false },
      admin: { email: 'demo@example.test', password: 'demo' },
      server: { host: '127.0.0.1', port: 8080, mode: 'release' },
    })

    expect(status).toEqual({ needs_setup: false, step: '' })
    expect(install).toEqual(expect.objectContaining({ restart: false }))
    expect(setupClient.get).not.toHaveBeenCalled()
    expect(setupClient.post).not.toHaveBeenCalled()
  })

  it('keeps normal setup status requests on the standalone client', async () => {
    setupClient.get.mockResolvedValue({ data: { data: { needs_setup: true, step: 'database' } } })
    const { getSetupStatus } = await import('../setup')

    await expect(getSetupStatus()).resolves.toEqual({ needs_setup: true, step: 'database' })
    expect(setupClient.get).toHaveBeenCalledWith('/setup/status')
  })
})
