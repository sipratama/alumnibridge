import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@/app/authContext'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/shared/api/client'
import { useOwnerQuery } from '@/features/profile/api'
import { LoadError, Loading } from '@/features/profile/components'
import {
  updateDiscovery,
  type AcademicConsent, type DiscoveryPatch, type DiscoverySettings,
} from './api'

function choices(settings: DiscoverySettings): AcademicConsent[] {
  return settings.memberships.map(({ membership_id, show_institution, show_study_program, show_graduation_year }) => ({ membership_id, show_institution, show_study_program, show_graduation_year }))
}

export function DiscoveryPanel({ visibility }: { visibility: 'public' | 'private' }) {
  const query = useOwnerQuery<DiscoverySettings>('profile/discovery')
  const [reloadKey, setReloadKey] = useState(0)
  if (query.isPending) return <Loading label="Loading directory discovery settings…" />
  if (query.error && (!query.data || (query.error instanceof ApiError && query.error.status === 401))) return <LoadError error={query.error} retry={() => void query.refetch()} />
  return <>{query.error && <p className="profile-error" role="alert">Settings could not be refreshed. Your choices are preserved; retry before saving.</p>}<DiscoveryForm key={reloadKey} initial={query.data} visibility={visibility} onReload={() => { void query.refetch().then((result) => { if (result.data && !result.error) setReloadKey((key) => key + 1) }) }} /></>
}

