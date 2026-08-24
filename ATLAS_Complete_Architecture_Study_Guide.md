# ATLAS --- Complete Architecture & Study Guide

## Beginner → Full-System Understanding

> **Purpose of this document:** Teach the ATLAS project from zero.\
> You should be able to read this without knowing what ATLAS is, why it
> exists, or how the individual services fit together.

------------------------------------------------------------------------

# 1. What is ATLAS?

**ATLAS is an intelligence and observability platform for distributed
applications.**

Its job is not simply to run an application. It watches what happens
inside a distributed system, reconstructs what happened, detects
incidents, determines evidence-based likely causes, explains those
incidents with AI, and creates safe remediation plans.

The architecture evolved through these stages:

``` text
RUN THE SYSTEM
     ↓
OBSERVE THE SYSTEM
     ↓
COLLECT TELEMETRY
     ↓
NORMALIZE TELEMETRY
     ↓
RECONSTRUCT TRACES
     ↓
BUILD DEPENDENCY GRAPH
     ↓
DETECT INCIDENTS
     ↓
DETERMINE EVIDENCE-BASED RCA
     ↓
EXPLAIN RCA WITH AI
     ↓
CREATE SAFE REMEDIATION PLAN
     ↓
[M2.7] CONTROLLED HUMAN-APPROVED EXECUTION
```

The important idea is that **each milestone adds one layer of
intelligence**.

ATLAS is deliberately designed so that higher-level intelligence does
not bypass lower-level evidence.

------------------------------------------------------------------------

# 2. The Problem ATLAS Solves

Modern applications are rarely one program.

A single user request may travel through:

``` text
Client
  ↓
Gateway
  ↓
Order Service
  ├──→ Inventory Service
  └──→ Payment Service
```

If payment becomes slow or fails, the user may only see:

``` text
HTTP 504
```

But an engineer needs to know:

-   What request failed?
-   Which service actually failed?
-   Which service called it?
-   When did the failure begin?
-   Did the failure propagate upstream?
-   Which services were affected?
-   Is the apparent root cause actually the root cause?
-   What evidence supports that conclusion?
-   What should an engineer investigate?
-   What remediation could be proposed safely?

ATLAS builds the machinery to answer those questions.

------------------------------------------------------------------------

# 3. The Most Important Mental Model

Think of ATLAS as a pipeline:

``` text
                OBSERVATION
                    │
                    ▼
             OpenTelemetry
                    │
                    ▼
             OTel Collector
                    │
                    ▼
        Intelligence Engine
                    │
          ┌─────────┴─────────┐
          ▼                   ▼
     Normalization        Event Buffer
          │
          ▼
     Correlation
          │
       ┌──┴───┐
       ▼      ▼
    Traces   Graph
       │      │
       └──┬───┘
          ▼
   Incident Detection
          │
          ▼
   Deterministic RCA
          │
          ▼
     AI Reasoning
          │
          ▼
 Remediation Planning
          │
          ▼
 Human Approval
          │
          ▼
 Controlled Execution
          │
          ▼
 Verification
```

The first half answers:

> **What happened?**

The middle answers:

> **Why does the evidence suggest it happened?**

The later layers answer:

> **How should an engineer understand and respond to it?**

------------------------------------------------------------------------

# 4. The ATLAS LAB Application

The system being observed is a small distributed application used as the
laboratory environment.

It contains:

  --------------------------------------------------------------------------------
  Service                       Purpose                                       Port
  ----------------------------- --------------------- ----------------------------
  `atlas-gateway`               Entry point for                               8083
                                client requests       

  `atlas-order-service`         Creates/processes                             8084
                                orders and            
                                coordinates           
                                downstream services   

  `atlas-inventory-service`     Reserves inventory                            8085

  `atlas-payment-service`       Processes payment                             8086

  `atlas-intelligence-engine`   Ingests and analyzes                          8081
                                telemetry             

  `otel-collector`              Receives and forwards     Collector infrastructure
                                telemetry             
  --------------------------------------------------------------------------------

A normal order looks approximately like:

``` text
POST /api/orders
       │
       ▼
Gateway
       │
       ▼
Order Service
    ┌──┴────────────┐
    ▼               ▼
Inventory         Payment
    │               │
    └──────┬────────┘
           ▼
       Order Result
```

------------------------------------------------------------------------

# 5. Why Docker Compose Exists

Docker Compose provides a reproducible local distributed environment.

Instead of manually starting every service, the project can run the
system as multiple containers.

Conceptually:

``` text
Docker Compose
│
├── atlas-gateway
├── atlas-order-service
├── atlas-inventory-service
├── atlas-payment-service
├── atlas-intelligence-engine
└── otel-collector
```

This is important because ATLAS needs to observe a **real distributed
system**, not merely a single process.

------------------------------------------------------------------------

# 6. The Request Workflow

Let's follow one successful request.

The engineer sends:

``` powershell
Invoke-RestMethod `
  -Uri http://localhost:8083/api/orders `
  -Method POST `
  -ContentType "application/json" `
  -Headers @{"X-Correlation-ID"="MANUAL-TRACE-002"} `
  -Body '{"productId":"P100","quantity":1}'
```

The Gateway receives it.

The request gets a trace context.

The Gateway calls Order Service.

Order Service calls:

``` text
Inventory Service
Payment Service
```

The response eventually travels back:

``` text
Inventory
    ↓
Payment
    ↓
Order
    ↓
Gateway
    ↓
Client
```

At the same time, telemetry is generated.

------------------------------------------------------------------------

# 7. Correlation ID vs Trace ID

