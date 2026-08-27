package ingestion

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"

	metriccollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// newTestHandler wires a real, unmocked OTLPHandler against real
// EventBuffer/correlation.Engine/incidentdetector.Detector instances,
// matching this project's established "exercise the real implementation"
// testing convention.
func newTestHandler(t *testing.T) (*OTLPHandler, *buffer.EventBuffer) {
	t.Helper()
	buf := buffer.NewEventBuffer(100)
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	signals := make(chan incidentsignal.Signal, 10)
	detector := incidentdetector.NewDetector(incidentdetector.DefaultConfig(), depGraph, signals)
	return NewOTLPHandler(buf, corrEngine, detector), buf
}

func validTraceBody(t *testing.T, serviceName, spanName string) []byte {
	t.Helper()
	req := &tracecollectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{{
					Key:   "service.name",
					Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: serviceName}},
				}},
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Spans: []*tracepb.Span{{
					Name:              spanName,
					TraceId:           []byte{1, 2, 3, 4},
					SpanId:            []byte{5, 6},
					StartTimeUnixNano: uint64(time.Now().UnixNano()),
				}},
			}},
		}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal test trace request: %v", err)
	}
	return body
}

func validMetricBody(t *testing.T, serviceName, metricName string) []byte {
	t.Helper()
	req := &metriccollectorpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{{
					Key:   "service.name",
					Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: serviceName}},
				}},
			},
			ScopeMetrics: []*metricpb.ScopeMetrics{{
				Metrics: []*metricpb.Metric{{
					Name: metricName,
					Data: &metricpb.Metric_Gauge{Gauge: &metricpb.Gauge{
						DataPoints: []*metricpb.NumberDataPoint{{
							TimeUnixNano: uint64(time.Now().UnixNano()),
							Value:        &metricpb.NumberDataPoint_AsDouble{AsDouble: 1.0},
						}},
					}},
				}},
			}},
		}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal test metrics request: %v", err)
	}
	return body
}

func gzipCompress(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		t.Fatalf("failed to gzip-compress test body: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func TestHandleTraces_WrongMethod_Returns405(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleTraces_MalformedProtobuf_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("this is not a valid protobuf message")))
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed protobuf, got %d", rec.Code)
	}
}

func TestHandleTraces_MalformedGzip_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("not actually gzip data")))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a Content-Encoding: gzip body that isn't valid gzip, got %d", rec.Code)
	}
}

func TestHandleTraces_ValidGzipCompressedPayload_DecodesAndDispatches(t *testing.T) {
	h, buf := newTestHandler(t)
	body := gzipCompress(t, validTraceBody(t, "atlas-payment-service", "http post /api/payments"))

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid gzip-compressed payload, got %d", rec.Code)
	}
	events := buf.GetAll()
	if len(events) != 1 {
		t.Fatalf("expected the gzip payload to be decompressed and its 1 span dispatched to the buffer, got %d events", len(events))
	}
	if events[0].ServiceName != "atlas-payment-service" {
		t.Errorf("expected the decompressed event's ServiceName to survive intact, got %q", events[0].ServiceName)
	}
}

func TestHandleTraces_ValidUncompressedPayload_DispatchesToBuffer(t *testing.T) {
	h, buf := newTestHandler(t)
	body := validTraceBody(t, "atlas-order-service", "http post /api/orders")

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	events := buf.GetAll()
	if len(events) != 1 {
		t.Fatalf("expected 1 event in the buffer, got %d", len(events))
	}
	if events[0].OperationName != "http post /api/orders" {
		t.Errorf("expected OperationName to survive the ingest->normalize->buffer path intact, got %q", events[0].OperationName)
	}
}

func TestHandleTraces_EmptyValidPayload_ReturnsOKWithNoEvents(t *testing.T) {
	h, buf := newTestHandler(t)
	body, err := proto.Marshal(&tracecollectorpb.ExportTraceServiceRequest{})
	if err != nil {
		t.Fatalf("failed to marshal empty request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleTraces(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a structurally valid but empty payload, got %d", rec.Code)
	}
	if got := len(buf.GetAll()); got != 0 {
		t.Fatalf("expected no events for an empty payload, got %d", got)
	}
}

func TestHandleMetrics_WrongMethod_Returns405(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	rec := httptest.NewRecorder()

	h.HandleMetrics(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleMetrics_MalformedProtobuf_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader([]byte("garbage")))
	rec := httptest.NewRecorder()

	h.HandleMetrics(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed protobuf, got %d", rec.Code)
	}
}

func TestHandleMetrics_ValidPayload_AddsToBufferOnly(t *testing.T) {
	h, buf := newTestHandler(t)
	body := validMetricBody(t, "atlas-gateway", "request.latency")

	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	events := buf.GetAll()
	if len(events) != 1 {
		t.Fatalf("expected 1 metric event in the buffer, got %d", len(events))
	}
	if events[0].MetricName != "request.latency" {
		t.Errorf("expected MetricName to survive intact, got %q", events[0].MetricName)
	}
}
