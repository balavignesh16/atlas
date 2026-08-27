# ATLAS M2.9 Verification Report — Security & Authorization Boundary

Generated 2026-08-27 from actual command output and live `docker compose` runs. Nothing in this milestone has been committed — this report is for review before that decision.

## 1. Objective

Close the gap identified in the M2.9 investigation: the remediation/execution API had no authentication or authorization, and the `approver` audit field was an unverified client-supplied string. This milestone establishes: API key → authenticated Principal → RBAC permission check → the existing, frozen M2.6/M2.7 remediation pipeline, unchanged.

## 2. Initial security finding (recap)

Confirmed by direct code reading during the investigation: `HandleApprove` decoded only `{reason}` from the request body — no approver identity was ever captured, not even self-reported. `HandleExecute` recorded `req.Approver`, a raw, unverified client-supplied string, directly into `ExecutionRecord.Approver`. Any network caller reaching `:8081` could generate, approve, and execute a real remediation action with zero identity check anywhere on the path.

## 3. Implemented architecture

Static API keys, resolved at request time to a named `Principal{Name, Role}`, checked against a per-endpoint required `Permission` via new Go `net/http` middleware (`internal/security`). No external dependency, no JWT/OAuth/mTLS/reverse proxy — matching the approved scope exactly. Disabled by default (`ATLAS_SECURITY_ENABLED=false`), mirroring the existing `ATLAS_EXECUTION_ENABLED` convention, so `test-m27-docker.ps1`/`test-m28-chaos.ps1` remain byte-for-byte unaffected (confirmed: neither file has any diff).

## 4. Authentication design

`ATLAS_API_KEYS` environment variable, format `name:key:role,name:key:role,...` — a single env var, consistent with this project's existing `${VAR:-default}` docker-compose convention, no new configuration system. Header: `X-Atlas-Api-Key` (`security.APIKeyHeader`). Missing key → `401`. Invalid key → `401` (identical response body, so a caller cannot distinguish "key absent" from "key wrong" — no enumeration signal). `KeyStore.Lookup` compares the presented key against every configured key via `crypto/subtle.ConstantTimeCompare` (not a map lookup on the presented value), so lookup timing carries no information about which, or whether any, key matched. Raw keys are never logged: log statements throughout the new package reference only `Principal.Name`; a dedicated test (`TestAuthenticate_ErrorResponse_NeverContainsThePresentedKey`) proves the error body never echoes the presented key. Malformed configuration (wrong field count, empty fields, unrecognized role, duplicate key, duplicate name) fails the server at startup with a descriptive error — confirmed live: an early version of the E2E script's PowerShell string interpolation bug (`"$Key:ROLE"` silently truncating to `"Key:"` — a real PowerShell scope-qualifier gotcha, not a security package defect) caused exactly this startup rejection, which is precisely the fail-closed behavior intended.

## 5. Authorization / RBAC model

Five permissions (`VIEW`, `CREATE_PLAN`, `APPROVE_PLAN`, `EXECUTE`, `READ_AUDIT`), five roles (`VIEWER`, `OPERATOR`, `APPROVER`, `EXECUTOR`, `ADMIN`), matching the investigation's proposed mapping exactly — confirmed against the actual API surface, no permission added without a corresponding endpoint. `Guard.enabled` (a global on/off switch) is unrelated and untouched; RBAC is a strictly additive per-caller layer on top of it. Handlers declare `RequirePermission(x)`; none reference a role directly — the mapping lives in one place (`internal/security/model.go`).

## 6. Trusted identity flow

`Authenticate` middleware resolves the API key to a `Principal` and attaches it to the request context — never to the request body. Handlers read `security.FromContext(r.Context())`; when absent (security disabled), they fall back to pre-M2.9 behavior exactly. Two mandatory properties, both proven live in Scenario E below, not merely unit-tested: `ApprovalMetadata.ApprovedBy` and `ExecutionRecord.Approver` are always sourced from the authenticated principal when one exists, never from a client-supplied field.

## 7. Approval audit identity