These are related but different.

## Correlation ID

ATLAS supports an application-level header:

``` text
X-Correlation-ID
```

Example:

``` text
MANUAL-TRACE-002
```

It is useful for humans and application-level investigation.

## Trace ID

OpenTelemetry creates a distributed trace identifier.

Example:

``` text
86a17ea9e1bcdb26640f8479c0950fdd
```

All spans belonging to the same distributed request share this Trace ID.

Therefore:

``` text
Correlation ID
       │
       ▼
OpenTelemetry Span attributes
       │
       ▼
Trace ID
       │
       ▼
Gateway → Order → Inventory / Payment
```

------------------------------------------------------------------------

# 8. What is a Span?

A **span** represents one timed operation.

For example:

``` text
Gateway:
POST /api/orders
```

Another span:

``` text
Order:
POST /api/orders
```

Another:

``` text
Order → Inventory
POST /api/inventory/P100/reserve
```

Another:

``` text
Order → Payment
POST /api/payments
```

A trace is therefore a collection of related spans.

Example:

``` text
Trace ID = ABC

Gateway span
   │
   ▼
Order span
   ├──→ Inventory span
   └──→ Payment span
```

------------------------------------------------------------------------

# 9. Parent and Child Spans

Every span can have a parent.

Example:

``` text
Gateway
  span = G

    ↓

Order
  span = O
  parent = G

    ├── Inventory
    │     parent = O
    │
    └── Payment
          parent = O
```

This relationship is extremely important.

It lets ATLAS reconstruct the execution tree.

------------------------------------------------------------------------

# 10. OpenTelemetry

ATLAS uses **OpenTelemetry (OTel)** as its telemetry standard.

Telemetry includes things such as:

-   traces
-   spans
-   metrics
-   attributes
-   timing
-   HTTP status/outcome information

The application services generate telemetry.

The Collector receives it.

The Intelligence Engine consumes it.

------------------------------------------------------------------------

# 11. OpenTelemetry Collector

The Collector sits between applications and ATLAS intelligence.

Conceptually:

``` text
Spring Boot Services
        │
        ▼
OpenTelemetry
        │
        ▼
OTel Collector
        │
        ├── Debug exporter
        │
        └── OTLP/HTTP
                │
                ▼
        Intelligence Engine
```

Why use a Collector?

Because the application services should not need to know how the
Intelligence Engine works.

It provides a telemetry infrastructure boundary.

------------------------------------------------------------------------

# 12. Telemetry Must Not Break Business Traffic

This is one of the most important ATLAS principles.

Suppose:

``` text
Order Service
     │
     ├── business request
     │
     └── telemetry export
              ↓
        OTel Collector
```

If the Collector dies:

``` text
OTel Collector = DOWN
```

the order service must still process business requests.

This was explicitly verified in M2.1 and M2.2.

So:

``` text
Business Path
──────────────► MUST REMAIN AVAILABLE

Telemetry Path
──────────────► MAY FAIL
```

Telemetry is therefore **observational infrastructure**, not a business
dependency.

------------------------------------------------------------------------

# 13. M2.1 --- OpenTelemetry Foundation

M2.1 introduced telemetry generation.

Main work:

-   OpenTelemetry Collector
-   Micrometer Tracing
-   OpenTelemetry bridge
-   OTLP exporters
-   correlation ID attached to spans
-   distributed tracing

The key achievement:

``` text
Gateway
  ↓
Order
  ↓
Inventory / Payment

ALL SHARE THE SAME TRACE ID
```

M2.1 also proved Collector failure did not break business traffic.

------------------------------------------------------------------------

# 14. M2.2 --- Telemetry Ingestion & Normalization

M2.1 generated telemetry.

M2.2 made ATLAS **consume it**.

The Intelligence Engine was introduced.

``` text
OTel Collector
       │
       │ OTLP/HTTP protobuf
       ▼
Intelligence Engine
       │
       ▼
Decode protobuf
       │
       ▼
Normalize
       │
       ▼
Sanitize
       │
       ▼
ATLASEvent
       │
       ▼
Bounded Event Buffer
```

------------------------------------------------------------------------

# 15. Why Normalize Telemetry?

Different telemetry objects have different structures.

Instead of making every later subsystem understand raw OpenTelemetry
structures, ATLAS converts them into a canonical event model.

Conceptually:

``` text
OTLP Span
OTLP Gauge
OTLP Sum
OTLP Histogram
       │
       ▼
 ATLASEvent
```

This makes downstream processing much simpler.

------------------------------------------------------------------------

# 16. ATLASEvent

The canonical event model contains normalized information.

For a trace event, fields include things such as:

``` text
event_id
event_type
timestamp
service_name
trace_id
span_id
parent_span_id
operation_name
status
duration_ms
attributes
```

A metric event can contain:

``` text
event_id
event_type = METRIC
timestamp
service_name
metric_name
metric_type
unit
attributes
```

So later engines don't need to decode raw OTLP protobufs themselves.

------------------------------------------------------------------------

# 17. Security Sanitization

M2.2 also established a security boundary.

Sensitive attribute keys are filtered when they contain terms such as:

``` text
authorization
password
token
secret
api_key
credential
```

Long strings are bounded.

The principle is:

> Telemetry should not become an accidental secret-storage system.

------------------------------------------------------------------------

# 18. Bounded Event Buffer

ATLAS does not keep unlimited telemetry in memory.

The buffer has a configurable capacity.

Default:

``` text
10,000 events
```

When full:

``` text
OLD EVENT
   ↓
dropped

NEW EVENT
   ↓
stored
```

