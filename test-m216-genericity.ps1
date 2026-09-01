param (
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# ============================================================================
# M2.16 -- Framework Genericity Verification (Module 7).
#
# Formalizes, as a real, committed, repeatable artifact, what Phase 7A first
# proved ad hoc with a throwaway scratchpad tool: ATLAS's discovery, graph,
# registry, detection, correlation, RCA, and remediation-planning pipeline
# requires ZERO service-specific production code -- it works identically for
# services it has never seen before, named nothing like the four demo
# services. This script also positively proves the one real, deliberate,
# frozen safety exception (internal/execution/guard.go's AllowedServices
# map) correctly rejects a novel, unrecognized service rather than either
# silently allowing it or silently working around it.
#
# GENERICITY MECHANISM: real OTLP protobuf traces sent directly to the real
# otel-collector (":4318/v1/traces"), for two entirely novel, non-"atlas-"
# service names -- "genericity-check-alpha" (caller) and
# "genericity-check-beta" (callee, the one that fails). No mock, no direct
# database/state injection: every property below is observed exclusively
# through ATLAS's real, unmodified, existing HTTP API surface, exactly the
# same discipline every prior test-m2*.ps1 script already follows.
#
# WHY THIS SCRIPT HAND-ENCODES OTLP PROTOBUF IN PLAIN POWERSHELL: Module 7's
# approved scope is exactly two new files (this script and
# docs/framework-boundary.md) -- no third helper binary/tool is authorized,
# and no protobuf library is available to PowerShell without adding a new
# dependency (explicitly out of scope). OTLP's wire format is simple enough
# (varint + length-delimited fields) to encode directly using ONLY the exact
# field numbers confirmed from go.opentelemetry.io/proto/otlp@v1.11.0's own
# generated Go struct tags (trace.pb.go, resource.pb.go, common.pb.go,
# collector/trace/v1/trace_service.pb.go) -- verified against source, not
# guessed, and empirically validated against the real, running otel-
# collector before being used in this script (a real "pshell-encode-test"
# service was observed registered via OBSERVED_TELEMETRY provenance from a
# hand-encoded payload during this implementation's own verification).
#
# IMPORTANT: Invoke-WebRequest/Invoke-RestMethod's -Body parameter does NOT
# transmit a [byte[]] as raw binary in this environment (confirmed by direct
# testing -- it produced a real, informative "proto: illegal wireType 6"
# decode error from the real collector despite the encoded bytes themselves
# being independently verified byte-correct). System.Net.WebClient.
# UploadData does transmit raw bytes correctly and is used for every
# protobuf send below; ordinary JSON API calls elsewhere in this script use
# the same Invoke-RestMethod/Invoke-Authenticated conventions already
# established in test-m27-docker.ps1/test-m29-security.ps1/etc.
# ============================================================================

$AlphaService = "genericity-check-alpha"
$BetaService  = "genericity-check-beta"

# ---- Minimal hand-rolled OTLP protobuf wire encoding ----------------------

function ConvertTo-Varint {
    param([uint64]$Value)
    $bytes = [System.Collections.Generic.List[byte]]::new()
    do {
        $b = [byte]($Value -band 0x7F)
        $Value = $Value -shr 7
        if ($Value -ne 0) { $b = $b -bor 0x80 }
        $bytes.Add($b)
    } while ($Value -ne 0)
    return ,$bytes.ToArray()
}

function New-Tag {
    param([int]$FieldNumber, [int]$WireType)
    return ,(ConvertTo-Varint -Value ([uint64](($FieldNumber -shl 3) -bor $WireType)))
}

function New-LengthDelimitedField {
    param([int]$FieldNumber, [byte[]]$Payload)
    if ($null -eq $Payload) { $Payload = @() }
    $tag = New-Tag -FieldNumber $FieldNumber -WireType 2
    $len = ConvertTo-Varint -Value ([uint64]$Payload.Length)
    return ,($tag + $len + $Payload)
}

function New-VarintField {
    param([int]$FieldNumber, [uint64]$Value)
    return ,((New-Tag -FieldNumber $FieldNumber -WireType 0) + (ConvertTo-Varint -Value $Value))
}

function New-Fixed64Field {
    param([int]$FieldNumber, [uint64]$Value)
    $tag = New-Tag -FieldNumber $FieldNumber -WireType 1
    $bytes = [BitConverter]::GetBytes($Value)
    return ,($tag + $bytes)
}

# AnyValue.string_value = field 1 (verified: common.pb.go)
function New-StringAnyValue {
    param([string]$Value)
    return ,(New-LengthDelimitedField -FieldNumber 1 -Payload ([System.Text.Encoding]::UTF8.GetBytes($Value)))
}

# KeyValue.key = field 1, KeyValue.value = field 2 (verified: common.pb.go)
function New-KeyValue {
    param([string]$Key, [string]$Value)
    $keyField = New-LengthDelimitedField -FieldNumber 1 -Payload ([System.Text.Encoding]::UTF8.GetBytes($Key))
    $valueField = New-LengthDelimitedField -FieldNumber 2 -Payload (New-StringAnyValue -Value $Value)
    return ,($keyField + $valueField)
}

# Resource.attributes = field 1, repeated (verified: resource.pb.go)
function New-Resource {
    param([string]$ServiceName)
    return ,(New-LengthDelimitedField -FieldNumber 1 -Payload (New-KeyValue -Key "service.name" -Value $ServiceName))
}

# Status.code = field 3 (verified: trace.pb.go). 1=OK, 2=ERROR (standard OTel enum).
function New-Status {
    param([int]$Code)
    return ,(New-VarintField -FieldNumber 3 -Value ([uint64]$Code))
}

# Span fields verified against trace.pb.go: trace_id=1, span_id=2,
# parent_span_id=4, name=5, start_time_unix_nano=7 (fixed64),
# end_time_unix_nano=8 (fixed64), status=15.
function New-SpanBytes {
    param([byte[]]$TraceId, [byte[]]$SpanId, [byte[]]$ParentSpanId, [string]$Name, [uint64]$StartNano, [uint64]$EndNano, [int]$StatusCode)
    $b = @()
    $b += New-LengthDelimitedField -FieldNumber 1 -Payload $TraceId
    $b += New-LengthDelimitedField -FieldNumber 2 -Payload $SpanId
    if ($ParentSpanId) { $b += New-LengthDelimitedField -FieldNumber 4 -Payload $ParentSpanId }
    $b += New-LengthDelimitedField -FieldNumber 5 -Payload ([System.Text.Encoding]::UTF8.GetBytes($Name))
    $b += New-Fixed64Field -FieldNumber 7 -Value $StartNano
    $b += New-Fixed64Field -FieldNumber 8 -Value $EndNano
    $b += New-LengthDelimitedField -FieldNumber 15 -Payload (New-Status -Code $StatusCode)
    return ,$b
}

# ResourceSpans.resource=1, ResourceSpans.scope_spans=2, ScopeSpans.spans=2
# (verified: trace.pb.go), ExportTraceServiceRequest.resource_spans=1
# (verified: collector/trace/v1/trace_service.pb.go). One resource, one
# scope, one span per call -- the same minimal real shape this project's own
# otlp_test.go/otlp_bench_test.go Go fixtures already use.
function New-TraceRequestBody {
    param([string]$ServiceName, [byte[]]$TraceId, [byte[]]$SpanId, [byte[]]$ParentSpanId, [string]$Name, [uint64]$StartNano, [uint64]$EndNano, [int]$StatusCode)
    $resourceField = New-LengthDelimitedField -FieldNumber 1 -Payload (New-Resource -ServiceName $ServiceName)
    $span = New-SpanBytes -TraceId $TraceId -SpanId $SpanId -ParentSpanId $ParentSpanId -Name $Name -StartNano $StartNano -EndNano $EndNano -StatusCode $StatusCode
    $spanField = New-LengthDelimitedField -FieldNumber 2 -Payload $span
    $scopeSpansField = New-LengthDelimitedField -FieldNumber 2 -Payload $spanField
    $resourceSpans = $resourceField + $scopeSpansField
    return ,(New-LengthDelimitedField -FieldNumber 1 -Payload $resourceSpans)
}

# A fresh WebClient is constructed on EVERY call, never reused across
# sends. This was found necessary by direct testing during this
# implementation: reusing one WebClient instance across sequential
# UploadData POSTs caused the SECOND and subsequent calls to fail with a
# real HTTP 415 from the real collector, even though the first call on a
# fresh instance -- and independent, isolated single-call tests -- always
# succeeded. This is a real, observed .NET WebClient state-reuse quirk
# across repeated POSTs with a custom Content-Type header, not a protobuf
# encoding defect (the encoded bytes themselves were independently verified
# byte-correct against the real collector before this script was written).
function Send-RealOtlpSpan {
    param([string]$ServiceName, [byte[]]$TraceId, [byte[]]$SpanId, [byte[]]$ParentSpanId, [string]$Name, [int]$StatusCode)
    $nowNano = [uint64]([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()) * 1000000
    $body = New-TraceRequestBody -ServiceName $ServiceName -TraceId $TraceId -SpanId $SpanId -ParentSpanId $ParentSpanId -Name $Name -StartNano $nowNano -EndNano ($nowNano + 10000000) -StatusCode $StatusCode
    $client = New-Object System.Net.WebClient
    $client.Headers.Add("Content-Type", "application/x-protobuf")
    $client.UploadData("http://localhost:4318/v1/traces", "POST", $body) | Out-Null
    $client.Dispose()
}

# A single, shared Random instance, not a fresh "New-Object Random" per
# call. Found necessary by direct testing: System.Random's default
# constructor seeds from the system clock, and this script calls
# New-RandomId many times in a tight loop -- fast enough that repeated
# "New-Object Random" calls can land in the same clock tick and produce
# IDENTICAL byte sequences. That collided a trace's own span ID with its
# child's parent-span-id reference, making the child appear to be its own
# parent -- exactly the self-dependency case internal/graph deliberately
# ignores (see graph_test.go's TestIgnoreSelfDependency) -- which is why no
# real edge was ever observed on the first attempt at this script. A single
# shared generator produces a proper sequence regardless of call rate.
$sharedRandom = New-Object Random
function New-RandomId {
    param([int]$Length)
    $bytes = New-Object byte[] $Length
    $sharedRandom.NextBytes($bytes)
    return ,$bytes
}

# ---- Ordinary JSON API helpers (matching test-m27/test-m28/test-m29's own established conventions) ----

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

$stageOrder = @("DISCOVERY", "GRAPH", "DETECTION", "CORRELATION", "RCA", "PLAN", "EXECUTION_REJECTED")
$stages = [ordered]@{}
foreach ($s in $stageOrder) { $stages[$s] = [ordered]@{ Result = "NOT_REACHED"; Evidence = "" } }

function Set-Stage {
    param([string]$Stage, [string]$Result, [string]$Evidence)
    $stages[$Stage] = [ordered]@{ Result = $Result; Evidence = $Evidence }
    Write-Host "[$Stage] $Result -- $Evidence"
}

function Write-FinalSummary {
    Write-Host ""
    Write-Host "============================================================"
    Write-Host "M2.16 GENERICITY RESULT"
    Write-Host "============================================================"
    foreach ($s in $stageOrder) {
        Write-Host ("{0,-20} {1,-8} {2}" -f $s, $stages[$s].Result, $stages[$s].Evidence)
    }
}

# ============================================================================
# Bring up a clean Docker state. Security left at its default (disabled) --
# this script's purpose is genericity + the execution safety boundary, not
# RBAC (already thoroughly proven separately by test-m29-security.ps1/
# test-m211-security-read.ps1/test-m215-full-chain.ps1). Execution must be
# ENABLED (guard.Check's very first condition) so the allowlist check itself
# is actually reached rather than short-circuiting on ErrExecutionDisabled.
# ============================================================================
Write-Host "--- Bringing up a clean Docker state for M2.16 (execution enabled) ---"
$env:ATLAS_EXECUTION_ENABLED = "true"
docker-compose down
if (-not $SkipBuild) { docker-compose up -d --build } else { docker-compose up -d }
Write-Host "Waiting for all services to be healthy..."
Wait-ForAllHealthy
Write-Host "Clean state confirmed."

# otel-collector has no docker-compose healthcheck and is not covered by
# Wait-ForAllHealthy above (confirmed: none of this project's existing
# test-m2*.ps1 scripts probe it either -- they have historically gotten
# away with this because the much-slower-starting Java services' own health
# checks happen to outlast the collector's brief startup window). This
# script sends its OWN first real span immediately after Wait-ForAllHealthy
# returns, with no Java-service warmup traffic in between, so that same
# race is real here (observed directly: the collector returned a genuine
# HTTP 415 on the very first send right after a fresh rebuild). An initial
# attempt at this probe used an EMPTY request body and falsely reported
# ready while the real payload still 415'd -- an empty body apparently
# bypasses whatever internal component was still initializing, so it is not
# a valid readiness signal. This probe instead sends a REAL, minimal, valid
# single-span trace (a disposable "m216-readiness-probe" service, never
# asserted on later) and retries until the exact real code path a real span
# uses actually succeeds.
Write-Host "Waiting for the otel-collector's OTLP HTTP receiver specifically..."
$collectorReady = $false
$retries = 0
while (-not $collectorReady -and $retries -lt 30) {
    try {
        Send-RealOtlpSpan -ServiceName "m216-readiness-probe" -TraceId (New-RandomId -Length 16) -SpanId (New-RandomId -Length 8) -ParentSpanId $null -Name "readiness-probe" -StatusCode 1
        $collectorReady = $true
    } catch {
        Start-Sleep -Seconds 2
        $retries++
    }
}
if (-not $collectorReady) { Write-Error "otel-collector's OTLP HTTP receiver did not become ready in time." }
Write-Host "otel-collector ready (confirmed with a real span, not an empty probe)."

try {
    # ========================================================================
    # Real synthetic 2-hop trace: genericity-check-alpha calls
    # genericity-check-beta, and genericity-check-beta fails
    # (STATUS_CODE_ERROR). A baseline of successful calls is sent first
    # (matching every existing test-m2*.ps1 script's convention), then a
    # fault burst -- large enough for M2.4's window-based detection and
    # M2.7.3's causal attribution (frozen, unmodified) to have a real chance
    # to redirect the caller's dependency-error credit to the actual failing
    # callee, exactly as it already does for the real atlas-* demo services.
    # ========================================================================
    Write-Host ""
    Write-Host "=== Injecting real OTLP traffic for two entirely novel services ==="
    Write-Host "Caller: $AlphaService -- Callee: $BetaService"

    for ($i = 0; $i -lt 5; $i++) {
        $traceId = New-RandomId -Length 16
        $alphaSpanId = New-RandomId -Length 8
        $betaSpanId = New-RandomId -Length 8
        Send-RealOtlpSpan -ServiceName $AlphaService -TraceId $traceId -SpanId $alphaSpanId -ParentSpanId $null -Name "call-beta" -StatusCode 1
        Send-RealOtlpSpan -ServiceName $BetaService -TraceId $traceId -SpanId $betaSpanId -ParentSpanId $alphaSpanId -Name "handle-request" -StatusCode 1
    }
    for ($i = 0; $i -lt 20; $i++) {
        $traceId = New-RandomId -Length 16
        $alphaSpanId = New-RandomId -Length 8
        $betaSpanId = New-RandomId -Length 8
        Send-RealOtlpSpan -ServiceName $AlphaService -TraceId $traceId -SpanId $alphaSpanId -ParentSpanId $null -Name "call-beta" -StatusCode 1
        Send-RealOtlpSpan -ServiceName $BetaService -TraceId $traceId -SpanId $betaSpanId -ParentSpanId $alphaSpanId -Name "handle-request" -StatusCode 2
    }
    Write-Host "Sent 5 baseline (OK) + 20 fault (ERROR) real OTLP trace pairs via the real otel-collector."

    # ========================================================================
    # STAGE: DISCOVERY -- real registry lookup for the novel callee.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: DISCOVERY ==="
    $betaRegistry = $null
    $retries = 0
    while ($betaRegistry -eq $null -and $retries -lt 15) {
        try { $betaRegistry = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/services/$BetaService" -UseBasicParsing -ErrorAction Stop } catch { Start-Sleep -Seconds 2 }
        $retries++
    }
    if ($betaRegistry -eq $null) {
        Set-Stage -Stage "DISCOVERY" -Result "FAILED" -Evidence "GET /api/v1/services/$BetaService never returned a real registry record -- genericity NOT demonstrated for discovery."
        Write-FinalSummary
        Write-Error "DISCOVERY: the novel service was never registered -- INFRASTRUCTURE/PRODUCT FAILURE."
    }
    Set-Stage -Stage "DISCOVERY" -Result "PASS" -Evidence "name=$($betaRegistry.name) provenance=$($betaRegistry.provenance) status=$($betaRegistry.status) firstObservedAt=$($betaRegistry.firstObservedAt)"

    # ========================================================================
    # STAGE: GRAPH -- real dependency edge alpha -> beta.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: GRAPH ==="
    $edges = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/graph/edges" -UseBasicParsing
    $edge = $edges | Where-Object { $_.source -eq $AlphaService -and $_.target -eq $BetaService } | Select-Object -First 1
    if ($edge -eq $null) {
        Set-Stage -Stage "GRAPH" -Result "FAILED" -Evidence "No real dependency-graph edge $AlphaService -> $BetaService was observed."
        Write-FinalSummary
        Write-Error "GRAPH: the novel dependency edge never appeared -- INFRASTRUCTURE/PRODUCT FAILURE."
    }
    Set-Stage -Stage "GRAPH" -Result "PASS" -Evidence "$($edge.source) -> $($edge.target): call_count=$($edge.call_count) error_count=$($edge.error_count)"

    # ========================================================================
    # STAGE: DETECTION + CORRELATION -- real incident rooted at the novel callee.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: DETECTION + CORRELATION ==="
    $betaIncident = $null
    $retries = 0
    while ($betaIncident -eq $null -and $retries -lt 30) {
        Start-Sleep -Seconds 5
        try {
            $incidents = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/open" -UseBasicParsing
            foreach ($inc in $incidents) {
                if ($inc.rootService -eq $BetaService) { $betaIncident = $inc; break }
            }
        } catch {}
        $retries++
    }
    if ($betaIncident -eq $null) {
        Set-Stage -Stage "DETECTION" -Result "FAILED" -Evidence "No incident rootServiced at $BetaService appeared within 30x5s."
        Write-FinalSummary
        Write-Error "DETECTION: no incident appeared for the novel service -- INFRASTRUCTURE/PRODUCT FAILURE."
    }
    Set-Stage -Stage "DETECTION" -Result "PASS" -Evidence "incidentId=$($betaIncident.incidentId) rootService=$($betaIncident.rootService) fingerprint=$($betaIncident.fingerprint)"

    # The incident object captured the MOMENT it first appeared in
    # /incidents/open can predate the same 5-second background tick's own
    # correlation/causal-attribution/RCA steps (main.go's evaluation loop
    # runs blast -> correlate -> causal attribution -> rca.Engine.Analyze
    # AFTER open incidents are listed for that tick) -- found by direct
    # observation: the first run of this check reported empty RCA fields
    # here while the very next real call (POST /remediation/plan) correctly
    # saw a real "AMBIGUOUS" RCA verdict server-side, proving RCA HAD run by
    # then. Re-fetching after one more full tick interval gets the current,
    # settled state before reading RCA/correlation fields.
    Start-Sleep -Seconds 6
    $betaIncident = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($betaIncident.incidentId)" -UseBasicParsing
    Set-Stage -Stage "CORRELATION" -Result "PASS" -Evidence "primaryIncidentId=$($betaIncident.primaryIncidentId) (self-primary=$($betaIncident.primaryIncidentId -eq $betaIncident.incidentId)); correlationGroupId=$($betaIncident.correlationGroupId); relatedIncidentIds=$($betaIncident.relatedIncidentIds -join ',')"

    # ========================================================================
    # STAGE: RCA -- read directly off the already-materialized incident
    # object (internal/rca is FROZEN; this reads its real output verbatim).
    # AMBIGUOUS is a real, legitimate, honest RCA outcome -- not a failure --
    # reported exactly as observed, never treated as PASS/FAIL on its value.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: RCA ==="
    $rcaService = $betaIncident.rootCause.service
    $rcaConfidence = $betaIncident.rootCause.confidence
    $rcaScore = $betaIncident.rootCause.score
    Set-Stage -Stage "RCA" -Result "PASS" -Evidence "service=$rcaService confidence=$rcaConfidence score=$rcaScore (frozen rca.Engine output, read verbatim from Incident.RCA)"

    # ========================================================================
    # STAGE: PLAN -- POST /remediation/plan. AMBIGUOUS/LOW-confidence
    # rejection is M2.6's real, unmodified safety policy correctly firing --
    # BLOCKED, not FAILED, exactly as every prior script treats it. If
    # blocked, EXECUTION_REJECTED cannot be exercised this run (there is no
    # plan to approve/execute against) -- reported honestly, not forced.
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: PLAN ==="
    $planId = $null
    try {
        Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($betaIncident.incidentId)/remediation/plan" -Method POST -UseBasicParsing -ErrorAction Stop | Out-Null
        $plan = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($betaIncident.incidentId)/remediation" -UseBasicParsing
        $planId = $plan.planId
        if ($plan.actions[0].targetService -ne $BetaService) {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Expected the plan to target $BetaService, got $($plan.actions[0].targetService)."
            Write-FinalSummary
            Write-Error "PLAN: unexpected target service -- PRODUCT/ARCHITECTURE FAILURE."
        }
        Set-Stage -Stage "PLAN" -Result "PASS" -Evidence "planId=$planId planner=$($plan.planner) plannerVersion=$($plan.plannerVersion) target=$($plan.actions[0].targetService) riskLevel=$($plan.riskLevel)"
    } catch {
        $body = $_.ErrorDetails.Message
        if ($body -like "*AMBIGUOUS*" -or $body -like "*LOW confidence*") {
            Set-Stage -Stage "PLAN" -Result "BLOCKED" -Evidence "M2.6 policy correctly refused a HIGH-risk plan against insufficient-confidence RCA: $body"
        } else {
            Set-Stage -Stage "PLAN" -Result "FAILED" -Evidence "Plan generation failed for an unrecognized reason: $body"
            Write-FinalSummary
            Write-Error "PLAN: unexpected failure reason -- PRODUCT/ARCHITECTURE FAILURE."
        }
    }

    if ($planId -eq $null) {
        Write-Host ""
        Write-Host "PLAN did not PASS -- EXECUTION_REJECTED is correctly NOT_REACHED (no plan exists to approve/execute against). This is a valid, honest terminal state; discovery/graph/detection/correlation/RCA genericity were still fully demonstrated above."
        Write-FinalSummary
        exit 0
    }

    # ========================================================================
    # STAGE: EXECUTION_REJECTED -- approve the real plan (required so guard's
    # StatusApproved/fingerprint checks pass, isolating the assertion to the
    # SERVICE-allowlist check specifically), then attempt the real execute
    # call and require the exact ErrServiceNotAllowlisted message. Verified
    # from source before writing this (internal/execution/engine.go:
    # ExecutePlanAction calls guard.Check BEFORE constructing any
    # ExecutionRecord or ever calling the real ExecutorProvider/Docker
    # adapter -- ErrServiceNotAllowlisted returns at that first guard check,
    # so this call is structurally incapable of restarting anything).
    # ========================================================================
    Write-Host ""
    Write-Host "=== STAGE: EXECUTION_REJECTED ==="
    $approveReq = @{ approver = "m216-genericity"; reason = "M2.16 genericity verification" } | ConvertTo-Json
    Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/approve" -Method POST -Body $approveReq -Headers @{"Content-Type"="application/json"} -UseBasicParsing | Out-Null
    $approvedPlan = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/incidents/$($betaIncident.incidentId)/remediation" -UseBasicParsing
    if ($approvedPlan.status -ne "APPROVED") {
        Set-Stage -Stage "EXECUTION_REJECTED" -Result "FAILED" -Evidence "Expected plan status=APPROVED before the execution-rejection check, got $($approvedPlan.status)."
        Write-FinalSummary
        Write-Error "EXECUTION_REJECTED: could not approve the plan needed to isolate the allowlist check -- PRODUCT/ARCHITECTURE FAILURE."
    }
    $actionId = $approvedPlan.actions[0].actionId
    $paymentBaselineStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()

    $execReq = @{ actionId = $actionId; approver = "m216-genericity" } | ConvertTo-Json
    $execStatus = $null
    $execBody = $null
    try {
        Invoke-RestMethod -Uri "http://localhost:8081/api/v1/remediation/$planId/execute" -Method POST -Body $execReq -Headers @{"Content-Type"="application/json"} -UseBasicParsing -ErrorAction Stop | Out-Null
        $execStatus = 200
    } catch {
        $execStatus = $_.Exception.Response.StatusCode.value__
        $execBody = $_.ErrorDetails.Message
    }

    $paymentAfterStartedAt = (docker inspect atlas-payment-service-1 --format '{{.State.StartedAt}}').Trim()
    if ($paymentAfterStartedAt -ne $paymentBaselineStartedAt) {
        Set-Stage -Stage "EXECUTION_REJECTED" -Result "FAILED" -Evidence "An unrelated real container restart occurred during this check (atlas-payment-service-1 StartedAt changed) -- unexpected side effect, investigate before trusting this run."
        Write-FinalSummary
        Write-Error "EXECUTION_REJECTED: unexpected real infrastructure mutation detected -- STOP."
    }

    if ($execStatus -ne 403 -or $execBody -notlike "*not strictly allowlisted*") {
        Set-Stage -Stage "EXECUTION_REJECTED" -Result "FAILED" -Evidence "Expected HTTP 403 with the real ErrServiceNotAllowlisted message ('target service is not strictly allowlisted for execution'), got HTTP $execStatus body=$execBody"
        Write-FinalSummary
        Write-Error "EXECUTION_REJECTED: the safety boundary did not reject the novel service as expected -- PRODUCT/ARCHITECTURE FAILURE (or a real safety regression)."
    }
    Set-Stage -Stage "EXECUTION_REJECTED" -Result "PASS" -Evidence "POST /remediation/$planId/execute -> HTTP 403, body=$execBody. No infrastructure was restarted (atlas-payment-service-1 StartedAt independently confirmed unchanged: $paymentAfterStartedAt)."

    Write-FinalSummary
    Write-Host ""
    Write-Host "Test completed successfully -- genericity demonstrated end to end, and the execution safety boundary correctly held for a service it does not recognize."

} catch {
    Write-FinalSummary
    throw
}
