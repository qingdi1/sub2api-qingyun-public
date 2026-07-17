import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AccountDomainWhitelistError,
  fetchAccountDomainWhitelist,
  validateAccountEndpoint,
  validateAccountPayloadEndpoints,
  type AccountDomainWhitelist
} from '../accountDomainWhitelist'

const whitelist: AccountDomainWhitelist = {
  version: 2,
  endpoints: [
    'https://api.qinggekeji.top/v1',
    'http://47.107.127.143:8888/v1',
    'http://8.134.222.190:8888/v1',
    'http://192.140.188.165:8888/v1'
  ]
}

describe('account endpoint whitelist', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('allows each exact endpoint and a trailing slash', () => {
    for (const endpoint of whitelist.endpoints) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).not.toThrow()
      expect(() => validateAccountEndpoint(`${endpoint}/`, whitelist)).not.toThrow()
    }
  })

  it('requires the exact protocol, host, port, and /v1 path', () => {
    expect(() => validateAccountEndpoint('https://api.qinggekeji.top/v2', whitelist)).toThrowError(
      expect.objectContaining({ code: 'endpoint_not_allowed' })
    )
    expect(() => validateAccountEndpoint('http://47.107.127.143/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'endpoint_not_allowed' })
    )
    expect(() => validateAccountEndpoint('http://47.107.127.143:8888/v1/models', whitelist)).toThrowError(
      expect.objectContaining({ code: 'endpoint_not_allowed' })
    )
  })

  it('rejects query strings, fragments, credentials, and lookalikes', () => {
    for (const endpoint of [
      'https://api.qinggekeji.top/v1?tenant=test',
      'https://api.qinggekeji.top/v1#fragment',
      'https://user:pass@api.qinggekeji.top/v1'
    ]) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).toThrowError(
        expect.objectContaining({ code: 'invalid_url' })
      )
    }
    expect(() => validateAccountEndpoint('https://api.qinggekeji.top.evil.example/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'endpoint_not_allowed' })
    )
  })

  it('loads the repository whitelist without browser caching', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await expect(fetchAccountDomainWhitelist()).resolves.toEqual(whitelist)
    expect(fetchMock).toHaveBeenCalledWith('/account-domain-whitelist.json', { cache: 'no-store' })
  })

  it('fetches again for every payload validation and checks both endpoint fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () =>
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await validateAccountPayloadEndpoints({ credentials: { base_url: 'http://47.107.127.143:8888/v1' } })
    await validateAccountPayloadEndpoints({ extra: { custom_base_url: 'https://api.qinggekeji.top/v1' } })

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('can skip fixed OAuth credentials while still validating custom extras', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await expect(validateAccountPayloadEndpoints(
      {
        credentials: { base_url: 'https://cli-chat-proxy.grok.com/v1' },
        extra: { custom_base_url: 'http://8.134.222.190:8888/v1' }
      },
      { validateCredentialsBaseUrl: false }
    )).resolves.toBeUndefined()
  })

  it('blocks creation when the whitelist cannot be loaded', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 404 }))

    await expect(
      validateAccountPayloadEndpoints({ credentials: { base_url: 'http://47.107.127.143:8888/v1' } })
    ).rejects.toEqual(expect.objectContaining({
      code: 'load_failed',
      name: AccountDomainWhitelistError.name
    }))
  })
})
