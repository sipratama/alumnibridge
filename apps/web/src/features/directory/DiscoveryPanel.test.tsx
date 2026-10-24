import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DiscoveryPanel } from './DiscoveryPanel'
import type { DiscoverySettings } from './api'

const auth = vi.hoisted(() => ({ getToken: vi.fn(async () => 'signed-owner-token'), user: { id: 'owner' } }))
vi.mock('@/app/authContext', () => ({ useAuth: () => auth }))
const verified = { membership_id: '77777777-7777-4777-8777-777777777777', institution: 'UHAMKA', study_program: 'Mathematics Education', graduation_year: 2020, show_institution: false, show_study_program: false, show_graduation_year: false }
let settings: DiscoverySettings
let patches: Array<Record<string, unknown>>
let fail = false
let revision = 1
let client: QueryClient
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ETag: `"ab-privacy-v${revision}"` } })
function mount(visibility: 'public' | 'private' = 'public') {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><DiscoveryPanel visibility={visibility} /></QueryClientProvider>)
}
beforeEach(() => {
  settings = { directory_discoverable: false, directory_consented_at: null, eligible: true, effective_discoverable: false, memberships: [{ ...verified }] }
  patches = []
  fail = false
  revision = 1
  auth.getToken.mockClear()
  vi.stubGlobal('confirm', vi.fn(() => true))
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    expect(url).toBe('/api/v1/users/me/profile/discovery')
    expect(init?.headers).toHaveProperty('Authorization', 'Bearer signed-owner-token')
    if (init?.method === 'PATCH') {
      const patch = JSON.parse(String(init.body)) as Record<string, unknown>
      patches.push(patch)
      const expected = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      if (expected && expected !== `"ab-privacy-v${revision}"`) return response({ error: 'stale_revision' }, 412)
      if (fail) return response({ error: 'conflict' }, 409)
      settings = {
        ...settings, directory_discoverable: patch.directory_discoverable as boolean ?? settings.directory_discoverable,
        effective_discoverable: patch.directory_discoverable as boolean ?? settings.effective_discoverable,
        memberships: settings.memberships.map((m) => ({ ...m, ...(patch.affiliations ? { show_institution: false, show_study_program: false, show_graduation_year: false } : {}), ...((patch.affiliations as typeof settings.memberships)?.find((a) => a.membership_id === m.membership_id) ?? {}) })),
      }
      revision++
      return response(settings)
    }
    return response(settings)
  }))
})
afterEach(() => { cleanup(); client?.clear(); vi.unstubAllGlobals() })

describe('owner discovery consent', () => {
  it('rejects stale draft via original If-Match and retains unsaved choices', async () => {
    mount()
    fireEvent.click(await screen.findByRole('checkbox', { name: /Include my profile/ }))
    revision = 2 // Another session committed after this draft loaded.
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/changed in another session/)
    const calls = vi.mocked(fetch).mock.calls
    const patchCall = calls.find(([, init]) => init?.method === 'PATCH')
    expect((patchCall?.[1]?.headers as Record<string, string>)['If-Match']).toBe('"ab-privacy-v1"')
    expect(screen.getByRole('checkbox', { name: /Include my profile/ })).toBeChecked()
    expect(settings.directory_discoverable).toBe(false)
  })
  it('retains unsaved academic choices after a transient background GET failure', async () => {
    mount()
    fireEvent.click(await screen.findByRole('checkbox', { name: /Include my profile/ }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Show institution' }))
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError('offline'))
    await act(async () => { await client.invalidateQueries({ queryKey: ['professional'] }) })
    expect(await screen.findByRole('alert')).toHaveTextContent('choices are preserved')
    expect(screen.getByRole('checkbox', { name: 'Show institution' })).toBeChecked()
  })
  it('keeps a successful PATCH result visible when its follow-up GET fails', async () => {
    const original = fetch
    let patchSucceeded = false
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      if (patchSucceeded && init?.method !== 'PATCH') throw new TypeError('offline')
      const result = await original(url, init)
      if (init?.method === 'PATCH') patchSucceeded = true
      return result
    }))
    mount()
    fireEvent.click(await screen.findByRole('checkbox', { name: /Include my profile/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    expect(await screen.findByText(/can now appear/)).toBeInTheDocument()
    expect(await screen.findByRole('alert')).toHaveTextContent('choices are preserved')
    expect(screen.getByRole('checkbox', { name: /Include my profile/ })).toBeChecked()
  })
  it('keeps academic disclosure off until explicit owner opt-in and enforces institution dependency', async () => {
    mount()
    const toggle = await screen.findByRole('checkbox', { name: /Include my profile/ })
    expect(toggle).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Show study program' })).toBeDisabled()
    fireEvent.click(toggle)
    const institution = screen.getByRole('checkbox', { name: 'Show institution' })
    fireEvent.click(institution)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Show study program' }))
    fireEvent.click(institution)
    expect(screen.getByRole('checkbox', { name: 'Show study program' })).not.toBeChecked()
    fireEvent.click(institution)
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    await waitFor(() => expect(patches).toHaveLength(1))
    expect(patches[0]).toEqual({ directory_discoverable: true, affiliations: [{ membership_id: verified.membership_id, show_institution: true, show_study_program: false, show_graduation_year: false }] })
    expect(await screen.findByText(/can now appear/)).toBeInTheDocument()
  })
  it('blocks ineligible and private owners without sending a PATCH', async () => {
    settings.eligible = false
    settings.memberships = []
    mount('private')
    expect(await screen.findByRole('checkbox', { name: /Include my profile/ })).toBeDisabled()
    expect(screen.getByText(/Publish your profile first/)).toBeInTheDocument()
    expect(patches).toHaveLength(0)
  })
  it('explains pending-alumni ineligibility on a public profile', async () => {
    settings.eligible = false
    settings.memberships = []
    mount()
    expect(await screen.findByRole('checkbox', { name: /Include my profile/ })).toBeDisabled()
    expect(screen.getByText(/Student and pending alumni memberships are not eligible/)).toBeInTheDocument()
  })
  it('sends an empty replacement array when academic publication is withdrawn and clears public caches', async () => {
    settings.directory_discoverable = true
    settings.effective_discoverable = true
    settings.memberships[0].show_institution = true
    mount()
    await screen.findByRole('checkbox', { name: 'Show institution' })
    client.setQueryData(['directory', 'old'], { items: [{ slug: 'private' }], next_cursor: null })
    client.setQueryData(['public-profile', 'old'], { slug: 'private' })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Show institution' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    await waitFor(() => expect(patches).toHaveLength(1))
    expect(patches[0]).toEqual({ affiliations: [] })
    await waitFor(() => expect(client.getQueryData(['directory', 'old'])).toBeUndefined())
    expect(client.getQueryData(['public-profile', 'old'])).toBeUndefined()
  })
  it('confirms opt-out and preserves choices when a save fails', async () => {
    settings.directory_discoverable = true
    settings.effective_discoverable = true
    settings.memberships[0].show_institution = true
    mount()
    const toggle = await screen.findByRole('checkbox', { name: /Include my profile/ })
    fireEvent.click(toggle)
    vi.mocked(confirm).mockReturnValueOnce(false)
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    expect(patches).toHaveLength(0)
    expect(screen.getByRole('checkbox', { name: 'Show institution' })).toBeChecked()
    fail = true
    fireEvent.click(screen.getByRole('button', { name: 'Save discovery settings' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/verification status/)
    expect(toggle).not.toBeChecked()
    expect(patches[0]).toEqual({ directory_discoverable: false })
  })
})
