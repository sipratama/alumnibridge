import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@/app/authContext'
import { apiRequest, apiResponse, request, versionedRequest, ApiError, type WithETag } from '@/shared/api/client'

export type Profile = WithETag & {
  display_name: string
  slug: string
  headline: string
  bio: string
  location: string
  current_company: string
  current_job_title: string
  completeness: number
}
export type ProfileInput = Omit<Profile, 'completeness' | '__etag'>
export type ExperienceInput = {
  company: string
  job_title: string
  employment_type: string
  location: string
  start_date: string
  end_date?: string | null
  is_current: boolean
  description: string
}
export type Experience = ExperienceInput & { id: string; revision: number }
export type EducationInput = {
  institution: string
  degree: string
  field_of_study: string
  start_date?: string | null
  end_date?: string | null
  description: string
}
export type Education = EducationInput & { id: string; revision: number }
export type Skill = { id: string; name: string }
export const privacyFields = {
  show_bio: 'About',
  show_location: 'Location',
  show_current_company: 'Current company',
  show_current_job_title: 'Current job title',
  show_experiences: 'Work experience',
  show_educations: 'Education history',
  show_skills: 'Skills',
  show_photo: 'Profile photo',
} as const
export type Privacy = { visibility: 'private' | 'public' } & Record<
  keyof typeof privacyFields,
  boolean
> & WithETag
export type PublicProfile = Pick<
  Profile,
  'display_name' | 'slug' | 'headline'
> & {
  verified_alumni: boolean
  bio?: string
  location?: string
  current_company?: string
  current_job_title?: string
  experiences?: ExperienceInput[]
  educations?: EducationInput[]
  skills?: string[]
  photo_url?: string
}
export const ownerPath = '/users/me'
export const publicKey = ['public-profile'] as const
export function useOwnerQuery<T>(resource: string) {
  const { getToken, user } = useAuth()
  return useQuery({
    queryKey: ['professional', user?.id, resource],
    queryFn: ({ signal }) =>
      resource === 'profile' || resource === 'profile/privacy' || resource === 'profile/discovery'
        ? versionedRequest(`${ownerPath}/${resource}`, getToken, { signal }) as Promise<T>
        : request<T>(`${ownerPath}/${resource}`, getToken, { signal }),
    retry: false,
  })
}
export function useOwnerMutation<T, V>(
  mutate: (value: V, token: () => Promise<string>) => Promise<T>,
) {
  const { getToken, user } = useAuth()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (value: V) => mutate(value, getToken),
    onSuccess: async () => {
      // Public data is always re-read; never seed it with the owner's projection.
      client.removeQueries({ queryKey: publicKey })
      client.removeQueries({ queryKey: ['directory'] })
      await Promise.all([
        client.invalidateQueries({ queryKey: ['professional', user?.id] }),
        client.invalidateQueries({ queryKey: ['auth', 'me'] }),
      ])
    },
  })
}
export const json = (method: string, body?: unknown): RequestInit => ({
  method,
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
})
export function usePublicProfile(slug: string) {
  return useQuery({
    queryKey: [...publicKey, slug],
    queryFn: ({ signal }) =>
      apiRequest<PublicProfile>(`/profiles/${encodeURIComponent(slug)}`, {
        signal,
      }),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
    refetchInterval: 15000,
  })
}
export async function photoBlob(
  path: string,
  signal: AbortSignal,
  getToken?: () => Promise<string>,
): Promise<Blob | null> {
  try {
    const headers: Record<string, string> = { Accept: 'image/jpeg,image/png' }
    if (getToken) headers.Authorization = `Bearer ${await getToken()}`
    return await (await apiResponse(path, { signal, headers })).blob()
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401)
      return 'Your session expired. Sign in again to continue.'
    if (error.status === 412)
      return 'These settings changed in another session. Your edits are preserved; reload and review the latest values before retrying.'
    if (error.status === 428)
      return 'Your settings revision is unavailable. Reload the page before saving.'
    if (error.status === 409)
      return 'This change conflicts with saved data or the collection limit. Review your entries and try again.'
    if (error.status === 413)
      return 'This image is too large. Choose a file smaller than 5 MiB.'
    if (error.status === 415) return 'Choose a JPEG, PNG or WebP image.'
    if (error.status === 400)
      return 'Check the fields or image. Images must be valid, at most 4096 pixels per side and 8 million pixels in total.'
    if (error.status === 404)
      return 'This item is no longer available. Reload the page to see current data.'
    if (error.status === 503)
      return 'Photo storage is temporarily unavailable. Please try again.'
  }
  return 'We could not save your changes. Please try again; your entries are still here.'
}
