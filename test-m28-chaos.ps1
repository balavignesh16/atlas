param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# ============================================================================
# M2.8 -- Chaos Engineering / Fault-Injection E2E validation.
#
# This script is DELIBERATELY separate from test-m27-docker.ps1. M2.7's test
# harness proves the execution/verification engine's own correctness and
# must never be touched or weakened by this milestone. This script instead
# drives real faults into the ATLAS lab application and observes whether the
# frozen M2.7 pipeline (detection -> correlation -> causal attribution -> RCA
# -> M2.6 policy -> M2.7 guard/execution -> M2.7.4 verification) behaves
# correctly under those faults. No production Go code is expected to change
# as a result of this milestone -- this script only drives HTTP traffic and
# container lifecycle commands against the existing, frozen system.
#
# Every scenario resets to a fully clean Docker state first (down + up),
# rather than relying on wait budgets to decontaminate leftover state --
# EventBuffer, the dependency graph's cumulative edge stats, and incident
# LastUpdatedAt are all documented (M2.7.3/M2.7.4) as having no reliable
# time-based decay, so a hard reset is the only deterministic isolation
# boundary available.
# ============================================================================

function Wait-ForVerificationOutcome {
    param(
        [string]$ExecutionId,
        [int]$MaxRetries = 20,
        [int]$SleepSeconds = 2
    )
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
                if ($url -like "*actuator*") {
                    if ($resp.status -eq "UP") { $healthy = $true }
                } else {
                    $healthy = $true
                }
            } catch {
                Start-Sleep -Seconds 2
                $retries++
            }
        }
        if (-not $healthy) {
            Write-Error "Endpoint did not become healthy in time: $url"
        }
    }
}

function Reset-CleanDockerState {
    param([string]$Reason)
    Write-Host ""
    Write-Host "--- Deterministic isolation reset: $Reason ---"
    docker-compose down
    if (-not $SkipBuild) {
        docker-compose up -d --build
    } else {
        docker-compose up -d
    }
    Write-Host "Waiting for all services to be healthy..."
    Wait-ForAllHealthy
    Write-Host "Clean state confirmed. Reset mechanism: full docker-compose down + up (no wait-based decontamination)."
}

$scenarioResults = @()

# ============================================================================
# SCENARIO 1 -- Single-service payment failure.
#
# Uses the existing, already-proven, isolated payment-service fault path
# (amount=8888.00, sent directly to :8086, bypassing gateway/order-service --
# the same mechanism M2.7.1-M2.7.4's Scenario B/C/D have used throughout).
#
# IMPLEMENTATION NOTE (found live, during this scenario's first run): the
# ISOLATED direct-to-:8086 injection path structurally caps RCA at
# LOW/score=25 -- a single-incident group has no other correlated member to
# receive a causal-attribution dependency-error credit from, so it can never
# cross M2.6's MEDIUM threshold. This is not bad luck; it is the same,
# already-documented characteristic behind M2.7.1's report ("isolated
# single-evidence-type path is structurally capped below MEDIUM
# confidence") and the exact reason M2.7.4's own Scenario D was redesigned
# away from this mechanism. To actually exercise this scenario's target
# property (the full plan->approve->execute->verify chain, not just
# detection), this scenario uses the SAME existing amount=8888.00 fault
# trigger, routed through the existing gateway cascade path instead
# (proven reliable, live, across M2.7.3/M2.7.4) -- not a new mechanism, just
# the existing trigger sent via a different existing entry point.
# Causal attribution (frozen, unmodified) still correctly narrows the
# remediation target down to atlas-payment-service alone, even though
# caller-side symptom incidents also form -- which is realistic: a real
# single-service failure causes caller-side symptoms in any real system.
# ============================================================================

Reset-CleanDockerState -Reason "Scenario 1 isolation"

Write-Host ""
Write-Host "=== SCENARIO 1: single-service payment failure ==="

$paymentBaselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
Write-Host "Baseline atlas-payment-service-1 StartedAt: $paymentBaselineStartedAt"

