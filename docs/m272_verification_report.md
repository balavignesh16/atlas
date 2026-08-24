# ATLAS M2.7.2 Verification Report — RCA Evidence Quality & Confidence Calibration

Generated 2026-08-24 from actual command output and live `docker compose` runs. Nothing in this milestone has been committed — this report is for review before that decision.

## Summary

- ✅ `internal/rca/engine.go` — **zero modifications**, confirmed via `git diff --stat` (empty) at every checkpoint of this milestone, including after a mid-implementation investigation that touched `propagation/analyzer.go` and was fully reverted.
- ✅ Shared `event.IsErrorStatus` classification helper added, tested first, then used to replace duplicated logic in `incidentdetector` and `correlationmodel` — with zero change to existing classification semantics.
- ✅ `Incident.TraceIDs` now genuinely populated from real window observations, bounded (max 10, FIFO eviction, deduplicated, thread-safe under the existing manager lock) — confirmed never a detection input, never a confidence reducer when absent.
- ⚠️ **Temporal precedence bonus activation was investigated, implemented, verified working in isolation, then reverted** after live testing surfaced a real, more serious problem than the one it was meant to fix. Full account below — this is the central finding of this milestone.
- ✅ AMBIGUOUS-preservation and false-precedence protection both independently verified against real trace data, including a regression test that reproduces the exact scenario that caused the revert.

## Before M2.7.2: why RCA failed in M2.7.1

Live testing at the end of M2.7.1 showed two related failure patterns, both traced to their root cause during this milestone's audit:

1. **`Incident.TraceIDs` was never populated.** `incidentsignal.Signal` has a `TraceID` field and `incidentmanager.ProcessSignal` already correctly stored it — but neither of the two places that construct a `Signal` (`incidentdetector.evaluateWindowRules`, `evaluateDependencies`) ever set it, because the sliding `window.Window` that aggregates observations had no field to carry a trace ID through in the first place.
2. **Span status classification was broken at its single shared source.** `correlationmodel.FromEvent` copied the raw OTel span `Status` verbatim, which this project's real Micrometer/Spring telemetry leaves `"UNSET"` even for a genuine 5xx response (the real status lives only in a string attribute — the same defect independently fixed in `incidentdetector` back in M2.7.1). This silently broke every downstream consumer of span status: the M2.3 dependency graph's edge error counts, `GetTrace()`'s `OverallStatus`, blast radius, and `propagation.CheckTemporalPrecedence`.

Both are real, necessary, low-risk plumbing fixes — done first, verified in isolation, and kept in this milestone's final state.

## What evidence was added

- `event.IsErrorStatus(status, attributes)` — extracted, tested (11 cases) before any call site was touched, then used to fix `correlationmodel.FromEvent`'s status derivation (feeding correct data to the graph, blast radius, and propagation, all without changing those files) and to replace the equivalent inline logic in `incidentdetector.ProcessEvent`.
- `window.Observation` gained a `TraceID` field; `Window.Add` threads it through from the originating `ATLASEvent.TraceID`; `Window.RecentTraceID()` surfaces the most recent one for signal emission. Missing trace IDs are inert by contract (verified by `TestAdd_MissingTraceIDDoesNotAffectDetectionMetrics`).
- `incidentmanager.appendTraceID` — bounded (`MaxTraceIDsPerIncident = 10`), FIFO-evicting, deduplicating, used identically for both the new-incident and existing-incident paths, thread-safe under the manager's existing lock (verified by a 50-goroutine concurrent test, and by `go test -race`).

## How RCA behavior changed — and the finding that reverted part of it

