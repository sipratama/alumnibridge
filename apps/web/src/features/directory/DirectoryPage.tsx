import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, GraduationCap, Search, ShieldCheck, SlidersHorizontal } from 'lucide-react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/app/authContext'
import { Button } from '@/components/ui/button'
import { Brand } from '@/features/profile/components'
import {
  controlledPhotoURL, defaultFilters, DirectoryError, parseFilters,
  publicProfilePath, searchDirectory, serializeFilters, validateFilters,
  type DirectoryEntry, type DirectoryFilters,
} from './api'
import './directory.css'

type PageState = { signature: string; cursors: Array<string | null>; index: number }

export function DirectoryPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const resultsHeading = useRef<HTMLHeadingElement>(null)
  const { status, login } = useAuth()
  const parsed = parseFilters(location.search)
  const signature = serializeFilters(parsed.filters)
  const [pageState, setPageState] = useState<PageState>({ signature, cursors: [null], index: 0 })
  const current = pageState.signature === signature ? pageState : { signature, cursors: [null], index: 0 }
  const cursor = current.cursors[current.index]
  const query = useQuery({
    queryKey: ['directory', signature, cursor],
    queryFn: ({ signal }) => searchDirectory(parsed.filters, cursor, signal),
    enabled: !parsed.error,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
  })
  function applyFilters(filters: DirectoryFilters) {
    const next = serializeFilters(filters)
    setPageState({ signature: next, cursors: [null], index: 0 })
    navigate(next ? `/alumni?${next}` : '/alumni')
    requestAnimationFrame(() => resultsHeading.current?.focus())
  }
  function nextPage() {
    if (!query.data?.next_cursor) return
    setPageState({ signature, cursors: [...current.cursors.slice(0, current.index + 1), query.data.next_cursor], index: current.index + 1 })
    requestAnimationFrame(() => resultsHeading.current?.focus())
    window.scrollTo?.({ top: 0, behavior: 'smooth' })
  }
  function previousPage() {
    if (current.index > 0) {
      setPageState({ ...current, index: current.index - 1 })
      requestAnimationFrame(() => resultsHeading.current?.focus())
    }
  }
  return (
    <div className="directory-page">
      <a className="skip-link" href="#directory-main">Skip to directory</a>
      <header className="member-header directory-header">
        <div className="container">
          <Brand />
          <nav aria-label="Directory navigation">
            <Link to="/">Home</Link>
            {status === 'authenticated' ? <Link to="/profile">My Profile</Link> :
              <Button variant="outline" onClick={() => void login()} disabled={status === 'loading'}>Sign in</Button>}
          </nav>
        </div>
      </header>
      <main id="directory-main" className="container directory-main">
        <section className="directory-intro" aria-labelledby="directory-title">
          <span className="section-kicker">ALUMNI DIRECTORY</span>
          <h1 id="directory-title">Find alumni. <em>Explore their paths.</em></h1>
          <p>Explore professional profiles shared by verified alumni. Members choose whether they appear here and which details they publish.</p>
        </section>
        <div className="directory-layout">
          <DirectoryFiltersForm key={location.search} initial={parsed.error ? defaultFilters : parsed.filters} onApply={applyFilters} />
          <section className="directory-results" aria-labelledby="directory-results-title" aria-busy={query.isFetching}>
            <div className="directory-results-heading">
              <div>
                <span className="section-kicker">PEOPLE & PROFESSIONS</span>
                <h2 id="directory-results-title" ref={resultsHeading} tabIndex={-1}>Alumni profiles</h2>
              </div>
              <p>Names and professional fields match from the beginning. Skills match exact names.</p>
            </div>
            {parsed.error ? <div className="directory-state" role="alert"><h3>Search URL needs attention</h3><p>{parsed.error}</p><Button onClick={() => applyFilters(defaultFilters)}>Clear invalid filters</Button></div> :
              query.isPending ? <div className="directory-state" role="status"><h3>Finding alumni…</h3><p>Checking the latest published profiles.</p></div> :
              query.error ? <DirectoryFailure error={query.error} onRetry={() => void query.refetch()} onReset={() => setPageState({ signature, cursors: [null], index: 0 })} hasCursor={!!cursor} /> :
              query.data.items.length === 0 ? <div className="directory-state" role="status"><h3>{current.index > 0 ? 'This page has changed' : 'No profiles match these filters'}</h3><p>{current.index > 0 ? 'Profiles may have been removed from search since your last page. Return to an earlier page to see current results.' : 'Try a shorter prefix, fewer skills or a different academic reference slug. Only published information can be searched.'}</p>{current.index > 0 ? <Button variant="outline" onClick={() => setPageState({ signature, cursors: [null], index: 0 })}>Back to first page</Button> : <Button variant="outline" onClick={() => applyFilters(defaultFilters)}>Clear filters</Button>}</div> : <>
                <p className="directory-count" role="status">Showing {query.data.items.length} {query.data.items.length === 1 ? 'profile' : 'profiles'} on page {current.index + 1}.</p>
                <div className="directory-cards">{query.data.items.map((entry) => <DirectoryCard key={entry.slug} entry={entry} />)}</div>
                <nav className="directory-pagination" aria-label="Directory pages">
                  <Button variant="outline" disabled={current.index === 0 || query.isFetching} onClick={previousPage}>Previous</Button>
                  <span>Page {current.index + 1}</span>
                  <Button variant="outline" disabled={!query.data.next_cursor || query.isFetching} onClick={nextPage}>Next</Button>
                </nav>
                <p className="directory-page-note">Results reflect current publication choices. Pages can change when members update their profiles.</p>
              </>}
          </section>
        </div>
      </main>
    </div>
  )
}

