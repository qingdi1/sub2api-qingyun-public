import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  query: { order_id: 'demo-order', method: 'wechat_pay', amount: '10' },
}))
const loadStripe = vi.hoisted(() => vi.fn())

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
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@stripe/stripe-js', () => ({ loadStripe }))

import StripePopupView from '../StripePopupView.vue'

describe('StripePopupView demo isolation', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: -1, is_demo: true, role: 'user' }))
    loadStripe.mockReset()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('demo must not poll orders')))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('stops before Stripe initialization or order polling for a demo session', async () => {
    const wrapper = mount(StripePopupView)

    await flushPromises()
    window.dispatchEvent(new MessageEvent('message', {
      origin: window.location.origin,
      data: { type: 'STRIPE_POPUP_INIT', clientSecret: 'secret', publishableKey: 'pk_test' },
    }))
    await flushPromises()

    expect(wrapper.text()).toContain('演示账号不会处理真实支付。')
    expect(loadStripe).not.toHaveBeenCalled()
    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
