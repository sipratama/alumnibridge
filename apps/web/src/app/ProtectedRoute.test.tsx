import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ProtectedRoute } from './ProtectedRoute'

const auth = vi.hoisted(() => ({ status: 'anonymous', user: undefined as undefined | { memberships: Array<{ id: string }> } }))
vi.mock('./authContext', () => ({ useAuth: () => ({ ...auth, login: vi.fn() }) }))

function renderRoute(path = '/account') {
  render(<MemoryRouter initialEntries={[path]}><Routes>
    <Route element={<ProtectedRoute requireOnboarding />}>
      <Route path="/account" element={<h1>Account details</h1>} />
    </Route>
    <Route element={<ProtectedRoute />}>
      <Route path="/onboarding" element={<h1>Onboarding form</h1>} />
    </Route>
  </Routes></MemoryRouter>)
}

describe('protected routes', () => {
  it('prompts anonymous visitors to sign in', () => { auth.status = 'anonymous'; auth.user = undefined; renderRoute(); expect(screen.getByText('Sign in to continue.')).toBeInTheDocument(); expect(screen.queryByText('Account details')).not.toBeInTheDocument() })
  it('keeps account content hidden while checking the session', () => { auth.status = 'loading'; renderRoute(); expect(screen.getByText('Connecting to AlumniBridge…')).toBeInTheDocument(); expect(screen.queryByText('Account details')).not.toBeInTheDocument() })
  it('routes new members to onboarding', () => { auth.status = 'authenticated'; auth.user = { memberships: [] }; renderRoute(); expect(screen.getByText('Onboarding form')).toBeInTheDocument() })
  it('allows onboarded members into their account', () => { auth.status = 'authenticated'; auth.user = { memberships: [{ id: 'one' }] }; renderRoute(); expect(screen.getByText('Account details')).toBeInTheDocument() })
  it('shows expired sessions without account content', () => { auth.status = 'unauthorized'; auth.user = undefined; renderRoute(); expect(screen.getByText('Your session has expired.')).toBeInTheDocument(); expect(screen.queryByText('Account details')).not.toBeInTheDocument() })
})
