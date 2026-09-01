import type { IncidentSeverity, IncidentStatus } from '@/api/types'

export type IncidentsSearch = {
  status?: IncidentStatus
  severity?: IncidentSeverity
  service?: string
}
