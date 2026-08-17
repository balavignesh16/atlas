# Correlation & Dependency Reconstruction Engine

The Correlation & Dependency Reconstruction Engine in ATLAS is responsible for ingesting stream of telemetry events and correlating them into structured insights. This includes Trace Reconstruction, Trace Tree, Timeline, and Dynamic Dependency Graph construction.

## Goal and Limitations
M2.3 reconstructs **observed relationships**. It does **not** perform root-cause analysis (RCA).

If the Order service calls the Payment service, and the Payment service fails, the engine correctly attributes an error to the Payment service and marks the Order -> Payment dependency as failing. It does NOT automatically state "Payment is the root cause of Order's failure." Inference and RCA are reserved for future milestones.

## Memory Management & Bounded Indexes
The engine maintains four main indexes:
- **Trace Index**: `map[string]*traceInternal`
- **Span Index**: `map[string]*CorrelatedSpan`
- **Service Index**: `map[string]map[string]struct{}`
- **Dependency Graph**: `map[string]*DependencyEdge` and `map[string]*ServiceNode`

To prevent memory leaks, **retention** is strictly enforced. The default retention is 300 seconds (5 minutes). A background routine and programmatic calls to `CleanupExpired()` aggressively sweep old data to ensure the platform can operate indefinitely under high load without OOM crashes.

## Trace Reconstruction Features
- **Duplicate Span Deduplication**: Exact spans are deduplicated safely using the `TraceID + SpanID` composite key.
- **Out-of-Order Delivery**: The platform does not rely on telemetry arriving in chronological order. Relationships are linked using a bidirectional parent/child resolver that connects spans whenever their matching halves arrive.
- **Partial Traces**: If a downstream service fails to report its span, the trace remains valid and reconstructs what it observed.
- **Tree and Timeline Views**: Spans are structurally sorted into parent-child trees and strictly chronologically sorted timelines.

## Graph Aggregation
Service relationships are created solely by observing explicit spans. Aggregation avoids precision loss by summing total duration and dividing by call count for `AverageDurationMs` dynamically. Edge states are updated lock-free per-edge. Error counts only increment if the specific target span reported an error, enforcing strict observed fact tracking.

## Concurrency
All reads and writes to indexes and the graph are strictly protected by `sync.RWMutex` locks, making the entire engine thread-safe, race-free, and suitable for high concurrency.
