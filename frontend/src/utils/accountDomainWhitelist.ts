export interface AccountDomainWhitelist {
  version: 3
  domains: string[]
  ips: string[]
}

export interface AccountDomainWhitelistValidationOptions {
  validateCredentialsBaseUrl?: boolean
  validateCustomBaseUrl?: boolean
}

export type AccountDomainWhitelistErrorCode =
  | 'load_failed'
  | 'invalid_config'
  | 'invalid_url'
  | 'endpoint_not_allowed'

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

const domainPatternRegex = /^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

const normalizeDomainPattern = (entry: string): string => {
  const pattern = entry.trim().toLowerCase()
  if (!pattern.startsWith('*.')) {
    throw new AccountDomainWhitelistError('invalid_config', entry)
  }

  const suffix = pattern.slice(2)
  if (!domainPatternRegex.test(suffix)) {
    throw new AccountDomainWhitelistError('invalid_config', entry)
  }
  return `*.${suffix}`
}

const normalizeIPv4 = (entry: string): string => {
  const value = entry.trim()
  const parts = value.split('.')
  if (
    parts.length !== 4 ||
    parts.some((part) => !/^\d{1,3}$/.test(part) || Number(part) > 255)
  ) {
    throw new AccountDomainWhitelistError('invalid_config', entry)
  }
  return parts.map((part) => String(Number(part))).join('.')
}

const parseEndpointURL = (endpoint: string): URL => {
  let url: URL
  try {
    url = new URL(endpoint.trim())
  } catch {
    throw new AccountDomainWhitelistError('invalid_url', endpoint)
  }

  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
    throw new AccountDomainWhitelistError('invalid_url', endpoint, url.hostname)
  }
  return url
}

const parseWhitelist = (value: unknown): AccountDomainWhitelist => {
  if (!value || typeof value !== 'object') {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const candidate = value as Partial<AccountDomainWhitelist>
  if (
    candidate.version !== 3 ||
    !Array.isArray(candidate.domains) ||
    !Array.isArray(candidate.ips) ||
    candidate.domains.length + candidate.ips.length === 0
  ) {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const domains = candidate.domains.map((entry) => {
    if (typeof entry !== 'string' || !entry.trim()) {
      throw new AccountDomainWhitelistError('invalid_config')
    }
    return normalizeDomainPattern(entry)
  })
  const ips = candidate.ips.map((entry) => {
    if (typeof entry !== 'string' || !entry.trim()) {
      throw new AccountDomainWhitelistError('invalid_config')
    }
    return normalizeIPv4(entry)
  })

  if (new Set(domains).size !== domains.length || new Set(ips).size !== ips.length) {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  return { version: 3, domains, ips }
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
  const url = parseEndpointURL(endpoint)
  const hostname = url.hostname.toLowerCase().replace(/\.$/, '')
  const domainAllowed = whitelist.domains.some((pattern) =>
    hostname.endsWith(`.${pattern.slice(2)}`)
  )
  const ipAllowed = whitelist.ips.includes(hostname)

  if (!domainAllowed && !ipAllowed) {
    throw new AccountDomainWhitelistError('endpoint_not_allowed', endpoint, hostname)
  }
}

export interface AccountEndpointPayload {
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
}

export const validateAccountPayloadEndpoints = async (
  payload: AccountEndpointPayload,
  options: AccountDomainWhitelistValidationOptions = {}
): Promise<void> => {
  const whitelist = await fetchAccountDomainWhitelist()
  const checkCredentialsBaseUrl = options.validateCredentialsBaseUrl !== false
  const checkCustomBaseUrl = options.validateCustomBaseUrl !== false
  const endpoints = [
    checkCredentialsBaseUrl ? payload.credentials?.base_url : undefined,
    checkCustomBaseUrl ? payload.extra?.custom_base_url : undefined
  ]
    .filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
    .map((value) => value.trim())

  endpoints.forEach((endpoint) => validateAccountEndpoint(endpoint, whitelist))
}
