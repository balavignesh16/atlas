# M2.13 Verification Report

## 1. Problem Statement

`Verifier.Verify()` (`internal/execution/verification.go`) accepted `VERIFIED` whenever the watched incident's `Status` became `"RESOLVED"`, on every code path, without ever consulting the `EventBuffer` for genuine post-execution failure evidence first. M2.12's live E2E exposed this directly: `test-m27-docker.ps1` Scenario D — which sends genuinely new, fresh failing traffic strictly after a remediation executes — returned `VERIFIED` in one of two clean runs when `FAILED` was the correct, expected outcome. `ExecutionRecord.Message` would then read `"Verification Passed: service is healthy"` despite real, fresh failure evidence sitting in the buffer.

## 2. Root Cause

`incidentmanager.Manager.CleanupAndResolve()` resolves any `OPEN` incident purely from absence of a fresh signal touching *that specific incident record* for `RecoverySeconds` (30s default) — an independent timeout, unrelated to whether the remediated service is actually healthy. `ProcessSignal()` only reuses an existing incident for a matching fingerprint while it is still `OPEN`; once resolved, a fresh signal with the same fingerprint creates a *new*, differently-IDed incident instead. `Verify()` watches one specific `incidentID` forever, so it has no way to learn that a new incident now holds the fresh evidence — and every path that observed `Status == "RESOLVED"` (entry check, ticker/poll check, and `finalVerdict`'s deadline/cancel path) returned `VerificationVerified` unconditionally, before ever calling `hasGenuinePostExecutionFailure`.

## 3. Exact Production Change

`services/intelligence-engine/internal/execution/verification.go` only. Added one new unexported method, `verdictForResolved(serviceName, executionFinishedAt) VerificationStatus`, which checks `hasGenuinePostExecutionFailure` (the existing, unmodified evidence-scanning function — no new evidence mechanism was introduced) and returns `VerificationFailed` if true, `VerificationVerified` otherwise. All three places that previously returned `VerificationVerified` directly upon observing `Status == "RESOLVED"` (the entry check, the ticker/poll check, and `finalVerdict`'s resolved branch) now call `v.verdictForResolved(...)` instead. No other function, signature, field, or file was touched. Net diff: +39/−10 lines, entirely within this one file.

## 4. Exact Tests Added/Changed

`services/intelligence-engine/internal/execution/verification_test.go`: all 14 pre-existing tests are byte-for-byte unchanged. 4 new tests appended:

- `TestVerify_ResolvedWithGenuinePostExecutionFailure_ReturnsFailedNotVerified` — resolved at entry, genuine failure evidence already present → must be `FAILED`.
- `TestVerify_ResolvesMidPollWithGenuinePostExecutionFailure_ReturnsFailedNotVerified` — incident resolves mid-poll (via the ticker path specifically) while genuine failure evidence is already present → must be `FAILED`.
- `TestVerify_ResolvedWithEvidenceExactlyAtExecutionFinishedAt_ReturnsVerified` — boundary: an event timestamped exactly *at* (not strictly after) `executionFinishedAt`, combined with `RESOLVED`, must not count as evidence → `VERIFIED`.
- `TestVerify_ResolvedWithUnrelatedServiceFailure_StillReturnsVerified` — evidence for a *different* service, combined with `RESOLVED`, must not block a genuine `VERIFIED` for the remediated service.

**Total: 18 top-level test functions in this file** (14 pre-existing + 4 new), counted directly via `grep -c "^func Test"`, not estimated.

## 5. Why the Regression Tests Catch the Old Behavior

Not asserted from reasoning alone — mechanically proven this session. The pre-fix `verification.go` was temporarily restored (`git show HEAD:...verification.go`) with the new test file in place, and the two bug-reproducing tests were run directly against it:

```
--- FAIL: TestVerify_ResolvedWithGenuinePostExecutionFailure_ReturnsFailedNotVerified
    expected FAILED ... got VERIFIED
--- FAIL: TestVerify_ResolvesMidPollWithGenuinePostExecutionFailure_ReturnsFailedNotVerified
    expected FAILED ... got VERIFIED
```

Both boundary/scoping tests (`...ExactlyAtExecutionFinishedAt...` and `...UnrelatedServiceFailure...`) correctly **passed** even against the pre-fix code — expected, since they assert `VERIFIED`, which was already (and remains) the pre-fix code's unconditional answer whenever `Status == "RESOLVED"`; they exist to prove the *fix* doesn't introduce new false positives, not to reproduce the bug. The fixed `verification.go` was then restored and all four new tests re-run, confirmed passing (see §8).

## 6. go build

`go build ./...` — **PASS**, no errors, no changes needed.

## 7. go vet

`go vet ./...` — **PASS**, clean.

## 8. go test

`go test ./...` — **PASS**, all packages `ok`. `internal/execution` specifically: 65.8s (unchanged from pre-fix timing; the new tests use the same short-`RecoverySeconds`/sub-15s-context pattern as the existing suite, no new long waits), all 18 tests in `verification_test.go` passing, including the 4 new ones confirmed individually.

## 9. go test -race

`go test -race -count=1 ./...` via the established `golang:1.25` Docker container — **PASS**, all 26 packages `ok`, 0 races, exit code 0.

## 10. E2E Results

**Locally executed** (Windows PowerShell 5.1, not `pwsh` — `pwsh` is not installed on this development machine, an explicit, previously-documented limitation carried over from M2.12; this is not equivalent to GitHub Actions' Ubuntu/`pwsh` environment): `test-m27-docker.ps1`, unmodified, run twice in full, clean (`docker-compose down` between runs, fresh build the first time, cached images the second):

- **Run 1**: Scenarios A, B, C, D all passed. Scenario D: `Final Verification Status: FAILED` — correct.
- **Run 2**: Scenarios A, B, C, D all passed. Scenario D: `Final Verification Status: FAILED` — correct.

**2 of 2 clean runs correct**, matching the deterministic unit-test proof: the fix makes the specific defect structurally unreachable (evidence is now checked as an unconditional part of every `RESOLVED` observation), not merely less probable, so this is consistent with — not merely lucky compared to — M2.12's 1-of-2 pre-fix result.

**Not executed this session**: `test-m28-chaos.ps1`. Reasoning, stated plainly rather than assumed: its Scenario 3 exercises the same underlying `RESOLVED`-vs-evidence mechanism this fix targets (and already treats false `VERIFIED` as a hard failure), but running its full 3-scenario suite (historically longer than `test-m27`'s already-substantial runtime) was judged lower-value given the mechanism was already (a) deterministically proven at the unit level in both directions, and (b) independently re-confirmed live, twice, via `test-m27`'s Scenario D — the same code path `test-m28` Scenario 3 exercises. This is a time/practicality trade-off, disclosed rather than hidden; `test-m28-chaos.ps1` itself was not modified and its historical behavior is unaffected by this fix in any way that would make it *less* correct.

**GitHub Actions**: **not run**. No real GitHub Actions execution was triggered or observed from this environment. This must not be read as "CI passed" — it did not run at all this session.

## 11. Frozen-Path Audit

Empty diff confirmed for every path required: `execution/model.go`, `execution/engine.go`, `execution/guard.go`, `incidentmanager/manager.go`, the Docker adapter, `buffer/`, `event/`, `ingestion/`, `normalization/`, `rca/`, `propagation/`, `incidentdetector/`, `remediation/` production logic, and all four E2E scripts (`test-m27-docker.ps1`, `test-m28-chaos.ps1`, `test-m29-security.ps1`, `test-m211-security-read.ps1`).

## 12. Scope Audit

`git status --short`:
```
 M .github/workflows/ci.yml
 M services/intelligence-engine/internal/execution/verification.go
 M services/intelligence-engine/internal/execution/verification_test.go
?? docs/m212_verification_report.md
```
The `ci.yml` modification and `docs/m212_verification_report.md` are **pre-existing, uncommitted M2.12 work**, unrelated to and untouched by this M2.13 session — carried over from the prior turn, still awaiting your review/commit decision, not part of this milestone's changes. M2.13's own changes are exactly the two files listed in §3–4, plus this report. No `go.mod`/`go.sum` changes (confirmed via `git diff --stat`), no new dependency, no unrelated file, no secret, no author/co-author/contributor/AI attribution added anywhere. `git diff --check` reports no whitespace errors.

## 13. Findings/Limitations

1. `pwsh` remains absent from this local development machine; local E2E verification used Windows PowerShell 5.1, a known, previously-disclosed imperfection relative to the actual GitHub Actions environment.
2. `test-m28-chaos.ps1` was not re-run this session (see §10's reasoning) — a legitimate candidate for follow-up confirmation, not a defect.
3. No new defect was discovered during this fix's implementation or verification.
4. The M2.12 `ci.yml`/report changes remain uncommitted in the working tree, unrelated to M2.13; they are not part of this milestone's diff and were not touched.

## 14. Commit/Push Statement

**No commit was made. No push was made.** All changes remain in the working tree, unstaged, awaiting explicit review and approval.