Write-Host "Triggering a normal traffic baseline (product P100)..."
for ($i = 0; $i -lt 5; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M28-S1-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null
    } catch {}
}
Write-Host "Triggering the existing deterministic payment-service fault (amount=8888.00) via the gateway (product P200, quantity=4)..."
for ($i = 0; $i -lt 15; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M28-S1-PAYFAIL-$i"} -Body '{"productId":"P200","quantity":4}' -TimeoutSec 5 | Out-Null
    } catch {}
}

Write-Host "Waiting for a non-ambiguous, uncorrelated payment-service incident..."
$s1Incident = $null
$retries = 0
while ($s1Incident -eq $null -and $retries -lt 30) {
    Start-Sleep -Seconds 5
    try {
        $incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
        foreach ($inc in $incidents) {
            if ($inc.rootService -eq "atlas-payment-service" -and $inc.primaryIncidentId -eq $inc.incidentId) {
                $s1Incident = $inc
                break
            }
        }
    } catch {}
    $retries++
}

if ($s1Incident -eq $null) {
    Write-Error "SCENARIO 1: no payment-service incident appeared -- INFRASTRUCTURE FAILURE or detection did not fire."
}

$s1IncidentId = $s1Incident.incidentId
Write-Host "Incident detected: $s1IncidentId (service=$($s1Incident.rootCause.service) confidence=$($s1Incident.rootCause.confidence) score=$($s1Incident.rootCause.score))"

$s1PlanBlocked = $false
$s1Outcome = "UNKNOWN"
try {
    Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s1IncidentId/remediation/plan" -Method POST -UseBasicParsing -ErrorAction Stop | Out-Null
} catch {
    $body = $_.ErrorDetails.Message
    if ($body -like "*LOW confidence*" -or $body -like "*AMBIGUOUS*") {
        $s1PlanBlocked = $true
        Write-Host "SAFE BLOCK: M2.6 correctly refused to plan a HIGH-risk action against insufficient-confidence RCA ($body)."
        $s1Outcome = "SAFE BLOCK -- RCA confidence insufficient this run (known, pre-existing, evidence-dependent characteristic; not a chaos-specific defect)"
    } else {
        Write-Error "SCENARIO 1: plan generation failed for an unexpected reason: $body -- PRODUCT/ARCHITECTURE FAILURE"
    }
}

if (-not $s1PlanBlocked) {
    $plan = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s1IncidentId/remediation" -UseBasicParsing
    $planId = $plan.planId
    if ($plan.actions[0].targetService -ne "atlas-payment-service") {
        Write-Error "SCENARIO 1: expected the plan to target atlas-payment-service, got $($plan.actions[0].targetService)"
    }
    $approvalReq = @{ approver = "chaos-m28"; reason = "M2.8 Scenario 1: single-service payment failure" }
    Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Body ($approvalReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} | Out-Null

    $planApproved = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s1IncidentId/remediation" -UseBasicParsing
    $actionId = $planApproved.actions[0].actionId
    $execReq = @{ actionId = $actionId; approver = "chaos-m28" }
    $execRecord = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Body ($execReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} -UseBasicParsing

    Write-Host "Execution Status: $($execRecord.executionStatus)"
    if ($execRecord.executionStatus -ne "EXECUTED") {
        Write-Error "SCENARIO 1: execution did not succeed: $($execRecord.message) / $($execRecord.error) -- PRODUCT/ARCHITECTURE FAILURE"
    }

    $paymentAfterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
    Write-Host "atlas-payment-service-1 StartedAt after execution: $paymentAfterStartedAt"
    if ($paymentAfterStartedAt -eq $paymentBaselineStartedAt) {
        Write-Error "SCENARIO 1: independent Docker restart proof FAILED -- StartedAt did not change ($paymentBaselineStartedAt)"
    }
    Write-Host "Independent Docker restart proof: StartedAt changed from $paymentBaselineStartedAt to $paymentAfterStartedAt"

    $verifStatus = Wait-ForVerificationOutcome -ExecutionId $execRecord.executionId
    Write-Host "Final Verification Status: $verifStatus"

    switch ($verifStatus) {
        "VERIFIED" { $s1Outcome = "PASS -- incident genuinely reached RESOLVED, verified by direct Status observation" }
        "VERIFICATION_TIMEOUT" { $s1Outcome = "EXPECTED TIMEOUT -- real restart confirmed, recovery not yet confirmed within the verification window; not a false failure" }
        "FAILED" { $s1Outcome = "EXPECTED FAILED -- genuine renewed post-execution degradation evidence observed (would indicate the fault is still active, unexpected for a clean single-fault scenario -- flagged for review, not silently accepted)" }
        default { Write-Error "SCENARIO 1: unexpected verification status $verifStatus -- PRODUCT/ARCHITECTURE FAILURE" }
    }
}

