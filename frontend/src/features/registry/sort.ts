import type { Service, ServiceStatus } from '@/api/types'

const STATUS_RANK: Record<ServiceStatus, number> = { ACTIVE: 0, STALE: 1, RETIRED: 2 }

/** ACTIVE first, then STALE, then RETIRED -- within the same status,
 * alphabetical. No fabricated priority score, just the real status field
 * and name. */
export function sortServices(services: Service[]): Service[] {
  return [...services].sort((a, b) => {
    const rankDiff = STATUS_RANK[a.status] - STATUS_RANK[b.status]
    if (rankDiff !== 0) return rankDiff
    return a.name.localeCompare(b.name)
  })
}
