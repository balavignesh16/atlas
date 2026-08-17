$ErrorActionPreference = "Stop"

Write-Host "M2.2 Docker Integration Test" -ForegroundColor Cyan

Write-Host "Restarting Docker Compose..."
docker-compose down
docker-compose up -d --build

Write-Host "Waiting for services to become healthy..."
$healthy = $false
$retries = 0
while (-not $healthy -and $retries -lt 30) {
    try {
        $gatewayHealth = Invoke-RestMethod -Uri "http://localhost:8083/actuator/health" -UseBasicParsing
        $engineHealth = Invoke-RestMethod -Uri "http://localhost:8081/health" -UseBasicParsing
        if ($gatewayHealth.status -eq "UP" -and $engineHealth.status -eq "UP") {
            $healthy = $true
        }
    } catch {
        Start-Sleep -Seconds 2
        $retries++
    }
}
if (-not $healthy) {
    Write-Host "Services did not start correctly." -ForegroundColor Red
    exit 1
}

Write-Host "Services are UP." -ForegroundColor Green
Start-Sleep -Seconds 5

# Test 1: Successful Order
Write-Host "Test 1: Successful Order (ATLAS-M22-TRACE-PROOF)"
$successResponse = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M22-TRACE-PROOF"} -Body '{"productId":"P100","quantity":1}' -UseBasicParsing
if ($successResponse.StatusCode -ne 201) {
    Write-Host "Order creation failed!" -ForegroundColor Red
    exit 1
}
Write-Host "Order successful."

# Test 2: Payment Timeout
Write-Host "Test 2: Payment Timeout (ATLAS-M22-TIMEOUT-PROOF)"
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M22-TIMEOUT-PROOF"} -Body '{"productId":"P100","quantity":3}' -UseBasicParsing | Out-Null
} catch {}

# Test 3: Inventory Conflict
Write-Host "Test 3: Inventory Conflict (ATLAS-M22-INVENTORY-PROOF)"
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M22-INVENTORY-PROOF"} -Body '{"productId":"P100","quantity":100}' -UseBasicParsing | Out-Null
} catch {}

Write-Host "Waiting for Collector to export telemetry (10s)..."
Start-Sleep -Seconds 10

# Ingestion Verification
Write-Host "Querying Intelligence Engine for events..."
$eventsResponse = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/events" -UseBasicParsing

# Find Trace ID from Correlation ID
$traceId = $null
foreach ($e in $eventsResponse) {
    if ($e.attributes.correlation_id -eq "ATLAS-M22-TRACE-PROOF") {
        $traceId = $e.trace_id
        break
    }
}

if (-not $traceId) {
    Write-Host "Failed to find trace ID for correlation ID ATLAS-M22-TRACE-PROOF" -ForegroundColor Red
    exit 1
}
Write-Host "Found Trace ID: $traceId" -ForegroundColor Green

# Trace Verification API
Write-Host "Querying /api/v1/events/trace/$traceId"
$traceEvents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/events/trace/$traceId" -UseBasicParsing

$services = @()
foreach ($e in $traceEvents) {
    if ($e.service_name -notin $services) {
        $services += $e.service_name
    }
}

Write-Host "Services involved in trace: $($services -join ', ')"
if ($services.Count -lt 4) {
    Write-Host "Missing services in trace!" -ForegroundColor Red
    exit 1
}

# Metric Verification API
Write-Host "Querying /api/v1/events/metrics"
$metricEvents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/events/metrics" -UseBasicParsing
if ($metricEvents.Count -eq 0) {
    Write-Host "No metrics found!" -ForegroundColor Red
    exit 1
}
Write-Host "Found $($metricEvents.Count) metric events." -ForegroundColor Green

# Intelligence Engine Down Test
Write-Host "Stopping Intelligence Engine..."
docker-compose stop atlas-intelligence-engine

Write-Host "Sending traffic while engine is down..."
$downResponse = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M22-ENGINE-DOWN"} -Body '{"productId":"P100","quantity":1}' -UseBasicParsing
if ($downResponse.StatusCode -ne 201) {
    Write-Host "Order failed while engine was down! Resilience broken!" -ForegroundColor Red
    exit 1
}
Write-Host "Business traffic succeeded while Intelligence Engine was down." -ForegroundColor Green

Write-Host "Restarting Intelligence Engine..."
docker-compose start atlas-intelligence-engine
Start-Sleep -Seconds 5

Write-Host "M2.2 Verification Complete!" -ForegroundColor Green
