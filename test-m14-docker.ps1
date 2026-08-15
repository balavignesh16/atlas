$ErrorActionPreference = "Stop"

Write-Host "Waiting for services to be healthy..."
$retryCount = 0
$maxRetries = 30
$healthy = $false

while (-not $healthy -and $retryCount -lt $maxRetries) {
    Start-Sleep -Seconds 2
    $retryCount++
    
    try {
        $gatewayHealth = Invoke-RestMethod -Uri "http://localhost:8083/actuator/health" -UseBasicParsing -ErrorAction Stop
        $orderHealth = Invoke-RestMethod -Uri "http://localhost:8084/actuator/health" -UseBasicParsing -ErrorAction Stop
        $inventoryHealth = Invoke-RestMethod -Uri "http://localhost:8085/actuator/health" -UseBasicParsing -ErrorAction Stop
        $paymentHealth = Invoke-RestMethod -Uri "http://localhost:8086/actuator/health" -UseBasicParsing -ErrorAction Stop
        
        if ($gatewayHealth.status -eq "UP" -and $orderHealth.status -eq "UP" -and $inventoryHealth.status -eq "UP" -and $paymentHealth.status -eq "UP") {
            $healthy = $true
            Write-Host "All services are UP!"
        }
    } catch {
        Write-Host "Waiting... ($retryCount/$maxRetries)"
    }
}

if (-not $healthy) {
    Write-Error "Services failed to become healthy in time."
    exit 1
}

Write-Host "`n--- Running M1.4 Docker Integration Tests ---`n"

$headers = @{
    "Content-Type" = "application/json"
    "X-Correlation-ID" = "ATLAS-M14-TEST"
}

# Test 1: Successful end-to-end flow
Write-Host "Test 1: Successful end-to-end flow"
$body1 = '{"productId":"P100","quantity":1}'
$response1 = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body1 -UseBasicParsing
if ($response1.StatusCode -ne 201) { Write-Error "Test 1 Failed" ; exit 1 }
Write-Host "Test 1 Passed"

# Test 2: Payment Decline (402)
Write-Host "Test 2: Payment Decline Sandbox (Quantity=2 -> 402)"
$body2 = '{"productId":"P100","quantity":2}'
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body2 -UseBasicParsing | Out-Null
    Write-Error "Test 2 Failed: Expected 402"
    exit 1
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 402) {
        Write-Error "Test 2 Failed: Expected 402, got $($_.Exception.Response.StatusCode.value__)"
        exit 1
    }
    Write-Host "Test 2 Passed: Received 402"
}

# Test 3: Payment Timeout (504)
Write-Host "Test 3: Payment Timeout Sandbox (Quantity=3 -> 504)"
$body3 = '{"productId":"P100","quantity":3}'
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body3 -UseBasicParsing | Out-Null
    Write-Error "Test 3 Failed: Expected 504"
    exit 1
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 504) {
        Write-Error "Test 3 Failed: Expected 504, got $($_.Exception.Response.StatusCode.value__)"
        exit 1
    }
    Write-Host "Test 3 Passed: Received 504"
}

# Test 4: Payment Server Error (500 -> 502)
Write-Host "Test 4: Payment Server Error Sandbox (Quantity=4 -> 502)"
$body4 = '{"productId":"P100","quantity":4}'
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body4 -UseBasicParsing | Out-Null
    Write-Error "Test 4 Failed: Expected 502"
    exit 1
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 502) {
        Write-Error "Test 4 Failed: Expected 502, got $($_.Exception.Response.StatusCode.value__)"
        exit 1
    }
    Write-Host "Test 4 Passed: Received 502"
}

# Test 5: Payment Unavailable (Stop Container -> 503/504)
Write-Host "Test 5: Payment Unavailable (Stop Container -> 503/504)"
docker-compose stop payment-service | Out-Null
Start-Sleep -Seconds 3

try {
    $body5 = '{"productId":"P100","quantity":1}'
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body5 -UseBasicParsing | Out-Null
    Write-Error "Test 5 Failed: Expected 503 or 504"
    exit 1
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    if ($statusCode -ne 503 -and $statusCode -ne 504) {
        Write-Error "Test 5 Failed: Expected 503 or 504, got $statusCode"
        exit 1
    }
    Write-Host "Test 5 Passed: Received $statusCode"
}

# Test 6: Recovery
Write-Host "Test 6: Recovery after Payment restart"
docker-compose start payment-service | Out-Null

$retryCount = 0
$healthy = $false
while (-not $healthy -and $retryCount -lt 15) {
    Start-Sleep -Seconds 2
    $retryCount++
    try {
        $paymentHealth = Invoke-RestMethod -Uri "http://localhost:8086/actuator/health" -UseBasicParsing -ErrorAction Stop
        if ($paymentHealth.status -eq "UP") { $healthy = $true }
    } catch {}
}

$body6 = '{"productId":"P100","quantity":1}'
$response6 = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body6 -UseBasicParsing
if ($response6.StatusCode -ne 201) { Write-Error "Test 6 Failed" ; exit 1 }
Write-Host "Test 6 Passed: Order created successfully after recovery without restarting Gateway/Order."

Write-Host "`nAll M1.4 Docker tests PASSED!"
