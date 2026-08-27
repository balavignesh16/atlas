package normalization

import (
	"fmt"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/event"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func strAttr(key, val string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: val}},
	}
}

func resourceWithService(serviceName string, extraAttrs ...*commonpb.KeyValue) *resourcepb.Resource {
	attrs := append([]*commonpb.KeyValue{strAttr("service.name", serviceName)}, extraAttrs...)
	return &resourcepb.Resource{Attributes: attrs}
}

func TestNormalizeTraces_BasicSpanMapping(t *testing.T) {
	traceID := []byte{0x01, 0x02, 0x03, 0x04}
	spanID := []byte{0xaa, 0xbb}
	startNano := uint64(1_700_000_000_000_000_000)
	endNano := startNano + 250_000_000 // +250ms

	spans := []*tracepb.ResourceSpans{{
		Resource: resourceWithService("atlas-payment-service"),
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{
				TraceId:           traceID,
				SpanId:            spanID,
				Name:              "http post /api/payments",
				Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
				StartTimeUnixNano: startNano,
				EndTimeUnixNano:   endNano,
			}},
		}},
	}}

	events := NormalizeTraces(spans)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]

	if e.EventType != event.EventTypeTraceSpan {
		t.Errorf("expected EventType=%q, got %q", event.EventTypeTraceSpan, e.EventType)
	}
	if e.ServiceName != "atlas-payment-service" {
		t.Errorf("expected ServiceName=atlas-payment-service, got %q", e.ServiceName)
	}
	if e.TraceID != "01020304" {
		t.Errorf("expected hex-encoded TraceID=01020304, got %q", e.TraceID)
	}
	if e.SpanID != "aabb" {
		t.Errorf("expected hex-encoded SpanID=aabb, got %q", e.SpanID)
	}
	if e.OperationName != "http post /api/payments" {
		t.Errorf("expected OperationName preserved, got %q", e.OperationName)
	}
	if e.Status != "OK" {
		t.Errorf("expected Status=OK, got %q", e.Status)
	}
	if e.DurationMs != 250 {
		t.Errorf("expected DurationMs=250, got %d", e.DurationMs)
	}
	if e.ParentSpanID != "" {
		t.Errorf("expected no ParentSpanID for a root span, got %q", e.ParentSpanID)
	}
	if e.EventID == "" {
		t.Error("expected a generated EventID")
	}
}

func TestNormalizeTraces_UnknownServiceFallback(t *testing.T) {
	spans := []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{}, // no service.name attribute at all
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{Name: "orphan-span"}},
		}},
	}}

	events := NormalizeTraces(spans)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ServiceName != "unknown_service" {
		t.Errorf("expected the documented unknown_service fallback, got %q", events[0].ServiceName)
	}
}

func TestNormalizeTraces_StatusCodeMapping(t *testing.T) {
	cases := []struct {
		name string
		code tracepb.Status_StatusCode
		want string
	}{
		{"ok", tracepb.Status_STATUS_CODE_OK, "OK"},
		{"error", tracepb.Status_STATUS_CODE_ERROR, "ERROR"},
		{"unset", tracepb.Status_STATUS_CODE_UNSET, "UNSET"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spans := []*tracepb.ResourceSpans{{
				Resource: resourceWithService("svc"),
				ScopeSpans: []*tracepb.ScopeSpans{{
					Spans: []*tracepb.Span{{Name: "op", Status: &tracepb.Status{Code: c.code}}},
				}},
			}}
			events := NormalizeTraces(spans)
			if events[0].Status != c.want {
				t.Errorf("expected Status=%q, got %q", c.want, events[0].Status)
			}
		})
	}
}

func TestNormalizeTraces_NoStatusField_DefaultsToUnset(t *testing.T) {
	// A span with no Status message at all (nil) -- GetStatus().GetCode()
	// on a nil *Status must not panic and must fall into the UNSET default.
	spans := []*tracepb.ResourceSpans{{
		Resource:   resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "op"}}}},
	}}
	events := NormalizeTraces(spans)
	if events[0].Status != "UNSET" {
		t.Errorf("expected UNSET for a span with no Status message, got %q", events[0].Status)
	}
}

