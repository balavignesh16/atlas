param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# ============================================================================
# M2.15 -- Full-Chain End-to-End Validation.
#
# Closes the roadmap's last-named gap (docs/roadmap-checklist.md SS3): "Final
# end-to-end validation script covering the full chaos -> detect -> RCA -> AI
# -> plan -> approve -> execute -> verify chain." Ten prior scripts each
# proved one milestone's slice of this chain in isolation; this is the first
# to walk every stage in one continuous run and report the REAL, honest
# result of each one.
#
# This script is deliberately separate from every prior test-m2*.ps1 script,
# NONE of which are modified here. It composes only already-proven
# mechanisms:
#   - the gateway-cascade fault trigger (P100 baseline, then P200/qty=4),
#     used by test-m27-docker.ps1 Scenario A/D and test-m28-chaos.ps1
#     Scenario 1/3 -- the one path that can reach M2.7.3's causal-attribution
#     MEDIUM confidence boost and clear M2.6's policy gate.
#   - Wait-ForAllHealthy, exactly as in test-m28-chaos.ps1/test-m29-
#     security.ps1/test-m211-security-read.ps1.
#   - Wait-ForVerificationOutcome, exactly as in test-m27-docker.ps1/
#     test-m28-chaos.ps1.
#   - the API-key / role / Invoke-Authenticated pattern from
#     test-m29-security.ps1/test-m211-security-read.ps1, so this script also
#     genuinely exercises the M2.9/M2.11 authenticated approval flow rather
#     than relying on the ATLAS_SECURITY_ENABLED=false default the pure
#     pipeline scripts use.
#
# Two stages no prior script exercises at all are added here, both read-only
# probes of already-existing, already-frozen behavior -- no new production
# code required either one to exist:
#   - POST /api/v1/incidents/{id}/analyze (M2.5's AI reasoning entry point),
#     asserting the response's provider/model fields literally equal
#     "fake"/"fake-model" -- genuine proof FakeProvider ran, not an
#     assumption.
#   - The generated plan's planner/plannerVersion fields, asserted to
#     literally equal "fake"/"1.0" -- genuine proof FakePlanner ran.
#
# HONESTY MODEL (see the Module 3 spec this script implements): each stage is
# recorded as exactly one of PASS / BLOCKED / FAILED / NOT_REACHED.
#   PASS         -- the stage genuinely completed.
#   BLOCKED      -- a legitimate system gate (M2.6 confidence policy) correctly
#                   prevented the next stage. This is a valid, safe outcome,
#                   not a defect, and does NOT fail the script.
#   FAILED       -- the stage should have completed but did not -- a genuine
#                   defect or infrastructure problem. DOES fail the script
#                   (Write-Error, non-zero exit), exactly like every prior
#                   test-m2*.ps1 script's own convention for unexpected
#                   outcomes.
#   NOT_REACHED  -- a later stage was never attempted because an earlier
#                   stage legitimately BLOCKED the chain.
# No gate is ever bypassed, no backend behavior is ever weakened, and no
# outcome is ever converted from BLOCKED to PASS to make the chain look more
# complete than it actually is.
# ============================================================================

$ApiKeyHeader = "X-Atlas-Api-Key"
$OperatorKey  = "test-m215-operator-key-$(New-Guid)"
$ApproverKey  = "test-m215-approver-key-$(New-Guid)"
$ExecutorKey  = "test-m215-executor-key-$(New-Guid)"

$env:ATLAS_SECURITY_ENABLED  = "true"
$env:ATLAS_API_KEYS          = "operator-m215:${OperatorKey}:OPERATOR,approver-m215:${ApproverKey}:APPROVER,executor-m215:${ExecutorKey}:EXECUTOR"
$env:ATLAS_EXECUTION_ENABLED = "true"
$env:ATLAS_EXECUTION_PROVIDER = "docker"

# ----------------------------------------------------------------------
# Stage bookkeeping -- an ordered array of {Stage, Result, Evidence}, printed
# as both a human-readable table and a machine-readable JSON blob at the end.
# ----------------------------------------------------------------------
$stageOrder = @("CHAOS", "TELEMETRY", "DETECTION", "CORRELATION", "RCA", "AI", "PLAN", "APPROVAL", "EXECUTION", "VERIFICATION")
$stages = [ordered]@{}
foreach ($s in $stageOrder) { $stages[$s] = [ordered]@{ Result = "NOT_REACHED"; Evidence = "" } }

