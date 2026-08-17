package correlation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
)

func TestDuplicateSpan(t *testing.T) {
	g := graph.NewDependencyGraph(300)
	engine := correlation.NewEngine(g, 300)

	now := time.Now()
	ev := event.ATLASEvent{
		EventID:     "1",
		EventType:   event.EventTypeTraceSpan,
		TraceID:     "trace1",
		SpanID:      "span1",
		ServiceName: "ServiceA",
		Timestamp:   now,
		Status:      "OK",
	}

	// Send same event twice
	engine.ProcessEvent(ev)
	ev.EventID = "2" // different event ID, but same trace/span
	engine.ProcessEvent(ev)

	trace, exists := engine.GetTrace("trace1")
	if !exists {
		t.Fatalf("Trace not found")
	}
	if trace.SpanCount != 1 {
		t.Fatalf("Expected 1 span, got %d", trace.SpanCount)
	}
}

func TestOutOfOrderEvents(t *testing.T) {
	g := graph.NewDependencyGraph(300)
	engine := correlation.NewEngine(g, 300)

	now := time.Now()
	
	// Create events backwards
	payment := event.ATLASEvent{EventID: "4", EventType: event.EventTypeTraceSpan, TraceID: "t1", SpanID: "payment", ParentSpanID: "order", ServiceName: "Payment", Timestamp: now.Add(3 * time.Second), Status: "OK"}
	inventory := event.ATLASEvent{EventID: "3", EventType: event.EventTypeTraceSpan, TraceID: "t1", SpanID: "inventory", ParentSpanID: "order", ServiceName: "Inventory", Timestamp: now.Add(2 * time.Second), Status: "OK"}
	order := event.ATLASEvent{EventID: "2", EventType: event.EventTypeTraceSpan, TraceID: "t1", SpanID: "order", ParentSpanID: "gateway", ServiceName: "Order", Timestamp: now.Add(1 * time.Second), Status: "OK"}
	gateway := event.ATLASEvent{EventID: "1", EventType: event.EventTypeTraceSpan, TraceID: "t1", SpanID: "gateway", ParentSpanID: "", ServiceName: "Gateway", Timestamp: now, Status: "OK"}

	// Feed backwards
	engine.ProcessEvent(payment)
	engine.ProcessEvent(inventory)
	engine.ProcessEvent(order)
	engine.ProcessEvent(gateway)

	trace, _ := engine.GetTrace("t1")
	if trace.RootService != "Gateway" {
		t.Fatalf("Expected Gateway root, got %s", trace.RootService)
	}

	snapshot := g.GetSnapshot()
	if len(snapshot.Edges) != 3 {
		t.Fatalf("Expected 3 edges, got %d", len(snapshot.Edges))
	}
	
	// Check for order -> payment
	found := false
	for _, edge := range snapshot.Edges {
		if edge.SourceService == "Order" && edge.TargetService == "Payment" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Missing Order -> Payment edge")
	}
}

func TestPartialTrace(t *testing.T) {
	g := graph.NewDependencyGraph(300)
	engine := correlation.NewEngine(g, 300)
	now := time.Now()

	// Missing child
	gateway := event.ATLASEvent{EventID: "1", EventType: event.EventTypeTraceSpan, TraceID: "t2", SpanID: "gw", ServiceName: "Gateway", Timestamp: now, Status: "OK"}
	order := event.ATLASEvent{EventID: "2", EventType: event.EventTypeTraceSpan, TraceID: "t2", SpanID: "order", ParentSpanID: "gw", ServiceName: "Order", Timestamp: now, Status: "OK"}
	
	engine.ProcessEvent(gateway)
	engine.ProcessEvent(order)
	
	// Simulate payment span never arrived (there is no event here representing payment)
	// But order might have been called, but we only reconstruct what's observed.

	trace, _ := engine.GetTrace("t2")
	if trace.SpanCount != 2 {
		t.Fatalf("Expected 2 spans")
	}
}

func TestRetention(t *testing.T) {
	g := graph.NewDependencyGraph(1)
	engine := correlation.NewEngine(g, 1)

	ev := event.ATLASEvent{
		EventID:     "1",
		EventType:   event.EventTypeTraceSpan,
		TraceID:     "t3",
		SpanID:      "s1",
		ServiceName: "Gateway",
		Timestamp:   time.Now(),
		Status:      "OK",
	}

	engine.ProcessEvent(ev)
	time.Sleep(1200 * time.Millisecond)
	engine.CleanupExpired(time.Now())

	_, exists := engine.GetTrace("t3")
	if exists {
		t.Fatalf("Trace should have expired")
	}
}

func TestEngineConcurrency(t *testing.T) {
	g := graph.NewDependencyGraph(300)
	engine := correlation.NewEngine(g, 300)
	var wg sync.WaitGroup

	now := time.Now()
	
	// 1000 concurrent events
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			engine.ProcessEvent(event.ATLASEvent{
				EventID:     "e",
				EventType:   event.EventTypeTraceSpan,
				TraceID:     "trace_concurrent",
				SpanID:      "span" + string(rune(i)),
				ParentSpanID: "root",
				ServiceName: "ServiceA",
				Timestamp:   now,
				Status:      "OK",
			})
			engine.GetTrace("trace_concurrent")
		}(i)
	}

	// Also add the root
	engine.ProcessEvent(event.ATLASEvent{
		EventID:     "root",
		EventType:   event.EventTypeTraceSpan,
		TraceID:     "trace_concurrent",
		SpanID:      "root",
		ServiceName: "Gateway",
		Timestamp:   now,
		Status:      "OK",
	})

	wg.Wait()
}
