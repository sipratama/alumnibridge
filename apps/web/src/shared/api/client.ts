export type Membership = {
  id: string
  role: 'student' | 'alumni'
  status: 'active' | 'pending' | 'verified' | 'rejected'
  institution: string
  study_program: string
  graduation_year?: number
}
export type CurrentUser = {
  id: string
  email?: string
  display_name?: string
  memberships: Membership[]
}
export type OnboardingInput = {
  role: 'student' | 'alumni'
  display_name: string
  institution: string
  study_program: string
  graduation_year?: number
}

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string) {
    super(code)
    this.status = status
    this.code = code
  }
}

export async function request<T>(
  path: string,
  getToken: () => Promise<string>,
  init?: RequestInit,
): Promise<T> {
  const token = await getToken()
  return apiRequest<T>(path, {
    ...init,
    headers: { ...init?.headers, Authorization: `Bearer ${token}` },
  })
}

// Owner ETags stay out of JSON; preserving them on the exact fetched object
// ensures a draft submits the validator it originally observed.
export type WithETag = { readonly __etag?: string }
export async function versionedRequest<T extends object>(
  path: string,
  getToken: () => Promise<string>,
  init?: RequestInit,
): Promise<T & WithETag> {
  const token = await getToken()
  const response = await apiResponse(path, {
    ...init,
    headers: { ...init?.headers, Authorization: `Bearer ${token}` },
  })
  const body = (await response.json()) as T & WithETag
  const etag = response.headers.get('ETag')
  if (etag) Object.defineProperty(body, '__etag', { value: etag, enumerable: false })
  return body
}

export async function apiResponse(
  path: string,
  init?: RequestInit,
): Promise<Response> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
    cache: 'no-store',
  })
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string }
    throw new ApiError(response.status, body.error ?? 'request_failed')
  }
  return response
}

export async function apiRequest<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const response = await apiResponse(path, init)
  return response.status === 204
    ? (undefined as T)
    : (response.json() as Promise<T>)
}
