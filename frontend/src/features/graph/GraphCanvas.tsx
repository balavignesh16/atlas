import { Background, BackgroundVariant, ReactFlow, useReactFlow, type Edge, type Node } from '@xyflow/react'
import { useCallback, useEffect } from 'react'
import type { Incident } from '@/api/types'
import { PollIndicator } from '@/components/layout/PollIndicator'
import { ServiceNodeView, type ServiceNodeData } from './ServiceNodeView'
import { DependencyEdgeView, type AtlasEdgeData } from './DependencyEdgeView'
import { GraphToolbar } from './GraphToolbar'
import { IncidentBanner } from './IncidentBanner'

const nodeTypes = { service: ServiceNodeView }
const edgeTypes = { dependency: DependencyEdgeView }

/** Rendered inside a ReactFlowProvider (see GraphPage) so it can call
 * useReactFlow() for fit-view. Holds no data-fetching of its own -- purely
 * the viewport, toolbar, and overlay wiring. */
export function GraphCanvas({
  nodes,
  edges,
  search,
  onSearchChange,
  serviceCount,
  incident,
  onClearHighlight,
  dataUpdatedAt,
  polling,
}: {
  nodes: Node<ServiceNodeData>[]
  edges: Edge<AtlasEdgeData>[]
  search: string
  onSearchChange: (value: string) => void
  serviceCount: number
  incident: Incident | undefined
  onClearHighlight: () => void
  dataUpdatedAt: number
  polling: boolean
}) {
  const { fitView } = useReactFlow()

  const handleFitView = useCallback(() => {
    fitView({ padding: 0.2, duration: 200 })
  }, [fitView])

  const handleReset = useCallback(() => {
    onSearchChange('')
    fitView({ padding: 0.2, duration: 200 })
  }, [fitView, onSearchChange])

  const nodeCount = nodes.length
  useEffect(() => {
    const id = requestAnimationFrame(() => fitView({ padding: 0.2, duration: 0 }))
    return () => cancelAnimationFrame(id)
  }, [nodeCount, fitView])

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable
      proOptions={{ hideAttribution: true }}
      minZoom={0.3}
      maxZoom={1.5}
      defaultEdgeOptions={{ focusable: true }}
    >
      <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="var(--color-border-subtle)" />
      <GraphToolbar
        search={search}
        onSearchChange={onSearchChange}
        onFitView={handleFitView}
        onReset={handleReset}
        serviceCount={serviceCount}
      />
      {incident ? <IncidentBanner incident={incident} onClear={onClearHighlight} /> : null}
      <div className="pointer-events-none absolute bottom-2 right-3 z-10">
        <PollIndicator polling={polling} lastUpdatedAt={dataUpdatedAt} />
      </div>
    </ReactFlow>
  )
}
