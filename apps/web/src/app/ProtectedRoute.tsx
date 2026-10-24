import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from './authContext'
import { Button } from '@/components/ui/button'

export function ProtectedRoute({ requireOnboarding = false }: { requireOnboarding?: boolean }) {
  const { status, user, login } = useAuth()
  if (status === 'loading') return <main className="state-page" role="status"><h1>Connecting to AlumniBridge…</h1><p>Checking your session.</p></main>
  if (status === 'error') return <main className="state-page" role="alert"><h1>We could not load your account.</h1><p>Check the API and try refreshing this page.</p></main>
  if (status === 'unauthorized') return <main className="state-page" role="alert"><h1>Your session has expired.</h1><p>Please sign in again.</p><Button onClick={() => void login()}>Sign in</Button></main>
  if (status === 'anonymous') return <main className="state-page"><h1>Sign in to continue.</h1><p>Your account details are available only to you.</p><Button onClick={() => void login()}>Sign in</Button></main>
  if (requireOnboarding && !user?.memberships.length) return <Navigate to="/onboarding" replace />
  return <Outlet />
}
