import { createContext, useContext } from 'react'
import type { CurrentUser } from '@/shared/api/client'

export type AuthStatus = 'loading' | 'anonymous' | 'authenticated' | 'error' | 'unauthorized'
export type AuthContextValue = {
  status: AuthStatus
  user?: CurrentUser
  login: () => Promise<void>
  register: () => Promise<void>
  logout: () => Promise<void>
  getToken: () => Promise<string>
  refreshUser: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('AuthProvider is required')
  return context
}
