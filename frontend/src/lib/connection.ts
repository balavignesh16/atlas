// Global "is Atlas reachable" signal, derived honestly from the real
// success/failure of whatever queries are already running -- no separate
// heartbeat request invented just to power this indicator.

import { useSyncExternalStore } from 'react'
import { NetworkError } from '@/api/client'

interface ConnectionState {
  lastSuccessAt: Date | null
  degraded: boolean
}

let state: ConnectionState = { lastSuccessAt: null, degraded: false }
const listeners = new Set<() => void>()

function setState(next: Partial<ConnectionState>) {
  state = { ...state, ...next }
  for (const listener of listeners) listener()
}

export function reportQuerySuccess() {
  setState({ lastSuccessAt: new Date(), degraded: false })
}

export function reportQueryError(error: unknown) {
  if (error instanceof NetworkError) {
    setState({ degraded: true })
  }
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useConnectionStatus() {
  return useSyncExternalStore(subscribe, () => state)
}
