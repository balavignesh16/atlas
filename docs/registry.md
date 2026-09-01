# Canonical Service Registry (Phase 7B/7C)

## What it is

The canonical service registry (`internal/registry`) is a small, persistent
record of **which services are known to this Atlas deployment**, even if
they haven't emitted telemetry recently. It answers a question the existing
telemetry graph cannot: "was this service ever part of this system?"

## How it differs from the telemetry dependency graph

Atlas already had a live dependency graph (`internal/graph.DependencyGraph`)
before this phase. The two are deliberately separate and answer different
questions:

| | Telemetry Graph (`internal/graph`) | Canonical Registry (`internal/registry`) |
|---|---|---|
| Question answered | "What is this system doing *right now*?" | "What services exist in this system?" |
| Storage | In-memory only | Persistent (SQLite) |
| Survives restart | No -- wiped on every `intelligence-engine` restart | Yes |
| Retention | Nodes/edges expire after `ATLAS_CORRELATION_RETENTION_SECONDS` (default 300s) of inactivity, then are gone entirely | Never deleted. Transitions through ACTIVE → STALE → RETIRED instead |
| Content | Call counts, error counts, latency -- live evidence | Identity, provenance, lifecycle status, timestamps |

Phase 7A proved the telemetry graph already discovers arbitrary services
generically (see `docs/architecture.md`'s Phase 7A findings, or the Phase 7A
report). This phase does **not** replace that mechanism -- it adds a
persistence layer beside it. A single telemetry event updates both,
independently: `internal/ingestion/otlp.go`'s `HandleTraces`/`HandleMetrics`
call both `correlationEngine.ProcessEvent` (feeds the graph) and
`registry.Observe` (feeds the registry), and neither one's failure or
absence affects the other.

## How services enter the registry

There is exactly one real producer today: **real OTel telemetry.** When a
trace or metric payload arrives with a `service.name` resource attribute,
`internal/ingestion` constructs a `registry.Evidence{Source:
ProvenanceObservedTelemetry, ...}` and calls `registry.Store.Record`
(`Observe` is a thin, unchanged-signature convenience wrapper around
`Record` for this exact case). There is no registration API, no manual-add
endpoint, and no seeding of known names -- the registry contains only what
real evidence has actually named, identical in spirit to how the graph
itself works (see Phase 7A).

## The evidence model (Phase 7C)

Phase 7B's `Observe` was a simple upsert with no way to represent a second
source. Phase 7C generalizes this into `registry.Evidence` -- one
observation, from exactly one source, at exactly one moment:

```go
type Evidence struct {
    ServiceName string
    Source      Provenance
    ObservedAt  time.Time
    Metadata    map[string]string // reserved, not yet persisted -- see below
}
```

`Store.Record(evidence)` is now the one mechanism ANY source uses -- today
only `internal/ingestion` calls it, but a future DOCKER/KUBERNETES/
DECLARED source needs only to construct an `Evidence` value and call
`Record`; no registry rewrite. `Metadata` exists in the struct now but is
deliberately **not persisted yet** -- no real source has anything to put
there, and adding a database column for a shape nothing populates would be
exactly the kind of speculative field this project's own discipline
forbids. Adding persistence for it is a small, additive schema change
whenever a real source actually needs it.

## Provenance semantics

Every record has a `provenance` field. Exactly one value is actually
produced by anything in this codebase:

- `OBSERVED_TELEMETRY` -- the service's `service.name` was seen in a real
  OTLP trace or metric payload.

Five more values are declared in `internal/registry/model.go` as a fixed
vocabulary for future phases, **none of which are implemented**:
`DECLARED`, `DOCKER`, `KUBERNETES`, `CONFIG`, `INFERRED`. Nothing in this
codebase produces those values today.

## Confidence (Phase 7C)

