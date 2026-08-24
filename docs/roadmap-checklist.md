# ATLAS Roadmap & Status Checklist

Source of truth for "what's actually done" vs "what's planned." Generated from a direct audit of the repository (git history, code, `go build`/`go test`, and a live `docker compose` E2E run) on 2026-08-24 — not from prior verification reports alone, several of which overstated coverage or described scenarios that didn't reproduce (see §2).

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
- [ ] ⚠️ **New gap found during the live E2E run**: incidents are currently scoped per-service (`affectedServices` contains only the one degrading service), not linked into a cross-service cascade. When a failure at one service (e.g. inventory) cascades upward, ATLAS opens **separate, independent incidents** for each affected service (e.g. `atlas-gateway degradation`, `atlas-order-service degradation`) and RCA scores each in isolation — so a downstream victim can be reported as its own "root cause" rather than tracing back to the true originator. This contradicts the architecture doc's own worked example (§33, §56) where a single Payment-rooted incident is expected. Pre-existing behavior, not introduced by this session's changes; needs a dedicated RCA/incident-manager design pass to correlate related incidents across the dependency graph before scoring root cause.

### M2.5 — AI-Assisted Incident Reasoning
- [x] Provider interface (Fake/Gemini), bounded context builder
- [x] Prompt-injection defense, facts/inferences separation, evidence grounding
- [x] `ATLAS_AI_ENABLED=false` fallback path

### M2.6 — Remediation Planning & Safety Engine
- [x] Planner with action allowlist, safety validator, approval lifecycle
- [x] Confirmed: no `os/exec`, no shell, no Docker/K8s CLI calls
- [x] `APPROVED != EXECUTED` boundary enforced

---

## 2. Implemented but NOT yet committed / verified — M2.7 Controlled Execution Engine

Code is complete in the working tree and compiles clean (`go build ./...` passes, `go test ./...` passes). A live `docker compose` E2E run was performed on 2026-08-24 with `ATLAS_EXECUTION_ENABLED=true`. Not frozen yet — treat as **in review**, not done:

- [x] Execution guard (enabled flag, approval status, fingerprint match, action/service allowlist, evidence check)
- [x] Idempotency by `(planId, actionId)`
- [x] Typed Docker SDK adapter (`RestartService`, `Observe`, `Investigate`) — no shell/CLI execution
- [x] Async post-execution verification against M2.4 incident state
- [x] Execution HTTP API (`execute`, `GET /executions/{id}`, `GET /incidents/{id}/executions`)
- [x] Unit tests for guard/engine (`go test ./internal/execution/...` passes)
- [x] Docker build verified — `docker compose up -d --build` succeeds, all 8 containers reach healthy
- [ ] **Live E2E scenario does NOT reproduce the claims in `docs/m27_verification_report.md`.** Ran `test-m27-docker.ps1` for real: it never got past plan generation ("Remediation plan was not generated in time"). Root cause, confirmed from container logs and the live incidents API:
  - The script's "PAYFAIL" burst (`quantity: 3`) mostly triggers `409 CONFLICT "Insufficient inventory"` at order-service (stock exhausted by the earlier "normal" burst), not a payment failure. Only one request actually hit payment, as a client-side timeout artifact.
  - Two incidents *were* created (`atlas-gateway degradation`, `atlas-order-service degradation` — confirming the M2.4 detector fix works end-to-end) but both auto-resolved (~80s, per the 60s window + 30s recovery buffer) before the script's polling loop checked `/incidents/open`.
  - **No incident ever targeted `atlas-payment-service`**, so even with better timing this scenario could never have produced the "restart payment-service" plan the verification report walks through. `docs/m27_verification_report.md`'s specific narrative is not currently reproducible with this script against this codebase — treat that report as aspirational/unverified, not evidence.
  - This is a test-script defect (unreliable failure injection), not something this session's code changes touched. Deferred — see the new item below.
- [ ] `internal/infrastructure/docker` has **zero unit tests** — the actual restart logic is only exercised by the (currently unreliable) E2E script
- [ ] `-race` not verifiable on this dev machine (broken local cgo/gcc toolchain) — confirm CI still runs it
- [ ] Nothing is committed to git yet

### Defects fixed this session (uncommitted)
1. ~~`docker-compose.yml` defaulted `ATLAS_EXECUTION_ENABLED=true`~~ — now `${ATLAS_EXECUTION_ENABLED:-false}`, safe by default, opt-in via env var (matches the file's existing `${VAR:-default}` convention). Docker-socket mount is still present but inert unless explicitly enabled.
2. ~~RCA confidence threshold silently changed `< 40` → `< 30`~~ — reverted to the original frozen value, now covered by a regression test. Confirmed correct against live data (see §1).
3. ~~Detector `EventType` bug had no regression test~~ — added (`internal/incidentdetector/detector_test.go`), verified against live traffic.

### Known defects still open (deferred, not fixed this session — logged per explicit decision)
4. **`test-m27-docker.ps1`'s failure-injection doesn't reliably target payment-service** — see the E2E result above. The script needs a scenario that reliably starves *payment* specifically (e.g. a dedicated `/api/payments` failure trigger) rather than relying on inventory exhaustion as a side effect of request volume.
5. **RCA doesn't correlate cascading incidents across services** — see §1's new M2.4 gap. Needed before the "M2.7 restarts the true root cause" story can work end-to-end.
6. Verbose `slog.Info` debug logging left in the hot path of `incidentdetector.ProcessEvent` and `EvaluateAll` — should drop to `Debug` or be removed. (Not fixed — out of the approved scope for this session.)

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
- Test coverage gap: `incidentmanager`, `ingestion`, `normalization`, `httpapi`, `infrastructure/docker`, and several model packages still have no unit tests at all (`incidentdetector` and `rca` gained regression tests this session).

## 5. Reconciling the new 18-module roadmap

The alternate M0-M18 plan (Spring Boot control plane, per-service Go agents, Kafka pipeline, leases/workers, chaos, dashboard, replay, security, perf, final E2E) largely **already exists** under this project's own M2.1-M2.7 numbering, just built as one Go service instead of split across a message bus and satellite agents. Adopting it literally would mean either:
- reviving the dead `control-plane`/`atlas-agent` stubs and a Kafka bus for no demonstrated architectural benefit over what's already verified, **or**
- rewriting working, tested M2.1-M2.6 code — which conflicts with the "don't silently redesign previous modules" rule.

**Recommendation:** keep the current architecture and numbering, continue as `M2.8`, `M2.9`, ... for chaos engineering, dashboard, security, replay, and performance work (all pure additions, zero rewrite risk). Only pull in Kafka / separate control-plane+agents / lease-based distributed workers if there's a specific reason to want them (e.g. deliberately demonstrating message-bus and distributed-systems patterns for a resume) — that's a scope decision, not a correction, and is called out here rather than assumed.