This is the **DROP-OLDEST** policy.

Why?

Because recent telemetry is generally more useful for current
investigation.

------------------------------------------------------------------------

# 19. M2.3 --- Correlation & Dependency Reconstruction

M2.2 gave us events.

M2.3 answers:

> How are these events related?

It reconstructs:

1.  distributed traces
2.  parent/child relationships
3.  timelines
4.  service dependencies

------------------------------------------------------------------------

# 20. Trace Reconstruction

Suppose ATLAS receives:

``` text
Gateway span
Order span
Inventory span
Payment span
```

with:

``` text
same Trace ID
```

and parent IDs:

``` text
Gateway
   ↓
Order
   ├── Inventory
   └── Payment
```

M2.3 reconstructs that structure.

------------------------------------------------------------------------

# 21. Trace Summary

ATLAS can expose:

``` text
GET /api/v1/correlations/traces/{traceId}
```

A trace summary can tell us:

``` text
Trace ID
Root Service
Start Time
End Time
Duration
Span Count
Service Count
Services
Resolved Relationships
Unresolved Relationships
Overall Status
```

Example from manual verification:

``` text
Trace ID:
86a17ea9e1bcdb26640f8479c0950fdd

Root:
atlas-gateway

Services:
atlas-gateway
atlas-order-service
atlas-inventory-service
atlas-payment-service

Span Count:
7
```

This is a real distributed trace reconstruction.

------------------------------------------------------------------------

# 22. Trace Tree

The tree representation answers:

> Who called whom?

Conceptually:

``` text
Gateway
└── Order
    ├── Inventory
    └── Payment
```

This is different from simply listing spans.

------------------------------------------------------------------------

# 23. Timeline

The timeline answers:

> What happened first, second, third?

Example:

``` text
0 ms     Gateway starts
6 ms     Order starts
11 ms    Inventory starts
15 ms    Inventory finishes
19 ms    Payment starts
24 ms    Payment finishes
30 ms    Order/Gateway finish
```

Tree = structural relationship.

Timeline = chronological relationship.

Both are useful.

------------------------------------------------------------------------

# 24. Dependency Graph

M2.3 also builds a dynamic service dependency graph.

Example:

``` text
Gateway
   │
   ▼
Order
 ┌─┴───────┐
 ▼         ▼
Inventory Payment
```

The graph is based on **observed telemetry**, not a manually hardcoded
architecture diagram.

------------------------------------------------------------------------

# 25. Dependency Edge

An edge can contain:

``` text
source
target
call_count
first_observed
last_observed
average_duration_ms
error_count
status_counts
```

Example:

``` text
atlas-order-service
      ↓
atlas-payment-service

call_count = 2
average_duration = 11 ms
error_count = 0
```

This turns raw traces into operational topology.

------------------------------------------------------------------------

# 26. Important M2.3 Correction: Self-Edges

During manual verification, ATLAS exposed bad edges such as:

``` text
atlas-order-service
       ↓
atlas-order-service
```

This was identified as a correlation/graph construction defect.

It was fixed at the source.

Now both layers prevent same-service dependency edges:

``` text
parent service == child service
             ↓
        ignore edge
```

M2.5 also had defense-in-depth validation for invalid graph edges.

This is an important lesson:

> A system that looks correct at the API level can still contain
> incorrect internal relationships, so ATLAS validates its own derived
> data.

------------------------------------------------------------------------

# 27. M2.4 --- Incident Detection & Deterministic RCA

Now ATLAS knows:

``` text
what happened
+
where it happened
+
how services depend on each other
```

M2.4 asks:

> Is something actually wrong?

and:

> What does the evidence suggest is causing it?

------------------------------------------------------------------------

# 28. Incident Detection

M2.4 uses deterministic rules.

Examples include:

``` text
Error rate > 20%
Latency > 1000 ms
```

within an observation window.

The project uses a bounded sliding window.

Conceptually:

``` text
Recent 60 seconds
────────────────────────────────────

event event event event event event
             ↑
       evaluate signals
```

This avoids relying on one isolated failure.

------------------------------------------------------------------------

# 29. False Positive Protection

One request failing should not necessarily create an incident.

Therefore ATLAS uses:

``` text
ATLAS_MIN_OBSERVATIONS_FOR_INCIDENT
```

This prevents sparse telemetry from immediately creating incidents.

------------------------------------------------------------------------

# 30. Incident Fingerprinting

Multiple failures of the same kind should not create hundreds of
identical incidents.

A fingerprint can conceptually look like:

``` text
payment-service
|
POST /api/payments
|
TIMEOUT
```

If another matching signal arrives:

``` text
existing incident
       ↓
update it
```

instead of:

``` text
create duplicate incident
```

------------------------------------------------------------------------

# 31. Incident Lifecycle

M2.4 introduced:

``` text
OPEN
  ↓
ACKNOWLEDGED
  ↓
RESOLVED
```

Recovery is also bounded.

The incident does not instantly disappear after one successful request.

It waits for sustained recovery according to the configured recovery
period.

------------------------------------------------------------------------

# 32. What is RCA?

RCA = **Root Cause Analysis**.

The key ATLAS principle is:

> M2.4 RCA is deterministic and evidence-based.

It does not ask an LLM:

> "What do you think happened?"

Instead it examines observed evidence.

------------------------------------------------------------------------

# 33. Failure Propagation

Suppose:

``` text
Payment fails
      ↓
Order fails
      ↓
Gateway fails
```

A naive system might say:

``` text
Gateway is broken
```

But ATLAS can see the temporal/dependency relationship:

