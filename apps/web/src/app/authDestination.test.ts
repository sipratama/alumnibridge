import { afterEach, describe, expect, it } from 'vitest'
import { clearDestination, rememberDestination, takeDestination } from './authDestination'

afterEach(() => { clearDestination(); window.history.replaceState(null, '', '/') })

describe('OIDC destination restoration', () => {
  it('preserves a protected route across a root callback', () => {
    window.history.replaceState(null, '', '/onboarding?step=student')
    rememberDestination()
    window.history.replaceState(null, '', '/?state=keycloak-state&code=example')
    rememberDestination()
    expect(takeDestination()).toBe('/onboarding?step=student')
    expect(takeDestination()).toBeNull()
  })

  it('rejects external and malformed destinations', () => {
    sessionStorage.setItem('alumnibridge.auth.destination', '//evil.example/')
    expect(takeDestination()).toBeNull()
    sessionStorage.setItem('alumnibridge.auth.destination', '/\\evil.example')
    expect(takeDestination()).toBeNull()
  })
})
