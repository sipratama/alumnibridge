import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { focusManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DirectoryPage } from './DirectoryPage'

vi.mock('@/app/authContext', () => ({ useAuth: () => ({ status: 'anonymous', login: vi.fn() }) }))

const member = { slug: 'siti-alumni', display_name: 'Siti Alumni', headline: 'Teacher', verified_alumni: true }
const page = (items = [member], next_cursor: string | null = null) =>
  new Response(JSON.stringify({ items, next_cursor }), { status: 200 })
const failure = (status: number) =>
  new Response(JSON.stringify({ error: status === 429 ? 'directory_rate_limited' : 'directory_overloaded' }), { status })
let client: QueryClient

function mount(search = '') {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[`/alumni${search}`]}><Routes><Route path="/alumni" element={<DirectoryPage />} /></Routes></MemoryRouter></QueryClientProvider>)
}
async function tick(milliseconds: number) {
  await act(async () => { await vi.advanceTimersByTimeAsync(milliseconds) })
}
async function settle() {
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
}

beforeEach(() => {
  vi.useFakeTimers()
  focusManager.setFocused(true)
  vi.stubGlobal('scrollTo', vi.fn())
  vi.stubGlobal('fetch', vi.fn(async () => page()))
})
afterEach(() => {
  cleanup()
  client?.clear()
  focusManager.setFocused(undefined)
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('visible Directory revalidation', () => {
  it('rechecks the visible active page every 15 seconds with one interval', async () => {
    mount()
    await settle()
    expect(screen.getByRole('heading', { name: 'Siti Alumni' })).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledTimes(1)
    await tick(14_999)
    expect(fetch).toHaveBeenCalledTimes(1)
    await tick(1)
    expect(fetch).toHaveBeenCalledTimes(2)
    await tick(30_000)
    expect(fetch).toHaveBeenCalledTimes(4)
    for (const [url, init] of vi.mocked(fetch).mock.calls) {
      expect(url).toBe('/api/v1/directory')
      expect(init).toMatchObject({ cache: 'no-store' })
    }
  })

  it('suppresses hidden-tab polling and revalidates again on focus', async () => {
    mount()
    await settle()
    focusManager.setFocused(false)
    await tick(45_000)
    expect(fetch).toHaveBeenCalledTimes(1)
    await act(async () => { focusManager.setFocused(true) })
    expect(fetch).toHaveBeenCalledTimes(2)
    await tick(15_000)
    expect(fetch).toHaveBeenCalledTimes(3)
  })

  it('masks the old card throughout a pending periodic recheck', async () => {
    let complete: ((response: Response) => void) | undefined
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(page()).mockImplementationOnce(() => new Promise<Response>((resolve) => { complete = resolve })))
    mount()
    await settle()
    await tick(15_000)
    await tick(1)
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
    expect(screen.getByText('Checking published profiles…')).toBeInTheDocument()
    await tick(30_000)
    expect(fetch).toHaveBeenCalledTimes(2)
    await act(async () => complete?.(page([])))
    await tick(1)
    expect(screen.getByText('No profiles match these filters')).toBeInTheDocument()
  })

  it.each([429, 503])('stops automatic polling and focus retries after %i, leaving manual retry available', async (status) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(page()).mockResolvedValueOnce(failure(status)).mockResolvedValueOnce(page([])))
    mount()
    await settle()
    await tick(15_000)
    await tick(1)
    expect(screen.queryByRole('heading', { name: 'Siti Alumni' })).not.toBeInTheDocument()
    expect(screen.getByRole('alert')).toBeInTheDocument()
    await tick(60_000)
    await act(async () => { focusManager.setFocused(false); focusManager.setFocused(true) })
    expect(fetch).toHaveBeenCalledTimes(2)
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    await settle()
    expect(fetch).toHaveBeenCalledTimes(3)
    expect(screen.getByText('No profiles match these filters')).toBeInTheDocument()
  })

  it('keeps the committed filters and current cursor, then aborts an obsolete poll on filter change', async () => {
    let aborted = false
    vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => {
      const params = new URL(url, 'http://localhost').searchParams
      if (params.get('cursor') === 'opaque' && vi.mocked(fetch).mock.calls.length === 3) {
        return new Promise<Response>((_, reject) => init?.signal?.addEventListener('abort', () => {
          aborted = true
          reject(new DOMException('Aborted', 'AbortError'))
        }))
      }
      return Promise.resolve(page([member], params.has('cursor') ? null : 'opaque'))
    }))
    mount('?query=Ari')
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await settle()
    expect(fetch).toHaveBeenCalledTimes(2)
    await tick(15_000)
    const polled = new URL(String(vi.mocked(fetch).mock.calls[2][0]), 'http://localhost').searchParams
    expect(polled.get('query')).toBe('Ari')
    expect(polled.get('cursor')).toBe('opaque')
    fireEvent.change(screen.getByLabelText('Name prefix'), { target: { value: 'Budi' } })
    fireEvent.click(screen.getByRole('button', { name: /Search alumni/i }))
    await settle()
    await settle()
    expect(aborted).toBe(true)
    const replacement = new URL(String(vi.mocked(fetch).mock.calls.at(-1)?.[0]), 'http://localhost').searchParams
    expect(replacement.get('query')).toBe('Budi')
    expect(replacement.has('cursor')).toBe(false)
  })

  it('does not poll invalid filter URLs and clears its interval on unmount', async () => {
    const invalid = mount('?query=x')
    await tick(45_000)
    expect(fetch).not.toHaveBeenCalled()
    invalid.unmount()
    const valid = mount()
    await settle()
    valid.unmount()
    await tick(45_000)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
})
