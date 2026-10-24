import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ProfilePage } from './ProfilePage'
import { PrivacyPage } from './PrivacyPage'
import { PublicProfilePage } from './PublicProfilePage'
import { ProfileEditor } from './ProfileEditor'
import { HistorySection } from './HistorySection'
import { PhotoEditor } from './PhotoEditor'
import { SkillsSection } from './SkillsSection'
import { privacyFields, type Profile, type Privacy } from './api'
import { educationSchema, experienceSchema, profileSchema } from './validation'

Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
  configurable: true,
  writable: true,
  value() {
    this.setAttribute('open', '')
  },
})
Object.defineProperty(HTMLDialogElement.prototype, 'close', {
  configurable: true,
  writable: true,
  value() {
    this.removeAttribute('open')
  },
})

const auth = vi.hoisted(() => ({
  status: 'authenticated',
  getToken: vi.fn(async () => 'test-token'),
  user: {
    id: 'owner',
    display_name: 'Ari Member',
    memberships: [{ id: 'membership', role: 'alumni', status: 'pending' }],
  },
}))
vi.mock('@/app/authContext', () => ({
  useAuth: () => ({ ...auth, login: vi.fn(), logout: vi.fn() }),
}))
const profile: Profile = {
  display_name: 'Ari Member',
  slug: 'ari-member',
  headline: '',
  bio: '',
  location: '',
  current_company: '',
  current_job_title: '',
  completeness: 11,
  __etag: '"ab-profile-v1"',
}
const privacy: Privacy = {
  visibility: 'private',
  ...Object.fromEntries(Object.keys(privacyFields).map((key) => [key, false])),
} as Privacy
let savedProfile: Profile
let savedPrivacy: Privacy
let privacyRevision: number
let profileRevision: number
let experiences: Record<string, unknown>[]
let educations: Record<string, unknown>[]
let skills: { id: string; name: string }[]
let calls: {
  path: string
  method: string
  body: Record<string, unknown>
  headers?: HeadersInit
}[]
let clients: QueryClient[] = []
let rejectPath: string | undefined
let rejectStatus: number
const response = (body: unknown, status = 200) =>
  new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