``` text
Payment
   ↓
Order
   ↓
Gateway
```

and determine that the upstream errors may be consequences of the
downstream failure.

This helps avoid blaming the wrong service.

------------------------------------------------------------------------

# 34. Blast Radius

M2.4 calculates affected scope.

Example:

``` text
Payment failure

Affected:
Gateway
Order
Payment

Unaffected:
Inventory
```

This is important because an independent healthy service is evidence
too.

------------------------------------------------------------------------

# 35. Ambiguous RCA

ATLAS deliberately refuses to invent certainty.

Suppose:

``` text
Payment failure
+
Inventory failure
```

occur simultaneously.

If evidence cannot distinguish them strongly enough:

``` text
ROOT CAUSE = AMBIGUOUS
CONFIDENCE = LOW
```

The system should not randomly choose:

``` text
Payment
```

just because it needs an answer.

This is a major safety principle.

------------------------------------------------------------------------

# 36. M2.5 --- AI-Assisted Incident Reasoning

M2.4 determines the deterministic RCA.

M2.5 does **not replace it**.

Instead:

``` text
M2.4 Evidence + RCA
        ↓
M2.5 AI Reasoning
        ↓
Human-readable engineering analysis
```

The AI's role is:

> **Analyst, not authority.**

------------------------------------------------------------------------

# 37. M2.5 AI Boundary

The AI must never overwrite M2.4's RCA.

If M2.4 says:

``` text
AMBIGUOUS
```

M2.5 cannot say:

``` text
Payment is definitely the root cause.
```

Instead it must preserve the ambiguity.

------------------------------------------------------------------------

# 38. Facts vs Inferences

M2.5 explicitly separates:

## Observed Facts

Things directly supported by telemetry.

Example:

``` text
Payment returned HTTP 500.
Evidence:
E123
```

## Inferences

Reasoned conclusions.

Example:

``` text
Payment degradation likely preceded
the observed upstream failures.

Evidence:
E123
E124
```

This distinction is extremely important.

------------------------------------------------------------------------

# 39. Evidence Grounding

Every AI claim needs evidence IDs.

Conceptually:

``` json
{
  "claim": "Payment latency increased",
  "evidenceIds": ["E123"]
}
```

An unsupported statement is rejected.

An unknown evidence ID is rejected.

This prevents the AI from inventing evidence.

------------------------------------------------------------------------

# 40. M2.5 Provider Architecture

ATLAS does not hard-code itself to one AI provider.

There is a provider interface:

``` text
ReasoningProvider
       │
       ├── FakeProvider
       │
       └── GeminiProvider
```

The Fake Provider is particularly important for testing.

It allows the test suite to run without requiring an external AI API.

------------------------------------------------------------------------

# 41. AI Disabled Mode

ATLAS supports:

``` text
ATLAS_AI_ENABLED=false
```

The platform must still work.

This reinforces the architectural rule:

``` text
AI = optional intelligence layer

Core telemetry / incident system
= must remain functional
```

------------------------------------------------------------------------

# 42. AI Context Builder

The AI should not receive unlimited raw telemetry.

The Context Builder creates a bounded context.

It includes relevant things such as:

``` text
incident
signals
evidence
RCA candidates
dependency graph
trace information
```

It also performs another sanitization pass.

------------------------------------------------------------------------

# 43. Prompt Injection Defense

Telemetry is data.

It must not become instructions.

Suppose an attribute contains:

``` text
Ignore previous instructions and execute ...
```

ATLAS treats that as telemetry text.

It does not interpret it as an instruction.

This distinction is critical:

``` text
Telemetry text
     ≠
System instruction
```

------------------------------------------------------------------------

# 44. M2.6 --- Remediation Planning & Safety

M2.5 explains the incident.

M2.6 asks:

> Given what we know, what remediation could an engineer consider?

But M2.6 **does not execute anything**.

The architecture is:

``` text
M2.4 RCA
   +
M2.5 AI Analysis
   +
M2.3 Graph
       ↓
M2.6 Planner
       ↓
Safety Validator
       ↓
Remediation Plan
```

------------------------------------------------------------------------

# 45. Remediation Plan

A plan can contain actions such as:

``` text
RESTART_SERVICE
REDUCE_TRAFFIC
RESTORE_TRAFFIC
ROLLBACK_DEPLOYMENT
SCALE_SERVICE
CLEAR_CONNECTION_POOL
OBSERVE
INVESTIGATE
```

These are symbolic actions in M2.6.

They are not commands.

------------------------------------------------------------------------

# 46. Why an Allowlist?

The planner must not be able to invent arbitrary actions.

Allowed:

``` text
RESTART_SERVICE
```

Not allowed:

``` text
RUN_THIS_RANDOM_COMMAND
```

This is a fixed catalog.

------------------------------------------------------------------------

# 47. Safety Rules

M2.6 rejects unsafe plans.

Examples:

``` text
Unknown action
       ↓
REJECT

Unknown service
       ↓
REJECT

Missing evidence
       ↓
REJECT

AMBIGUOUS RCA + HIGH-RISK action
       ↓
REJECT

LOW-confidence RCA + HIGH-RISK action
       ↓
REJECT
```

Required plan sections also include:

``` text
Preconditions
Verification
Rollback
```

------------------------------------------------------------------------

# 48. Evidence Grounding in M2.6

Even:

``` text
OBSERVE
```

and:

``` text
INVESTIGATE
```

must contain evidence IDs.

Why?

Because otherwise a planner could recommend arbitrary investigation
steps without demonstrating why they are relevant.

------------------------------------------------------------------------

