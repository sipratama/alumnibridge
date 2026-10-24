import { afterEach, describe, expect, it } from 'vitest'
import { clearDestination, rememberDestination, takeCallbackDestination, takeDestination } from './authDestination'

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

  it('restores an anonymous public deep link after check-sso without requiring login', () => {
    window.history.replaceState(null, '', '/in/ari-member')
    rememberDestination()
    expect(takeCallbackDestination('loading', '/')).toBeNull()
    expect(takeCallbackDestination('anonymous', '/')).toBe('/in/ari-member')
    expect(takeCallbackDestination('anonymous', '/')).toBeNull()
  })

  it('does not consume a pending destination outside the callback or on authentication errors', () => {
    window.history.replaceState(null, '', '/profile')
    rememberDestination()
    expect(takeCallbackDestination('authenticated', '/account')).toBeNull()
    expect(takeCallbackDestination('error', '/')).toBeNull()
    expect(takeCallbackDestination('authenticated', '/')).toBe('/profile')
  })

  it('rejects external and malformed destinations', () => {
    sessionStorage.setItem('alumnibridge.auth.destination', '//evil.example/')
    expect(takeDestination()).toBeNull()
    sessionStorage.setItem('alumnibridge.auth.destination', '/\\evil.example')
    expect(takeDestination()).toBeNull()
  })
})
