import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { reportQueryError, reportQuerySuccess } from '@/lib/connection'

/** Mounted once at the app root. Listens to every query's real outcome and
 * updates the global connection indicator -- not a separate polled
 * endpoint, just an honest reflection of whether recent real requests have
 * been succeeding. */
export function ConnectionMonitor() {
  const queryClient = useQueryClient()

  useEffect(() => {
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== 'updated') return
      const query = event.query
      if (query.state.status === 'success') {
        reportQuerySuccess()
      } else if (query.state.status === 'error') {
        reportQueryError(query.state.error)
      }
    })
  }, [queryClient])

  return null
}