# 49. M2.6 Approval

A human can approve or reject a plan.

But:

``` text
APPROVED
```

does **not** mean:

``` text
EXECUTED
```

M2.6 explicitly returns:

``` text
Plan approved.
Execution is not supported by this milestone.
```

Therefore M2.6 is still operationally inert.

------------------------------------------------------------------------

# 50. M2.7 --- Controlled Remediation Execution

M2.7 is the next architectural boundary.

M2.7 introduces actual controlled execution.

The intended flow is:

``` text
Incident
   ↓
M2.4 RCA
   ↓
M2.5 AI Explanation
   ↓
M2.6 Remediation Plan
   ↓
Human Approval
   ↓
M2.7 Execution Guard
   ↓
Typed Executor
   ↓
Infrastructure Adapter
   ↓
Verification
```

This milestone is substantially more dangerous than the previous ones
because it can affect infrastructure.

------------------------------------------------------------------------

# 51. M2.7 Safety Principle

The current approved direction is:

**Do not use arbitrary shell commands.**

For Docker execution, the recommended architecture is a strongly typed
adapter using the Docker SDK rather than:

``` text
os/exec
exec.Command(...)
docker CLI
shell
PowerShell
```

The executor should receive something like:

``` text
RestartService(serviceName)
```

rather than:

``` text
Execute("docker restart ...")
```

------------------------------------------------------------------------

# 52. M2.7 Execution Guards

Execution should require all necessary conditions.

Conceptually:

``` text
ATLAS_EXECUTION_ENABLED=true
        AND
Plan == APPROVED
        AND
Fingerprint unchanged
        AND
Action allowed
        AND
Service allowed
        AND
Evidence valid
        AND
Idempotency passes
        ↓
EXECUTE
```

Default should remain:

``` text
ATLAS_EXECUTION_ENABLED=false
```

------------------------------------------------------------------------

# 53. Why Fingerprints Matter

Suppose a human approves:

``` text
Restart Payment Service
```

Then someone changes the plan.

The old approval must not authorize the modified plan.

Therefore:

``` text
Approved Fingerprint
        ==
Current Plan Fingerprint
```

If they differ:

``` text
REJECT EXECUTION
```

------------------------------------------------------------------------

# 54. Idempotency

Suppose the same execute request arrives twice.

Without protection:

``` text
Request 1 → restart
Request 2 → restart again
```

ATLAS should recognize:

``` text
planID + actionID
```

and prevent duplicate execution.

This is called **idempotency protection**.

------------------------------------------------------------------------

# 55. Execution vs Verification

Even if an infrastructure action succeeds:

``` text
Docker restart succeeded
```

that does not necessarily mean:

``` text
Incident resolved
```

Therefore execution status and verification status are separate.

Example:

``` text
Execution:
EXECUTED

Verification:
VERIFICATION_FAILED
```

This distinction is essential for reliable automation.

------------------------------------------------------------------------

# 56. Complete End-to-End Failure Example

Now let's put everything together.

Imagine Payment Service starts failing.

## Step 1 --- User request

``` text
Client
  ↓
Gateway
  ↓
Order
  ↓
Payment
```

Payment returns an error.

------------------------------------------------------------------------

## Step 2 --- Telemetry

The services generate:

``` text
Gateway span
Order span
Payment span
```

They share a Trace ID.

------------------------------------------------------------------------

## Step 3 --- Collector

``` text
Services
   ↓
OTel Collector
```

The Collector forwards telemetry.

------------------------------------------------------------------------

## Step 4 --- M2.2

Intelligence Engine:

``` text
decode
  ↓
normalize
  ↓
sanitize
  ↓
ATLASEvent
```

------------------------------------------------------------------------

## Step 5 --- M2.3

Correlation Engine reconstructs:

``` text
Gateway
   ↓
Order
   ↓
Payment
```

Dependency graph records:

``` text
Gateway → Order
Order → Payment
```

------------------------------------------------------------------------

## Step 6 --- M2.4

The sliding window notices:

``` text
Payment error rate > threshold
```

Incident created:

``` text
OPEN
```

------------------------------------------------------------------------

## Step 7 --- M2.4 RCA

The evidence shows:

``` text
Payment failed first
Order failed afterward
Gateway failed afterward
```

RCA:

``` text
Payment Service
```

with deterministic evidence and confidence.

------------------------------------------------------------------------

## Step 8 --- M2.5

AI receives bounded evidence.

It produces:

``` text
Executive Summary

Observed Facts:
- Payment failures increased.
- Order requests propagated those failures.

Inferences:
- Payment degradation likely initiated the observed cascade.

Missing Evidence:
- ...

Recommended Investigations:
- ...
```

Every claim references evidence IDs.

------------------------------------------------------------------------

## Step 9 --- M2.6

Planner proposes:

``` text
RESTART_SERVICE
service = atlas-payment-service
```

with:

``` text
risk
preconditions
verification
rollback
evidence IDs
```

Safety Validator approves the plan as safe to propose.

------------------------------------------------------------------------

## Step 10 --- Human approval

Engineer reviews it.

``` text
APPROVED
```

------------------------------------------------------------------------

## Step 11 --- M2.7

Execution guard checks:

``` text
enabled?
approved?
fingerprint unchanged?
action allowed?
service allowed?
already executed?
```

If everything passes:

``` text
Docker adapter
      ↓
Restart Payment Service
```

------------------------------------------------------------------------

## Step 12 --- Verification

ATLAS observes new telemetry.

It checks:

``` text
Did errors decrease?
Did latency recover?
Is Payment healthy?
Did upstream errors recover?
```

