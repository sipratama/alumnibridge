import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Copy, ExternalLink, LockKeyhole } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ApiError, versionedRequest } from '@/shared/api/client'
import {
  json,
  ownerPath,
  privacyFields,
  useOwnerMutation,
  useOwnerQuery,
  type Privacy,
  type Profile,
} from './api'
import { Feedback, LoadError, Loading } from './components'
import { DiscoveryPanel } from '@/features/directory/DiscoveryPanel'

export function PrivacyPage() {
  const privacy = useOwnerQuery<Privacy>('profile/privacy')
  const profile = useOwnerQuery<Profile>('profile')
  if (privacy.isPending || profile.isPending)
    return <Loading label="Loading privacy settings…" />
  if (privacy.error || profile.error)
    return (
      <LoadError
        error={privacy.error || profile.error}
        retry={() => {
          void privacy.refetch()
          void profile.refetch()
        }}
      />
    )
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Privacy Settings</h1>
          <p>Choose what the world can see.</p>
        </div>
      </div>
      <PrivacyForm initial={privacy.data} slug={profile.data.slug} />
      <DiscoveryPanel visibility={privacy.data.visibility} />
    </>
  )
}
function PrivacyForm({ initial, slug }: { initial: Privacy; slug: string }) {
  const [values, setValues] = useState(initial)
  const [saved, setSaved] = useState(initial)
  const [consent, setConsent] = useState(false)
  const [message, setMessage] = useState('')
  const [validation, setValidation] = useState('')
  const mutation = useOwnerMutation(async (patch: Partial<Privacy>, token) => {
    if (!saved.__etag) throw new ApiError(428, 'revision_missing')
    // The validator belongs to the original displayed draft, not a preflight GET.
    return versionedRequest<Privacy>(
      `${ownerPath}/profile/privacy`,
      token,
      { ...json('PATCH', patch), headers: saved.__etag ? { 'If-Match': saved.__etag } : {} },
    )
  })
  const needsConsent =
    saved.visibility === 'private' && values.visibility === 'public'
  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setMessage('')
    if (needsConsent && !consent) {
      setValidation(
        'Confirm publication consent before making your profile public.',
      )
      return
    }
    setValidation('')
    const changed = Object.fromEntries(
      Object.keys(privacyFields)
        .filter(
          (key) => values[key as keyof Privacy] !== saved[key as keyof Privacy],
        )
        .map((key) => [key, values[key as keyof Privacy]]),
    )
    try {
      const result = await mutation.mutateAsync({
        visibility: values.visibility,
        ...changed,
      })
      setSaved(result)
      setValues(result)
      setConsent(false)
      setMessage(
        result.visibility === 'private'
          ? 'Profile is private. New anonymous requests cannot access it.'
          : 'Privacy settings saved. Use the public view to check what visitors can see.',
      )
    } catch {
      /* Preserve consent and choices for retry. */
    }
  }
  return (
    <div className="privacy-layout">
      <section className="profile-panel">
        <h2>Profile visibility</h2>
        <p className="muted">
          Public profiles share your display name and headline. Only the
          additional fields you select below are shared. Your email, academic
          identifiers and verification evidence are never included.
        </p>
        <form onSubmit={(event) => void submit(event)}>
          <fieldset disabled={mutation.isPending} className="form-fieldset">
            <fieldset className="visibility-options">
              <legend className="sr-only">Profile visibility</legend>
              <label>
                <input
                  type="radio"
                  name="visibility"
                  value="private"
                  checked={values.visibility === 'private'}
                  onChange={() => {
                    setValues({ ...values, visibility: 'private' })
                    setConsent(false)
                  }}
                />
                <span>
                  <strong>Private</strong>
                  <small>Only you can access your profile.</small>
                </span>
              </label>
              <label>
                <input
                  type="radio"
                  name="visibility"
                  value="public"
                  checked={values.visibility === 'public'}
                  onChange={() =>
                    setValues({ ...values, visibility: 'public' })
                  }
                />
                <span>
                  <strong>Public</strong>
                  <small>
                    Anyone with your link can view the fields you share.
                  </small>
                </span>
              </label>
            </fieldset>
            <h3>Additional public information</h3>
            <p className="muted">
              These choices only apply while your profile is public.
            </p>
            <div className="privacy-fields">
              {Object.entries(privacyFields).map(([key, label]) => (
                <label className="privacy-choice" key={key}>
                  <span>{label}</span>
                  <input
                    type="checkbox"
                    checked={values[key as keyof typeof privacyFields]}
                    onChange={(event) =>
                      setValues({ ...values, [key]: event.target.checked })
                    }
                  />
                </label>
              ))}
            </div>
            {needsConsent && (
              <label className="consent-box">
                <input
                  type="checkbox"
                  checked={consent}
                  onChange={(event) => {
                    setConsent(event.target.checked)
                    setValidation('')
                  }}
                />
                <span>
                  I consent to publishing my display name, headline and selected
                  fields so anyone can view them.
                </span>
              </label>
            )}
          </fieldset>
          {validation && (
            <p className="profile-error" role="alert">
              {validation}
            </p>
          )}
          {mutation.error instanceof ApiError &&
          (mutation.error.status === 412 || mutation.error.code === 'privacy_changed') ? (
            <p className="profile-error" role="alert">
              Privacy settings changed in another session. Your choices are
              preserved here. Reload before saving so an earlier publication
              choice is not restored accidentally.
            </p>
          ) : (
            <Feedback error={mutation.error} message={message} />
          )}
          <div className="form-actions">
            <Button disabled={mutation.isPending}>
              {mutation.isPending ? 'Saving…' : 'Save privacy settings'}
            </Button>
          </div>
        </form>
      </section>
      <aside>
        <section className="profile-panel public-link-panel">
          <LockKeyhole size={22} aria-hidden="true" />
          <h2>Your public view</h2>
          <p>
            Publication does not grant alumni verification. A moderation
            restriction can still make a public profile unavailable.
          </p>
          <code className="profile-address">/in/{slug}</code>
          {saved.visibility === 'public' ? (
            <>
              <Button variant="outline" asChild>
                <Link
                  to={`/in/${encodeURIComponent(slug)}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  View public profile <ExternalLink size={15} />
                </Link>
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  void (async () => {
                    try {
                      if (!navigator.clipboard)
                        throw new Error('Clipboard unavailable')
                      await navigator.clipboard.writeText(
                        `${window.location.origin}/in/${encodeURIComponent(slug)}`,
                      )
                      setMessage('Public profile link copied.')
                    } catch {
                      setMessage(
                        'Copy is unavailable. Use the address shown above.',
                      )
                    }
                  })()
                }}
              >
                <Copy size={15} aria-hidden="true" />
                Copy link
              </Button>
            </>
          ) : (
            <p>
              Your profile is private. Publish with explicit consent to enable
              the public view.
            </p>
          )}
          <p className="muted">
            Choosing Private stops future public reads. Previously downloaded
            content cannot be recalled. Consent has no separate revocation
            record; saving Private withdraws access.
          </p>
        </section>
      </aside>
    </div>
  )
}
