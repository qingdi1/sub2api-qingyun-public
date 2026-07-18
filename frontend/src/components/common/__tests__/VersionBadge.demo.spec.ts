import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const authStore = vi.hoisted(() => ({ isAdmin: true }))
const appStore = vi.hoisted(() => ({
  versionLoading: false,
  currentVersion: 'demo',
  latestVersion: 'demo',
  hasUpdate: false,
  releaseInfo: null,
  buildType: 'release',
  fetchVersion: vi.fn().mockResolvedValue(null),
  clearVersionCache: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => authStore,
  useAppStore: () => appStore,
}))

vi.mock('@/api/admin/system', () => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn(),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }),
}))

import VersionBadge from '../VersionBadge.vue'

describe('VersionBadge demo isolation', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
    appStore.fetchVersion.mockClear()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('demo must not poll /health')))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('treats the local demo service as healthy without fetching /health', async () => {
    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState as Record<string, () => Promise<boolean>>
    await expect(setupState.checkServiceAndReload()).resolves.toBe(true)

    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
