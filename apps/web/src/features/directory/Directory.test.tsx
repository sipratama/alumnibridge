import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { focusManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DirectoryPage } from './DirectoryPage'
import { controlledPhotoURL, defaultFilters, parseFilters, retryAfterSeconds, serializeFilters, type DirectoryEntry } from './api'

vi.mock('@/app/authContext', () => ({ useAuth: () => ({ status: 'anonymous', login: vi.fn() }) }))
const member: DirectoryEntry = { slug: 'siti-alumni', display_name: 'Siti Alumni', headline: 'Mathematics educator', verified_alumni: true, current_company: 'Acme School', skills: ['Teaching'], affiliations: [{ institution: { slug: 'uhamka', name: 'UHAMKA' }, study_program: { slug: 'mathematics-education', name: 'Mathematics Education' }, graduation_year: 2020 }] }
const response = (body: unknown, status = 200, headers?: Record<string, string>) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
const clients: QueryClient[] = []
function LocationActions() {
  const location = useLocation()
  const navigate = useNavigate()
  return <div><output data-testid="location">{location.search}</output><button onClick={() => navigate(-1)}>Browser back</button><button onClick={() => navigate(1)}>Browser forward</button></div>
}
function mount(search = '') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  clients.push(client)
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[`/alumni${search}`]}><LocationActions /><Routes><Route path="/alumni" element={<DirectoryPage />} /></Routes></MemoryRouter></QueryClientProvider>)
}
beforeEach(() => {
  vi.stubGlobal('scrollTo', vi.fn())
  vi.stubGlobal('fetch', vi.fn(async () => response({ items: [member], next_cursor: null })))
})
afterEach(() => { cleanup(); clients.splice(0).forEach((client) => client.clear()); vi.unstubAllGlobals() })

