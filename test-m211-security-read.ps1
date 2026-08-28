param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# ============================================================================
# M2.11 -- Read-surface RBAC E2E validation.
#
# Deliberately a new, isolated script -- test-m27-docker.ps1,
# test-m28-chaos.ps1, and test-m29-security.ps1 are none of them modified by
# this milestone. This script is the only one that exercises the read-only
# RBAC boundary M2.11 wires up (PermissionView / PermissionReadAudit on the
# GET routes); it reuses test-m29-security.ps1's established
# Invoke-Authenticated / Invoke-Unauthenticated / Wait-ForAllHealthy helpers
# and Docker lifecycle pattern.
#
# API keys are test-only, generated fresh for this run, never committed.
# The header used is X-Atlas-Api-Key (security.APIKeyHeader).
# ============================================================================

$ApiKeyHeader = "X-Atlas-Api-Key"
$OperatorKey  = "test-operator-key-$(New-Guid)"
$ApproverKey  = "test-approver-key-$(New-Guid)"
$ExecutorKey  = "test-executor-key-$(New-Guid)"
$ViewerKey    = "test-viewer-key-$(New-Guid)"

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

function Invoke-Unauthenticated {
    param([string]$Uri, [string]$Method = "GET")
    try {
        $body = Invoke-RestMethod -Uri $Uri -Method $Method -UseBasicParsing -ErrorAction Stop
        return @{ Status = 200; Body = $body }
    } catch {
        return @{ Status = $_.Exception.Response.StatusCode.value__; Body = $null }
    }
}

function Invoke-Authenticated {
    param([string]$Uri, [string]$Method = "GET", [string]$Key, [string]$Body)
    $headers = @{ $ApiKeyHeader = $Key }
    try {
        if ($Body) {
            $resp = Invoke-RestMethod -Uri $Uri -Method $Method -Headers (@{"Content-Type"="application/json"} + $headers) -Body $Body -UseBasicParsing -ErrorAction Stop
        } else {
            $resp = Invoke-RestMethod -Uri $Uri -Method $Method -Headers $headers -UseBasicParsing -ErrorAction Stop
        }
        return @{ Status = 200; Body = $resp }
    } catch {
        return @{ Status = $_.Exception.Response.StatusCode.value__; Body = $null }
    }
}

