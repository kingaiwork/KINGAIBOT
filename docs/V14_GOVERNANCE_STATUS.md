# KINGAIBOT v1.4 Governance and Replay-Safety Status

Status: Draft development baseline
Owner: USDX TECH LLC / KING AI
Branch: `agent/v1.4-governance`

## Purpose

KINGAIBOT v1.4 turns governance declarations into runtime-enforced behavior. The model remains replaceable and untrusted for authority decisions. Identity, authority, budgets, durable work state, reconciliation and evidence belong to KINGAIBOT.

Core invariant:

```text
intent
  -> trusted identity
  -> capability envelope
  -> hierarchical budget
  -> approval / policy
  -> audit
  -> execution
  -> evidence
  -> completion or reconciliation
```

## Runtime task lifecycle

The v1.4 Runtime task lifecycle is now directional and replay-aware:

```text
pending_audit
  -> queued
  -> running
  -> completing
  -> completed
```

Ambiguous execution never silently returns to `queued`:

```text
running + process restart      -> reconciliation
completing + process restart   -> reconciliation
completion audit failure       -> reconciliation
```

Only tasks that were still `queued` are automatically re-enqueued after restart.

### Audit-before-queue

New tasks persist as `pending_audit`. The `task.created` event must be durably appended before the task can become `queued` and executable. A crash before that audit leaves an inert task that recovery ignores.

### Idempotent task creation

Runtime supports stable idempotent task creation for durable orchestration. A caller-owned key is SHA-256 hashed; the raw key is never persisted. The digest determines a deterministic task identity and is paired with an input digest.

Rules:

- same key + same input -> same Task
- same key + different input -> conflict
- repeated creation never enqueues duplicate work
- `pending_audit` ambiguity is not auto-promoted

### Two-phase completion

A successful agent result first becomes `completing`. Output and provider are durable, but the task is not yet trusted as completed.

The Runtime then appends `task.completed` with an output SHA-256 digest. Only after that audit succeeds may the state become `completed`.

If completion audit fails, the output remains available for inspection but state becomes `reconciliation`.

## Runtime reconciliation

`reconciliation` is a governance state, not an automatic retry queue.

Admin-only decisions are:

- `accept_completed`: requires durable output; audit-before-completed.
- `mark_failed`: state-first fail-closed terminal decision.
- `retry`: requires `allow_replay=true`, requires an operator note, and is rejected when durable output exists.

Normal cancel is rejected for `completing` and `reconciliation` tasks so ambiguous side effects cannot be hidden behind a canceled status.

Admin API:

```text
POST /v1/tasks/{id}/reconcile
```

This API is intentionally not exposed through model tools, MCP or A2A.

## Workflow V14

New workflow runs use a distinct `running_v14` status and a stable key per step:

```text
king-workflow:<run-id>:step:<index>:<step-id>
```

If Runtime task creation succeeds but the process crashes before `CurrentTaskID` is persisted, recovery calls idempotent creation with the same key and reattaches to the existing Task instead of creating another one.

Workflow behavior for Runtime ambiguity:

- `pending_audit` -> workflow reconciliation
- `reconciliation` -> workflow reconciliation
- `completing` -> continue waiting for completion evidence
- failed/canceled -> workflow failed
- completed -> audited step advance

## Mission V14

Mission dispatch now persists the Mission before creating child Runtime Tasks. Every child slot receives a stable key:

```text
king-mission:<mission-id>:task:<index>:<agent-id>
```

Each Task ID is persisted immediately after idempotent creation. Recovery resumes missing slots without duplicating already-created child work.

If a linked or newly recovered child task is `pending_audit` or `reconciliation`, the entire Mission enters reconciliation rather than remaining indefinitely `running`.

The model-facing `platform_mission_dispatch` tool is routed through the V14 dispatcher.

## Session and inbound execution

Production session submission uses the durable V14 path. Once Runtime task creation succeeds, failure to synchronize derived Session state is not returned as a retry-safe submission failure.

Inbound webhook execution uses durable receipts:

- `processing`
- `task_created`
- `accepted`
- `reconciliation`
- `failed`

Known duplicate events return the existing Task identity. An ambiguous processing window never blindly replays work.

Admin-only inbound reconciliation can:

- link a receipt to an existing Task whose durable metadata matches channel/sender/session evidence
- mark an unresolved receipt failed

There is intentionally no automatic inbound retry action.

## Authority and budgets

Capability Envelopes remain immutable grants with audit-before-activation and fail-closed revocation.

Runtime governance includes:

- hierarchical concurrent-work budgets
- hierarchical cost budgets
- ancestor accounting for delegated descendants
- durable idempotent reservations and charges
- preflight visibility without TOCTOU authorization

Caller-supplied `authority_id` metadata is stripped by the trusted task binding layer. Effective authority is derived from durable Agent identity state.

## Platform trust surface

Production uses the safe V14 control surface.

Path-level scoped permissions distinguish:

- `platform.read`
- `platform.write`
- `platform.automation`
- `platform.admin`

Trust-expanding platform resources are inert until their required audit is durable.

Node execution requires an audited fresh heartbeat. Disabling an Agent prevents its schedule from launching new work.

## Knowledge trust

Long-term knowledge remains trust-separated:

```text
pending_audit -> proposed -> approved/rejected
```

Models may propose knowledge but cannot approve it. Opposing concurrent reviews are serialized so only one decision commits.

## Validation policy

No v1.4 release tag should be created and the PR should remain Draft until the exact final head passes all formal workflows:

- CI
- CodeQL
- Full Validation
- Container Smoke
- Container Validation

Known earlier all-green checkpoints remain useful evidence, but they do not substitute for validating the exact final head.
