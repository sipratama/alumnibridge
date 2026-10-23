export type Membership = { id: string; role: 'student' | 'alumni'; status: 'active' | 'pending' | 'verified' | 'rejected'; institution: string; study_program: string; graduation_year?: number }
export type CurrentUser = { id: string; email?: string; display_name?: string; memberships: Membership[] }
export type OnboardingInput = { role: 'student' | 'alumni'; display_name: string; institution: string; study_program: string; graduation_year?: number }

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string) { super(code); this.status = status; this.code = code }
}

export async function request<T>(path: string, getToken: () => Promise<string>, init?: RequestInit): Promise<T> {
  const token = await getToken()
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), Authorization: `Bearer ${token}`, ...init?.headers },
    cache: 'no-store',
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string }
    throw new ApiError(response.status, body.error ?? 'request_failed')
  }
  return response.json() as Promise<T>
}