`CreatedBy` was investigated and deliberately **not** added: no current endpoint or safety decision consumes "who created the plan," separation-of-duties enforcement is explicitly out of scope for M2.9, and adding it would have expanded the frozen-path reopening beyond the approved minimal scope. `ApprovedBy` was added (Option 2, as explicitly approved) — a single new field on `ApprovalMetadata` ([model.go](services/intelligence-engine/internal/remediation/model.go)) and one new parameter on `Planner.ApprovePlan` ([planner.go](services/intelligence-engine/internal/remediation/planner.go)). Every call site was located before changing the signature (`grep` confirmed exactly one Go call site for `ApprovePlan`, in `httpapi/remediation.go`) and updated deliberately — no accidental compilation-driven discovery. `RejectPlan`'s signature was deliberately left unchanged (no `RejectedBy` added) to keep the reopening minimal, though the reject endpoint is now gated behind the same `APPROVE_PLAN` permission as approve, since both are the same class of plan-disposition action. The approval request body was **not** extended with any identity-like field — the cleanest way to guarantee nothing can be forged there is for no such field to exist at all; `TestHandleApprove_RequestBodyHasNoIdentityFieldToForge` proves a body attempting to smuggle `approver`/`approvedBy` has zero effect.

## 8. Protected routes

**Protected** (require authentication + the named permission when `ATLAS_SECURITY_ENABLED=true`):
- `POST /api/v1/incidents/{id}/remediation/plan` → `CREATE_PLAN`
- `POST /api/v1/remediation/{planId}/approve` → `APPROVE_PLAN`
- `POST /api/v1/remediation/{planId}/reject` → `APPROVE_PLAN`
- `POST /api/v1/remediation/{planId}/execute` → `EXECUTE`

**Intentionally left public in this milestone** (documented decision, not an oversight): all `GET` endpoints (incidents, graph, correlations, events, executions, plan reads), `/health`, and the OTLP ingestion endpoints (`/v1/traces`, `/v1/metrics`). Reasoning: (a) the core security property this milestone targets is specifically *unauthorized remediation execution*, not read confidentiality; (b) `/health` and OTLP ingestion have no caller-supplied credentials in this architecture (Docker healthchecks, the OTel Collector) and protecting them would have broken container health checks and the telemetry pipeline, which the investigation explicitly warned against; (c) protecting reads would have broken `test-m27-docker.ps1`/`test-m28-chaos.ps1`'s extensive unauthenticated read calls even with security disabled by default, since those scripts never set `ATLAS_SECURITY_ENABLED=true` and were never intended to change. `VIEW`/`READ_AUDIT` permissions exist in the role model for completeness and future use but are not enforced on any route today — a deliberate, disclosed scope boundary.

## 9. Unit test results

`go test -v ./internal/security/...`: all pass — `TestParseAPIKeys_*` (empty, valid, whitespace-tolerant, six distinct malformed-entry cases, duplicate key, duplicate name), `TestKeyStore_Lookup_*` (unknown key, empty key, deterministic across repeated calls), `TestAuthenticate_*` (missing key → 401, invalid key → 401, valid key attaches principal → 200, disabled → pass-through with no principal, error response never contains the presented key), `TestRequirePermission_*` (unauthorized role → 403, authorized role → 200, disabled → pass-through, VIEWER blocked from all three mutating permissions, ADMIN allowed all five), `TestHasPermission_RoleMapping` (25 role×permission combinations), `TestHasPermission_UnrecognizedRoleFailsClosed`, context round-trip tests. 25 top-level test functions in the new package (56 individual assertions counting every subtest leaf, e.g. the 6 malformed-entry cases and 25 role×permission combinations above).

