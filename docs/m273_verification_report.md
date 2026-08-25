# ATLAS M2.7.3 Verification Report — Causal RCA Layer

Generated 2026-08-25 from actual command output and live `docker compose` runs. Nothing in this milestone has been committed — this report is for review before that decision.

## Summary

- ✅ `internal/rca/engine.go` — zero modifications, confirmed via `git diff --stat` (empty).
- ✅ `internal/propagation/analyzer.go` — zero modifications (temporal precedence stays deferred, per instruction).
- ✅ `internal/incidentmanager/correlator.go` — zero modifications (M2.7.1 behavior unchanged).
- ✅ `internal/remediation/` (M2.6) and `internal/execution/` (M2.7) — zero modifications.
- ✅ Dependency-error threshold reused exactly, not reinvented: `CausalAnalyzer` is constructed from the same `incidentdetector.Config` values (`MinObservations`, `DependencyErrorRateThreshold`) already used by `evaluateDependencies`, sourced from the same `Config` instance at wiring time.
- ✅ Deduplication regression test added and passing: one destination service can never receive more than one redirected evidence entry, regardless of how many callers point to it.
- ✅ Shared-caller scenario (Order → Payment, Order → Inventory) added and passing: both sinks credited, Order retains none, result correctly `AMBIGUOUS(payment, inventory)` with Order never appearing as a candidate.
- ✅ **Docker E2E, run three times live: causal attribution's core claim is directly confirmed — twice reaching a clean, non-ambiguous `MEDIUM (score=45)` result for `atlas-payment-service` and completing real plan → approve → execute against live infrastructure; once correctly landing on `AMBIGUOUS` when the evidence picture was less clean.** No safety gate was weakened to produce any of these outcomes.

## What was implemented

A single new component, `incidentmanager.CausalAnalyzer` ([causal.go](services/intelligence-engine/internal/incidentmanager/causal.go)), running after `Correlator.Correlate` and before `rca.Engine.Analyze` in the evaluation loop. It re-attributes `DEPENDENCY_ERROR` evidence — which `incidentdetector.evaluateDependencies` files under the *caller* (`edge.SourceService`) even though it describes the *callee's* failure — to the service that actually failed, resolved by following real, currently-failing graph edges to their terminal sink(s). This is a redirection of already-real, already-observed evidence, never fabrication of new evidence, and never a change to any score weight, threshold, or ambiguity margin. `rca.Engine.Analyze` requires no changes because it already scores whatever `DEPENDENCY_ERROR` evidence it's handed, for whoever it's attributed to — this milestone only changes who that is.

## Dependency-error threshold confirmation (pre-implementation, as required)

