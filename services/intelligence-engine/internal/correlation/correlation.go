package correlation

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
)

// traceInternal tracks state for a specific trace.
type traceInternal struct {
	traceID      string
	lastObserved time.Time
	spans        map[string]*correlationmodel.CorrelatedSpan
	// track parent-child for edges
	resolvedEdges map[string]struct{}
}

// Engine processes normalized ATLASEvents to reconstruct traces and dependency graphs.
type Engine struct {
	mu               sync.RWMutex
	graphBuilder     *graph.DependencyGraph
	retentionSeconds int

	// Indexes
	traceIndex   map[string]*traceInternal
	spanIndex    map[string]*correlationmodel.CorrelatedSpan // traceID+spanID -> span
	serviceIndex map[string]map[string]struct{}              // serviceName -> map[traceID+spanID]

	// To handle parent-child relationship ordering independent of arrival
	// child_trace_span_id -> parent_trace_span_id
	parentLinks map[string]string
	// parent_trace_span_id -> []child_trace_span_id
	childrenLinks map[string][]string
}

// NewEngine creates a new correlation engine.
func NewEngine(g *graph.DependencyGraph, retentionSeconds int) *Engine {
	if retentionSeconds <= 0 {
		retentionSeconds = 300 // default 5 min
	}
	return &Engine{
		graphBuilder:     g,
		retentionSeconds: retentionSeconds,
		traceIndex:       make(map[string]*traceInternal),
		spanIndex:        make(map[string]*correlationmodel.CorrelatedSpan),
		serviceIndex:     make(map[string]map[string]struct{}),
		parentLinks:      make(map[string]string),
		childrenLinks:    make(map[string][]string),
	}
}

func spanKey(traceID, spanID string) string {
	return fmt.Sprintf("%s-%s", traceID, spanID)
}

// ProcessEvent consumes a normalized event.
func (e *Engine) ProcessEvent(ev event.ATLASEvent) {
	if ev.EventType != event.EventTypeTraceSpan {
		// Only process trace spans for correlation
		return
	}

	if ev.TraceID == "" || ev.SpanID == "" {
		return
	}

	key := spanKey(ev.TraceID, ev.SpanID)

	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	// Deduplication
	if _, exists := e.spanIndex[key]; exists {
		// Duplicate span. Update last observed trace time if possible, but do not process again.
		if t, ok := e.traceIndex[ev.TraceID]; ok {
			t.lastObserved = now
		}
		return
	}

	span := correlationmodel.FromEvent(&ev)
	e.spanIndex[key] = span

	// Trace Index Update
	tInternal, exists := e.traceIndex[ev.TraceID]
	if !exists {
		tInternal = &traceInternal{
			traceID:       ev.TraceID,
			spans:         make(map[string]*correlationmodel.CorrelatedSpan),
			resolvedEdges: make(map[string]struct{}),
		}
		e.traceIndex[ev.TraceID] = tInternal
	}
	tInternal.lastObserved = now
	tInternal.spans[key] = span

	// Service Index Update
	if ev.ServiceName != "" {
		srvSet, exists := e.serviceIndex[ev.ServiceName]
		if !exists {
			srvSet = make(map[string]struct{})
			e.serviceIndex[ev.ServiceName] = srvSet
		}
		srvSet[key] = struct{}{}
	}

	// Link Processing
	if ev.ParentSpanID != "" {
		parentKey := spanKey(ev.TraceID, ev.ParentSpanID)
		e.parentLinks[key] = parentKey
		e.childrenLinks[parentKey] = append(e.childrenLinks[parentKey], key)

		// Resolve upward (I am a child, does my parent exist?)
		if parentSpan, parentExists := e.spanIndex[parentKey]; parentExists {
			e.resolveDependency(tInternal, parentSpan, span)
		}
	}

	// Resolve downward (I am a parent, do my children exist already?)
	if children, hasChildren := e.childrenLinks[key]; hasChildren {
		for _, childKey := range children {
			if childSpan, childExists := e.spanIndex[childKey]; childExists {
				e.resolveDependency(tInternal, span, childSpan)
			}
		}
	}
}