Write-Host ""
Write-Host "=== SCENARIO 1 RESULT: $s1Outcome ==="
$scenarioResults += "Scenario 1 (single-service payment failure): $s1Outcome"

# ============================================================================
# SCENARIO 2 -- True independent concurrent failure.
#
# INVESTIGATION FINDING (verified live before writing this scenario, see
# docs/m28_verification_report.md for full detail): stopping
# atlas-inventory-service-1's container does NOT produce an incident whose
# rootService is atlas-inventory-service, and produces NO dependency-graph
# edge pointing at it at all. Confirmed both empirically (live incidents/open
# and graph/edges queries showed zero inventory-service presence) and in the
# actual code: internal/correlation/correlation.go's AddDependency call
# (line ~158) requires a matched parent+child span pair; a fully-stopped
# service never emits its own child span, so no edge is ever constructed to
# it, for ANY service, not inventory specifically. Per instruction, this is
# reported rather than worked around with new tooling or application code.
#
# What IS still testable with existing mechanisms: two GENUINELY
# graph-disconnected fault chains running concurrently.
#   Fault A: inventory-service container-stop + normal gateway-routed
#            traffic (P100/qty=1). This produces incidents rooted at
#            atlas-order-service/atlas-gateway (the callers that
#            experienced the failure), never at inventory itself.
#   Fault B: payment-service's own isolated fault (amount=8888.00, direct to
#            :8086), which never touches gateway/order-service at all --
#            structurally disconnected in the dependency graph from Fault A.
# The safety assertion under test: these two chains, triggered concurrently,
# must not be falsely merged into one correlation group, and whatever RCA/
# remediation-gate outcome each side reaches on its own must be observed and
# reported honestly, not forced.
# ============================================================================

Reset-CleanDockerState -Reason "Scenario 2 isolation"

Write-Host ""
Write-Host "=== SCENARIO 2: true independent concurrent failure ==="
Write-Host "NOTE: inventory-service container-stop cannot produce an incident rootServiced at atlas-inventory-service under the current, frozen architecture -- see docs/m28_verification_report.md. This scenario instead tests two genuinely graph-disconnected fault chains: an inventory-outage-driven order-service/gateway chain, and an isolated payment-service chain."

Write-Host "Injecting Fault A: stopping atlas-inventory-service-1..."
docker stop atlas-inventory-service-1 | Out-Null

Write-Host "Generating normal gateway-routed traffic (P100/qty=1) that will fail at the inventory-reservation step..."
for ($i = 0; $i -lt 15; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M28-S2-INV-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null
    } catch {}
}

Write-Host "Injecting Fault B: isolated payment-service fault directly on :8086 (structurally disconnected from the gateway/order-service graph)..."
for ($i = 0; $i -lt 15; $i++) {
    $idempotencyKey = "ATLAS-M28-S2-PAY-$i-$(New-Guid)"
    $body = @{ orderId = "ORD-M28-S2-$i"; amount = 8888.00 } | ConvertTo-Json
    try {
        Invoke-RestMethod -Uri "http://localhost:8086/api/payments" -Method POST -Headers @{"Content-Type"="application/json"; "Idempotency-Key"=$idempotencyKey} -Body $body -TimeoutSec 5 | Out-Null
    } catch {}
}