function DirectoryFiltersForm({ initial, onApply }: { initial: DirectoryFilters; onApply: (filters: DirectoryFilters) => void }) {
  const [values, setValues] = useState<DirectoryFilters>(initial)
  const [skillInput, setSkillInput] = useState(initial.skills.join(', '))
  const [error, setError] = useState('')
  const [mobileOpen, setMobileOpen] = useState(false)
  function set<K extends keyof DirectoryFilters>(key: K, value: DirectoryFilters[K]) {
    setValues((old) => ({ ...old, [key]: value, ...(key === 'institution' && !value ? { study_program: '' } : {}) }))
  }
  function submit(event: React.FormEvent) {
    event.preventDefault()
    const filters = { ...values, skills: skillInput.split(',').map((s) => s.trim()).filter(Boolean) }
    const issue = validateFilters(filters)
    if (issue) { setError(issue); return }
    setError('')
    setMobileOpen(false)
    onApply(filters)
  }
  return <aside className="directory-filter-panel" aria-label="Search filters">
    <button className="directory-filter-toggle" type="button" aria-expanded={mobileOpen} aria-controls="directory-filter-form" onClick={() => setMobileOpen(!mobileOpen)}><SlidersHorizontal size={18} aria-hidden="true" /> Search and filters</button>
    <form id="directory-filter-form" className={mobileOpen ? 'is-open' : ''} onSubmit={submit}>
      <div className="directory-filter-title"><Search size={20} aria-hidden="true" /><h2>Refine your search</h2></div>
      <p>Search published details only. Prefix matches are literal, including % and _.</p>
      <label>Name prefix<input value={values.query} onChange={(event) => set('query', event.target.value)} maxLength={100} placeholder="e.g. Siti" /></label>
      <label>Current company<input value={values.company} onChange={(event) => set('company', event.target.value)} maxLength={160} placeholder="e.g. Acme" /></label>
      <label>Current job title<input value={values.job_title} onChange={(event) => set('job_title', event.target.value)} maxLength={160} placeholder="e.g. Teacher" /></label>
      <div className="directory-filter-divider">Academic affiliation</div>
      <p>Enter exact public reference slugs, such as a slug from a published alumni affiliation. Institution and program must belong together.</p>
      <label>Institution slug<input value={values.institution} onChange={(event) => set('institution', event.target.value)} maxLength={64} placeholder="e.g. uhamka" /></label>
      <label>Study program slug<input value={values.study_program} onChange={(event) => set('study_program', event.target.value)} maxLength={64} disabled={!values.institution} placeholder="Requires institution" /></label>
      <label>Graduation year<input type="number" min="1950" max="2100" value={values.graduation_year} onChange={(event) => set('graduation_year', event.target.value)} placeholder="e.g. 2020" /></label>
      <div className="directory-filter-divider">Skills</div>
      <label>Skill names, separated by commas<input value={skillInput} onChange={(event) => setSkillInput(event.target.value)} placeholder="e.g. teaching, research" aria-describedby="directory-skill-hint" /></label>
      <p id="directory-skill-hint">Up to five exact normalized skill names. All selected skills must match.</p>
      <label>Profiles per page<input type="number" min="1" max="50" value={values.limit} onChange={(event) => set('limit', Number(event.target.value))} /></label>
      {error && <p className="directory-form-error" role="alert">{error}</p>}
      <div className="directory-filter-actions"><Button type="submit">Search alumni <ArrowRight size={16} aria-hidden="true" /></Button><Button type="button" variant="outline" onClick={() => { setValues(defaultFilters); setSkillInput(''); setError(''); onApply(defaultFilters) }}>Clear filters</Button></div>
    </form>
  </aside>
}

