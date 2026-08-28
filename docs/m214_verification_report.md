# M2.14 Verification Report

## Scope

Fix the confirmed data-integrity defect in `internal/blast/blast.go` (`Calculate()` computed `failureCount` locally but never stored it, and never set `TraceCount` at all, leaving both fields permanently `0` in the live incident API despite real failing traces). Add direct package-level test coverage for `internal/blast/` (previously zero) and `internal/propagation/` (previously zero, production logic untouched — frozen).

## Investigation (Before Editing)

- Read `blast.go` completely: confirmed the exact defect — `failureCount` incremented locally, `Incident.TraceCount`/`.FailureCount` never assigned; the file's own trailing comments documented the author's unresolved uncertainty about this.
- Read `Incident` model: `TraceCount`/`FailureCount` are plain `int` fields, JSON-exposed (`traceCount`/`failureCount`).
- Found every caller of `Calculate()`: exactly one, `main.go`'s background evaluation loop, called on every open incident before RCA runs; the same incident pointer is later persisted via `incManager.UpdateIncident(inc)` later in the same loop iteration — confirming the fix will be visible through the live incident API once fixed.
- Found every consumer of `Incident.TraceCount`/`.FailureCount`: **none** (`grep -r "FailureCount\|TraceCount"` across the *entire* repository — all `.go` and `.java` files, not just `intelligence-engine` — returns matches only in `blast.go`, `blast_test.go`, and the `incidentmodel.Incident` field declarations themselves). Confirms this is a pure data-integrity/observability defect — no RCA, remediation, or execution logic reads these fields, so fixing them carries zero decision-logic regression risk.
- Confirmed intended semantics from the code's own structure: `TraceCount` should count trace IDs the calculation actually resolved via `corrEngine.GetTrace` (not raw `len(inc.TraceIDs)`, since a `TraceID` can be present but no longer resolvable), and `FailureCount` should count how many of those resolved traces have `OverallStatus == "ERROR"` — exactly what the existing (already-written, already-incrementing) `failureCount` local variable already computed; no new semantics were invented.

## Exact Production Change

`services/intelligence-engine/internal/blast/blast.go` only (+9/−5 lines). Added a `traceCount` local, incremented immediately after a trace ID successfully resolves via `GetTrace`; at the end of `Calculate`, added `inc.TraceCount = traceCount` and `inc.FailureCount = failureCount`. Removed the stale, now-resolved comment block documenting the original implementation's incompleteness. No other function, signature, field, or file touched. The blast-radius algorithm itself (service/operation/edge collection from ERROR spans) is completely unchanged.

## Tests Added

**`internal/blast/blast_test.go`** (new, 10 tests): nil incident, no `TraceIDs` (zero values preserved), a single failing trace in isolation, multiple ERROR traces (the exact regression case), all-success traces, mixed success/error, an unresolvable `TraceID` correctly excluded from `TraceCount`, all-unresolvable `TraceIDs`, and two tests locking in the pre-existing (unmodified) `AffectedServices`/`AffectedOperations`/`AffectedEdges` behavior.

**`internal/propagation/analyzer_test.go`** (new, 23 tests, production code untouched): `IsPath` (direct edge, multi-hop, no edges, disconnected graph, reverse-direction, a real cycle — confirming BFS termination — and a documented characterization of `source==target` always returning true, since the loop checks equality before any edge lookup); `CheckTemporalPrecedence` (candidate-first, target-first, exactly-equal timestamps as a boundary, candidate/target never failing, non-ERROR spans ignored, earliest-across-multiple-traces, an unresolvable trace ID skipped safely, empty `TraceIDs`); `CheckPropagation` (direct path with precedence, no path, path without precedence, multi-hop path, candidate excluded from comparison against itself, empty affected list, multiple qualifying callers producing one evidence each).

**Total: 33 new tests**, counted directly via `grep -c "^func Test"` (10 + 23), not estimated.

## Why the Blast Regression Tests Catch the Old Behavior

Mechanically proven, not assumed. The pre-fix `blast.go` was temporarily restored (`git show HEAD:...blast.go`) with the new test file in place, and the suite was run directly against it:

```
--- FAIL: TestCalculate_MultipleErrorTraces_ReportsCorrectCounts
    expected TraceCount 2, got 0 / expected FailureCount 2, got 0
--- FAIL: TestCalculate_AllSuccessTraces_ZeroFailureCount
    expected TraceCount 2, got 0
--- FAIL: TestCalculate_MixedSuccessAndErrorTraces_CorrectCounts
    expected TraceCount 2, got 0 / expected FailureCount 1, got 0
--- FAIL: TestCalculate_UnresolvableTraceID_NotCountedInTraceCount
    expected TraceCount 1, got 0 / expected FailureCount 1, got 0
--- FAIL: TestCalculate_ErrorSpans_PopulateAffectedServicesOperationsEdges
    expected TraceCount=1 FailureCount=1, got TraceCount=0 FailureCount=0
```

