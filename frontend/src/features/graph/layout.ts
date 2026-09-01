/**
 * Deterministic left-to-right layered layout (Kahn's algorithm topological
 * layering, not a force simulation -- the current Atlas topology is small
 * and a physics layout would jitter between polls for no benefit). Same
 * nodes/edges always produce the same positions: ties within a layer are
 * broken alphabetically, and a cycle (no genuine zero-in-degree node left)
 * is broken by picking the lowest-in-degree node, alphabetically, rather
 * than looping forever.
 */

export const LAYER_WIDTH = 260
export const NODE_HEIGHT = 76

export interface LayoutPosition {
  x: number
  y: number
}

export interface LayoutResult {
  positions: Map<string, LayoutPosition>
  layers: string[][]
}

export function computeLayeredLayout(nodes: string[], edges: { source: string; target: string }[]): LayoutResult {
  const outgoing = new Map<string, Set<string>>()
  const inDegree = new Map<string, number>()

  for (const node of nodes) {
    outgoing.set(node, new Set())
    inDegree.set(node, 0)
  }

  for (const edge of edges) {
    if (!outgoing.has(edge.source) || !inDegree.has(edge.target)) continue
    const targets = outgoing.get(edge.source)!
    if (!targets.has(edge.target)) {
      targets.add(edge.target)
      inDegree.set(edge.target, (inDegree.get(edge.target) ?? 0) + 1)
    }
  }

  const remaining = new Set(nodes)
  const localInDegree = new Map(inDegree)
  const layers: string[][] = []

  while (remaining.size > 0) {
    let layer = Array.from(remaining)
      .filter((n) => (localInDegree.get(n) ?? 0) === 0)
      .sort()

    if (layer.length === 0) {
      const [pick] = Array.from(remaining).sort((a, b) => {
        const diff = (localInDegree.get(a) ?? 0) - (localInDegree.get(b) ?? 0)
        return diff !== 0 ? diff : a.localeCompare(b)
      })
      layer = [pick]
    }

    layers.push(layer)
    for (const n of layer) {
      remaining.delete(n)
      for (const target of outgoing.get(n) ?? []) {
        if (remaining.has(target)) {
          localInDegree.set(target, (localInDegree.get(target) ?? 0) - 1)
        }
      }
    }
  }

  const positions = new Map<string, LayoutPosition>()
  layers.forEach((layer, layerIndex) => {
    const centerOffset = -(layer.length - 1) / 2
    layer.forEach((node, i) => {
      positions.set(node, { x: layerIndex * LAYER_WIDTH, y: (centerOffset + i) * NODE_HEIGHT })
    })
  })

  return { positions, layers }
}