function Set-Stage {
    param([string]$Stage, [string]$Result, [string]$Evidence)
    $stages[$Stage] = [ordered]@{ Result = $Result; Evidence = $Evidence }
    Write-Host "[$Stage] $Result -- $Evidence"
}

function Test-AnyStageFailed {
    foreach ($s in $stageOrder) { if ($stages[$s].Result -eq "FAILED") { return $true } }
    return $false
}

function Write-FinalSummary {
    Write-Host ""
    Write-Host "============================================================"
    Write-Host "M2.15 FULL-CHAIN RESULT (human-readable)"
    Write-Host "============================================================"
    $rows = foreach ($s in $stageOrder) {
        [PSCustomObject]@{ Stage = $s; Result = $stages[$s].Result; Evidence = $stages[$s].Evidence }
    }
    $rows | Format-Table -AutoSize -Wrap | Out-String | Write-Host

    Write-Host "------------------------------------------------------------"
    Write-Host "M2.15 FULL-CHAIN RESULT (machine-readable JSON)"
    Write-Host "------------------------------------------------------------"
    $obj = [ordered]@{}
    foreach ($s in $stageOrder) { $obj[$s] = $stages[$s] }
    ($obj | ConvertTo-Json -Depth 5) | Write-Host
}

# ----------------------------------------------------------------------
# Helpers -- copied verbatim in behavior from test-m27-docker.ps1 /
# test-m28-chaos.ps1 / test-m29-security.ps1 / test-m211-security-read.ps1.
# ----------------------------------------------------------------------
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

function Wait-ForVerificationOutcome {
    param([string]$ExecutionId, [int]$MaxRetries = 20, [int]$SleepSeconds = 2)
    $status = "VERIFYING"
    $retries = 0
    while (($status -eq "VERIFYING" -or $status -eq "PENDING") -and $retries -lt $MaxRetries) {
        Start-Sleep -Seconds $SleepSeconds
        $check = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/executions/$ExecutionId" -Method GET -Key $ExecutorKey
        if ($check.Status -ne 200) { Write-Error "Failed to poll execution status: HTTP $($check.Status)" }
        $status = $check.Body.verificationStatus
        $retries++
    }
    return $status
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
        $statusCode = $null
        if ($_.Exception.Response) { $statusCode = $_.Exception.Response.StatusCode.value__ }
        return @{ Status = $statusCode; Body = $null; ErrorMessage = $_.ErrorDetails.Message }
    }
}

# ============================================================================
# Bring up a clean, isolated Docker state (security enabled) -- same pattern
# as test-m29-security.ps1/test-m211-security-read.ps1.
# ============================================================================
Write-Host "--- Bringing up a clean, isolated Docker state for M2.15 (security + execution enabled) ---"
docker-compose down
if (-not $SkipBuild) { docker-compose up -d --build } else { docker-compose up -d }
Write-Host "Waiting for all services to be healthy..."
Wait-ForAllHealthy
Write-Host "Clean state confirmed."

$runTag = (New-Guid).ToString().Substring(0, 8)
$paymentBaselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
Write-Host "Baseline atlas-payment-service-1 StartedAt: $paymentBaselineStartedAt"

