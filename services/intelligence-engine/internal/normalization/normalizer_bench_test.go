package normalization

import (
	"fmt"
	"testing"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// buildRealisticTraceBatch constructs one OTLP export batch shaped like a
// real ATLAS trace: a gateway -> order -> payment/inventory fan-out, the
// same 4-service topology this project's own live Docker verification runs
// have repeatedly exercised (see Modules 3/4/5's release reviews). Each of
// the 4 services contributes 5 spans (20 spans total), a realistic size for
// one real OTel SDK export interval, not an arbitrarily inflated batch.
func buildRealisticTraceBatch() []*tracepb.ResourceSpans {
	services := []string{"atlas-gateway", "atlas-order-service", "atlas-payment-service", "atlas-inventory-service"}
	spansPerService := 5

	resourceSpans := make([]*tracepb.ResourceSpans, 0, len(services))
	startNano := uint64(1_700_000_000_000_000_000)

	for si, service := range services {
		spans := make([]*tracepb.Span, 0, spansPerService)
		for i := 0; i < spansPerService; i++ {
			spanStart := startNano + uint64(i)*10_000_000 // 10ms apart
			spans = append(spans, &tracepb.Span{
				TraceId:           []byte{byte(si), byte(i), 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				SpanId:            []byte{byte(si), byte(i), 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				Name:              fmt.Sprintf("http post /api/%s/op-%d", service, i),
				Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
				StartTimeUnixNano: spanStart,
				EndTimeUnixNano:   spanStart + 25_000_000, // 25ms duration
			})
		}
		resourceSpans = append(resourceSpans, &tracepb.ResourceSpans{
			Resource:   resourceWithService(service),
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}},
		})
	}
	return resourceSpans
}

// BenchmarkNormalizeTraces measures the real per-span mapping/sanitization
// cost (internal/normalization/normalizer.go's NormalizeTraces) against a
// realistic 20-span, 4-service batch. Fixture construction happens entirely
// before b.ResetTimer() so only the actual normalization work is measured.
func BenchmarkNormalizeTraces(b *testing.B) {
	spans := buildRealisticTraceBatch()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		events := NormalizeTraces(spans)
		if len(events) == 0 {
			b.Fatal("NormalizeTraces returned zero events for a non-empty realistic batch")
		}
	}
}
