import type { ExecutionRecord, Incident } from '@/api/types'

export type CommandResult =
  | { kind: 'nav'; id: string; label: string; to: string }
  | { kind: 'jump-incident'; id: string; incidentId: string }
  | { kind: 'jump-execution'; id: string; executionId: string }
  | { kind: 'incident'; id: string; incident: Incident }
  | { kind: 'execution'; id: string; execution: ExecutionRecord }

const NAV_COMMANDS = [
  { label: 'Go to Command Center', to: '/' },
  { label: 'Go to Incidents', to: '/incidents' },
  { label: 'Go to Graph', to: '/graph' },
  { label: 'Go to Executions', to: '/executions' },
]

const MAX_DATA_RESULTS = 6

/** Loose enough to catch real Atlas incident/execution IDs (UUIDs and the
 * shorter hand-written IDs used in tests) without matching a plain word
 * search like "payment" or "gateway". */
function looksLikeId(query: string): boolean {
  return /^[a-zA-Z0-9-]{6,}$/.test(query) && /[0-9]/.test(query)
}

/**
 * Pure so it's testable without mounting the palette or a router. Combines
 * three genuinely different kinds of result -- static navigation, a
 * direct ID jump (which does not check whether the ID actually exists;
 * the destination page's own 404/empty handling covers that), and search
 * over already-loaded incidents/executions -- and keeps them visually and
 * structurally distinct rather than merging into one flat list.
 */
export function buildCommandResults(query: string, incidents: Incident[], executions: ExecutionRecord[]): CommandResult[] {
  const needle = query.trim().toLowerCase()
  const results: CommandResult[] = []

  const navMatches = NAV_COMMANDS.filter((cmd) => needle === '' || cmd.label.toLowerCase().includes(needle))
  for (const cmd of navMatches) {
    results.push({ kind: 'nav', id: `nav:${cmd.to}`, label: cmd.label, to: cmd.to })
  }

  if (needle && looksLikeId(query.trim())) {
    results.push({ kind: 'jump-incident', id: `jump-incident:${query.trim()}`, incidentId: query.trim() })
    results.push({ kind: 'jump-execution', id: `jump-execution:${query.trim()}`, executionId: query.trim() })
  }

  const incidentPool = needle === '' ? incidents.slice(0, MAX_DATA_RESULTS) : incidents.filter(matchesIncident(needle)).slice(0, MAX_DATA_RESULTS)
  for (const incident of incidentPool) {
    results.push({ kind: 'incident', id: `incident:${incident.incidentId}`, incident })
  }

  if (needle !== '') {
    const executionPool = executions.filter(matchesExecution(needle)).slice(0, MAX_DATA_RESULTS)
    for (const execution of executionPool) {
      results.push({ kind: 'execution', id: `execution:${execution.executionId}`, execution })
    }
  }

  return results
}

function matchesIncident(needle: string) {
  return (incident: Incident) =>
    incident.incidentId.toLowerCase().includes(needle) ||
    incident.rootService.toLowerCase().includes(needle) ||
    incident.title.toLowerCase().includes(needle)
}

function matchesExecution(needle: string) {
  return (execution: ExecutionRecord) =>
    execution.executionId.toLowerCase().includes(needle) ||
    execution.incidentId.toLowerCase().includes(needle) ||
    execution.service.toLowerCase().includes(needle) ||
    execution.action.toLowerCase().includes(needle)
}
