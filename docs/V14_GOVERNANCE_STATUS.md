# KINGAIBOT v1.4.0 Governance and Replay-Safety Baseline

Status: release-candidate governance baseline  
Owner: USDX TECH LLC / KING AI  
Branch: `agent/v1.4-governance`

## Purpose

KINGAIBOT v1.4 turns governance declarations into runtime-enforced behavior. The model remains replaceable and untrusted for authority decisions. Identity, authority, budgets, durable work state, approvals, reconciliation and evidence belong to KINGAIBOT.

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

The v1.4 Runtime lifecycle is directional and replay-aware:

```text
pending_audit -> queued -> running -> waiting_approval -> completing -> completed
```

Ambiguous state never silently returns to executable work:

```text
pending_audit + process restart -> reconciliation
running + process restart       -> reconciliation
completing + process restart    -> reconciliation
completion audit failure        -> reconciliation
```

Only Tasks that were durably `queued` are automatically re-enqueued after restart.

### Audit-before-queue

New Tasks persist as `pending_audit`. `task.created` must be durably appended before the Task can become `queued` and executable.

If the process restarts while creation/audit/activation is ambiguous, recovery moves the Task to `reconciliation` rather than ignoring it or attempting execution.

### Idempotent task creation

Runtime supports stable idempotent Task creation for durable orchestration. A caller-owned key is SHA-256 hashed; the raw key is not persisted. The digest determines a deterministic Task identity and is paired with an input digest.

Rules:

- same key + same input -> same Task
- same key + different input -> conflict
- repeated creation does not enqueue duplicate work
- ambiguous creation state requires reconciliation rather than auto-promotion

### Two-phase completion

A successful result first becomes `completing`. Output and provider are durable, but the Task is not yet trusted as completed.

The Runtime then appends `task.completed` with the output SHA-256 digest. Only after that audit succeeds may state become `completed`.

If completion audit fails, the output remains available for inspection and state becomes `reconciliation`.

## Runtime reconciliation

`reconciliation` is a governance state, not an automatic retry queue.

Admin-only decisions:

- `accept_completed`: requires durable output and audit-before-completed.
- `mark_failed`: fail-closed terminal decision.
- `retry`: requires an operator note and `allow_replay=true`.
- retry is rejected while durable output exists.

Normal cancel is rejected for `completing` and `reconciliation` Tasks so ambiguous side effects cannot be hidden behind a canceled state.

Admin API:

```text
POST /v1/tasks/{id}/reconcile
```

This API is intentionally not exposed through model tools, MCP or A2A.

## Staged approval

Trust-expanding approval uses:

```text
pending -> approving / denying -> audit -> approved / denied -> Task transition
```

`approving` and `denying` are non-executable staging states. An audit outage cannot leave an unaudited approval executable, and the same staged decision can be resumed after recovery.

Production routes:

```text
POST /v1/approvals/{id}
POST /v1/approvals/{id}/decision
```

Both use the V14 staged state machine. Repeating an already-finalized approval does not enqueue the same Task twice.

## Workflow V14

New Workflow runs use a distinct `running_v14` state and a stable key per step:

```text
king-workflow:<run-id>:step:<index>:<step-id>
```

If Runtime Task creation succeeds but the process crashes before `CurrentTaskID` is persisted, recovery calls idempotent creation with the same key and reattaches to the existing Task instead of creating another one.

Workflow behavior for Runtime ambiguity:

- `pending_audit` -> Workflow reconciliation
- `reconciliation` -> Workflow reconciliation
- `completing` -> continue waiting for completion evidence
- failed/canceled -> Workflow failed
- completed -> audited step advance

## Mission V14

Mission dispatch persists the Mission before creating child Runtime Tasks. Every child slot receives a stable key:

```text
king-mission:<mission-id>:task:<index>:<agent-id>
```

Each Task ID is persisted immediately after idempotent creation. Recovery resumes missing slots without duplicating already-created child work.

V14 uses dedicated `dispatching_v14` and `running_v14` states plus its own synchronizer. If a linked child Task requires reconciliation, the entire Mission enters reconciliation instead of remaining indefinitely running.

The model-facing `platform_mission_dispatch` tool is routed through the V14 dispatcher.

## Session and inbound execution

Production Session submission uses the durable V14 path. Once Runtime Task creation succeeds, failure to synchronize derived Session state is not returned as a retry-safe submission failure.

Inbound webhook execution uses durable receipts:

- `processing`
- `task_created`
- `accepted`
- `reconciliation`
- `failed`

Known duplicates return the existing Task identity. An ambiguous processing window never blindly replays work.

Admin-only inbound reconciliation can:

- link a receipt to an existing Task whose durable metadata matches channel/sender/session evidence
- mark an unresolved receipt failed

Normal `processing` is tracked separately from reconciliation and does not set `attention_required` by itself.

## Authority and budgets

Capability Envelopes are durable grants with audit-before-activation and fail-closed revocation.

Runtime governance includes:

- hierarchical concurrent-work budgets
- hierarchical cost budgets
- ancestor accounting for delegated descendants
- durable idempotent reservations and charges
- advisory preflight visibility without TOCTOU authorization

Caller-supplied `authority_id` metadata is stripped by the trusted task-binding layer. Effective authority is derived from durable Agent identity state.

## WorkGraph and Cluster

WorkGraph represents explicit durable work/evidence state rather than hidden model reasoning. High/critical-risk completion can require evidence.

Cluster and orchestration use conservative side-effect semantics:

- Worker credentials are staged/audited before enablement.
- Jobs are audited before queue/activation.
- WorkGraph-to-Cluster handoff can use durable `held` Jobs.
- Authority is rechecked before lease delivery and completion commit.
- Ambiguous or unauthorized side effects move to reconciliation rather than false success.

## Platform trust surface

Production uses the safe V14 control surface.

Path-level scoped permissions distinguish:

- `platform.read`
- `platform.write`
- `platform.automation`
- `platform.admin`

Trust-expanding platform resources are inert until required audit is durable.

Node registration starts offline. Only an audited fresh heartbeat may promote a Node online. Status/list reads may demote stale Nodes but never promote unaudited Nodes.

Disabling an Agent prevents its schedules from launching new work.

## Knowledge trust

Long-term knowledge remains trust-separated:

```text
pending_audit -> proposed -> approved / rejected
```

Models may propose knowledge but cannot approve it. Opposing concurrent reviews are serialized so only one decision commits.

## Operations and attention

`/v1/platform/status` and `/v1/platform/metrics` surface reconciliation counts for Runtime, Workflow, Mission and inbound receipts.

`attention_required` is raised by reconciliation conditions, not by ordinary inbound `processing`.

## Release engineering

Current runtime identity: **1.4.0**.

The authoritative v1.4 validation/release path uses:

- Go 1.26.6
- CI
- CodeQL
- V1.4 Full Validation
- V1.4 Container Smoke
- V1.4 Container Validation
- `scripts/build-release-v14.sh`
- six target archives under `dist-v14/`
- CycloneDX SBOM
- SHA-256 checksums
- Release Manifest
- provenance/attestation
- Sigstore-oriented release signing

Only one Tag publication workflow is authoritative: `.github/workflows/release.yml`.

## Validation policy

No v1.4 release tag should be created before the exact final PR head passes all formal workflows:

- CI
- CodeQL
- V1.4 Full Validation
- V1.4 Container Smoke
- V1.4 Container Validation

Earlier green checkpoints are useful evidence but do not substitute for exact-final-head validation.
