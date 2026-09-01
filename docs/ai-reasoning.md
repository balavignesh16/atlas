# AI Reasoning Engine

The ATLAS AI Reasoning Engine (M2.5) provides human-readable explanations of deterministic incidents detected by the platform.

## Architecture

1. **Context Builder**: Pulls bounded subsets of Signals, Evidences, RCA Candidates, and Dependency Edges.
2. **Double Sanitization Pass**: Strips fields containing keywords like `password`, `token`, `secret`, `api_key` and truncates extremely long strings.
3. **Graph Validation**: Explicitly rejects invalid edges (e.g. `source == target`) before sending the context to the AI.
4. **Provider Interface**: Pluggable providers (`FakeProvider` for tests, `GeminiProvider` for external APIs).
5. **Validator**: Enforces structural and logical constraints (e.g. unknown evidence rejection, ambiguity preservation).
6. **Retention Engine**: Hashes incident state to prevent duplicate analyses and retains results in memory for a bounded window.

## Using the API

**Trigger Analysis**
```http
POST /api/v1/incidents/{incidentId}/analyze
```
Forces a re-evaluation with `?force=true`. Requires an authenticated
`X-Atlas-Api-Key` with `VIEW` permission when `ATLAS_SECURITY_ENABLED=true`
-- the same requirement as every other read/insight endpoint (including its
own sibling below). (Module 4: this endpoint was unreachable at all through
`main.go`'s HTTP routing prior to Module 4 -- see `docs/roadmap-checklist.md`
and the Module 4 release review for the routing defect and its fix.)

**Retrieve Analysis**
```http
GET /api/v1/incidents/{incidentId}/analysis
```
Also requires `VIEW` permission when security is enabled.

## AI/Remediation Safety Boundary (Module 4)

`remediation.RemediationContext` (in the FROZEN `internal/remediation`
package) carries an `Analysis *aireasoning.AnalysisResult` field, populated
by `HandlePostPlan` from `aiEngine.GetAnalysis(incidentId)` whenever a prior
`/analyze` call produced one. **This is documentation of an existing,
verified invariant, not a new restriction**: as of this writing, none of
`internal/remediation`'s planner implementations (`FakePlanner`,
`FallbackPlanner`, or the unimplemented `AIPlanner` placeholder) read this
field at all, and `internal/remediation/policy.go`'s `EvaluatePolicy` --
the actual HIGH/CRITICAL-risk safety gate -- derives its RCA
service/confidence exclusively from `Incident.RCA`, never from
`AnalysisResult`. Any future `RemediationPlannerProvider` implementation
**must preserve this**: `RemediationContext.Analysis` may inform a plan's
*rationale text*, but must never determine `RiskLevel`, `ActionType`,
target-service selection, or bypass `EvaluatePolicy`'s RCA-keyed checks --
AI output is advisory context only, never a safety authority. Because
`internal/remediation` is frozen, this invariant is recorded here rather
than as a code comment inside that package.

## Schema

Outputs distinguish explicitly between **Facts** and **Inferences**. All statements must be grounded by `Evidence IDs` provided by M2.4.

```json
{
  "executiveSummary": "...",
  "observedFacts": [
    { "claim": "Latency spiked", "evidenceIds": ["E123"] }
  ],
  "inferences": [
    { "claim": "Payment service degraded first", "evidenceIds": ["E123", "E124"] }
  ]
}
```