Write-Host "Waiting for both incident chains to settle..."
Start-Sleep -Seconds 25

$allIncidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
$edges = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph/edges" -UseBasicParsing

Write-Host ""
Write-Host "--- Observed incidents (actual, unforced) ---"
foreach ($inc in $allIncidents) {
    Write-Host "  incidentId=$($inc.incidentId) rootService=$($inc.rootService) fingerprint=$($inc.fingerprint) correlationGroupId=$($inc.correlationGroupId) primary=$($inc.primaryIncidentId -eq $inc.incidentId) related=$($inc.relatedIncidentIds -join ',') rca=$($inc.rootCause.service)/$($inc.rootCause.confidence)/$($inc.rootCause.score)"
}
Write-Host "--- Observed dependency-graph edges ---"
foreach ($e in $edges) {
    Write-Host "  $($e.source) -> $($e.target) calls=$($e.call_count) errors=$($e.error_count)"
}

$inventoryRootedIncident = $allIncidents | Where-Object { $_.rootService -eq "atlas-inventory-service" }
if ($inventoryRootedIncident) {
    Write-Error "SCENARIO 2: unexpected -- an incident rootServiced at atlas-inventory-service actually appeared this run ($($inventoryRootedIncident.incidentId)). This contradicts the investigation finding; STOP and re-investigate rather than silently accepting -- PRODUCT/ARCHITECTURE FAILURE (or a stale investigation finding)."
}

$paymentIncident = $allIncidents | Where-Object { $_.rootService -eq "atlas-payment-service" -and $_.primaryIncidentId -eq $_.incidentId } | Select-Object -First 1
$orderGatewayIncidents = $allIncidents | Where-Object { $_.rootService -in @("atlas-order-service", "atlas-gateway") }
$orderGatewayPrimary = $orderGatewayIncidents | Where-Object { $_.primaryIncidentId -eq $_.incidentId } | Select-Object -First 1

$s2Outcome = "UNKNOWN"
if ($paymentIncident -eq $null) {
    Write-Error "SCENARIO 2: no payment-service incident appeared for Fault B -- INFRASTRUCTURE FAILURE or detection did not fire."
} elseif ($orderGatewayPrimary -eq $null) {
    Write-Error "SCENARIO 2: no order-service/gateway incident appeared for Fault A -- INFRASTRUCTURE FAILURE or detection did not fire."
} else {
    $falselyMerged = ($paymentIncident.correlationGroupId -eq $orderGatewayPrimary.correlationGroupId) -or
                      ($paymentIncident.relatedIncidentIds -contains $orderGatewayPrimary.incidentId) -or
                      ($orderGatewayPrimary.relatedIncidentIds -contains $paymentIncident.incidentId)
    if ($falselyMerged) {
        Write-Error "SCENARIO 2: the two structurally-disconnected fault chains were falsely correlated together -- PRODUCT/ARCHITECTURE FAILURE (this is the core safety property this scenario exists to check)."
    } else {
        Write-Host "PASS: the two genuinely disconnected fault chains remained in separate correlation groups (payment=$($paymentIncident.correlationGroupId), order/gateway=$($orderGatewayPrimary.correlationGroupId))."
        $s2Outcome = "PASS -- two structurally independent fault chains (isolated payment fault; inventory-outage-driven order/gateway chain) stayed correctly uncorrelated. Payment RCA: $($paymentIncident.rootCause.service)/$($paymentIncident.rootCause.confidence)/$($paymentIncident.rootCause.score). Order/gateway RCA: $($orderGatewayPrimary.rootCause.service)/$($orderGatewayPrimary.rootCause.confidence)/$($orderGatewayPrimary.rootCause.score). Inventory-service itself never appeared as a candidate or edge target (documented architectural limitation, not a defect of this milestone)."
    }
}

