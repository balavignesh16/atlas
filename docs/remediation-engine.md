# Remediation & Safety Engine (M2.6)

The M2.6 milestone introduces a completely sandboxed Remediation Planning engine.

> [!WARNING]
> M2.6 explicitly refuses to implement an Executor. No actions are executed by ATLAS. All planned remediation actions are entirely symbolic.

## Architecture

1. **Planners**: Uses `RemediationPlannerProvider` interface with `AI`, `Fake`, and `Fallback` implementations.
2. **Validator**: Applies heavy scrutiny on generated plans to protect against LLM hallucinations, ensuring all references match deterministic context data.
3. **Risk & Policy Engine**: Statically scores actions against `LOW`, `MEDIUM`, `HIGH`, or `CRITICAL` risk and rejects combinations like "AMBIGUOUS RCA + HIGH RISK ACTION".

## Evidence Grounding

To prevent hallucinated explanations, EVERY single remediation action (even purely diagnostic `OBSERVE` actions) MUST attach valid Evidence IDs from the incident context. Any generated action carrying an invalid or missing evidence ID is instantly rejected (`ErrMissingEvidence`).

## API Endpoints

- `POST /api/v1/incidents/{incidentId}/remediation/plan`: Generates a remediation plan.
- `GET /api/v1/incidents/{incidentId}/remediation`: Retrieves the dry-run plan (`executionSupported: false`).
- `POST /api/v1/remediation/{planId}/approve`: Safely approves the plan without executing any operations.
- `POST /api/v1/remediation/{planId}/reject`: Rejects the proposed plan.
