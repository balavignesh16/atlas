package graph

import (
	"fmt"
	"sync"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
)

// DependencyGraph manages the observed service-to-service dependencies.
type DependencyGraph struct {
	mu               sync.RWMutex
	nodes            map[string]*correlationmodel.ServiceNode
	edges            map[string]*correlationmodel.DependencyEdge
	retentionSeconds int
}

// NewDependencyGraph creates a new thread-safe dependency graph.
func NewDependencyGraph(retentionSeconds int) *DependencyGraph {
	if retentionSeconds <= 0 {
		retentionSeconds = 300 // default 5 minutes
	}
	return &DependencyGraph{
		nodes:            make(map[string]*correlationmodel.ServiceNode),
		edges:            make(map[string]*correlationmodel.DependencyEdge),
		retentionSeconds: retentionSeconds,
	}
}

func edgeKey(source, target string) string {
	return fmt.Sprintf("%s->%s", source, target)
}

// AddDependency records an observed relationship between two services.
func (g *DependencyGraph) AddDependency(sourceService, targetService string, durationMs int64, isError bool, status string) {
	if sourceService == "" || targetService == "" {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()

	// Update Source Node
	g.updateNodeUnsafe(sourceService, now)
	
	// Update Target Node
	g.updateNodeUnsafe(targetService, now)

	// Update Edge
	key := edgeKey(sourceService, targetService)
	edge, exists := g.edges[key]
	if !exists {
		edge = &correlationmodel.DependencyEdge{
			SourceService: sourceService,
			TargetService: targetService,
			FirstObserved: now,
			StatusCounts:  make(map[string]int64),
		}
		g.edges[key] = edge
	}

	edge.LastObserved = now
	edge.CallCount++
	edge.TotalDurationMs += durationMs
	edge.AverageDurationMs = edge.TotalDurationMs / edge.CallCount
	
	if isError {
		edge.ErrorCount++
	}
	
	if status != "" {
		edge.StatusCounts[status]++
	} else {
		edge.StatusCounts["UNSET"]++
	}
}

func (g *DependencyGraph) updateNodeUnsafe(service string, now time.Time) {
	node, exists := g.nodes[service]
	if !exists {
		node = &correlationmodel.ServiceNode{
			ServiceName:   service,
			FirstObserved: now,
		}
		g.nodes[service] = node
	}
	node.LastObserved = now
	node.SpanCount++
}

// GetSnapshot returns a point-in-time snapshot of the current dependency graph.
func (g *DependencyGraph) GetSnapshot() *correlationmodel.GraphSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()

	snapshot := &correlationmodel.GraphSnapshot{
		Nodes: make([]string, 0, len(g.nodes)),
		Edges: make([]*correlationmodel.DependencyEdge, 0, len(g.edges)),
	}

	for service := range g.nodes {
		snapshot.Nodes = append(snapshot.Nodes, service)
	}

	for _, edge := range g.edges {
		// Deep copy to prevent concurrent mutation during JSON serialization
		eCopy := *edge
		eCopy.StatusCounts = make(map[string]int64)
		for k, v := range edge.StatusCounts {
			eCopy.StatusCounts[k] = v
		}
		snapshot.Edges = append(snapshot.Edges, &eCopy)
	}

	return snapshot
}

// GetServiceDependencies returns incoming and outgoing edges for a specific service.
func (g *DependencyGraph) GetServiceDependencies(service string) ([]*correlationmodel.DependencyEdge, []*correlationmodel.DependencyEdge) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var incoming []*correlationmodel.DependencyEdge
	var outgoing []*correlationmodel.DependencyEdge

	for _, edge := range g.edges {
		if edge.SourceService == service {
			eCopy := g.copyEdgeUnsafe(edge)
			outgoing = append(outgoing, eCopy)
		}
		if edge.TargetService == service {
			eCopy := g.copyEdgeUnsafe(edge)
			incoming = append(incoming, eCopy)
		}
	}

	return incoming, outgoing
}

func (g *DependencyGraph) copyEdgeUnsafe(edge *correlationmodel.DependencyEdge) *correlationmodel.DependencyEdge {
	eCopy := *edge
	eCopy.StatusCounts = make(map[string]int64)
	for k, v := range edge.StatusCounts {
		eCopy.StatusCounts[k] = v
	}
	return &eCopy
}

// GetEdges returns a list of all edges.
func (g *DependencyGraph) GetEdges() []*correlationmodel.DependencyEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := make([]*correlationmodel.DependencyEdge, 0, len(g.edges))
	for _, edge := range g.edges {
		edges = append(edges, g.copyEdgeUnsafe(edge))
	}
	return edges
}

// CleanupExpired removes nodes and edges that have not been observed within the retention window.
func (g *DependencyGraph) CleanupExpired(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := now.Add(-time.Duration(g.retentionSeconds) * time.Second)

	// Remove stale edges
	for key, edge := range g.edges {
		if edge.LastObserved.Before(cutoff) {
			delete(g.edges, key)
		}
	}

	// Remove stale nodes
	for service, node := range g.nodes {
		if node.LastObserved.Before(cutoff) {
			delete(g.nodes, service)
		}
	}
}
