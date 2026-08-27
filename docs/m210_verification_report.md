# M2.10 Verification Report

Generated 2026-08-27 from actual command output. Nothing in this milestone has been committed — this report is for review before that decision.

## Objective

Make the project's continuously enforced CI safety net match the verification discipline already being performed manually every milestone: add `go test -race` as an actual CI step, reconcile CI's Go/Java version pins with what the repository itself declares as authoritative, rename the stale "M0" workflow, and add real unit test coverage to the two zero-coverage packages sitting directly on the untrusted-input boundary (`ingestion`, `normalization`).

## Changes Made

- `.github/workflows/ci.yml` — modified (7 insertions, 4 deletions).
- `services/intelligence-engine/internal/ingestion/otlp_test.go` — new, 9 top-level test functions.
- `services/intelligence-engine/internal/normalization/normalizer_test.go` — new, 15 top-level test functions (one, `TestNormalizeTraces_StatusCodeMapping`, has 3 table-driven subtests).
- `services/intelligence-engine/internal/normalization/sanitizer_test.go` — new, 6 top-level test functions (`TestIsSensitiveAttribute` has 14 table-driven subtests; the other 5 are standalone `SanitizeString` cases).
- No production `.go` file was modified. No dependency was added (`go.mod`/`go.sum` diff confirmed empty).

## CI Changes

Added a new step, `Go Test with Race Detector (Intelligence Engine)`, running `go test -race -v ./...` immediately after the existing `go test -v ./...` step — it does not replace or weaken the existing step, both now run. Renamed the workflow from `ATLAS M0 CI` to `ATLAS CI`. No other job, step, or check was removed or altered — `agents/atlas-agent`'s and `services/control-plane`'s existing format/vet/test/build steps are byte-identical to before.

**Important distinction, stated explicitly per instruction**: this milestone adds the race-detector step to CI *configuration*. It has not been executed *by* CI, because nothing has been pushed to `origin/main` — GitHub Actions has not run this workflow. "The race suite passes locally" (verified below, in a manually-invoked Linux container, exactly as every prior milestone this session) is a different, narrower claim than "the CI race gate has been exercised by CI" — only the former is true right now.

## Go/Java Version Reconciliation

| | Previous CI value | Repository's own declared value | Final CI value |
|---|---|---|---|
| Go | `1.26` (comment: "User requested Go 1.26") | `go 1.25.0` in `services/intelligence-engine/go.mod` (the only Go module CI actually builds/tests beyond `atlas-agent`, which declares `go 1.24` — satisfied by anything ≥1.24) | `1.25` |
| Java | `25` (comment: "User requested Java 25") | `<java.version>24</java.version>` in `services/control-plane/pom.xml`, with its own existing comment: *"User requested Java 25 LTS but local compiler is Java 24. Falling back to 24 to fix compatibility issue"* | `24` |