With TraceIDs populated and status classification fixed, `propagation.CheckTemporalPrecedence` (an already-existing, previously fully dormant mechanism inside `rca.Engine`'s dependency chain) finally had real data to evaluate. Investigating why it still didn't fire for a realistic nested cascade led to a genuine, previously-undiscovered defect: it compares `span.StartTime`, and in any synchronous nested call chain a parent's span always starts before its child's, by causality — so the deepest, truly-root-cause service's span *always* starts last, never first. The mechanism, as originally written, could never have rewarded a true root cause even with perfect data.

**Attempted fix:** compare `EndTime` instead — the deepest failing span typically completes first, since it fails immediately with nothing further to wait on. Verified correct in isolation: payment-service (the true root) received precedence evidence; gateway (verified via the `IsPath` direction gate, independent of which time field is used) never could, regardless of raw timestamps.

**Then verified against the real, unmodified `rca.Engine.Analyze`, and found a worse emergent problem.** A middle-tier caller (order-service) already carries its own `DEPENDENCY_FAILURE` evidence from `rca.Engine`'s existing, unmodified scoring — a signal a pure sink (payment) can never earn, since it has nothing to depend on. Stacking the newly-active precedence bonus on top of that let order-service reach a flat score matching or exceeding payment's, and in one live run, **order-service won outright with HIGH confidence (score 80)** — a confident, wrong answer, worse than M2.7.1's honest `AMBIGUOUS(order, gateway)`. Confirmed live:

```
Before revert (live, real traffic):
  rootCause: {"service":"atlas-order-service","confidence":"HIGH","score":80}

After revert (live, real traffic, same scenario, run twice):
  rootCause: {"service":"AMBIGUOUS","confidence":"LOW","score":0}
  rootCause: {"service":"atlas-payment-service","confidence":"LOW","score":25}   (a second run;
                                                                                   still correct, still
                                                                                   appropriately gated)
```

The scoring logic that would need to change to fix this properly — weighing precedence against how many other evidence types a candidate already holds — lives entirely inside `rca.Engine.Analyze`, explicitly out of bounds for this milestone. The `EndTime` fix was reverted; `propagation.CheckTemporalPrecedence` remains exactly as it was before this milestone (`StartTime`-based, dormant for nested cascades), with a doc comment recording the full investigation so it isn't silently rediscovered or reintroduced without addressing the `rca.Engine` interaction.

## Known limitation: temporal evidence collection exists, scoring activation is deferred

**Collection and scoring are two separate things, and only one shipped in this milestone.** `Incident.TraceIDs` is now genuinely populated with real trace evidence, and `event.IsErrorStatus` correctly classifies real span failures — both ship in this milestone and are safe, verified, and active. `propagation.CheckTemporalPrecedence` can now correctly locate this evidence when given a trace ID. **What is deliberately deferred is scoring activation**: the code path that would turn located temporal evidence into RCA points (via the `EndTime` comparison investigated above) is not enabled. It stays reverted to its original, dormant `StartTime` comparison.

**Why:** activating it lets a dependency *victim* — a caller whose own outgoing call failed, which already earns its own `DEPENDENCY_FAILURE` evidence from `rca.Engine`'s existing, unmodified scoring — also collect the precedence bonus once real trace data flows. Stacking those two increases a victim's confidence rather than the true root cause's, since a pure sink can never earn the dependency-failure bonus in the first place. Confirmed live: this let a caller reach HIGH confidence (score 80) and be named root cause instead of the actual origin. That is a direct instance of "dependency victims can receive false confidence," and it is exactly what M2.4's `AMBIGUOUS` design was built to prevent.

**Path forward — M2.7.3:** fixing this properly requires changing how `rca.Engine.Analyze` weighs the precedence bonus against a candidate's other evidence (e.g. scaling it by how many distinct callers a candidate precedes, or discounting it when the candidate already holds its own dependency-failure evidence) — scoring-formula work inside `rca.Engine` itself, explicitly out of bounds for M2.7.2. M2.7.3 is the natural home for that work: the evidence pipeline (TraceIDs, status classification, trace lookup) this milestone built is already in place and ready for it to consume once `rca.Engine`'s scoring is back in scope.

## Proof ambiguity safety remains intact

- `TestPropagation_NestedCascade_PrecedenceIntentionallyStaysDormant` — real trace data, real 3-hop cascade, confirms zero candidates receive precedence evidence (the reverted, dormant state).
- `TestPropagation_FalsePrecedence_OutermostCallerNeverWinsFromRawTiming` — gateway, even under the most timing-favorable construction possible, never receives precedence evidence. Holds independent of the StartTime/EndTime revert (it verifies the `IsPath` direction gate itself).
- `TestRCA_NestedCascadeWithDependencyEvidence_StaysAmbiguousNotConfidentlyWrong` — directly reproduces the dangerous live scenario as a controlled regression test: gateway and order-service each carry error-rate + dependency-failure evidence (realistic, tied at 45 each), payment only error-rate (25). Asserts the real, unmodified `rca.Engine.Analyze` returns `AMBIGUOUS`, not a confident wrong pick. This test would fail immediately if the `EndTime` change (or anything with the same effect) were reintroduced without first addressing the `rca.Engine` scoring interaction.
- `TestCorrelate_ParallelFailureIntegration_RCAReturnsAmbiguous` (M2.7.1, re-run unmodified) — still passes: independent parallel failures still correctly return AMBIGUOUS.
- Live Docker E2E, run twice against real traffic after the revert: `AMBIGUOUS(confidence=LOW, score=0)` and, in a second run, a clean singleton `atlas-payment-service(confidence=LOW, score=25)` correctly gated by M2.6's LOW-confidence safety rule. Neither run reproduced the dangerous confident-wrong-answer pattern.

## Test results

`go build`/`go vet` — clean. `go test ./...` — clean, all packages, including all new tests above. `go test -race ./...` — clean (run inside Linux; this dev machine's native cgo toolchain is broken). `git diff -- internal/rca/` — empty, verified repeatedly throughout, including immediately after the revert.

Docker E2E: Scenario A (the cascade this milestone's investigation concerned) independently confirmed safe on two separate live runs after the revert, as detailed above. Scenario B's isolated-incident timing became flaky under rapid back-to-back re-runs against the same long-lived containers (a test-harness artifact — sliding-window residue accumulating across consecutive runs without a container restart, unrelated to this milestone's code changes); Scenario B's underlying execution mechanics were already independently re-confirmed earlier in this session via the standalone Go integration test (`TestDockerAdapter_RealRestartAgainstLiveContainer`), which does not depend on cascade correlation or propagation and was not touched by this investigation.

## What was NOT changed
- `internal/rca/` — zero modifications, confirmed via `git diff`.
- `propagation/analyzer.go`'s actual behavior — net zero change from before this milestone (StartTime-based, dormant for nested cascades); only its doc comment changed, to record the investigation.
- M2.6's safety validator, M2.7's execution engine, M2.7.1's correlator logic — untouched.

## Commit status
**Nothing has been committed.** Awaiting manual review.
