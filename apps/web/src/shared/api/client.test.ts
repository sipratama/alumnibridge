import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiRequest, ApiError, request } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})
describe('typed API transport', () => {
  it('accepts successful DELETE responses without parsing an empty body', async () => {
    const fetch = vi.fn(async () => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetch)
    expect(
      await request<void>(
        '/users/me/skills/owned',
        async () => 'memory-token',
        { method: 'DELETE' },
      ),
    ).toBeUndefined()
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/users/me/skills/owned',
      expect.objectContaining({
        method: 'DELETE',
        cache: 'no-store',
        headers: expect.objectContaining({
          Authorization: 'Bearer memory-token',
        }),
      }),
    )
  })
  it('renews the token before a protected request and never fetches when renewal fails', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    await expect(
      request('/users/me/profile', async () => {
        throw new ApiError(401, 'unauthorized')
      }),
    ).rejects.toMatchObject({ status: 401 })
    expect(fetch).not.toHaveBeenCalled()
  })
  it('does not attach authorization to public requests and propagates cancellation', async () => {
    const fetch = vi.fn(async () => new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    await apiRequest('/profiles/member', { signal: controller.signal })
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/profiles/member',
      expect.objectContaining({
        signal: controller.signal,
        cache: 'no-store',
        headers: { Accept: 'application/json' },
      }),
    )
  })
})