function renderPage(element: React.ReactNode, path = '/profile') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{element}</MemoryRouter>
    </QueryClientProvider>,
  )
}
beforeEach(() => {
  savedProfile = { ...profile }
  savedPrivacy = { ...privacy }
  privacyRevision = 1
  profileRevision = 1
  experiences = []
  educations = []
  skills = []
  calls = []
  rejectPath = undefined
  rejectStatus = 500
  auth.getToken.mockClear()
  vi.spyOn(HTMLDialogElement.prototype, 'showModal').mockImplementation(
    function (this: HTMLDialogElement) {
      this.setAttribute('open', '')
    },
  )
  vi.spyOn(HTMLDialogElement.prototype, 'close').mockImplementation(function (
    this: HTMLDialogElement,
  ) {
    this.removeAttribute('open')
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = url.replace('/api/v1', '')
      const method = init?.method ?? 'GET'
      const body = typeof init?.body === 'string' ? JSON.parse(init.body) : {}
      calls.push({ path, method, body, headers: init?.headers })
      if (path === rejectPath && method !== 'GET')
        return response({ error: 'conflict' }, rejectStatus)
      if (path === '/users/me/profile/photo')
        return response({ error: 'not_found' }, 404)
      if (path === '/users/me/profile/discovery')
        return response({
          directory_discoverable: false,
          directory_consented_at: null,
          eligible: false,
          effective_discoverable: false,
          memberships: [],
        })
      if (path === '/users/me/profile/privacy') {
        const currentETag = `"ab-privacy-v${privacyRevision}"`
        if (method === 'PATCH') {
          const supplied = (init?.headers as Record<string, string> | undefined)?.['If-Match']
          if (supplied && supplied !== currentETag) return response({ error: 'stale_revision' }, 412)
          savedPrivacy = { ...savedPrivacy, ...body }
          privacyRevision++
        }
        return new Response(JSON.stringify(savedPrivacy), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: `"ab-privacy-v${privacyRevision}"` },
        })
      }
      if (path === '/users/me/profile') {
        if (method === 'PATCH') {
          const expected = (init?.headers as Record<string, string> | undefined)?.['If-Match']
          if (!expected) return response({ error: 'precondition_required' }, 428)
          if (expected !== `"ab-profile-v${profileRevision}"`) return response({ error: 'stale_revision' }, 412)
          savedProfile = { ...savedProfile, ...body }
          profileRevision++
        }
        return new Response(JSON.stringify(savedProfile), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: `"ab-profile-v${profileRevision}"` },
        })
      }
      for (const [kind, list] of [
        ['experiences', experiences],
        ['educations', educations],
        ['skills', skills],
      ] as const) {
        if (path.startsWith(`/users/me/${kind}`)) {
          const id = path.split('/')[4]
          if (method === 'POST') {
            const entry = { id: `new-${kind}`, revision: 1, ...body }
            list.push(entry as never)
            return response(entry, 201)
          }
          if (method === 'DELETE') {
            list.splice(
              list.findIndex((item) => item.id === id),
              1,
            )
            return response(null, 204)
          }
          if (method === 'PATCH') {
            const entry = list.find((item) => item.id === id)
            Object.assign(entry!, body)
            if (entry) (entry as Record<string, unknown>).revision = Number((entry as Record<string, unknown>).revision ?? 1) + 1
            return response(entry)
          }
          return response(list)
        }
      }
      if (path.startsWith('/skills?'))
        return response([{ id: 'go', name: 'Golang' }])
      if (path === '/profiles/old-ari')
        return response({
          slug: 'ari-member',
          display_name: 'Ari Member',
          headline: 'Educator',
          verified_alumni: false,
        })
      if (path.startsWith('/profiles/'))
        return savedPrivacy.visibility === 'public'
          ? response({
              slug: 'ari-member',
              display_name: 'Ari Member',
              headline: 'Educator',
              verified_alumni: false,
              ...(savedPrivacy.show_bio ? { bio: 'Shared biography' } : {}),
            })
          : response({ error: 'not_found' }, 404)
      throw new Error(`Unexpected API request ${method} ${path}`)
    }),
  )
  vi.stubGlobal('URL', URL)
  URL.createObjectURL = vi.fn(() => 'blob:test-preview')
  URL.revokeObjectURL = vi.fn()
})
afterEach(() => {
  cleanup()
  clients.forEach((client) => client.clear())
  clients = []
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function change(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label, { exact: true }), {
    target: { value },
  })
}
function click(name: string | RegExp) {
  fireEvent.click(screen.getByRole('button', { name }))
}

