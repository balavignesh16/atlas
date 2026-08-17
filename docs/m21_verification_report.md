# M2.1 Telemetry Foundation Verification Report

## Verification Environment
- **Platform**: Docker Compose (`docker-compose up -d --build`)
- **Collector Version**: `otel-collector` (v0.106.1)
- **Services Instrumented**:
  - `gateway` (Port 8083)
  - `order-service` (Port 8084)
  - `inventory-service` (Port 8085)
  - `payment-service` (Port 8086)
- **Tracing Library**: Micrometer Tracing with OpenTelemetry Bridge
- **Exporter**: OpenTelemetry OTLP Exporter (`opentelemetry-exporter-otlp`)

## Verification Summary

1. **Native Regression Testing** (`make test`):
   - Verified that the M0-M1.4 business behaviors (timeouts, idempotency, compensations) are unaffected by the telemetry injection.
   - All tests passed successfully.

2. **Docker E2E Execution** (`test-m21-docker.ps1`):
   - **Test 1 (Successful Flow)**: Order processed through `Gateway -> Order -> Inventory & Payment` with success HTTP 201.
   - **Test 2 (Timeout Tolerance)**: Simulated Payment Gateway timeout correctly yielded `HTTP 504 Gateway Timeout` without hanging the application.
   - **Test 3 (Compensation)**: Simulated Inventory rejection properly compensated and yielded `HTTP 409 Conflict`.
   
3. **Resilience & Collector Outage Tolerance**:
   - The OpenTelemetry Collector was explicitly stopped (`docker-compose stop otel-collector`).
   - Business traffic was sent through the Gateway with correlation ID `ATLAS-M21-COLLECTOR-DOWN`.
   - **Result**: Order successfully created (`HTTP 201`). The business logic did **not** break or time out due to the collector being unavailable.
   - The Collector was restarted (`docker-compose start otel-collector`) and traffic successfully recovered (`ATLAS-M21-COLLECTOR-UP`).

## OTLP Exporter Verification
- Confirmed that `micrometer-tracing-bridge-otel`, `opentelemetry-exporter-otlp`, and `micrometer-registry-otlp` are bundled in all Spring Boot applications.
- Confirmed that `application.yml` safely injects `OTEL_EXPORTER_OTLP_ENDPOINT` via Docker environment variables without hardcoding.
- Verified that `CorrelationIdFilter` attaches the application-level `X-Correlation-ID` context to the OpenTelemetry `Span` attributes, allowing exact cross-referencing between application correlation IDs and OTel Trace IDs.

## 4. Telemetry Evidence (Actual Collector Output)

### Trace 1: Successful Order (ATLAS-M21-TRACE-PROOF)
Correlation ID `ATLAS-M21-TRACE-PROOF` maps to Trace ID `9440596e679e15f9d2d758ff1871d4f5`.
The Collector received spans from `atlas-gateway`, `atlas-order-service`, `atlas-inventory-service`, and `atlas-payment-service` sharing the exact same Trace ID.

**Parent/Child Relationship Proof:**
```text
Gateway Span
    Trace ID       : 9440596e679e15f9d2d758ff1871d4f5
    Parent ID      : 
    ID             : 1af87e5b12850942
    Name           : http post /api/orders
    Attributes     : service.name=atlas-gateway, correlation_id=ATLAS-M21-TRACE-PROOF

Order Service Span (Child of Gateway)
    Trace ID       : 9440596e679e15f9d2d758ff1871d4f5
    Parent ID      : 1af87e5b12850942
    ID             : e906cbdba3bbc078
    Name           : http post /api/orders
    Attributes     : service.name=atlas-order-service, correlation_id=ATLAS-M21-TRACE-PROOF

Inventory Service Span (Child of Order)
    Trace ID       : 9440596e679e15f9d2d758ff1871d4f5
    Parent ID      : e906cbdba3bbc078
    ID             : 6802f27b3f5e3ada
    Name           : http post /api/inventory/P100/reserve
    Attributes     : service.name=atlas-inventory-service, correlation_id=ATLAS-M21-TRACE-PROOF

Payment Service Span (Child of Order)
    Trace ID       : 9440596e679e15f9d2d758ff1871d4f5
    Parent ID      : e906cbdba3bbc078
    ID             : 724fdbbb2216f27c
    Name           : http post /api/payments
    Attributes     : service.name=atlas-payment-service, correlation_id=ATLAS-M21-TRACE-PROOF
```

### Trace 2: Payment Timeout (ATLAS-M21-TIMEOUT-PROOF)
Correlation ID `ATLAS-M21-TIMEOUT-PROOF` maps to Trace ID `fa856b323521c35cbfae1a796a478fb8`.

```text
Order Service Span (Failing to call Payment)
    Trace ID       : fa856b323521c35cbfae1a796a478fb8
    ID             : 575189185b712719
    Name           : http post /api/orders
    Status         : 504
    Attributes     : service.name=atlas-order-service, outcome=SERVER_ERROR, exception.type=org.springframework.web.client.RestClientException
```

### Trace 3: Inventory Failure (ATLAS-M21-INVENTORY-PROOF)
Correlation ID `ATLAS-M21-INVENTORY-PROOF` maps to Trace ID `0e7250f04b622861af57c3703332d8a4`.

```text
Order Service Span (Calling Inventory)
    Trace ID       : 0e7250f04b622861af57c3703332d8a4
    ID             : 35e047a0a05eddbf
    Name           : http post /api/orders
    Status         : 409
    Status Message : "Insufficient inventory"
    Attributes     : service.name=atlas-order-service, outcome=CLIENT_ERROR, exception=Conflict
```

### Metric Export Verification
Metrics successfully reached the Collector via OTLP (`/v1/metrics`). Example Collector output proving arrival:

```text
info    MetricsExporter {"kind": "exporter", "data_type": "metrics", "name": "debug", "resource metrics": 1, "metrics": 53, "data points": 95}
Resource attributes:
     -> telemetry.sdk.name: Str(io.micrometer)
     -> service.name: Str(atlas-order-service)
Metric #0
     -> Name: jvm.threads.peak
     -> Description: The peak live thread count since the Java virtual machine started or peak was reset
```

## Conclusion
Milestone 2.1 is fully verified. The ATLAS lab applications are successfully instrumented with the OpenTelemetry foundation, and their core business flow remains perfectly resilient to telemetry infrastructure outages. The telemetry has been definitively proven to arrive at the OpenTelemetry Collector with proper contextual correlation intact.
