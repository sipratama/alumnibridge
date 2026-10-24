import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Button } from '@/components/ui/button'
import { ApiError, request } from '@/shared/api/client'
import { Dialog, Feedback, Field } from './components'
import {
  json,
  ownerPath,
  useOwnerMutation,
  type Profile,
  type ProfileInput,
} from './api'
import { profileSchema } from './validation'

export function ProfileEditor({
  profile,
  onClose,
  onSaved,
}: {
  profile: Profile
  onClose: () => void
  onSaved: () => void
}) {
  const [baseline] = useState<ProfileInput>(() => {
    const { completeness: _, ...values } = profile
    return values
  })
  const form = useForm<ProfileInput>({
    resolver: zodResolver(profileSchema),
    defaultValues: baseline,
  })
  const dirtyFields = form.formState.dirtyFields
  const mutation = useOwnerMutation(
    async (values: Partial<ProfileInput>, token) => {
      const latest = await request<Profile>(`${ownerPath}/profile`, token)
      if (
        (Object.keys(values) as (keyof ProfileInput)[]).some(
          (key) => latest[key] !== baseline[key],
        )
      )
        throw new ApiError(409, 'profile_changed')
      return request<Profile>(
        `${ownerPath}/profile`,
        token,
        json('PATCH', values),
      )
    },
  )
  async function submit(values: ProfileInput) {
    const patch = Object.fromEntries(
      Object.entries(values).filter(
        ([key]) => dirtyFields[key as keyof ProfileInput],
      ),
    )
    if (!Object.keys(patch).length) {
      onClose()
      return
    }
    try {
      await mutation.mutateAsync(patch)
      onSaved()
    } catch (error) {
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
        mutation.error.code === 'profile_changed' ? (
          <p role="alert" className="profile-error">
            Your profile changed in another session. Your entries are preserved
            here. Close and reload the profile before trying again.
          </p>
        ) : (
          <Feedback error={mutation.error} />
        )}
        <div className="form-actions">
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button disabled={mutation.isPending}>
            {mutation.isPending ? 'Saving…' : 'Save profile'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