describe('owner professional profile', () => {
  it('renders server completeness, private state, pending verification and meaningful empty sections', async () => {
    renderPage(<ProfilePage />)
    expect(await screen.findByText('Ari Member')).toBeInTheDocument()
    expect(screen.getByText('Private')).toBeInTheDocument()
    expect(
      screen.getByText('Alumni · verification pending'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('progressbar', { name: 'Profile completeness' }),
    ).toHaveValue(11)
    expect(
      await screen.findByText(/Every path starts somewhere/),
    ).toBeInTheDocument()
    expect(screen.getByText(/Your skills will appear here/)).toBeInTheDocument()
    expect(screen.queryByText('owner')).not.toBeInTheDocument()
  })
  it('validates profile fields and sends only changed fields, including explicit clearing', async () => {
    savedProfile.headline = 'Old headline'
    renderPage(
      <ProfileEditor
        profile={savedProfile}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    )
    change('Display name', 'A')
    click('Save profile')
    expect(
      await screen.findByText('Enter at least 2 characters.'),
    ).toBeInTheDocument()
    change('Display name', 'Ari Member')
    change('Professional headline', '')
    click('Save profile')
    await waitFor(() =>
      expect(calls.some((call) => call.method === 'PATCH')).toBe(true),
    )
    expect(calls.find((call) => call.method === 'PATCH')?.body).toEqual({
      headline: '',
    })
  })
  it('keeps entries and gives a field error when a slug is reserved by another owner', async () => {
    rejectPath = '/users/me/profile'
    rejectStatus = 409
    renderPage(
      <ProfileEditor profile={profile} onClose={vi.fn()} onSaved={vi.fn()} />,
    )
    change('Public profile slug', 'reserved-person')
    click('Save profile')
    expect(
      await screen.findByText(/This address is already reserved/),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Public profile slug')).toHaveValue(
      'reserved-person',
    )
  })
  it('detects a changed dirty field in another session without overwriting it', async () => {
    savedProfile.headline = 'Changed elsewhere'
    profileRevision = 2
    renderPage(
      <ProfileEditor profile={profile} onClose={vi.fn()} onSaved={vi.fn()} />,
    )
    change('Professional headline', 'My unsaved draft')
    click('Save profile')
    expect(
      await screen.findByText(/Your profile changed in another session/),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Professional headline')).toHaveValue(
      'My unsaved draft',
    )
    expect(calls.filter((call) => call.method === 'PATCH')).toHaveLength(1)
  })
})

describe('experience and education', () => {
  it('adds, edits and deletes an experience; current employment clears the end date', async () => {
    renderPage(<HistorySection kind="experiences" />)
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Add experience' }),
      ).toBeEnabled(),
    )
    click('Add experience')
    change('Company', 'Campus Lab')
    change('Job title', 'Researcher')
    change('Start date', '2024-02-01')
    change('End date', '2025-01-01')
    fireEvent.click(screen.getByLabelText('I currently work here'))
    expect(screen.getByLabelText('End date')).toBeDisabled()
    click('Save experience')
    expect(
      await screen.findByRole('heading', { name: 'Researcher' }),
    ).toBeInTheDocument()
    expect(
      calls.find((call) => call.method === 'POST')?.body.end_date,
    ).toBeNull()
    click('Edit experience 1')
    change('Job title', 'Senior Researcher')
    click('Save experience')
    expect(
      await screen.findByRole('heading', { name: 'Senior Researcher' }),
    ).toBeInTheDocument()
    click('Delete experience 1')
    expect(
      screen.getByRole('dialog', { name: 'Remove experience?' }),
    ).toBeInTheDocument()
    click('Remove entry')
    expect(
      await screen.findByText(/Every path starts somewhere/),
    ).toBeInTheDocument()
  })
  it('validates date order and supports education add, edit and delete with nullable dates', async () => {
    renderPage(<HistorySection kind="educations" />)
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Add education' }),
      ).toBeEnabled(),
    )
    click('Add education')
    change('Institution', 'Sample University')
    change('Start date', '2024-01-01')
    change('End date', '2023-01-01')
    click('Save education')
    expect(
      await screen.findByText(/End date must be on or after/),
    ).toBeInTheDocument()
    change('End date', '')
    click('Save education')
    expect(
      await screen.findByRole('heading', { name: 'Sample University' }),
    ).toBeInTheDocument()
    expect(
      calls.find((call) => call.method === 'POST')?.body.end_date,
    ).toBeNull()
    click('Edit education 1')
    change('Degree', 'Mathematics')
    click('Save education')
    expect(await screen.findByText('Mathematics')).toBeInTheDocument()
    click('Delete education 1')
    click('Remove entry')
    expect(await screen.findByText(/Add where you learned/)).toBeInTheDocument()
  })
})

