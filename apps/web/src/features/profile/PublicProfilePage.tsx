import { usePhoto } from './usePhoto'
import { useEffect, useLayoutEffect, useRef } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { MapPin } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/authContext'
import { ApiError } from '@/shared/api/client'
import { usePublicProfile, type PublicProfile } from './api'
import {
  Avatar,
  Brand,
  EducationList,
  ExperienceList,
  Loading,
  Verification,
} from './components'

export function PublicProfilePage() {
  const { slug = '' } = useParams()
  const query = usePublicProfile(slug)
  const navigate = useNavigate()
  const { status, login } = useAuth()
  const canonical = query.data?.slug
  useEffect(() => {
    if (
      canonical &&
      canonical !== slug &&
      /^[a-z0-9]+(-[a-z0-9]+)*$/.test(canonical)
    )
      navigate(`/in/${encodeURIComponent(canonical)}`, { replace: true })
  }, [canonical, slug, navigate])
  const story = useRef<HTMLDivElement>(null)
  const revalidating = query.isFetching && !!query.data
  useLayoutEffect(() => {
    const element = story.current
    if (!element || query.isFetching || query.error) return
    const measure = () => {
      element.style.setProperty(
        '--public-story-height',
        `${element.getBoundingClientRect().height}px`,
      )
    }
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [query.dataUpdatedAt, query.isFetching, query.error])
  return (
    <div className="public-profile-page">
      <a className="skip-link" href="#public-main">
        Skip to profile
      </a>
      <header className="member-header">
        <div className="container">
          <Brand />
          {status === 'authenticated' ? (
            <Link className="text-link" to="/profile">
              My Profile
            </Link>
          ) : (
            <Button
              variant="outline"
              onClick={() => void login()}
              disabled={status === 'loading'}
            >
              Sign in
            </Button>
          )}
        </div>
      </header>
      <main id="public-main" className="public-profile-main">
        <div
          ref={story}
          className={
            revalidating ? 'public-story is-revalidating' : 'public-story'
          }
          aria-busy={query.isFetching}
        >
          {revalidating ? (
            <div className="public-revalidation" aria-hidden="true" />
          ) : query.isPending || (canonical && canonical !== slug) ? (
            <Loading label="Loading public profile…" />
          ) : query.error ? (
            <section className="profile-panel unavailable-profile">
              <h1>
                {query.error instanceof ApiError && query.error.status === 404
                  ? 'Profile unavailable'
                  : 'Unable to load this profile'}
              </h1>
              <p>
                {query.error instanceof ApiError && query.error.status === 404
                  ? 'This profile may be private, unavailable or no longer exist.'
                  : 'Please check your connection and try again.'}
              </p>
              <Button variant="outline" onClick={() => void query.refetch()}>
                Try again
              </Button>
              <Link className="text-link" to="/">
                Back to AlumniBridge
              </Link>
            </section>
          ) : (
            <PublicContent
              profile={query.data}
              revision={query.dataUpdatedAt}
            />
          )}
        </div>
      </main>
      <footer className="public-profile-footer">
        Shared through AlumniBridge. Visibility and fields are chosen by the
        profile owner.
      </footer>
    </div>
  )
}
function PublicContent({
  profile: p,
  revision,
}: {
  profile: PublicProfile
  revision: number
}) {
  // Build only the controlled delivery path. Never render an arbitrary URL supplied as profile text.
  const photo = usePhoto(
    p.photo_url ? `/profiles/${encodeURIComponent(p.slug)}/photo` : undefined,
    revision,
  )
  return (
    <>
      <section className="profile-panel public-overview">
        <div className="profile-cover" />
        <div className="public-intro">
          <Avatar name={p.display_name} src={photo?.url} />
          <h1>{p.display_name}</h1>
          {p.headline && <p className="profile-headline">{p.headline}</p>}
          <Verification verified={p.verified_alumni} />
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
          {!!photo?.error && (
            <p role="status" className="muted">
              Profile photo is temporarily unavailable.
            </p>
          )}
        </div>
      </section>
      {p.bio && (
        <section className="profile-panel">
          <h2>About</h2>
          <p className="prose-text">{p.bio}</p>
        </section>
      )}
      {!!p.experiences?.length && (
        <section className="profile-panel">
          <h2>Experience</h2>
          <ExperienceList items={p.experiences} />
        </section>
      )}
      {!!p.educations?.length && (
        <section className="profile-panel">
          <h2>Education</h2>
          <EducationList items={p.educations} />
        </section>
      )}
      {!!p.skills?.length && (
        <section className="profile-panel">
          <h2>Skills</h2>
          <ul className="skill-list public-skills">
            {p.skills.map((skill) => (
              <li key={skill}>{skill}</li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