Both previous CI values were arbitrary external requests, not derived from the repository. Both final values are the repository's own authoritative declarations — chosen per the explicit instruction to prefer the repository's declared version over the local environment. `1.25` for Go satisfies both `intelligence-engine`'s `go 1.25.0` and `atlas-agent`'s `go 1.24` directives simultaneously (a newer toolchain always satisfies an older module's minimum). No `toolchain` directive exists in either `go.mod`, so this does not rely on Go's automatic toolchain-download behavior — the installed `1.25` toolchain directly satisfies the declared minimum.

**Scope note**: CI's Java build step (`mvn clean test package`) only ever ran for `control-plane`. `gateway`, `order-service`, `inventory-service`, and `payment-service` — the actual lab application services — also declare `<java.version>24</java.version>` (confirmed by direct inspection) but have **no CI coverage at all**, before or after this milestone. Adding that coverage is outside M2.10's approved scope (it would be a new CI job, not a version reconciliation) and is documented here as a finding, not implemented.

## Ingestion Test Coverage

`services/intelligence-engine/internal/ingestion/otlp_test.go`, 9 tests against the real, unmodified `OTLPHandler`, wired to real `buffer.EventBuffer`, `correlation.Engine`, and `incidentdetector.Detector` instances (no mocks): wrong HTTP method (405) for both `/v1/traces` and `/v1/metrics`; malformed protobuf body (400) for both; a `Content-Encoding: gzip` header with a non-gzip body (400); a genuinely gzip-compressed valid payload, verified to decompress correctly and land in the real buffer with fields intact; an uncompressed valid payload dispatched end-to-end (ingest → normalize → buffer, with the real detector's `ProcessEvent` log line observed firing during the test run — confirmed real, not assumed); a structurally valid but empty payload (200, zero events); a valid metrics payload confirmed to reach only the buffer (not the detector/correlator, matching `HandleMetrics`'s actual, narrower dispatch — an asymmetry worth locking in as a regression test since it's easy to accidentally "fix" without noticing it was deliberate).

## Normalization Test Coverage

`sanitizer_test.go` (6 top-level tests, 19 assertions counting subtests): `IsSensitiveAttribute` against all six documented tokens (case-insensitive, substring-matched) plus non-sensitive keys and the empty string (14 table-driven subtests); `SanitizeString` at under/at/over the limit, empty input, and the zero-limit edge case (5 standalone tests).

`normalizer_test.go` (15 top-level tests): basic span→event field mapping (trace/span ID hex encoding, operation name, duration computed from start/end nanos); the `unknown_service` fallback when no `service.name` resource attribute is present; all three status-code mappings (OK/ERROR/UNSET) plus a nil-`Status`-message case (proving `GetStatus().GetCode()` on nil doesn't panic and correctly defaults to UNSET); parent-span-ID presence and absence; the zero-start-time → `time.Now()` fallback (asserted via a before/after time window, not a sleep); sensitive-attribute exclusion integrated through the real `normalizeSpan`→`extractSafeAttributes`→`IsSensitiveAttribute` path; the documented 256-attribute cap; multi-resource/multi-scope flattening; empty input; `NormalizeMetrics` for both `Sum`/`AsDouble` and `Gauge`/`AsInt`; the documented zero-event result for an unsupported metric type (neither Sum nor Gauge set); and the same zero-timestamp fallback for metric data points.

## Test Results

```
go build ./...                exit=0
go vet ./...                  exit=0
go test ./...                 exit=0, all packages pass, including internal/ingestion and
                               internal/normalization for the first time (previously
                               "[no test files]")
go test -race ./... (Linux
container, golang:1.25)       exit=0, all packages pass, no races detected
```
All four commands re-run fresh in this session, output captured directly, not cited from a prior report.

## Existing E2E Regression Results

**NOT re-executed live this session.** M2.10 changes zero runtime/production behavior — confirmed by an empty `git diff` on every production `.go` file and on all three existing E2E scripts (`test-m27-docker.ps1`, `test-m28-chaos.ps1`, `test-m29-security.ps1`, all byte-identical, verified via `git diff --stat`). Re-running all three live (each requiring a fresh Docker build and multi-minute cascade/wait cycles) would consume significant time to reconfirm something the diff already proves: nothing they exercise has changed since their last successful execution, earlier in this same session (M2.9's release review). Per instruction, this is reported honestly rather than claimed as freshly verified — the E2E suites are **UNAVAILABLE-BY-CHOICE this session, not FAILED, not SKIPPED-SILENTLY**.

## Frozen-Path Audit

```
git diff --stat -- internal/rca/                                (empty)
git diff --stat -- internal/propagation/                         (empty)
git diff --stat -- internal/incidentmanager/correlator.go        (empty)
git diff --stat -- internal/incidentmanager/causal.go             (empty)
git diff --stat -- internal/incidentdetector/                      (empty)
git diff --stat -- internal/remediation/                            (empty)
git diff --stat -- internal/execution/guard.go                       (empty)
git diff --stat -- internal/execution/model.go                        (empty)
git diff --stat -- internal/execution/engine.go                        (empty)
git diff --stat -- internal/execution/verification.go                   (empty)
git diff --stat -- internal/infrastructure/docker/                       (empty)
git diff --stat -- internal/buffer/                                       (empty)
git diff --stat -- internal/event/                                         (empty)
git diff --stat -- internal/ingestion/otlp.go                               (empty)
git diff --stat -- internal/normalization/normalizer.go                      (empty)
git diff --stat -- internal/normalization/sanitizer.go                        (empty)
git diff --stat -- test-m27-docker.ps1 / test-m28-chaos.ps1 / test-m29-security.ps1 (empty)
```
Every path independently re-checked in this session. Under `ingestion`/`normalization`, only new `_test.go` files appear — confirmed via `git status --short` on those two directories.

## Scope / Diff Audit

`git status --short`: `.github/workflows/ci.yml` (modified) + 3 new test files. `git diff --stat`: `.github/workflows/ci.yml | 11 +++++++----, 1 file changed, 7 insertions(+), 4 deletions(-)`. No unrelated files, no `go.mod`/`go.sum` changes, no secrets (the workflow file contains no credentials; the new test files contain only synthetic in-memory test data). Exactly matches the expected scope stated in the implementation instructions — nothing additional.

## Findings

1. **`x-api-key` sanitizer gap.** `normalization.IsSensitiveAttribute`'s `"api_key"` token only matches the underscored form, not the equally common hyphenated `"api-key"`/`"x-api-key"` convention (notably including this project's own M2.9 header name, `X-Atlas-Api-Key`).
   - **Status: PRE-EXISTING BEHAVIOR, discovered by M2.10 testing.** The behavior existed in `sanitizer.go` before this milestone touched anything; it surfaced when `TestNormalizeTraces_SensitiveAttributesExcluded`'s original hyphenated assertion failed against the real, unmodified implementation — a real discovery, not a test-authoring mistake.
   - **NOT introduced by M2.10.** No line of `sanitizer.go` or `normalizer.go` was changed by this milestone (confirmed empty diff, §"Frozen-Path Audit").
   - **Intentionally NOT fixed in M2.10.** Changing `sanitizer.go`'s token list is a production-behavior change outside this milestone's approved CI/test-infrastructure scope, and is exactly the class of discovery the implementation instructions required stopping on rather than silently patching.
   - **Follow-up classification: a security/data-sanitization issue**, tracked here for a future, explicitly-scoped milestone — not CI/test-infrastructure work. Severity low-moderate: it affects what gets *stored/logged* from span attributes, not M2.9's authentication itself (the key-comparison and header-extraction code paths are entirely separate from this sanitizer and are unaffected).
   - Encoded, without endorsement, as `TestNormalizeTraces_HyphenatedApiKey_NotCaught`.
2. **UTF-8 truncation boundary.** `normalization.SanitizeString` truncates by byte length (`val[:limit]`), which can split a multi-byte UTF-8 character at the boundary and produce an invalid UTF-8 result — confirmed via a one-off scratch verification (`SanitizeString("héllo", 2)` → `"h\xc3...(truncated)"`, invalid UTF-8).
   - **Status: PRE-EXISTING LIMITATION**, not a regression — the byte-slice truncation logic is original, untouched code; M2.10 only observed it.
   - **NOT fixed in M2.10** and not committed as a test asserting the broken output (that would read as locking in a bug rather than documenting a promise the function actually makes).
   - Severity low: ASCII attribute values are the overwhelming common case across this project's lab services, so real-world impact is limited, but it remains a genuine correctness gap in a function whose entire purpose is safe truncation.
3. **Java CI coverage gap.** CI never builds or tests the four lab services (`gateway`, `order-service`, `inventory-service`, `payment-service`) — only `control-plane` gets a Maven step.
   - **Status: PRE-EXISTING**, unrelated to and unchanged by M2.10's version-reconciliation work (§"Version Reconciliation" only corrected the version *values* CI already used for `control-plane`; it did not add coverage for the other four services).
   - **Deliberately NOT expanded into this milestone** — adding new Java CI jobs is a scope increase beyond "reconcile existing CI," not a version fix.

None of these three findings block this milestone's own Definition of Done — none touch what M2.10 actually changed (CI config, new test files), and none were introduced by this milestone; all three were *discovered*, not caused, by it. None have been addressed by modifying any production implementation.

## What Was NOT Changed

`internal/rca/`, `internal/propagation/`, `internal/incidentmanager/correlator.go`, `internal/incidentmanager/causal.go`, `internal/incidentdetector/`, `internal/remediation/`, `internal/execution/guard.go`, `internal/execution/model.go`, `internal/execution/engine.go`, `internal/execution/verification.go`, Docker adapter files, `internal/buffer/`, `internal/event/`, and — critically for this milestone — the actual *implementations* of `internal/ingestion/otlp.go` and `internal/normalization/{normalizer,sanitizer}.go` (only new test files were added alongside them). `test-m27-docker.ps1`, `test-m28-chaos.ps1`, `test-m29-security.ps1` are untouched. `go.mod`/`go.sum` are untouched — no new dependency. `services/control-plane/`, `agents/atlas-agent/` were inspected but neither deleted, rewritten, nor repurposed.

## Definition of Done

- [x] `go test -race ./...` added as a real CI step (not merely mentioned).
- [x] Go/Java version pins reconciled to the repository's own authoritative declarations, with the previous/required/final values and reasoning documented.
- [x] Workflow renamed away from "M0".
- [x] `internal/ingestion/` and `internal/normalization/` have real, behavior-exercising unit tests (previously zero).
- [x] All existing M2.1–M2.9 Go tests and both `go vet`/`go build` remain clean.
- [x] `go test -race ./...` clean, locally, in the same Linux-container method used throughout this project.
- [x] Existing E2E regression scripts confirmed byte-identical (not re-executed live, honestly disclosed why).
- [x] Every frozen path independently re-confirmed empty diff.
- [x] No new dependency, no secret, no unrelated change.

## Final Verdict

M2.10 IMPLEMENTATION: COMPLETE (as scoped — CI configuration and new test coverage only; no production behavior touched)

Awaiting your review before any commit.
