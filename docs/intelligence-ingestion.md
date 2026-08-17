# ATLAS Intelligence Ingestion Engine

This document explains the M2.2 Telemetry Ingestion and Normalization system.

## 1. Why M2.2 Exists
OpenTelemetry provides standardized trace and metric data, but ATLAS requires an internal canonical representation to drive its upcoming intelligence capabilities (e.g., event correlation, dependency graphs, AI root-cause analysis). The M2.2 milestone establishes the foundation to ingest standard OTLP data and normalize it into ATLAS Events.

## 2. Ingestion Flow (Collector → Intelligence Engine)
The business applications emit telemetry to the OpenTelemetry Collector. 
The Collector runs two pipelines:
1. `debug` exporter (stdout logs for existing verification).
2. `otlphttp` exporter (sends `application/x-protobuf` OTLP payloads to `http://atlas-intelligence-engine:8081`).

## 3. OTLP Decoding & Normalization
The Intelligence Engine natively handles POST requests to `/v1/traces` and `/v1/metrics`. It decodes the OTLP protobufs (`ExportTraceServiceRequest` / `ExportMetricsServiceRequest`) and normalizes them into the `ATLASEvent` canonical schema.

## 4. ATLAS Event Model
Every event is given a globally unique `event_id` (UUID). 
For traces, `trace_id`, `span_id`, and `parent_span_id` are preserved, along with the `service_name`, operation name, duration, and status.

## 5. Security Sanitization & Attribute Limits
Incoming attributes are untrusted. Keys containing `authorization`, `password`, `token`, `secret`, `api_key`, or `credential` are stripped completely. Remaining strings are truncated to 1024 characters, and spans are limited to 256 attributes to prevent memory exhaustion.

## 6. Bounded Event Buffer
In M2.2, a thread-safe, in-memory circular buffer is used. The buffer capacity defaults to 10,000. It employs a **DROP-OLDEST** FIFO policy. If event 10,001 arrives, event 1 is dropped, ensuring no unbounded memory growth or process panic occurs. An `events_dropped_total` metric is maintained.

## 7. Deferred Implementations (M2.3+)
- **Kafka / PostgreSQL**: Deferred until true streaming and persistence are required.
- **Event Correlation**: The engine preserves Trace IDs but does not perform causality analysis (e.g., "Payment caused the Order failure"). That requires building the Dependency Graph in M2.3.

## 8. Failure Behavior & Resiliency
The OpenTelemetry Collector uses bounded retries and queuing. If the Intelligence Engine goes down, business traffic (`Gateway -> Order -> ...`) is completely unaffected. Telemetry is queued by the Collector and either eventually delivered or dropped based on bounded configuration.