describe('skills and photos', () => {
  it('searches the shared vocabulary, selects a skill, blocks duplicates and removes the skill', async () => {
    renderPage(<SkillsSection />)
    await screen.findByText(/Your skills will appear here/)
    change('Search or add a skill', 'Go')
    const result = await screen.findByRole('button', { name: 'Golang' })
    fireEvent.click(result)
    expect(
      await screen.findByRole('button', { name: 'Remove Golang' }),
    ).toBeInTheDocument()
    change('Search or add a skill', 'golang')
    expect(screen.getByRole('button', { name: 'Add skill' })).toBeDisabled()
    click('Remove Golang')
    expect(
      await screen.findByText(/Your skills will appear here/),
    ).toBeInTheDocument()
    expect(
      calls.find((call) => call.path.startsWith('/skills?'))?.path,
    ).toContain('limit=10')
  })
  it('rejects unsupported and oversized files before upload; keeps preview on upload failure', async () => {
    renderPage(<PhotoEditor name="Ari" />)
    const input = screen.getByLabelText('Profile photo file')
    fireEvent.change(input, {
      target: {
        files: [new File(['svg'], 'bad.svg', { type: 'image/svg+xml' })],
      },
    })
    expect(screen.getByRole('alert')).toHaveTextContent('Choose a JPEG')
    fireEvent.change(input, {
      target: {
        files: [
          new File([new Uint8Array(5 * 1024 * 1024 + 1)], 'big.png', {
            type: 'image/png',
          }),
        ],
      },
    })
    expect(screen.getByRole('alert')).toHaveTextContent('smaller than 5 MiB')
    rejectPath = '/users/me/profile/photo'
    rejectStatus = 503
    fireEvent.change(input, {
      target: {
        files: [new File(['pixels'], 'ok.png', { type: 'image/png' })],
      },
    })
    click('Save photo')
    expect(
      await screen.findByText(/Photo storage is temporarily unavailable/),
    ).toBeInTheDocument()
    expect(screen.getByRole('img')).toHaveAttribute('src', 'blob:test-preview')
    expect(screen.getByRole('button', { name: 'Save photo' })).toBeEnabled()
  })
})

