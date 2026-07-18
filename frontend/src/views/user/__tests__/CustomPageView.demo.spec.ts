import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  params: { id: 'demo-page' },
}))

const appStore = vi.hoisted(() => ({
  cachedPublicSettings: {
    custom_menu_items: [{ id: 'demo-page', page_slug: 'demo-guide', url: '' }],
  },
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}))

const authStore = vi.hoisted(() => ({
  user: { id: -1, is_demo: true },
  token: 'demo-token',
  isAdmin: false,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

import CustomPageView from '../CustomPageView.vue'

class TestMutationObserver {
  observe = vi.fn()
  disconnect = vi.fn()
}

describe('CustomPageView demo isolation', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
    routeState.params = { id: 'demo-page' }
    appStore.cachedPublicSettings = {
      custom_menu_items: [{ id: 'demo-page', page_slug: 'demo-guide', url: '' }],
    }
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('demo must not fetch pages')))
    vi.stubGlobal('MutationObserver', TestMutationObserver)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders local Markdown instead of fetching page content', async () => {
    const wrapper = mount(CustomPageView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
        },
      },
    })

    await flushPromises()

    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('演示页面')
    expect(wrapper.text()).toContain('本地模拟内容')
    wrapper.unmount()
  })

  it('does not embed an external page during a demo session', async () => {
    appStore.cachedPublicSettings = {
      custom_menu_items: [{ id: 'demo-page', page_slug: '', url: 'https://external.example.test/guide' }],
    }
    const wrapper = mount(CustomPageView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