function DirectoryFailure({ error, onRetry, onReset, hasCursor }: { error: unknown; onRetry: () => void; onReset: () => void; hasCursor: boolean }) {
  const status = error instanceof DirectoryError ? error.status : 0
  const code = error instanceof DirectoryError ? error.code : ''
  const title = status === 429 ? 'Search limit reached' : status === 503 ? 'Directory is busy' : status === 400 ? 'Search could not be continued' : 'Directory is unavailable'
  const detail = status === 429 ? `Please wait${error instanceof DirectoryError && error.retryAfter ? ` about ${error.retryAfter} seconds` : ' a moment'} before trying again.` :
    status === 503 ? code === 'directory_database_timeout' || code === 'directory_timeout' ? 'The search took too long. Try narrower filters or retry shortly.' : 'Too many searches are running. Please retry shortly.' :
      status === 400 && hasCursor ? 'This page link expired or no longer matches your filters. Start from the first page.' :
        status === 400 ? 'Review your filters and submit a new search.' : 'Check your connection and try again.'
  return <div className="directory-state" role="alert"><h3>{title}</h3><p>{detail}</p>{status === 400 && hasCursor ? <Button onClick={onReset}>Back to first page</Button> : <Button variant="outline" onClick={onRetry}>Try again</Button>}</div>
}

function DirectoryCard({ entry }: { entry: DirectoryEntry }) {
  const [brokenPhoto, setBrokenPhoto] = useState(false)
  const photo = controlledPhotoURL(entry)
  return <article className="directory-card">
    <div className="directory-card-top">
      {photo && !brokenPhoto ? <img className="directory-avatar" src={photo} alt="" loading="lazy" onError={() => setBrokenPhoto(true)} /> : <span className="directory-avatar directory-avatar-fallback" aria-hidden="true">{entry.display_name.trim().slice(0, 1).toUpperCase()}</span>}
      <div><h3><Link to={publicProfilePath(entry.slug)}>{entry.display_name}</Link></h3>{entry.verified_alumni && <span className="directory-verified"><ShieldCheck size={15} aria-hidden="true" /> Verified alumni</span>}</div>
    </div>
    {entry.headline && <p className="directory-headline">{entry.headline}</p>}
    {(entry.current_job_title || entry.current_company) && <p className="directory-role">{[entry.current_job_title, entry.current_company].filter(Boolean).join(' at ')}</p>}
    {!!entry.affiliations?.length && <ul className="directory-affiliations">{entry.affiliations.map((affiliation, index) => <li key={`${affiliation.institution.slug}-${affiliation.study_program?.slug ?? ''}-${index}`}><GraduationCap size={15} aria-hidden="true" />{[affiliation.institution.name, affiliation.study_program?.name, affiliation.graduation_year].filter(Boolean).join(' · ')}</li>)}</ul>}
    {!!entry.skills?.length && <ul className="directory-skills" aria-label="Published skills">{entry.skills.map((skill) => <li key={skill}>{skill}</li>)}</ul>}
    <Link className="directory-card-link" to={publicProfilePath(entry.slug)} aria-label={`View ${entry.display_name}'s profile`}>View profile <ArrowRight size={16} aria-hidden="true" /></Link>
  </article>
}
