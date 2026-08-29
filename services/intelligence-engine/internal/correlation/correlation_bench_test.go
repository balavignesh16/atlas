package correlation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
)

// buildRealisticChain constructs one real 3-hop trace (gateway -> order ->
// payment, this project's own demonstrated topology, see
// correlation_test.go's TestOutOfOrderEvents for the same shape) with a
// unique trace/span ID set. Distinct IDs per call matter: ProcessEvent
// dedups by (TraceID, SpanID), so feeding the identical trace repeatedly
// would silently degrade into no-op dedup checks after the first call and
// measure the wrong thing entirely.
func buildRealisticChain(seq int) [3]event.ATLASEvent {
	traceID := fmt.Sprintf("bench-trace-%d", seq)
	now := time.Now()
	gateway := event.ATLASEvent{
		EventID: traceID + "-gw", EventType: event.EventTypeTraceSpan,
		TraceID: traceID, SpanID: traceID + "-gw", ParentSpanID: "",
		ServiceName: "atlas-gateway", OperationName: "http post /api/orders",
		Timestamp: now, Status: "OK",
	}
	order := event.ATLASEvent{
		EventID: traceID + "-ord", EventType: event.EventTypeTraceSpan,
		TraceID: traceID, SpanID: traceID + "-ord", ParentSpanID: traceID + "-gw",
		ServiceName: "atlas-order-service", OperationName: "http post /api/orders",
		Timestamp: now.Add(2 * time.Millisecond), Status: "OK",
	}
	payment := event.ATLASEvent{
		EventID: traceID + "-pay", EventType: event.EventTypeTraceSpan,
		TraceID: traceID, SpanID: traceID + "-pay", ParentSpanID: traceID + "-ord",
		ServiceName: "atlas-payment-service", OperationName: "http post /api/payments",
		Timestamp: now.Add(4 * time.Millisecond), Status: "OK",
	}
	return [3]event.ATLASEvent{gateway, order, payment}
}

// BenchmarkProcessEvent measures the real per-trace correlation cost --
// trace reconstruction plus the two real DependencyGraph.AddDependency
// calls each 3-hop chain triggers (gateway->order, order->payment) --
// against b.N distinct, realistic traces. Reported ns/op is the cost of
// correlating ONE COMPLETE 3-SPAN TRACE (3 ProcessEvent calls), not a
// single isolated span, since that is the real unit of work this engine
// actually processes per request under real traffic.
func BenchmarkProcessEvent(b *testing.B) {
	chains := make([][3]event.ATLASEvent, b.N)
	for i := 0; i < b.N; i++ {
		chains[i] = buildRealisticChain(i)
	}

	g := graph.NewDependencyGraph(300)
	engine := correlation.NewEngine(g, 300)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.ProcessEvent(chains[i][0])
		engine.ProcessEvent(chains[i][1])
		engine.ProcessEvent(chains[i][2])
	}
}
