export interface AccountDomainWhitelistEntry {
  hostname: string
  include_subdomains?: boolean
}

export interface AccountDomainWhitelist {
  version: number
  domains: AccountDomainWhitelistEntry[]
}

export type AccountDomainWhitelistErrorCode =
  | 'load_failed'
  | 'invalid_config'
  | 'invalid_url'
  | 'https_required'
  | 'domain_not_allowed'

export class AccountDomainWhitelistError extends Error {
  constructor(
    public readonly code: AccountDomainWhitelistErrorCode,
    public readonly endpoint?: string,
    public readonly hostname?: string
  ) {
    super(code)
    this.name = 'AccountDomainWhitelistError'
  }
}

const normalizeHostname = (hostname: string) => hostname.trim().toLowerCase().replace(/\.$/, '')

const parseWhitelist = (value: unknown): AccountDomainWhitelist => {
  if (!value || typeof value !== 'object') {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const candidate = value as Partial<AccountDomainWhitelist>
  if (candidate.version !== 1 || !Array.isArray(candidate.domains) || candidate.domains.length === 0) {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const domains = candidate.domains.map((entry) => {
    if (!entry || typeof entry.hostname !== 'string') {
      throw new AccountDomainWhitelistError('invalid_config')
    }
    const hostname = normalizeHostname(entry.hostname)
    if (!hostname || hostname.includes('/') || hostname.includes(':') || hostname.startsWith('.')) {
      throw new AccountDomainWhitelistError('invalid_config')
    }
    return {
      hostname,
      include_subdomains: entry.include_subdomains === true
    }
  })

  return { version: 1, domains }
}

export const fetchAccountDomainWhitelist = async (): Promise<AccountDomainWhitelist> => {
  let response: Response
  try {
    response = await fetch('/account-domain-whitelist.json', { cache: 'no-store' })
  } catch {
    throw new AccountDomainWhitelistError('load_failed')
  }

  if (!response.ok) {
    throw new AccountDomainWhitelistError('load_failed')
  }

  try {
    return parseWhitelist(await response.json())
  } catch (error) {
    if (error instanceof AccountDomainWhitelistError) {
      throw error
    }
    throw new AccountDomainWhitelistError('invalid_config')
  }
}

export const validateAccountEndpoint = (
  endpoint: string,
  whitelist: AccountDomainWhitelist
): void => {
  let url: URL
  try {
    url = new URL(endpoint)
  } catch {
    throw new AccountDomainWhitelistError('invalid_url', endpoint)
  }

  if (url.protocol !== 'https:') {
    throw new AccountDomainWhitelistError('https_required', endpoint, url.hostname)
  }

  const hostname = normalizeHostname(url.hostname)
  const allowed = whitelist.domains.some((entry) => {
    const allowedHostname = normalizeHostname(entry.hostname)
    return hostname === allowedHostname || (
      entry.include_subdomains === true && hostname.endsWith(`.${allowedHostname}`)
    )
  })

  if (!allowed) {
    throw new AccountDomainWhitelistError('domain_not_allowed', endpoint, hostname)
  }
}

interface AccountEndpointPayload {
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
}

export const validateAccountPayloadEndpoints = async (
  payload: AccountEndpointPayload
): Promise<void> => {
  const whitelist = await fetchAccountDomainWhitelist()
  const endpoints = [payload.credentials?.base_url, payload.extra?.custom_base_url]
    .filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
    .map((value) => value.trim())

  endpoints.forEach((endpoint) => validateAccountEndpoint(endpoint, whitelist))
}
