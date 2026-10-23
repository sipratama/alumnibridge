import { useEffect, useMemo, useState, type ReactNode } from 'react'
import Keycloak from 'keycloak-js'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, request, type CurrentUser } from '@/shared/api/client'
import { useLocation, useNavigate } from 'react-router-dom'
import { clearDestination, rememberDestination, takeDestination } from './authDestination'

const callbackUri = import.meta.env.VITE_OIDC_CALLBACK_URI ?? 'http://localhost:5173/'

const keycloak = new Keycloak({
  url: import.meta.env.VITE_KEYCLOAK_URL ?? 'http://localhost:18085',
  realm: import.meta.env.VITE_KEYCLOAK_REALM ?? 'alumnibridge',
  clientId: import.meta.env.VITE_KEYCLOAK_CLIENT_ID ?? 'alumnibridge-web',
})
let initialization: Promise<boolean> | undefined

import { AuthContext, type AuthStatus, type AuthContextValue } from './authContext'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [phase, setPhase] = useState<AuthStatus>('loading')
  const queryClient = useQueryClient()
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    let active = true
    if (!initialization) {
      rememberDestination()
      initialization = keycloak.init({ onLoad: 'check-sso', flow: 'standard', pkceMethod: 'S256', checkLoginIframe: false, redirectUri: callbackUri })
    }
    initialization.then((authenticated) => { if (active) setPhase(authenticated ? 'authenticated' : 'anonymous') })
      .catch(() => { if (active) setPhase('error') })
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (phase !== 'authenticated') return
    const destination = takeDestination()
    if (destination && location.pathname === '/') navigate(destination, { replace: true })
  }, [phase, location.pathname, navigate])

  async function getToken(): Promise<string> {
    try { await keycloak.updateToken(30) } catch { throw new ApiError(401, 'unauthorized') }
    if (!keycloak.token) throw new ApiError(401, 'unauthorized')
    return keycloak.token
  }

  const me = useQuery({ queryKey: ['auth', 'me'], queryFn: () => request<CurrentUser>('/auth/me', getToken), enabled: phase === 'authenticated', retry: false })
  const status: AuthStatus = phase === 'authenticated' && me.isPending ? 'loading'
    : phase === 'authenticated' && me.error ? (me.error instanceof ApiError && me.error.status === 401 ? 'unauthorized' : 'error')
    : phase

  const value = useMemo<AuthContextValue>(() => ({
    status,
    user: me.data,
    login: async () => { rememberDestination(); await keycloak.login({ redirectUri: callbackUri }) },
    register: async () => { rememberDestination(); await keycloak.register({ redirectUri: callbackUri }) },
    logout: async () => { clearDestination(); queryClient.clear(); await keycloak.logout({ redirectUri: callbackUri }) },
    getToken,
    refreshUser: async () => { await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] }) },
  }), [status, me.data, queryClient])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
