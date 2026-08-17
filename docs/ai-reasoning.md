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
Forces a re-evaluation with `?force=true`.

**Retrieve Analysis**
```http
GET /api/v1/incidents/{incidentId}/analysis
```

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
