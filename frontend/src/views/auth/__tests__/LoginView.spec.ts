import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'

const {
  routeState,
  pushMock,
  loginMock,
  login2FAMock,
  showSuccessMock,
  showErrorMock,
  showWarningMock,
  clearAffiliateMock,
  authStoreState,
  getPublicSettingsMock,
} = vi.hoisted(() => ({
  routeState: {
    query: {} as Record<string, unknown>,
  },
  pushMock: vi.fn(),
  loginMock: vi.fn(),
  login2FAMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  showWarningMock: vi.fn(),
  clearAffiliateMock: vi.fn(),
  authStoreState: {
    isAdmin: false,
  },
  getPublicSettingsMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: routeState },
    push: (...args: unknown[]) => pushMock(...args),
  }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { locale: { value: 'en' }, t: (key: string) => key } }),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    get isAdmin() {
      return authStoreState.isAdmin
    },
    login: (...args: unknown[]) => loginMock(...args),
    login2FA: (...args: unknown[]) => login2FAMock(...args),
  }),
  useAppStore: () => ({
    showSuccess: (...args: unknown[]) => showSuccessMock(...args),
    showError: (...args: unknown[]) => showErrorMock(...args),
    showWarning: (...args: unknown[]) => showWarningMock(...args),
  }),
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
  isTotp2FARequired: (response: { requires_2fa?: unknown }) => response?.requires_2fa === true,
  isWeChatWebOAuthEnabled: () => false,
}))

vi.mock('@/utils/oauthAffiliate', () => ({
  clearAllAffiliateReferralCodes: (...args: unknown[]) => clearAffiliateMock(...args),
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: () => 'login failed',
}))

const globalStubs = {
  AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
  Icon: true,
  TurnstileWidget: true,
  LinuxDoOAuthSection: true,
  DingTalkOAuthSection: true,
  OidcOAuthSection: true,
  WechatOAuthSection: true,
  EmailOAuthButtons: true,
  LoginAgreementPrompt: true,
  TotpLoginModal: {
    props: ['tempToken'],
    emits: ['verify'],
    methods: {
      setVerifying() {},
      setError() {},
    },
    template: '<button data-testid="totp-verify" @click="$emit(\'verify\', \'123456\')">verify</button>',
  },
  RouterLink: true,
  transition: false,
}

function mountLoginView() {
  return mount(LoginView, { global: { stubs: globalStubs } })
}

async function submitCredentials(wrapper: ReturnType<typeof mount>) {
  await wrapper.find('#email').setValue('demo@example.test')
  await wrapper.find('#password').setValue('demo-password')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

describe('LoginView administrator redirects', () => {
  beforeEach(() => {
    routeState.query = {}
    authStoreState.isAdmin = false
    pushMock.mockReset()
    loginMock.mockReset()
    login2FAMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    showWarningMock.mockReset()
    clearAffiliateMock.mockReset()
    getPublicSettingsMock.mockReset()
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      linuxdo_oauth_enabled: false,
      dingtalk_oauth_enabled: false,
      wechat_oauth_enabled: false,
      oidc_oauth_enabled: false,
      github_oauth_enabled: false,
      google_oauth_enabled: false,
      backend_mode_enabled: false,
      password_reset_enabled: false,
    })
  })

  it('sends a direct demo-administrator login to the administrator dashboard', async () => {
    loginMock.mockImplementation(async () => {
      authStoreState.isAdmin = true
      return { access_token: 'demo-token', user: { is_demo: true, role: 'admin' } }
    })
    const wrapper = mountLoginView()
    await flushPromises()

    await submitCredentials(wrapper)

    expect(pushMock).toHaveBeenCalledWith('/admin/dashboard')
    expect(clearAffiliateMock).toHaveBeenCalledTimes(1)
  })

  it('uses the administrator dashboard after a demo-administrator 2FA login', async () => {
    loginMock.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'totp-token',
      user_email_masked: 'd***@example.test',
    })
    login2FAMock.mockImplementation(async () => {
      authStoreState.isAdmin = true
    })
    const wrapper = mountLoginView()
    await flushPromises()

    await submitCredentials(wrapper)
    await wrapper.find('[data-testid="totp-verify"]').trigger('click')
    await flushPromises()

    expect(login2FAMock).toHaveBeenCalledWith('totp-token', '123456')
    expect(pushMock).toHaveBeenCalledWith('/admin/dashboard')
  })

  it('keeps an explicitly requested route ahead of the administrator default', async () => {
    routeState.query = { redirect: '/admin/settings' }
    loginMock.mockImplementation(async () => {
      authStoreState.isAdmin = true
      return { access_token: 'demo-token', user: { is_demo: true, role: 'admin' } }
    })
    const wrapper = mountLoginView()
    await flushPromises()

    await submitCredentials(wrapper)

    expect(pushMock).toHaveBeenCalledWith('/admin/settings')
  })
})