try {

    # ========================================================================
    # STAGE: CHAOS -- the same deterministic, already-proven payment-service
    # fault trigger used throughout M2.7/M2.7.1/M2.7.3/M2.7.4/M2.8: quantity=4
    # on product P200 drives PaymentController's sandbox amount=8888.00 path
    # to a genuine, uncaught HTTP 500, via the real gateway -> order-service ->
    # payment-service call chain (not a direct/isolated call), which is the
    # ONLY path that can accumulate M2.7.3's causal-attribution credit.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: CHAOS ==="
    Write-Host "Triggering a normal traffic baseline (product P100)..."
    for ($i = 0; $i -lt 5; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M215-$runTag-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null
        } catch {}
    }
    Write-Host "Triggering the deterministic payment-service fault (amount=8888.00) via the gateway (product P200, quantity=4)..."
    for ($i = 0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M215-$runTag-PAYFAIL-$i"} -Body '{"productId":"P200","quantity":4}' -TimeoutSec 5 | Out-Null
        } catch {}
    }
    Set-Stage -Stage "CHAOS" -Result "PASS" -Evidence "Sent 5 baseline (P100/qty=1) + 15 fault (P200/qty=4) requests via the real gateway, tagged X-Correlation-ID=ATLAS-M215-$runTag-*"

    # ========================================================================
    # STAGE: TELEMETRY -- independent confirmation, via the live dependency
    # graph (a genuinely separate read path from incident detection below),
    # that the fault traffic actually produced real OTel-derived evidence.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: TELEMETRY ==="
    # A short poll, not a single immediate check: OTel's batch span
    # processor/exporter and the collector's own forwarding introduce real,
    # variable latency between the HTTP calls above returning and the
    # resulting spans actually landing in the dependency graph. The
    # DETECTION stage below already polls (30x5s, the budget proven
    # throughout M2.7/M2.8); this stage uses the same short-poll pattern
    # rather than a single race-prone immediate read.
    $paymentEdge = $null
    $retries = 0
    while ($paymentEdge -eq $null -and $retries -lt 10) {
        $edgesCheck = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/graph/edges" -Method GET -Key $OperatorKey
        if ($edgesCheck.Status -ne 200) { Write-Error "TELEMETRY: failed to read graph edges: HTTP $($edgesCheck.Status)" }
        $paymentEdge = $edgesCheck.Body | Where-Object { $_.target -eq "atlas-payment-service" -and $_.error_count -gt 0 } | Select-Object -First 1
        if ($paymentEdge -eq $null) { Start-Sleep -Seconds 2 }
        $retries++
    }
    if ($paymentEdge -eq $null) {
        Write-Error "TELEMETRY: no dependency-graph edge into atlas-payment-service with error_count>0 was observed within 10x2s -- the fault traffic did not produce real telemetry. INFRASTRUCTURE/PRODUCT FAILURE."
    }
    Set-Stage -Stage "TELEMETRY" -Result "PASS" -Evidence "Real graph edge $($paymentEdge.source)->$($paymentEdge.target): call_count=$($paymentEdge.call_count) error_count=$($paymentEdge.error_count)"

    # ========================================================================
    # STAGE: DETECTION + CORRELATION -- same poll used by test-m27/test-m28:
    # a correlated PRIMARY incident rooted at atlas-payment-service.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: DETECTION + CORRELATION ==="
    $primaryIncident = $null
    $retries = 0
    while ($primaryIncident -eq $null -and $retries -lt 30) {
        Start-Sleep -Seconds 5
        $incidentsCheck = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET -Key $OperatorKey
        if ($incidentsCheck.Status -eq 200) {
            foreach ($inc in $incidentsCheck.Body) {
                if ($inc.primaryIncidentId -eq $inc.incidentId -and $inc.rootService -eq "atlas-payment-service") {
                    $primaryIncident = $inc
                    break
                }
            }
        }
        $retries++
    }

    if ($primaryIncident -eq $null) {
        Set-Stage -Stage "DETECTION" -Result "FAILED" -Evidence "No incident rooted at atlas-payment-service appeared within 30x5s -- detection did not fire on real fault traffic."
        Write-Error "DETECTION: no incident appeared in time -- INFRASTRUCTURE/PRODUCT FAILURE."
    }
    $incidentId = $primaryIncident.incidentId
    Set-Stage -Stage "DETECTION" -Result "PASS" -Evidence "incidentId=$incidentId rootService=$($primaryIncident.rootService) fingerprint=$($primaryIncident.fingerprint)"
    Set-Stage -Stage "CORRELATION" -Result "PASS" -Evidence "primaryIncidentId==incidentId (self-primary); correlationGroupId=$($primaryIncident.correlationGroupId); relatedIncidentIds=$($primaryIncident.relatedIncidentIds -join ',')"

    # ========================================================================
    # STAGE: RCA -- read directly off the already-materialized incident
    # object (internal/rca is FROZEN; this reads its output, never invokes
    # it directly, exactly like handleGetRCA in internal/httpapi/incident.go).
    # RCA itself always produces a verdict; PASS records whatever real
    # confidence resulted, never judges it.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: RCA ==="
    $rcaService = $primaryIncident.rootCause.service
    $rcaConfidence = $primaryIncident.rootCause.confidence
    $rcaScore = $primaryIncident.rootCause.score
    Set-Stage -Stage "RCA" -Result "PASS" -Evidence "service=$rcaService confidence=$rcaConfidence score=$rcaScore (frozen rca.Engine output, read verbatim from Incident.RCA)"

    # ========================================================================
    # STAGE: AI (Fake) -- POST /analyze, now correctly routed and
    # PermissionView-gated as of Module 4 (previously misrouted into a
    # GET-only handler -- see the Module 4 release review for the routing
    # defect and its fix), then independently GET /analysis (also
    # PermissionView) to cross-check. Asserts the REAL FakeProvider identity
    # fields, not an assumption that FakeProvider ran.
    # ========================================================================
    # NOTE: AI reasoning (POST /analyze) and remediation planning (POST
    # /remediation/plan) are architecturally INDEPENDENT paths in ATLAS --
    # internal/remediation's Planner never calls internal/aireasoning, and
    # vice versa. A defect isolated to the AI stage must therefore not be
    # treated as gating PLAN/APPROVAL/EXECUTION/VERIFICATION, which is why
    # this stage records its outcome and continues rather than aborting the
    # script -- unlike a genuine BLOCKED/FAILED in PLAN or later, which
    # really does gate everything downstream of it.
    Write-Host ""
    Write-Host "=== STAGE: AI (Fake reasoning) ==="
    $analysis = $null
    $analyzeResult = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$incidentId/analyze" -Method POST -Key $OperatorKey
    if ($analyzeResult.Status -ne 200) {
        Set-Stage -Stage "AI" -Result "FAILED" -Evidence "POST /analyze failed: HTTP $($analyzeResult.Status) $($analyzeResult.ErrorMessage). Not treated as gating PLAN/APPROVAL/EXECUTION/VERIFICATION below, since internal/remediation's Planner never depends on internal/aireasoning -- see the release report for root-cause analysis."
    } else {
        $analysis = $analyzeResult.Body
    }
    if ($analysis -ne $null) {
        if ($analysis.status -eq "DISABLED") {
            Set-Stage -Stage "AI" -Result "BLOCKED" -Evidence "AI reasoning reported DISABLED (ATLAS_AI_ENABLED=false) -- a legitimate configuration state, not exercised further."
        } elseif ($analysis.provider -ne "fake" -or $analysis.model -ne "fake-model") {
            Set-Stage -Stage "AI" -Result "FAILED" -Evidence "Expected provider=fake/model=fake-model (FakeProvider), got provider=$($analysis.provider)/model=$($analysis.model) -- this would mean a REAL AI provider ran, which Module 3 must never do."
            Write-Error "AI: unexpected provider/model -- STOP, this must never be a real provider."
        } else {
            $analysisCheck = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$incidentId/analysis" -Method GET -Key $OperatorKey
            if ($analysisCheck.Status -ne 200 -or $analysisCheck.Body.analysisId -ne $analysis.analysisId) {
                Set-Stage -Stage "AI" -Result "FAILED" -Evidence "Independent GET /analysis did not confirm the same analysisId ($($analysis.analysisId))."
            } else {
                Set-Stage -Stage "AI" -Result "PASS" -Evidence "analysisId=$($analysis.analysisId) provider=fake model=fake-model (confirmed via POST /analyze and independently cross-checked via GET /analysis)"
            }
        }
    }

    # ========================================================================
    # STAGE: PLAN -- POST /remediation/plan as OPERATOR (PermissionCreatePlan).
    # AMBIGUOUS/LOW-confidence rejection is M2.6's real, unmodified safety
    # policy correctly firing -- BLOCKED, not FAILED, exactly as every prior
    # script treats it.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: PLAN ==="
    $planAttempt = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation/plan" -Method POST -Key $OperatorKey
    $plan = $null
    if ($planAttempt.Status -ne 200 -and $planAttempt.Status -ne $null) {
        $errBody = $planAttempt.ErrorMessage
        if ($errBody -like "*AMBIGUOUS*" -or $errBody -like "*LOW confidence*") {
            Set-Stage -Stage "PLAN" -Result "BLOCKED" -Evidence "M2.6 policy correctly refused a HIGH-risk plan against insufficient-confidence RCA: $errBody"
        } else {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Plan generation failed for an unrecognized reason (HTTP $($planAttempt.Status)): $errBody"
            Write-Error "PLAN: unexpected failure reason -- PRODUCT/ARCHITECTURE FAILURE."
        }
    } else {
        $planCheck = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation" -Method GET -Key $OperatorKey
        if ($planCheck.Status -ne 200 -or -not $planCheck.Body.planId) {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Plan generation reported success but the plan could not be independently retrieved."
            Write-Error "PLAN: could not retrieve the generated plan -- PRODUCT/ARCHITECTURE FAILURE."
        }
        $plan = $planCheck.Body
        if ($plan.planner -ne "fake" -or $plan.plannerVersion -ne "1.0") {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Expected planner=fake/plannerVersion=1.0 (FakePlanner), got planner=$($plan.planner)/plannerVersion=$($plan.plannerVersion) -- this would mean a REAL AI planner ran, which Module 3 must never do."
            Write-Error "PLAN: unexpected planner identity -- STOP, this must never be a real provider."
        }
        if ($plan.actions[0].targetService -ne "atlas-payment-service") {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Expected the plan to target atlas-payment-service, got $($plan.actions[0].targetService)."
            Write-Error "PLAN: unexpected target service -- PRODUCT/ARCHITECTURE FAILURE."
        }
        Set-Stage -Stage "PLAN" -Result "PASS" -Evidence "planId=$($plan.planId) planner=fake plannerVersion=1.0 target=$($plan.actions[0].targetService) riskLevel=$($plan.riskLevel)"
    }

    if ($stages["PLAN"].Result -ne "PASS") {
        Write-Host ""
        Write-Host "PLAN did not PASS -- APPROVAL/EXECUTION/VERIFICATION are correctly NOT_REACHED. This is a valid, honest terminal state (unless PLAN itself is FAILED rather than BLOCKED)."
        Write-FinalSummary
        Write-Host ""
        if (Test-AnyStageFailed) {
            Write-Host "Test completed -- but at least one stage genuinely FAILED (see table above). Exiting non-zero."
            exit 1
        }
        Write-Host "Test completed -- chain legitimately stopped at PLAN (BLOCKED by M2.6 policy). No stage FAILED. Exiting zero."
        exit 0
    }

    # ========================================================================
    # STAGE: APPROVAL -- POST /approve as APPROVER (PermissionApprovePlan).
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: APPROVAL ==="
    $approveReq = @{ approver = "m215-approver"; reason = "M2.15 full-chain validation run $runTag" } | ConvertTo-Json
    $approveResult = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$($plan.planId)/approve" -Method POST -Key $ApproverKey -Body $approveReq
    if ($approveResult.Status -ne 200) {
        Set-Stage -Stage "APPROVAL" -Result "FAILED" -Evidence "APPROVER's approval call failed: HTTP $($approveResult.Status) $($approveResult.ErrorMessage)"
        Write-Error "APPROVAL: legitimate APPROVER could not approve an already-cleared plan -- PRODUCT/ARCHITECTURE FAILURE."
    }
    $planApprovedCheck = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation" -Method GET -Key $OperatorKey
    if ($planApprovedCheck.Body.status -ne "APPROVED") {
        Set-Stage -Stage "APPROVAL" -Result "FAILED" -Evidence "Expected plan status=APPROVED after a legitimate APPROVER call, got $($planApprovedCheck.Body.status)."
        Write-Error "APPROVAL: plan did not transition to APPROVED -- PRODUCT/ARCHITECTURE FAILURE."
    }
    Set-Stage -Stage "APPROVAL" -Result "PASS" -Evidence "planId=$($plan.planId) status=APPROVED approvedBy=$($planApprovedCheck.Body.approval.approvedBy) (authenticated identity, matches M2.9's guarantee)"

    # ========================================================================
    # STAGE: EXECUTION -- POST /execute as EXECUTOR (PermissionExecute).
    # Independently cross-checked via `docker inspect` StartedAt, exactly
    # like test-m28-chaos.ps1's own restart proof.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: EXECUTION ==="
    $actionId = $planApprovedCheck.Body.actions[0].actionId
    $execReq = @{ actionId = $actionId; approver = "m215-executor" } | ConvertTo-Json
    $execResult = Invoke-Authenticated -Uri "http://localhost:8081/api/v1/remediation/$($plan.planId)/execute" -Method POST -Key $ExecutorKey -Body $execReq
    if ($execResult.Status -ne 200 -or $execResult.Body.executionStatus -ne "EXECUTED") {
        Set-Stage -Stage "EXECUTION" -Result "FAILED" -Evidence "Execution did not report EXECUTED (HTTP $($execResult.Status)): $($execResult.Body.message) / $($execResult.Body.error) / $($execResult.ErrorMessage)"
        Write-Error "EXECUTION: did not succeed after a legitimately approved plan -- PRODUCT/ARCHITECTURE FAILURE."
    }
    $paymentAfterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
    if ($paymentAfterStartedAt -eq $paymentBaselineStartedAt) {
        Set-Stage -Stage "EXECUTION" -Result "FAILED" -Evidence "executionStatus=EXECUTED but atlas-payment-service-1's independently-inspected StartedAt did NOT change ($paymentBaselineStartedAt) -- execution claimed success without a real restart."
        Write-Error "EXECUTION: independent Docker restart proof failed -- PRODUCT/ARCHITECTURE FAILURE."
    }
    $executionId = $execResult.Body.executionId
    Set-Stage -Stage "EXECUTION" -Result "PASS" -Evidence "executionId=$executionId executionStatus=EXECUTED; independent docker inspect StartedAt changed $paymentBaselineStartedAt -> $paymentAfterStartedAt"

    # ========================================================================
    # STAGE: VERIFICATION -- poll /executions/{id} as EXECUTOR
    # (PermissionReadAudit). VERIFIED and VERIFICATION_TIMEOUT are both
    # legitimate per M2.7.4; a plain FAILED here (no continuous fault was
    # injected after execution) would indicate genuine unexplained
    # renewed degradation and is treated as a real FAILED outcome.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: VERIFICATION ==="
    $verifStatus = Wait-ForVerificationOutcome -ExecutionId $executionId
    switch ($verifStatus) {
        "VERIFIED" {
            Set-Stage -Stage "VERIFICATION" -Result "PASS" -Evidence "verificationStatus=VERIFIED -- incident genuinely reached RESOLVED. FULL CHAIN REACHED EXECUTED -> VERIFIED."
        }
        "VERIFICATION_TIMEOUT" {
            Set-Stage -Stage "VERIFICATION" -Result "PASS" -Evidence "verificationStatus=VERIFICATION_TIMEOUT -- real restart confirmed (see EXECUTION evidence); the incident's own M2.4 recovery window simply had not elapsed within the verification budget. Per M2.7.4, this is NOT a restart failure."
        }
        default {
            Set-Stage -Stage "VERIFICATION" -Result "FAILED" -Evidence "Expected VERIFIED or VERIFICATION_TIMEOUT (a real restart was already independently confirmed and no continuous post-execution fault was injected), got $verifStatus."
            Write-Error "VERIFICATION: unexpected outcome $verifStatus -- PRODUCT/ARCHITECTURE FAILURE."
        }
    }

    Write-FinalSummary
    Write-Host ""
    if (Test-AnyStageFailed) {
        Write-Host "Test completed -- the chain reached VERIFICATION, but at least one stage genuinely FAILED along the way (see table above, e.g. AI). Exiting non-zero so this does not silently pass in CI."
        exit 1
    }
    Write-Host "Test completed successfully -- full chain outcome recorded above, no stage FAILED."

} catch {
    Write-FinalSummary
    throw
}
