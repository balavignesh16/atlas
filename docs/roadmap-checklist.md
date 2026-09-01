# ATLAS Roadmap & Status Checklist

Source of truth for "what's actually done" vs "what's planned." Generated from a direct audit of the repository (git history, code, `go build`/`go test`, and multiple live `docker compose` E2E runs) on 2026-08-24 — not from prior verification reports alone, several of which overstated coverage or described scenarios that didn't reproduce (see §2). Updated same day after implementing and live-verifying M2.7.1 (see `docs/m271_verification_report.md` for full command output).

**Updated again on 2026-09-01**, re-audited from `git log`, current source, and the repository's own `docs/m2*_verification_report.md` reports rather than copied from prior conversational summaries. Brings this document current through M2.16 and the dashboard/registry/replay/benchmarking work committed since the previous update. Everything through M2.16 below is now **committed to `main`** (not yet pushed to `origin/main` as of this update) — the "still uncommitted" language throughout the 2026-08-24 version of §2 no longer applies.

Numbering follows the milestone scheme actually used in the repo (`M0`, `M1.x`, `M2.x`) rather than the alternate 18-module ChatGPT plan, per the reconciliation in §7. Some work committed after 2026-08-24 was tracked with session-local labels ("Phase 7", "Module 3" through "Module 7") in commit messages and doc comments rather than repository `M2.x` numbers — see the note at the top of §4 before treating any such label as an official roadmap milestone.

---

## 1. Completed & Frozen (committed to `main`)

### M0 — Engineering Foundation
- [x] Repository structure, Docker Compose environment, dev workflow

### M1 — ATLAS Lab Application
- [x] Gateway, Order, Inventory, Payment services (Java/Spring Boot)
- [x] Inter-service communication, order workflow, compensation logic

### M2.1 — OpenTelemetry Foundation
- [x] OTel/Micrometer tracing, OTLP exporters, Collector, trace propagation
- [x] Verified: Collector failure does not break business traffic

### M2.2 — Telemetry Ingestion & Normalization
- [x] OTLP protobuf decoding, `ATLASEvent` normalization
- [x] Sensitive-data sanitization, bounded event buffer (DROP-OLDEST)

### M2.3 — Correlation & Dependency Graph
- [x] Trace reconstruction, tree/timeline APIs
- [x] Dynamic dependency graph with edge stats
- [x] Self-edge bug fixed (`TestIgnoreSelfDependency`)

