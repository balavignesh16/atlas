param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# ============================================================================
# M2.9 -- Security / Authorization Boundary E2E validation.
#
# Deliberately separate from test-m27-docker.ps1 and test-m28-chaos.ps1,
# neither of which is modified by this milestone. Those scripts run with
# ATLAS_SECURITY_ENABLED unset (default false), so the new authentication/
# authorization middleware is a pure no-op for them -- this script is the
# only one that sets ATLAS_SECURITY_ENABLED=true and exercises the new
# boundary.
#
# API keys are test-only, generated fresh for this run, never committed --
# see ATLAS_API_KEYS below. The header used is X-Atlas-Api-Key
# (security.APIKeyHeader).
# ============================================================================

$ApiKeyHeader = "X-Atlas-Api-Key"
$OperatorKey  = "test-operator-key-$(New-Guid)"
$ApproverKey  = "test-approver-key-$(New-Guid)"
$ExecutorKey  = "test-executor-key-$(New-Guid)"
$ViewerKey    = "test-viewer-key-$(New-Guid)"

$env:ATLAS_SECURITY_ENABLED = "true"
$env:ATLAS_API_KEYS = "operator1:${OperatorKey}:OPERATOR,approver1:${ApproverKey}:APPROVER,executor1:${ExecutorKey}:EXECUTOR,viewer1:${ViewerKey}:VIEWER"
$env:ATLAS_EXECUTION_ENABLED = "true"
$env:ATLAS_EXECUTION_PROVIDER = "docker"

function Wait-ForAllHealthy {
    param([int]$MaxRetries = 60)
    $endpoints = @(
        "http://localhost:8083/actuator/health",
        "http://localhost:8084/actuator/health",
        "http://localhost:8085/actuator/health",
        "http://localhost:8086/actuator/health",
        "http://localhost:8081/health"
    )
    foreach ($url in $endpoints) {
        $healthy = $false
        $retries = 0
        while (-not $healthy -and $retries -lt $MaxRetries) {
            try {
                $resp = Invoke-RestMethod -Uri $url -UseBasicParsing -ErrorAction Stop
                if ($url -like "*actuator*") { if ($resp.status -eq "UP") { $healthy = $true } } else { $healthy = $true }
            } catch {
                Start-Sleep -Seconds 2
                $retries++
            }
        }
        if (-not $healthy) { Write-Error "Endpoint did not become healthy in time: $url" }
    }
}

Write-Host "--- Bringing up a clean, isolated Docker state for M2.9 (security enabled) ---"
docker-compose down
if (-not $SkipBuild) { docker-compose up -d --build } else { docker-compose up -d }
Write-Host "Waiting for all services to be healthy..."
Wait-ForAllHealthy
Write-Host "Clean state confirmed."

function Wait-ForVerificationOutcome {
    param([string]$ExecutionId, [int]$MaxRetries = 20, [int]$SleepSeconds = 2)
    $status = "VERIFYING"
    $retries = 0
    while (($status -eq "VERIFYING" -or $status -eq "PENDING") -and $retries -lt $MaxRetries) {
        Start-Sleep -Seconds $SleepSeconds
        $check = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/executions/$ExecutionId" -UseBasicParsing
        $status = $check.verificationStatus
        $retries++
    }
    return $status
}

function Invoke-Unauthenticated {
    param([string]$Uri, [string]$Method, [string]$Body)
    try {
        if ($Body) {
            Invoke-RestMethod -Uri $Uri -Method $Method -Headers @{"Content-Type"="application/json"} -Body $Body -UseBasicParsing -ErrorAction Stop | Out-Null
        } else {
            Invoke-RestMethod -Uri $Uri -Method $Method -UseBasicParsing -ErrorAction Stop | Out-Null
        }
        return $null
    } catch {
        return $_.Exception.Response.StatusCode.value__
    }
}

function Invoke-Authenticated {
    param([string]$Uri, [string]$Method, [string]$Key, [string]$Body)
    $headers = @{"Content-Type"="application/json"; $ApiKeyHeader = $Key}
    try {
        if ($Body) {
            Invoke-RestMethod -Uri $Uri -Method $Method -Headers $headers -Body $Body -UseBasicParsing -ErrorAction Stop | Out-Null
        } else {
            Invoke-RestMethod -Uri $Uri -Method $Method -Headers $headers -UseBasicParsing -ErrorAction Stop | Out-Null
        }
        return $null
    } catch {
        return $_.Exception.Response.StatusCode.value__
    }
}