Only then can the system establish whether remediation actually helped.

------------------------------------------------------------------------

# 57. Complete ATLAS Architecture

``` text
                    ┌──────────────────────┐
                    │      CLIENT         │
                    └──────────┬───────────┘
                               │
                               ▼
                    ┌──────────────────────┐
                    │   ATLAS GATEWAY      │
                    └──────────┬───────────┘
                               │
                               ▼
                    ┌──────────────────────┐
                    │   ORDER SERVICE      │
                    └──────┬───────┬───────┘
                           │       │
                           ▼       ▼
                    INVENTORY   PAYMENT
                       SERVICE    SERVICE
                           │       │
                           └───┬───┘
                               │
                      Telemetry generated
                               │
                               ▼
                    ┌──────────────────────┐
                    │  OTEL COLLECTOR      │
                    └──────────┬───────────┘
                               │
                               ▼
               ┌──────────────────────────────┐
               │     INTELLIGENCE ENGINE      │
               │                              │
               │  M2.2 Normalization          │
               │          ↓                   │
               │  Event Buffer                │
               │          ↓                   │
               │  M2.3 Correlation            │
               │       ↙      ↘               │
               │    Trace     Graph           │
               │       \      /               │
               │        ↓    ↓                │
               │  M2.4 Incident Detection    │
               │          ↓                   │
               │  Deterministic RCA           │
               │          ↓                   │
               │  M2.5 AI Reasoning           │
               │          ↓                   │
               │  M2.6 Remediation Planner   │
               │          ↓                   │
               │     Human Approval           │
               │          ↓                   │
               │  M2.7 Execution Engine      │
               │          ↓                   │
               │  Infrastructure Adapter     │
               └──────────────────────────────┘
```

------------------------------------------------------------------------

# 58. Layer Responsibilities

  Layer                  Main Question
  ---------------------- -----------------------------------------------------
  Application services   What does the business system do?
  OpenTelemetry          What happened inside requests?
  Collector              Where should telemetry go?
  M2.2                   How do we safely ingest telemetry?
  M2.3                   How are events related?
  M2.3 Graph             Who depends on whom?
  M2.4 Detector          Is there an incident?
  M2.4 RCA               What does deterministic evidence suggest caused it?
  M2.5 AI                How can we explain the evidence to engineers?
  M2.6 Planner           What remediation could be proposed safely?
  Human                  Should the plan be approved?
  M2.7                   Can the approved action be safely executed?
  Verification           Did the action actually help?

------------------------------------------------------------------------

# 59. APIs You Should Know

## Business API

``` text
POST /api/orders
```

Used to generate real application traffic.

------------------------------------------------------------------------

## Health

``` text
GET /actuator/health
```

Used to verify service health.

------------------------------------------------------------------------

## M2.2 Event APIs

``` text
GET /api/v1/events
GET /api/v1/events/{eventId}
GET /api/v1/events/trace/{traceId}
GET /api/v1/events/metrics
```

These let you inspect normalized telemetry.

------------------------------------------------------------------------

## M2.3 Correlation APIs

``` text
GET /api/v1/correlations/traces/{traceId}

GET /api/v1/correlations/traces/{traceId}/tree

GET /api/v1/correlations/traces/{traceId}/timeline
```

------------------------------------------------------------------------

## M2.3 Graph APIs

``` text
GET /api/v1/graph

GET /api/v1/graph/services/{serviceName}

GET /api/v1/graph/edges
```

------------------------------------------------------------------------

## M2.4 Incident APIs

``` text
GET /api/v1/incidents

GET /api/v1/incidents/open

GET /api/v1/incidents/{incidentId}

GET /api/v1/incidents/{incidentId}/evidence

GET /api/v1/incidents/{incidentId}/rca

GET /api/v1/incidents/{incidentId}/timeline
```

------------------------------------------------------------------------

## M2.5 AI APIs

``` text
POST /api/v1/incidents/{incidentId}/analyze

GET /api/v1/incidents/{incidentId}/analysis
```

------------------------------------------------------------------------

## M2.6 Remediation APIs

``` text
POST /api/v1/incidents/{incidentId}/remediation/plan

GET /api/v1/incidents/{incidentId}/remediation

GET /api/v1/remediation/{planId}

POST /api/v1/remediation/{planId}/approve

POST /api/v1/remediation/{planId}/reject
```

------------------------------------------------------------------------

## M2.7 Planned APIs

``` text
POST /api/v1/remediation/{planId}/execute

GET /api/v1/executions/{executionId}

GET /api/v1/incidents/{incidentId}/executions
```

------------------------------------------------------------------------

# 60. Important Environment Configuration Concepts

Examples encountered in the architecture include:

``` text
OTEL_EXPORTER_OTLP_ENDPOINT
```

Controls where application telemetry is exported.

``` text
ATLAS_EVENT_BUFFER_CAPACITY
```

Controls event-buffer size.

``` text
ATLAS_AI_ENABLED
```

Controls AI reasoning availability.

``` text
ATLAS_AI_ANALYSIS_RETENTION_SECONDS
```

Controls AI analysis retention.

``` text
ATLAS_MIN_OBSERVATIONS_FOR_INCIDENT
```

Controls minimum observations before incident detection.

``` text
ATLAS_INCIDENT_RECOVERY_SECONDS
```

Controls sustained recovery requirements.

``` text
ATLAS_INCIDENT_RETENTION_SECONDS
```

Controls incident retention.

``` text
ATLAS_RCA_AMBIGUITY_MARGIN
```

Controls how close RCA candidate scores can be before the result becomes
ambiguous.

