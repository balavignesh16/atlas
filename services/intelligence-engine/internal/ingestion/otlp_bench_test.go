package ingestion

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/registry"

	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// benchTraceBody mirrors otlp_test.go's validTraceBody in shape (one
// resource, one span, service.name attribute) but takes no *testing.T --
// validTraceBody itself can't be reused directly from a benchmark since it
// calls t.Fatalf on a *testing.T specifically, and this file must not
// modify otlp_test.go. seq makes each body's trace/span IDs and service
// name genuinely distinct, so HandleTraces does real decode+normalize+
// buffer+correlate+registry-observe work on every call rather than
// degrading into a correlation/registry no-op after the first identical
// payload.
func benchTraceBody(seq int) ([]byte, error) {
	services := []string{"atlas-gateway", "atlas-order-service", "atlas-payment-service", "atlas-inventory-service"}
	service := services[seq%len(services)]

	traceID := make([]byte, 16)
	spanID := make([]byte, 8)
	for i := range traceID {
		traceID[i] = byte(seq >> (i % 8))
	}
	for i := range spanID {
		spanID[i] = byte((seq + 1) >> (i % 8))
	}

	req := &tracecollectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{{
					Key:   "service.name",
					Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: service}},
				}},
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Spans: []*tracepb.Span{{
					Name:              fmt.Sprintf("http post /api/bench-%d", seq),
					TraceId:           traceID,
					SpanId:            spanID,
					StartTimeUnixNano: uint64(time.Now().UnixNano()),
				}},
			}},
		}},
	}
	return proto.Marshal(req)
}

// BenchmarkHandleTraces measures the real HTTP ingestion entry point end to
// end: protobuf decode -> normalization.NormalizeTraces -> EventBuffer.Add
// -> correlation.Engine.ProcessEvent -> registry.Store.Observe, using the
// same real, unmocked construction as newTestHandler (otlp_test.go). This
// is the single most representative "real sustained OTLP load" measurement
// in this module -- the literal front door.
//
// All b.N request bodies are marshaled BEFORE b.ResetTimer() so that
// client-side fixture construction (marshal) is never counted as part of
// the server-side decode cost being measured; only the per-iteration
// httptest.NewRequest/NewRecorder construction (an unavoidable, minimal
// per-call cost of driving the real http.Handler interface) happens inside
// the timed loop.
func BenchmarkHandleTraces(b *testing.B) {
	bodies := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		body, err := benchTraceBody(i)
		if err != nil {
			b.Fatalf("failed to marshal benchmark trace body: %v", err)
		}
		bodies[i] = body
	}

	buf := buffer.NewEventBuffer(10000)
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	signals := make(chan incidentsignal.Signal, 10000)
	detector := incidentdetector.NewDetector(incidentdetector.DefaultConfig(), depGraph, signals)
	reg, err := registry.NewStore(":memory:")
	if err != nil {
		b.Fatalf("registry.NewStore: %v", err)
	}
	defer reg.Close()
	handler := NewOTLPHandler(buf, corrEngine, detector, reg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(bodies[i]))
		rec := httptest.NewRecorder()
		handler.HandleTraces(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("HandleTraces returned %d, want 200", rec.Code)
		}
	}
}
