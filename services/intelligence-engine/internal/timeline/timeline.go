package timeline

import (
	"sort"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
)

// BuildTimeline returns chronologically ordered spans.
func BuildTimeline(spans []*correlationmodel.CorrelatedSpan) []*correlationmodel.TimelineNode {
	nodes := make([]*correlationmodel.TimelineNode, 0, len(spans))
	for _, s := range spans {
		nodes = append(nodes, &correlationmodel.TimelineNode{
			SpanID:        s.SpanID,
			ServiceName:   s.ServiceName,
			OperationName: s.OperationName,
			StartTime:     s.StartTime,
			EndTime:       s.EndTime,
			DurationMs:    s.DurationMs,
			Status:        s.Status,
		})
	}

	// Sort by StartTime. If identical, sort by SpanID deterministically.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartTime.Equal(nodes[j].StartTime) {
			return nodes[i].SpanID < nodes[j].SpanID
		}
		return nodes[i].StartTime.Before(nodes[j].StartTime)
	})

	return nodes
}

// BuildTree constructs the hierarchical tree of spans.
func BuildTree(spans []*correlationmodel.CorrelatedSpan) []*correlationmodel.TreeNode {
	nodes := make(map[string]*correlationmodel.TreeNode)
	var roots []*correlationmodel.TreeNode

	for _, s := range spans {
		nodes[s.SpanID] = &correlationmodel.TreeNode{
			SpanID:        s.SpanID,
			ParentSpanID:  s.ParentSpanID,
			ServiceName:   s.ServiceName,
			OperationName: s.OperationName,
			StartTime:     s.StartTime,
			EndTime:       s.EndTime,
			DurationMs:    s.DurationMs,
			Status:        s.Status,
			Children:      make([]*correlationmodel.TreeNode, 0),
		}
	}

	for _, s := range spans {
		node := nodes[s.SpanID]
		if s.ParentSpanID == "" {
			roots = append(roots, node)
		} else {
			if parent, exists := nodes[s.ParentSpanID]; exists {
				parent.Children = append(parent.Children, node)
			} else {
				// Parent doesn't exist (partial trace) - treat this node as a root for the tree to ensure it's not lost
				roots = append(roots, node)
			}
		}
	}

	// Sort roots and children for deterministic output
	sortTreeNodes(roots)

	return roots
}

func sortTreeNodes(nodes []*correlationmodel.TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartTime.Equal(nodes[j].StartTime) {
			return nodes[i].SpanID < nodes[j].SpanID
		}
		return nodes[i].StartTime.Before(nodes[j].StartTime)
	})

	for _, n := range nodes {
		if len(n.Children) > 0 {
			sortTreeNodes(n.Children)
		}
	}
}
