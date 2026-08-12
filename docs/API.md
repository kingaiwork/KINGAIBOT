# HTTP API

## Authentication boundaries

KINGAIBOT deliberately uses three independent bearer-token namespaces:

- `/v1/*` uses `KINGAGENT_ADMIN_TOKEN`.
- `/mcp` uses `KINGAGENT_MCP_TOKEN`.
- `/a2a` uses `KINGAGENT_A2A_TOKEN`.
- `/healthz`, `/readyz`, and `/.well-known/agent-card.json` are public and return no secret material.

Bearer tokens are compared in constant time. Production operators should keep tokens in the OS secret/environment facility and avoid command-line token flags.

## Health

`GET /healthz` is a liveness endpoint.

`GET /readyz` checks required protocol/admin credentials, provider credential availability, and the runtime audit-integrity state. It intentionally exposes only a coarse readiness result.

## Create task

`POST /v1/tasks`

```json
{"input":"Inspect the workspace and summarize the project","metadata":{"customer":"example"}}
```

Returns HTTP 202 after the task snapshot and creation audit record are durable. If the audit write fails, the task is marked failed and is not scheduled for future recovery execution.

## Approvals

List: `GET /v1/approvals`

Decide:

```http
POST /v1/approvals/{id}
Content-Type: application/json
Authorization: Bearer <admin-token>

{"status":"approved"}
```

An approval is bound to task ID, tool name and canonical argument hash. A changed argument set requires a new approval. Side-effect execution uses a durable execution state so a completed approved action is not blindly replayed; an execution left in an ambiguous `executing` state after a crash requires operator reconciliation.

Denial moves the waiting task to failed.

## Protocol endpoints

See `PROTOCOLS.md` for MCP/A2A headers, method names and current scope.