`go test -v ./internal/httpapi/...`: 9 new tests, all pass — `TestHandleApprove_AuthenticatedPrincipalRecordedAsApprovedBy`, `TestHandleApprove_RequestBodyHasNoIdentityFieldToForge`, `TestHandleApprove_NoPrincipalInContext_ApprovedByStaysEmpty`, `TestHandleApprove_ReApprovalRemainsRejected`, `TestHandleApprove_NonexistentPlanRejected`, `TestHandleExecute_AuthenticatedPrincipalRecordedAsApprover`, **`TestHandleExecute_ForgedApproverInBody_CannotOverrideAuthenticatedIdentity`** (the mandatory forged-identity test), `TestHandleExecute_NoPrincipalInContext_FallsBackToBodyApprover`, `TestHandleExecute_GuardStillRejectsUnapprovedPlan`.

## 10. Integration test results

N/A as a separate category — the httpapi tests above already exercise real `remediation.Planner`, `incidentmanager.Manager`, and `execution.Engine` components end-to-end at the Go level (not mocked), matching this project's established testing convention.

## 11. Docker E2E results

`test-m29-security.ps1`, full run against a fresh build, `ATLAS_SECURITY_ENABLED=true`:

```
Scenario B (part 1 -- unauthorized approval): PASS (403, plan not approved)
Scenario A (unauthenticated remediation): PASS (401 on plan/approve/execute, StartedAt unchanged)
Scenario C (unauthorized execution): PASS (403, StartedAt unchanged)
Scenario B (part 2 -- unauthorized execution): PASS (403, StartedAt unchanged)
Scenario E (forged approver identity): PASS (authenticated identity 'executor1' recorded, forged 'admin' ignored)
Scenario D (legitimate authorized chain): EXPECTED TIMEOUT -- real restart confirmed, M2.7.4 semantics unchanged
Scenario F (existing safety invariants): PASS (idempotency intact, APPROVED != EXECUTED holds)
```
Exit code 0. One real defect was hit and fixed during this phase: the script's own `"$Key:ROLE"` PowerShell string interpolation was silently truncating to `"Key:"` (a documented PowerShell scope-qualifier parsing gotcha, `$var:text` inside a double-quoted string), causing every configured API key to be malformed and the server to correctly fail closed at startup (`ATLAS_API_KEYS` parse error, container crash-looping). This was a test-script bug, not a security-package bug — confirmed via `docker inspect`'s actual received env var value showing `operator1:,approver1:,...` (keys and roles missing). Fixed via `${Key}:ROLE` brace-delimited interpolation; re-run succeeded cleanly.

## 12. Unauthorized Docker restart proof

Scenario A: baseline `StartedAt=2026-08-27T16:53:39.230747277Z`, captured again after three separate unauthenticated attempts (plan/approve/execute) — unchanged. Scenario B (part 2) and Scenario C: same baseline value re-confirmed unchanged after each unauthorized attempt (`OPERATOR`/`APPROVER` respectively attempting execute). All three captured via `docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}'`, independent of any API response.

## 13. Authorized Docker restart proof

Scenario D: baseline `StartedAt=2026-08-27T16:53:39.230747277Z` (same value as the unauthorized attempts above — confirming those genuinely never touched the container), after the real authorized execute call: `StartedAt=2026-08-27T16:54:08.105766053Z`. Changed, independently confirmed via `docker inspect`, not the API's self-report alone.

## 14. Forged-identity test

Live, in Scenario D+E: authenticated as `executor1` (via `X-Atlas-Api-Key`), request body `{"actionId":"...","approver":"admin"}`. Resulting `ExecutionRecord.Approver` (returned in the API response and independently asserted): `"executor1"` — the forged `"admin"` claim had zero effect. The script explicitly asserts both `record.approver == "executor1"` and `record.approver != "admin"`. Mirrored at the unit level by `TestHandleExecute_ForgedApproverInBody_CannotOverrideAuthenticatedIdentity`, so this property is proven at both layers, not just one.

## 15. Frozen-path diff audit

