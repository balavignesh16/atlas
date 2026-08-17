Write-Host "M2.4 Docker E2E Test Script"
Write-Host "==========================="

function Send-Normal {
    for ($i=0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M24-NORMAL-$i"} -Body '{"productId":"P100","quantity":1}' | Out-Null
        } catch {}
    }
}

function Send-PaymentFailure {
    for ($i=0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M24-PAYFAIL-$i"} -Body '{"productId":"P100","quantity":1}' -TimeoutSec 1 | Out-Null
        } catch {}
    }
}

function Send-InventoryFailure {
    for ($i=0; $i -lt 15; $i++) {
        try {
            Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M24-INVFAIL-$i"} -Body '{"productId":"P100","quantity":9999}' | Out-Null
        } catch {}
    }
}

function Send-FalsePositive {
    try {
        Invoke-RestMethod -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M24-FP"} -Body '{"productId":"P100","quantity":9999}' | Out-Null
    } catch {}
}

Write-Host "1. Normal Traffic"
Send-Normal
Start-Sleep -Seconds 6

Write-Host "2-5. Payment Failure & RCA"
Send-PaymentFailure
Start-Sleep -Seconds 6
$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
Write-Host "Open Incidents after Payment Failure:"
$incidents | ConvertTo-Json -Depth 5 | Write-Host
if ($incidents.Count -gt 0) {
    $payInc = $incidents[0].incidentId
    Write-Host "RCA for $payInc :"
    Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/rca" -Method GET | ConvertTo-Json -Depth 5 | Write-Host
    
    Write-Host "Evidence for $payInc :"
    Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$payInc/evidence" -Method GET | ConvertTo-Json -Depth 2 | Write-Host
}

Write-Host "6-7. Inventory Failure & RCA"
Send-InventoryFailure
Start-Sleep -Seconds 6
$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
Write-Host "Open Incidents after Inventory Failure:"
$incidents | ConvertTo-Json -Depth 5 | Write-Host

Write-Host "10. Ambiguous RCA (Simultaneous Failures)"
Send-PaymentFailure
Send-InventoryFailure
Start-Sleep -Seconds 6
$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
$incidents | ConvertTo-Json -Depth 5 | Write-Host

Write-Host "14. Recovery"
Write-Host "Waiting for 35s to test recovery..."
Start-Sleep -Seconds 35
$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
Write-Host "Open Incidents after recovery: $($incidents.Count)"

Write-Host "15. False Positive (Single failure)"
Send-FalsePositive
Start-Sleep -Seconds 6
$incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -Method GET
Write-Host "Open Incidents after 1 failure: $($incidents.Count)"

Write-Host "17-18. Intelligence Engine Outage & Recovery"
docker stop atlas-intelligence-engine-1
Start-Sleep -Seconds 5
Send-Normal
docker start atlas-intelligence-engine-1
Start-Sleep -Seconds 5

Write-Host "19-20. Regression tests"
Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph" -Method GET | ConvertTo-Json -Depth 3 | Write-Host