func (e *Engine) resolveDependency(t *traceInternal, parent, child *correlationmodel.CorrelatedSpan) {
	if parent.ServiceName == "" || child.ServiceName == "" {
		return
	}
	if parent.ServiceName == child.ServiceName {
		// Exclude internal service-to-service self calls from dependency graph
		return
	}

	edgeIdentifier := fmt.Sprintf("%s->%s", parent.ServiceName, child.ServiceName)

	// Deduplicate client/server logical calls within the same trace.
	// E.g. Order -> Payment might happen once, but we only record it once per logical invocation.
	// Actually, if Order calls Payment 3 times, there are 3 distinct Order spans calling 3 distinct Payment spans.
	// So we can deduplicate using the specific parent-child span pair.
	pairIdentifier := fmt.Sprintf("%s_%s_%s", parent.SpanID, child.SpanID, edgeIdentifier)

	if _, alreadyResolved := t.resolvedEdges[pairIdentifier]; !alreadyResolved {
		t.resolvedEdges[pairIdentifier] = struct{}{}
		
		isError := child.Status == "ERROR"
		// Only create edge based on observed relationships!
		e.graphBuilder.AddDependency(parent.ServiceName, child.ServiceName, child.DurationMs, isError, child.Status)
	}
}

// GetTrace reconstructs a trace by its ID.
func (e *Engine) GetTrace(traceID string) (*correlationmodel.CorrelatedTrace, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tInternal, exists := e.traceIndex[traceID]
	if !exists {
		return nil, false
	}

	return e.buildCorrelatedTrace(tInternal), true
}

func (e *Engine) buildCorrelatedTrace(t *traceInternal) *correlationmodel.CorrelatedTrace {
	trace := &correlationmodel.CorrelatedTrace{
		TraceSummary: correlationmodel.TraceSummary{
			TraceID:       t.traceID,
			SpanCount:     len(t.spans),
			Services:      make([]string, 0),
			OverallStatus: "OK", // default, derived below
		},
		Spans: make([]*correlationmodel.CorrelatedSpan, 0, len(t.spans)),
	}

	srvMap := make(map[string]struct{})
	var minStart, maxEnd time.Time
	first := true

	var roots []*correlationmodel.CorrelatedSpan
	hasError := false
	hasUnset := false

	resolvedRel := 0
	unresolvedRel := 0

	for _, span := range t.spans {
		// Deep copy to prevent race conditions when returned
		sCopy := *span
		// Copy map
		if span.Attributes != nil {
			sCopy.Attributes = make(map[string]string)
			for k, v := range span.Attributes {
				sCopy.Attributes[k] = v
			}
		}

		trace.Spans = append(trace.Spans, &sCopy)

		if sCopy.ServiceName != "" {
			srvMap[sCopy.ServiceName] = struct{}{}
		}

		if first || sCopy.StartTime.Before(minStart) {
			minStart = sCopy.StartTime
		}
		if first || sCopy.EndTime.After(maxEnd) {
			maxEnd = sCopy.EndTime
		}
		first = false

		if sCopy.Status == "ERROR" {
			hasError = true
		} else if sCopy.Status == "UNSET" {
			hasUnset = true
		}

		if sCopy.ParentSpanID == "" {
			roots = append(roots, &sCopy)
		} else {
			parentKey := spanKey(t.traceID, sCopy.ParentSpanID)
			if _, ok := e.spanIndex[parentKey]; ok {
				resolvedRel++
			} else {
				unresolvedRel++
			}
		}
	}

	for srv := range srvMap {
		trace.Services = append(trace.Services, srv)
	}
	sort.Strings(trace.Services)
	trace.ServiceCount = len(trace.Services)

	if hasError {
		trace.OverallStatus = "ERROR"
	} else if hasUnset {
		trace.OverallStatus = "UNSET"
	}

	if len(roots) == 1 {
		trace.RootService = roots[0].ServiceName
	} else if len(roots) == 0 {
		trace.RootService = "unknown"
	} else {
		trace.RootService = "ambiguous"
	}

	if !first {
		trace.StartTime = minStart
		trace.EndTime = maxEnd
		trace.DurationMs = maxEnd.Sub(minStart).Milliseconds()
	}

	trace.ResolvedRelationships = resolvedRel
	trace.UnresolvedRelationships = unresolvedRel

	return trace
}

// CleanupExpired removes stale correlation state.
func (e *Engine) CleanupExpired(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := now.Add(-time.Duration(e.retentionSeconds) * time.Second)

	for traceID, t := range e.traceIndex {
		if t.lastObserved.Before(cutoff) {
			// Expire this trace
			for key, span := range t.spans {
				// Remove from spanIndex
				delete(e.spanIndex, key)
				
				// Remove from parentLinks and childrenLinks
				delete(e.parentLinks, key)
				delete(e.childrenLinks, key)
				
				// Remove from serviceIndex
				if srvSet, ok := e.serviceIndex[span.ServiceName]; ok {
					delete(srvSet, key)
					if len(srvSet) == 0 {
						delete(e.serviceIndex, span.ServiceName)
					}
				}
			}
			delete(e.traceIndex, traceID)
		}
	}
}