### M2.4 — Incident Detection & Deterministic RCA
- [x] Sliding-window detection, fingerprinting/dedup, incident lifecycle
- [x] Evidence-based RCA, blast radius, ambiguous-RCA handling
- [x] ~~Regression~~ **Fixed & verified live**: the detector's `EventType != "SPAN"` check never matched real events (`"TRACE_SPAN"`), so detection was inert in the live pipeline since this milestone's original commit. Fixed, covered by a new regression test (`internal/incidentdetector/detector_test.go`), and confirmed working in a real `docker compose` run on 2026-08-24 — real incidents were created from real traffic for the first time.
- [x] RCA confidence threshold reverted `< 30` → `< 40` (the original, frozen value) with a new regression test (`internal/rca/engine_test.go`) locking in the boundary. Confirmed correct live: two incidents scored exactly 30 during the E2E run and correctly stayed LOW confidence under `< 40` (they would have wrongly been promoted to MEDIUM under the uncommitted `< 30`).
- [x] **Fixed & verified live (M2.7.1)**: the detector's status-code fallback checked `http.status_code`/`http.response.status_code` attributes this project's real Micrometer/Spring telemetry never populates (the actual key is `status`). Before this fix, `atlas-payment-service` — a pure sink with no outbound calls of its own — could never register an error at all (its own span's `Status` field also never becomes `ERROR`/`5xx` under this instrumentation). Fixed with regression tests (`TestProcessEvent_MicrometerStatusAttributeMarksError` etc.), confirmed live: payment-service now correctly generates its own incidents from real traffic.
- [x] **Cross-service incident correlation added (M2.7.1)**: `internal/incidentmanager/correlator.go` groups incidents connected through the M2.3 dependency graph within a time window, and selects a causally-correct primary via a caller/callee sink rule (a service can only be primary if no other group member is something it calls). Metadata-only (`correlationGroupId`, `primaryIncidentId`, `relatedIncidentIds`) — `Incident.Status` and `rca.Engine` itself are both untouched. Verified live: a real 3-service cascade (payment→order→gateway) correctly grouped into one `correlationGroupId` with **payment-service correctly selected as primary**, not one of its callers.
- [x] **RCA scoring gap partially addressed, remainder still open (M2.7.2, see below)**: `Incident.TraceIDs` is now genuinely populated as RCA evidence, but the specific fix investigated for the temporal-precedence bonus was implemented, verified, and then **reverted** after live testing surfaced a deeper defect — see M2.7.2 below. The gap this entry originally described is **not closed**, only better understood; do not treat it as resolved.

### M2.5 — AI-Assisted Incident Reasoning
- [x] Provider interface (Fake/Gemini), bounded context builder
- [x] Prompt-injection defense, facts/inferences separation, evidence grounding
- [x] `ATLAS_AI_ENABLED=false` fallback path

### M2.6 — Remediation Planning & Safety Engine
- [x] Planner with action allowlist, safety validator, approval lifecycle
- [x] Confirmed: no `os/exec`, no shell, no Docker/K8s CLI calls
- [x] `APPROVED != EXECUTED` boundary enforced

---

## 2. M2.7 Controlled Execution Engine + M2.7.1–M2.7.4 Correlation, RCA & Verification Hardening

**Status: COMPLETE, committed to `main`.** All of M2.7 and its sub-milestones (M2.7.1–M2.7.4) are committed; `go build`/`go vet`/`go test`/`go test -race` all pass, and each sub-milestone was independently confirmed against live `docker compose` E2E runs (see `docs/m27_verification_report.md` through `docs/m274_verification_report.md`).

- [x] Execution guard (enabled flag, approval status, fingerprint match, action/service allowlist, evidence check)
- [x] Idempotency by `(planId, actionId)` — verified against a real Docker restart, not just the fake executor
- [x] Typed Docker SDK adapter (`RestartService`, `Observe`, `Investigate`) — no shell/CLI execution
- [x] Async post-execution verification against M2.4 incident state
- [x] Execution HTTP API (`execute`, `GET /executions/{id}`, `GET /incidents/{id}/executions`)
- [x] Unit tests for guard/engine (`go test ./internal/execution/...` passes)
- [x] Docker build verified — `docker compose up -d --build` succeeds, all containers reach healthy
- [x] `internal/infrastructure/docker` unit tests added (`adapter_test.go`, via a `ContainerRestarter` interface seam, no live daemon needed)
- [x] **Real Docker restart proven against live infrastructure**: `TestDockerAdapter_RealRestartAgainstLiveContainer` (opt-in, `ATLAS_DOCKER_INTEGRATION_TEST=true`) runs the real, unmodified `execution.Engine` + real Docker adapter against a live container. `docker inspect` independently confirmed the container's `StartedAt` matched the test's execution timestamp — a genuine restart, not a claim.
- [x] Cross-service correlation added and verified live (M2.7.1, see §1)

### M2.7.2 — RCA Evidence Quality & Confidence Calibration
- [x] `internal/rca/engine.go` — zero modifications (confirmed via `git diff --stat`, including after a mid-implementation investigation that was fully reverted).
- [x] Shared `event.IsErrorStatus` classification helper added, replacing duplicated logic in `incidentdetector`/`correlationmodel`.
- [x] `Incident.TraceIDs` now genuinely populated from real window observations (bounded, deduplicated, thread-safe).
- [ ] ⚠️ **Temporal-precedence bonus activation investigated, implemented, verified working in isolation, then reverted.** Root cause: `propagation.CheckTemporalPrecedence` compared `span.StartTime`, but in any synchronous nested call chain a parent's span always starts before its child's by causality — so the true root cause's span always starts *last*, never first. The mechanism as originally written could never reward a true root cause even with perfect data. The fix (comparing `EndTime` instead) was verified working, then reverted because the scoring-weight interaction it exposed lives inside frozen `rca.Engine.Analyze` and was out of this milestone's bounds. **`propagation.CheckTemporalPrecedence` remains exactly as it was: `StartTime`-based, dormant for nested cascades** — this is a deliberately-preserved, documented limitation, not an oversight. See `docs/m272_verification_report.md`.

### M2.7.3 — Causal RCA Re-attribution Layer
- [x] `internal/incidentmanager/causal.go` added: re-attributes `DEPENDENCY_ERROR` evidence from a caller to the callee it actually names, before RCA scores it.
- [x] `internal/rca/engine.go`, `internal/propagation/analyzer.go`, `internal/incidentmanager/correlator.go`, `internal/remediation/`, `internal/execution/` — all confirmed zero modifications.
- [x] Live Docker E2E, run three times: causal attribution reached a clean, non-ambiguous `MEDIUM (score=45)` result for the true root cause and completed a real plan → approve → execute cycle against live infrastructure, twice; once correctly landed on `AMBIGUOUS` when the evidence picture was less clean. No safety gate was weakened to produce either outcome.

### M2.7.4 — Execution Verification Correctness
- [x] Fixed a genuine defect found during its own first-round Docker E2E: `VerificationStatus.FAILED` could be produced merely because `Incident.LastUpdatedAt` had advanced past `executionFinishedAt` (a value re-stamped on every M2.4 evaluation tick, independent of data freshness) — a stale, already-resolved-by-restart incident could be misreported as a confirmed remediation failure.
- [x] `FAILED` now depends only on genuinely new, independently-timestamped `ERROR` events observed in `buffer.EventBuffer` after execution finished.
- [x] Both directions proven live: the false-FAILED scenario now correctly settles to `VERIFICATION_TIMEOUT`, and a genuine post-execution failure is still correctly detected as `FAILED`. Independent `docker inspect ... StartedAt` cross-check confirms real restarts occurred.

### Known defects/limitations still open (carried forward, not fixed)
1. **RCA temporal-precedence scoring remains dormant for nested cascades** (M2.7.2, above) — collection ships, scoring activation does not. Fixing it requires touching frozen `rca.Engine.Analyze`'s scoring-weight logic.
2. Verbose `slog.Info` debug logging left in the hot path of `incidentdetector.ProcessEvent`/`EvaluateAll` — should drop to `Debug` or be removed. Not fixed — out of every subsequent milestone's approved scope so far.
3. **Full cascade plan → execute end-to-end via the HTTP planner/confidence gate specifically** was never proven in the original M2.7.1-era scripts (`test-m27-docker.ps1`'s Scenario A hits AMBIGUOUS via the dormant temporal-precedence gap above; Scenario B hits a separate LOW-confidence gate). M2.7.3's causal-attribution layer later did reach a clean `MEDIUM(45)` execute cycle live (see above) — but only via causal re-attribution, not via the temporal-precedence path this item originally called out. Treat as **substantially, not fully, resolved**.

---

## 3. M2.8–M2.16 — Chaos, Security/RBAC, CI Hardening, Data-Integrity Fixes, Final E2E, Framework Verification

All items below are **committed to `main`**, each with its own `docs/m2*_verification_report.md` and live Docker E2E verification. All frozen paths (`internal/execution/`, `internal/rca/`, `internal/incidentdetector/`, `internal/remediation/`, `internal/blast/`, `internal/propagation/`) were confirmed untouched at every one of these milestones except where a milestone's own stated purpose was to fix a genuine defect inside one of them (M2.13, M2.14 — both documented below); no milestone weakened an existing safety gate.

### M2.8 — Chaos Engineering / Fault-Injection E2E
- [x] `test-m28-chaos.ps1` added: single-service, independent-concurrent, and persistent-fault scenarios against live infrastructure.
- [x] M2.7's full protected-path set and its own regression suite (`test-m27-docker.ps1`) confirmed zero diff.

### M2.9 — Security & Authorization Boundary (Authentication + RBAC on write endpoints)
- [x] Static API-key authentication (`ATLAS_API_KEYS`, header `X-Atlas-Api-Key`) resolving to a named `Principal{Name, Role}`, checked via `internal/security` middleware.
- [x] Constant-time key comparison (`crypto/subtle.ConstantTimeCompare`); raw keys never logged (regression-tested).
- [x] Disabled by default (`ATLAS_SECURITY_ENABLED=false`), matching the existing `ATLAS_EXECUTION_ENABLED` convention — pre-existing scripts unaffected unless explicitly enabled.
- [x] Authenticated identity routed into the approval/execution audit trail (replacing the previous unverified client-supplied `approver` string).
- [x] `test-m29-security.ps1` added.

### M2.10 — CI Race Gate, Version-Pin Reconciliation & Ingestion/Normalization Coverage
- [x] `go test -race -v ./...` added as an **additional** CI step (does not replace the existing non-race step).
- [x] Go/Java CI version pins reconciled to the repository's own declared minimums (Go 1.26→1.25 matching `go.mod`'s `go 1.25.0`; Java 25→24 matching `control-plane/pom.xml`'s `<java.version>24</java.version>`) — resolves the version-pin discrepancy previously flagged in §5 of this document.
- [x] Unit test coverage added for `internal/ingestion` (9 tests) and `internal/normalization` (21 tests across `normalizer_test.go`/`sanitizer_test.go`) — both previously zero-coverage packages sitting on the untrusted-input boundary.
- [ ] **Important, preserved distinction**: this milestone added the race-detector step to CI *configuration*. As of this document, that CI job has still never actually been *executed* by GitHub Actions, because nothing in this repository has been pushed to `origin/main`. "The race suite passes locally" — true, verified repeatedly inside a Linux container throughout this project (this dev machine's native Windows cgo/gcc toolchain is broken, a pre-existing, unrelated environment limitation) — is a materially different claim from "the CI race gate has been exercised by CI," and only the former is true today.

### M2.11 — RBAC on Read-Only Routes
- [x] Wires the `PermissionView`/`PermissionReadAudit` permissions M2.9 already defined onto the read-only HTTP surface (previously fully unauthenticated even with `ATLAS_SECURITY_ENABLED=true`): telemetry, correlation, graph, incidents, RCA, evidence, plan-viewing → `PermissionView`; execution/audit history → `PermissionReadAudit`.
- [x] Direct unit-test coverage added for the four previously-untested read-only `httpapi` handlers plus `rca.Engine.Analyze()`.
- [x] **Deliberate, documented boundary case at the time**: `POST /api/v1/incidents/{id}/analyze` was left explicitly unauthenticated, since gating it would have required touching the same dispatcher entry point serving unrelated GET routes, outside M2.11's "read-only routes" scope. **This gap was subsequently closed** — see the "route POST /analyze..." commit in §4 below, which routes it to a dedicated authenticated handler.

### M2.12 — Docker E2E in CI
- [x] `docker-e2e` CI job added, running the existing, unmodified `test-m27-docker.ps1` on `ubuntu-latest` (with a `docker-compose`→`docker compose` shim and required `ATLAS_EXECUTION_ENABLED`/`ATLAS_EXECUTION_PROVIDER` env vars, since a clean runner otherwise fails Scenario A's execute call against the fail-closed default).
- [x] Verified locally (Windows, PowerShell 5.1) across multiple clean runs. Verification against actual GitHub Actions infrastructure has not occurred, for the same reason as M2.10's race gate above (nothing pushed to `origin/main`).

### M2.13 — Execution Verification Evidence Fix
- [x] Fixed a genuine defect in `internal/execution/verification.go`: `Verifier.Verify()` accepted `VERIFIED` on every code path the moment the watched incident's `Status` became `RESOLVED`, without ever consulting `EventBuffer` for genuine post-execution failure evidence — live E2E showed this return `VERIFIED` in a run where fresh, real post-execution failure traffic made `FAILED` the correct outcome.
- [x] Root cause: incident resolution and verification watch different, disconnected signals (a resolution timeout vs. an evidence scan against one specific `incidentID`) — a fresh failure after resolution creates a *new* incident the verifier never learns about.
- [x] Fix confined to one new unexported method (`verdictForResolved`) reusing the existing, unmodified evidence-scanning function; net diff +39/−10 in one file. All 14 pre-existing tests unchanged; 4 new regression tests added.

### M2.14 — Blast-Radius Data Integrity & Propagation Coverage
- [x] Fixed `internal/blast`'s `Calculate()`: computed a local `failureCount` while iterating traces but never wrote it to `Incident.FailureCount`, and never populated `TraceCount` at all — confirmed live via the incident API returning `traceCount:0`/`failureCount:0` despite real failing traces. No consumer of either field exists anywhere in the repository, so this is a pure data-integrity fix with zero decision-logic impact on RCA, remediation, or execution.
- [x] 10 new tests, including mechanical proof against the temporarily-restored pre-fix implementation (5 failed with the exact observed defect before the fix, all pass after).
- [x] `internal/propagation` given its first dedicated package-level test file: 23 tests covering `IsPath`, `CheckTemporalPrecedence`, and `CheckPropagation` against the real, unmodified implementation (production code in `analyzer.go` unmodified).

### M2.15 — Full-Chain End-to-End Validation
- [x] `test-m215-full-chain.ps1` added — closes the gap this document's §5 (2026-08-24 version) named explicitly as "not started": one continuous script walking chaos → telemetry → detection → correlation → RCA → AI → plan → approval → execution → verification in a single run, reporting an honest per-stage PASS/BLOCKED/FAILED/NOT_REACHED outcome rather than one pass/fail. Ten prior scripts each proved one milestone's slice of this chain in isolation; this is the first to walk all of them together.
- [x] Wired into CI as the `full-chain-e2e` job (`.github/workflows/ci.yml`), alongside a separate `frontend-build` job (typecheck/lint/test/build) added in the same change for the dashboard described in §4. Same CI-execution caveat as M2.10/M2.12 above: verified locally, not yet exercised by GitHub Actions.
- [x] Live-verified: all stages PASS end-to-end, including a real AI-analysis stage (`provider=fake`/`model=fake-model`, confirming no real LLM integration exists or ran) and, separately, a real HTTP 403 execution rejection proving the execution allowlist boundary holds.

### M2.16 — Framework Genericity Verification
This entry needs a different framing than M2.8–M2.15 above: it did not add a new capability. It documented and live-verified an existing architectural property — that non-frozen production code contains zero hardcoded references to this project's four demo service names, with the one deliberate, frozen, documented exception (`internal/execution/guard.go`'s `AllowedServices` allowlist, which is the intended safety boundary, not a genericity defect).
- [x] `docs/framework-boundary.md` added, documenting the boundary and the `AllowedServices` exception explicitly.
- [x] `test-m216-genericity.ps1` added: sends synthetic OTLP traffic for a never-before-seen service name through the existing, unmodified ingestion path, and confirms it is discovered, graphed, detected, correlated, and RCA'd identically to the four demo services — then confirms the execution stage correctly rejects it with HTTP 403 (`"target service is not strictly allowlisted for execution"`), proving the *one* intentional exception holds without the container actually restarting (`docker inspect ... StartedAt` unchanged).
- [x] Both new files self-label `M2.16` in their own header comments, which is why this section uses that number rather than a session-local label — but note that M2.16 was not part of the roadmap's originally-planned M2.x sequence; it was scoped and added within a single working session as a verification/documentation milestone. Treat it as real and committed, not as evidence that an M2.17+ sequence is already planned.

---

## 4. Dashboard, Service Registry, Service Intelligence & Replay (post-M2.16 additions)

The work in this section was committed after M2.16 without repository `M2.x` numbers — commit messages and internal doc comments instead use session-local labels (`docs/registry.md` says "Phase 7B/7C/7D"; `internal/replay`'s doc comments say "Module 5"; the benchmark files' commit says "Module 6"). **These are not official roadmap milestones** — they are referenced here by label only because that label is what the actual commit message or doc comment uses, for traceability, not to imply they extend the M2.x sequence. All items below are committed to `main`.

- [x] **Canonical service registry** (`internal/registry`, SQLite-backed) — a persistent record of which services are known to this deployment, separate from the ephemeral, retention-based live dependency graph. Populated automatically from real observed OTLP telemetry (`internal/ingestion`), never from a hardcoded list. See `docs/registry.md`.
- [x] **Service intelligence** (`internal/serviceintel`) — a composed, read-only per-service view assembled at request time from the registry, the live dependency graph, and incident history; `GET /api/v1/services/{name}/intelligence`.
- [x] **Auth identity endpoint** — `GET /api/v1/auth/me`, gated by `PermissionView` like every other read endpoint.
- [x] **CORS support** (`internal/httpapi/cors.go`) for the separate-origin dashboard frontend; disabled entirely when `ATLAS_CORS_ORIGIN` is unset to an empty value.
- [x] **React dashboard frontend** (`frontend/`) — command center, incident list/detail (including RCA, blast radius, evidence timeline, remediation panel with approve/reject, execution panel, and an AI-analysis panel with a "Generate Analysis" control), dependency graph view, service registry page, and execution history — all reading real backend data, gated client-side by the real `GET /api/v1/auth/me` permission set (server-side 403 remains authoritative regardless of what the UI shows).
- [x] **Fixed a real routing defect while wiring the dashboard's AI-analysis panel**: `POST /api/v1/incidents/{id}/analyze` was dispatched into a GET-only handler (always 405) and, independent of that bug, was left unauthenticated even with `ATLAS_SECURITY_ENABLED=true` (a gap M2.11 explicitly deferred, see §3 above). Now routed to the real, already-complete `HandlePostAnalyze` handler, gated by `PermissionView`. Regression-tested, including the honest 503 the real evidence-grounding validator returns for an incident with no evidence (proving no analysis is ever fabricated to avoid that error).
- [x] **Read-only incident replay / simulation** (`internal/replay`) — `POST /api/v1/incidents/{id}/replay`, gated by `PermissionView`. Reads the incident's already-computed RCA fields read-only rather than re-invoking frozen `rca.Engine` (which reads live, mutable state — re-running it later would not be an honest historical replay). AI analysis and remediation planning are pure functions of their inputs, so those are re-invoked through fresh, throwaway `Engine`/`Planner` instances wrapping the same provider objects production uses — never the shared production instances — so a replay can never contaminate the real analysis cache or plan store (proven by a dedicated non-contamination test). `internal/execution` is never imported anywhere in replay's dependency chain, so a replay request structurally cannot reach approval or execution.
- [x] **Performance benchmarking** — real `go test -bench -benchmem` measurements added for four hot paths: OTLP trace normalization, dependency-graph edge insertion/lookup, correlation event processing, and OTLP trace ingestion end-to-end. These are recorded `ns/op`/`B/op`/`allocs/op` numbers from this dev machine, not production capacity or throughput figures — no requests/sec, SLA, or scalability claim is made or implied anywhere in this repository from this data, and none should be inferred from it.

---

## 5. Not started / Open

- [ ] **Security hardening: audit-log persistence.** `internal/execution/audit.go`'s execution audit trail (`ExecutionRecord`s — who executed what, when, with what verification outcome) is confirmed still an in-memory `map[string]*ExecutionRecord`, wiped on every restart. Authentication/RBAC on approve/execute themselves is done (M2.9/M2.11, above) — this item is specifically about the audit trail's durability. **BLOCKED / FROZEN-PATH ISSUE**: addressing it requires modifying `internal/execution`, which is frozen for the remainder of this working session per explicit instruction. Not implemented; the frozen boundary was not weakened or bypassed to work around this.
- [ ] **Reliable distributed execution**: worker pool, leases, retries beyond the single in-process idempotency map. **OPEN / OPTIONAL / SPECULATIVE** — only matters for multi-instance execution, which this project does not currently run. Not implemented, not scoped as a committed milestone.
- [ ] **RCA temporal-precedence scoring** remains dormant for nested cascades (M2.7.2, §2) — the collection mechanism (`Incident.TraceIDs`) is live; the scoring activation was investigated, verified working in isolation, and deliberately reverted because completing it correctly requires touching frozen `rca.Engine.Analyze`'s scoring-weight logic. Same frozen-path status as the audit-log item above.
- [ ] `test-m27-docker.ps1`'s HTTP-planner-gated full cascade plan → execute path is still not proven end-to-end through the *temporal-precedence* route specifically (§2, "Known defects/limitations still open" item 3) — a *different* route (M2.7.3's causal-attribution layer) does reach a clean execute cycle live.
- [ ] CI's `go test -race` step (M2.10) and the `docker-e2e`/`full-chain-e2e` jobs (M2.12/M2.15) have all been verified locally but have never actually been exercised by GitHub Actions, since nothing in this repository has been pushed to `origin/main`.

## 6. Housekeeping / decisions needed

- **`services/control-plane` (Spring Boot) and `agents/atlas-agent` (Go)** are unbuilt M0-era stubs, not part of the actual architecture (study guide never mentions them), yet still built in CI and run in `docker-compose.yml`. Still undecided as of this update — no commits have touched either directory since M1.2. Decide: retire them, or repurpose them for a real future milestone (see §7).
- ~~CI (`ci.yml`) requests Go 1.26 / Java 25; local dev machine has Go 1.25.0 — confirm this isn't silently masking a version-specific issue.~~ **Resolved by M2.10** (§3): CI now pins Go 1.25 / Java 24, matching the repository's own `go.mod`/`pom.xml` declarations.
- ~~Test coverage gap: `ingestion`, `normalization`, `httpapi`, and several model packages still have no unit tests at all.~~ **Resolved**: `internal/ingestion` and `internal/normalization` gained coverage in M2.10; `internal/httpapi` gained coverage across M2.11 and the dashboard/registry/replay work in §4 (10 test files as of this update); `internal/propagation` gained a dedicated test file in M2.14.
- CI's Go/Java build steps still only cover `services/intelligence-engine`, `agents/atlas-agent`, and `services/control-plane` — the actual lab application services (`gateway`, `order-service`, `inventory-service`, `payment-service`) have no CI coverage at all, before or after every milestone in this document. Not addressed by any milestone so far; still an open gap.

## 7. Reconciling the new 18-module roadmap

The alternate M0-M18 plan (Spring Boot control plane, per-service Go agents, Kafka pipeline, leases/workers, chaos, dashboard, replay, security, perf, final E2E) largely **already exists** under this project's own M2.1-M2.16 numbering (see §1-§4), just built as one Go service instead of split across a message bus and satellite agents. Adopting it literally would mean either:
- reviving the dead `control-plane`/`atlas-agent` stubs and a Kafka bus for no demonstrated architectural benefit over what's already verified, **or**
- rewriting working, tested M2.1-M2.16 code — which conflicts with the "don't silently redesign previous modules" rule.

**Recommendation (unchanged since 2026-08-24, and followed since):** keep the current architecture and numbering, continue as `M2.x` for pure additions (this is exactly what happened through M2.16). Only pull in Kafka / separate control-plane+agents / lease-based distributed workers if there's a specific reason to want them (e.g. deliberately demonstrating message-bus and distributed-systems patterns for a resume) — that's a scope decision, not a correction, and is called out here rather than assumed.

**Note on non-`M2.x` labels used in commit history (§4):** the dashboard/registry/service-intelligence/replay/benchmarking work was tracked with session-local labels ("Phase 7", "Module 3" through "Module 7") rather than repository `M2.x` numbers, unlike M2.8-M2.16 which self-label `M2.x` in their own committed source. Do not read those session-local labels as an extension of this roadmap's numbering, and do not assume a "Module 8" or an "M2.17" is implied or already planned by their existence.
