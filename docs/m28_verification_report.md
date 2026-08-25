# ATLAS M2.8 Verification Report — Chaos Engineering / Fault-Injection E2E Validation

Generated 2026-08-25 from actual command output and live `docker compose` runs. Nothing in this milestone has been committed — this report is for review before that decision.

## Summary

- ✅ M2.7 remains frozen: all fourteen protected paths (`internal/rca/`, `internal/propagation/`, `internal/incidentmanager/correlator.go`, `internal/incidentmanager/causal.go`, `internal/incidentdetector/`, `internal/remediation/`, `internal/execution/guard.go`, `internal/execution/model.go`, `internal/execution/engine.go`, `internal/execution/verification.go`, `internal/buffer/`, `internal/event/`, `internal/ingestion/`, `internal/infrastructure/docker/`) — `git diff --stat` empty, confirmed after implementation.
- ✅ `test-m27-docker.ps1` — zero diff, confirmed. M2.7's own regression suite was never touched.
- ✅ New, separate `test-m28-chaos.ps1` — the only file this milestone adds.
- ✅ `go build`, `go vet`, `go test ./...`, `go test -race ./...` — all clean, including the full M2.7 regression suite re-run unmodified.
- ✅ **All three chaos scenarios ran to completion, exercising the properties they were designed for, with results that are correct given the actual live evidence — not forced.**
- ⚠️ **A real, live-verified architectural limitation was found during implementation**: stopping a monitored service's container entirely (tested on `atlas-inventory-service-1`) cannot produce an incident attributed to that service, or any dependency-graph edge pointing at it, under the current, frozen M2.3 correlation mechanism. Documented in detail below, per instruction — not worked around with new tooling or application code.
- ⚠️ **Scenario 1 and Scenario 3 were revised during implementation** after their first live run showed the originally-chosen isolated fault-injection path (direct `:8086`, structurally capped at LOW confidence) never reached execution. Switched to the gateway-cascade injection path — the same existing `amount=8888.00` trigger, already proven reliable throughout M2.7.3/M2.7.4, just sent via a different existing entry point, not a new mechanism. Both scenarios then reached their intended target property on the corrected re-run.

## Investigation finding: container-stop cannot produce an attributable incident

Before writing Scenario 2, `atlas-inventory-service-1` was stopped live and real order traffic (`P100/quantity=1`) was sent through the gateway. Result:

