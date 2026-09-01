# ATLAS Framework Boundary (Module 7)

This document names, precisely and honestly, which parts of ATLAS are
already generic (reusable for services/providers it has never seen) and
which parts are not. Every claim below is traced against real source, not
assumed from prior reports. It distinguishes three categories throughout:
**already implemented and verified**, **intentionally deferred**, and
**explicitly out of scope**.

## Already implemented and verified

### Provider boundaries

- **`aireasoning.ReasoningProvider`** (`internal/aireasoning/model.go`) --
  one method, `Analyze(ctx, *IncidentAnalysisContext) (*AnalysisResult, error)`.
  Two implementations exist: `FakeProvider` (deterministic, what actually
  runs today) and `GeminiProvider` (an intentional, unimplemented
  placeholder -- see "Explicitly out of scope" below). Module 4 verified
  this interface requires no changes to be provider-agnostic.
- **`remediation.RemediationPlannerProvider`** (`internal/remediation/model.go`) --
  one method, `GeneratePlan(ctx, *RemediationContext) (*RemediationPlan, error)`.
  Three implementations: `FakePlanner`, `FallbackPlanner` (deterministic,
  non-AI, always available), and `AIPlanner` (placeholder).
- **`execution.ExecutorProvider`** (`internal/execution/engine.go`) --
  `RestartService`/`Observe`/`Investigate`. Two implementations: a real
  Docker adapter (`internal/infrastructure/docker`) and `FakeExecutor`.
  Structurally ready for a non-Docker adapter without touching
  `internal/execution` itself.

### RBAC boundary

`internal/security`'s `Permission`/`Role` model (`VIEW`, `CREATE_PLAN`,
`APPROVE_PLAN`, `EXECUTE`, `READ_AUDIT`; `VIEWER`/`OPERATOR`/`APPROVER`/
`EXECUTOR`/`ADMIN`) is a pure, static data mapping with zero service-name
coupling. Keys are configured via one environment variable
(`ATLAS_API_KEYS`, `name:key:role` tuples) -- adding a new role or
principal requires no code change.

### Registry evidence-source vocabulary

`internal/registry`'s `Provenance` enum (`OBSERVED_TELEMETRY`, `DECLARED`,
`DOCKER`, `KUBERNETES`, `CONFIG`, `INFERRED`) already anticipates
non-telemetry evidence sources; `ConfidenceFor`/`ShouldSupersede`/
`EvaluateLifecycle` are pure functions of the enum and timestamps, with no
branching on any specific service name.

### Generic telemetry/evidence ingestion

`internal/ingestion` and `internal/normalization` derive every field
(`service.name`, span identity, status, duration) from the real OTLP
payload's own attributes -- zero service-name branching anywhere.

### Generic incident lifecycle

`internal/incidentmanager`/`internal/incidentmodel` key incidents purely on
real signal data (`Fingerprint` derived from `service|operation|type`) --
zero service-name branching.

### Generic graph/intelligence contracts

`internal/graph`, `internal/registry`, and `internal/serviceintel`
(Phase 7D) are all pure functions of observed data, not tied to any
specific service identity.

### Deterministic reasoning/safety boundary

Module 4 traced `internal/remediation/policy.go`'s `EvaluatePolicy` --
the actual HIGH/CRITICAL-risk safety gate -- and confirmed its
`rcaService`/`rcaConfidence` inputs come exclusively from `Incident.RCA`,
**never** from `AnalysisResult`. AI output cannot influence this gate; this
was verified by reading the call sites, not assumed.

### AI reasoning over structured evidence

`IncidentAnalysisContext`/`AnalysisResult` (`internal/aireasoning/model.go`)
already carry exactly this: bounded, sanitized incident/evidence/RCA-
candidate/graph-edge context in, structured facts-vs-inferences out
(`EvidenceReference{Claim, EvidenceIDs}`), enforced by a real `Validator`
that rejects ungrounded or ambiguity-violating results.

### Configuration/extensibility already provided

