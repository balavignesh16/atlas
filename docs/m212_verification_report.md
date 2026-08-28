# M2.12 Verification Report

## Scope

CI-gate the Docker-based end-to-end execution verification by adding a new `docker-e2e` job to `.github/workflows/ci.yml` that runs the existing, unmodified `test-m27-docker.ps1` on a real `ubuntu-latest` GitHub Actions runner.

## Repository Baseline

Verified before implementation: `HEAD=609ba3c` (M2.11 verification report), working tree clean, `main` ahead of `origin/main` by 13 commits (4 M2.9 + 4 M2.10 + 5 M2.11).

## Why `test-m27-docker.ps1` Was Selected

Per the M2.12 investigation: it is the base execution E2E (M2.7), the most self-contained of the four scripts (it brings up and builds the stack itself, unlike `test-m29`/`test-m211` which additionally require `ATLAS_SECURITY_ENABLED`/API-key setup), and proves the core plan→approve→execute→verify chain against real Docker infrastructure — the most fundamental safety property to gate first.

## Exact CI Changes

Added a new job, `docker-e2e`, to `.github/workflows/ci.yml`, gated with `needs: build-and-test` (runs only after the existing Go/Java checks pass), `timeout-minutes: 30`. All existing jobs/steps (`build-and-test`: Go fmt/vet/test/race/build, ATLAS Agent fmt/vet/test/build, control-plane Java test/build) are byte-for-byte unchanged.

Steps, in order:
1. `actions/checkout@v4`
2. **Ensure docker-compose CLI is available** — `test-m27-docker.ps1` invokes the legacy hyphenated `docker-compose` throughout; this step installs a one-line shim forwarding to `docker compose` (v2 plugin) only if the hyphenated binary isn't already on `PATH`. Never touches the test script.
3. **Verify pwsh is available** — `pwsh --version`, a fast, clear failure point if it's ever missing, rather than a deep, confusing failure inside the script invocation.
4. **Run Docker E2E** — `pwsh -File ./test-m27-docker.ps1`, invoked exactly as it would be run locally, with no flags. The script is self-contained for build/bring-up (`docker-compose down; docker-compose up -d --build`) and its own readiness wait (polling the gateway's health endpoint) when not passed `-SkipBuild`.
5. **Collect service logs on failure** (`if: failure()`) — `docker-compose ps` + `docker-compose logs --no-color`, visible directly in the CI run's log output.
6. **Tear down** (`if: always()`) — `docker-compose down`, runs regardless of outcome.

A job-level `env:` block sets `ATLAS_EXECUTION_ENABLED=true` and `ATLAS_EXECUTION_PROVIDER=docker` — see Findings below for why this was required.

## Local Verification Performed

**Go suite** (no Go source changed this milestone; re-verified anyway per the repository rules): `go build ./...`, `go vet ./...`, `go test ./...` (26/26 packages, all pass), `go test -race ./...` via the established `golang:1.25` Docker container (26/26 packages, 0 races) — all re-run fresh this session, after the CI YAML change, all clean.

**YAML validation:** `python -c "import yaml; yaml.safe_load(...)"` — parses correctly; the `docker-e2e` job's structure was additionally dumped and inspected field-by-field.

**Shell syntax validation:** every `run:` script in the new job was extracted from the parsed YAML and checked with `bash -n` — all pass.

**Shim logic verification:** the `docker-compose`-missing branch was exercised in isolation (a fake empty directory prepended to `PATH`, the shim written and made executable, then invoked against a fake `docker` binary) — confirmed it correctly forwards `docker-compose down` → `docker compose down` with arguments preserved intact.

**Local functional reproduction of the E2E step, with an important environment caveat:** `pwsh` (PowerShell Core) is **not installed on this local Windows development machine** — only Windows PowerShell 5.1 is available. GitHub's `ubuntu-latest` runners do ship `pwsh`, which is why the CI job correctly targets it; this local machine simply doesn't have it, so a fully faithful local reproduction of the exact CI shell was not possible here. As a best-effort substitute, `test-m27-docker.ps1` was run twice, unmodified, via Windows PowerShell 5.1 (`powershell -File ./test-m27-docker.ps1`, no flags — the same invocation form the CI step uses, modulo the `pwsh`/`powershell.exe` binary difference), against the real local Docker Engine. **This is explicitly not a GitHub Actions run and is not presented as one.**

## CI Workflow Behavior (Local Reproduction Results)

**Run 1** (first-ever fully clean invocation, no environment set up in advance): failed immediately at Scenario A's execute call with `{"error":"execution is disabled by configuration (ATLAS_EXECUTION_ENABLED=false)"}`. Root-caused precisely, not guessed: `test-m27-docker.ps1` (confirmed via `grep`, frozen, unmodified) never sets `ATLAS_EXECUTION_ENABLED`/`ATLAS_EXECUTION_PROVIDER` itself — unlike `test-m29-security.ps1`/`test-m211-security-read.ps1`, which both set them internally — and `docker-compose.yml` defaults `ATLAS_EXECUTION_ENABLED` to `false` (fail-closed by design). This was, as far as this project's history shows, the **first time this script has ever been invoked in a genuinely clean shell** with no operator having manually exported these variables beforehand — every prior session's runs (this one included, until this point) had inherited them from earlier interactive setup. A fresh GitHub Actions runner is exactly such a clean shell, so this would have failed identically in real CI. **Fixed within M2.12 scope**: added `ATLAS_EXECUTION_ENABLED: "true"` / `ATLAS_EXECUTION_PROVIDER: "docker"` as job-level `env:` in `ci.yml` — this is environment provisioning for the existing test to run as designed, not a modification to the script, and not a change to any production default (`docker-compose.yml`'s fail-closed default is untouched).

