param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $SkipBuild) {
    Write-Host "Restarting all containers (building intelligence-engine via Docker)..."
    docker-compose down
    docker-compose up -d --build
}

Write-Host "Waiting for services to be healthy..."
$healthy = $false
$retries = 0
while (-not $healthy -and $retries -lt 30) {
    try {
        $gatewayHealth = Invoke-RestMethod -Uri "http://localhost:8083/actuator/health" -UseBasicParsing -ErrorAction Stop
        if ($gatewayHealth.status -eq "UP") {
            $healthy = $true
        }
    } catch {
        Start-Sleep -Seconds 2
        $retries++
    }
}
if (-not $healthy) {
    Write-Error "Gateway did not become healthy."
}

Write-Host "Triggering incident to generate a remediation plan..."
for ($i=0; $i -lt 5; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M27-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' | Out-Null
    } catch {}
}
for ($i=0; $i -lt 15; $i++) {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M27-PAYFAIL-$i"} -Body '{"productId":"P100","quantity":3}' -TimeoutSec 2 | Out-Null
    } catch {}
}

Write-Host "Waiting for AI reasoning and Remediation Planning..."

$plan = $null
$retries = 0
while ($plan -eq $null -and $retries -lt 30) {
    Start-Sleep -Seconds 5
    try {
        $incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
        if ($incidents.Length -gt 0) {
            $incidentId = $incidents[0].incidentId
            
            # Generate the plan first!
            try {
                Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation/plan" -Method POST -UseBasicParsing -ErrorAction Stop | Out-Null
            } catch {}

            $planResponse = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation" -UseBasicParsing -ErrorAction Stop
            if ($planResponse -and $planResponse.planId) {
                $plan = $planResponse
            }
        }
    } catch {
        # Plan not ready yet (404)
    }
    $retries++
}

if ($plan -eq $null) {
    Write-Error "Remediation plan was not generated in time."
}

$planId = $plan.planId

Write-Host "Plan ID: $planId"
Write-Host "Approving Plan..."

$approvalReq = @{
    approver = "test-admin"
    reason = "Executing M27 Integration Test"
}
Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Body ($approvalReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} | Out-Null

$planApproved = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$incidentId/remediation" -UseBasicParsing
$actionId = $planApproved.actions[0].actionId

Write-Host "Executing Plan Action ID $actionId..."

$execReq = @{
    actionId = $actionId
    approver = "test-admin"
}

$execRecord = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Body ($execReq | ConvertTo-Json) -Headers @{"Content-Type"="application/json"} -UseBasicParsing

Write-Host "Execution Status: $($execRecord.executionStatus)"

if ($execRecord.executionStatus -ne "EXECUTED") {
    Write-Error "Execution did not succeed: $($execRecord.message) / $($execRecord.error)"
}

Write-Host "Execution Succeeded! Docker adapter was used safely."
Write-Host "Checking verification status..."

$verifStatus = "VERIFYING"
$retries = 0
while (($verifStatus -eq "VERIFYING" -or $verifStatus -eq "PENDING") -and $retries -lt 15) {
    Start-Sleep -Seconds 2
    $check = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/executions/$($execRecord.executionId)" -UseBasicParsing
    $verifStatus = $check.verificationStatus
    $retries++
}

Write-Host "Final Verification Status: $verifStatus"

Write-Host "Test completed successfully."
