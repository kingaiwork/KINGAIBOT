# KING AI Enterprise Workforce on KINGAIBOT v1.4

## Boundary

KINGAIASE is the commercial and coordination authority. KINGAIBOT remains the customer-local execution authority.

A cloud digital employee is **not** a local authority identity. A cloud task cannot choose an `authority_id`, create a Capability Envelope, weaken tool policy, bypass approval, or turn a connector into permission.

The execution path is:

```text
KINGAIASE workforce task
  -> service identity (ksv_ token, hash stored in D1)
  -> KINGAIBOT Workforce client
  -> local employee binding (optional)
  -> authority.BoundTaskRuntime.CreateIdempotent
  -> local Agent identity
  -> local Capability Envelope / policy / approvals / audit
  -> local tools / MCP / A2A
```

## Service identity

Create a private runtime in the KING AI Workforce console. The raw `ksv_...` token is shown once. Store it only on the customer machine, for example in `/etc/kingagent/kingagent.env`.

```env
KINGAI_WORKFORCE_URL=https://api.kingai.work
KINGAI_WORKFORCE_SERVICE_TOKEN=ksv_REPLACE_ME
```

The control plane stores only the SHA-256 token hash.

## Local authority binding

By default a cloud employee has no `agent_id`, so v14 cannot derive a Capability Envelope for it.

An operator may explicitly map a cloud employee to a local Agent identity:

```json
{
  "version": 1,
  "employees": {
    "cloud-employee-id": {
      "agent_id": "local.agent.sales"
    }
  }
}
```

Default location:

`/var/lib/kingagent/workforce/bindings.json`

On Unix this file must be `0600` or stricter. Group/world-accessible bindings fail closed because changing this file changes which local Agent identity may be resolved.

Even after a binding exists, `BoundTaskRuntime` deletes caller-supplied `authority_id` and derives effective authority only from the local Authority Store.

## Private connectors

Cloud connector records may contain only:

- provider type;
- display name;
- local alias;
- allowed skill names;
- small non-secret scalar configuration.

Supported authentication modes are only:

- `local-mcp`;
- `local-a2a`;
- `local-secret`.

`local-secret` means the credential is installed into a separate local integration/secret store. It does **not** make the secret visible to the cloud or directly to the task prompt.

The runtime rejects secret-like cloud configuration keys such as token, password, API key, private key, authorization, cookie, or session.

Employee connector access is recomputed locally as the intersection of:

`employee skills ∩ connector allowed skills ∩ binding skill scope`.

## Crash and ambiguity semantics

Cloud tasks are converted to local tasks with a stable idempotency key derived from the cloud task ID. Duplicate delivery therefore does not create duplicate local work.

KINGAIBOT v1.4 has explicit `Completing` and `Reconciliation` states for ambiguous side effects. Workforce does not translate these states into cloud success/failure. An operator must first resolve the local ambiguity using the normal v14 reconciliation surface.

## Privacy

Full local model output is not uploaded by default:

```env
KINGAI_WORKFORCE_REPORT_OUTPUT=false
```

When opt-in is enabled, result output remains bounded by `KINGAI_WORKFORCE_MAX_REPORT_BYTES`.

Provider credentials, local memory, files, Capability Envelopes, approval records, tool policies and connector credentials remain customer-local unless an already-approved local integration explicitly transmits data as part of its business action.