func TestNormalizeTraces_ParentSpanID_PresentAndAbsent(t *testing.T) {
	withParent := []*tracepb.ResourceSpans{{
		Resource: resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{Name: "child", ParentSpanId: []byte{0x9, 0x9}}},
		}},
	}}
	events := NormalizeTraces(withParent)
	if events[0].ParentSpanID != "0909" {
		t.Errorf("expected hex-encoded ParentSpanID=0909, got %q", events[0].ParentSpanID)
	}

	withoutParent := []*tracepb.ResourceSpans{{
		Resource:   resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "root"}}}},
	}}
	events = NormalizeTraces(withoutParent)
	if events[0].ParentSpanID != "" {
		t.Errorf("expected empty ParentSpanID for a root span, got %q", events[0].ParentSpanID)
	}
}

func TestNormalizeTraces_ZeroStartTime_FallsBackToNow(t *testing.T) {
	before := time.Now()
	spans := []*tracepb.ResourceSpans{{
		Resource:   resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "op"}}}}, // StartTimeUnixNano left at zero-value
	}}
	events := NormalizeTraces(spans)
	after := time.Now()

	ts := events[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("expected Timestamp to fall back to time.Now() when StartTimeUnixNano is 0, got %v (window %v..%v)", ts, before, after)
	}
	if events[0].DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when no start time was provided to compute a duration from, got %d", events[0].DurationMs)
	}
}

func TestNormalizeTraces_SensitiveAttributesExcluded(t *testing.T) {
	spans := []*tracepb.ResourceSpans{{
		Resource: resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{
				Name: "op",
				Attributes: []*commonpb.KeyValue{
					strAttr("http.method", "POST"),
					strAttr("Authorization", "Bearer secret-token-value"),
					strAttr("my_api_key_value", "should-not-appear"), // matches the literal "api_key" token
				},
			}},
		}},
	}}

	events := NormalizeTraces(spans)
	attrs := events[0].Attributes

	if attrs["http.method"] != "POST" {
		t.Errorf("expected the non-sensitive attribute to be preserved, got %q", attrs["http.method"])
	}
	if _, present := attrs["Authorization"]; present {
		t.Error("expected the Authorization attribute to be excluded as sensitive")
	}
	if _, present := attrs["my_api_key_value"]; present {
		t.Error("expected the my_api_key_value attribute to be excluded as sensitive")
	}
}

// TestNormalizeTraces_HyphenatedApiKey_NotCaught documents a real, discovered
// gap (see docs/m210_verification_report.md's Findings): IsSensitiveAttribute
// only matches the literal substring "api_key" (underscore). The equally
// common hyphenated convention "x-api-key" / "api-key" -- notably including
// this project's own M2.9 header name, X-Atlas-Api-Key -- is NOT caught.
// This test encodes that as CURRENT, ACTUAL behavior; it is not an
// endorsement, and this milestone does not change sanitizer.go to fix it.
func TestNormalizeTraces_HyphenatedApiKey_NotCaught(t *testing.T) {
	spans := []*tracepb.ResourceSpans{{
		Resource: resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{
				Name:       "op",
				Attributes: []*commonpb.KeyValue{strAttr("x-api-key", "leaks-today")},
			}},
		}},
	}}

	events := NormalizeTraces(spans)
	if _, present := events[0].Attributes["x-api-key"]; !present {
		t.Fatal("this test is documenting current (gap) behavior and should be revisited if IsSensitiveAttribute's token list ever changes")
	}
}

func TestNormalizeTraces_AttributeCountCap(t *testing.T) {
	var attrs []*commonpb.KeyValue
	for i := 0; i < 300; i++ {
		attrs = append(attrs, strAttr(fmt.Sprintf("attr_%d", i), "v"))
	}
	spans := []*tracepb.ResourceSpans{{
		Resource:   resourceWithService("svc"),
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "op", Attributes: attrs}}}},
	}}

	events := NormalizeTraces(spans)
	if got := len(events[0].Attributes); got != 256 {
		t.Errorf("expected the documented 256-attribute cap to be enforced, got %d attributes", got)
	}
}

func TestNormalizeTraces_MultipleResourceAndScopeSpans_Flattened(t *testing.T) {
	spans := []*tracepb.ResourceSpans{
		{
			Resource: resourceWithService("svc-a"),
			ScopeSpans: []*tracepb.ScopeSpans{
				{Spans: []*tracepb.Span{{Name: "a1"}, {Name: "a2"}}},
				{Spans: []*tracepb.Span{{Name: "a3"}}},
			},
		},
		{
			Resource:   resourceWithService("svc-b"),
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "b1"}}}},
		},
	}

	events := NormalizeTraces(spans)
	if len(events) != 4 {
		t.Fatalf("expected all spans across every resource/scope to flatten into 4 events, got %d", len(events))
	}
}