describe('directory URL and API contract', () => {
  it.each([200, 429, 503, 0])('masks downloaded cards during focus revalidation ending with %i', async (status) => {
    let finish: (() => void) | undefined
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(response({ items: [member], next_cursor: null })).mockImplementationOnce(() => new Promise<Response>((resolve, reject) => {
      finish = () => status === 0 ? reject(new TypeError('offline')) : resolve(response(status === 200 ? { items: [], next_cursor: null } : { error: 'directory_overloaded' }, status))
    })))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    act(() => { focusManager.setFocused(false); focusManager.setFocused(true) })
    await waitFor(() => expect(finish).toBeDefined())
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
    expect(screen.getByText('Checking published profiles…')).toBeInTheDocument()
    await act(async () => finish?.())
    if (status === 200) await screen.findByText('No profiles match these filters')
    else await screen.findByRole('alert')
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
    expect(fetch).toHaveBeenCalledTimes(2)
    focusManager.setFocused(undefined)
  })
  it('preserves commas inside skill names when restoring and submitting URL filters', async () => {
    mount('?skills=Design%2C+research&skills=Teaching')
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    expect(screen.getByLabelText('Skill names, one per line')).toHaveValue('Design, research\nTeaching')
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    expect(new URLSearchParams(screen.getByTestId('location').textContent ?? '').getAll('skills')).toEqual(['Design, research', 'Teaching'])
  })
  it('serializes repeated skills, restores shareable filters, and rejects invalid or repeated parameters', () => {
    const query = serializeFilters({ ...defaultFilters, institution: 'uhamka', study_program: 'mathematics-education', skills: ['Teaching', 'Research'], limit: 50 })
    const params = new URLSearchParams(query)
    expect(params.getAll('skills')).toEqual(['Teaching', 'Research'])
    expect(parseFilters(`?${query}`).filters.study_program).toBe('mathematics-education')
    expect(parseFilters('?query=a').error).toMatch(/2–100/)
    expect(parseFilters('?study_program=math').error).toMatch(/institution/)
    expect(parseFilters('?query=Siti&query=Adi').error).toMatch(/repeated/)
    expect(parseFilters('?unknown=private').error).toMatch(/unsupported/)
    expect(parseFilters('?skills=Teaching&skills=teaching').error).toMatch(/distinct/)
    expect(retryAfterSeconds('12')).toBe(12)
    expect(retryAfterSeconds('999')).toBeNull()
    expect(retryAfterSeconds('Wed, 21 Oct 2026 07:28:00 GMT')).toBeNull()
    expect(controlledPhotoURL({ ...member, photo_url: 'https://storage.invalid/private' })).toBeUndefined()
  })
  it('renders anonymously with only public fields and a canonical profile link', async () => {
    mount()
    expect(await screen.findByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
    expect(screen.getByText('Verified alumni')).toBeInTheDocument()
    expect(screen.getByText('Mathematics educator')).toBeInTheDocument()
    expect(screen.getByText('Teaching')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: "View profile for Siti Alumni" })).toHaveAttribute('href', '/in/siti-alumni')
    expect(screen.queryByText('membership_id')).not.toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls[0][1]).toMatchObject({ cache: 'no-store' })
    expect(vi.mocked(fetch).mock.calls[0][1]?.headers).not.toHaveProperty('Authorization')
  })
  it('submits filters explicitly, updates URL, and restores them with browser back/forward', async () => {
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.change(screen.getByLabelText('Name prefix'), { target: { value: 'Ari' } })
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('query=Ari'))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Alumni profiles' })).toHaveFocus())
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalledTimes(2))
    fireEvent.click(screen.getByRole('button', { name: 'Browser back' }))
    await waitFor(() => expect(screen.getByLabelText('Name prefix')).toHaveValue(''))
    fireEvent.click(screen.getByRole('button', { name: 'Browser forward' }))
    await waitFor(() => expect(screen.getByLabelText('Name prefix')).toHaveValue('Ari'))
  })
  it('blocks a program without an institution and avoids a request from invalid URLs', async () => {
    mount('?study_program=math')
    expect(screen.getByRole('alert')).toHaveTextContent('institution')
    expect(fetch).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Clear invalid filters' }))
    expect(await screen.findByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
    expect(screen.getByLabelText('Study program slug')).toBeDisabled()
  })
  it('traverses opaque cursors, refetches previous pages, and handles expired cursors', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      const cursor = new URL(url, 'http://localhost').searchParams.get('cursor')
      return cursor ? response({ error: 'invalid_input' }, 400) : response({ items: [member], next_cursor: 'opaque-token' })
    }))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText(/page link expired/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Back to first page' }))
    expect(await screen.findByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  })
  it.each([
    [429, 'directory_rate_limited', '5', /wait about 5 seconds/],
    [503, 'directory_overloaded', null, /Too many searches/],
    [503, 'directory_database_timeout', null, /took too long/],
  ])('shows bounded manual retry guidance for %i %s', async (status, code, retryAfter, detail) => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: code }, status, retryAfter ? { 'Retry-After': retryAfter } : undefined)))
    mount()
    expect(await screen.findByRole('alert')).toHaveTextContent(detail)
    expect(screen.getByRole('button', { name: 'Try again' })).toBeEnabled()
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('does not leave old cards visible when a later query fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => new URL(url, 'http://localhost').searchParams.has('query') ? response({ error: 'directory_overloaded' }, 503) : response({ items: [member], next_cursor: null })))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.change(screen.getByLabelText('Name prefix'), { target: { value: 'Ari' } })
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    expect(await screen.findByText(/Too many searches/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
  })
  it('cancels stale searches and displays only the latest filter response', async () => {
    let releaseSlow: (() => void) | undefined
    let canceled = false
    vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => {
      const query = new URL(url, 'http://localhost').searchParams.get('query')
      if (query === 'Slow') return new Promise<Response>((resolve, reject) => {
        releaseSlow = () => resolve(response({ items: [{ ...member, slug: 'slow-person', display_name: 'Slow Person' }], next_cursor: null }))
        init?.signal?.addEventListener('abort', () => { canceled = true; reject(new DOMException('Aborted', 'AbortError')) })
      })
      if (query === 'Fast') return Promise.resolve(response({ items: [{ ...member, slug: 'fast-person', display_name: 'Fast Person' }], next_cursor: null }))
      return Promise.resolve(response({ items: [member], next_cursor: null }))
    }))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.change(screen.getByLabelText('Name prefix'), { target: { value: 'Slow' } })
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    await waitFor(() => expect(releaseSlow).toBeDefined())
    fireEvent.change(screen.getByLabelText('Name prefix'), { target: { value: 'Fast' } })
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    expect(await screen.findByRole('heading', { name: 'Fast Person' })).toBeInTheDocument()
    releaseSlow?.()
    expect(canceled).toBe(true)
    expect(screen.queryByRole('heading', { name: 'Slow Person' })).not.toBeInTheDocument()
  })
  it('shows optional fields only when present and stops at the final page', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => new URL(url, 'http://localhost').searchParams.has('cursor') ?
      response({ items: [{ slug: 'ari-alumni', display_name: 'Ari Alumni', headline: '', verified_alumni: false }], next_cursor: null }) :
      response({ items: [member], next_cursor: 'opaque-token' })))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByRole('heading', { name: 'Ari Alumni' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.queryByText('Verified alumni')).not.toBeInTheDocument()
    expect(screen.queryByText('Teaching')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(await screen.findByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
  })
  it('recovers when privacy withdrawal leaves a later page empty', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => new URL(url, 'http://localhost').searchParams.has('cursor') ?
      response({ items: [], next_cursor: null }) : response({ items: [member], next_cursor: 'opaque-token' })))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByRole('heading', { name: 'This page has changed' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Back to first page' }))
    expect(await screen.findByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
  })
  it('accepts any backend page size from 1 to 50 and resets the cursor', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ items: [member], next_cursor: 'opaque-token' })))
    mount()
    await screen.findByRole('heading', { name: 'Siti Alumni' })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => expect(screen.getByText('Page 2')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Profiles per page'), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    await waitFor(() => expect(screen.getByText('Page 1')).toBeInTheDocument())
    const latest = vi.mocked(fetch).mock.calls.at(-1)?.[0]
    const params = new URL(String(latest), 'http://localhost').searchParams
    expect(params.get('limit')).toBe('1')
    expect(params.has('cursor')).toBe(false)
  })
})