describe('privacy and public projection', () => {
  it('starts private with all optional flags off and requires unselected explicit consent', async () => {
    renderPage(<PrivacyPage />)
    const publicOption = await screen.findByRole('radio', { name: /^Public/ })
    expect(screen.getByRole('radio', { name: /^Private/ })).toBeChecked()
    for (const checkbox of screen.getAllByRole('checkbox'))
      expect(checkbox).not.toBeChecked()
    fireEvent.click(publicOption)
    const consent = screen.getByRole('checkbox', { name: /I consent/ })
    expect(consent).not.toBeChecked()
    click('Save privacy settings')
    expect(
      await screen.findByText(/Confirm publication consent/),
    ).toBeInTheDocument()
    expect(calls.filter((call) => call.method === 'PATCH')).toHaveLength(0)
    fireEvent.click(consent)
    fireEvent.click(screen.getByRole('checkbox', { name: 'About' }))
    click('Save privacy settings')
    expect(
      await screen.findByRole('link', { name: /View public profile/ }),
    ).toHaveAttribute('href', '/in/ari-member')
    expect(calls.find((call) => call.method === 'PATCH')?.body).toEqual({
      visibility: 'public',
      show_bio: true,
    })
    fireEvent.click(screen.getByRole('radio', { name: /^Private/ }))
    click('Save privacy settings')
    await screen.findByText(/Profile is private. New anonymous/)
    expect(
      screen.queryByRole('link', { name: /View public profile/ }),
    ).not.toBeInTheDocument()
  })
  it('does not restore an outdated visibility choice after another session changes privacy', async () => {
    renderPage(<PrivacyPage />)
    await screen.findByRole('radio', { name: /^Private/ })
    savedPrivacy = { ...savedPrivacy, visibility: 'public' }
    privacyRevision++
    click('Save privacy settings')
    expect(
      await screen.findByText(/Privacy settings changed in another session/),
    ).toBeInTheDocument()
    const privacyPatches = calls.filter((call) => call.method === 'PATCH' && call.path === '/users/me/profile/privacy')
    expect(privacyPatches).toHaveLength(1)
    expect((privacyPatches[0].headers as Record<string, string>)['If-Match']).toBe('"ab-privacy-v1"')
    expect(savedPrivacy.visibility).toBe('public')
  })
  it('renders a public allowlist anonymously without owner requests or verification inference', async () => {
    savedPrivacy.visibility = 'public'
    renderPage(
      <Routes>
        <Route path="/in/:slug" element={<PublicProfilePage />} />
      </Routes>,
      '/in/ari-member',
    )
    expect(
      await screen.findByRole('heading', { name: 'Ari Member' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Shared biography')).not.toBeInTheDocument()
    expect(screen.getByText('Not verified as alumni')).toBeInTheDocument()
    expect(calls.some((call) => call.path.startsWith('/users/'))).toBe(false)
    expect(auth.getToken).not.toHaveBeenCalled()
    expect(
      calls.find((call) => call.path === '/profiles/ari-member')?.headers,
    ).not.toHaveProperty('Authorization')
  })
  it('preserves public story geometry without exposing stale content during a background privacy check', async () => {
    savedPrivacy.visibility = 'public'
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      height: 1600,
    } as DOMRect)
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    const view = render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/in/ari-member']}>
          <Routes>
            <Route path="/in/:slug" element={<PublicProfilePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await screen.findByRole('heading', { name: 'Ari Member' })
    let finish!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve
          }),
      ),
    )
    act(() => {
      void client.invalidateQueries({ queryKey: ['public-profile'] })
    })
    await waitFor(() =>
      expect(view.container.querySelector('.public-story')).toHaveStyle({
        '--public-story-height': '1600px',
      }),
    )
    expect(view.container.querySelector('.public-story')).toHaveClass(
      'is-revalidating',
    )
    expect(screen.queryByText('Ari Member')).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await act(async () => {
      finish(response({ error: 'not_found' }, 404))
    })
    expect(
      await screen.findByRole('heading', { name: 'Profile unavailable' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Ari Member')).not.toBeInTheDocument()
  })
  it('hides a previously cached public projection while revalidating private access', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    client.setQueryData(['public-profile', 'ari-member'], {
      slug: 'ari-member',
      display_name: 'Previously public',
      headline: 'Old data',
      verified_alumni: false,
    })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/in/ari-member']}>
          <Routes>
            <Route path="/in/:slug" element={<PublicProfilePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    expect(screen.queryByText('Previously public')).not.toBeInTheDocument()
    expect(
      await screen.findByRole('heading', { name: 'Profile unavailable' }),
    ).toBeInTheDocument()
  })
  it('follows the server canonical slug using local navigation and presents a non-disclosing private state', async () => {
    savedPrivacy.visibility = 'public'
    renderPage(
      <Routes>
        <Route path="/in/:slug" element={<PublicProfilePage />} />
      </Routes>,
      '/in/old-ari',
    )
    await screen.findByRole('heading', { name: 'Ari Member' })
    expect(calls.some((call) => call.path === '/profiles/ari-member')).toBe(
      true,
    )
    savedPrivacy.visibility = 'private'
    cleanup()
    renderPage(
      <Routes>
        <Route path="/in/:slug" element={<PublicProfilePage />} />
      </Routes>,
      '/in/old-ari-unavailable',
    )
    expect(
      await screen.findByRole('heading', { name: 'Profile unavailable' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Ari Member' }),
    ).not.toBeInTheDocument()
  })
})

describe('server-aligned validation', () => {
  it('rejects reserved slugs, malformed dates, null characters and text limits without throwing', () => {
    expect(profileSchema.safeParse({ ...profile, slug: 'admin' }).success).toBe(
      false,
    )
    expect(
      profileSchema.safeParse({ ...profile, display_name: 'A\0ri' }).success,
    ).toBe(false)
    expect(
      profileSchema.safeParse({ ...profile, headline: 'x'.repeat(161) })
        .success,
    ).toBe(false)
    expect(
      profileSchema.safeParse({ ...profile, display_name: '😀'.repeat(100) })
        .success,
    ).toBe(true)
    expect(
      educationSchema.safeParse({
        institution: 'Campus',
        degree: '',
        field_of_study: '',
        start_date: '2024-99-99',
        end_date: '',
        description: '',
      }).success,
    ).toBe(false)
    expect(
      experienceSchema.safeParse({
        company: 'Campus',
        job_title: 'Teacher',
        employment_type: '',
        location: '',
        start_date: '',
        end_date: '',
        is_current: false,
        description: '',
      }).success,
    ).toBe(false)
  })
})
