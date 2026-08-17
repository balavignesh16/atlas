# test-m23-docker.ps1
$ErrorActionPreference = "Stop"

Write-Host "Starting ATLAS M2.3 Verification..."
docker-compose down -v
docker-compose up -d --build

Write-Host "Waiting for services to become healthy..."
$healthy = $false
while (-not $healthy) {
    try {
        $gatewayHealth = Invoke-RestMethod -Uri "http://localhost:8083/actuator/health" -UseBasicParsing -ErrorAction Stop
        $healthy = ($gatewayHealth.status -eq "UP")
    } catch {
        Start-Sleep -Seconds 2
    }
}
Write-Host "Services are UP."
Start-Sleep -Seconds 5

# TEST 1 & 2: Successful Order -> Trace and Tree
Write-Host "Test 1: Successful Order (ATLAS-M23-TRACE-PROOF)"
$successReq = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M23-TRACE-PROOF"} -Body '{"productId":"P100","quantity":1}' -UseBasicParsing
Write-Host "Order successful."

# TEST 6: Payment Timeout
Write-Host "Test 6: Payment Timeout (ATLAS-M23-TIMEOUT-PROOF)"
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M23-TIMEOUT-PROOF"} -Body '{"productId":"P100","quantity":3}' -UseBasicParsing | Out-Null
} catch {}

# TEST 7: Inventory Conflict
Write-Host "Test 7: Inventory Conflict (ATLAS-M23-INVENTORY-PROOF)"
try {
    Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"; "X-Correlation-ID"="ATLAS-M23-INVENTORY-PROOF"} -Body '{"productId":"P100","quantity":100}' -UseBasicParsing | Out-Null
} catch {}

# TEST 5: Multiple calls to aggregate edges
Write-Host "Test 5: Multiple calls"
for ($i = 0; $i -lt 5; $i++) {
    try {
        Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"} -Body '{"productId":"P100","quantity":1}' -UseBasicParsing | Out-Null
    } catch {}
}

Write-Host "Waiting for telemetry processing (5s)..."
Start-Sleep -Seconds 5

# Retrieve Trace
Write-Host "Querying /api/v1/events for trace ID..."
$events = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/events" -UseBasicParsing
$traceId = ($events | Where-Object { $_.attributes.correlation_id -eq "ATLAS-M23-TRACE-PROOF" } | Select-Object -First 1).trace_id

Write-Host "Found Trace ID: $traceId"

Write-Host "Retrieving Trace Summary..."
$trace = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/correlations/traces/$traceId" -UseBasicParsing
$trace | ConvertTo-Json -Depth 5 | Out-File -FilePath trace_summary.json -Encoding UTF8

Write-Host "Retrieving Trace Tree..."
$tree = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/correlations/traces/$traceId/tree" -UseBasicParsing
$tree | ConvertTo-Json -Depth 10 | Out-File -FilePath trace_tree.json -Encoding UTF8

Write-Host "Retrieving Trace Timeline..."
$timeline = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/correlations/traces/$traceId/timeline" -UseBasicParsing
$timeline | ConvertTo-Json -Depth 5 | Out-File -FilePath trace_timeline.json -Encoding UTF8

# Retrieve Failure Traces
$timeoutTraceId = ($events | Where-Object { $_.attributes.correlation_id -eq "ATLAS-M23-TIMEOUT-PROOF" } | Select-Object -First 1).trace_id
if ($timeoutTraceId) {
    Write-Host "Retrieving Timeout Trace Summary ($timeoutTraceId)..."
    $timeoutTrace = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/correlations/traces/$timeoutTraceId" -UseBasicParsing
    $timeoutTrace | ConvertTo-Json -Depth 5 | Out-File -FilePath timeout_trace.json -Encoding UTF8
}

$invTraceId = ($events | Where-Object { $_.attributes.correlation_id -eq "ATLAS-M23-INVENTORY-PROOF" } | Select-Object -First 1).trace_id
if ($invTraceId) {
    Write-Host "Retrieving Inventory Trace Summary ($invTraceId)..."
    $invTrace = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/correlations/traces/$invTraceId" -UseBasicParsing
    $invTrace | ConvertTo-Json -Depth 5 | Out-File -FilePath inventory_trace.json -Encoding UTF8
}

Write-Host "Retrieving Dependency Graph..."
$graph = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph" -UseBasicParsing
$graph | ConvertTo-Json -Depth 5 | Out-File -FilePath graph_snapshot.json -Encoding UTF8

Write-Host "Retrieving Edges..."
$edges = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph/edges" -UseBasicParsing
$edges | ConvertTo-Json -Depth 5 | Out-File -FilePath graph_edges.json -Encoding UTF8

Write-Host "Retrieving Service Dependencies for atlas-order-service..."
$orderDeps = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph/services/atlas-order-service" -UseBasicParsing
$orderDeps | ConvertTo-Json -Depth 5 | Out-File -FilePath order_deps.json -Encoding UTF8

# TEST 10: Intelligence Engine Outage
Write-Host "Stopping Intelligence Engine..."
docker-compose stop intelligence-engine
Start-Sleep -Seconds 2

Write-Host "Sending traffic while engine is down..."
$downReq = Invoke-WebRequest -Uri "http://localhost:8083/api/orders" -Method POST -Headers @{"Content-Type"="application/json"} -Body '{"productId":"P100","quantity":1}' -UseBasicParsing
Write-Host "Business traffic succeeded while Intelligence Engine was down."

Write-Host "Restarting Intelligence Engine..."
docker-compose start intelligence-engine
Start-Sleep -Seconds 10

Write-Host "M2.3 Verification Complete!"