func TestNormalizeTraces_EmptyInput_ReturnsNoEvents(t *testing.T) {
	events := NormalizeTraces(nil)
	if len(events) != 0 {
		t.Fatalf("expected no events for nil input, got %d", len(events))
	}
}

func numberDataPoint(withDouble bool, val float64, timeNano uint64) *metricpb.NumberDataPoint {
	dp := &metricpb.NumberDataPoint{TimeUnixNano: timeNano}
	if withDouble {
		dp.Value = &metricpb.NumberDataPoint_AsDouble{AsDouble: val}
	} else {
		dp.Value = &metricpb.NumberDataPoint_AsInt{AsInt: int64(val)}
	}
	return dp
}

func TestNormalizeMetrics_Sum_AsDouble(t *testing.T) {
	rm := []*metricpb.ResourceMetrics{{
		Resource: resourceWithService("svc"),
		ScopeMetrics: []*metricpb.ScopeMetrics{{
			Metrics: []*metricpb.Metric{{
				Name: "request.count",
				Data: &metricpb.Metric_Sum{Sum: &metricpb.Sum{
					DataPoints: []*metricpb.NumberDataPoint{numberDataPoint(true, 42.5, 1_700_000_000_000_000_000)},
				}},
			}},
		}},
	}}

	events := NormalizeMetrics(rm)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.EventType != event.EventTypeMetric {
		t.Errorf("expected EventType=%q, got %q", event.EventTypeMetric, e.EventType)
	}
	if e.MetricType != "Sum" {
		t.Errorf("expected MetricType=Sum, got %q", e.MetricType)
	}
	if e.MetricName != "request.count" {
		t.Errorf("expected MetricName preserved, got %q", e.MetricName)
	}
	if e.Value != 42.5 {
		t.Errorf("expected Value=42.5, got %v", e.Value)
	}
}

func TestNormalizeMetrics_Gauge_AsInt(t *testing.T) {
	rm := []*metricpb.ResourceMetrics{{
		Resource: resourceWithService("svc"),
		ScopeMetrics: []*metricpb.ScopeMetrics{{
			Metrics: []*metricpb.Metric{{
				Name: "queue.depth",
				Data: &metricpb.Metric_Gauge{Gauge: &metricpb.Gauge{
					DataPoints: []*metricpb.NumberDataPoint{numberDataPoint(false, 7, 1_700_000_000_000_000_000)},
				}},
			}},
		}},
	}}

	events := NormalizeMetrics(rm)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].MetricType != "Gauge" {
		t.Errorf("expected MetricType=Gauge, got %q", events[0].MetricType)
	}
	if events[0].Value != 7 {
		t.Errorf("expected Value=7, got %v", events[0].Value)
	}
}

func TestNormalizeMetrics_UnsupportedMetricType_ProducesNoEvents(t *testing.T) {
	// A Metric with neither Sum nor Gauge set (Data left nil) -- documented,
	// real behavior: normalizeMetric only has cases for Sum and Gauge, with
	// no else branch, so anything else silently produces zero events.
	rm := []*metricpb.ResourceMetrics{{
		Resource: resourceWithService("svc"),
		ScopeMetrics: []*metricpb.ScopeMetrics{{
			Metrics: []*metricpb.Metric{{Name: "unsupported.metric"}},
		}},
	}}

	events := NormalizeMetrics(rm)
	if len(events) != 0 {
		t.Fatalf("expected 0 events for an unsupported metric type, got %d", len(events))
	}
}

func TestNormalizeMetrics_ZeroTimestamp_FallsBackToNow(t *testing.T) {
	before := time.Now()
	rm := []*metricpb.ResourceMetrics{{
		Resource: resourceWithService("svc"),
		ScopeMetrics: []*metricpb.ScopeMetrics{{
			Metrics: []*metricpb.Metric{{
				Name: "m",
				Data: &metricpb.Metric_Sum{Sum: &metricpb.Sum{
					DataPoints: []*metricpb.NumberDataPoint{numberDataPoint(true, 1, 0)}, // TimeUnixNano left at 0
				}},
			}},
		}},
	}}
	events := NormalizeMetrics(rm)
	after := time.Now()

	ts := events[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("expected Timestamp to fall back to time.Now() when TimeUnixNano is 0, got %v (window %v..%v)", ts, before, after)
	}
}