5 of the (then-9) tests failed against the pre-fix code, every failure showing `got 0` — reproducing the exact live defect precisely. The remaining tests (zero-`TraceIDs`, all-unresolvable, and the two pre-existing-behavior tests) correctly passed either way, since their expected values were already `0`/pre-existing behavior unrelated to the counts. The fixed `blast.go` was then restored and the full suite re-confirmed passing. A tenth test (`TestCalculate_SingleFailingTrace_ReportsOneAndOne`, an isolated single-trace case) was added afterward for completeness and re-verified passing against the fixed code; it was not separately re-run against the pre-fix code since its logic is a strict subset of the already-proven multi-trace regression tests.

## go build

`go build ./...` — **PASS**, no errors.

## go vet

`go vet ./...` — **PASS**, clean.

## go test

`go test ./...` — **PASS**, all packages `ok`, including `internal/blast` (10 tests, 1.04–1.16s) and `internal/propagation` (23 tests, 1.01–1.19s) newly gaining coverage. Fresh (non-cached, `-count=1`) targeted re-run of `internal/execution`, `internal/rca`, `internal/incidentmanager`, `internal/remediation` — all **PASS**, confirming zero regression in RCA, incident lifecycle, remediation policy/validation, or M2.13's verification fix. Re-run once more, fresh, after adding the tenth blast test — full `go test ./...` still all `ok`.

## go test -race

`go test -race -count=1 ./...` via the established `golang:1.25` Docker container — **PASS**, all 26 packages `ok` (`internal/blast` 1.036s, `internal/propagation` 1.018s, `internal/execution` 65.7s), 0 races, exit code 0. Run twice this session (Docker Desktop required a restart mid-session between them); both runs, including the final one after the tenth blast test was added, clean.

## Regression Confirmation

- **RCA**: `internal/rca` tests pass fresh, unchanged.
- **Propagation**: production `analyzer.go` has zero diff; all new tests characterize existing, unmodified behavior.
- **Incident detection/lifecycle**: `internal/incidentdetector`, `internal/incidentmanager` pass fresh, unchanged.
- **Remediation targeting**: `internal/remediation` (policy/risk/validator) passes fresh, unchanged.
- **Execution/verification**: `internal/execution` passes fresh (65.7s, same as M2.13's own timing), confirming M2.13's fix and its 18 tests remain fully intact.
- **M2.12**: `.github/workflows/ci.yml` diff is byte-identical to before this session; `docs/m212_verification_report.md` untouched.
- **M2.13**: commit `8bf1311` unchanged, unmodified, unamended.

## Frozen-Path Audit

Empty diff confirmed for every listed frozen path: `internal/rca/`, `internal/propagation/analyzer.go` (production), `incidentmanager/{correlator,causal}.go`, `internal/incidentdetector/`, `internal/remediation/` production logic, `internal/execution/{guard,model,engine,verification}.go`, the Docker adapter, `internal/buffer/`, `internal/event/`, `internal/ingestion/`, `internal/normalization/`, and all four E2E scripts. `internal/blast/blast.go` — not frozen — is the only production file modified, exactly as scoped.

## Scope Audit

`git status --short`:
```
 M .github/workflows/ci.yml
 M services/intelligence-engine/internal/blast/blast.go
?? docs/m212_verification_report.md
?? services/intelligence-engine/internal/blast/blast_test.go
?? services/intelligence-engine/internal/propagation/analyzer_test.go
```
`.github/workflows/ci.yml` and `docs/m212_verification_report.md` are **pre-existing, uncommitted M2.12 work**, confirmed byte-identical to their state at the start of this session — not touched by M2.14. `go.mod`/`go.sum`: empty diff, confirmed across root, `services/intelligence-engine/`, and `agents/atlas-agent/`. No new dependency. No unrelated file. No secret. No author/co-author/contributor/AI attribution added anywhere. `git diff --check`: no whitespace errors.

## Findings/Limitations

None discovered beyond the fix's own scope. The defect was exactly as characterized in M2.12's original investigation — no additional hidden issues surfaced while reading `blast.go` or `propagation/analyzer.go` in full.

## Commit/Push Statement

**No commit was made. No push was made.** All M2.14 changes remain in the working tree, unstaged, awaiting explicit review and approval.
