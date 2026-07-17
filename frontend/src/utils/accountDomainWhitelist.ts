export interface AccountDomainWhitelist {
  version: 2
  endpoints: string[]
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

const normalizeEndpoint = (endpoint: string, code: 'invalid_config' | 'invalid_url'): string => {
  let url: URL
  try {
    url = new URL(endpoint.trim())
  } catch {
    throw new AccountDomainWhitelistError(code, endpoint)
  }

  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  ) {
    throw new AccountDomainWhitelistError(code, endpoint, url.hostname)
  }

  const pathname = url.pathname.replace(/\/+$/, '') || '/'
  return `${url.protocol}//${url.host}${pathname === '/' ? '' : pathname}`
}

const parseWhitelist = (value: unknown): AccountDomainWhitelist => {
  if (!value || typeof value !== 'object') {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const candidate = value as Partial<AccountDomainWhitelist>
  if (candidate.version !== 2 || !Array.isArray(candidate.endpoints) || candidate.endpoints.length === 0) {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  const endpoints = candidate.endpoints.map((entry) => {
    if (typeof entry !== 'string' || !entry.trim()) {
      throw new AccountDomainWhitelistError('invalid_config')
    }
    try {
      return normalizeEndpoint(entry, 'invalid_config')
    } catch (error) {
      if (error instanceof AccountDomainWhitelistError) {
        throw error
      }
      throw new AccountDomainWhitelistError('invalid_config')
    }
  })

  if (new Set(endpoints).size !== endpoints.length) {
    throw new AccountDomainWhitelistError('invalid_config')
  }

  return { version: 2, endpoints }
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
  let normalized: string
  try {
    normalized = normalizeEndpoint(endpoint, 'invalid_url')
  } catch (error) {
    if (error instanceof AccountDomainWhitelistError) {
      throw error
    }
    throw new AccountDomainWhitelistError('invalid_url', endpoint)
  }

  if (!whitelist.endpoints.includes(normalized)) {
    throw new AccountDomainWhitelistError('endpoint_not_allowed', endpoint, normalized)
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
