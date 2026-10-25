import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/authContext'
import { ApiError, request } from '@/shared/api/client'
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
import { ConflictReview, type ConflictField } from './ConflictReview'

const experienceFields: Array<[keyof ExperienceInput, string]> = [
  ['company', 'Company'], ['job_title', 'Job title'], ['employment_type', 'Employment type'],
  ['location', 'Location'], ['start_date', 'Start date'], ['end_date', 'End date'],
  ['is_current', 'Current role'], ['description', 'Description'],
]
const educationFields: Array<[keyof EducationInput, string]> = [
  ['institution', 'Institution'], ['degree', 'Degree'], ['field_of_study', 'Field of study'],
  ['start_date', 'Start date'], ['end_date', 'End date'], ['description', 'Description'],
]
function experienceValues(item?: Experience) {
  return {
    company: item?.company ?? '', job_title: item?.job_title ?? '',
    employment_type: item?.employment_type ?? '', location: item?.location ?? '',
    start_date: item?.start_date ?? '', end_date: item?.end_date ?? '',
    is_current: item?.is_current ?? false, description: item?.description ?? '',
  }
}
function educationValues(item?: Education) {
  return {
    institution: item?.institution ?? '', degree: item?.degree ?? '',
    field_of_study: item?.field_of_study ?? '', start_date: item?.start_date ?? '',
    end_date: item?.end_date ?? '', description: item?.description ?? '',
  }
}

