import { Link } from 'react-router-dom'
import { useAuth } from '@/app/authContext'
import { Button } from '@/components/ui/button'

export function AccountPage() {
  const { user, logout } = useAuth()
  return <main className="account-page"><div className="container"><header className="account-header"><Link className="brand" to="/"><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span><span>Alumni<span className="brand-accent">Bridge</span></span></Link><Button variant="outline" onClick={() => void logout()}>Sign out</Button></header><div className="account-card"><span className="section-kicker">YOUR COMMUNITY</span><h1>Welcome, {user?.display_name || 'member'}.</h1><p>Your AlumniBridge account is ready. Professional profiles and the alumni directory are coming later.</p><div className="membership-list">{user?.memberships.map((membership) => <article key={membership.id}><strong>{membership.role === 'alumni' ? 'Alumni' : 'Student'}</strong><span>{membership.institution.toUpperCase()} · Mathematics Education</span><span className="membership-status">{membership.status === 'pending' ? 'Pending administrator verification' : membership.status}</span></article>)}</div><Link className="text-link" to="/onboarding">Add another affiliation →</Link></div></div></main>
}
