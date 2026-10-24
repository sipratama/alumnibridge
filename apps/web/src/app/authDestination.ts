const destinationKey = 'alumnibridge.auth.destination'

export function rememberDestination() {
  if (window.location.pathname === '/') return
  sessionStorage.setItem(destinationKey, window.location.pathname + window.location.search + window.location.hash)
}

export function takeDestination(): string | null {
  const value = sessionStorage.getItem(destinationKey)
  sessionStorage.removeItem(destinationKey)
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) return null
  const url = new URL(value, window.location.origin)
  return url.origin === window.location.origin ? url.pathname + url.search + url.hash : null
}

export function clearDestination() {
  sessionStorage.removeItem(destinationKey)
}

// Anonymous check-sso also returns to the stable callback; public deep links must survive it.
export function takeCallbackDestination(phase: string, pathname: string): string | null {
  if (pathname !== '/' || (phase !== 'authenticated' && phase !== 'anonymous')) return null
  return takeDestination()
}