**Run 2** (after the fix, full clean cycle: teardown → rebuild → bring-up → all 4 scenarios): Scenarios A, B, C passed. **Scenario D failed**: `Expected FAILED given real, freshly-sent post-execution payment errors, got VERIFIED`.

**Run 3** (retry, same fixed configuration, full clean cycle again): **all 4 scenarios passed**, exit code 0, "Test completed successfully."

## E2E Result

`test-m27-docker.ps1` has **not** been run on actual GitHub Actions — no run was triggered or observed on the real GitHub Actions infrastructure from this environment. **CI workflow configured but not live-verified on GitHub Actions.**

Locally (Windows PowerShell 5.1, not `pwsh`/not Ubuntu — an explicitly imperfect substitute), the script passed 1 of 2 clean full runs after the required environment fix; the first of those two failed on Scenario D specifically, not on the CI-added machinery (Scenarios A–C, plan/approve/execute/real-Docker-restart, all passed in both post-fix runs).

## Timing/Health-Check/Flakiness Observations — Root-Caused, Not Guessed

Read `internal/execution/verification.go` (frozen) directly to understand Scenario D's failure rather than speculate. `Verifier.Verify()` polls the *specific* incident it was called with for `Status == "RESOLVED"` on a 2-second ticker; the moment that specific incident resolves, it returns `VerificationVerified` immediately, **without checking the EventBuffer for genuine post-execution failures first**. M2.4's own incident-manager independently resolves any incident after `RecoverySeconds` (30s) with no fresh signal *touching that same incident's fingerprint*. Scenario D's fresh, genuinely-new post-execution traffic does not necessarily land under the *same* incident record — if enough of the original incident's 30-second recovery clock had already elapsed by the time execution finished (which varies run to run, since Scenarios A–C's traffic/wait timing shifts how "aged" Scenario D's own detected incident already is by the time its own execute call completes), the *original* incident can resolve via ordinary timeout before the fresh failure is correlated and reflected back onto it — and `Verify()` declares `VERIFIED` as soon as it observes that resolution, regardless of the genuinely fresh failure sitting in the EventBuffer. This is a **real, pre-existing timing race in frozen code**, first empirically exposed by running the script in truly clean, back-to-back conditions (1 failure in 2 post-fix runs) — not introduced by this milestone (no execution/verification code was touched), and not fixed by this milestone (out of scope, frozen). Per the explicit failure-handling rule for this milestone, this is reported rather than patched, retried-around, or hidden.

## Files Changed

- `.github/workflows/ci.yml` — modified, +63 lines (new `docker-e2e` job only; all existing jobs/steps unchanged).
- `docs/m212_verification_report.md` — new, this report.

## Frozen-Path Audit

`git diff --stat` against every listed frozen path (`internal/rca/`, `internal/propagation/`, `incidentmanager/{correlator,causal}.go`, `internal/incidentdetector/`, `internal/remediation/` production logic, `internal/execution/{guard,model,engine,verification}.go`, the Docker adapter, `internal/buffer/`, `internal/event/`, `internal/ingestion/`/`internal/normalization/` implementations, all four E2E scripts): **empty in every case.** Confirmed both before and after the local E2E diagnostic session.

## Dependency Audit

`git diff --stat` against `go.mod`/`go.sum` (root, `services/intelligence-engine/`, `agents/atlas-agent/`): empty. No new dependency introduced. The CI job uses only tooling already present on `ubuntu-latest` (Docker, Docker Compose or the v2 plugin, `pwsh`) plus `actions/checkout@v4`, already used elsewhere in this same workflow.

## Git State

```
 M .github/workflows/ci.yml
```
Nothing staged, nothing committed, nothing pushed. `main` remains 13 commits ahead of `origin/main`, unchanged.

## Known Limitations

1. **No actual GitHub Actions run was performed or observed.** The workflow is configured and locally validated (YAML syntax, shell syntax, shim logic, and a best-effort functional reproduction) but not proven on real CI infrastructure. The `docker-compose`-vs-`docker compose` CLI-name shim in particular is a defensive measure for an unconfirmed runner-image detail that could not be checked from this environment.
2. **`pwsh` is not installed on this local development machine**, so local verification used Windows PowerShell 5.1 instead — not the same shell CI will actually use, and not the same OS.
3. **Scenario D of `test-m27-docker.ps1` has demonstrated real, reproducible flakiness** (1 failure in 2 clean post-fix runs), root-caused to a genuine timing race in frozen `internal/execution/verification.go` logic, unrelated to this milestone's changes. This means the newly CI-gated job may intermittently fail even when the code under test is entirely correct. This is a legitimate finding for a follow-up milestone to address (in `internal/execution/verification.go`, out of M2.12's scope) — not something this milestone attempted to fix, retry around, or paper over.
4. Per instruction, `internal/blast/`'s known `FailureCount`/`TraceCount` bug, sanitizer gaps, the `POST /{id}/analyze` auth gap, Java lab-service CI, and all other M2.12-excluded items remain exactly as they were — untouched.

## Final Verdict

M2.12's CI configuration change is complete, locally validated at every level available in this environment, and has not introduced any frozen-path, dependency, or unrelated change. It surfaced one real, in-scope defect (missing `ATLAS_EXECUTION_ENABLED` provisioning), which was fixed within scope, and one real, out-of-scope, pre-existing defect (Scenario D's timing race), which is reported rather than fixed. The workflow is **configured but not live-verified on GitHub Actions.**