Write-Host "Restoring atlas-inventory-service-1 (fault-injection cleanup, not ATLAS remediation)..."
docker start atlas-inventory-service-1 | Out-Null

Write-Host ""
Write-Host "=== SCENARIO 2 RESULT: $s2Outcome ==="
$scenarioResults += "Scenario 2 (true independent concurrent failure): $s2Outcome"

# ============================================================================
# SCENARIO 3 -- Persistent fault during remediation.
#
# The chaos driver keeps sending the deterministic payment-service fault
# (amount=8888.00) continuously, in a background job, spanning before,
# during, and after the real plan -> approve -> execute chain. The chaos
# script never calls the Docker adapter or execution.Engine directly -- it
# only ever drives the real HTTP remediation endpoints, exactly like
# Scenario 1. Because the fault trigger is unconditional application logic
# (not fixed by a restart), traffic sent after the container comes back up
# will keep producing genuinely NEW, real, post-execution ERROR events --
# this is expected to settle on VERIFICATION_FAILED (real evidence) rather
# than VERIFICATION_TIMEOUT, and must NEVER settle on VERIFIED while the
# fault is still active.
#
# Routed via the gateway cascade path (product P200, quantity=4), not the
# isolated direct-to-:8086 path, for the same reason as Scenario 1: the
# isolated path structurally caps RCA at LOW/25 and would prevent this
# scenario from ever reaching execution to exercise its target property.
# ============================================================================

Reset-CleanDockerState -Reason "Scenario 3 isolation"

Write-Host ""
Write-Host "=== SCENARIO 3: persistent fault during remediation ==="

$s3PaymentBaselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
Write-Host "Baseline atlas-payment-service-1 StartedAt: $s3PaymentBaselineStartedAt"

Write-Host "Triggering a normal traffic baseline (product P100)..."
for ($i = 0; $i -lt 5; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M28-S3-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 5 | Out-Null
    } catch {}
}

Write-Host "Starting continuous fault-generating traffic via the gateway (background job)..."
$faultJob = Start-Job -ScriptBlock {
    while ($true) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M28-S3-PAYFAIL-$(New-Guid)"} -Body '{"productId":"P200","quantity":4}' -TimeoutSec 5 -ErrorAction SilentlyContinue | Out-Null
        } catch {}
        Start-Sleep -Milliseconds 800
    }
}

