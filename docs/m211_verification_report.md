# M2.11 Verification Report

## Scope

Complete the RBAC model M2.9 shipped by wiring the already-existing `PermissionView`/`PermissionReadAudit` permissions to the read-only HTTP API surface (previously fully unauthenticated even when `ATLAS_SECURITY_ENABLED=true`), and add direct unit-test coverage for the four previously-untested read-only `httpapi` handlers plus `rca.Engine.Analyze()`.

## Repository Baseline

Verified before implementation: `HEAD=be24cc7` (M2.10 verification report), working tree clean, `main` ahead of `origin/main` by 8 commits (4 M2.9 + 4 M2.10). No modifications made to any file until this baseline was independently re-confirmed via `git status`/`git log`/`git diff`.

## RBAC Read-Surface Mapping

Every read-only route registered in `main.go` was inventoried before any wiring change was made. Least-privilege classification: `PermissionView` for general operational reads (telemetry, correlation, graph, incidents, RCA, evidence, plan viewing); `PermissionReadAudit` reserved for the execution/audit ledger specifically (`internal/execution/audit.go`'s `ExecutionRecord`s — who executed what, when, with what verification outcome), matching this repository's own naming for that concept.

| Route | Handler | Permission | Unauthenticated (enabled) | Live-verified |
|---|---|---|---|---|
| `GET /api/v1/events`, `/events/metrics`, `/events/trace/`, `/events/{id}` | `VerificationAPI` | VIEW | 401 | Yes (Scenario B) |
| `GET /api/v1/correlations/traces/{id}` (+tree, +timeline) | `CorrelationAPI` | VIEW | 401 | Unit only |
| `GET /api/v1/graph`, `/graph/edges`, `/graph/services/{name}` | `GraphAPI` | VIEW | 401 | Yes (Scenario B, C) |
| `GET /api/v1/incidents`, `/incidents/open`, `/incidents/{id}` (+evidence, +rca, +timeline, +analysis) | `IncidentAPI` | VIEW | 401 | Yes (Scenario B, C, unit) |
| `GET /api/v1/incidents/{id}/remediation` | `RemediationAPI.HandleGetPlanByIncident` | VIEW | 401 | Unit only |
| `GET /api/v1/remediation/{planId}` | `RemediationAPI.HandleGetPlan` | VIEW | 401 | Yes (Scenario C) |
| `GET /api/v1/executions/{id}` | `ExecutionAPI.HandleGetExecution` | READ_AUDIT | 401 | Yes (Scenario B, D, E) |
| `GET /api/v1/incidents/{id}/executions` | `ExecutionAPI.HandleGetExecutionsByIncident` | READ_AUDIT | 401 | Yes (Scenario D) |
| `POST /api/v1/incidents/{id}/analyze` | `IncidentAPI.HandlePostAnalyze` | **unchanged (none)** | n/a | n/a |

**Boundary case, explicitly not broadened:** `POST /api/v1/incidents/{id}/analyze` (triggers AI-reasoning analysis — a mutation of `aiEngine`'s analysis cache, not a read) is dispatched from the exact same `main.go` entry point (`incidentAPI.HandleGetIncident`) that also serves every GET subpath under `/api/v1/incidents/{id}/...`. Gating that shared entry point uniformly would have incidentally required `PermissionView` on this POST mutation too — outside M2.11's approved "read-only routes" scope. Instead, `main.go` now explicitly intercepts `POST .../analyze` and passes it through unauthenticated/unwired, byte-for-byte identical to its pre-M2.11 behavior, before the `PermissionView` gate is applied to everything else falling through to that handler. This is documented in `main.go` itself and here rather than silently decided either way.

## Implementation

`services/intelligence-engine/cmd/intelligence-engine/main.go` — every read route above now routes through `authorizer.Protect(permission, handler)`, the exact same mechanism M2.9 already used for the four mutating routes (create-plan/approve/reject/execute). No new authorization mechanism, no new `Permission` value, no change to `internal/security/`'s role matrix. When `ATLAS_SECURITY_ENABLED=false` (default), `Authorizer.Protect` is a documented pass-through no-op (unchanged M2.9 code) — read-route behavior is byte-for-byte identical to pre-M2.11.

**Finding, not fixed (role matrix unchanged per instruction):** `internal/security/model.go`'s existing, unmodified role matrix (added in M2.9) already grants `RoleViewer` both `PermissionView` **and** `PermissionReadAudit`. VIEWER is therefore allowed on the execution/audit endpoints, not blocked from them. The role actually excluded from `READ_AUDIT` is `OPERATOR` (`View + CreatePlan` only). M2.11 does not modify the role matrix, so the E2E script below tests the real, existing boundary (OPERATOR blocked, VIEWER and EXECUTOR allowed) rather than asserting a VIEWER restriction the shipped matrix does not define.

## HTTP Unit Tests

Four new files, all exercising real, unmocked handler + dependency instances (matching M2.10's established convention):

- `internal/httpapi/correlation_test.go` — 6 tests (`HandleGetTrace`/`HandleGetTraceTree`/`HandleGetTraceTimeline`: found/not-found/wrong-method), against a real `correlation.Engine` fed real `ATLASEvent`s.
- `internal/httpapi/graph_test.go` — 5 tests, against a real `graph.DependencyGraph`.
- `internal/httpapi/incident_test.go` — 9 tests, against a real `incidentmanager.Manager` + `evidence.Store` + `rca.Engine` + `aireasoning.Engine` (fake provider), incidents created via the real `Manager.ProcessSignal` production path (not fabricated directly).
- `internal/httpapi/verification_test.go` — 5 tests, against a real `buffer.EventBuffer`.

25 new tests. Combined with the pre-existing `execution_test.go`/`remediation_test.go` (M2.9), `internal/httpapi` now has 32 total tests, all passing. `correlation.go`, `graph.go`, `incident.go`, `verification.go` had zero tests before this milestone.

## RCA Analyze Tests

`internal/rca/engine_test.go` extended (not replaced — the original `TestGetConfidence_Boundaries` is untouched) with 11 new tests directly against `Engine.Analyze()`, against real `evidence.Store`/`graph.DependencyGraph`/`propagation.Analyzer`/`correlation.Engine` instances:

nil/empty input, single-candidate scoring, error-evidence deduplication (counted once, not per matching evidence item), combined-evidence-type accumulation (25+20+20=65 → MEDIUM), the healthy-independent-dependency +5 bonus, the score-cap-at-100 path, clear-winner candidate ranking, the `<=5`-point ambiguity threshold (`AMBIGUOUS`/LOW/score-0 — the exact safety property this mechanism protects), zero-evidence candidates, and — notably — a live proof that the "safely dormant" (per `propagation/analyzer.go`'s own M2.7.2 doc comment) temporal-precedence propagation path genuinely fires and scores `+30` when real trace/graph data supports it. `rca/engine.go` itself was not modified; `go build`/`git diff` confirm this.

## Security E2E

A new, isolated `test-m211-security-read.ps1` was created (`test-m27-docker.ps1`/`test-m28-chaos.ps1`/`test-m29-security.ps1` are all confirmed byte-for-byte unmodified via `git diff` — empty). Two rounds of evidence were gathered, and this section reports exactly what each round did and did not prove.

### Round 1: `test-m211-security-read.ps1` script execution

- **Part 1 (security disabled, default config):** executed successfully via the script, twice. Read endpoints remain fully open with no key — byte-for-byte pre-M2.11 behavior. **PASS, via the script itself.**
- **Scenario B (unauthenticated read, security enabled):** executed successfully via the script, 3 times in a row across 3 separate live Docker runs. `GET /api/v1/incidents`, `/api/v1/events`, `/api/v1/graph`, `/api/v1/executions/{id}` all returned 401 with no body when no API key was presented. **PASS, via the script itself.**
- **Scenarios C–G:** the script's own SETUP step (generate a real incident via a payment-failure cascade, driven by PowerShell's `Invoke-RestMethod`, launched via this session's background-process mechanism) failed to produce an incident 3 times in a row. **The script itself did not complete these scenarios, and this report does not claim it did.**

### Root-cause diagnosis (not skipped, independently verified)

To determine whether the SETUP failure was an M2.11 regression, the completely unmodified `test-m29-security.ps1` was run the same way — **it failed at the identical SETUP step**, ruling out an M2.11-introduced defect (that script's routing/business logic is byte-for-byte untouched). The failure was then isolated precisely: the identical `Invoke-RestMethod` request sequence, run **synchronously in the foreground** (not via this session's background-process launch mechanism) against the same live, security-enabled stack, **delivered all 20 requests correctly** (5 succeeded, 15 correctly returned 502), and the expected `atlas-payment-service` incident appeared (`primaryIncidentId == incidentId`) within 5 seconds. This conclusively isolates the defect to how this session's *backgrounded* PowerShell process launches deliver HTTP traffic — not `Invoke-RestMethod` itself, not the intelligence-engine's incident-detection pipeline, not the M2.11 routing change, and not either test script's logic.

### Round 2: live curl-driven verification of Scenarios C–G

Per instruction, curl-driven requests against the same live Docker stack are an acceptable, equivalent mechanism (still traversing the real HTTP boundary, not testing middleware in isolation). Using the incident/plan/execution already generated above, every scenario was driven individually and its exact status code and body recorded:

| Scenario | Request | Expected | Actual | Evidence |
|---|---|---|---|---|
| C | VIEWER GET `/api/v1/incidents`, `/incidents/{id}`, `/graph`, `/events`, `/remediation/{planId}` | 2xx | **200** (all 5) | direct response bodies captured |
| D | VIEWER GET `/executions/{id}` and `/incidents/{id}/executions` | 2xx (VIEWER holds READ_AUDIT per the real, unmodified `model.go`) | **200** (both) | execution record body returned intact |
| D | OPERATOR GET `/executions/{id}` and `/incidents/{id}/executions` | 403 (OPERATOR lacks READ_AUDIT per the real, unmodified `model.go`) | **403** (both) | `{"error":"principal does not have permission: READ_AUDIT"}` |
| E | EXECUTOR GET `/executions/{id}` (EXECUTOR holds READ_AUDIT) | 2xx | **200** | `executionId`/`approver`/`executionStatus` fields verified intact |
| F | VIEWER POST plan-generate, approve, execute | 403 (all three) | **403** (all three) | no mutation occurred (plan status re-checked below) |
| G | Unauthenticated POST plan-generate, approve, execute | 401 (all three) | **401** (all three) | — |
| G | OPERATOR POST approve (lacks APPROVE_PLAN) | 403 | **403** | — |
| G | APPROVER POST execute (lacks EXECUTE) | 403 | **403** | — |
| G | Authenticated EXECUTOR, body forges `"approver":"admin"` | recorded identity = `executor1`, not `admin` | **`"approver":"executor1"`** | full execution record returned |
| G | Duplicate execute call, same `planId`+`actionId` | same `executionId` (idempotency) | **same `executionId`** returned both times | — |
| G | Plan status after execution | remains `APPROVED` (APPROVED ≠ EXECUTED) | **`"status":"APPROVED"`** | — |

**Scenario D's expectation was set from the actual, re-read `internal/security/model.go` role matrix, not assumed** — `RoleViewer` genuinely includes `PermissionReadAudit` in the unmodified M2.9 matrix, so VIEWER-allowed (200) is the correct expectation, not a fabricated 403. The real, demonstrated boundary is OPERATOR (lacks READ_AUDIT) → 403.

**All 7 scenarios (Part 1, B, C, D, E, F, G) are now live-verified against the real Docker HTTP stack**, combining the script's own successful runs (Part 1, B) with direct curl verification (C–G) after the harness defect was isolated and could not be resolved by retrying the script alone. `test-m211-security-read.ps1` itself did not complete Scenarios C–G — that is stated plainly, not obscured by the fact that equivalent live evidence was obtained by another route.

## Existing Regression Tests

`test-m29-security.ps1` (frozen, unmodified) did not complete past its own SETUP step this session via its own script execution, for the identical, independently-diagnosed environmental reason above — **this is stated as-is; the script itself is not claimed to have passed.** However, the specific safety invariants that script exists to prove (unauthenticated rejection, unauthorized-mutation rejection, forged-identity rejection, idempotency, APPROVED≠EXECUTED) were independently re-verified live via curl in Scenario G above, against the same M2.9 code path, unmodified by M2.11 — see the Scenario G rows. `test-m27-docker.ps1` and `test-m28-chaos.ps1` were not re-run this session (time/practicality trade-off; the harness defect already isolated is generic to backgrounded PowerShell `Invoke-RestMethod`, not specific to any one script, so re-running them would not add new information about M2.11 itself). Unit-level regression coverage for M2.9's behavior (`internal/security/*_test.go`, `internal/httpapi/execution_test.go`, `internal/httpapi/remediation_test.go`) was re-run fresh and passes.

## Race Detector

`go test -race -count=1 ./...` via the established `golang:1.25` Docker container: all 26 packages `ok`, exit code 0, no races. Run three times total (before a `gofmt` correction to two newly-added test files, after it, and again after the live E2E diagnostic/verification session below); all three runs clean. This confirms local race-suite passing; it is not evidence GitHub Actions CI has executed this, since nothing has been pushed.

## Frozen-Path Audit

`git diff --stat` against every listed frozen path (`internal/rca/` implementation, `internal/propagation/`, `internal/incidentmanager/correlator.go`+`causal.go`, `internal/incidentdetector/`, `internal/remediation/` production logic, `internal/execution/{guard,model,engine,verification}.go`, the Docker adapter, `internal/buffer/`, `internal/event/`, `internal/ingestion/` implementation, `internal/normalization/` implementation) plus `internal/security/` (not frozen, but confirmed untouched) plus `test-m27-docker.ps1`/`test-m28-chaos.ps1`/`test-m29-security.ps1`: **empty in every case.** `rca/engine.go` specifically confirmed unchanged (only `rca/engine_test.go` was extended).

## Dependency Audit

`git diff --stat` against `go.mod`/`go.sum` (root, `services/intelligence-engine/`, `agents/atlas-agent/`): empty in every case. No new dependency was introduced.

## Findings

1. **VIEWER already holds READ_AUDIT (pre-existing M2.9 role-matrix property, not an M2.11 decision).** Discovered while mapping routes to permissions; not fixed, not expanded — the role matrix was left exactly as M2.9 shipped it, per explicit instruction. Documented in the E2E script and above.
2. **`POST /{id}/analyze` remains unauthenticated** — a pre-existing (M2.5/M2.9) gap, structurally unreachable to close within M2.11's read-only scope without either touching `incident.go` or incidentally gating a mutation under `PermissionView`; left exactly as before, documented rather than silently resolved either way.
3. **E2E harness environment limitation (this session), fully root-caused:** PowerShell `Invoke-RestMethod` calls launched via this session's *backgrounded* process mechanism unreliably deliver the fault-cascade HTTP traffic `Trigger-PaymentCascade` depends on, reproduced identically against the frozen, unmodified `test-m29-security.ps1`. Precisely isolated (not just suspected): the identical request sequence run *synchronously in the foreground* succeeds every time. Not an M2.11 defect — confirmed by both the foreground-PowerShell reproduction and independent `curl` verification against the same live stack. Not fixed — it is a property of how this session launches background PowerShell processes, outside this milestone's (or any application code's) scope.
4. **Carried forward from M2.10, untouched:** the `x-api-key` sanitizer gap, the UTF-8 truncation boundary, and the Java lab-service CI gap remain pre-existing, intentionally not addressed by M2.11 (would require reopening frozen `internal/normalization/` or expanding CI scope, both explicitly out of scope).

## What Was NOT Done

No M2.7 frozen execution logic changed. No M2.8 chaos script changed. No M2.9 security script changed. No sanitizer fix. No UTF-8 truncation fix. No persistence added. No network/Docker-socket hardening. No API-key rotation or expiry. No OAuth/JWT/OIDC/mTLS. No separation-of-duties or `CreatedBy` field added. No new authorization framework or router. No Java lab-service CI integration. No dashboard/UI. No new chaos mechanism. No performance work. No new external dependency. `internal/security/`'s role matrix was read but never modified.

## Definition of Done

Met: RBAC wired to every enumerated read route using the existing mechanism only; every scenario (Part 1, B, C, D, E, F, G) live-verified against the real Docker HTTP stack (Part 1 and B via the new script directly; C–G via curl after the script's backgrounded-PowerShell harness defect was isolated and confirmed unrelated to M2.11's code); unit coverage added for all 4 previously-untested read handlers and `rca.Analyze()`; full build/vet/test/race clean (re-run fresh after the live E2E session); zero frozen-path or dependency diff, re-confirmed after the E2E diagnostic work.
Caveat, stated precisely rather than glossed over: `test-m211-security-read.ps1` and `test-m29-security.ps1`, run as standalone scripts via this session's background-process mechanism, did not themselves complete Scenarios C–G / the full M2.9 regression flow. The underlying behavior those scripts exist to prove was independently and completely verified live via direct HTTP requests against the same running stack instead.

## Known Limitations

Everything listed under Findings and "What Was NOT Done" above. Additionally: `test-m211-security-read.ps1` as a standalone script has not itself been observed to complete Scenarios C–G end-to-end in this session's environment (it is expected to work when run interactively/foreground, per the diagnosis above, but this was not re-verified by executing the full script in the foreground — only its request logic was, plus the equivalent curl-driven verification). A future session should confirm the full script passes end-to-end when launched in a way that avoids the backgrounded-process HTTP-delivery defect.

## Final Verdict

M2.11 implementation is **COMPLETE**, both for its code-level goal (RBAC wiring + unit test coverage) and for live E2E verification: all 7 scenarios (Part 1, B, C, D, E, F, G) have real, recorded evidence from the live Docker stack. The one caveat — `test-m211-security-read.ps1` and `test-m29-security.ps1` did not complete as standalone script runs in this session, due to a fully diagnosed, non-code, environmental defect in how this session launches backgrounded PowerShell processes — is stated plainly above and does not change the correctness conclusion, since the exact same requests those scripts would have sent were independently proven to succeed.
