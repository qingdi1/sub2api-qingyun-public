/**
 * Local-only API adapter for the configured demonstration account.
 *
 * The adapter deliberately lives below the API modules. Existing views keep
 * using the normal API functions, while demo requests are answered here and
 * never reach the network. State is kept in memory and is reset on reload.
 */

import type {
  AxiosAdapter,
  AxiosHeaders,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from 'axios'
import { AxiosHeaders as AxiosHeadersImpl } from 'axios'
import type { ApiKey, Group, User } from '@/types'

const DEMO_DATE = '2026-01-01T00:00:00.000Z'

export const DEMO_USER: User = {
  id: -1,
  username: 'demo-user',
  email: 'demo@qingyun.local',
  is_demo: true,
  // The demo console intentionally mirrors the administrator surface. This
  // is presentation-only; the backend virtual identity and JWT remain a
  // regular user and every request is handled by this local adapter.
  role: 'admin',
  balance: 128.88,
  concurrency: 3,
  rpm_limit: 0,
  status: 'active',
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  last_active_at: DEMO_DATE,
  created_at: DEMO_DATE,
  updated_at: DEMO_DATE,
}

const DEMO_GROUP: Group = {
  id: 1,
  name: '演示模型组',
  description: '仅用于界面演示的本地模拟分组',
  platform: 'openai',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  default_mapped_model: '',
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: DEMO_DATE,
  updated_at: DEMO_DATE,
}

const DEMO_KEY: ApiKey = {
  id: -1,
  user_id: -1,
  key: 'sk-demo-qingyun-local-only',
  name: '演示密钥（不会真实调用）',
  group_id: 1,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: DEMO_DATE,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: DEMO_DATE,
  updated_at: DEMO_DATE,
  current_concurrency: 0,
  group: DEMO_GROUP,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
}

const DEMO_PLAN = {
  id: -1,
  group_id: 1,
  group_platform: 'openai',
  group_name: '演示模型组',
  rate_multiplier: 1,
  name: '演示套餐',
  description: '此套餐仅用于预览购买流程，不会产生订单',
  price: 0,
  original_price: 0,
  currency: 'CNY',
  validity_days: 30,
  validity_unit: 'day',
  features: ['模拟用量数据', '本地演示流程'],
  for_sale: false,
  sort_order: 0,
}

type DemoState = {
  user: User
  keys: ApiKey[]
  orders: Array<Record<string, unknown>>
  redeemedCodes: Array<Record<string, unknown>>
}

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

const state: DemoState = {
  user: clone(DEMO_USER),
  keys: [clone(DEMO_KEY)],
  orders: [],
  redeemedCodes: [],
}

let hasHydratedSessionUser = false

/** Whether the persisted session belongs to the isolated demo account. */
export function isDemoSession(): boolean {
  if (typeof globalThis.localStorage === 'undefined') {
    return false
  }

  try {
    const raw = globalThis.localStorage.getItem('auth_user')
    if (!raw) return false
    return (JSON.parse(raw) as { is_demo?: unknown }).is_demo === true
  } catch {
    return false
  }
}

/**
 * The backend owns the configurable demo account identity. Preserve that
 * response for the local fixtures rather than hard-coding a display name in
 * the browser, while retaining the fixed virtual-user security boundary.
 */
function hydrateSessionUser(): void {
  if (hasHydratedSessionUser || typeof globalThis.localStorage === 'undefined') {
    return
  }
  hasHydratedSessionUser = true

  try {
    const raw = globalThis.localStorage.getItem('auth_user')
    if (!raw) return
    const persisted = JSON.parse(raw) as Partial<User>
    if (persisted.is_demo !== true) return

    state.user = {
      ...DEMO_USER,
      // Only identity supplied by the backend configuration survives a page
      // reload. Mutable demo fixtures deliberately reset with the module.
      username: typeof persisted.username === 'string' && persisted.username.trim()
        ? persisted.username
        : DEMO_USER.username,
      email: typeof persisted.email === 'string' && persisted.email.trim()
        ? persisted.email
        : DEMO_USER.email,
      avatar_url: typeof persisted.avatar_url === 'string' || persisted.avatar_url === null
        ? persisted.avatar_url
        : DEMO_USER.avatar_url,
      id: -1,
      role: 'admin',
      is_demo: true,
    }
  } catch {
    // Invalid persisted data is handled by the auth store; fixtures stay safe.
  }
}

function normalizePath(url: unknown): string {
  const raw = String(url || '')
  try {
    const parsed = new URL(raw, 'http://demo.local')
    return parsed.pathname.replace(/^\/api\/v\d+(?=\/|$)/, '') || '/'
  } catch {
    return raw.split('?')[0].replace(/^\/api\/v\d+(?=\/|$)/, '') || '/'
  }
}

/**
 * Login reaches the server only before a demo session exists so it can issue
 * the isolated demo JWT. Once that session exists, every endpoint stays local,
 * including /auth/login, so a demo screen never mixes simulated and production
 * data or accidentally changes account type through a real login request.
 */
export function shouldMockDemoEndpoint(url: unknown): boolean {
  void url
  return isDemoSession()
}

function requestBody(config: InternalAxiosRequestConfig): Record<string, unknown> {
  if (!config.data) return {}
  if (typeof config.data === 'object') return config.data as Record<string, unknown>
  try {
    return JSON.parse(String(config.data)) as Record<string, unknown>
  } catch {
    return {}
  }
}

function nowISO(): string {
  return new Date().toISOString()
}

function paginated<T>(items: T[], config: InternalAxiosRequestConfig) {
  const page = Number((config.params as Record<string, unknown> | undefined)?.page || 1)
  const pageSize = Number((config.params as Record<string, unknown> | undefined)?.page_size || 20)
  return {
    items,
    total: items.length,
    page,
    page_size: pageSize,
    pages: items.length ? Math.ceil(items.length / pageSize) : 0,
  }
}

function dashboardStats() {
  return {
    total_api_keys: state.keys.length,
    active_api_keys: state.keys.filter((key) => key.status === 'active').length,
    total_requests: 128,
    total_input_tokens: 18240,
    total_output_tokens: 9120,
    total_cache_creation_tokens: 0,
    total_cache_read_tokens: 3580,
    total_tokens: 30940,
    total_cost: 1.28,
    total_actual_cost: 0.96,
    today_requests: 18,
    today_input_tokens: 2300,
    today_output_tokens: 1170,
    today_cache_creation_tokens: 0,
    today_cache_read_tokens: 520,
    today_tokens: 3990,
    today_cost: 0.18,
    today_actual_cost: 0.13,
    average_duration_ms: 680,
    rpm: 2,
    tpm: 540,
    by_platform: [],
  }
}

function trend() {
  const days = [0, 1, 2, 3, 4, 5, 6]
  return days.map((offset) => {
    const date = new Date(Date.now() - (6 - offset) * 86_400_000).toISOString().slice(0, 10)
    return {
      date,
      requests: 10 + offset * 2,
      input_tokens: 1200 + offset * 140,
      output_tokens: 600 + offset * 80,
      cache_creation_tokens: 0,
      cache_read_tokens: 180 + offset * 20,
      total_tokens: 1800 + offset * 220,
      cost: Number((0.08 + offset * 0.015).toFixed(4)),
      actual_cost: Number((0.06 + offset * 0.012).toFixed(4)),
    }
  })
}

function modelStats() {
  return [
    {
      model: 'gpt-5.4',
      requests: 72,
      input_tokens: 10400,
      output_tokens: 5200,
      cache_creation_tokens: 0,
      cache_read_tokens: 1820,
      total_tokens: 17420,
      cost: 0.78,
      actual_cost: 0.58,
    },
    {
      model: 'claude-sonnet-4',
      requests: 56,
      input_tokens: 7840,
      output_tokens: 3920,
      cache_creation_tokens: 0,
      cache_read_tokens: 1760,
      total_tokens: 13520,
      cost: 0.5,
      actual_cost: 0.38,
    },
  ]
}

function adminDashboardStats() {
  const now = nowISO()
  return {
    total_users: 24,
    today_new_users: 2,
    active_users: 8,
    hourly_active_users: 4,
    stats_updated_at: now,
    stats_stale: false,
    total_api_keys: 18,
    active_api_keys: 15,
    total_accounts: 6,
    normal_accounts: 5,
    error_accounts: 1,
    ratelimit_accounts: 0,
    overload_accounts: 0,
    total_requests: 128,
    total_input_tokens: 18240,
    total_output_tokens: 9120,
    total_cache_creation_tokens: 0,
    total_cache_read_tokens: 3580,
    total_tokens: 30940,
    total_cost: 1.28,
    total_actual_cost: 0.96,
    total_account_cost: 0.72,
    today_requests: 18,
    today_input_tokens: 2300,
    today_output_tokens: 1170,
    today_cache_creation_tokens: 0,
    today_cache_read_tokens: 520,
    today_tokens: 3990,
    today_cost: 0.18,
    today_actual_cost: 0.13,
    today_account_cost: 0.1,
    average_duration_ms: 680,
    uptime: 86400,
    rpm: 2,
    tpm: 540,
  }
}

function adminRealtimeMetrics() {
  return {
    active_requests: 1,
    requests_per_minute: 2,
    average_response_time: 680,
    error_rate: 0.01,
  }
}

function adminDashboardSnapshot(config: InternalAxiosRequestConfig) {
  return {
    generated_at: nowISO(),
    start_date: String((config.params as Record<string, unknown> | undefined)?.start_date || ''),
    end_date: String((config.params as Record<string, unknown> | undefined)?.end_date || ''),
    granularity: String((config.params as Record<string, unknown> | undefined)?.granularity || 'day'),
    stats: adminDashboardStats(),
    trend: trend(),
    models: modelStats(),
    groups: [{
      group_id: DEMO_GROUP.id,
      group_name: DEMO_GROUP.name,
      requests: 128,
      total_tokens: 30940,
      cost: 1.28,
      actual_cost: 0.96,
      account_cost: 0.72,
    }],
    users_trend: [{
      date: new Date().toISOString().slice(0, 10),
      user_id: DEMO_USER.id,
      email: DEMO_USER.email,
      username: DEMO_USER.username,
      requests: 18,
      tokens: 3990,
      cost: 0.18,
      actual_cost: 0.13,
    }],
  }
}

function demoAdminComplianceStatus() {
  return {
    required: false,
    version: 'demo',
    document_path_zh: 'docs/legal/admin-compliance.zh.md',
    document_path_en: 'docs/legal/admin-compliance.en.md',
    document_url_zh: '',
    document_url_en: '',
    ack_phrase_zh: '',
    ack_phrase_en: '',
  }
}

function demoAdminSettings() {
  return {
    site_name: '青云演示',
    site_subtitle: '本地模拟数据演示',
    custom_menu_items: [],
    ops_monitoring_enabled: false,
    ops_realtime_monitoring_enabled: false,
    ops_query_mode_default: 'auto',
    payment_enabled: false,
    registration_enabled: false,
    email_verify_enabled: false,
    backend_mode_enabled: false,
    default_subscription_settings: [],
    auth_source_defaults: {},
    available_channels_enabled: true,
  }
}

function demoAdminPaymentConfig() {
  return {
    enabled: false,
    min_amount: 1,
    max_amount: 1000,
    daily_limit: 1000,
    order_timeout_minutes: 30,
    max_pending_orders: 1,
    enabled_payment_types: [],
    balance_disabled: true,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 7,
    recharge_fee_rate: 0,
    load_balance_strategy: 'direct',
    product_name_prefix: '',
    product_name_suffix: '',
    help_image_url: '',
    help_text: '演示账号不会创建真实订单',
  }
}

function usageLog() {
  return {
    id: -1,
    user_id: -1,
    api_key_id: -1,
    account_id: null,
    request_id: 'demo-request-0001',
    model: 'gpt-5.4',
    service_tier: null,
    reasoning_effort: null,
    inbound_endpoint: '/v1/chat/completions',
    upstream_endpoint: 'demo://local',
    group_id: 1,
    subscription_id: null,
    input_tokens: 1200,
    output_tokens: 600,
    cache_creation_tokens: 0,
    cache_read_tokens: 180,
    cache_creation_5m_tokens: 0,
    cache_creation_1h_tokens: 0,
    input_cost: 0.05,
    output_cost: 0.04,
    cache_creation_cost: 0,
    cache_read_cost: 0.01,
    total_cost: 0.1,
    actual_cost: 0.075,
    rate_multiplier: 1,
    long_context_billing_applied: false,
    billing_type: 0,
    request_type: 'sync',
    stream: false,
    openai_ws_mode: false,
    duration_ms: 620,
    first_token_ms: 220,
    image_count: 0,
    image_size: null,
    image_input_size: null,
    image_output_size: null,
    image_size_source: null,
    image_size_breakdown: null,
    image_input_tokens: 0,
    image_input_cost: 0,
    image_output_tokens: 0,
    image_output_cost: 0,
    user_agent: 'Sub2API Demo',
    ip_address: '127.0.0.1',
    cache_ttl_overridden: false,
    billing_mode: 'token',
    created_at: DEMO_DATE,
    api_key: clone(DEMO_KEY),
    group: clone(DEMO_GROUP),
  }
}

function subscriptions() {
  return [
    {
      id: -1,
      user_id: -1,
      group_id: 1,
      status: 'active',
      starts_at: DEMO_DATE,
      daily_usage_usd: 0.25,
      weekly_usage_usd: 0.75,
      monthly_usage_usd: 1.5,
      daily_window_start: DEMO_DATE,
      weekly_window_start: DEMO_DATE,
      monthly_window_start: DEMO_DATE,
      created_at: DEMO_DATE,
      updated_at: DEMO_DATE,
      expires_at: null,
      group: clone(DEMO_GROUP),
    },
  ]
}

function subscriptionProgress() {
  return {
    subscription_id: -1,
    daily: { used: 0.05, limit: 0.25, percentage: 20, reset_in_seconds: 86_400 },
    weekly: { used: 0.1, limit: 0.75, percentage: 13, reset_in_seconds: 604_800 },
    monthly: { used: 0.2, limit: 1.5, percentage: 13, reset_in_seconds: 2_592_000 },
    expires_at: null,
    days_remaining: null,
  }
}

function publicSettings() {
  return {
    registration_enabled: false,
    email_verify_enabled: false,
    force_email_on_third_party_signup: false,
    registration_email_suffix_whitelist: [],
    promo_code_enabled: false,
    password_reset_enabled: false,
    invitation_code_enabled: false,
    turnstile_enabled: false,
    turnstile_site_key: '',
    site_name: '青云演示',
    site_logo: '',
    site_subtitle: '本地模拟数据演示',
    api_base_url: '',
    contact_info: '',
    doc_url: '',
    home_content: '',
    hide_ccs_import_button: true,
    payment_enabled: false,
    risk_control_enabled: false,
    table_default_page_size: 20,
    table_page_size_options: [10, 20, 50, 100],
    custom_menu_items: [],
    custom_endpoints: [],
    linuxdo_oauth_enabled: false,
    dingtalk_oauth_enabled: false,
    wechat_oauth_enabled: false,
    wechat_oauth_open_enabled: false,
    wechat_oauth_mp_enabled: false,
    wechat_oauth_mobile_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: false,
    google_oauth_enabled: false,
    backend_mode_enabled: false,
    version: 'demo',
    balance_low_notify_enabled: false,
    account_quota_notify_enabled: false,
    balance_low_notify_threshold: 0,
    channel_monitor_enabled: false,
    channel_monitor_default_interval_seconds: 60,
    available_channels_enabled: true,
    service_quota_enabled: false,
    affiliate_enabled: true,
    allow_user_view_error_requests: true,
  }
}

function handleDemoRequest(config: InternalAxiosRequestConfig): unknown {
  const path = normalizePath(config.url)
  const method = String(config.method || 'get').toLowerCase()
  const body = requestBody(config)

  hydrateSessionUser()

  if (path === '/auth/login' || path === '/auth/login/2fa') {
    return {
      access_token: globalThis.localStorage?.getItem('auth_token') || 'demo-local-token',
      token_type: 'Bearer',
      user: clone(state.user),
    }
  }

  if (path === '/setup/status') return { needs_setup: false }
  if (path === '/settings/public') return publicSettings()

  // The demo account can browse the full administrator surface, but these
  // responses are always local fixtures. No admin JWT is sent to the backend
  // and no operation below can mutate the real database.
  if (path === '/admin/compliance') return demoAdminComplianceStatus()
  if (path === '/admin/settings' && method === 'get') return demoAdminSettings()
  if (path === '/admin/payment/config' && method === 'get') return demoAdminPaymentConfig()
  if (path === '/admin/system/version' && method === 'get') return { version: '0.1.158-qingyun.1' }
  if (path === '/admin/system/check-updates' && method === 'get') {
    return {
      current_version: '0.1.158-qingyun.1',
      latest_version: '0.1.158-qingyun.1',
      has_update: false,
      cached: true,
      delivery_mode: 'demo-local',
      build_type: 'demo',
    }
  }
  if (path === '/admin/system/rollback-versions' && method === 'get') return { versions: [] }
  if (path === '/admin/dashboard/stats' && method === 'get') return adminDashboardStats()
  if (path === '/admin/dashboard/realtime' && method === 'get') return adminRealtimeMetrics()
  if (path === '/admin/dashboard/snapshot-v2' && method === 'get') return adminDashboardSnapshot(config)
  if (path === '/admin/dashboard/trend' && method === 'get') {
    return { trend: trend(), start_date: '', end_date: '', granularity: 'day' }
  }
  if (path === '/admin/dashboard/models' && method === 'get') return { models: modelStats(), start_date: '', end_date: '' }
  if (path === '/admin/dashboard/groups' && method === 'get') {
    return { groups: adminDashboardSnapshot(config).groups, start_date: '', end_date: '' }
  }
  if (path === '/admin/dashboard/users-trend' && method === 'get') {
    return { trend: adminDashboardSnapshot(config).users_trend, start_date: '', end_date: '', granularity: 'day' }
  }
  if (path === '/admin/dashboard/users-ranking' && method === 'get') {
    return {
      ranking: [{ user_id: DEMO_USER.id, email: DEMO_USER.email, actual_cost: 0.13, requests: 18, tokens: 3990 }],
      total_actual_cost: 0.13,
      total_requests: 18,
      total_tokens: 3990,
      start_date: '',
      end_date: '',
    }
  }
  if (path === '/admin/dashboard/user-breakdown' && method === 'get') {
    return {
      users: [{ user_id: DEMO_USER.id, email: DEMO_USER.email, requests: 18, input_tokens: 2300, output_tokens: 1170, cache_tokens: 520, total_tokens: 3990, cost: 0.18, actual_cost: 0.13, account_cost: 0.1 }],
      start_date: '',
      end_date: '',
    }
  }
  if (path === '/admin/dashboard/api-keys-trend' && method === 'get') return { trend: [], start_date: '', end_date: '', granularity: 'day' }
  if (path === '/admin/dashboard/users-usage' && method === 'post') return { stats: {} }
  if (path === '/admin/dashboard/api-keys-usage' && method === 'post') return { stats: {} }

  // Read-only list views get an empty, correctly shaped page so the demo
  // sidebar can be explored without a request falling through to production.
  if (path.startsWith('/admin/') && method === 'get') {
    if (/\/all$/.test(path) || /\/available$/.test(path)) return []
    if (/\/stats$/.test(path) || /\/summary$/.test(path)) return {}
    return paginated([], config)
  }
  if (path.startsWith('/admin/')) {
    return { message: '演示操作已完成，数据仅保存在当前页面内，不会写入数据库', demo: true }
  }

  if (path === '/auth/me' || path === '/user/profile') {
    return clone(state.user)
  }

  if (path === '/keys' && method === 'get') return paginated(clone(state.keys), config)
  if (path === '/keys' && method === 'post') {
    const next: ApiKey = {
      ...clone(DEMO_KEY),
      id: -state.keys.length - 1,
      name: typeof body.name === 'string' && body.name.trim() ? body.name : '演示密钥',
      key: `sk-demo-${Math.random().toString(36).slice(2, 10)}`,
      group_id: typeof body.group_id === 'number' ? body.group_id : 1,
      created_at: nowISO(),
      updated_at: nowISO(),
    }
    state.keys.push(next)
    return clone(next)
  }
  const keyMatch = path.match(/^\/keys\/(\-?\d+)$/)
  if (keyMatch) {
    const keyId = Number(keyMatch[1])
    const key = state.keys.find((item) => item.id === keyId) || state.keys[0]
    if (method === 'get') return clone(key)
    if (method === 'delete') {
      state.keys = state.keys.filter((item) => item.id !== keyId)
      return { message: '演示操作已完成，数据不会保存' }
    }
    if (method === 'put' || method === 'patch') {
      Object.assign(key, body, { updated_at: nowISO() })
      if (body.reset_quota === true) key.quota_used = 0
      return clone(key)
    }
  }

  if (path === '/usage' && method === 'get') return paginated([usageLog()], config)
  if (path === '/usage/stats') {
    return {
      period: String((config.params as Record<string, unknown> | undefined)?.period || 'today'),
      total_requests: 18,
      total_input_tokens: 2300,
      total_output_tokens: 1170,
      total_cache_tokens: 520,
      total_cache_read_tokens: 520,
      total_cache_creation_tokens: 0,
      total_tokens: 3990,
      total_cost: 0.18,
      total_actual_cost: 0.13,
      average_duration_ms: 680,
      models: { 'gpt-5.4': 12, 'claude-sonnet-4': 6 },
    }
  }
  if (path === '/usage/dashboard/stats') return dashboardStats()
  if (path === '/usage/dashboard/trend') {
    return { trend: trend(), start_date: '', end_date: '', granularity: 'day' }
  }
  if (path === '/usage/dashboard/models') return { models: modelStats(), start_date: '', end_date: '' }
  if (path === '/usage/dashboard/snapshot-v2') {
    return { generated_at: nowISO(), start_date: '', end_date: '', granularity: 'day', trend: trend(), models: modelStats(), groups: [] }
  }
  if (path === '/usage/dashboard/api-keys-usage') return { stats: { '-1': { api_key_id: -1, today_actual_cost: 0.13, total_actual_cost: 0.96 } } }
  if (path.startsWith('/usage/errors')) return paginated([], config)
  if (/^\/user\/api-keys\/.*\/usage\/daily$/.test(path)) return { items: [], days: 30, start_date: '', end_date: '' }

  if (path === '/groups/available') return [clone(DEMO_GROUP)]
  if (path === '/groups/rates') return { 1: 1 }
  if (path === '/channels/available') {
    return [{
      name: '演示渠道',
      description: '仅展示本地模拟的可用模型，不会连接上游服务',
      platforms: [{
        platform: 'openai',
        groups: [{ id: 1, name: '演示模型组', platform: 'openai', subscription_type: 'standard', rate_multiplier: 1, peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1, is_exclusive: false }],
        supported_models: [{ name: 'gpt-5.4', platform: 'openai', pricing: { billing_mode: 'token', input_price: 0.01, output_price: 0.03, cache_write_price: 0, cache_read_price: 0.001, image_input_price: null, image_output_price: null, per_request_price: null, intervals: [] } }],
      }],
    }]
  }
  if (path === '/channel-monitors') return { items: [] }
  if (path.startsWith('/channel-monitors/')) return { id: -1, name: '演示监控', provider: 'openai', group_name: '演示模型组', models: [] }

  if (path === '/subscriptions' || path === '/subscriptions/active') return subscriptions()
  if (path === '/subscriptions/progress') return [subscriptionProgress()]
  if (path === '/subscriptions/summary') return { active_count: 1, subscriptions: [{ id: -1, group_name: '演示模型组', status: 'active', daily_progress: 20, weekly_progress: 13, monthly_progress: 13, expires_at: null, days_remaining: null }] }
  if (/^\/subscriptions\/.*\/progress$/.test(path)) return subscriptionProgress()

  if (path === '/payment/config') return { payment_enabled: false, min_amount: 1, max_amount: 1000, daily_limit: 1000, max_pending_orders: 1, order_timeout_minutes: 30, balance_disabled: true, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 7, enabled_payment_types: [], help_image_url: '', help_text: '演示账号不会创建真实订单', stripe_publishable_key: '' }
  if (path === '/payment/plans') return [clone(DEMO_PLAN)]
  if (path === '/payment/limits') return { methods: {}, global_min: 0, global_max: 0 }
  if (path === '/payment/checkout-info') return { methods: {}, global_min: 0, global_max: 0, plans: [clone(DEMO_PLAN)], balance_disabled: true, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 7, recharge_fee_rate: 0, help_text: '演示账号不会创建真实订单', help_image_url: '', stripe_publishable_key: '' }
  if (path === '/payment/orders/my') return paginated(clone(state.orders), config)
  if (path.startsWith('/payment/orders')) {
    if (method === 'post') return { message: '演示操作已完成，未创建真实订单', order_id: -1, status: 'CANCELLED' }
    return state.orders[0] || { id: -1, status: 'CANCELLED', amount: 0, pay_amount: 0, refund_amount: 0, created_at: DEMO_DATE, expires_at: DEMO_DATE }
  }
  if (path.startsWith('/payment/public/orders')) return { out_trade_no: 'demo-order', status: 'CANCELLED', paid: false, created_at: DEMO_DATE, expires_at: DEMO_DATE }
  if (path === '/payment/orders/refund-eligible-providers') return { provider_instance_ids: [] }

  if (path === '/announcements') return []
  if (path.startsWith('/announcements/')) return { message: '演示操作已完成，数据不会保存' }
  if (path === '/redeem/history') return clone(state.redeemedCodes)
  if (path === '/redeem') return { message: '演示操作已完成，数据不会保存', type: 'balance', value: 0, new_balance: state.user.balance }
  if (path === '/user/aff') return { user_id: -1, aff_code: 'DEMO', inviter_id: null, aff_count: 0, aff_quota: 0, aff_frozen_quota: 0, aff_history_quota: 0, effective_rebate_rate_percent: 0, invitees: [] }
  if (path === '/user/aff/transfer') return { transferred_quota: 0, balance: state.user.balance }
  if (path === '/user/platform-quotas') return { platform_quotas: [] }

  if (path === '/user/totp/status') return { enabled: false, enabled_at: null, feature_enabled: false }
  if (path === '/user/totp/verification-method') return { method: 'password' }
  if (path.startsWith('/user/totp/')) return { success: true, verified: true, expires_in: 300, secret: 'DEMO-SECRET', qr_code_url: '' }

  if (path === '/user' && (method === 'put' || method === 'patch')) {
    Object.assign(state.user, body, { updated_at: nowISO(), is_demo: true })
    return clone(state.user)
  }
  if (path === '/user/notify-email/toggle') {
    const email = typeof body.email === 'string' ? body.email : ''
    const disabled = body.disabled === true
    const entry = state.user.balance_notify_extra_emails.find((item) => item.email === email)
    if (entry) entry.disabled = disabled
    return clone(state.user)
  }
  if (path === '/user/notify-email/verify') {
    const email = typeof body.email === 'string' ? body.email : ''
    if (email && !state.user.balance_notify_extra_emails.some((item) => item.email === email)) {
      state.user.balance_notify_extra_emails.push({ email, disabled: false, verified: true })
    }
    return { message: '演示操作已完成，数据不会保存' }
  }
  if (path === '/user/notify-email') {
    const email = typeof body.email === 'string' ? body.email : ''
    state.user.balance_notify_extra_emails = state.user.balance_notify_extra_emails.filter((item) => item.email !== email)
    return { message: '演示操作已完成，数据不会保存' }
  }
  if (path.startsWith('/user/account-bindings/')) return clone(state.user)
  if (path.startsWith('/user/')) return { message: '演示操作已完成，数据不会保存' }

  // Any other authenticated request is answered locally. This last branch is
  // intentional: an unlisted user endpoint must never fall through to the
  // real backend while the demo session is active.
  return method === 'get' ? {} : { message: '演示操作已完成，数据不会保存' }
}

function response<T>(config: InternalAxiosRequestConfig, data: T): AxiosResponse {
  const headers: AxiosHeaders = new AxiosHeadersImpl({ 'content-type': 'application/json' })
  return {
    data: { code: 0, message: 'demo', data },
    status: 200,
    statusText: 'OK',
    headers,
    config,
  }
}

/** Axios adapter used only after `auth_user.is_demo` is persisted. */
export const demoAdapter: AxiosAdapter = async (config) => {
  if (config.signal?.aborted) {
    throw new Error('canceled')
  }
  return response(config, handleDemoRequest(config))
}