Each provenance source has a fixed, honest reliability class -- never a
fabricated numeric score (there is no real basis for claiming e.g. "87%
confidence" about whether a service exists). `registry.ConfidenceFor` is a
total, pure function over the six provenance values:

| Provenance | Confidence | Why |
|---|---|---|
| `OBSERVED_TELEMETRY` | `OBSERVED` | A real span/metric was actually seen |
| `DOCKER` / `KUBERNETES` (not implemented) | `OBSERVED` | Would query a live container/pod via the orchestrator's own API -- direct observation, just a different API than OTel |
| `DECLARED` / `CONFIG` (not implemented) | `DECLARED` | A human or config asserted it should exist -- a statement of intent, not proof it's running |
| `INFERRED` (not implemented) | `INFERRED` | A guess |

Exposed on the API as `confidence`, alongside the already-present
`provenance` field.

## Precedence rules (Phase 7C: now a real, tested resolution mechanism)

`internal/registry/precedence.go`'s ranking, most to least authoritative:

```
OBSERVED_TELEMETRY  (rank 0)
DOCKER / KUBERNETES (rank 1, tied)
DECLARED            (rank 2)
CONFIG              (rank 3)
INFERRED            (rank 4)
```

**This revises Phase 7B's original ordering**, which ranked `DECLARED`
above `DOCKER`/`KUBERNETES` ("declared intent outranks platform
inference"). On review that gets it backwards: `DECLARED` is a statement of
intent that can be stale or simply wrong (a compose entry for a service
nobody actually starts); `DOCKER`/`KUBERNETES`, once implemented, would
mean Atlas queried the orchestrator's own live API and found a real,
currently-running object -- direct observation of present infrastructure
beats a declaration about it.

`registry.ShouldSupersede(currentSource, currentAt, newSource, newAt)` is
the actual, tested decision function, called from `Store.Record` every
time evidence arrives for an already-known service. It governs only
**identity** fields (`Provenance`, `DisplayName`) -- **never**
`LastObservedAt`, which always advances to the latest evidence regardless
of source, because "is this service still around" doesn't depend on which
source is trusted for its identity. The rule, stated once so it never
needs an LLM to arbitrate an ordinary conflict:

1. Higher precedence (lower rank number) always wins, regardless of
   timestamp.
2. Equal precedence: the later `ObservedAt` wins.
3. A full tie (equal precedence AND equal timestamp, only realistic in
   tests): broken by comparing source names, so the result is a pure
   function of its inputs, never dependent on call order.

This makes the registry's final state for any service a **deterministic
function of the full evidence set, independent of the order it arrived
in** -- verified directly in `internal/registry/record_test.go`
(`TestRecord_ConflictingEvidence_DeterministicRegardlessOfInsertionOrder`
folds the same three conflicting `Evidence` values in four different
orders and asserts an identical result every time).

A related, subtle correctness point: the tie-break in rule 2 compares
against `authorityObservedAt` (an internal, unexported field tracking "when
was the evidence that's CURRENTLY authoritative last confirmed"), never
against `LastObservedAt` (which any weaker evidence can also advance). Using
`LastObservedAt` for this would let an unrelated, non-authoritative
sighting corrupt a later equal-precedence tie-break -- caught during Phase
7C's own implementation before it shipped, not by a failing test in
production.

## Staleness / retirement policy

A service's `status` is independent of the telemetry graph's retention and
moves through three states:

```
ACTIVE ──(no telemetry for ATLAS_REGISTRY_STALE_AFTER_SECONDS)──► STALE
STALE  ──(no telemetry for ATLAS_REGISTRY_RETIRE_AFTER_SECONDS)──► RETIRED
(any status) ──(new telemetry observed)──► ACTIVE
```

Defaults: `ATLAS_REGISTRY_STALE_AFTER_SECONDS=1800` (30 minutes),
`ATLAS_REGISTRY_RETIRE_AFTER_SECONDS=86400` (24 hours) -- both far longer
than the telemetry graph's 300-second retention, deliberately: a service
missing from the live graph for 5 minutes is completely normal and must
not make it disappear from "what services exist." `intelligence-engine`
refuses to start if `RETIRE_AFTER <= STALE_AFTER`, since that would let a
service skip STALE entirely.

Transitions are evaluated every 5 seconds by the same background loop that
runs the telemetry graph's own cleanup (`cmd/intelligence-engine/main.go`).
**RETIRED records are never deleted** -- a retired service's identity,
`firstObservedAt`, and full timestamp history remain queryable via
`GET /api/v1/services/{name}` indefinitely. A RETIRED (or STALE) service
that resumes emitting telemetry becomes ACTIVE again automatically; there
is no separate "reactivate" call.

**"Registry" vs. "active" -- stated precisely.** ACTIVE, STALE, and RETIRED
are all answers of "yes, Atlas knows this service exists" -- they differ
only in recency, never in whether the record is real or complete. The
registry is not a diminished version of the graph: a RETIRED entry is
exactly as real as an ACTIVE one. Contrast this with the telemetry graph,
which has no lifecycle of its own at all -- a node is either present
(recently observed) or simply absent (expired, not "retired").

**Known Phase 7C scope boundary:** `EvaluateLifecycle` keys strictly on
`LastTelemetryAt`, not the broader `LastObservedAt`. A hypothetical service
known only through a future non-telemetry source (e.g. `DECLARED`, not yet
implemented) would have `LastTelemetryAt = NULL` and would therefore never
transition out of ACTIVE under today's sweep. Designing lifecycle semantics
for a source that doesn't exist yet would be speculative; this is a
documented limitation to revisit once a real non-telemetry source exists,
not something silently patched over now.

## Persistence behavior

Backed by SQLite via `modernc.org/sqlite` (a pure-Go driver -- required
because this repo builds with `CGO_ENABLED=0`), one table, no ORM, no
migration framework. A single database connection is used deliberately
(`db.SetMaxOpenConns(1)`): SQLite serializes writers regardless, this
process is the only writer, and one connection keeps behavior
deterministic. In Docker, the database file lives on the named volume
`atlas-registry-data` (see `docker-compose.yml`), so it survives both a
plain container restart and a full `docker-compose up --force-recreate`.
Locally (outside Docker), it defaults to `./atlas-registry.db` unless
`ATLAS_REGISTRY_DB_PATH` is set.

## What is intentionally NOT implemented yet

Per Phase 7A's conclusion and Phase 7B's explicit scope:

- **No Docker discovery connector.** Phase 7A proved the telemetry pipeline
  already discovers arbitrary services without one.
- **No Kubernetes discovery.** There is no Kubernetes deployment target in
  this project yet.
- **No config/code metadata discovery** (scanning `DATABASE_URL`-shaped env
  vars, etc.) -- the weakest, most drift-prone evidence source; not worth
  the false-positive risk when OTel evidence already works.
- **No AI/LLM-based discovery.** There is no working AI/LLM integration in
  this project at all (`internal/aireasoning/provider/gemini_provider.go`
  and `internal/remediation/provider/ai.go` are both unimplemented
  placeholders) -- and this phase's problem (deterministic service
  registration) doesn't need one regardless.
- **No mutation API.** `GET /api/v1/services` and
  `GET /api/v1/services/{name}` are the only registry endpoints. A write
  endpoint would let a caller fabricate a service Atlas never actually
  observed.

## Why OTel remains the primary observed-evidence source

Phase 7A's live experiment (renaming all 4 sample services and introducing
two entirely novel ones, `discovery-test-alpha`/`discovery-test-beta`,
purely via real OTLP wire traffic) proved the existing pipeline requires
zero backend or frontend code changes to discover arbitrary services,
their real dependencies, and to drive incident detection/correlation/RCA/
remediation planning correctly. No other evidence source in this codebase
offers that combination of genuineness (an actual protocol, not a guess)
and zero required code changes. Future sources (declared config, Docker/
Kubernetes metadata) would supplement identity/ownership information OTel
doesn't carry, not replace it as the primary source of "is this real."

## Query/filter API (Phase 7C)

`GET /api/v1/services` accepts three optional, independent query
parameters, all resolved server-side and deterministically ordered
(alphabetical by name, regardless of which filters are set):

- `status` -- `ACTIVE` / `STALE` / `RETIRED` (case-insensitive)
- `source` -- any real `Provenance` value (case-insensitive)
- `q` -- case-insensitive substring match on the service name

An unrecognized `status` or `source` value is a real `400 Bad Request`
(`INVALID_STATUS` / `INVALID_SOURCE`), not a silently-ignored filter. This
is deliberately small: no pagination, no full-text search, no sorting
options. At the scale a single Atlas deployment's registry actually
reaches (dozens to low hundreds of services), a plain filtered list is
sufficient; a distributed/paginated query layer would be solving a problem
this project doesn't have yet.

## Architectural boundary: Registry, Graph, and Intelligence

Five layers, each answering a different question, each staying inside its
own boundary:

```
Registry            "What services exist / have existed?"
  (persistent, evidence + provenance, internal/registry)
        │
        ▼
Graph                "What is currently talking to what, right now?"
  (ephemeral, trace-derived topology, internal/graph)
        │
        ▼
Incident Detection   "Is something behaving abnormally, right now?"
  (internal/incidentdetector -- FROZEN, untouched by this phase)
        │
        ▼
RCA                  "Given the observed evidence, what's the likely cause?"
  (internal/rca -- FROZEN, untouched by this phase)
        │
        ▼
Remediation          "What plan addresses this, and did executing it work?"
  (internal/remediation, internal/execution -- FROZEN, untouched)
        │
        ▼
Service Intelligence "What does Atlas know about this ONE service, right
  (internal/serviceintel,   now, from every source at once?"
   Phase 7D)          composes Registry + Graph + incident history
                     (RCA output already materialized on Incident) at
                     request time -- imports none of the frozen layers above
        │
        ▼
Future AI            reasons over the EvidencePackage/ServiceIntelligence
                     contracts -- it does not own discovery, truth, or
                     execution
```

The registry and graph sit *beneath* incident detection/RCA/remediation,
supplying identity and topology; they do not themselves detect, diagnose,
or remediate anything, and nothing in this phase changed any frozen layer's
behavior. Service Intelligence sits *above* all of them as a read-only,
request-time composition layer -- it queries the registry, the graph, and
already-computed incident/RCA state, but writes to none of them and detects
nothing itself.

## Service Intelligence (Phase 7D)

### What it is

`internal/serviceintel` answers a question none of the existing per-source
APIs answer alone: **"what does Atlas know about this one service, right
now, from every source at once?"** It is a small `Assembler` that, on every
request, reads fresh from three already-existing, independent sources --
the registry, the live dependency graph, and incident history -- and
composes them into one `ServiceIntelligence` object. It stores nothing,
caches nothing, and mutates none of its three sources; every field in the
response is a direct read or a deterministic sort of a real source, never a
computed metric or a generated summary.

### Why RCA needs no new import

RCA's output is not something `serviceintel` has to go re-compute or query
live. By the time an `Incident` exists, `internal/rca.Engine` (a FROZEN
package) has already written its findings onto the incident's own fields --
`RCA`, `Confidence`, `Score`, `DetectionReason`. Even
`GET /api/v1/incidents/{id}/rca` (`internal/httpapi/incident.go`) works this
same way: it repackages those already-materialized fields, it does not call
into `internal/rca` at request time. So `serviceintel` reads incident
history via `internal/incidentmanager` exactly like that existing endpoint
does, and imports `internal/rca` (or any of the other five frozen packages
-- `internal/execution`, `internal/incidentdetector`,
`internal/remediation`, `internal/blast`, `internal/propagation`) not at
all. Verified directly: `go list -deps ./internal/serviceintel/...` contains
none of the six frozen import paths.

### The three sources, and what "known" means for each

| Source | "Known" means | What Service Intelligence reads from it |
|---|---|---|
| Registry (`internal/registry`) | The service has real evidence in the canonical registry (any status: ACTIVE/STALE/RETIRED) | Status, provenance, confidence, first/last observed timestamps |
| Graph (`internal/graph`) | The service currently has at least one live incoming or outgoing edge | Aggregated call count, error count, average duration, first/last observed, per dependency |
| Incident history (`internal/incidentmanager`) | The service is the `RootService` of an incident, or appears in an incident's `AffectedServices` | A bounded, most-recent-first list of incident summaries |

A service can be known to any subset of these three -- and often is. A
brand-new service that has only just started emitting traces is
graph-known but not yet registry-known (the registry sweep and the graph
update from the same telemetry event happen independently; see "How
services enter the registry" above). A service that was RETIRED from the
registry weeks ago but is named in an old incident's `AffectedServices` is
incident-known but neither registry- nor graph-known. Service Intelligence
represents this honestly rather than flattening it: `registry.known` is its
own independent boolean, never inferred from whether dependencies or
incidents were found.

### 404 semantics

`GET /api/v1/services/{name}/intelligence` returns `404 SERVICE_NOT_FOUND`
**only when the name is unknown to all three sources at once.** Being known
to even one source is a real, meaningful, structurally valid result -- it
is always a `200`, never a partial error. This is the same principle Phase
7A already established for the registry generally: partial evidence is
still evidence, not a degraded error state.

### Response contract

```json
{
  "serviceName": "checkout-service",
  "registry": {
    "known": true,
    "status": "ACTIVE",
    "provenance": "OBSERVED_TELEMETRY",
    "confidence": "OBSERVED",
    "firstObservedAt": "2026-01-01T00:00:00Z",
    "lastObservedAt": "2026-01-01T00:05:00Z"
  },
  "dependencies": {
    "incoming": [
      { "service": "gateway", "callCount": 42, "errorCount": 1, "averageDurationMs": 25, "firstObserved": "...", "lastObserved": "..." }
    ],
    "outgoing": []
  },
  "relevantIncidents": [
    { "incidentId": "...", "status": "OPEN", "severity": "CRITICAL", "title": "...", "startedAt": "...", "rootService": "checkout-service", "confidence": "HIGH" }
  ],
  "generatedAt": "2026-01-01T00:06:00Z"
}
```

Contract rules, all enforced by tests (`internal/serviceintel/assembler_test.go`,
`internal/httpapi/intelligence_test.go`):

- When `registry.known` is `false`, `status`/`provenance`/`confidence`/
  `firstObservedAt`/`lastObservedAt` are **omitted from the JSON entirely**
  (not present as zero values or empty strings) -- "the registry never
  heard of this service" must never be visually indistinguishable from "the
  registry knows it and every field happens to be zero."
- `dependencies.incoming` and `dependencies.outgoing` are always real JSON
  arrays, `[]` when empty, never `null`.
- `relevantIncidents` is bounded to the 10 most recent (by `startedAt`,
  ties broken by `incidentId` for a fully deterministic order regardless of
  `internal/incidentmanager`'s own unspecified iteration order), never an
  unbounded per-service history -- a long-running deployment's incident
  count for a hot service could otherwise grow without limit. This bound is
  a local, unexported constant in `internal/serviceintel`, not imported
  from any other package's convention, frozen or otherwise.
- `relevantIncidents[].confidence` is `Incident.Confidence` verbatim and is
  legitimately omitted when RCA has not run for that incident yet -- never
  a fabricated placeholder.
- Every numeric/timestamp dependency field (`callCount`, `errorCount`,
  `averageDurationMs`, `firstObserved`, `lastObserved`) is copied verbatim
  from `correlationmodel.DependencyEdge`, never recomputed or estimated.

### Determinism

`Assembler.Build(name, now)` is a pure function of its three sources' state
and the supplied `now` -- it never reads the wall clock itself. Both
dependency lists are explicitly sorted by service name (compensating for
`internal/graph.DependencyGraph.GetServiceDependencies`'s own unspecified,
map-iteration-based order -- sorting happens in `serviceintel`, not by
modifying the unrelated `internal/graph` package), and the incident list is
explicitly sorted most-recent-first with an ID tie-break. Calling `Build`
twice with identical inputs and an identical `now` produces byte-identical
JSON, which `TestBuild_Deterministic_IdenticalInputsProduceEqualResults`
asserts directly.

### No mutation, no persistence, no caching

There are no write endpoints. `Assembler.Build` never calls `Record`,
`AddDependency`, `UpdateIncident`, or any other mutating method on any of
its three sources -- `TestBuild_DoesNotMutateSources` asserts this directly
by comparing each source's state before and after a `Build` call. The
composed `ServiceIntelligence` result itself is never written back to disk
or held in memory between requests; every request re-reads all three
sources fresh. This keeps the endpoint's behavior exactly as fresh as its
three underlying sources, with no separate cache-invalidation problem to
get wrong.

### Why this involves no AI, and why it's AI-ready anyway

Nothing in `internal/serviceintel` calls an LLM, generates text, infers
ownership, or produces a recommendation -- every field is a direct
transcription or a deterministic sort of real evidence, matching this
project's existing FACT/INFERENCE/UNKNOWN discipline (see "Mapping future
AI output to FACT / INFERENCE / UNKNOWN" above): everything
`ServiceIntelligence` carries is FACT or derived FACT, never INFERENCE.
It is "AI-ready" in the same sense Phase 7C's `EvidencePackage` is: a
future reasoning layer that needed a single, well-defined, per-service
evidence bundle instead of querying the registry/graph/incident manager
separately would have exactly that available at
`GET /api/v1/services/{name}/intelligence` -- but building that consumer is
explicitly out of scope here, same as it was for `EvidencePackage`, since
there is still no working AI/LLM integration anywhere in this codebase to
justify wiring one up.

### Explicit non-goals (Phase 7D)

- No AI/LLM/provider/SDK of any kind.
- No Docker/Kubernetes/config discovery -- unchanged from Phase 7A/7B/7C's
  conclusions.
- No ownership inference, generated summaries, or speculative text of any
  kind on the frontend or backend.
- No persistence or caching of the composed `ServiceIntelligence` result.
- No mutation endpoints.
- No pagination or full-text search beyond what Phase 7C's registry query
  API already provides.
- No changes to the registry schema, `EvidencePackage`, the execution
  allowlist, or any frozen package.

`internal/registry/evidence_package.go` defines `EvidencePackage`: a
structured, deterministic bundle a future AI reasoning layer could consume,
so it reasons over one well-defined contract instead of querying registry/
graph/incident internals directly. **Nothing calls an LLM. Nothing calls
this from a live HTTP endpoint yet** -- there is no AI consumer in this
codebase (`internal/aireasoning/provider/gemini_provider.go` and
`internal/remediation/provider/ai.go` are both unimplemented placeholders;
`FakeProvider`/`FakePlanner` are what actually run). This is purely an
architectural preparation.

```go
type EvidencePackage struct {
    ServiceName     string
    LifecycleStatus Status
    Provenance      Provenance
    Confidence      Confidence
    FirstObservedAt time.Time
    LastObservedAt  time.Time
    Dependencies    []string  // real, caller-supplied -- e.g. from internal/graph
    IncidentFacts   []string  // real, caller-supplied -- e.g. from internal/incidentmanager
    GeneratedAt     time.Time
}
```

`internal/registry` deliberately does not import `internal/graph` or
`internal/incidentmanager` -- `Dependencies`/`IncidentFacts` are supplied by
the caller, keeping this contract decoupled from any one evidence producer.
`Store.BuildEvidencePackage(name, dependencies, incidentFacts, now)` is
real, tested, and returns `ok=false` for an unknown service rather than a
fabricated empty package. A fuller package that automatically pulls live
graph/incident data would require a cross-package assembler wired into
`main.go`; that's a larger integration exercise deliberately **not** done
in this phase (see Known Limitations in the Phase 7C report) rather than
built speculatively without an actual AI consumer to justify it yet.

## Mapping future AI output to FACT / INFERENCE / UNKNOWN

`internal/aireasoning`'s `AnalysisResult` already has this shape --
`observedFacts`, `inferences`, `alternativeExplanations` -- and Phase 7C
does not replace or reinterpret it. The principle for how a future real AI
layer should use `EvidencePackage`:

- **FACT**: every field in `EvidencePackage` itself. `ServiceName`,
  `LifecycleStatus`, `Provenance`, timestamps, and any `Dependencies`/
  `IncidentFacts` the caller supplied are either directly observed
  telemetry or deterministically derived from it (via `ShouldSupersede`,
  `EvaluateLifecycle`, etc.) -- never an AI opinion.
- **Derived FACT**: a deterministic computation over real evidence (e.g.
  "error rate exceeded 20%") is still FACT, even though it required logic
  to compute -- the existing RCA/blast-radius engines already work this
  way and this phase doesn't change that.
- **INFERENCE**: only a future AI layer's own output -- a hypothesis about
  *why* something happened, framed as a probability or explanation, never
  presented as certain.
- **UNKNOWN**: information the evidence genuinely doesn't establish must
  stay explicitly unknown. An AI layer must never fill an `UNKNOWN` gap
  with a plausible-sounding assumption and let it silently read as FACT
  downstream -- the existing `AnalysisResult.limitations` field is exactly
  this kind of honest disclosure, and any future AI output must preserve
  that same discipline.

## Framework-readiness review (Phase 7C)

The test this project holds itself to: *"If I replace the current 4 sample
services with 20 completely differently-named services, does Atlas require
service-specific code?"*

- **Discovery, graphing, registry, incident detection, RCA input,
  remediation planning: NO.** Verified concretely in Phase 7A (two entirely
  novel service names, introduced via real OTLP wire traffic, correctly
  drove the graph, incident detection, correlation, RCA, and remediation
  planning with zero code changes) and reconfirmed structurally in Phase
  7C: `Store.Record`/`ShouldSupersede`/`ConfidenceFor`/`EvaluateLifecycle`
  are all pure functions of `Provenance`/timestamps, with no service-name
  branching anywhere.
- **Execution: YES, one deliberate exception.**
  `internal/execution/guard.go`'s `AllowedServices` map hardcodes the 4
  original demo service names to Docker container names. This is a
  **safety boundary, not a discovery gap** -- it exists specifically so
  Atlas cannot execute a remediation action against a service it has never
  been explicitly told is safe to touch, regardless of how confidently that
  service was discovered. Extending execution to arbitrary discovered
  services is a frozen-path decision for a future phase, made deliberately
  and explicitly, never worked around.

## Known limitation: execution's service allowlist

See "Framework-readiness review" above -- `internal/execution/guard.go`'s
`AllowedServices` map is the one deliberate, safety-motivated exception to
"Atlas requires no service-specific code," called out explicitly rather
than disguised as a discovery gap. It remains untouched (frozen path) by
both Phase 7B and Phase 7C.
