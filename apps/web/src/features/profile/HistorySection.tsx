import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { request } from '@/shared/api/client'
import {
  json,
  ownerPath,
  useOwnerMutation,
  useOwnerQuery,
  type Education,
  type Experience,
  type ExperienceInput,
  type EducationInput,
} from './api'
import {
  ConfirmDelete,
  Dialog,
  EducationList,
  ExperienceList,
  Feedback,
  Field,
  LoadError,
  Loading,
} from './components'
import { educationSchema, experienceSchema } from './validation'

export function HistorySection({
  kind,
}: {
  kind: 'experiences' | 'educations'
}) {
  const isWork = kind === 'experiences'
  const title = isWork ? 'Experience' : 'Education'
  const singular = isWork ? 'experience' : 'education'
  const query = useOwnerQuery<(Experience | Education)[]>(kind)
  const [editing, setEditing] = useState<Experience | Education | 'new'>()
  const [removing, setRemoving] = useState<Experience | Education>()
  const [message, setMessage] = useState('')
  const remove = useOwnerMutation((id: string, token) =>
    request<void>(
      `${ownerPath}/${kind}/${encodeURIComponent(id)}`,
      token,
      json('DELETE'),
    ),
  )
  return (
    <section className="profile-panel" aria-labelledby={`${kind}-heading`}>
      <div className="section-top">
        <h2 id={`${kind}-heading`}>{title}</h2>
        <Button
          variant="outline"
          disabled={!query.data || query.data.length >= 100}
          onClick={() => {
            setMessage('')
            setEditing('new')
          }}
        >
          <Plus size={16} aria-hidden="true" />
          Add {singular}
        </Button>
      </div>
      {query.isPending ? (
        <Loading label={`Loading ${singular}…`} />
      ) : query.error ? (
        <LoadError error={query.error} retry={() => void query.refetch()} />
      ) : !query.data.length ? (
        <p className="empty-copy">
          {isWork
            ? 'Every path starts somewhere. Add a role, internship or part-time position.'
            : 'Add where you learned and what you studied. This does not change your membership verification.'}
        </p>
      ) : (
        (() => {
          const actions = (index: number) => (
            <div className="entry-actions">
              <button
                className="icon-action"
                aria-label={`Edit ${singular} ${index + 1}`}
                onClick={() => {
                  setMessage('')
                  setEditing(query.data[index])
                }}
              >
                <Pencil size={17} />
              </button>
              <button
                className="icon-action"
                aria-label={`Delete ${singular} ${index + 1}`}
                onClick={() => {
                  remove.reset()
                  setRemoving(query.data[index])
                }}
              >
                <Trash2 size={17} />
              </button>
            </div>
          )
          return isWork ? (
            <ExperienceList
              items={query.data as Experience[]}
              actions={actions}
            />
          ) : (
            <EducationList
              items={query.data as Education[]}
              actions={actions}
            />
          )
        })()
      )}
      {query.data?.length === 100 && (
        <p className="muted">You have reached the 100-entry limit.</p>
      )}
      <Feedback message={message} />
      {editing &&
        (isWork ? (
          <ExperienceEditor
            item={editing === 'new' ? undefined : (editing as Experience)}
            onClose={() => setEditing(undefined)}
            onSaved={() => {
              setEditing(undefined)
              setMessage('Experience saved.')
            }}
          />
        ) : (
          <EducationEditor
            item={editing === 'new' ? undefined : (editing as Education)}
            onClose={() => setEditing(undefined)}
            onSaved={() => {
              setEditing(undefined)
              setMessage('Education saved.')
            }}
          />
        ))}
      {removing && (
        <ConfirmDelete
          title={`Remove ${singular}?`}
          busy={remove.isPending}
          error={remove.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() => {
            void remove
              .mutateAsync(removing.id)
              .then(() => {
                setRemoving(undefined)
                setMessage(`${title} removed.`)
              })
              .catch(() => {})
          }}
        />
      )}
    </section>
  )
}

