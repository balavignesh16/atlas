Write-Host "M2.6 Docker E2E Test Script"
Write-Host "==========================="

Write-Host "Waiting for Gateway..."
$healthy = $false
while (-not $healthy) {
    try {
        $res = Invoke-RestMethod -Uri "http://localhost:8083/actuator/health" -UseBasicParsing -ErrorAction Stop
        if ($res.status -eq "UP") { $healthy = $true }
    } catch {
        Start-Sleep -Seconds 2
    }
}
Write-Host "Services are UP."

function Send-Normal {
    for ($i=0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M26-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' | Out-Null
        } catch {}
    }
}

function Send-PaymentFailure {
    for ($i=0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M26-PAYFAIL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 1 | Out-Null
        } catch {}
    }
}

Write-Host "1. Generate Payment Incident"
Send-Normal
Send-PaymentFailure
Start-Sleep -Seconds 10

$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
$payInc = ""
if ($incidents -is [array] -and $incidents.Count -gt 0) {
    $payInc = $incidents[0].incidentId
} elseif ($null -ne $incidents -and $null -ne $incidents.incidentId) {
    $payInc = $incidents.incidentId
}

if ($payInc -ne "") {
    Write-Host "Found Incident: $payInc"
    
    Write-Host "2. AI Analysis"
    try {
        $analysis = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/analyze" -Method POST
        Write-Host "AI Analysis output: $($analysis | ConvertTo-Json -Depth 2)"
    } catch {
        Write-Host "AI Analysis POST failed: $_"
    }

    Write-Host "3. Remediation Planning"
    try {
        $planRes = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/remediation/plan" -Method POST
        if ($planRes.status -eq "VALIDATED") {
            Write-Host "Plan generation Successful! Status: $($planRes.status)"
            $planId = $planRes.planId

            Write-Host "4. Approve Plan"
            $approveRes = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Body '{"reason": "Testing approval"}' -ContentType "application/json"
            Write-Host "Approval Result: $($approveRes.message)"
            
            if ($approveRes.plan.status -eq "APPROVED") {
                Write-Host "Plan status updated to APPROVED."
            }

            Write-Host "5. Verify Dry-Run Endpoint"
            $dryRes = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/remediation" -Method GET
            if ($dryRes.executionSupported -eq $false) {
                Write-Host "Execution Supported flag correctly set to false."
            }
        }
    } catch {
        Write-Host "Remediation generation Failed: $_"
    }
}

Write-Host "6. M2.5 Regression"
try {
    $analysisRes = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/analysis" -Method GET
    if ($analysisRes) {
        Write-Host "M2.5 Analysis exists."
    }
} catch {
    Write-Host "M2.5 Regression GET failed: $_"
}

Write-Host "7. M2.4 Regression"
Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET | ConvertTo-Json -Depth 5 | Write-Host

Write-Host "8. M2.3 Regression"
Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph" -Method GET | ConvertTo-Json -Depth 3 | Write-Host

Write-Host "Test complete."
