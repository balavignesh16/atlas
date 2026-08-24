# ATLAS M2.7.1 Verification Report — Incident Correlation & Execution Hardening

Generated 2026-08-24 from actual command output and live `docker compose` runs, not from aspirational description. Nothing in this milestone has been committed — this report is for review before that decision.

## Summary (read this first)

- ✅ **Correlation succeeded.** Cross-service incidents connected through the M2.3 dependency graph are correctly grouped, verified against real live traffic.
- ✅ **Primary selection succeeded.** The caller/callee sink rule correctly selects the true root-cause service as primary, never a caller showing propagated symptoms — verified against a real 3-service cascade.
- ✅ **Execution infrastructure verified.** Guard validation, idempotency, and a real Docker container restart are all proven against live infrastructure (evidence below).
- ⚠️ **Full cascade plan → execute flow is blocked by existing RCA confidence limitations.** This milestone does **not** demonstrate an approved plan executed against a real multi-hop cascade end to end. `rca.Engine`'s own scoring — deliberately left unmodified — can still return AMBIGUOUS or LOW confidence for a correctly-identified primary, and M2.6's safety validator then correctly refuses to plan a HIGH-risk action. That refusal is correct, pre-existing, by-design behavior, not a defect introduced here. See "Known limitation carried into future milestone" below.

## What was implemented

1. **Cross-service incident correlation** (`internal/incidentmanager/correlator.go`) — groups currently-open incidents that are connected through the observed M2.3 dependency graph and occurred within a configurable time window (`ATLAS_CORRELATION_WINDOW_SECONDS`, default 20s), and selects a causally-correct primary using a caller/callee sink rule: a service can be primary only if no other group member is something it calls. Metadata-only (`correlationGroupId`, `primaryIncidentId`, `relatedIncidentIds`) — no `Incident.Status` change, no merging/hiding. Runs after blast-radius calculation and before RCA in the evaluation loop (`cmd/intelligence-engine/main.go`). **`rca.Engine` was not modified — confirmed by `git diff`, which shows zero changes to `internal/rca/engine.go` against the last commit.**
2. **M2.4 detector fix** (`internal/incidentdetector/detector.go`) — the status-code fallback checked `http.status_code`/`http.response.status_code` attributes that this project's actual Micrometer/Spring telemetry never populates; the real key is `status`. Before this fix, any service with no outbound calls of its own (i.e. `atlas-payment-service`, a pure sink) could never register an error at all, since its server span's own `Status` field also never becomes `ERROR`/`5xx` under this instrumentation stack. Discovered via live traffic, not inspection.
3. **Docker adapter test coverage** — introduced a `ContainerRestarter` interface seam (`internal/infrastructure/docker/client.go`) so the adapter's control flow is unit-testable without a live daemon (`adapter_test.go`), plus a separate, opt-in integration test (`docker_integration_test.go`, gated behind `ATLAS_DOCKER_INTEGRATION_TEST=true`) that runs the real `execution.Engine` against the real Docker SDK adapter and a live container.
4. **E2E test script** (`test-m27-docker.ps1`) — rewritten into two scenarios (detailed below); the original single-scenario script never reliably reached payment-service at all (see `docs/roadmap-checklist.md` for the two prior root-caused defects: inventory contention, then the wrong sandbox trigger).

## Test results (re-run for this report)

### `go build` / `go vet` — clean (both native Windows and cross-compiled Linux)

### `go test ./...` — all green
```
ok  	github.com/atlas/intelligence-engine/cmd/intelligence-engine
ok  	github.com/atlas/intelligence-engine/internal/aireasoning
ok  	github.com/atlas/intelligence-engine/internal/buffer
ok  	github.com/atlas/intelligence-engine/internal/correlation
ok  	github.com/atlas/intelligence-engine/internal/execution
ok  	github.com/atlas/intelligence-engine/internal/graph
ok  	github.com/atlas/intelligence-engine/internal/incidentdetector
ok  	github.com/atlas/intelligence-engine/internal/incidentmanager
ok  	github.com/atlas/intelligence-engine/internal/infrastructure/docker
ok  	github.com/atlas/intelligence-engine/internal/rca
ok  	github.com/atlas/intelligence-engine/internal/remediation
ok  	github.com/atlas/intelligence-engine/internal/timeline
```

