// Optional shared-token auth for the dashboard. When the server is started with
// DASHBOARD_TOKEN set, /api/* and /ws require the token (Authorization: Bearer …,
// or ?token= for URLs that can't set headers). Empty token = open (Tailscale is the
// perimeter) and everything here is a no-op.
//
// The browser learns the token from a one-time ?token=… in the URL (a shared link
// or an ntfy action button), which we persist to localStorage and strip from the
// address bar.

const KEY = 'dashboard-token'

function bootstrap(): string {
  try {
    const url = new URL(window.location.href)
    const q = url.searchParams.get('token')
    if (q) {
      localStorage.setItem(KEY, q)
      url.searchParams.delete('token')
      window.history.replaceState({}, '', url.toString())
    }
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return '' // non-browser / sandboxed context
  }
}

let token = bootstrap()

export function getToken(): string {
  return token
}

export function setToken(t: string): void {
  token = t
  try {
    if (t) localStorage.setItem(KEY, t)
    else localStorage.removeItem(KEY)
  } catch {
    /* ignore storage failures */
  }
}

// withToken appends ?token=… to a same-origin URL — for the WebSocket and for
// <img>/<video> elements that can't send an Authorization header. No-op when unset.
export function withToken(url: string): string {
  if (!token) return url
  return url + (url.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(token)
}

// installFetchAuth patches window.fetch once so same-origin /api/* requests carry
// the bearer token (and /frigate/, which the server gates the same way). Centralised so individual call sites (and future ones) don't
// each have to remember it; a no-op when no token is configured.
export function installFetchAuth(): void {
  if (!token) return
  const orig = window.fetch.bind(window)
  window.fetch = (input, init) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
    if (url.startsWith('/api/') || url.startsWith('/frigate/')) {
      const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined))
      if (!headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`)
      return orig(input, { ...init, headers })
    }
    return orig(input, init)
  }
}
