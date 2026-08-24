# ATLAS Roadmap & Status Checklist

Source of truth for "what's actually done" vs "what's planned." Generated from a direct audit of the repository (git history, code, `go build`/`go test`, and multiple live `docker compose` E2E runs) on 2026-08-24 — not from prior verification reports alone, several of which overstated coverage or described scenarios that didn't reproduce (see §2). Updated same day after implementing and live-verifying M2.7.1 (see `docs/m271_verification_report.md` for full command output).

Numbering follows the milestone scheme actually used in the repo (`M0`, `M1.x`, `M2.x`) rather than the alternate 18-module ChatGPT plan, per the reconciliation in §5.

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
- [x] ~~Regression~~ **Fixed & verified live**: the detector's `EventType != "SPAN"` check never matched real events (`"TRACE_SPAN"`), so detection was inert in the live pipeline since this milestone's original commit. Fixed, covered by a new regression test (`internal/incidentdetector/detector_test.go`), and confirmed working in a real `docker compose` run on 2026-08-24 — real incidents were created from real traffic for the first time. Still **uncommitted**.
- [x] RCA confidence threshold reverted `< 30` → `< 40` (the original, frozen value) with a new regression test (`internal/rca/engine_test.go`) locking in the boundary. Confirmed correct live: two incidents scored exactly 30 during the E2E run and correctly stayed LOW confidence under `< 40` (they would have wrongly been promoted to MEDIUM under the uncommitted `< 30`).
- [x] **Fixed & verified live (M2.7.1)**: the detector's status-code fallback checked `http.status_code`/`http.response.status_code` attributes this project's real Micrometer/Spring telemetry never populates (the actual key is `status`). Before this fix, `atlas-payment-service` — a pure sink with no outbound calls of its own — could never register an error at all (its own span's `Status` field also never becomes `ERROR`/`5xx` under this instrumentation). Fixed with regression tests (`TestProcessEvent_MicrometerStatusAttributeMarksError` etc.), confirmed live: payment-service now correctly generates its own incidents from real traffic.
- [x] **Cross-service incident correlation added (M2.7.1)**, resolving the gap noted below in the previous version of this doc: `internal/incidentmanager/correlator.go` groups incidents connected through the M2.3 dependency graph within a time window, and selects a causally-correct primary via a caller/callee sink rule (a service can only be primary if no other group member is something it calls). Metadata-only (`correlationGroupId`, `primaryIncidentId`, `relatedIncidentIds`) — `Incident.Status` and `rca.Engine` itself are both untouched. Verified live: a real 3-service cascade (payment→order→gateway) correctly grouped into one `correlationGroupId` with **payment-service correctly selected as primary**, not one of its callers.
- [ ] ⚠️ **New structural finding (M2.7.1, not fixed — logged for a future RCA-scoring milestone)**: even with correlation correctly identifying the true root cause as primary, `rca.Engine`'s own scoring can still legitimately return AMBIGUOUS or LOW-confidence for it. A caller whose dependency fails earns both its own error-rate evidence (+25) *and* a dependency-error signal (+20) = 45; the sink that actually failed only ever earns its own error-rate evidence (+25), since it has nothing to depend on and can never earn that bonus (the trace-based temporal-precedence bonus, +30, is separately dormant — `Incident.TraceIDs` is never populated). Practical effect, confirmed live twice: a real multi-hop cascade structurally cannot reach non-ambiguous, execution-eligible RCA for the true root cause under the current scoring formula, independent of correlation. Correlation didn't cause this — it exposed it: before correlation existed, every incident had exactly one candidate (itself), so it was trivially "non-ambiguous" by default and this weakness was invisible. See `docs/m271_verification_report.md` for the full trace and exact score breakdown.

### M2.5 — AI-Assisted Incident Reasoning
- [x] Provider interface (Fake/Gemini), bounded context builder
- [x] Prompt-injection defense, facts/inferences separation, evidence grounding
- [x] `ATLAS_AI_ENABLED=false` fallback path

### M2.6 — Remediation Planning & Safety Engine
- [x] Planner with action allowlist, safety validator, approval lifecycle
- [x] Confirmed: no `os/exec`, no shell, no Docker/K8s CLI calls
- [x] `APPROVED != EXECUTED` boundary enforced

---

## 2. M2.7 Controlled Execution Engine + M2.7.1 Correlation & Hardening

Code is complete in the working tree and compiles clean (`go build`/`go vet`/`go test`/`go test -race` all pass). Multiple live `docker compose` E2E runs were performed on 2026-08-24. **Still uncommitted** — awaiting review (see `docs/m271_verification_report.md` for full command output and exact live results).

- [x] Execution guard (enabled flag, approval status, fingerprint match, action/service allowlist, evidence check)
- [x] Idempotency by `(planId, actionId)` — verified against a real Docker restart, not just the fake executor
- [x] Typed Docker SDK adapter (`RestartService`, `Observe`, `Investigate`) — no shell/CLI execution
- [x] Async post-execution verification against M2.4 incident state
- [x] Execution HTTP API (`execute`, `GET /executions/{id}`, `GET /incidents/{id}/executions`)
- [x] Unit tests for guard/engine (`go test ./internal/execution/...` passes)
- [x] Docker build verified — `docker compose up -d --build` succeeds, all 8 containers reach healthy
- [x] `internal/infrastructure/docker` unit tests added (`adapter_test.go`, 4 tests via a `ContainerRestarter` interface seam, no live daemon needed)
- [x] **Real Docker restart proven against live infrastructure**: `TestDockerAdapter_RealRestartAgainstLiveContainer` (opt-in, `ATLAS_DOCKER_INTEGRATION_TEST=true`) runs the real, unmodified `execution.Engine` + real Docker adapter against a live `atlas-payment-service-1` container. `docker inspect` independently confirmed the container's `StartedAt` matched the test's execution timestamp — a genuine restart, not a claim.
- [x] `-race` verified clean (run inside a Linux container; this dev machine's native cgo/gcc toolchain is broken)
- [x] Cross-service correlation added and verified live (see §1)
- [x] Correlation succeeded, and primary selection succeeded, against real live traffic (`test-m27-docker.ps1` Scenario A: a real 3-service cascade correctly grouped into one `correlationGroupId` with payment-service — the true root — correctly selected as primary, never one of its callers).
- [x] Execution infrastructure verified independently of the above, against real infrastructure (guard validation, real Docker restart, idempotency — see `docs/m271_verification_report.md`).
- [ ] ⚠️ **Full cascade plan → execute flow is blocked by existing RCA confidence limitations, not by anything built in M2.7.1.** Neither live scenario in `test-m27-docker.ps1` reaches plan execution:
  - **Scenario A** (real cascade via the gateway): correlation correctly identifies payment-service as primary; `rca.Engine`'s own (unmodified) scoring lands on AMBIGUOUS; M2.6 correctly refuses to plan a HIGH-risk action.
  - **Scenario B** (isolated payment-only failure, bypassing gateway/order-service): reaches non-ambiguous RCA but still hits a second M2.6 gate (LOW confidence, same root mechanism) — plan generation blocked.
  - The original `docs/m27_verification_report.md`'s "restart payment-service" narrative is **not** reproduced end to end by either scenario. What *is* proven, decoupled from the RCA gate above by deliberate design, is the execution mechanics themselves (guard, idempotency, real Docker restart) via a Go-level integration test that constructs an already-approved plan directly rather than routing through the HTTP planner/confidence gate. See "Known limitation carried into future milestone" in `docs/m271_verification_report.md` — the recommended fix (M2.7.2) is populating `Incident.TraceIDs` to activate the RCA engine's existing, currently-dormant temporal-precedence bonus.
- [ ] Nothing is committed to git yet

### Defects fixed across this session (uncommitted)
1. ~~`docker-compose.yml` defaulted `ATLAS_EXECUTION_ENABLED=true`~~ — now `${ATLAS_EXECUTION_ENABLED:-false}`, safe by default, opt-in via env var. Docker-socket mount still present but inert unless explicitly enabled.
2. ~~RCA confidence threshold silently changed `< 40` → `< 30`~~ — reverted, covered by a regression test, confirmed correct against live data.
3. ~~Detector `EventType` bug had no regression test~~ — added, verified against live traffic.
4. ~~`test-m27-docker.ps1`'s failure-injection didn't reliably target payment-service~~ — root-caused twice (inventory contention, then wrong sandbox trigger) and fixed; script now correctly reaches payment-service reliably in both scenarios.
5. ~~Detector's status-code attribute-key mismatch~~ (§1) — fixed with regression tests, confirmed live.
6. ~~`internal/infrastructure/docker` had zero unit tests~~ — added (mocked + live-integration).

### Known defects still open (deferred, logged per explicit decision — not fixed this session)
7. **Structural RCA scoring gap** — see §1. Scoped as a dedicated future milestone (**M2.7.2**): populate `Incident.TraceIDs` to activate the RCA engine's existing, currently-dormant temporal-precedence bonus (the most promising lead, since that mechanism already exists and was designed for exactly this) rather than rebalancing the scoring formula's point values by trial and error.
8. Verbose `slog.Info` debug logging left in the hot path of `incidentdetector.ProcessEvent` and `EvaluateAll` — should drop to `Debug` or be removed. (Not fixed — out of approved scope.)
9. Execution-endpoint authentication/RBAC — explicitly deferred to a future security milestone per instruction.

---

## 3. Not started

Pure additions on top of the existing architecture — no rewrite required:

- [ ] Chaos engineering (fault injection: kill/latency/CPU/mem/DB-exhaustion scenarios)
- [ ] Dashboard / UI
- [ ] Replay / simulation mode for historical incidents
- [ ] Security hardening: authN/authZ, RBAC on approve/execute endpoints, audit-log persistence (current execution "audit" is an in-memory map, wiped on restart)
- [ ] Performance benchmarking (real measured numbers, not estimates)
- [ ] Reliable distributed execution: worker pool, leases, retries beyond a single in-process idempotency map (only matters if you want multi-instance execution)
- [ ] Final end-to-end validation script covering the full chaos → detect → RCA → AI → plan → approve → execute → verify chain

## 4. Housekeeping / decisions needed

- **`services/control-plane` (Spring Boot) and `agents/atlas-agent` (Go)** are unbuilt M0-era stubs, not part of the actual architecture (study guide never mentions them), yet still built in CI and run in `docker-compose.yml`. Decide: retire them, or repurpose them for a real future milestone (see §5).
- CI (`ci.yml`) requests Go 1.26 / Java 25; local dev machine has Go 1.25.0 — confirm this isn't silently masking a version-specific issue.
- Test coverage gap: `ingestion`, `normalization`, `httpapi`, and several model packages still have no unit tests at all (`incidentdetector`, `rca`, `incidentmanager`, and `infrastructure/docker` all gained tests this session).

## 5. Reconciling the new 18-module roadmap

The alternate M0-M18 plan (Spring Boot control plane, per-service Go agents, Kafka pipeline, leases/workers, chaos, dashboard, replay, security, perf, final E2E) largely **already exists** under this project's own M2.1-M2.7 numbering, just built as one Go service instead of split across a message bus and satellite agents. Adopting it literally would mean either:
- reviving the dead `control-plane`/`atlas-agent` stubs and a Kafka bus for no demonstrated architectural benefit over what's already verified, **or**
- rewriting working, tested M2.1-M2.6 code — which conflicts with the "don't silently redesign previous modules" rule.

**Recommendation:** keep the current architecture and numbering, continue as `M2.8`, `M2.9`, ... for chaos engineering, dashboard, security, replay, and performance work (all pure additions, zero rewrite risk). Only pull in Kafka / separate control-plane+agents / lease-based distributed workers if there's a specific reason to want them (e.g. deliberately demonstrating message-bus and distributed-systems patterns for a resume) — that's a scope decision, not a correction, and is called out here rather than assumed.