try {
    Write-Host "Waiting for the persistent-fault incident to be detected..."
    $s3Incident = $null
    $retries = 0
    while ($s3Incident -eq $null -and $retries -lt 30) {
        Start-Sleep -Seconds 5
        try {
            $incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
            foreach ($inc in $incidents) {
                if ($inc.rootService -eq "atlas-payment-service" -and $inc.primaryIncidentId -eq $inc.incidentId) {
                    $s3Incident = $inc
                    break
                }
            }
        } catch {}
        $retries++
    }

    if ($s3Incident -eq $null) {
        Write-Error "SCENARIO 3: no payment-service incident appeared -- INFRASTRUCTURE FAILURE or detection did not fire."
    }

    $s3IncidentId = $s3Incident.incidentId
    Write-Host "Incident detected: $s3IncidentId (service=$($s3Incident.rootCause.service) confidence=$($s3Incident.rootCause.confidence) score=$($s3Incident.rootCause.score))"

    $s3PlanBlocked = $false
    $s3Outcome = "UNKNOWN"
    try {
        Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s3IncidentId/remediation/plan" -Method POST -UseBasicParsing -ErrorAction Stop | Out-Null
    } catch {
        $body = $_.ErrorDetails.Message
        if ($body -like "*LOW confidence*" -or $body -like "*AMBIGUOUS*") {
            $s3PlanBlocked = $true
            Write-Host "SAFE BLOCK: M2.6 correctly refused to plan a HIGH-risk action against insufficient-confidence RCA ($body)."
            $s3Outcome = "SAFE BLOCK -- RCA confidence insufficient this run; the persistent-fault verification property was not exercised because remediation never reached execution (acceptable safety outcome, but re-run recommended to exercise the intended property)"
        } else {
            Write-Error "SCENARIO 3: plan generation failed for an unexpected reason: $body -- PRODUCT/ARCHITECTURE FAILURE"
        }
    }

    if (-not $s3PlanBlocked) {
        $plan = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s3IncidentId/remediation" -UseBasicParsing
        $planId = $plan.planId
        $approvalReq = @{ approver = "chaos-m28"; reason = "M2.8 Scenario 3: persistent fault during remediation" }
        Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Body ($approvalReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} | Out-Null

        $planApproved = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$s3IncidentId/remediation" -UseBasicParsing
        $actionId = $planApproved.actions[0].actionId
        $execReq = @{ actionId = $actionId; approver = "chaos-m28" }
        $execRecord = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Body ($execReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} -UseBasicParsing

        Write-Host "Execution Status: $($execRecord.executionStatus)"
        if ($execRecord.executionStatus -ne "EXECUTED") {
            Write-Error "SCENARIO 3: execution did not succeed: $($execRecord.message) / $($execRecord.error) -- PRODUCT/ARCHITECTURE FAILURE"
        }

        $s3PaymentAfterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
        Write-Host "atlas-payment-service-1 StartedAt after execution: $s3PaymentAfterStartedAt"
        if ($s3PaymentAfterStartedAt -eq $s3PaymentBaselineStartedAt) {
            Write-Error "SCENARIO 3: independent Docker restart proof FAILED -- StartedAt did not change ($s3PaymentBaselineStartedAt)"
        }
        Write-Host "Independent Docker restart proof: StartedAt changed from $s3PaymentBaselineStartedAt to $s3PaymentAfterStartedAt"
        Write-Host "Fault-generating traffic continues in the background through and after this restart (fault trigger is unconditional application logic, unaffected by a restart)."

        $verifStatus = Wait-ForVerificationOutcome -ExecutionId $execRecord.executionId -MaxRetries 25

        Write-Host "Final Verification Status: $verifStatus"

        switch ($verifStatus) {
            "VERIFIED" {
                Write-Error "SCENARIO 3: verification reported VERIFIED while the fault-generating traffic was still active throughout -- this would be a FALSE VERIFIED, exactly what M2.7.4 exists to prevent -- PRODUCT/ARCHITECTURE FAILURE."
            }
            "VERIFICATION_TIMEOUT" {
                $s3Outcome = "EXPECTED TIMEOUT -- real restart confirmed; the still-active fault correctly did not produce a false VERIFIED. (No genuine post-execution error event landed within the verification window this run -- a legitimate, safe outcome, though FAILED was also anticipated as at least as likely given continuous fault traffic.)"
            }
            "FAILED" {
                $s3Outcome = "EXPECTED FAILED -- real restart confirmed; genuine renewed post-execution degradation evidence (from the still-active fault) was correctly detected via EventBuffer, exactly as M2.7.4 is designed to do. No false VERIFIED occurred."
            }
            default {
                Write-Error "SCENARIO 3: unexpected verification status $verifStatus -- PRODUCT/ARCHITECTURE FAILURE"
            }
        }
    }
} finally {
    Write-Host "Stopping the persistent-fault background job..."
    Stop-Job -Job $faultJob -ErrorAction SilentlyContinue | Out-Null
    Remove-Job -Job $faultJob -Force -ErrorAction SilentlyContinue | Out-Null
}

Write-Host ""
Write-Host "=== SCENARIO 3 RESULT: $s3Outcome ==="
$scenarioResults += "Scenario 3 (persistent fault during remediation): $s3Outcome"

Write-Host ""
Write-Host "============================================================"
Write-Host "M2.8 CHAOS SUITE SUMMARY"
Write-Host "============================================================"
foreach ($r in $scenarioResults) {
    Write-Host $r
}
Write-Host ""
Write-Host "Test completed."
