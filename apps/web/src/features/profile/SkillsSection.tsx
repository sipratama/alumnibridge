import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { apiRequest, request } from '@/shared/api/client'
import {
  json,
  ownerPath,
  useOwnerMutation,
  useOwnerQuery,
  type Skill,
} from './api'
import { Feedback, Field, LoadError, Loading } from './components'

const normalize = (s: string) => s.trim().replace(/\s+/gu, ' ').toLowerCase()
export function SkillsSection() {
  const query = useOwnerQuery<Skill[]>('skills')
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  const [message, setMessage] = useState('')
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(search.trim()), 300)
    return () => clearTimeout(timer)
  }, [search])
  const valid = !!debounced && Array.from(debounced).length <= 80
  const results = useQuery({
    queryKey: ['skill-search', debounced],
    queryFn: ({ signal }) =>
      apiRequest<Skill[]>(
        `/skills?query=${encodeURIComponent(debounced)}&limit=10`,
        { signal },
      ),
    enabled: valid,
    retry: false,
  })
  const add = useOwnerMutation((name: string, token) =>
    request<Skill>(`${ownerPath}/skills`, token, json('POST', { name })),
  )
  const remove = useOwnerMutation((id: string, token) =>
    request<void>(
      `${ownerPath}/skills/${encodeURIComponent(id)}`,
      token,
      json('DELETE'),
    ),
  )
  const busy = add.isPending || remove.isPending
  const existing = new Set(query.data?.map((skill) => normalize(skill.name)))
  const duplicate = existing.has(normalize(search))
  async function choose(name: string) {
    if (existing.has(normalize(name))) return
    setMessage('')
    try {
      await add.mutateAsync(name)
      setSearch('')
      setMessage('Skill added.')
    } catch {
      /* Preserve search for retry. */
    }
  }
  return (
    <section className="profile-panel" aria-labelledby="skills-heading">
      <h2 id="skills-heading">Skills</h2>
      <p className="muted">
        Highlight what you bring. Choose a shared skill or add one by name.
      </p>
      {query.isPending ? (
        <Loading label="Loading skills…" />
      ) : query.error ? (
        <LoadError error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          {query.data.length > 0 ? (
            <ul className="skill-list">
              {query.data.map((skill) => (
                <li key={skill.id}>
                  <span>{skill.name}</span>
                  <button
                    className="icon-action"
                    aria-label={`Remove ${skill.name}`}
                    disabled={busy}
                    onClick={() => {
                      setMessage('')
                      void remove
                        .mutateAsync(skill.id)
                        .then(() => setMessage('Skill removed.'))
                        .catch(() => {})
                    }}
                  >
                    <X size={15} />
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="empty-copy">
              Your skills will appear here. Start with something you enjoy
              doing.
            </p>
          )}
          <form
            className="skill-search"
            onSubmit={(event) => {
              event.preventDefault()
              if (
                search.trim() &&
                !duplicate &&
                Array.from(search.trim()).length <= 80
              )
                void choose(search.trim())
            }}
          >
            <Field
              label="Search or add a skill"
              hint="Up to 100 skills, 80 characters per name."
            >
              {(id, help) => (
                <input
                  id={id}
                  value={search}
                  onChange={(event) => {
                    setSearch(event.target.value)
                    add.reset()
                  }}
                  aria-describedby={help}
                  disabled={busy || query.data.length >= 100}
                  autoComplete="off"
                />
              )}
            </Field>
            <Button
              disabled={
                busy ||
                !search.trim() ||
                Array.from(search.trim()).length > 80 ||
                duplicate ||
                query.data.length >= 100
              }
            >
              <Plus size={16} aria-hidden="true" />
              {add.isPending ? 'Adding…' : 'Add skill'}
            </Button>
          </form>
          {duplicate && (
            <p role="status" className="muted">
              This skill is already on your profile.
            </p>
          )}
          {query.data.length >= 100 && (
            <p role="status">You have reached the 100-skill limit.</p>
          )}
          {valid && debounced === search.trim() && (
            <div className="skill-results" aria-label="Available skills">
              {results.isFetching ? (
                <Loading label="Searching skills…" />
              ) : results.error ? (
                <p role="alert">
                  Skill search is unavailable. Try another search.
                </p>
              ) : (
                results.data
                  ?.filter((skill) => !existing.has(normalize(skill.name)))
                  .map((skill) => (
                    <button
                      key={skill.id}
                      disabled={busy}
                      className="skill-option"
                      onClick={() => void choose(skill.name)}
                    >
                      {skill.name}
                      <Plus size={14} aria-hidden="true" />
                    </button>
                  ))
              )}
            </div>
          )}
        </>
      )}
      <Feedback error={add.error || remove.error} message={message} />
    </section>
  )
}
