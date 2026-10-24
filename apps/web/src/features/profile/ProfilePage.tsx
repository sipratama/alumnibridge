import { usePhoto } from './usePhoto'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, LockKeyhole, MapPin, Pencil, Globe2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/authContext'
import { ApiError } from '@/shared/api/client'
import { useOwnerQuery, type Privacy, type Profile } from './api'
import { Feedback, LoadError, Loading, Verification } from './components'
import { ProfileEditor } from './ProfileEditor'
import { PhotoEditor } from './PhotoEditor'
import { HistorySection } from './HistorySection'
import { SkillsSection } from './SkillsSection'

export function ProfilePage() {
  const profile = useOwnerQuery<Profile>('profile')
  const privacy = useOwnerQuery<Privacy>('profile/privacy')
  const photo = usePhoto('/users/me/profile/photo', profile.dataUpdatedAt, true)
  const { user } = useAuth()
  const [editing, setEditing] = useState(false)
  const [saved, setSaved] = useState(false)
  if (profile.isPending) return <Loading />
  if (profile.error && (!profile.data || (profile.error instanceof ApiError && profile.error.status === 401)))
    return (
      <LoadError error={profile.error} retry={() => void profile.refetch()} />
    )
  const p = profile.data
  const verified = !!user?.memberships.some(
    (m) => m.role === 'alumni' && m.status === 'verified',
  )
  const pending = !!user?.memberships.some(
    (m) => m.role === 'alumni' && m.status === 'pending',
  )
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>My Profile</h1>
          <p>A professional story that grows with you.</p>
        </div>
        <Link className="text-link" to="/settings/privacy">
          <LockKeyhole size={16} aria-hidden="true" />
          Manage privacy
        </Link>
      </div>
      {profile.error && <p role="alert" className="profile-error">Profile could not be refreshed. Your open draft is preserved.</p>}
      <div className="profile-columns">
        <div className="profile-content">
          <section
            className="profile-panel profile-overview"
            aria-label="Professional profile"
          >
            <div className="profile-cover" />
            <div className="profile-intro">
              <div className="intro-top">
                <span className="visibility-label">
                  {privacy.data?.visibility === 'public' ? (
                    <>
                      <Globe2 size={14} />
                      Public
                    </>
                  ) : privacy.data ? (
                    <>
                      <LockKeyhole size={14} />
                      Private
                    </>
                  ) : (
                    'Checking visibility…'
                  )}
                </span>
                <Button
                  variant="outline"
                  onClick={() => {
                    setSaved(false)
                    setEditing(true)
                  }}
                >
                  <Pencil size={15} aria-hidden="true" />
                  Edit profile
                </Button>
              </div>
              <h2>{p.display_name}</h2>
              <p className="profile-headline">
                {p.headline || 'Add a headline that introduces what you do.'}
              </p>
              <Verification verified={verified} pending={pending} />
              {(p.current_job_title || p.current_company) && (
                <p className="current-role">
                  {[p.current_job_title, p.current_company]
                    .filter(Boolean)
                    .join(' at ')}
                </p>
              )}
              {p.location && (
                <p className="entry-meta">
                  <MapPin size={15} aria-hidden="true" />
                  {p.location}
                </p>
              )}
              <Feedback message={saved ? 'Profile saved.' : ''} />
              {privacy.error && <Feedback error={privacy.error} />}
              <PhotoEditor
                name={p.display_name}
                src={photo?.url}
                error={photo?.error}
                etag={p.__etag}
                reloadProfile={async () => {
                  const result = await profile.refetch()
                  if (result.error) throw result.error
                  return result.data
                }}
              />
            </div>
          </section>
          <section className="profile-panel">
            <h2>About</h2>
            {p.bio ? (
              <p className="prose-text">{p.bio}</p>
            ) : (
              <p className="empty-copy">
                What interests you? What are you working toward? Introduce
                yourself by editing your profile.
              </p>
            )}
          </section>
          <HistorySection kind="experiences" />
          <HistorySection kind="educations" />
          <SkillsSection />
        </div>
        <aside className="profile-aside">
          <section className="completeness-panel">
            <h2>Make your story complete</h2>
            <div className="completeness-value">
              {p.completeness}
              <span>% complete</span>
            </div>
            <progress
              max={100}
              value={p.completeness}
              aria-label="Profile completeness"
            />
            <p>
              Nine details help tell your story: your introduction, current
              role, experience, education and skills.
            </p>
            <p>Completeness does not affect visibility or verification.</p>
          </section>
          <section className="aside-note">
            <LockKeyhole size={22} aria-hidden="true" />
            <h2>You're in control</h2>
            <p>
              Your profile starts private. Choose exactly what to share before
              publishing.
            </p>
            <Link to="/settings/privacy" className="text-link">
              Privacy settings <ArrowRight size={16} aria-hidden="true" />
            </Link>
          </section>
        </aside>
      </div>
      {editing && (
        <ProfileEditor
          profile={p}
          onClose={() => setEditing(false)}
          onSaved={() => {
            setEditing(false)
            setSaved(true)
          }}
        />
      )}
    </>
  )
}
export function MemberHome() {
  const { user } = useAuth()
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Welcome, {user?.display_name || 'member'}.</h1>
          <p>A shared beginning. A story of your own.</p>
        </div>
      </div>
      <section className="profile-panel home-profile">
        <h2>Build your professional profile</h2>
        <p>
          Bring your experiences, education and skills together. Keep it for
          yourself or choose what to share publicly.
        </p>
        <Button asChild>
          <Link to="/profile">
            Go to My Profile <ArrowRight size={17} />
          </Link>
        </Button>
      </section>
      <section className="profile-panel">
        <h2>Your community membership</h2>
        <p>
          Professional information and alumni verification are managed
          separately.
        </p>
        <Link className="text-link" to="/account">
          View your account <ArrowRight size={16} />
        </Link>
      </section>
    </>
  )
}