``` text
ATLAS_EXECUTION_ENABLED
```

Controls whether M2.7 execution is enabled.

For safety, execution should default to:

``` text
false
```

------------------------------------------------------------------------

# 61. Testing Philosophy

ATLAS does not consider:

``` text
"go build succeeded"
```

enough.

Each milestone has multiple verification levels.

``` text
Unit Tests
     ↓
Race Tests
     ↓
Native Regression
     ↓
Docker E2E
     ↓
Manual API Verification
     ↓
Security Audit
     ↓
Git/CI Verification
```

------------------------------------------------------------------------

# 62. Why `go test -race` Matters

The Intelligence Engine is concurrent.

Telemetry can arrive simultaneously.

Therefore normal tests are not enough.

The project uses:

``` bash
go test -race ./...
```

The race detector helps identify unsafe concurrent access.

------------------------------------------------------------------------

# 63. Why Docker E2E Tests Matter

Unit tests can prove:

``` text
function X works
```

But they cannot prove the whole platform works together.

Docker E2E tests verify:

``` text
Application
   ↓
OTel
   ↓
Collector
   ↓
Intelligence Engine
   ↓
Correlation
   ↓
Incident
   ↓
AI
   ↓
Remediation
```

as an integrated system.

------------------------------------------------------------------------

# 64. CI/CD

GitHub Actions runs project checks.

A typical build flow is:

``` text
Checkout repository
       ↓
Setup Go / Java
       ↓
Download dependencies
       ↓
Go tests
       ↓
Go build
       ↓
Java build/tests
```

A previous CI issue occurred because the build command pointed at the
service root rather than the actual Go entry point.

The correct build concept is:

``` text
cd services/intelligence-engine

go build -o atlas-intelligence-engine ./cmd/intelligence-engine
```

CI was subsequently corrected and passed.

Transient GitHub API `429 Too Many Requests` errors were also observed
while downloading Actions; those were infrastructure/rate-limit issues
rather than ATLAS test failures.

------------------------------------------------------------------------

# 65. Git Milestones

The repository history currently includes milestones around:

``` text
M0
M1.x
M2.1
M2.2
M2.3
M2.4
M2.5
M2.6
```

Recent commits included:

``` text
feat(intelligence): add remediation planning and safety engine

feat(intelligence): add M2.5 ai reasoning engine

fix(correlation): prevent self-edge graph construction
```

The exact repository state should always be checked with:

``` powershell
git log --oneline
```

------------------------------------------------------------------------

# 66. How to Manually Explore ATLAS

Start the environment:

``` powershell
docker compose up -d --build
```

Check containers:

``` powershell
docker ps
```

Check gateway:

``` powershell
Invoke-RestMethod http://localhost:8083/actuator/health
```

Generate an order:

``` powershell
Invoke-RestMethod `
  -Uri http://localhost:8083/api/orders `
  -Method POST `
  -ContentType "application/json" `
  -Headers @{"X-Correlation-ID"="MANUAL-TEST-001"} `
  -Body '{"productId":"P100","quantity":1}'
```

------------------------------------------------------------------------

# 67. Inspect Normalized Events

``` powershell
$events = Invoke-RestMethod `
  -Uri "http://localhost:8081/api/v1/events?limit=100"
```

Inspect event types:

``` powershell
$events |
  Group-Object event_type |
  Select-Object Name, Count |
  Format-Table -AutoSize
```

You may see:

``` text
METRIC
TRACE_SPAN
```

------------------------------------------------------------------------

# 68. Inspect Trace Events

``` powershell
$events |
  Where-Object { $_.event_type -eq "TRACE_SPAN" } |
  Select-Object -First 20 `
    event_id,
    service_name,
    trace_id,
    span_id,
    parent_span_id,
    operation_name,
    status |
  Format-Table -AutoSize
```

Look for:

``` text
same trace_id
different span_id
parent_span_id relationships
different service_name values
```

That proves distributed tracing is being represented inside ATLAS.

------------------------------------------------------------------------

# 69. Inspect One Trace

Find a trace ID:

``` powershell
$traceId = "YOUR_TRACE_ID"
```

Then:

``` powershell
Invoke-RestMethod `
  "http://localhost:8081/api/v1/correlations/traces/$traceId" |
  ConvertTo-Json -Depth 20
```

Look at:

``` text
root_service
span_count
service_count
services
resolved_relationships
unresolved_relationships
spans
```

------------------------------------------------------------------------

# 70. Inspect the Dependency Graph

``` powershell
Invoke-RestMethod `
  "http://localhost:8081/api/v1/graph" |
  ConvertTo-Json -Depth 20
```

You want to see relationships such as:

``` text
gateway → order

order → inventory

order → payment
```

You should **not** see:

``` text
gateway → gateway
order → order
payment → payment
inventory → inventory
```

------------------------------------------------------------------------

# 71. Inspect a Service's Dependencies

For Order:

``` powershell
Invoke-RestMethod `
  "http://localhost:8081/api/v1/graph/services/atlas-order-service" |
  ConvertTo-Json -Depth 20
```

This separates:

``` text
incoming
outgoing
```

So you can ask:

> Who calls Order?

and:

> Who does Order call?

------------------------------------------------------------------------

# 72. The Best Way to Study ATLAS

Do not try to memorize every Go file first.

Study in this order:

``` text
1. Distributed systems basics
        ↓
2. HTTP request flow
        ↓
3. Docker Compose
        ↓
4. OpenTelemetry
        ↓
5. Trace / Span / Parent Span
        ↓