function ExperienceEditor({
  item,
  onClose,
  onSaved,
}: {
  item?: Experience
  onClose: () => void
  onSaved: () => void
}) {
  const form = useForm({
    resolver: zodResolver(experienceSchema),
    defaultValues: {
      company: item?.company ?? '',
      job_title: item?.job_title ?? '',
      employment_type: item?.employment_type ?? '',
      location: item?.location ?? '',
      start_date: item?.start_date ?? '',
      end_date: item?.end_date ?? '',
      is_current: item?.is_current ?? false,
      description: item?.description ?? '',
    },
  })
  const current = useWatch({ control: form.control, name: 'is_current' })
  const mutation = useOwnerMutation((body: ExperienceInput, token) =>
    request<Experience>(
      `${ownerPath}/experiences${item ? `/${encodeURIComponent(item.id)}` : ''}`,
      token,
      json(item ? 'PATCH' : 'POST', body),
    ),
  )
  return (
    <Dialog
      title={`${item ? 'Edit' : 'Add'} experience`}
      onClose={onClose}
      busy={mutation.isPending}
    >
      <form
        noValidate
        onSubmit={form.handleSubmit(async (values) => {
          try {
            await mutation.mutateAsync({
              ...values,
              end_date: values.is_current ? null : values.end_date || null,
            })
            onSaved()
          } catch {
            /* Keep the form available for retry. */
          }
        })}
      >
        <fieldset disabled={mutation.isPending} className="form-fieldset">
          <div className="form-grid">
            {(
              [
                ['company', 'Company'],
                ['job_title', 'Job title'],
                ['employment_type', 'Employment type'],
                ['location', 'Location'],
                ['start_date', 'Start date'],
                ['end_date', 'End date'],
              ] as const
            ).map(([key, label]) => (
              <Field
                key={key}
                label={label}
                error={form.formState.errors[key]?.message}
              >
                {(id, help) => (
                  <input
                    id={id}
                    type={key.endsWith('date') ? 'date' : 'text'}
                    aria-invalid={!!form.formState.errors[key]}
                    aria-describedby={help}
                    {...form.register(key)}
                    disabled={key === 'end_date' && current}
                  />
                )}
              </Field>
            ))}
            <label className="check-row full-width">
              <input
                type="checkbox"
                {...form.register('is_current')}
                onChange={(event) => {
                  void form.setValue('is_current', event.target.checked, {
                    shouldDirty: true,
                  })
                  if (event.target.checked)
                    form.setValue('end_date', '', {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                }}
              />
              I currently work here
            </label>
            <div className="full-width">
              <Field
                label="Description"
                error={form.formState.errors.description?.message}
              >
                {(id, help) => (
                  <textarea
                    id={id}
                    aria-describedby={help}
                    rows={4}
                    {...form.register('description')}
                  />
                )}
              </Field>
            </div>
          </div>
        </fieldset>
        <Feedback error={mutation.error} />
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
            {mutation.isPending ? 'Saving…' : 'Save experience'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
function EducationEditor({
  item,
  onClose,
  onSaved,
}: {
  item?: Education
  onClose: () => void
  onSaved: () => void
}) {
  const form = useForm({
    resolver: zodResolver(educationSchema),
    defaultValues: {
      institution: item?.institution ?? '',
      degree: item?.degree ?? '',
      field_of_study: item?.field_of_study ?? '',
      start_date: item?.start_date ?? '',
      end_date: item?.end_date ?? '',
      description: item?.description ?? '',
    },
  })
  const mutation = useOwnerMutation((body: EducationInput, token) =>
    request<Education>(
      `${ownerPath}/educations${item ? `/${encodeURIComponent(item.id)}` : ''}`,
      token,
      json(item ? 'PATCH' : 'POST', body),
    ),
  )
  return (
    <Dialog
      title={`${item ? 'Edit' : 'Add'} education`}
      onClose={onClose}
      busy={mutation.isPending}
    >
      <p className="muted">
        Education entries do not verify alumni membership.
      </p>
      <form
        noValidate
        onSubmit={form.handleSubmit(async (values) => {
          try {
            await mutation.mutateAsync({
              ...values,
              start_date: values.start_date || null,
              end_date: values.end_date || null,
            })
            onSaved()
          } catch {
            /* Keep the form available for retry. */
          }
        })}
      >
        <fieldset className="form-fieldset" disabled={mutation.isPending}>
          <div className="form-grid">
            {(
              [
                ['institution', 'Institution'],
                ['degree', 'Degree'],
                ['field_of_study', 'Field of study'],
                ['start_date', 'Start date'],
                ['end_date', 'End date'],
              ] as const
            ).map(([key, label]) => (
              <Field
                key={key}
                label={label}
                error={form.formState.errors[key]?.message}
              >
                {(id, help) => (
                  <input
                    id={id}
                    type={key.endsWith('date') ? 'date' : 'text'}
                    aria-invalid={!!form.formState.errors[key]}
                    aria-describedby={help}
                    {...form.register(key)}
                  />
                )}
              </Field>
            ))}
            <div className="full-width">
              <Field
                label="Description"
                error={form.formState.errors.description?.message}
              >
                {(id, help) => (
                  <textarea
                    id={id}
                    aria-describedby={help}
                    rows={4}
                    {...form.register('description')}
                  />
                )}
              </Field>
            </div>
          </div>
        </fieldset>
        <Feedback error={mutation.error} />
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
            {mutation.isPending ? 'Saving…' : 'Save education'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
