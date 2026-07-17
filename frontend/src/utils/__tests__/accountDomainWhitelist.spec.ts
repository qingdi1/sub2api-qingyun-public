import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AccountDomainWhitelistError,
  fetchAccountDomainWhitelist,
  validateAccountEndpoint,
  validateAccountPayloadEndpoints,
  type AccountDomainWhitelist
} from '../accountDomainWhitelist'

const whitelist: AccountDomainWhitelist = {
  version: 3,
  domains: ['*.qinggekeji.top'],
  ips: ['47.107.127.143', '8.134.222.190', '192.140.188.165']
}

describe('account endpoint whitelist', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('allows any subdomain, port, path, query, or fragment under the domain pattern', () => {
    for (const endpoint of [
      'https://api.qinggekeji.top/v1',
      'http://api.qinggekeji.top:8888/v2/models',
      'https://hz.api.qinggekeji.top/custom/path?tenant=test#section'
    ]) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).not.toThrow()
    }
  })

  it('uses strict wildcard semantics and rejects the apex or lookalike domains', () => {
    for (const endpoint of [
      'https://qinggekeji.top/v1',
      'https://qinggekeji.top.evil.example/v1',
      'https://fakeqinggekeji.top/v1'
    ]) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).toThrowError(
        expect.objectContaining({ code: 'endpoint_not_allowed' })
      )
    }
  })

  it('allows whitelisted IP hosts with any port and path', () => {
    for (const endpoint of [
      'http://47.107.127.143/v1',
      'https://47.107.127.143:9443/custom/models?tenant=test',
      'http://8.134.222.190:3000/',
      'http://192.140.188.165:8888/v2'
    ]) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).not.toThrow()
    }
  })

  it('rejects unlisted hosts, userinfo, and non-HTTP protocols', () => {
    expect(() => validateAccountEndpoint('http://47.107.127.144:8888/v1', whitelist)).toThrowError(
      expect.objectContaining({ code: 'endpoint_not_allowed' })
    )
    for (const endpoint of [
      'https://user:pass@api.qinggekeji.top/v1',
      'ftp://api.qinggekeji.top/v1'
    ]) {
      expect(() => validateAccountEndpoint(endpoint, whitelist)).toThrowError(
        expect.objectContaining({ code: 'invalid_url' })
      )
    }
  })

  it('loads the repository whitelist without browser caching', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await expect(fetchAccountDomainWhitelist()).resolves.toEqual(whitelist)
    expect(fetchMock).toHaveBeenCalledWith('/account-domain-whitelist.json', { cache: 'no-store' })
  })

  it('rejects malformed or duplicate host rules', async () => {
    for (const invalidWhitelist of [
      { version: 2, endpoints: ['https://api.qinggekeji.top/v1'] },
      { version: 3, domains: ['qinggekeji.top'], ips: [] },
      { version: 3, domains: ['*.qinggekeji.top', '*.qinggekeji.top'], ips: [] },
      { version: 3, domains: [], ips: ['999.1.1.1'] }
    ]) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
        new Response(JSON.stringify(invalidWhitelist), { status: 200 })
      )
      await expect(fetchAccountDomainWhitelist()).rejects.toEqual(
        expect.objectContaining({ code: 'invalid_config' })
      )
    }
  })

  it('fetches again for every payload validation and checks both endpoint fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () =>
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await validateAccountPayloadEndpoints({ credentials: { base_url: 'http://47.107.127.143:9000/v2' } })
    await validateAccountPayloadEndpoints({ extra: { custom_base_url: 'https://other.qinggekeji.top/custom' } })

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('can skip fixed OAuth credentials while still validating custom extras', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(whitelist), { status: 200 })
    )

    await expect(validateAccountPayloadEndpoints(
      {
        credentials: { base_url: 'https://cli-chat-proxy.grok.com/v1' },
        extra: { custom_base_url: 'http://8.134.222.190:7777/custom' }
      },
      { validateCredentialsBaseUrl: false }
    )).resolves.toBeUndefined()
  })

  it('blocks creation when the whitelist cannot be loaded', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 404 }))

    await expect(
      validateAccountPayloadEndpoints({ credentials: { base_url: 'http://47.107.127.143/v1' } })
    ).rejects.toEqual(expect.objectContaining({
      code: 'load_failed',
      name: AccountDomainWhitelistError.name
    }))
  })
})
