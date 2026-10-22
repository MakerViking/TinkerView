// Small REST helpers for the device forms. The global fetch patch (lib/auth)
// already attaches the bearer token to same-origin /api/* calls.

// errorFrom extracts a human message from a failed response: {"error": msg}
// JSON when present, otherwise the plain-text body, otherwise the status line.
async function errorFrom(res: Response): Promise<string> {
  let text = ''
  try {
    text = (await res.text()).trim()
  } catch {
    /* body unreadable */
  }
  if (text) {
    try {
      const j = JSON.parse(text) as { error?: unknown }
      if (j && typeof j.error === 'string' && j.error) return j.error
    } catch {
      /* not JSON — fall through to the raw text */
    }
    return text
  }
  return `${res.status} ${res.statusText}`.trim()
}

// apiSend performs a JSON request and resolves to null on success (2xx) or to the
// error message on failure (including network errors), so forms can render it inline.
export async function apiSend(method: string, url: string, body?: unknown): Promise<string | null> {
  try {
    const res = await fetch(url, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    return res.ok ? null : await errorFrom(res)
  } catch (e) {
    return e instanceof Error ? e.message : 'network error'
  }
}

// apiGet fetches JSON; the result is either the parsed value or an error message.
export async function apiGet<T>(url: string): Promise<{ data: T; error?: undefined } | { data?: undefined; error: string }> {
  try {
    const res = await fetch(url)
    if (!res.ok) return { error: await errorFrom(res) }
    return { data: (await res.json()) as T }
  } catch (e) {
    return { error: e instanceof Error ? e.message : 'network error' }
  }
}