function Trigger-PaymentCascade {
    param([string]$Tag)
    for ($i = 0; $i -lt 5; $i++) {
        try { Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M29-$Tag-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null } catch {}
    }
    for ($i = 0; $i -lt 15; $i++) {
        try { Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M29-$Tag-PAYFAIL-$i"} -Body '{"productId":"P200","quantity":4}' -TimeoutSec 5 | Out-Null } catch {}
    }
    $incident = $null
    $retries = 0
    while ($incident -eq $null -and $retries -lt 30) {
        Start-Sleep -Seconds 5
        try {
            $incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
            foreach ($inc in $incidents) {
                if ($inc.rootService -eq "atlas-payment-service" -and $inc.primaryIncidentId -eq $inc.incidentId) { $incident = $inc; break }
            }
        } catch {}
        $retries++
    }
    return $incident
}

$results = @()

# The whole scenario body runs inside try/finally so Docker state is always
# torn down -- on success or on an early Write-Error termination (this
# script sets $ErrorActionPreference="Stop", so any Write-Error below exits
# immediately unless this covers it). Without this, a failed run leaves
# ATLAS_SECURITY_ENABLED=true containers running, which could confusingly
# affect a subsequent test-m27-docker.ps1/test-m28-chaos.ps1 -SkipBuild run.
try {

# ============================================================================
# Setup: a legitimate OPERATOR-generated, APPROVER-approved plan used as the
# fixed "target" for the negative execute tests in Scenarios A/B/C below.
# (Read endpoints remain unauthenticated in this milestone -- see
# docs/m29_verification_report.md -- so incident lookups below use no key.)
# ============================================================================
Write-Host ""
Write-Host "=== SETUP: legitimate plan generation + approval (target for negative tests) ==="
$incidentA = Trigger-PaymentCascade -Tag "TARGET"
if ($incidentA -eq $null) { Write-Error "SETUP: no payment-service incident appeared -- INFRASTRUCTURE FAILURE." }
Write-Host "Incident A: $($incidentA.incidentId) (confidence=$($incidentA.rootCause.confidence) score=$($incidentA.rootCause.score))"

$planAStatus = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incidentA.incidentId)/remediation/plan" -Method POST -Key $OperatorKey
if ($planAStatus -ne $null) {
    Write-Host "SETUP: OPERATOR could not generate a plan this run (status=$planAStatus, likely RCA confidence -- pre-existing, unrelated variability). Re-run the script if this repeats."
    Write-Error "SETUP: cannot proceed without a plan to approve."
}
$planA = (Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($incidentA.incidentId)/remediation" -UseBasicParsing)
$planAId = $planA.planId
Write-Host "Plan A generated by OPERATOR: $planAId"

# ----------------------------------------------------------------------
# Scenario B (part 1): OPERATOR (lacks APPROVE_PLAN) attempts to approve.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO B (part 1): OPERATOR attempts approval (unauthorized) ==="
$status = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/approve" -Method POST -Key $OperatorKey -Body '{"reason":"should be blocked"}'
if ($status -ne 403) { Write-Error "SCENARIO B: expected 403 for OPERATOR attempting approval, got $status -- PRODUCT/ARCHITECTURE FAILURE." }
$planACheck = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planAId" -UseBasicParsing
if ($planACheck.status -eq "APPROVED") { Write-Error "SCENARIO B: plan A became approved despite an unauthorized approval attempt -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: OPERATOR blocked from approving (403), plan A remains status=$($planACheck.status)."
$results += "Scenario B (part 1 -- unauthorized approval): PASS (403, plan not approved)"

# Now legitimately approve plan A with APPROVER, to set up the execute-target.
Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/approve" -Method POST -Key $ApproverKey -Body '{"reason":"legitimate setup approval"}' | Out-Null
$planACheck = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planAId" -UseBasicParsing
if ($planACheck.status -ne "APPROVED") { Write-Error "SETUP: expected plan A to be APPROVED after a legitimate APPROVER call, got $($planACheck.status)." }
if ($planACheck.approval.approvedBy -ne "approver1") { Write-Error "SETUP: expected approval.approvedBy='approver1' (authenticated identity), got '$($planACheck.approval.approvedBy)'." }
Write-Host "Plan A legitimately approved by approver1 (approval.approvedBy correctly recorded as the authenticated principal, not any client-supplied value)."

$planAActionId = $planACheck.actions[0].actionId

# ============================================================================
# SCENARIO A -- Unauthenticated remediation.
# ============================================================================
Write-Host ""
Write-Host "=== SCENARIO A: unauthenticated remediation ==="
$baselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
Write-Host "Baseline atlas-payment-service-1 StartedAt: $baselineStartedAt"

$status = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/execute" -Method POST -Body "{`"actionId`":`"$planAActionId`"}"
if ($status -ne 401) { Write-Error "SCENARIO A: expected 401 for unauthenticated execute, got $status -- PRODUCT/ARCHITECTURE FAILURE." }

$status = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/incidents/$($incidentA.incidentId)/remediation/plan" -Method POST -Body $null
if ($status -ne 401) { Write-Error "SCENARIO A: expected 401 for unauthenticated plan generation, got $status -- PRODUCT/ARCHITECTURE FAILURE." }

$status = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/approve" -Method POST -Body '{"reason":"unauthenticated"}'
if ($status -ne 401) { Write-Error "SCENARIO A: expected 401 for unauthenticated approval, got $status -- PRODUCT/ARCHITECTURE FAILURE." }

$afterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
if ($afterStartedAt -ne $baselineStartedAt) { Write-Error "SCENARIO A: StartedAt changed despite every attempt being unauthenticated -- INFRASTRUCTURE/PRODUCT FAILURE (real restart occurred without authentication)." }
Write-Host "PASS: all three unauthenticated attempts rejected with 401; atlas-payment-service-1 StartedAt independently confirmed unchanged ($afterStartedAt)."
$results += "Scenario A (unauthenticated remediation): PASS (401 on plan/approve/execute, StartedAt unchanged)"

# ============================================================================
# SCENARIO C -- Unauthorized execution (APPROVER lacks EXECUTE).
# ============================================================================
Write-Host ""
Write-Host "=== SCENARIO C: unauthorized execution (authenticated APPROVER, no EXECUTE permission) ==="
$baselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()

$status = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/execute" -Method POST -Key $ApproverKey -Body "{`"actionId`":`"$planAActionId`"}"
if ($status -ne 403) { Write-Error "SCENARIO C: expected 403 for APPROVER attempting execute, got $status -- PRODUCT/ARCHITECTURE FAILURE." }

$afterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
if ($afterStartedAt -ne $baselineStartedAt) { Write-Error "SCENARIO C: StartedAt changed despite an authorization rejection -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: APPROVER blocked from executing (403); StartedAt independently confirmed unchanged ($afterStartedAt)."
$results += "Scenario C (unauthorized execution): PASS (403, StartedAt unchanged)"

# ----------------------------------------------------------------------
# Scenario B (part 2): OPERATOR (lacks EXECUTE either) attempts to execute
# the same already-approved plan A.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO B (part 2): OPERATOR attempts execution (unauthorized) ==="
$baselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
$status = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planAId/execute" -Method POST -Key $OperatorKey -Body "{`"actionId`":`"$planAActionId`"}"
if ($status -ne 403) { Write-Error "SCENARIO B: expected 403 for OPERATOR attempting execute, got $status -- PRODUCT/ARCHITECTURE FAILURE." }
$afterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
if ($afterStartedAt -ne $baselineStartedAt) { Write-Error "SCENARIO B: StartedAt changed despite an authorization rejection -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: OPERATOR blocked from executing (403); StartedAt independently confirmed unchanged ($afterStartedAt)."
$results += "Scenario B (part 2 -- unauthorized execution): PASS (403, StartedAt unchanged)"

# ============================================================================
# SCENARIO D + E -- Legitimate full chain, combined with the mandatory
# forged-identity proof on the same execute call.
# ============================================================================
Write-Host ""
Write-Host "=== SCENARIO D+E: legitimate authorized chain, with a forged approver identity on the execute call ==="
$incidentB = Trigger-PaymentCascade -Tag "LEGIT"
if ($incidentB -eq $null) { Write-Error "SCENARIO D: no payment-service incident appeared -- INFRASTRUCTURE FAILURE." }
Write-Host "Incident B: $($incidentB.incidentId) (confidence=$($incidentB.rootCause.confidence) score=$($incidentB.rootCause.score))"

$status = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incidentB.incidentId)/remediation/plan" -Method POST -Key $OperatorKey
if ($status -ne $null) { Write-Error "SCENARIO D: OPERATOR plan generation failed unexpectedly, status=$status." }
$planB = (Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($incidentB.incidentId)/remediation" -UseBasicParsing)
$planBId = $planB.planId
Write-Host "Plan B generated by OPERATOR: $planBId"

Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planBId/approve" -Method POST -Key $ApproverKey -Body '{"reason":"legitimate M2.9 chain"}' | Out-Null
$planBCheck = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planBId" -UseBasicParsing
if ($planBCheck.status -ne "APPROVED") { Write-Error "SCENARIO D: expected plan B APPROVED, got $($planBCheck.status)." }
if ($planBCheck.approval.approvedBy -ne "approver1") { Write-Error "SCENARIO D: expected approval.approvedBy=approver1, got $($planBCheck.approval.approvedBy)." }
Write-Host "Plan B approved by approver1 (approvedBy correctly recorded)."

$planBActionId = $planBCheck.actions[0].actionId
$baselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
Write-Host "Baseline atlas-payment-service-1 StartedAt: $baselineStartedAt"

# The forged-identity proof: authenticated as executor1, body claims
# approver=admin. The trusted, recorded identity must be executor1.
$execBody = "{`"actionId`":`"$planBActionId`",`"approver`":`"admin`"}"
$execRecord = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planBId/execute" -Method POST -Headers @{"Content-Type"="application/json"; $ApiKeyHeader = $ExecutorKey} -Body $execBody -UseBasicParsing

Write-Host "Execution Status: $($execRecord.executionStatus)"
if ($execRecord.executionStatus -ne "EXECUTED") { Write-Error "SCENARIO D: execution did not succeed: $($execRecord.message) / $($execRecord.error) -- PRODUCT/ARCHITECTURE FAILURE." }

if ($execRecord.approver -ne "executor1") {
    Write-Error "SCENARIO E: SECURITY FAILURE -- forged body approver='admin' was not overridden by the authenticated identity. Expected 'executor1', got '$($execRecord.approver)'."
}
if ($execRecord.approver -eq "admin") {
    Write-Error "SCENARIO E: SECURITY FAILURE -- the forged identity 'admin' was trusted and recorded."
}
Write-Host "PASS: forged body approver='admin' correctly ignored; trusted recorded identity is '$($execRecord.approver)' (the authenticated principal)."
$results += "Scenario E (forged approver identity): PASS (authenticated identity 'executor1' recorded, forged 'admin' ignored)"

$afterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
if ($afterStartedAt -eq $baselineStartedAt) { Write-Error "SCENARIO D: independent Docker restart proof FAILED -- StartedAt did not change." }
Write-Host "Independent Docker restart proof: StartedAt changed from $baselineStartedAt to $afterStartedAt"

$verifStatus = Wait-ForVerificationOutcome -ExecutionId $execRecord.executionId
Write-Host "Final Verification Status: $verifStatus"
switch ($verifStatus) {
    "VERIFIED" { $dOutcome = "PASS -- real restart, verified RESOLVED" }
    "VERIFICATION_TIMEOUT" { $dOutcome = "EXPECTED TIMEOUT -- real restart confirmed, M2.7.4 semantics unchanged" }
    default { Write-Error "SCENARIO D: unexpected verification status $verifStatus -- PRODUCT/ARCHITECTURE FAILURE." }
}
Write-Host "SCENARIO D RESULT: $dOutcome"
$results += "Scenario D (legitimate authorized chain): $dOutcome (real Docker restart independently verified)"

# ============================================================================
# SCENARIO F -- Existing safety invariants: idempotency, APPROVED != EXECUTED.
# ============================================================================
Write-Host ""
Write-Host "=== SCENARIO F: existing safety invariants (idempotency, APPROVED != EXECUTED) ==="

$execRecord2 = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planBId/execute" -Method POST -Headers @{"Content-Type"="application/json"; $ApiKeyHeader = $ExecutorKey} -Body "{`"actionId`":`"$planBActionId`"}" -UseBasicParsing
if ($execRecord2.executionId -ne $execRecord.executionId) { Write-Error "SCENARIO F: idempotency broken -- a second execute call against the same (planId,actionId) produced a new executionId." }
$startedAtAfterSecondCall = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
if ($startedAtAfterSecondCall -ne $afterStartedAt) { Write-Error "SCENARIO F: a duplicate execute call caused a second real restart -- idempotency FAILURE." }
Write-Host "PASS: duplicate execute call returned the same executionId, no second restart occurred (idempotency intact)."

$planBFinal = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planBId" -UseBasicParsing
if ($planBFinal.status -ne "APPROVED") { Write-Error "SCENARIO F: plan.Status unexpectedly changed to '$($planBFinal.status)' -- APPROVED != EXECUTED invariant violated (plan status must remain APPROVED; execution outcome lives only in the execution record)." }
Write-Host "PASS: plan B's Status remains APPROVED after execution -- APPROVED != EXECUTED holds (execution outcome tracked only in the execution record, per the unmodified M2.7 architecture)."
$results += "Scenario F (existing safety invariants): PASS (idempotency intact, APPROVED != EXECUTED holds)"

Write-Host ""
Write-Host "============================================================"
Write-Host "M2.9 SECURITY SUITE SUMMARY"
Write-Host "============================================================"
foreach ($r in $results) { Write-Host $r }
Write-Host ""
Write-Host "Test completed."

} finally {
    Write-Host ""
    Write-Host "--- Tearing down Docker state (runs on success or failure) ---"
    docker-compose down
}
