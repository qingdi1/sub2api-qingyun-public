import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const authStore = vi.hoisted(() => ({
  store: null as { isAdmin: boolean } | null
}))
const appStore = vi.hoisted(() => ({
  versionLoading: false,
  currentVersion: '0.1.157',
  latestVersion: '0.1.158',
  hasUpdate: true,
  releaseInfo: null,
  buildType: 'release',
  fetchVersion: vi.fn().mockResolvedValue(null),
  clearVersionCache: vi.fn()
}))
const systemAPI = vi.hoisted(() => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/stores', async () => {
  const { reactive } = await vi.importActual<typeof import('vue')>('vue')
  authStore.store ??= reactive({ isAdmin: true })
  return {
    useAuthStore: () => authStore.store!,
    useAppStore: () => appStore
  }
})

vi.mock('@/api/admin/system', () => systemAPI)

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() })
}))

import VersionBadge from '../VersionBadge.vue'

function setTriggerRect(element: Element) {
  Object.defineProperty(element, 'getBoundingClientRect', {
    configurable: true,
    value: () => ({
      bottom: 44,
      height: 24,
      left: 20,
      right: 120,
      top: 20,
      width: 100,
      x: 20,
      y: 20,
      toJSON: () => ({})
    })
  })
}

describe('VersionBadge Docker update flow', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    authStore.store!.isAdmin = true
    appStore.versionLoading = false
    appStore.currentVersion = '0.1.157'
    appStore.latestVersion = '0.1.158'
    appStore.hasUpdate = true
    appStore.releaseInfo = null
    appStore.buildType = 'release'
    appStore.fetchVersion.mockReset()
    appStore.fetchVersion.mockResolvedValue(null)
    appStore.clearVersionCache.mockReset()
    systemAPI.performUpdate.mockReset()
    systemAPI.restartService.mockReset()
    systemAPI.getRollbackVersions.mockReset()
    systemAPI.rollback.mockReset()
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('checks once when administrator access becomes available after mount', async () => {
    authStore.store!.isAdmin = false
    const wrapper = mount(VersionBadge, {
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })

    await flushPromises()
    expect(appStore.fetchVersion).not.toHaveBeenCalled()

    authStore.store!.isAdmin = true
    await nextTick()
    await flushPromises()

    expect(appStore.fetchVersion).toHaveBeenCalledTimes(1)
    expect(appStore.fetchVersion).toHaveBeenCalledWith(false)

    authStore.store!.isAdmin = false
    await nextTick()
    authStore.store!.isAdmin = true
    await nextTick()
    await flushPromises()

    expect(appStore.fetchVersion).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('renders the update menu in a body portal above page stacking contexts', async () => {
    const wrapper = mount(VersionBadge, {
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })
    const trigger = wrapper.get('button[title="version.updateAvailable"]')
    setTriggerRect(trigger.element)

    await trigger.trigger('click')
    await flushPromises()

    const dropdown = document.body.querySelector<HTMLElement>('[data-testid="version-dropdown"]')
    expect(dropdown).not.toBeNull()
    expect(dropdown?.parentElement).toBe(document.body)
    expect(dropdown?.classList.contains('fixed')).toBe(true)
    expect(dropdown?.classList.contains('z-[100000030]')).toBe(true)
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.top).toBe('52px')

    const otherButton = document.createElement('button')
    document.body.append(otherButton)
    otherButton.click()
    await flushPromises()
    expect((wrapper.vm as any).$?.setupState.dropdownOpen).toBe(false)

    wrapper.unmount()
  })

  it('shows a queued Docker-agent update as accepted without asking for a restart', async () => {
    systemAPI.performUpdate.mockResolvedValue({
      queued: true,
      target_version: '0.1.158',
      delivery_mode: 'docker-agent',
      need_restart: false,
      message: 'Docker update agent accepted the request.',
      operation_id: 'update-test'
    })

    const wrapper = mount(VersionBadge, {
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })
    const trigger = wrapper.get('button[title="version.updateAvailable"]')
    setTriggerRect(trigger.element)

    await trigger.trigger('click')
    await flushPromises()

    const updateButton = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="version-update-action"]'
    )
    expect(updateButton).not.toBeNull()
    updateButton?.click()
    await flushPromises()

    const queued = document.body.querySelector('[data-testid="version-update-queued"]')
    expect(systemAPI.performUpdate).toHaveBeenCalledTimes(1)
    expect(queued).not.toBeNull()
    expect(queued?.textContent).toContain('version.updateScheduled')
    expect(queued?.textContent).toContain('Docker update agent accepted the request.')
    expect(queued?.textContent).toContain('v0.1.158')
    expect(document.body.textContent).not.toContain('version.restartNow')

    wrapper.unmount()
  })

  it('keeps polling until the deployed target is visible, then exposes restart without auto-restarting', async () => {
    appStore.fetchVersion.mockResolvedValue({
      current_version: '0.1.158',
      latest_version: '0.1.158',
      has_update: false,
      cached: false,
      build_type: 'release',
      delivery_mode: 'docker-agent'
    })
    systemAPI.performUpdate.mockResolvedValue({
      queued: true,
      target_version: '0.1.158',
      delivery_mode: 'docker-agent',
      need_restart: false,
      message: 'accepted'
    })

    const wrapper = mount(VersionBadge, {
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })
    const trigger = wrapper.get('button[title="version.updateAvailable"]')
    setTriggerRect(trigger.element)

    await trigger.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('[data-testid="version-update-action"]')?.click()
    await flushPromises()

    expect(document.body.querySelector('[data-testid="version-update-queued"]')).toBeNull()
    const restartButton = Array.from(document.body.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('version.restartNow')
    )
    expect(restartButton).not.toBeUndefined()
    expect(systemAPI.restartService).not.toHaveBeenCalled()

    wrapper.unmount()
  })
})
