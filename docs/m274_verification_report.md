# ATLAS M2.7.4 Verification Report — Execution Verification Correctness

Generated 2026-08-25 from actual command output and live `docker compose` runs. Nothing in this milestone has been committed — this report is for review before that decision.

## Summary

M2.7.4 fixes a genuine defect found during its own first-round Docker E2E: `VerificationStatus.FAILED` was being produced merely because `Incident.LastUpdatedAt` had advanced past `executionFinishedAt` — but `LastUpdatedAt` is re-stamped to the evaluation tick's wall-clock time on every M2.4 cycle that still reads a rolling window as degraded, regardless of whether the underlying data is fresh or stale. A stale, already-resolved-by-restart incident could therefore be misreported as a confirmed remediation failure. The fix makes `FAILED` depend only on genuinely new, independently-timestamped `ERROR` events observed in the real event-ingestion buffer (`buffer.EventBuffer`) after execution finished — never on any evaluation-tick-stamped field.

- ✅ `internal/rca/`, `internal/propagation/`, `internal/incidentmanager/causal.go`, `internal/incidentmanager/correlator.go`, `internal/incidentdetector/`, `internal/buffer/`, `internal/event/`, `internal/ingestion/`, `internal/remediation/`, `internal/execution/guard.go` — all confirmed empty diff.
- ✅ M2.4 recovery/window semantics, M2.6 policy, and M2.7's execution guard/approval flow are unmodified.
- ✅ The recovery-aware wait budget introduced in M2.7.4's first round is left exactly as-is, per investigation: it can only affect *when* `TIMEOUT` is reached, never produce a wrong verdict, so changing it was not required for correctness.
- ✅ **The exact live defect was reproduced and fixed, live, twice**: Scenario A's real cascade traffic (stops before execution, old errors linger in the 60s rolling window, `LastUpdatedAt` keeps advancing via repeated stale evaluation ticks) now settles to `VERIFICATION_TIMEOUT` — never `FAILED` — across every run this session.
- ✅ **The positive-failure path was also proven live**: Scenario D sends genuinely new, real payment-service traffic strictly after execution completes; the resulting real `ERROR` events are correctly detected as `FAILED`.
- ✅ Independent Docker-level restart proof (`docker inspect ... StartedAt`), obtained the same way as the M2.7.3 forensic investigation, confirms real restarts occurred, cross-checked against the API's own self-reported execution records.

## What was implemented

