// api.ts — minimal HTTP client for the afs control plane.
//
// dev: vite proxies /v1/* to 127.0.0.1:8091.
// prod: same-origin to whatever serves the agent-site.

const TOKEN_KEY = 'afs.token'

export const tokenStore = {
  get: (): string | null => {
    try { return localStorage.getItem(TOKEN_KEY) } catch { return null }
  },
  set: (token: string) => {
    try { localStorage.setItem(TOKEN_KEY, token) } catch { /* ignore */ }
  },
  clear: () => {
    try { localStorage.removeItem(TOKEN_KEY) } catch { /* ignore */ }
  },
}

export class APIError extends Error {
  status: number
  whyHref: string | null
  body: unknown
  constructor(status: number, message: string, whyHref: string | null, body: unknown) {
    super(message)
    this.status = status
    this.whyHref = whyHref
    this.body = body
  }
}

export async function fetchJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = tokenStore.get()
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(path, {
    ...init,
    headers,
    // forwards Clerk session cookie when same-origin in prod; no-op in dev.
    credentials: 'include',
  })

  // surface the /why link header for any 4xx
  const whyLink = parseLinkHeader(res.headers.get('Link'), 'why')

  if (!res.ok) {
    let body: unknown = null
    try { body = await res.json() } catch { /* ignore */ }
    throw new APIError(res.status, `${res.status} ${res.statusText} on ${path}`, whyLink, body)
  }

  return res.json() as Promise<T>
}

function parseLinkHeader(value: string | null, rel: string): string | null {
  if (!value) return null
  // Link: </why/abc>; rel="why", </ledger>; rel="ledger"
  const parts = value.split(',')
  for (const part of parts) {
    const m = /<([^>]+)>\s*;\s*rel="?([^";]+)"?/.exec(part.trim())
    if (m && m[2] === rel) return m[1]
  }
  return null
}