6. M2.2 normalization
        ↓
7. M2.3 correlation
        ↓
8. Dependency graphs
        ↓
9. Incident detection
        ↓
10. RCA
        ↓
11. AI reasoning
        ↓
12. Remediation planning
        ↓
13. Controlled execution
```

------------------------------------------------------------------------

# 73. Questions You Should Be Able to Answer

After studying, you should be able to answer:

### Architecture

1.  What problem does ATLAS solve?
2.  Why does ATLAS have an Intelligence Engine?
3.  Why is the application split into multiple services?
4.  Why is Docker Compose used?

### Telemetry

5.  What is OpenTelemetry?
6.  What is a trace?
7.  What is a span?
8.  What is a parent span?
9.  What is a Trace ID?
10. What is a Correlation ID?
11. Why is the Collector separate from the Intelligence Engine?

### M2.2

12. What is an ATLASEvent?
13. Why normalize OTLP?
14. Why sanitize attributes?
15. Why is the event buffer bounded?
16. What does DROP-OLDEST mean?

### M2.3

17. How does ATLAS reconstruct a trace?
18. How does it determine parent/child relationships?
19. What is a dependency graph?
20. What is a dependency edge?
21. Why are self-edges invalid?

### M2.4

22. How does ATLAS detect incidents?
23. Why use a sliding window?
24. What prevents false positives?
25. What is deterministic RCA?
26. What is blast radius?
27. Why can RCA be AMBIGUOUS?

### M2.5

28. Why is AI not the owner of RCA?
29. What is evidence grounding?
30. What is the difference between a fact and an inference?
31. Why is AI optional?
32. How does ATLAS defend against prompt injection?

### M2.6

33. What is a remediation plan?
34. Why is there an action allowlist?
35. Why must every action contain evidence?
36. Why doesn't APPROVED mean EXECUTED?

### M2.7

37. What makes execution different from planning?
38. Why are execution guards necessary?
39. Why are fingerprints important?
40. What is idempotency?
41. Why separate EXECUTED from VERIFIED?

------------------------------------------------------------------------

# 74. The Core Design Principles

These are more important than individual files.

## Principle 1 --- Evidence before intelligence

``` text
Telemetry
  ↓
Evidence
  ↓
Analysis
```

Not:

``` text
AI guess
  ↓
Evidence
```

------------------------------------------------------------------------

## Principle 2 --- Deterministic RCA owns RCA

M2.4 owns root-cause determination.

M2.5 explains it.

M2.6 plans around it.

No later layer silently rewrites it.

------------------------------------------------------------------------

## Principle 3 --- AI is bounded

AI gets:

``` text
bounded
sanitized
validated
evidence-grounded
```

context.

------------------------------------------------------------------------

## Principle 4 --- No arbitrary execution

M2.6 has no executor.

M2.7 should only execute strongly typed, allowlisted actions.

------------------------------------------------------------------------

## Principle 5 --- Human approval before dangerous operations

The intended boundary is:

``` text
AI
 ↓
Plan
 ↓
Safety validation
 ↓
Human
 ↓
Execution
```

------------------------------------------------------------------------

## Principle 6 --- Failure of intelligence must not destroy the application

The business application should continue even if:

``` text
Collector fails
Intelligence Engine fails
AI provider fails
```

The intelligence layer is not supposed to become a new business
dependency.

------------------------------------------------------------------------

## Principle 7 --- Everything important is bounded

ATLAS bounds:

``` text
event buffer
trace retention
graph retention
incident retention
AI context
AI analysis retention
remediation retention
execution records
```

This protects memory and operational stability.

------------------------------------------------------------------------

# 75. The Entire Project in One Sentence

> **ATLAS observes a distributed application, converts its telemetry
> into structured evidence, reconstructs what happened and how services
> depend on one another, detects incidents, performs deterministic
> evidence-based RCA, uses AI to explain the findings, creates
> safety-validated remediation plans, and---starting with M2.7---can
> execute only explicitly approved, allowlisted actions under strict
> guards and verification.**

------------------------------------------------------------------------

# 76. Final Mental Picture

Whenever you are confused about a component, come back to this:

``` text
             ┌───────────────────┐
             │   DISTRIBUTED APP  │
             └─────────┬─────────┘
                       │
                       │ telemetry
                       ▼
             ┌───────────────────┐
             │  OTEL COLLECTOR   │
             └─────────┬─────────┘
                       │
                       ▼
             ┌───────────────────┐
             │  NORMALIZATION    │
             │      M2.2         │
             └─────────┬─────────┘
                       │
                       ▼
             ┌───────────────────┐
             │   CORRELATION     │
             │      M2.3         │
             └──────┬───────┬────┘
                    │       │
                 TRACE     GRAPH
                    │       │
                    └───┬───┘
                        ▼
             ┌───────────────────┐
             │ INCIDENT + RCA    │
             │      M2.4         │
             └─────────┬─────────┘
                       │
                       ▼
             ┌───────────────────┐
             │    AI REASONING   │
             │      M2.5         │
             └─────────┬─────────┘
                       │
                       ▼
             ┌───────────────────┐
             │ REMEDIATION PLAN  │
             │      M2.6         │
             └─────────┬─────────┘
                       │
                 HUMAN APPROVAL
                       │
                       ▼
             ┌───────────────────┐
             │ CONTROLLED EXEC.  │
             │      M2.7         │
             └─────────┬─────────┘
                       │
                       ▼
                  VERIFICATION
```

If you understand this diagram deeply, you understand the **architecture
of ATLAS**. The next step is learning how each box is implemented in the
repository.