function DiscoveryForm({ initial, visibility, onReload }: { initial: DiscoverySettings; visibility: 'public' | 'private'; onReload: () => void }) {
  const { getToken, user } = useAuth()
  const client = useQueryClient()
  const [draft, setDraft] = useState({ origin: initial, enabled: initial.directory_discoverable, affiliations: choices(initial) })
  // Keep the snapshot, including its original ETag, until an explicit reload
  // or a successful save. Background refetch must never silently rewrite a draft.
  const active = draft
  const { enabled, affiliations } = active
  const saved = draft.origin
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [message, setMessage] = useState('')
  const canOptIn = saved.eligible && visibility === 'public'
  function setFlag(id: string, flag: keyof Omit<AcademicConsent, 'membership_id'>, checked: boolean) {
    setDraft({ ...active, affiliations: affiliations.map((item) => item.membership_id === id ? {
      ...item, [flag]: checked,
      ...(flag === 'show_institution' && !checked ? { show_study_program: false, show_graduation_year: false } : {}),
    } : item) })
  }
  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (busy) return
    setError(null)
    setMessage('')
    const optingOut = saved.directory_discoverable && !enabled
    if (optingOut && !window.confirm('Remove your directory listing and all academic disclosure choices?')) return
    const changedFlags = JSON.stringify(affiliations) !== JSON.stringify(choices(saved))
    const patch: DiscoveryPatch = {}
    if (enabled !== saved.directory_discoverable) patch.directory_discoverable = enabled
    if (changedFlags && !optingOut) patch.affiliations = affiliations.filter((item) => item.show_institution)
    if (!Object.keys(patch).length) { setMessage('No changes to save.'); return }
    setBusy(true)
    try {
      // Compare-and-set is enforced in PostgreSQL. Never fetch a newer ETag
      // immediately before saving an older draft.
      if (!saved.__etag) throw new ApiError(428, 'revision_missing')
      const result = await updateDiscovery(getToken, patch, saved.__etag)
      setDraft({ origin: result, enabled: result.directory_discoverable, affiliations: choices(result) })
      client.setQueryData(['professional', user?.id, 'profile/discovery'], result)
      setMessage(result.effective_discoverable ? 'Your profile can now appear in Alumni Directory.' : 'Directory discovery is off. Your academic choices are no longer published here.')
      client.removeQueries({ queryKey: ['directory'] })
      client.removeQueries({ queryKey: ['public-profile'] })
      await client.invalidateQueries({ queryKey: ['professional', user?.id, 'profile/discovery'] })
    } catch (failure) { setError(failure) } finally { setBusy(false) }
  }
  const errorText = error instanceof ApiError ?
    error.status === 412 || error.code === 'discovery_changed' ? 'Discovery settings changed in another session. Your draft is preserved. Reload and review before saving again.' :
      error.status === 428 ? 'The revision is missing. Reload your settings before saving.' :
      error.status === 401 ? 'Your session expired. Sign in again to update discovery.' :
        error.status === 404 ? 'Your profile or membership is no longer available. Reload this page.' :
          error.status === 409 ? 'Your current profile or verification status does not allow this choice. Reload your settings.' :
            error.status === 413 ? 'Too many affiliations were submitted.' :
              error.status === 415 ? 'The request format was rejected. Reload and try again.' :
                error.status === 400 ? 'Review your academic disclosure choices and try again.' : 'Discovery settings could not be saved. Try again.' :
    'Could not reach AlumniBridge. Check your connection and try again.'
  return <section className="profile-panel discovery-panel" aria-labelledby="discovery-heading">
    <h2 id="discovery-heading">Alumni Directory discovery</h2>
    <p className="muted">A public profile link and an Alumni Directory listing are separate choices. Only a public, visible profile with a trusted verified alumni membership can be listed.</p>
    <p className="discovery-status" role="status"><strong>Status:</strong> {saved.effective_discoverable ? 'Listed when public search matches' : 'Not listed in Alumni Directory'}</p>
    {!saved.eligible && <p className="discovery-ineligible">{visibility === 'private' ? 'Publish your profile first if you want to request listing. Publication alone does not turn on discovery.' : 'Directory listing requires a verified alumni membership and a visible public profile. Student and pending alumni memberships are not eligible.'}</p>}
    <form onSubmit={(event) => void save(event)}>
      <fieldset disabled={busy} className="form-fieldset">
        <label className="privacy-choice discovery-choice"><span><strong>Include my profile in Alumni Directory</strong><small>Explicit opt-in. You can withdraw this choice at any time.</small></span><input type="checkbox" checked={enabled} disabled={!canOptIn && !enabled} onChange={(event) => { setDraft({ ...active, enabled: event.target.checked }) }} /></label>
        <h3>Academic details in Directory</h3>
        <p className="muted">Choose what to publish for each verified alumni membership. These choices do not change professional field visibility. Institution is required before study program or graduation year.</p>
        {saved.memberships.length === 0 ? <p>No verified alumni memberships are available for academic disclosure.</p> :
          saved.memberships.map((membership) => {
            const current = affiliations.find((item) => item.membership_id === membership.membership_id)
            if (!current) return null
            return <fieldset className="discovery-membership" key={membership.membership_id} disabled={!enabled || busy}>
              <legend>{membership.institution} · {membership.study_program} · {membership.graduation_year}</legend>
              <label><input type="checkbox" checked={current.show_institution} onChange={(event) => setFlag(current.membership_id, 'show_institution', event.target.checked)} /> Show institution</label>
              <label><input type="checkbox" checked={current.show_study_program} disabled={!current.show_institution} onChange={(event) => setFlag(current.membership_id, 'show_study_program', event.target.checked)} /> Show study program</label>
              <label><input type="checkbox" checked={current.show_graduation_year} disabled={!current.show_institution} onChange={(event) => setFlag(current.membership_id, 'show_graduation_year', event.target.checked)} /> Show graduation year</label>
            </fieldset>
          })}
      </fieldset>
      {error !== null && <p className="profile-error" role="alert">{errorText}</p>}
      {message && <p className="profile-success" role="status">{message}</p>}
      <div className="form-actions"><Button disabled={busy}>{busy ? 'Saving…' : 'Save discovery settings'}</Button><Button type="button" variant="outline" disabled={busy} onClick={onReload}>Reload settings</Button></div>
    </form>
  </section>
}
