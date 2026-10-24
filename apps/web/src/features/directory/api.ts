import { ApiError, versionedRequest, type WithETag } from '@/shared/api/client'

export type DirectoryFilters = {
  query: string
  company: string
  job_title: string
  institution: string
  study_program: string
  graduation_year: string
  skills: string[]
  limit: number
}
export type DirectoryEntry = {
  slug: string
  display_name: string
  headline: string
  verified_alumni: boolean
  current_company?: string
  current_job_title?: string
  skills?: string[]
  affiliations?: Array<{
    institution: { slug: string; name: string }
    study_program?: { slug: string; name: string }
    graduation_year?: number
  }>
  photo_url?: string
}
export type DirectoryPage = { items: DirectoryEntry[]; next_cursor: string | null }
export type AcademicConsent = {
  membership_id: string
  show_institution: boolean
  show_study_program: boolean
  show_graduation_year: boolean
}
export type DiscoverySettings = WithETag & {
  directory_discoverable: boolean
  directory_consented_at: string | null
  eligible: boolean
  effective_discoverable: boolean
  memberships: Array<AcademicConsent & {
    institution: string
    study_program: string
    graduation_year: number
  }>
}
export type DiscoveryPatch = {
  directory_discoverable?: boolean
  affiliations?: AcademicConsent[]
}

export const defaultFilters: DirectoryFilters = {
  query: '', company: '', job_title: '', institution: '', study_program: '',
  graduation_year: '', skills: [], limit: 20,
}
const scalarKeys = ['query', 'company', 'job_title', 'institution', 'study_program', 'graduation_year', 'limit'] as const
const allowedKeys = new Set<string>([...scalarKeys, 'skills'])
const referenceSlug = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
const hasControl = (value: string) => /\p{Cc}/u.test(value)

export function validateFilters(filters: DirectoryFilters): string | null {
  for (const [key, max] of [['query', 100], ['company', 160], ['job_title', 160]] as const) {
    const value = filters[key]
    if (value && (Array.from(value.trim()).length < 2 || Array.from(value.trim()).length > max || hasControl(value)))
      return `${key.replace('_', ' ')} must contain 2–${max} characters.`
  }
  for (const key of ['institution', 'study_program'] as const) {
    const value = filters[key]
    if (value && (value.length > 64 || !referenceSlug.test(value)))
      return `${key.replace('_', ' ')} must be a lowercase reference slug.`
  }
  if (filters.study_program && !filters.institution)
    return 'Choose an institution before entering a study program.'
  if (filters.graduation_year && (!/^\d{4}$/.test(filters.graduation_year) || Number(filters.graduation_year) < 1950 || Number(filters.graduation_year) > 2100))
    return 'Graduation year must be between 1950 and 2100.'
  if (!Number.isInteger(filters.limit) || filters.limit < 1 || filters.limit > 50)
    return 'Page size must be between 1 and 50.'
  if (filters.skills.length > 5) return 'Choose at most five skills.'
  const normalized = new Set<string>()
  for (const skill of filters.skills) {
    const clean = skill.trim().replace(/\s+/gu, ' ').toLowerCase()
    if (!clean || Array.from(clean).length > 80 || hasControl(skill) || normalized.has(clean))
      return 'Use one to five distinct skill names, each at most 80 characters.'
    normalized.add(clean)
  }
  return null
}

export function parseFilters(search: string): { filters: DirectoryFilters; error: string | null } {
  const params = new URLSearchParams(search)
  const filters: DirectoryFilters = { ...defaultFilters, skills: [] }
  if (new TextEncoder().encode(search.startsWith('?') ? search.slice(1) : search).length > 4096)
    return { filters, error: 'The search URL is too long.' }
  for (const key of params.keys()) {
    if (!allowedKeys.has(key) || (key !== 'skills' && params.getAll(key).length !== 1))
      return { filters, error: 'This search URL contains unsupported or repeated filters.' }
  }
  for (const key of scalarKeys) {
    const value = params.get(key)
    if (value === null) continue
    if (!value.trim()) return { filters, error: 'Search URL filters cannot be empty.' }
    if (key === 'limit') {
      if (!/^\d+$/.test(value)) return { filters, error: 'Invalid page size in the URL.' }
      filters.limit = Number(value)
    } else filters[key] = value
  }
  filters.skills = params.getAll('skills')
  return { filters, error: validateFilters(filters) }
}

export function serializeFilters(filters: DirectoryFilters): string {
  const params = new URLSearchParams()
  for (const key of scalarKeys) {
    const value = filters[key]
    if (value && !(key === 'limit' && value === 20)) params.set(key, String(value))
  }
  for (const skill of filters.skills) params.append('skills', skill)
  return params.toString()
}

export class DirectoryError extends ApiError {
  retryAfter: number | null
  constructor(status: number, code: string, retryAfter: number | null) {
    super(status, code)
    this.retryAfter = retryAfter
  }
}
export function retryAfterSeconds(value: string | null): number | null {
  if (!value || !/^\d+$/.test(value)) return null
  const seconds = Number(value)
  return Number.isSafeInteger(seconds) && seconds >= 1 && seconds <= 60 ? seconds : null
}

export async function searchDirectory(filters: DirectoryFilters, cursor: string | null, signal: AbortSignal): Promise<DirectoryPage> {
  const params = new URLSearchParams(serializeFilters(filters))
  if (cursor) params.set('cursor', cursor)
  const response = await fetch(`/api/v1/directory${params.size ? `?${params}` : ''}`, {
    signal, cache: 'no-store', headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string }
    throw new DirectoryError(response.status, body.error ?? 'request_failed', retryAfterSeconds(response.headers.get('Retry-After')))
  }
  return response.json() as Promise<DirectoryPage>
}
export function getDiscovery(getToken: () => Promise<string>, signal: AbortSignal) {
  return versionedRequest<DiscoverySettings>('/users/me/profile/discovery', getToken, { signal })
}
export function updateDiscovery(getToken: () => Promise<string>, patch: DiscoveryPatch, etag?: string) {
  return versionedRequest<DiscoverySettings>('/users/me/profile/discovery', getToken, {
    method: 'PATCH', body: JSON.stringify(patch),
    headers: etag ? { 'If-Match': etag } : {},
  })
}
export function controlledPhotoURL(entry: DirectoryEntry): string | undefined {
  const expected = `/api/v1/profiles/${encodeURIComponent(entry.slug)}/photo`
  return entry.photo_url === expected ? expected : undefined
}
// Public profile navigation always uses the canonical slug returned by the API.
export function publicProfilePath(slug: string): string {
  return `/in/${encodeURIComponent(slug)}`
}
