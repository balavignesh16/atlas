import type { GraphSnapshot, Incident } from '@/api/types'
import { edgeKey } from './edge-metrics'

export type HighlightState = 'affected' | 'related' | 'unrelated' | 'none'

export interface GraphHighlight {
  active: boolean
  nodes: Map<string, HighlightState>
  edges: Map<string, HighlightState>
}

const NO_HIGHLIGHT: GraphHighlight = { active: false, nodes: new Map(), edges: new Map() }

/**
 * Three real tiers, derived only from Incident.affectedServices/affectedEdges
 * (both real fields, verified against incidentmodel.Incident) plus the
 * graph's own real topology -- never from matching service names that look
 * similar, and never inventing a relationship the data doesn't support:
 *
 *  - affected:  the service/edge is literally in the incident's own
 *               affectedServices/affectedEdges lists.
 *  - related:   not directly listed, but the edge has one endpoint that IS
 *               an affected service (a real adjacency in the current graph,
 *               not a guess) -- or the node is the other end of such an edge.
 *  - unrelated: everything else.
 */
export function computeHighlight(snapshot: GraphSnapshot | undefined, incident: Incident | undefined): GraphHighlight {
  if (!snapshot || !incident) return NO_HIGHLIGHT

  const affectedServices = new Set(incident.affectedServices ?? [])
  const affectedEdgeKeys = new Set(incident.affectedEdges ?? [])
  if (affectedServices.size === 0 && affectedEdgeKeys.size === 0) return NO_HIGHLIGHT

  const nodes = new Map<string, HighlightState>()
  const edges = new Map<string, HighlightState>()
  const relatedNodes = new Set<string>()

  for (const node of snapshot.nodes) {
    nodes.set(node, affectedServices.has(node) ? 'affected' : 'unrelated')
  }

  for (const edge of snapshot.edges) {
    const key = edgeKey(edge.source, edge.target)
    const touchesAffected = affectedServices.has(edge.source) || affectedServices.has(edge.target)

    if (affectedEdgeKeys.has(key)) {
      edges.set(key, 'affected')
    } else if (touchesAffected) {
      edges.set(key, 'related')
    } else {
      edges.set(key, 'unrelated')
    }

    if (touchesAffected) {
      if (!affectedServices.has(edge.source)) relatedNodes.add(edge.source)
      if (!affectedServices.has(edge.target)) relatedNodes.add(edge.target)
    }
  }

  for (const node of relatedNodes) {
    if (nodes.get(node) !== 'affected') nodes.set(node, 'related')
  }

  return { active: true, nodes, edges }
}
