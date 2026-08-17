package normalization

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/google/uuid"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NormalizeTraces maps OTLP trace requests to ATLASEvents
func NormalizeTraces(resourceSpans []*tracepb.ResourceSpans) []event.ATLASEvent {
	var events []event.ATLASEvent

	for _, resourceSpan := range resourceSpans {
		serviceName := getServiceName(resourceSpan.GetResource())
		environment := getAttributeString(resourceSpan.GetResource().GetAttributes(), "deployment.environment")

		for _, scopeSpan := range resourceSpan.GetScopeSpans() {
			for _, span := range scopeSpan.GetSpans() {
				events = append(events, normalizeSpan(span, serviceName, environment))
			}
		}
	}

	return events
}

func normalizeSpan(span *tracepb.Span, serviceName, environment string) event.ATLASEvent {
	e := event.ATLASEvent{
		EventID:       uuid.New().String(),
		EventType:     event.EventTypeTraceSpan,
		ServiceName:   serviceName,
		Environment:   environment,
		TraceID:       hex.EncodeToString(span.GetTraceId()),
		SpanID:        hex.EncodeToString(span.GetSpanId()),
		OperationName: span.GetName(),
	}

	if len(span.GetParentSpanId()) > 0 {
		e.ParentSpanID = hex.EncodeToString(span.GetParentSpanId())
	}

	// Status
	switch span.GetStatus().GetCode() {
	case tracepb.Status_STATUS_CODE_OK:
		e.Status = "OK"
	case tracepb.Status_STATUS_CODE_ERROR:
		e.Status = "ERROR"
	default:
		e.Status = "UNSET"
	}
	e.StatusMessage = span.GetStatus().GetMessage()

	// Timestamps
	if span.GetStartTimeUnixNano() > 0 {
		e.Timestamp = time.Unix(0, int64(span.GetStartTimeUnixNano()))
		if span.GetEndTimeUnixNano() > 0 {
			e.DurationMs = int64((span.GetEndTimeUnixNano() - span.GetStartTimeUnixNano()) / 1e6)
		}
	} else {
		e.Timestamp = time.Now()
	}

	// Attributes
	e.Attributes = extractSafeAttributes(span.GetAttributes())

	return e
}

// NormalizeMetrics maps OTLP metrics requests to ATLASEvents
func NormalizeMetrics(resourceMetrics []*metricpb.ResourceMetrics) []event.ATLASEvent {
	var events []event.ATLASEvent

	for _, rm := range resourceMetrics {
		serviceName := getServiceName(rm.GetResource())
		environment := getAttributeString(rm.GetResource().GetAttributes(), "deployment.environment")

		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				events = append(events, normalizeMetric(m, serviceName, environment)...)
			}
		}
	}

	return events
}

func normalizeMetric(m *metricpb.Metric, serviceName, environment string) []event.ATLASEvent {
	var events []event.ATLASEvent

	base := event.ATLASEvent{
		EventType:   event.EventTypeMetric,
		ServiceName: serviceName,
		Environment: environment,
		MetricName:  m.GetName(),
		MetricDesc:  m.GetDescription(),
		Unit:        m.GetUnit(),
	}

	// We support Sum and Gauge (common)
	if sum := m.GetSum(); sum != nil {
		base.MetricType = "Sum"
		for _, dp := range sum.GetDataPoints() {
			events = append(events, buildMetricDataPointEvent(base, dp))
		}
	} else if gauge := m.GetGauge(); gauge != nil {
		base.MetricType = "Gauge"
		for _, dp := range gauge.GetDataPoints() {
			events = append(events, buildMetricDataPointEvent(base, dp))
		}
	}

	return events
}

func buildMetricDataPointEvent(base event.ATLASEvent, dp *metricpb.NumberDataPoint) event.ATLASEvent {
	e := base
	e.EventID = uuid.New().String()
	e.Attributes = extractSafeAttributes(dp.GetAttributes())

	if dp.GetTimeUnixNano() > 0 {
		e.Timestamp = time.Unix(0, int64(dp.GetTimeUnixNano()))
	} else {
		e.Timestamp = time.Now()
	}

	switch dp.GetValue().(type) {
	case *metricpb.NumberDataPoint_AsDouble:
		e.Value = dp.GetAsDouble()
	case *metricpb.NumberDataPoint_AsInt:
		e.Value = float64(dp.GetAsInt())
	}

	return e
}

// Helpers
func getServiceName(res *resourcepb.Resource) string {
	name := getAttributeString(res.GetAttributes(), "service.name")
	if name == "" {
		return "unknown_service"
	}
	return name
}

func getAttributeString(attributes []*commonpb.KeyValue, key string) string {
	for _, attr := range attributes {
		if attr.GetKey() == key {
			return attr.GetValue().GetStringValue()
		}
	}
	return ""
}

func extractSafeAttributes(attributes []*commonpb.KeyValue) map[string]string {
	safe := make(map[string]string)
	count := 0
	for _, attr := range attributes {
		if count >= 256 { // Limit to 256 attributes
			break
		}
		if IsSensitiveAttribute(attr.GetKey()) {
			continue
		}

		val := ""
		switch v := attr.GetValue().GetValue().(type) {
		case *commonpb.AnyValue_StringValue:
			val = v.StringValue
		case *commonpb.AnyValue_IntValue:
			val = fmt.Sprintf("%d", v.IntValue)
		case *commonpb.AnyValue_DoubleValue:
			val = fmt.Sprintf("%f", v.DoubleValue)
		case *commonpb.AnyValue_BoolValue:
			val = fmt.Sprintf("%t", v.BoolValue)
		}

		if val != "" {
			safe[attr.GetKey()] = SanitizeString(val, 1024)
			count++
		}
	}
	return safe
}