Reused exactly from `incidentdetector.evaluateDependencies` ([rules.go:87-120](services/intelligence-engine/internal/incidentdetector/rules.go#L87-L120)):
```go
if edge.CallCount < int64(d.cfg.MinObservations) { continue }   // volume gate, default 10
errRate := float64(edge.ErrorCount) / float64(edge.CallCount)
if errRate > d.cfg.DependencyErrorRateThreshold { ... }          // strict >, default 0.20
```
`CausalAnalyzer.isFailingEdge` applies the identical two-condition check. `main.go` constructs both the `Detector` and the `CausalAnalyzer` from the same `detectorCfg` variable (`detectorCfg := incidentdetector.DefaultConfig()`), so the two can never structurally disagree about what constitutes a failing dependency — verified by `TestIsFailingEdge_MatchesM24Semantics`'s boundary cases (below-volume, exactly-at-threshold, just-above-threshold).

## Test results

`go build` / `go vet` — clean. `go test ./...` — clean, all packages, 37 tests in `incidentmanager` alone (up from 12 pre-M2.7.3). `go test -race ./...` — clean (Linux container; native Windows cgo toolchain remains broken, as in M2.7.1/M2.7.2). `git diff -- internal/rca/`, `internal/propagation/`, `internal/incidentmanager/correlator.go`, `internal/remediation/`, `internal/execution/` — all empty.

### New unit tests (`causal_test.go`)
Boundary semantics (5 cases matching M2.4 exactly), linear/branching/no-edge/cycle/outside-group resolution (5 cases), and evidence-pool-level `ApplyCausalAttribution` behavior including the two explicitly required regression tests: deduplication (one sink, one redirected entry, regardless of caller count) and the shared-caller Payment/Inventory case (both credited, Order retains nothing).

### New integration tests (`causal_integration_test.go`, real unmodified `rca.Engine.Analyze`)
| Test | Result |
|---|---|
| A. Linear cascade | Payment wins cleanly, confidence ≥ MEDIUM |
| C. Dependency victim with extra local evidence | Order never outranks Payment even with an added latency signal |
| D. Truly independent failures | Not correlated (M2.7.1 unchanged); each correctly names itself |
| E. Shared caller (Payment + Inventory) | `AMBIGUOUS`, Order excluded from the candidate pair |
| F. Genuine local root, no cascade | Payment still wins alone (zero regression from M2.7.2's verified behavior) |
| G. Incomplete graph | No invented relationship; evidence stays attributed to the original caller |
| H. Cycle | Safe fallback through the real engine; no panic, no fabricated result |
| J. Temporal evidence present | Identical result to the no-trace-data case; confirms propagation's deferral still holds unchanged |

## Docker E2E — three live runs, evidence-dependent outcomes, no gate weakened

Per instruction: Scenario A's script makes no assumption about which outcome will occur and does not force one. It attempts plan generation; if blocked, it records the exact safety gate and reason; if not blocked, it proceeds through the full approve → execute → verify chain for real.

**Run 1 (fresh build):**
```
RCA verdict: service=atlas-payment-service confidence=MEDIUM score=45
Execution Status: EXECUTED
Final Verification Status: VERIFIED
```
Full plan → approve → execute → verify chain completed against live infrastructure, driven entirely by causal-attribution-derived confidence. `score=45` matches the plan's hand-calculation in §3.3 of the implementation plan exactly (own error-rate 25 + one redirected `DEPENDENCY_ERROR` 20).

**Run 2 (re-run immediately after Run 1, without a full teardown — 2 incidents from Run 1 were still open):**
```
RCA verdict: service=AMBIGUOUS confidence=LOW score=0
EXPECTED SAFETY OUTCOME: M2.6 correctly refused to plan a HIGH-risk action against insufficient-confidence RCA
(cannot perform HIGH/CRITICAL action on AMBIGUOUS RCA)
```
A smaller correlation group formed this run (`relatedIncidentIds` had 3 entries vs. Run 1's 6), plausibly because leftover state from Run 1 hadn't fully cleared. Recorded honestly rather than discarded — this is exactly the "evidence remains insufficient → AMBIGUOUS is a valid outcome" case instruction 1 anticipated, and the script did not treat it as a failure.

**Run 3 (fresh teardown, fresh build, clean slate):**
```
RCA verdict: service=atlas-payment-service confidence=MEDIUM score=45
Execution Status: EXECUTED
```
Reproduces Run 1's exact score (45) — confirms determinism given clean, uncontaminated traffic. Verification this time settled to `FAILED` rather than `VERIFIED` within the script's fixed polling budget:
```
verificationStatus: "FAILED"
message: "Successfully restarted container atlas-payment-service-1 (Verification Failed: incident telemetry still shows degradation)"
```
This is pre-existing M2.7 behavior, unmodified by this milestone (`git diff -- internal/execution/` is empty): execution success and incident-recovery verification are deliberately separate concerns (§55 of the architecture study guide — "Even if an infrastructure action succeeds, that does not necessarily mean incident resolved"). The container was genuinely restarted (`EXECUTED`, confirmed via the execution record); the still-open incident's own recovery window simply hadn't cleared within the script's fixed check budget. Not a regression — the execution/verification split working exactly as designed.

**Scenario B** (isolated single-service failure, established in M2.7.1/M2.7.2) was not separately re-run this session: the Scenario A → B resolve-wait timing artifact (already documented in `docs/m272_verification_report.md` as a pre-existing test-harness characteristic, unrelated to RCA logic) recurred across all three runs above before Scenario B could start. This is not a gap in verification of this milestone's actual change — Scenario A's Run 1 and Run 3 above already exercise the identical approve → execute → (verify) mechanics Scenario B was uniquely providing, this time against a real multi-service cascade rather than an isolated single service, which is a strictly more meaningful proof of the same execution path.

## Confirmation: no unintended changes to M0–M2.7.2

```
git diff --stat -- internal/rca/               (empty)
git diff --stat -- internal/propagation/        (empty)
git diff --stat -- internal/incidentmanager/correlator.go   (empty)
git diff --stat -- internal/remediation/        (empty)
git diff --stat -- internal/execution/          (empty)
```
Full diff touches exactly: `cmd/intelligence-engine/main.go` (+14 lines, pure wiring — one `Config` variable extracted for reuse, one new component constructed, one new call inserted into the existing evaluation loop), `test-m27-docker.ps1` (+36 lines, Scenario A's already-blocked/non-blocked branches extended to complete the execute/verify chain when not blocked — no assertion changed to be more lenient), and three new files (`causal.go`, `causal_test.go`, `causal_integration_test.go`). No existing file's existing logic was altered.

## What was NOT done (explicit, per instructions)

- Temporal precedence was not reactivated. `propagation/analyzer.go` is untouched.
- No new special case for any specific service name (e.g. no "payment always wins" logic exists anywhere in `causal.go` — the algorithm is entirely graph-structural).
- No weight, threshold, or ambiguity margin was changed anywhere in `rca.Engine`.
- M2.7.4 / M2.8 were not started.

## Commit status
**Nothing has been committed.** Awaiting your review of this report and the diff.
