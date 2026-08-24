# ATLAS M2.7 Execution Engine

The ATLAS Execution Engine is responsible for the controlled execution of human-approved remediation plans. 

## Architectural Principles

1. **Isolation of Decision and Execution**: The Intelligence Engine calculates deterministic RCAs (M2.4) and explains them with AI (M2.5), generating safe plans (M2.6). The Execution Engine (M2.7) executes *only* plans that have been explicitly approved by a human.
2. **Strict Adapters**: Executions are mediated by strongly-typed infrastructure adapters. The Docker SDK adapter is hard-coded to a strict set of allowable `TargetService` values mapped to actual Docker container names. No arbitrary strings or `os/exec` commands are used.
3. **Immutability of Approvals**: Any modification to a plan invalidates its approval fingerprint, preventing execution.
4. **Idempotency**: Execution is idempotent by `planId` and `actionId`. Duplicate execution requests return the existing execution record.
5. **Post-Execution Verification**: After execution, the engine verifies success against the M2.4 Incident Manager to ensure the service has actually recovered.

## Execution Guard Safety Boundaries

The `execution.Guard` enforces:
1. `ATLAS_EXECUTION_ENABLED=true`
2. Plan `Status == APPROVED`
3. Approval Fingerprint exactly matches current Plan Fingerprint.
4. Target action type is in the execution allowlist (`RESTART_SERVICE`, `OBSERVE`, `INVESTIGATE`).
5. Target service is strictly allowlisted.
6. Execution contains valid evidence IDs justifying the action.

## Testing Strategy

- `make test` runs the unit suite against a deterministic `FakeExecutor`, ensuring 100% logic coverage without requiring real infrastructure.
- `test-m27-docker.ps1` runs an End-to-End simulation that triggers an incident, generates an AI plan, approves it, and physically restarts the target container using the Docker SDK adapter.
