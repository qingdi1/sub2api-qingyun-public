import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AccountDomainWhitelistError,
  fetchAccountDomainWhitelist,
  validateAccountEndpoint,
  validateAccountPayloadEndpoints,
  type AccountDomainWhitelist
} from '../accountDomainWhitelist'

const whitelist: AccountDomainWhitelist = {
  version: 1,
  domains: [
    { hostname: 'api.openai.com' },
    { hostname: 'api.x.ai', include_subdomains: true }
  ]
}

describe('account domain whitelist', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('allows exact hosts and explicitly enabled subdomains', () => {
    expect(() => validateAccountEndpoint('https://api.openai.com/v1', whitelist)).not.toThrow()
    expect(() => validateAccountEndpoint('https://us-east-1.api.x.ai/v1', whitelist)).not.toThrow()
  })

  it('does not treat an exact-only host as a suffix wildcard', () => {
    expect(() => validateAccountEndpoint('https://relay.api.openai.com/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'domain_not_allowed' })
    )
  })

  it('rejects suffix lookalikes and non-HTTPS URLs', () => {
    expect(() => validateAccountEndpoint('https://api.openai.com.evil.example/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'domain_not_allowed' })
    )
    expect(() => validateAccountEndpoint('http://api.openai.com/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'https_required' })
    )
  })

  it('loads the repository whitelist without browser caching', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await expect(fetchAccountDomainWhitelist()).resolves.toEqual({
      version: 1,
      domains: [
        { hostname: 'api.openai.com', include_subdomains: false },
        { hostname: 'api.x.ai', include_subdomains: true }
      ]
    })
    expect(fetchMock).toHaveBeenCalledWith('/account-domain-whitelist.json', { cache: 'no-store' })
  })

  it('fetches again for every payload validation and checks both endpoint fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () =>
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await validateAccountPayloadEndpoints({ credentials: { base_url: 'https://api.openai.com/v1' } })
    await validateAccountPayloadEndpoints({ extra: { custom_base_url: 'https://us-east-1.api.x.ai/v1' } })

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('blocks creation when the whitelist cannot be loaded', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 404 }))

    await expect(
      validateAccountPayloadEndpoints({ credentials: { base_url: 'https://api.openai.com/v1' } })
    ).rejects.toEqual(expect.objectContaining({
      code: 'load_failed',
      name: AccountDomainWhitelistError.name
    }))
  })

  it('still fetches for account types without a configurable endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await validateAccountPayloadEndpoints({ credentials: { api_key: 'secret' }, extra: {} })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