export function HistorySection({
  kind,
}: {
  kind: 'experiences' | 'educations'
}) {
  const isWork = kind === 'experiences'
  const title = isWork ? 'Experience' : 'Education'
  const singular = isWork ? 'experience' : 'education'
  const { getToken, refreshUser } = useAuth()
  const query = useOwnerQuery<(Experience | Education)[]>(kind)
  const [editing, setEditing] = useState<Experience | Education | 'new'>()
  const [removing, setRemoving] = useState<Experience | Education>()
  const [message, setMessage] = useState('')
  const [latestRemoval, setLatestRemoval] = useState<Experience | Education | null>()
  const [approvedRemoval, setApprovedRemoval] = useState<Experience | Education>()
  const [removalReloadError, setRemovalReloadError] = useState(false)
  const remove = useOwnerMutation((item: Experience | Education, token) =>
    request<void>(
      `${ownerPath}/${kind}/${encodeURIComponent(item.id)}`,
      token,
      { ...json('DELETE'), headers: { 'If-Match': '"ab-' + (isWork ? 'experience' : 'education') + '-' + item.id + '-v' + item.revision + '"' } },
    ),
  )
  async function reloadRemoval() {
    if (!removing) return
    try {
      const items = await request<(Experience | Education)[]>(`${ownerPath}/${kind}`, getToken)
      setLatestRemoval(items.find((item) => item.id === removing.id) ?? null)
      setApprovedRemoval(undefined)
      setRemovalReloadError(false)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) void refreshUser()
      setRemovalReloadError(true)
    }
  }
  async function confirmRemoval() {
    if (!removing) return
    try {
      await remove.mutateAsync(approvedRemoval ?? removing)
      setRemoving(undefined)
      setLatestRemoval(undefined)
      setApprovedRemoval(undefined)
      setMessage(`${title} removed.`)
    } catch (error) {
      if (error instanceof ApiError && error.status === 412) void reloadRemoval()
    }
  }
  if (query.error instanceof ApiError && query.error.status === 401)
    return <LoadError error={query.error} retry={() => void query.refetch()} />
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
      ) : query.error && (!query.data || (query.error instanceof ApiError && query.error.status === 401)) ? (
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
                  setLatestRemoval(undefined)
                  setApprovedRemoval(undefined)
                  setRemovalReloadError(false)
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
      {query.error && query.data && !(query.error instanceof ApiError && query.error.status === 401) && <p role="alert" className="profile-error">{title} could not be refreshed. Your open draft is preserved.</p>}
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
          onClose={() => { setRemoving(undefined); setLatestRemoval(undefined); setApprovedRemoval(undefined) }}
          onConfirm={() => void confirmRemoval()}
          confirmDisabled={latestRemoval !== undefined && !approvedRemoval}
        >
          {latestRemoval === null && <p role="alert">This entry was already removed. Your original selection remains unchanged.</p>}
          {latestRemoval && !approvedRemoval && <div className="conflict-review">
            <h3>Review the current entry before deleting</h3>
            <dl>{Object.entries(removing).filter(([key]) => key !== 'id' && key !== 'revision').map(([key, original]) => (
              <div key={key}><dt>{key.replaceAll('_', ' ')}</dt><dd>Original: {String(original ?? '(empty)')} · Current: {String((latestRemoval as unknown as Record<string, unknown>)[key] ?? '(empty)')}</dd></div>
            ))}</dl>
            <Button type="button" variant="outline" onClick={() => { setApprovedRemoval(latestRemoval); remove.reset() }}>Use current entry revision for deletion</Button>
          </div>}
          {approvedRemoval && <p role="status">Current entry reviewed. Choose Remove entry to delete this version.</p>}
          {removalReloadError && <p role="alert">Current entry could not be loaded. Your deletion selection is preserved.</p>}
          <Button type="button" variant="outline" onClick={() => void reloadRemoval()}>Reload current entry</Button>
        </ConfirmDelete>
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
  const { getToken, refreshUser } = useAuth()
  const [base, setBase] = useState(item)
  const [latest, setLatest] = useState<Experience | null>()
  const [reloadError, setReloadError] = useState(false)
  const form = useForm({
    resolver: zodResolver(experienceSchema),
    defaultValues: experienceValues(item),
  })
  const draft = useWatch({ control: form.control })
  const current = draft.is_current
  const mutation = useOwnerMutation((body: ExperienceInput, token) =>
    request<Experience>(
      `${ownerPath}/experiences${base ? `/${encodeURIComponent(base.id)}` : ''}`,
      token,
      { ...json(base ? 'PATCH' : 'POST', body), headers: base ? { 'If-Match': '"ab-experience-' + base.id + '-v' + base.revision + '"' } : {} },
    ),
  )
  async function reloadLatest() {
    if (!base) return
    try {
      const items = await request<Experience[]>(`${ownerPath}/experiences`, getToken)
      setLatest(items.find((entry) => entry.id === base.id) ?? null)
      setReloadError(false)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) void refreshUser()
      setReloadError(true)
    }
  }
  const conflicts: ConflictField[] = base && latest ? experienceFields
    .filter(([key]) => draft[key] !== experienceValues(base)[key])
    .map(([key, label]) => ({ key, label, original: experienceValues(base)[key], current: experienceValues(latest)[key], draft: draft[key] })) : []
  function applySelected(selected: Set<string>) {
    if (!latest) return
    const currentDraft = form.getValues()
    const merged = experienceValues(latest)
    for (const [key] of experienceFields) {
      if (selected.has(key)) (merged as Record<string, unknown>)[key] = currentDraft[key]
    }
    setBase(latest)
    form.reset(merged)
    setLatest(undefined)
    mutation.reset()
  }
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
          } catch (error) {
            /* Keep the form available for retry. */
            if (error instanceof ApiError && error.status === 412) void reloadLatest()
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
        {latest && <ConflictReview key={latest.revision} fields={conflicts} onApply={applySelected} />}
        {latest === null && <p role="alert">This experience was removed in another session. Your draft is preserved; close and add a new entry if needed.</p>}
        {reloadError && <p role="alert">Current experience could not be loaded. Your draft is preserved.</p>}
        <div className="form-actions">
          {base && <Button type="button" variant="outline" onClick={() => void reloadLatest()}>Reload current experience</Button>}
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button disabled={mutation.isPending || latest !== undefined}>
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
  const { getToken, refreshUser } = useAuth()
  const [base, setBase] = useState(item)
  const [latest, setLatest] = useState<Education | null>()
  const [reloadError, setReloadError] = useState(false)
  const form = useForm({
    resolver: zodResolver(educationSchema),
    defaultValues: educationValues(item),
  })
  const draft = useWatch({ control: form.control })
  const mutation = useOwnerMutation((body: EducationInput, token) =>
    request<Education>(
      `${ownerPath}/educations${base ? `/${encodeURIComponent(base.id)}` : ''}`,
      token,
      { ...json(base ? 'PATCH' : 'POST', body), headers: base ? { 'If-Match': '"ab-education-' + base.id + '-v' + base.revision + '"' } : {} },
    ),
  )
  async function reloadLatest() {
    if (!base) return
    try {
      const items = await request<Education[]>(`${ownerPath}/educations`, getToken)
      setLatest(items.find((entry) => entry.id === base.id) ?? null)
      setReloadError(false)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) void refreshUser()
      setReloadError(true)
    }
  }
  const conflicts: ConflictField[] = base && latest ? educationFields
    .filter(([key]) => draft[key] !== educationValues(base)[key])
    .map(([key, label]) => ({ key, label, original: educationValues(base)[key], current: educationValues(latest)[key], draft: draft[key] })) : []
  function applySelected(selected: Set<string>) {
    if (!latest) return
    const currentDraft = form.getValues()
    const merged = educationValues(latest)
    for (const [key] of educationFields) {
      if (selected.has(key)) (merged as Record<string, unknown>)[key] = currentDraft[key]
    }
    setBase(latest)
    form.reset(merged)
    setLatest(undefined)
    mutation.reset()
  }
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
          } catch (error) {
            /* Keep the form available for retry. */
            if (error instanceof ApiError && error.status === 412) void reloadLatest()
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
        {latest && <ConflictReview key={latest.revision} fields={conflicts} onApply={applySelected} />}
        {latest === null && <p role="alert">This education was removed in another session. Your draft is preserved; close and add a new entry if needed.</p>}
        {reloadError && <p role="alert">Current education could not be loaded. Your draft is preserved.</p>}
        <div className="form-actions">
          {base && <Button type="button" variant="outline" onClick={() => void reloadLatest()}>Reload current education</Button>}
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button disabled={mutation.isPending || latest !== undefined}>
            {mutation.isPending ? 'Saving…' : 'Save education'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
