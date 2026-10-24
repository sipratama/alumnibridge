import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/authContext'
import { ApiError, versionedRequest } from '@/shared/api/client'
import { Dialog, Feedback, Field } from './components'
import { ConflictReview, type ConflictField } from './ConflictReview'
import {
  json,
  ownerPath,
  useOwnerMutation,
  type Profile,
  type ProfileInput,
} from './api'
import { profileSchema } from './validation'

const profileFields: Array<[keyof ProfileInput, string]> = [
  ['display_name', 'Display name'], ['headline', 'Professional headline'],
  ['location', 'General location'], ['current_job_title', 'Current job title'],
  ['current_company', 'Current company'], ['slug', 'Public profile slug'],
  ['bio', 'About'],
]

export function ProfileEditor({
  profile,
  onClose,
  onSaved,
}: {
  profile: Profile
  onClose: () => void
  onSaved: () => void
}) {
  const { getToken, refreshUser } = useAuth()
  const [origin] = useState(profile)
  const [base, setBase] = useState(origin)
  const [latest, setLatest] = useState<Profile>()
  const [reloadError, setReloadError] = useState(false)
  const [reloading, setReloading] = useState(false)
  const [baseline] = useState<ProfileInput>(() => {
    const { completeness: _, __etag: _etag, ...values } = origin
    return values
  })
  const form = useForm<ProfileInput>({
    resolver: zodResolver(profileSchema),
    defaultValues: baseline,
  })
  const draft = useWatch({ control: form.control })
  const mutation = useOwnerMutation(
    async (values: Partial<ProfileInput>, token) => {
      if (!base.__etag) throw new ApiError(428, 'revision_missing')
      return versionedRequest<Profile>(
        `${ownerPath}/profile`, token,
        { ...json('PATCH', values), headers: { 'If-Match': base.__etag } },
      )
    },
  )
  async function reloadLatest() {
    setReloading(true)
    try {
      const current = await versionedRequest<Profile>(`${ownerPath}/profile`, getToken)
      if (!current.__etag) throw new ApiError(428, 'revision_missing')
      setLatest(current)
      setReloadError(false)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) void refreshUser()
      setReloadError(true)
    } finally {
      setReloading(false)
    }
  }
  async function submit(values: ProfileInput) {
    const patch = Object.fromEntries(
      profileFields.filter(([key]) => values[key] !== base[key]).map(([key]) => [key, values[key]]),
    )
    if (!Object.keys(patch).length) {
      onClose()
      return
    }
    try {
      await mutation.mutateAsync(patch)
      onSaved()
    } catch (error) {
      if (error instanceof ApiError && error.status === 412) void reloadLatest()
      if (
        error instanceof ApiError &&
        error.status === 409 &&
        error.code !== 'profile_changed' &&
        'slug' in patch
      )
        form.setError(
          'slug',
          { message: 'This address is already reserved. Choose another slug.' },
          { shouldFocus: true },
        )
    }
  }
  const conflicts: ConflictField[] = latest ? profileFields
    .filter(([key]) => draft[key] !== base[key])
    .map(([key, label]) => ({ key, label, original: base[key], current: latest[key], draft: draft[key] })) : []
  function applySelected(selected: Set<string>) {
    if (!latest) return
    const values = form.getValues()
    const merged = Object.fromEntries(profileFields.map(([key]) => [key, selected.has(key) ? values[key] : latest[key]])) as ProfileInput
    setBase(latest)
    form.reset(merged)
    setLatest(undefined)
    mutation.reset()
  }
  return (
    <Dialog
      title="Edit professional profile"
      onClose={onClose}
      busy={mutation.isPending}
    >
      <p className="muted">
        Tell your story. Editing does not change your privacy settings.
      </p>
      <form onSubmit={form.handleSubmit(submit)} noValidate>
        <fieldset disabled={mutation.isPending} className="form-fieldset">
          <div className="form-grid">
            {(
              [
                ['display_name', 'Display name'],
                ['headline', 'Professional headline'],
                ['location', 'General location'],
                ['current_job_title', 'Current job title'],
                ['current_company', 'Current company'],
                ['slug', 'Public profile slug'],
              ] as const
            ).map(([key, label]) => (
              <Field
                key={key}
                label={label}
                error={form.formState.errors[key]?.message}
                hint={
                  key === 'slug'
                    ? '3–64 characters. Previously used addresses remain reserved.'
                    : undefined
                }
              >
                {(id, help) => (
                  <input
                    id={id}
                    aria-describedby={help}
                    aria-invalid={!!form.formState.errors[key]}
                    {...form.register(key)}
                    autoComplete={key === 'display_name' ? 'name' : 'off'}
                  />
                )}
              </Field>
            ))}
            <div className="full-width">
              <Field
                label="About"
                error={form.formState.errors.bio?.message}
                hint="Up to 5,000 characters."
              >
                {(id, help) => (
                  <textarea
                    id={id}
                    rows={6}
                    aria-describedby={help}
                    aria-invalid={!!form.formState.errors.bio}
                    {...form.register('bio')}
                  />
                )}
              </Field>
            </div>
          </div>
        </fieldset>
        {mutation.error instanceof ApiError &&
        (mutation.error.status === 412 || mutation.error.code === 'profile_changed') ? (
          <p role="alert" className="profile-error">
            Your profile changed in another session. Your entries are preserved
            here. Review the current values before saving again.
          </p>
        ) : (
          <Feedback error={mutation.error} />
        )}
        {latest && <ConflictReview key={latest.__etag} fields={conflicts} onApply={applySelected} />}
        {reloadError && <p role="alert" className="profile-error">Current profile could not be loaded. Your draft is preserved.</p>}
        <div className="form-actions">
          <Button type="button" variant="outline" disabled={reloading || mutation.isPending} onClick={() => void reloadLatest()}>
            {reloading ? 'Loading current profile…' : 'Reload current profile'}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button disabled={mutation.isPending || !!latest}>
            {mutation.isPending ? 'Saving…' : 'Save profile'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
