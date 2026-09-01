import { MarkerType, ReactFlowProvider, type Edge, type Node } from '@xyflow/react'
import { useMemo, useState } from 'react'
import { graphRoute } from '@/app/routes'
import { useGraphSnapshot } from '@/api/graph'
import { useIncident } from '@/api/incidents'
import type { DependencyEdge } from '@/api/types'
import { ErrorState } from '@/components/ui/ErrorState'
import { EmptyState } from '@/components/ui/EmptyState'
import type { AtlasEdgeData } from './DependencyEdgeView'
import { EdgeInspector } from './EdgeInspector'
import { GraphCanvas } from './GraphCanvas'
import { GraphSkeleton } from './GraphSkeleton'
import { computeHighlight } from './highlight'
import { colorVarFor, edgeKey } from './edge-metrics'
import { computeLayeredLayout } from './layout'
import { ServiceInspector } from './ServiceInspector'
import type { ServiceNodeData } from './ServiceNodeView'

/**
 * Orchestrates data fetching + interaction state; GraphCanvas (inside
 * ReactFlowProvider) owns the actual viewport so it can call useReactFlow().
 */
export function GraphPage() {
  const { highlight: highlightId } = graphRoute.useSearch()
  const navigate = graphRoute.useNavigate()

  const { data: snapshot, isPending, isError, error, dataUpdatedAt } = useGraphSnapshot()
  const { data: incident } = useIncident(highlightId)

  const [search, setSearch] = useState('')
  const [selectedService, setSelectedService] = useState<string | null>(null)
  const [selectedEdge, setSelectedEdge] = useState<DependencyEdge | null>(null)

  const trimmedSearch = search.trim().toLowerCase()

  const layout = useMemo(
    () => (snapshot ? computeLayeredLayout(snapshot.nodes, snapshot.edges) : null),
    [snapshot],
  )
  const highlight = useMemo(() => computeHighlight(snapshot, incident), [snapshot, incident])

  const nodes: Node<ServiceNodeData>[] = useMemo(() => {
    if (!snapshot || !layout) return []
    return snapshot.nodes.map((name) => {
      const position = layout.positions.get(name) ?? { x: 0, y: 0 }
      const state = highlight.active ? (highlight.nodes.get(name) ?? 'unrelated') : 'none'
      const matchesSearch = trimmedSearch === '' || name.toLowerCase().includes(trimmedSearch)
      const dimmed = (highlight.active && state === 'unrelated') || (trimmedSearch !== '' && !matchesSearch)
      return {
        id: name,
        type: 'service',
        position,
        data: { name, highlight: state, dimmed, onOpen: () => setSelectedService(name) },
        draggable: false,
        connectable: false,
      }
    })
  }, [snapshot, layout, highlight, trimmedSearch])

  const edges: Edge<AtlasEdgeData>[] = useMemo(() => {
    if (!snapshot) return []
    return snapshot.edges.map((edge) => {
      const key = edgeKey(edge.source, edge.target)
      const state = highlight.active ? (highlight.edges.get(key) ?? 'unrelated') : 'none'
      const matchesSearch =
        trimmedSearch === '' ||
        edge.source.toLowerCase().includes(trimmedSearch) ||
        edge.target.toLowerCase().includes(trimmedSearch)
      const dimmed = (highlight.active && state === 'unrelated') || (trimmedSearch !== '' && !matchesSearch)
      const selected = selectedEdge !== null && edgeKey(selectedEdge.source, selectedEdge.target) === key

      return {
        id: key,
        type: 'dependency',
        source: edge.source,
        target: edge.target,
        selected,
        data: { edge, highlight: state, dimmed, onOpen: () => setSelectedEdge(edge) },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          width: 14,
          height: 14,
          color: colorVarFor(state, edge.error_count > 0, selected),
        },
      }
    })
  }, [snapshot, highlight, trimmedSearch, selectedEdge])

  if (isError) {
    return (
      <div className="flex h-full items-center justify-center">
        <ErrorState error={error} />
      </div>
    )
  }

  if (isPending) {
    return <GraphSkeleton />
  }

  if (snapshot.nodes.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <EmptyState
          title="No dependency data available"
          description="Atlas has not observed any service-to-service calls yet."
        />
      </div>
    )
  }

  return (
    <div className="relative h-full w-full">
      <ReactFlowProvider>
        <GraphCanvas
          nodes={nodes}
          edges={edges}
          search={search}
          onSearchChange={setSearch}
          serviceCount={snapshot.nodes.length}
          incident={incident}
          onClearHighlight={() => navigate({ search: {} })}
          dataUpdatedAt={dataUpdatedAt}
          polling
        />
      </ReactFlowProvider>

      <ServiceInspector serviceName={selectedService} onClose={() => setSelectedService(null)} />
      <EdgeInspector edge={selectedEdge} onClose={() => setSelectedEdge(null)} />
    </div>
  )
}
