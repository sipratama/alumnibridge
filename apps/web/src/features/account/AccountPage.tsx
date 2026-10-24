import { Link } from 'react-router-dom'
import { useAuth } from '@/app/authContext'

export function AccountPage() {
  const { user } = useAuth()
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Your account</h1>
          <p>Your membership and professional story are managed separately.</p>
        </div>
      </div>
      <section className="profile-panel">
        <h2>Welcome, {user?.display_name || 'member'}.</h2>
        <p>
          Your affiliations connect you to the community. Adding education to
          your profile does not verify alumni membership.
        </p>
        <div className="membership-list">
          {user?.memberships.map((membership) => (
            <article key={membership.id}>
              <strong>
                {membership.role === 'alumni' ? 'Alumni' : 'Student'}
              </strong>
              <span>
                {membership.institution.toUpperCase()} · Mathematics Education
              </span>
              <span className="membership-status">
                {membership.status === 'pending'
                  ? 'Pending administrator verification'
                  : membership.status}
              </span>
            </article>
          ))}
        </div>
        <Link className="text-link" to="/profile">
          Build your professional profile →
        </Link>
        <p>
          <Link className="text-link" to="/onboarding">
            Manage affiliations →
          </Link>
        </p>
      </section>
    </>
  )
}