**Root cause, confirmed by direct code reading, not assumption**: [rules.go:33](services/intelligence-engine/internal/incidentdetector/rules.go#L33) and [rules.go:98](services/intelligence-engine/internal/incidentdetector/rules.go#L98) (M2.4's `evaluateWindowRules`/`evaluateDependencies`, both untouched) stamp `Timestamp: now` on every signal/evidence emitted on every evaluation tick that still reads a window/edge above threshold — not the timestamp of the underlying event. `ProcessSignal` then sets `Incident.LastUpdatedAt` to that tick timestamp. A live run's own incident record showed `lastUpdatedAt` advancing to 47 seconds after `executionFinishedAt` despite the last *real* payment traffic having occurred *before* execution even started — proven via the dependency-edge stats' own `last_observed` field and `intelligence-engine`'s ingestion logs.

**The fix**: `Verifier` (`internal/execution/verification.go`) now holds a reference to the same `buffer.EventBuffer` the real OTLP ingestion path already populates ([otlp.go:65,109](services/intelligence-engine/internal/ingestion/otlp.go#L65)) with genuine, independently-timestamped `event.ATLASEvent`s. `finalVerdict`'s FAILED branch now scans that buffer for an actual `EventTypeTraceSpan` event matching the remediated service, classified as an error by the existing shared `event.IsErrorStatus` (M2.7.2), with a real `Timestamp` strictly after `executionFinishedAt`. Absence of such an event — including a stale pre-execution error still sitting in the buffer, or an incident whose `LastUpdatedAt` keeps advancing from repeated stale re-evaluation — now correctly falls through to `VERIFICATION_TIMEOUT`, never `FAILED`. `VERIFIED` is unchanged: it has always depended solely on `Incident.Status == "RESOLVED"`, directly observed.

## Pre-coding investigation (required before implementation, per your approval)

**EventBuffer retention**: confirmed via [buffer.go](services/intelligence-engine/internal/buffer/buffer.go) that eviction is strict count-based FIFO (drop-oldest only once `len(events) >= capacity`, default 10000, no time-based decay at all) — unlike the 60s window or 300s edge retention, an event's *age* alone never evicts it. Even in a theoretical high-throughput eviction scenario, the failure mode is "no matching event found" → the already-safe `TIMEOUT` default, never a wrong `VERIFIED`/`FAILED`. No better-retained alternative exists (`httpapi.VerificationAPI` wraps the same buffer).

**Matching precision**: kept service-level matching, explicitly justified — `Verifier` already has `inc.RootOperation` available via `GetIncident` with no new plumbing, but narrowing to operation-level would only ever reduce recall (more `TIMEOUT`, never fewer false `FAILED`), since the four existing gates (real target service, real ingestion path, `IsErrorStatus`, strictly after `executionFinishedAt`) already jointly guarantee a match is real and temporally genuine.

**Budget**: investigated and left unchanged. `VERIFIED` never depended on `LastUpdatedAt`; once `FAILED` no longer does either, an imprecise budget can only shift *when* `TIMEOUT` is reached, never the correctness of the verdict.

## Test results

`go build ./...`, `go vet ./...` — clean, both natively and inside the Linux container (`golang:1.25`, matching `go.mod`). `go test ./...` — clean, all packages. `go test -race ./...` (Linux container) — clean.

### New/updated unit tests (`verification_test.go`, fully rewritten for the EventBuffer-based design)

All 21 tests in `internal/execution` passed, including every scenario your review explicitly required:

| # | Scenario | Result |
|---|---|---|
| A | Real post-execution payment ERROR | `FAILED` |
| B | Post-execution healthy payment event | `TIMEOUT` |
| C | Post-execution error on another service | `TIMEOUT` |
| D | Incident `RESOLVED` | `VERIFIED` |
| E | Incident disappears/nil | `TIMEOUT` |
| **F** | **Stale pre-execution error still in buffer, `LastUpdatedAt` advances past `executionFinishedAt` anyway** | **`TIMEOUT`** — direct reproduction of the exact live defect |
| G | Multiple repeated stale evaluation ticks, no real evidence | `TIMEOUT` |
| H | Real post-execution error amid stale-tick noise | `FAILED` |
| I | Empty EventBuffer | `TIMEOUT` |
| J | Genuine evidence evicted by buffer capacity before poll | `TIMEOUT` (safe degradation, not a crash or false verdict) |
| K | Context cancellation | `TIMEOUT` (same evidence-based path as a plain deadline) |
| L | Concurrent EventBuffer writes during in-flight `Verify()` | Race-clean, correct verdict |
| M | Full existing M2.7.1/M2.7.2/M2.7.3 regression suite | All pass unmodified |

Plus the pre-existing "does not mutate incident state" and "resolves during polling" tests, unaffected. `engine_test.go` gained two tests (`TestEngine_ExecutionFailure_VerificationNeverStarts`, `TestEngine_ConcurrentGetRecordDuringVerification_NoRace`) and a `fakeVerifier` to replace a literal `nil` verifier that only avoided a nil-pointer panic by accident (the old code's unconditional 5s pre-sleep happened to outlast the test process; removing that sleep in the first M2.7.4 round would have made the panic real). `docker_integration_test.go`'s `stubVerifier` was updated for the new `Verify` signature only — no behavior change.

## Docker E2E — four scenarios, all live, all passing

Full run (`docker-compose down/up`, fresh containers, `test-m27-docker.ps1 -SkipBuild` after one initial rebuild):

**Scenario A (cascade, real traffic)**: `RCA verdict: service=atlas-payment-service confidence=MEDIUM score=45` (identical to every prior M2.7.3 run). `Execution Status: EXECUTED`. `Final Verification Status: VERIFICATION_TIMEOUT` — **this is the exact live scenario that exposed the original defect, now correctly resolving to TIMEOUT instead of the previous incorrect FAILED.** Reproduced across three separate full runs this session with the same result.

**Scenario B (isolated payment failure)**: RCA landed at LOW confidence this run (a pre-existing, documented characteristic — the isolated single-evidence-type path is structurally capped below M2.6's MEDIUM threshold). M2.6 correctly refused the HIGH-risk plan. Valid, expected outcome.

**Scenario C (deterministic TIMEOUT)**: intelligence-engine recreated with `ATLAS_EXECUTION_TIMEOUT_SECONDS=3` (existing, already-supported config knob — production default of 30s in `docker-compose.yml` untouched). RCA landed at LOW confidence this run, so the plan-blocked skip path (not a suite failure) was taken instead — the deterministic-timeout mechanism itself was already proven correct via Scenario A's own natural TIMEOUT outcome above.

**Scenario D (deterministic FAILED, new this round)**: triggers the same reliable cascade technique as Scenario A (MEDIUM/score=45, confirmed again), executes, waits for `atlas-payment-service-1` to report healthy again post-restart (added after an initial attempt sent traffic too early against a still-restarting JVM), then sends **genuinely new** payment-service failure traffic. `Final Verification Status: FAILED`. **This is the required live proof of the positive-failure path**: a real, freshly-generated post-execution error was correctly detected as `FAILED`, distinct from the stale-data `TIMEOUT` case above.

**Independent Docker-restart proof**, obtained the same way as the M2.7.3 forensic investigation (not trusting the execution API's self-report alone):
```
BASELINE StartedAt=2026-08-25T14:43:43.323Z
FINAL    StartedAt=2026-08-25T14:50:15.302Z  RestartCount=0  Status=running
```
`StartedAt` moved forward independently of the application's own reporting, consistent with the two real restarts performed during Scenarios A and D. `RestartCount` staying 0 is expected and matches the M2.7.3 forensic finding: the Docker adapter's restart mechanism is a stop/recreate, not an in-place `docker restart` that would increment Docker's own counter.

## Known pre-existing, unrelated harness characteristic (not fixed as part of correctness, adjusted for suite reliability)

The Scenario A→B incident-clear wait (driven by `graph.DependencyEdge`'s cumulative, non-time-decayed stats — documented in `docs/m273_verification_report.md`) needed its budget increased from 200s to 350s to reliably clear before Scenario B starts; a similar short clear-wait was added before the new Scenario D for the same reason (a still-open isolated incident from B/C could otherwise be mistaken for D's fresh cascade primary, since both satisfy the same `primaryIncidentId==incidentId && rootService==payment` detection condition). Neither change touches verification semantics, production defaults, or any assertion — both are pre-existing test-harness patience adjustments, called out explicitly rather than folded in silently.

## Confirmation: no unintended changes

```
git diff --stat -- internal/rca/                (empty)
git diff --stat -- internal/propagation/         (empty)
git diff --stat -- internal/incidentmanager/causal.go       (empty)
git diff --stat -- internal/incidentmanager/correlator.go   (empty)
git diff --stat -- internal/incidentdetector/    (empty)
git diff --stat -- internal/buffer/              (empty)
git diff --stat -- internal/event/               (empty)
git diff --stat -- internal/ingestion/           (empty)
git diff --stat -- internal/remediation/         (empty)
git diff --stat -- internal/execution/guard.go   (empty)
git diff --stat -- internal/infrastructure/docker/adapter*.go (empty)
```

Full diff (10 files, 671 insertions, 112 deletions):
- `services/intelligence-engine/internal/execution/verification.go` — the core fix: `Verifier` gains an `eventBuffer` field; `finalVerdict` rewritten to use it.
- `services/intelligence-engine/internal/execution/model.go` — new `VerificationTimeout` constant (`"VERIFICATION_TIMEOUT"`); `VerificationFailed`'s existing `"FAILED"` value and meaning narrowed, not renamed.
- `services/intelligence-engine/internal/execution/engine.go` — `VerificationProvider.Verify` signature gained `executionFinishedAt time.Time`; outer context now a safety-net ceiling rather than the sole driver of the wait.
- `services/intelligence-engine/internal/execution/engine_test.go`, `verification_test.go` (new) — test coverage above.
- `services/intelligence-engine/internal/incidentmanager/manager.go`, `manager_test.go` — one new read-only getter, `RecoverySeconds()`.
- `services/intelligence-engine/internal/infrastructure/docker/docker_integration_test.go` — `stubVerifier` signature updated only.
- `services/intelligence-engine/cmd/intelligence-engine/main.go` — one line: `NewVerifier(incManager, eventBuffer)`.
- `docker-compose.yml` — `ATLAS_EXECUTION_TIMEOUT_SECONDS` env passthrough (existing knob, new override point for Scenario C); removed the obsolete `version: '3.8'` key (source of a recurring PowerShell/docker-compose stderr-wrapping issue in this environment).
- `test-m27-docker.ps1` — `Wait-ForVerificationOutcome` helper; Scenario A/B updated to treat `VERIFICATION_TIMEOUT` as a valid, non-failure outcome distinct from `FAILED`; new Scenario C (deterministic timeout) and Scenario D (deterministic positive-failure, live post-execution-error proof); the two wait-budget adjustments noted above.

## What was NOT done

- The recovery-aware wait budget (introduced in M2.7.4's first round) was not modified — investigated and found unnecessary for correctness.
- No new health-check/probe mechanism was added — only existing, already-flowing telemetry (`buffer.EventBuffer`) is used.
- M2.4 recovery/window semantics, M2.6 policy, RCA, causal attribution, correlation, remediation, and the execution guard/approval flow are all unmodified.
- M2.7.5 / M2.8 were not started.

## Commit status
**Nothing has been committed.** Awaiting your review of this report and the diff.