function Trigger-PaymentCascade {
    param([string]$Tag)
    for ($i = 0; $i -lt 5; $i++) {
        try { Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M211-$Tag-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null } catch {}
    }
    for ($i = 0; $i -lt 15; $i++) {
        try { Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M211-$Tag-PAYFAIL-$i"} -Body '{"productId":"P200","quantity":4}' -TimeoutSec 5 | Out-Null } catch {}
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

# ============================================================================
# PART 1: security disabled compatibility (ATLAS_SECURITY_ENABLED unset,
# default false). Byte-for-byte pre-M2.11 behavior -- read endpoints remain
# open with no key, exactly as before this milestone.
# ============================================================================
try {

Write-Host "--- Bringing up a clean, isolated Docker state (security DISABLED, default) ---"
docker-compose down
if (-not $SkipBuild) { docker-compose up -d --build } else { docker-compose up -d }
Write-Host "Waiting for all services to be healthy..."
Wait-ForAllHealthy
Write-Host "Clean state confirmed."

Write-Host ""
Write-Host "=== PART 1: security disabled -- read endpoints remain unauthenticated (unchanged pre-M2.11 behavior) ==="
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/incidents"
if ($r.Status -ne 200) { Write-Error "PART 1: expected 200 for GET /api/v1/incidents with security disabled, got $($r.Status) -- REGRESSION." }
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/events"
if ($r.Status -ne 200) { Write-Error "PART 1: expected 200 for GET /api/v1/events with security disabled, got $($r.Status) -- REGRESSION." }
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/graph"
if ($r.Status -ne 200) { Write-Error "PART 1: expected 200 for GET /api/v1/graph with security disabled, got $($r.Status) -- REGRESSION." }
Write-Host "PASS: read endpoints remain fully open when ATLAS_SECURITY_ENABLED is unset -- pre-M2.11 behavior unchanged."
$results += "Part 1 (security-disabled compatibility): PASS (200 on reads, no key required)"

} finally {
    Write-Host ""
    Write-Host "--- Tearing down Part 1 Docker state ---"
    docker-compose down
}

# ============================================================================
# PART 2: security enabled -- the actual read-surface RBAC boundary.
# ============================================================================
$env:ATLAS_SECURITY_ENABLED = "true"
$env:ATLAS_API_KEYS = "operator1:${OperatorKey}:OPERATOR,approver1:${ApproverKey}:APPROVER,executor1:${ExecutorKey}:EXECUTOR,viewer1:${ViewerKey}:VIEWER"
$env:ATLAS_EXECUTION_ENABLED = "true"
$env:ATLAS_EXECUTION_PROVIDER = "docker"

try {

Write-Host ""
Write-Host "--- Bringing up a clean, isolated Docker state for PART 2 (security ENABLED) ---"
docker-compose down
if (-not $SkipBuild) { docker-compose up -d --build } else { docker-compose up -d }
Write-Host "Waiting for all services to be healthy..."
Wait-ForAllHealthy
Write-Host "Clean state confirmed."
# Actuator "UP" (Wait-ForAllHealthy's signal) is not proof the JVM's downstream
# HTTP connection pools to its peers are warmed up yet; observed directly this
# session (both here and reproducing test-m29-security.ps1's identical,
# pre-existing SETUP flakiness unmodified) -- firing the fault-cascade
# immediately after health can silently lose the very first requests to
# connection-pool warmup. A short buffer here is cheaper than a flaky retry.
Start-Sleep -Seconds 15

# ----------------------------------------------------------------------
# B. Unauthenticated read -> 401, no protected data returned.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO B: unauthenticated read ==="
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/incidents"
if ($r.Status -ne 401) { Write-Error "SCENARIO B: expected 401 for unauthenticated GET /api/v1/incidents, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
if ($r.Body -ne $null) { Write-Error "SCENARIO B: unauthenticated request unexpectedly returned a body -- protected data may have leaked." }
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/events"
if ($r.Status -ne 401) { Write-Error "SCENARIO B: expected 401 for unauthenticated GET /api/v1/events, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/graph"
if ($r.Status -ne 401) { Write-Error "SCENARIO B: expected 401 for unauthenticated GET /api/v1/graph, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Unauthenticated -Uri "http://localhost:8081/api/v1/executions/does-not-matter"
if ($r.Status -ne 401) { Write-Error "SCENARIO B: expected 401 for unauthenticated GET /api/v1/executions/{id}, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: unauthenticated reads rejected with 401 across incidents/events/graph/executions, no data leaked."
$results += "Scenario B (unauthenticated read): PASS (401, no data returned)"

# ----------------------------------------------------------------------
# Setup: generate a real incident + approved plan so VIEWER/READ_AUDIT
# scenarios have real data to read.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SETUP: generate an incident + approved, executed plan (target for read scenarios) ==="
$incident = Trigger-PaymentCascade -Tag "M211"
if ($incident -eq $null) { Write-Error "SETUP: no payment-service incident appeared -- INFRASTRUCTURE FAILURE." }
Write-Host "Incident: $($incident.incidentId)"

$planStatus = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incident.incidentId)/remediation/plan" -Method POST -Key $OperatorKey
if ($planStatus.Status -ne 200) { Write-Error "SETUP: OPERATOR could not generate a plan (status=$($planStatus.Status)); cannot proceed." }
$plan = (Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incident.incidentId)/remediation" -Method GET -Key $OperatorKey).Body
$planId = $plan.planId
Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Key $ApproverKey -Body '{"reason":"M2.11 read-surface setup"}' | Out-Null
$planCheck = (Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId" -Method GET -Key $OperatorKey).Body
$actionId = $planCheck.actions[0].actionId
$execRecord = (Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Key $ExecutorKey -Body "{`"actionId`":`"$actionId`"}").Body
if ($execRecord.executionStatus -ne "EXECUTED") { Write-Error "SETUP: execution did not succeed: $($execRecord.message) -- cannot proceed to READ_AUDIT scenarios." }
$executionId = $execRecord.executionId
Write-Host "Setup complete: plan=$planId action=$actionId execution=$executionId"

# ----------------------------------------------------------------------
# C. VIEWER read access -- normal operational reads allowed.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO C: VIEWER read access ==="
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents" -Key $ViewerKey
if ($r.Status -ne 200) { Write-Error "SCENARIO C: expected 200 for VIEWER GET /api/v1/incidents, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incident.incidentId)" -Key $ViewerKey
if ($r.Status -ne 200) { Write-Error "SCENARIO C: expected 200 for VIEWER GET incident detail, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/graph" -Key $ViewerKey
if ($r.Status -ne 200) { Write-Error "SCENARIO C: expected 200 for VIEWER GET /api/v1/graph, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId" -Key $ViewerKey
if ($r.Status -ne 200) { Write-Error "SCENARIO C: expected 200 for VIEWER GET plan, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: VIEWER can read incidents/graph/plans -- normal operational visibility works."
$results += "Scenario C (VIEWER read access): PASS (200 on incidents/graph/plan reads)"

# ----------------------------------------------------------------------
# D. Audit-endpoint restriction. NOTE: per the existing, unmodified M2.9
# role matrix (internal/security/model.go), RoleViewer already includes
# PermissionReadAudit -- VIEWER is therefore ALLOWED on audit endpoints
# too, not blocked. The role actually excluded from PermissionReadAudit is
# OPERATOR (View + CreatePlan only). M2.11 does not modify the role
# matrix, so this scenario tests the real, existing boundary (OPERATOR
# blocked) rather than asserting a VIEWER restriction the shipped role
# matrix does not define. See docs/m211_verification_report.md Findings.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO D: audit-endpoint restriction (OPERATOR lacks READ_AUDIT) ==="
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/executions/$executionId" -Key $OperatorKey
if ($r.Status -ne 403) { Write-Error "SCENARIO D: expected 403 for OPERATOR GET /api/v1/executions/{id}, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incident.incidentId)/executions" -Key $OperatorKey
if ($r.Status -ne 403) { Write-Error "SCENARIO D: expected 403 for OPERATOR GET incident executions, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: OPERATOR (View+CreatePlan only, no READ_AUDIT) blocked from execution/audit endpoints with 403."
$results += "Scenario D (audit-endpoint restriction): PASS (403 for OPERATOR, which lacks READ_AUDIT)"

# ----------------------------------------------------------------------
# E. READ_AUDIT access -- EXECUTOR (has READ_AUDIT) can read audit data.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO E: READ_AUDIT access (EXECUTOR) ==="
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/executions/$executionId" -Key $ExecutorKey
if ($r.Status -ne 200) { Write-Error "SCENARIO E: expected 200 for EXECUTOR GET /api/v1/executions/{id}, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
if ($r.Body.executionId -ne $executionId) { Write-Error "SCENARIO E: unexpected execution record body." }
$rViewer = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/executions/$executionId" -Key $ViewerKey
if ($rViewer.Status -ne 200) { Write-Error "SCENARIO E: expected 200 for VIEWER GET /api/v1/executions/{id} (VIEWER role includes READ_AUDIT per the existing M2.9 role matrix), got $($rViewer.Status)." }
Write-Host "PASS: EXECUTOR and VIEWER (both hold READ_AUDIT) can read execution/audit records."
$results += "Scenario E (READ_AUDIT access): PASS (200 for EXECUTOR and VIEWER)"

# ----------------------------------------------------------------------
# F. VIEWER mutation restriction -- still blocked from approve/execute.
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO F: VIEWER mutation restriction ==="
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$($incident.incidentId)/remediation/plan" -Method POST -Key $ViewerKey
if ($r.Status -ne 403) { Write-Error "SCENARIO F: expected 403 for VIEWER plan generation, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Key $ViewerKey -Body '{"reason":"should be blocked"}'
if ($r.Status -ne 403) { Write-Error "SCENARIO F: expected 403 for VIEWER approval, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
$r = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Key $ViewerKey -Body "{`"actionId`":`"$actionId`"}"
if ($r.Status -ne 403) { Write-Error "SCENARIO F: expected 403 for VIEWER execute, got $($r.Status) -- PRODUCT/ARCHITECTURE FAILURE." }
Write-Host "PASS: VIEWER (View+READ_AUDIT only) remains blocked from create-plan/approve/execute -- M2.9 mutation boundary unaffected by M2.11."
$results += "Scenario F (VIEWER mutation restriction): PASS (403 on create-plan/approve/execute)"

# ----------------------------------------------------------------------
# G. Minimal regression check: existing authorized mutation chain
# (M2.9's own suite covers this exhaustively; this just confirms M2.11's
# routing changes did not disturb it).
# ----------------------------------------------------------------------
Write-Host ""
Write-Host "=== SCENARIO G: minimal regression check (existing authorized mutation chain) ==="
$planFinal = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$planId" -Key $OperatorKey
if ($planFinal.Status -ne 200 -or $planFinal.Body.status -ne "APPROVED") { Write-Error "SCENARIO G: expected plan to remain APPROVED (status=200, plan.status=APPROVED), got status=$($planFinal.Status) plan.status=$($planFinal.Body.status) -- APPROVED != EXECUTED invariant or read wiring regressed." }
if ($planFinal.Body.approval.approvedBy -ne "approver1") { Write-Error "SCENARIO G: expected approval.approvedBy=approver1, got $($planFinal.Body.approval.approvedBy) -- M2.9 identity wiring regressed." }
Write-Host "PASS: M2.9's authorized mutation chain (approve/execute + identity binding) is unaffected by M2.11's read-route wiring."
$results += "Scenario G (M2.9 regression check): PASS (plan.status=APPROVED, approvedBy correctly recorded)"

Write-Host ""
Write-Host "============================================================"
Write-Host "M2.11 READ-SURFACE RBAC SUITE SUMMARY"
Write-Host "============================================================"
foreach ($r in $results) { Write-Host $r }
Write-Host ""
Write-Host "Test completed."

} finally {
    Write-Host ""
    Write-Host "--- Tearing down Docker state (runs on success or failure) ---"
    docker-compose down
}
