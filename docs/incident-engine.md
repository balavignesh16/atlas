# Incident Detection and Root Cause Analysis Engine (M2.4)

## Overview
ATLAS M2.4 introduces the Incident Detection and deterministic Root Cause Analysis (RCA) engine. This system transforms raw, normalized, and correlated telemetry into actionable incident lifecycle management.

M2.4 performs deterministic evidence-based reasoning. It does not provide mathematical proof of causality.

## Observation Window
The engine relies on a bounded, thread-safe sliding observation window (`ATLAS_INCIDENT_WINDOW_SECONDS`, default 60s). It tracks the recent history of each service operation to compute:
- Error rates
- Average latency
- P95 / P99 latency

## Detection Rules and Thresholds
The `IncidentDetector` performs continuous background evaluation of the active windows against deterministic thresholds:
- `ATLAS_ERROR_RATE_THRESHOLD`: 20%
- `ATLAS_LATENCY_THRESHOLD_MS`: 1000ms
- `ATLAS_DEPENDENCY_ERROR_RATE_THRESHOLD`: 20%

### Minimum Observations
To prevent single-request false positives, a minimum number of observations must be met within the window before an incident signal is generated.
- `ATLAS_MIN_OBSERVATIONS_FOR_INCIDENT`: 10

## Incident Deduplication
Incidents are deduplicated using a deterministic fingerprint: `service|operation|failure_class`. If an incident with the same fingerprint is already OPEN, the existing incident is updated with the latest evidence instead of creating a new one.

## Incident Lifecycle
1. **OPEN**: Automatically created when thresholds are breached.
2. **RESOLVED**: Transitioned automatically when healthy behavior is observed for the configured recovery period.

### Recovery
Default recovery period: `ATLAS_INCIDENT_RECOVERY_SECONDS` (30s). The incident must not resolve after a single successful request; traffic must remain healthy for the duration of the recovery period.

### Retention
Resolved incidents are retained for `ATLAS_INCIDENT_RETENTION_SECONDS` (3600s). OPEN incidents are never deleted, ensuring they remain visible until resolved.

## Evidence and Blast Radius
RCA conclusions must be supported by evidence. Evidence includes error rates, latency spikes, span errors, temporal sequence precedence, and upstream healthy signals.
The engine calculates the **Blast Radius** by determining all `AffectedServices`, `AffectedOperations`, and `AffectedEdges` through graph and trace reconstruction.

## Failure Propagation and RCA Candidate Generation
The Propagation Analyzer examines timestamps and dependency edges. Candidates are generated from the affected services.
A deterministic 100-point scoring algorithm is applied:
- **Error increase**: +25
- **Latency increase**: +20
- **Dependency failure**: +20
- **Temporal precedence**: +20
- **Downstream propagation**: +10
- **Healthy independent dependency**: +5

## Confidence and Ambiguity
- **LOW**: 0-39
- **MEDIUM**: 40-69
- **HIGH**: 70-100

If two candidates score within `ATLAS_RCA_AMBIGUITY_MARGIN` (5 points) of each other, the RCA Engine will output `AMBIGUOUS`. ATLAS will not arbitrarily guess the root cause.

## Limitations
Root cause is probabilistic and based only on observed telemetry. If telemetry is missing, ATLAS will reflect it as incomplete evidence and will not fabricate conclusions.
