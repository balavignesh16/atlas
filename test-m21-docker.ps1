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
            Write-Host "All Java services are UP!"
        }
    } catch {
        Write-Host "Waiting... ($retryCount/$maxRetries)"
    }
}

if (-not $healthy) {
    Write-Error "Services failed to become healthy in time."
    exit 1
}

Write-Host "`n--- Running M2.1 Telemetry Tests ---`n"

$headers = @{
    "Content-Type" = "application/json"
}

# Test 1: Successful Order
Write-Host "Test 1: Successful Order (ATLAS-M21-SUCCESS)"
$headers["X-Correlation-ID"] = "ATLAS-M21-SUCCESS"
$body1 = '{"productId":"P100","quantity":1}'
$response1 = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body1 -UseBasicParsing
if ($response1.StatusCode -ne 201) { Write-Error "Test 1 Failed" ; exit 1 }
Write-Host "Test 1 Passed"

# Test 2: Payment Timeout (Quantity=3 simulates timeout)
Write-Host "Test 2: Payment Timeout (ATLAS-M21-TIMEOUT)"
$headers["X-Correlation-ID"] = "ATLAS-M21-TIMEOUT"
$body2 = '{"productId":"P100","quantity":3}'
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body2 -UseBasicParsing | Out-Null
    Write-Error "Test 2 Failed: Expected 504"
    exit 1
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 504) {
        Write-Error "Test 2 Failed: Expected 504, got $($_.Exception.Response.StatusCode.value__)"
        exit 1
    }
    Write-Host "Test 2 Passed: Received 504"
}

# Test 3: Inventory Failure (Quantity=100 simulates 409)
Write-Host "Test 3: Inventory Failure (ATLAS-M21-INV-FAIL)"
$headers["X-Correlation-ID"] = "ATLAS-M21-INV-FAIL"
$body3 = '{"productId":"P100","quantity":100}'
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body3 -UseBasicParsing | Out-Null
    Write-Error "Test 3 Failed: Expected 409"
    exit 1
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 409) {
        Write-Error "Test 3 Failed: Expected 409, got $($_.Exception.Response.StatusCode.value__)"
        exit 1
    }
    Write-Host "Test 3 Passed: Received 409"
}

# Test 4: Collector Outage Tolerance
Write-Host "Test 4: Stopping OTel Collector..."
docker-compose stop otel-collector | Out-Null
Start-Sleep -Seconds 5

Write-Host "Test 4: Verifying Business Traffic with Collector Down (ATLAS-M21-COLLECTOR-DOWN)"
$headers["X-Correlation-ID"] = "ATLAS-M21-COLLECTOR-DOWN"
$body4 = '{"productId":"P100","quantity":1}'
$response4 = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body4 -UseBasicParsing
if ($response4.StatusCode -ne 201) { Write-Error "Test 4 Failed" ; exit 1 }
Write-Host "Test 4 Passed: Order succeeded despite collector outage"

# Test 5: Collector Recovery
Write-Host "Test 5: Starting OTel Collector..."
docker-compose start otel-collector | Out-Null
Start-Sleep -Seconds 10

Write-Host "Test 5: Verifying Business Traffic after Collector Recovery (ATLAS-M21-COLLECTOR-UP)"
$headers["X-Correlation-ID"] = "ATLAS-M21-COLLECTOR-UP"
$body5 = '{"productId":"P100","quantity":1}'
$response5 = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers $headers -Body $body5 -UseBasicParsing
if ($response5.StatusCode -ne 201) { Write-Error "Test 5 Failed" ; exit 1 }
Write-Host "Test 5 Passed: Order succeeded"

Write-Host "`nAll M2.1 Telemetry Verification Tests Complete!"
Write-Host "Now verify Collector Logs for Trace/Metric presence: docker-compose logs otel-collector"