`ATLAS_API_KEYS`, `ATLAS_AI_PROVIDER`, `ATLAS_EXECUTION_PROVIDER`,
`ATLAS_SECURITY_ENABLED`, `ATLAS_EXECUTION_ENABLED`, registry retention
windows -- all already environment-driven, no hardcoded values anywhere
outside the one exception named below.

### Novel service names require no service-specific production code

Confirmed twice: Phase 7A's live experiment (renaming all four demo
services and introducing two entirely novel ones via real OTLP traffic,
observing discovery/graph/detection/correlation/RCA/remediation-planning
all work unmodified), and this module's own re-verification: a direct
search (`grep -rln "atlas-payment-service|atlas-order-service|atlas-gateway|
atlas-inventory-service" --include="*.go" internal/ cmd/`, excluding test
files and the frozen packages) returns **zero matches** in any non-test
production file. `test-m216-genericity.ps1` re-proves this live, as a
permanent, repeatable artifact, for two entirely new names
(`genericity-check-alpha`/`genericity-check-beta`).

## The one deliberate exception

**`internal/execution/guard.go`'s `AllowedServices` map** is hardcoded to
exactly the four demo services (`atlas-payment-service`,
`atlas-order-service`, `atlas-inventory-service`, `atlas-gateway`), mapped
to their exact container names. This is confirmed, by direct re-read this
module, to be the **only** hardcoded service-name reference anywhere in
production code, and it is a **deliberate safety boundary, not an
oversight**: it exists specifically so ATLAS cannot execute a remediation
action against a service nobody has explicitly declared safe to touch,
regardless of how confidently that service was discovered.
`test-m216-genericity.ps1` demonstrates this boundary positively -- a real,
approved remediation plan targeting a genuinely novel, correctly-discovered
service is real-verified to be rejected at execution with
`ErrServiceNotAllowlisted` (HTTP 403), and no container is restarted.

## Intentionally deferred (not implemented in Module 7)

- **Making `AllowedServices` configurable.** This would require modifying
  the FROZEN `internal/execution` package. Module 7 does **not** implement
  this -- it requires your separate, explicit authorization to reopen that
  package, which has not been given. The current hardcoding is documented
  here honestly as a real limitation for any deployment beyond the four
  demo services, not hidden.
- **An "ephemeral" (non-caching) mode for `aireasoning.Engine`/
  `remediation.Planner`** (identified in Module 5's deferred findings).
  `internal/remediation` is frozen; `internal/replay`'s existing
  throwaway-instance-per-request pattern already satisfies the actual need
  without touching it. Not implemented.
- **A richer, more precisely-scoped `IncidentAnalysisContext`** (the lossy
  single-candidate RCA reconstruction, the whole-live-graph AI context --
  both noted in Module 5's release review). Not implemented; no concrete
  requirement currently justifies the redesign this would need.

## Explicitly out of scope

- **A real Gemini/OpenAI/local-model provider implementation.** "AI
  Integration" in this module's original framing does not mean building
  one -- every prior module (3, 4, 5, 6) independently and repeatedly
  reaffirmed this as a standing non-goal, and this document does not
  override that.
- **Historical/persistent incident storage.** ATLAS has no durable incident
  history (Module 5's own finding, unchanged): everything is in-memory,
  bounded to roughly one hour past resolution, wiped on process restart.
  This document does not claim otherwise.
- **Arbitrary/configurable execution backends beyond Docker.**
  `ExecutorProvider` is structurally ready for one, but none beyond the
  real Docker adapter and `FakeExecutor` exists today.
- **A plugin registry, dependency-injection framework, or adapter
  factory.** The three existing provider interfaces (above) are already
  sufficient seams; no new abstraction layer is introduced by this
  document or by Module 7.
- **External IdP/OAuth for RBAC.** Static API keys only, today.

## Summary

Of the original "frameworkization" wishlist, everything except one item is
**already true today**, verified against real source rather than assumed:
telemetry, evidence, graph, registry, incident lifecycle, reasoning
boundaries, and provider abstractions are all already generic. The one real
gap -- the execution allowlist -- is a deliberate safety feature, not an
oversight, and remains a frozen-path-gated decision for you to make
separately, not something this module works around.