- `GET /api/v1/incidents/open` showed incidents rootServiced at `atlas-order-service` and `atlas-gateway` only — **zero incidents rootServiced at `atlas-inventory-service`**, across every run of this investigation and every subsequent Scenario 2 run.
- `GET /api/v1/graph/edges` showed **zero edges** with `atlas-inventory-service` as source or target — only the pre-existing `atlas-gateway -> atlas-order-service` edge.
- Confirmed in the actual code, not just behavior: [`correlation.go:158`](services/intelligence-engine/internal/correlation/correlation.go#L158) — `graphBuilder.AddDependency(parent.ServiceName, child.ServiceName, ...)` is only called when both a parent (client) span and a matched child (server) span exist, linked via `ParentSpanID`. A fully-stopped service never receives the request, so it never emits its own child span — there is nothing to pair with the caller's client span, and no edge is ever constructed. **This is service-agnostic**: it would apply identically to gateway, order-service, or payment-service if any of them were fully stopped rather than erroring while still running.

This closes off container-stop as a mechanism for producing a service's *own* attributable incident anywhere in this architecture. Per instruction, Scenario 2 was designed around this finding rather than against it — see below.

## Scenario 1: Single-service payment failure

**Terminal state: EXPECTED TIMEOUT**

First attempt (isolated `:8086` injection) produced `confidence=LOW score=25` and a `SAFE BLOCK` (M2.6 correctly refused the plan) — a legitimate safety outcome, but one that never reached execution, leaving the scenario's actual target property (full plan → approve → execute → verify chain, independently verified restart) unexercised. Reasoned live that this matches the already-documented, structural characteristic of the isolated single-evidence-type path (M2.7.1's report; the same reason M2.7.4's own Scenario D was redesigned away from it). Switched the injection to the gateway-routed cascade path (product P200, quantity=4 — the same `amount=8888.00` trigger, existing entry point, no new mechanism).

Corrected re-run:
```
Baseline atlas-payment-service-1 StartedAt: 2026-08-25T18:22:16.381417021Z
Incident detected: d97b022a-... (service=atlas-payment-service confidence=MEDIUM score=45)
Execution Status: EXECUTED
atlas-payment-service-1 StartedAt after execution: 2026-08-25T18:22:43.629868907Z
Independent Docker restart proof: StartedAt changed from ...18:22:16... to ...18:22:43...
Final Verification Status: VERIFICATION_TIMEOUT
```
Correlation/causal attribution correctly narrowed the remediation target to `atlas-payment-service` alone (score 45 = own error-rate 25 + causal-attributed dependency-error 20, matching M2.7.3/M2.7.4's proven mechanism). Real Docker restart independently confirmed via `StartedAt` changing, read directly from `docker inspect`, not from the API's self-report. `VERIFICATION_TIMEOUT` is correct here: the fault-generating traffic stopped before execution, so the incident's own M2.4 recovery window simply hadn't elapsed within the verification budget — the same, already-understood, non-defective characteristic documented throughout M2.7.3/M2.7.4. Not a false result.

## Scenario 2: True independent concurrent failure

**Terminal state: PASS**

Fault A: `docker stop atlas-inventory-service-1` + normal gateway-routed traffic (`P100/quantity=1`, which fails at the inventory-reservation step before payment is ever called). Fault B: the isolated payment fault, sent directly to `:8086`, never touching gateway/order-service.

Observed, unforced result:
```
Payment incident:      correlationGroupId=9ec8ca8e-...  primary=True  related=(empty)   rca=atlas-payment-service/LOW/25
Order/gateway primary: correlationGroupId=426556f9-...  primary=True  related=(4 entries) rca=atlas-order-service/MEDIUM/45
Dependency graph:       atlas-gateway -> atlas-order-service only (no inventory edge)
```
The two fault chains received **different `correlationGroupId`s**, and neither's `relatedIncidentIds` referenced the other — the core safety assertion (two independent faults must not be falsely correlated into one causal root) holds. `atlas-inventory-service` never appeared as an RCA candidate or graph node, consistent with the investigation finding above — this is an honest report of what the architecture can and cannot attribute, not a claim that inventory was correctly identified as a root cause (it structurally cannot be, under the current design). Neither chain reached remediation execution this run (payment stayed LOW/25 via the isolated path by design in this scenario; order-service's MEDIUM/45 was observed but no plan-generation attempt was made against it, since Scenario 2's scope — per the approved investigation — is the correlation/attribution safety property, not a second full execution proof, which Scenarios 1 and 3 already provide).

## Scenario 3: Persistent fault during remediation

**Terminal state: EXPECTED FAILED**

Same isolated→cascade correction as Scenario 1 was needed and applied before this scenario could reach execution. Continuous fault-generating traffic (gateway-routed, `P200/quantity=4`, one request every 800ms) ran in a PowerShell background job (`Start-Job`) spanning before, during, and after the real plan → approve → execute chain — the chaos driver never called the Docker adapter or `execution.Engine` directly at any point; every remediation step went through the real HTTP endpoints exactly as Scenario 1.

```
Baseline atlas-payment-service-1 StartedAt: 2026-08-25T18:24:15.50879623Z
Incident detected: 3e7206e6-... (service=atlas-payment-service confidence=MEDIUM score=45)
Execution Status: EXECUTED
atlas-payment-service-1 StartedAt after execution: 2026-08-25T18:24:36.344743385Z
Independent Docker restart proof: StartedAt changed from ...18:24:15... to ...18:24:36...
Final Verification Status: FAILED
```
This is the most meaningful live proof M2.7.4's EventBuffer-based fix has received: the restart genuinely occurred (independently confirmed), but the underlying fault (unconditional application logic, unaffected by a container restart) kept producing real, freshly-timestamped `ERROR` events after `executionFinishedAt`. `Verifier.hasGenuinePostExecutionFailure` correctly detected this real evidence and returned `FAILED` — **and, critically, never returned `VERIFIED`**, which the script explicitly treats as a hard failure if it occurs (`Write-Error` on that specific outcome). No false positive occurred.

## Contamination / isolation evidence

Every scenario began with a full `docker-compose down` + `up` (confirmed in the raw output: each scenario's block shows the complete container teardown/recreation sequence). Each scenario expecting a restart captured its own fresh `StartedAt` baseline immediately after its own containers came up — no baseline was reused across scenarios. `EventBuffer`, incident state, dependency-graph edges, and execution-audit records were all discarded by the container recreation between scenarios; no scenario's traffic or state carried into the next. This matches the deterministic-reset recommendation from the M2.8 investigation (no reliance on wait-based decontamination).

## Safety property confirmation

- `APPROVED != EXECUTED`: every scenario that reached execution went through the real `/remediation/plan` → `/approve` → `/execute` HTTP sequence; the chaos script never called the Docker adapter or `execution.Engine` directly for remediation purposes.
- Scenario 2's `docker stop`/`docker start` of `atlas-inventory-service-1` is explicitly fault-injection, not remediation — confirmed distinct in the script's own comments and structure, and it is the one deliberate, instruction-permitted exception to "never directly restart a container via the chaos driver."
- Guard/policy/idempotency/allowlist: unexercised by any bypass — Scenarios 1 and 3 both hit the real M2.6 policy gate (observed both a `SAFE BLOCK` on the first Scenario 1/3 attempts and a real pass-through on the corrected runs), and the real `Guard.Check` on every execute call.
- RCA scoring, correlation, and causal attribution: unmodified, confirmed by protected-path diffs; scores observed (45, 25) exactly match the already-established, documented values from M2.7.3/M2.7.4, not new/different numbers.

## Test results

`go build ./...` — clean. `go vet ./...` — clean (native and Linux container). `go test ./...` — clean, all packages. `go test -race ./...` (Linux container, `golang:1.25`) — clean, including the full, unmodified M2.7 regression suite (`internal/execution`, `internal/incidentmanager`, `internal/rca`, etc. all pass).

## Confirmation: no unintended changes

```
git diff --stat -- internal/rca/                          (empty)
git diff --stat -- internal/propagation/                   (empty)
git diff --stat -- internal/incidentmanager/correlator.go  (empty)
git diff --stat -- internal/incidentmanager/causal.go      (empty)
git diff --stat -- internal/incidentdetector/               (empty)
git diff --stat -- internal/remediation/                    (empty)
git diff --stat -- internal/execution/guard.go               (empty)
git diff --stat -- internal/execution/model.go                (empty)
git diff --stat -- internal/execution/engine.go                (empty)
git diff --stat -- internal/execution/verification.go           (empty)
git diff --stat -- internal/buffer/                               (empty)
git diff --stat -- internal/event/                                 (empty)
git diff --stat -- internal/ingestion/                               (empty)
git diff --stat -- internal/infrastructure/docker/                    (empty)
git diff --stat -- test-m27-docker.ps1                                  (empty)
```
Full diff: exactly one new file, `test-m28-chaos.ps1`. No Go source, no Java source, no `docker-compose.yml` change.

## What was NOT done

- No new fault-injection infrastructure (no network/latency tooling, no resource-exhaustion tooling, no new application fault switches) — confirmed by the investigation and honored throughout implementation.
- Inventory-service's inability to produce an attributable incident was not worked around with new code — reported, and Scenario 2 was redesigned around genuinely-available evidence instead.
- No M2.7/M2.4/M2.5/M2.6 production logic was touched.
- M2.9 or any further milestone was not started.

## Commit status
**Nothing has been committed.** Awaiting your review of this report and the diff.