```
git diff --stat -- internal/rca/                                    (empty)
git diff --stat -- internal/propagation/                             (empty)
git diff --stat -- internal/incidentmanager/correlator.go            (empty)
git diff --stat -- internal/incidentmanager/causal.go                (empty)
git diff --stat -- internal/incidentdetector/                        (empty)
git diff --stat -- internal/execution/guard.go                       (empty)
git diff --stat -- internal/execution/model.go                       (empty)
git diff --stat -- internal/execution/engine.go                      (empty)
git diff --stat -- internal/execution/verification.go                (empty)
git diff --stat -- internal/buffer/                                  (empty)
git diff --stat -- internal/event/                                   (empty)
git diff --stat -- internal/ingestion/                                (empty)
git diff --stat -- internal/infrastructure/docker/adapter.go          (empty)
git diff --stat -- internal/infrastructure/docker/adapter_windows.go  (empty)
git diff --stat -- test-m27-docker.ps1                                (empty)
git diff --stat -- test-m28-chaos.ps1                                 (empty)
```
Every protected path confirmed empty, checked directly against `git diff --stat` after implementation, not asserted from memory.

## 16. Build/vet/test/race results

`go build ./...` — clean. `go vet ./...` — clean. `go test ./...` — clean, all packages including new `internal/security` and the newly-tested `internal/httpapi`, plus the full unmodified M2.1-M2.8 suite. `go test -race ./...` (Linux container, `golang:1.25`) — clean, same full scope, no data races.

## 17. Known limitations

- Read endpoints (`VIEW`/`READ_AUDIT`) are not enforced in this milestone — deliberate, documented scope boundary (see §8), not an oversight.
- No separation-of-duties enforcement (explicitly out of scope, per instruction).
- No `CreatedBy` field (investigated, deliberately omitted — see §7).
- Static API keys have no expiry/rotation mechanism — acceptable for this project's actual scale, per the original investigation's own principle against over-engineering.
- Docker socket exposure and network segmentation remain out of scope, as before.

## 18. What was NOT changed

`internal/rca/`, `internal/propagation/`, `internal/incidentmanager/correlator.go`, `internal/incidentmanager/causal.go`, `internal/incidentdetector/`, `internal/execution/guard.go`, `internal/execution/model.go`, `internal/execution/engine.go`, `internal/execution/verification.go`, `internal/buffer/`, `internal/event/`, `internal/ingestion/`, both Docker adapter files, `test-m27-docker.ps1`, `test-m28-chaos.ps1`. `execution.Engine.ExecutePlanAction`'s signature is unchanged — its existing `approver string` parameter already gave the HTTP layer everywhere it needed to inject a trustworthy value without touching the frozen `execution` package at all.

## 19. Deviations / unexpected findings

- The PowerShell interpolation bug in §11 — not a deviation from the plan, but worth flagging as the one real bug hit during implementation, and instructive: it independently demonstrates the fail-closed behavior of `ParseAPIKeys` working exactly as designed (malformed config → hard startup error, not a silent, insecure fallback).
- No frozen component required modification — the investigation's prediction (§16 of the investigation report) held exactly: `internal/remediation/model.go` and `planner.go` were the only "frozen-adjacent" files reopened, both via the explicitly pre-approved, minimal Option 2.

## 20. Exact commit-ready file summary

**NEW:**
- `services/intelligence-engine/internal/security/{model,keystore,context,middleware}.go` + matching `_test.go` files
- `services/intelligence-engine/internal/httpapi/{remediation_test.go,execution_test.go}`
- `test-m29-security.ps1`
- `docs/m29_verification_report.md`

**MODIFIED:**
- `services/intelligence-engine/cmd/intelligence-engine/main.go` — Authorizer construction + route protection wiring
- `services/intelligence-engine/internal/httpapi/remediation.go` — `ApprovedBy` sourced from context
- `services/intelligence-engine/internal/httpapi/execution.go` — executor identity sourced from context
- `services/intelligence-engine/internal/remediation/model.go` — `ApprovalMetadata.ApprovedBy` field
- `services/intelligence-engine/internal/remediation/planner.go` — `ApprovePlan` gains `approvedBy` parameter
- `docker-compose.yml` — `ATLAS_SECURITY_ENABLED`, `ATLAS_API_KEYS` passthrough

**UNTOUCHED (frozen, confirmed):** every path listed in §15.

## Commit status
**Nothing has been committed.** Awaiting your review of this report and the diff.
