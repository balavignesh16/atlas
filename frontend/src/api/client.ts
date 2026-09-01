// Central, typed fetch wrapper. No component or hook should call fetch()
// directly -- every request goes through here so auth headers, error
// classification, and the base URL stay in exactly one place.

import { ATLAS_API_URL } from '@/lib/config'
import { clearSession, getApiKey } from './auth'

export class ApiError extends Error {
  readonly status: number
  readonly requiredPermission?: string

  constructor(message: string, status: number, requiredPermission?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.requiredPermission = requiredPermission
  }
}

export class UnauthorizedError extends ApiError {
  constructor(message = 'Your Atlas API key is invalid or missing.') {
    super(message, 401)
    this.name = 'UnauthorizedError'
  }
}

export class ForbiddenError extends ApiError {
  constructor(message: string, requiredPermission?: string) {
    super(message, 403, requiredPermission)
    this.name = 'ForbiddenError'
  }
}

export class NetworkError extends Error {
  constructor(message = 'The intelligence engine did not respond.') {
    super(message)
    this.name = 'NetworkError'
  }
}

interface ErrorBody {
  error?: string
  message?: string
}

/** Best-effort extraction of a required-permission name from the backend's
 * 403 body, e.g. {"error":"principal does not have permission: READ_AUDIT"}. */
function extractRequiredPermission(body: ErrorBody): string | undefined {
  const text = body.error ?? body.message ?? ''
  const match = /permission:\s*([A-Z_]+)/.exec(text)
  return match?.[1]
}

/**
 * Not every error path in the backend responds with JSON -- some handlers
 * (verified in remediation.go's HandleApprove/HandleReject/HandlePropose,
 * e.g. the real, meaningful "cannot approve plan in status: PROPOSED"
 * message) use net/http's `http.Error`, which writes plain text with a
 * `text/plain` content type. Calling `response.json()` on that throws and
 * silently discards the real message. Reading the body as text first and
 * only attempting to parse it as JSON preserves that message either way.
 */
async function parseErrorBody(response: Response): Promise<ErrorBody> {
  const text = await response.text().catch(() => '')
  if (!text) return {}
  try {
    return JSON.parse(text) as ErrorBody
  } catch {
    return { error: text }
  }
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const key = getApiKey()
  const headers = new Headers(init?.headers)
  if (key) headers.set('X-Atlas-Api-Key', key)
  headers.set('Content-Type', 'application/json')

  let response: Response
  try {
    response = await fetch(`${ATLAS_API_URL}${path}`, { ...init, headers })
  } catch {
    throw new NetworkError()
  }

  if (response.status === 401) {
    clearSession()
    throw new UnauthorizedError()
  }

  if (response.status === 403) {
    const body = await parseErrorBody(response)
    throw new ForbiddenError(
      body.error ?? body.message ?? 'This action is not permitted for your role.',
      extractRequiredPermission(body),
    )
  }

  if (!response.ok) {
    const body = await parseErrorBody(response)
    throw new ApiError(body.error ?? body.message ?? `Request failed (${response.status})`, response.status)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}
