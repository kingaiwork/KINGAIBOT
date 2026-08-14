# KINGAIBOT Authority-Bound Orchestration

Status: v1.3 implementation document
Owner: USDX TECH LLC / KING AI

## Purpose

KINGAIBOT separates reasoning, durable work intent, execution authority and remote worker execution into distinct trust domains.

The orchestration path is:

```text
Trusted Agent identity
  -> Capability Envelope
  -> durable Task / WorkGraph
  -> policy + exact approval
  -> held authority-bound Cluster Job
  -> WorkGraph Running commit
  -> Cluster Job activation
  -> Worker lease
  -> remote execution
  -> evidence
  -> reconciliation / completion
```

A model is never allowed to choose its own `authority_id`, approve its own high-impact work, or mark uncertain external work as complete merely because a tool call returned.

## Capability Envelope

`internal/authority` is the source of execution authority.

An Envelope may bound:

- capabilities
- data scopes
- tool scopes
- budget
- expiry
- delegation permission
- delegation depth

Delegation may only narrow the parent grant. Expiry or revocation of any parent makes descendants ineffective.

For trusted platform-created tasks, the runtime resolves a unique effective grant from the trusted `agent_id` and stores the resulting `authority_id` in durable Task metadata. If multiple effective grants exist for the same subject, resolution fails closed rather than guessing which grant should win.

## WorkGraph

`internal/workgraph` represents durable work intent as a typed DAG rather than transient model context.

Important states include:

```text
pending
  -> ready
  -> approval_required (when configured)
  -> running
  -> reconciling
  -> completed / failed
```

High and critical risk nodes require completion evidence.

For remote execution, only `execute` and `delegate` nodes are dispatchable by the orchestration bridge. The node must be `ready`, have a trusted `owner`, and include a bounded Cluster specification under `inputs.cluster`.

Example:

```json
{
  "id": "write-report",
  "type": "execute",
  "owner": "agent_ops",
  "risk": "high",
  "replay": "manual",
  "inputs": {
    "cluster": {
      "kind": "file.write",
      "payload": {
        "path": "report.txt",
        "content": "verified output"
      },
      "required_capabilities": ["task.execute"],
      "required_tool": "file.write"
    }
  }
}
```

No authority identifier appears in the node input.

## Race-Free Held Job Handoff

A normal queued Cluster Job may be leased immediately by a Worker. That creates an orchestration race if the WorkGraph has not yet durably committed its corresponding node to `running`.

KINGAIBOT solves this with a native `held` Cluster state.

Dispatch sequence:

1. validate the WorkGraph and node;
2. resolve the node Owner's unique effective Capability Envelope;
3. create an authority-bound Cluster Job as `held`;
4. persist the durable orchestration Binding;
5. append the dispatch-held audit event;
6. commit the WorkGraph node from `ready` to `running`;
7. revalidate the authority;
8. atomically activate the Cluster Job from `held` to `queued` while the Cluster mutex is held through the activation audit;
9. only then may a Worker lease the Job.

A `held` Job is never considered by the normal lease selector.

If WorkGraph Start fails, the held Job is canceled before any Worker can see it.

If activation fails and the coordinator can prove the Job remains `held`, the held Job is canceled and the WorkGraph can safely return to `ready` using the narrow `AbortUnleasedStart` transition.

If the system cannot prove that remote execution was impossible, the graph is not rolled back to a false pre-execution state. Recovery and reconciliation resolve the durable outcome.

## Durable Binding

`internal/orchestration` persists a Binding between:

- WorkGraph ID
- WorkGraph Node ID
- Cluster Job ID
- resolved Authority ID
- orchestration state

The deterministic dispatch identifier is derived from the graph and node identities and is used as the Cluster held-job control reference.

The model never supplies this identifier.

Binding states are:

- `held`
- `active`
- `reconciling`
- `completed`
- `failed`

## Authority Checks Across the Remote Lifecycle

Authority is checked more than once because a valid grant can be revoked while work is queued or executing.

### Submission

The requested capability, data scope and/or tool must be explicitly declared and allowed by the Envelope.

An authority-bound remote Job with no declared constraint is rejected.

### Before lease delivery

The coordinator revalidates the Job authority before returning a lease to a Worker. If the grant has expired or been revoked, the Job is failed closed and is not disclosed to the Worker.

### Before completion commit

The coordinator revalidates authority before accepting a Worker result as terminal success.

If authority changed while the Worker was executing, the result is retained as evidence and the Job moves to `reconciliation` instead of `completed`.

## Reconciliation

Reconciliation distinguishes two different actions:

### Record an already-observed outcome

An authorized administrator may mark a reconciliation Job complete or failed after independently verifying external state. This records reality that may already exist outside KINGAIBOT.

### Launch work again

`requeue` is different because it authorizes a new execution attempt. Before requeue, KINGAIBOT revalidates the original Job Authority. Revoked or expired authority cannot be used to relaunch work.

This prevents an administrator reconciliation action from accidentally bypassing the Capability Envelope execution boundary.

## Evidence Propagation

When an authority-bound Cluster Job completes, the orchestration bridge writes completion into the WorkGraph with evidence similar to:

```json
{
  "kind": "cluster_job",
  "reference": "job_...",
  "sha256": "<sha256-of-result>",
  "created_at": "..."
}
```

The WorkGraph output also records the Cluster Job ID and parsed remote result.

High/critical nodes therefore cannot become completed without durable evidence.

## Restart Recovery

The orchestration Bridge starts by reconciling persistent Bindings and Cluster held Jobs.

Recovery can:

- activate a held Job whose graph node is already running;
- start a ready graph node and then activate its existing held Job;
- synchronize queued/leased Jobs to active bindings;
- synchronize reconciliation state;
- propagate terminal Job completion/failure into WorkGraph state;
- cancel orphaned orchestration holds that never obtained a durable Binding.

An orphaned held Job could not have executed because held Jobs are not leaseable.

## Admin API

The bridge is currently an Admin/governance surface:

- `GET /v1/orchestration/bindings`
- `POST /v1/orchestration/sync`
- `POST /v1/orchestration/workgraphs/{graph}/nodes/{node}/dispatch`
- `GET /v1/orchestration/workgraphs/{graph}/nodes/{node}/binding`

The API intentionally has no `authority_id` input.

The model does not receive orchestration approval, dispatch, reconciliation or completion authority through a tool definition.

## Originality Boundary

This implementation follows `docs/ORIGINALITY_IP_POLICY.md`.

The design was derived from KINGAIBOT requirements for least privilege, durable state, race-free execution, evidence-bearing completion and conservative side-effect reconciliation. It does not use third-party agent-framework source code, prompts, internal schemas, UI, class hierarchies or copied tests as implementation material.

General engineering concepts such as DAGs, leases, capability authorization, idempotency, audit logs and reconciliation are used as technical primitives; KINGAIBOT's internal data models, trust boundaries, transition semantics, tests and integration contracts are independently implemented.