### `go test -race ./...` — clean
Run inside a Linux container (this dev machine's native cgo/gcc toolchain is broken, confirmed earlier this session); no data races detected across the concurrent evaluation loop, correlator, execution store, or incident manager.

## Execution verification evidence

### 1. Guard validation evidence
`internal/execution/guard_test.go` exercises the real, unmodified `Guard.Check` against four cases, all passing:
- `TestGuard_Disabled` — `ATLAS_EXECUTION_ENABLED=false` → `ErrExecutionDisabled`, no execution attempted.
- `TestGuard_NotApproved` — plan not in `APPROVED` status → `ErrPlanNotApproved`.
- `TestGuard_InvalidFingerprint` — approval fingerprint no longer matches the current plan fingerprint (plan was modified post-approval) → `ErrApprovalInvalid`.
- `TestGuard_Valid` — a correctly approved, unmodified plan targeting an allowlisted service passes and returns the action.

### 2. Real Docker restart evidence
`TestDockerAdapter_RealRestartAgainstLiveContainer` (opt-in, `ATLAS_DOCKER_INTEGRATION_TEST=true`) runs the real, unmodified `execution.Engine` + real Docker SDK adapter against a live `atlas-payment-service-1` container:
```
=== RUN   TestDockerAdapter_RealRestartAgainstLiveContainer
[INFO] Restarting strictly mapped Docker container: atlas-payment-service-1
    docker_integration_test.go:90: Real Docker restart succeeded: Successfully restarted container atlas-payment-service-1
--- PASS: TestDockerAdapter_RealRestartAgainstLiveContainer (0.67s)
```
Independently confirmed outside the test process via `docker inspect atlas-payment-service-1`: `StartedAt` matched the test's execution timestamp exactly — a genuine restart, not a claim.

### 3. Idempotency evidence
The same test's second call (same `planId`/`actionId`) against the same live container:
```
[INFO] Execution Engine: duplicate request for plan integration-test-plan action integration-test-action, returning existing record
```
and the test asserts `record2.ExecutionID == record.ExecutionID` — the cached record was returned, the container was **not** restarted a second time. Verified against real infrastructure, not the fake executor.

## Docker E2E — Scenario A (cascade: detection + correlation)
Real traffic through the gateway (`quantity=4` → deterministic payment-service 500). Result:
- **PASS** — correlation correctly grouped 5 incidents (payment ×1, order-service ×2, gateway ×2 — Micrometer emits both a client- and server-side span per hop) into one `correlationGroupId`, with the **payment-service incident correctly selected as `primaryIncidentId`** via the caller/callee sink rule.
- RCA's own verdict on the merged primary came back `AMBIGUOUS` (between order-service and gateway) — see "Known limitation" below. M2.6's safety validator correctly refused to generate a plan: `"cannot perform HIGH/CRITICAL action on AMBIGUOUS RCA"`.
- Both outcomes (correct correlation, correct safety refusal) are asserted as pass conditions in the script; RCA's confidence verdict is logged, not asserted on. **This scenario does not reach plan execution — see limitation below.**

## Docker E2E — Scenario B (isolated failure)
Traffic sent directly to `payment-service:8086`, bypassing gateway/order-service entirely (no cascade possible).
- Non-ambiguous incident formed: `rootCause.service=atlas-payment-service, confidence=LOW, score=25`.
- Plan generation **still failed**: `"cannot perform HIGH/CRITICAL action on LOW confidence RCA"` — a second, separate M2.6 safety gate (documented, pre-existing). **This scenario also does not reach plan execution.** Root cause traced below.

## Known limitation carried into future milestone

Neither live scenario above reaches a full plan → approve → execute → verify pass. This is a real, deliberate finding, not an oversight:

- **RCA scoring can still produce AMBIGUOUS/LOW confidence for a correctly-identified primary.** Correlation correctly names the true root cause as primary in both scenarios (verified). But `rca.Engine.Analyze` (unmodified, per instruction) scores a caller whose dependency fails higher than the dependency itself: the caller earns both its own error-rate evidence (+25) *and* a dependency-error signal (+20) = 45, while the true root — a sink with nothing to depend on — only ever earns its own error-rate evidence (+25), structurally below both the AMBIGUOUS-avoidance margin and the 40-point MEDIUM-confidence threshold.
- **`Incident.TraceIDs` are currently unavailable.** The scoring formula already has a temporal-precedence bonus (+30) designed for exactly this situation — it would let the true root, which fails *first* in time, outscore its callers regardless of the dependency-error asymmetry above. But it depends on `Incident.TraceIDs` being populated with real trace/span data, which nothing in the current pipeline does; the bonus exists in code but has never actually fired.
- **Future M2.7.2 will improve RCA temporal reasoning.** The recommended next step is populating `Incident.TraceIDs` so the existing (already-written, already-tested-for-ambiguity) temporal-precedence mechanism can activate, rather than rebalancing the point values themselves — that reuses a designed-but-dormant mechanism instead of retuning scoring by trial and error. This is scoped as its own milestone specifically because it requires touching `rca.Engine` and/or the incident-signal pipeline that populates `TraceIDs`, both explicitly out of bounds for M2.7.1.

Correlation did not cause this gap — it exposed it. Before correlation existed, every incident had exactly one candidate (itself) and was therefore trivially "non-ambiguous" by default, so this weakness in `rca.Engine` was invisible.

## What was NOT changed
- `rca.Engine` — zero modifications, confirmed via `git diff` (empty).
- No authentication/RBAC/security layer — explicitly out of scope, deferred.
- No changes to M2.6's planner or safety validator — the AMBIGUOUS/LOW-confidence gates are pre-existing, documented, correct behavior.
- No changes to `services/control-plane` or `agents/atlas-agent` (still-vestigial, per earlier session decision).

## Commit status
**Nothing has been committed.** Working tree contains all changes described above plus the earlier session's fixes (detector SPAN/TRACE_SPAN bug, RCA threshold revert to its original frozen value, docker-compose safety default). Awaiting manual review.
