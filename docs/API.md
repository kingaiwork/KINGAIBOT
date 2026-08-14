# KINGAIBOT v1.3 API

All JSON administration endpoints use bearer authentication. Core Admin, MCP and A2A credentials remain separate. Platform endpoints may also accept scoped platform access keys according to their permission level.

## Core runtime

```text
GET  /healthz
GET  /readyz
POST /v1/tasks
GET  /v1/tasks
GET  /v1/tasks/{id}
POST /v1/tasks/{id}/cancel
GET  /v1/approvals
POST /v1/approvals/{id}
GET  /v1/evolution/proposals
```

`POST /v1/tasks` returns `202 Accepted` after a durable task record is created and scheduled.

## MCP and A2A

```text
POST /mcp
POST /a2a
GET  /.well-known/agent-card.json
```

MCP and A2A use credentials distinct from the Admin token when enabled.

## Platform Control Plane

### Capabilities and status

```text
GET /v1/platform/capabilities
GET /v1/platform/status
GET /v1/platform/metrics
```

`/v1/platform/metrics` emits Prometheus-style text counters.

### Agent profiles

```text
GET  /v1/platform/agents
POST /v1/platform/agents
```

Example:

```json
{
  "name": "researcher",
  "description": "Research agent",
  "system_prompt": "Be concise and cite evidence."
}
```

Agent profile prompts are operator data. They do not replace the immutable runtime safety prompt.

### Durable sessions

```text
GET  /v1/platform/sessions
POST /v1/platform/sessions
GET  /v1/platform/sessions/{id}
POST /v1/platform/sessions/{id}/messages
```

A session turn stores the durable Runtime Task ID. Reading a session synchronizes terminal task output back into the turn.

### Schedules

```text
GET  /v1/platform/schedules
POST /v1/platform/schedules
POST /v1/platform/schedules/{id}/enabled
```

Intervals are bounded. Persistent schedule creation should normally use an `ask` tool policy when requested by an agent.

### Workflows

```text
GET  /v1/platform/workflows
POST /v1/platform/workflows
POST /v1/platform/workflows/{id}/run
GET  /v1/platform/workflow-runs
```

Workflow runs retain step/task state so a daemon restart can observe an existing task instead of blindly creating a duplicate.

### Multi-agent missions

```text
GET  /v1/platform/missions
POST /v1/platform/missions
GET  /v1/platform/missions/{id}
```

Parallel mission fan-out is bounded. Each child remains an ordinary policy-controlled KINGAIBOT task.

### Nodes, plugins, channels and skills

```text
GET  /v1/platform/nodes
POST /v1/platform/nodes
POST /v1/platform/nodes/{id}/heartbeat

GET  /v1/platform/plugins
POST /v1/platform/plugins

GET  /v1/platform/channels
POST /v1/platform/channels

GET  /v1/platform/skills
POST /v1/platform/skills
```

Remote endpoints require HTTPS except explicitly enabled loopback HTTP. Credentials are referenced by environment-variable name.

## Scoped identities and API keys

Identity and key lifecycle endpoints require platform Admin authority.

```text
GET  /v1/platform/identities
POST /v1/platform/identities
POST /v1/platform/identities/{id}/enabled

GET  /v1/platform/access-keys
POST /v1/platform/access-keys
POST /v1/platform/access-keys/{id}/revoke
```

Roles:

- `viewer`
- `operator`
- `automation`
- `admin`

A newly issued access key is returned once. KINGAIBOT persists only its SHA-256 verifier, prefix and lifecycle metadata.

## Inbound channel gateway

```text
POST /v1/inbound/{channel_id}
```

Inbound authentication uses the Channel's own bearer-token environment variable, not the core Admin token.

Example payload:

```json
{
  "event_id": "telegram-update-123",
  "sender": "user-42",
  "text": "Summarize today's report",
  "metadata": {
    "transport": "telegram-adapter"
  }
}
```

`event_id` is used for durable idempotency. A webhook retry returns the existing receipt/task rather than intentionally creating another task.

## Reviewed knowledge graph

Scoped read API exposes **approved knowledge only**:

```text
GET /v1/knowledge/items
GET /v1/knowledge/items/{id}
GET /v1/knowledge/search?q=...&scope=...
GET /v1/knowledge/neighbors?entity=...&scope=...
```

Admin proposal/review API:

```text
GET  /v1/knowledge/admin/items
POST /v1/knowledge/admin/items
GET  /v1/knowledge/admin/items/{id}
POST /v1/knowledge/admin/items/{id}/review
```

Agent-created long-term knowledge starts as `proposed`. It is excluded from trusted search until an Admin review records `approved`.

## Cluster coordinator

### Admin API

```text
GET  /v1/cluster/workers
POST /v1/cluster/workers
POST /v1/cluster/workers/{id}/enabled
GET  /v1/cluster/jobs
POST /v1/cluster/jobs
POST /v1/cluster/jobs/{id}/reconcile
```

Worker registration returns a one-time Worker token. Only its verifier hash is persisted.

Job replay policy:

- `manual` — default; an expired ambiguous lease moves to `reconciliation`.
- `safe` — use only for operations independently known to be idempotent/replay-safe; an expired lease may be requeued.

### Worker protocol

Worker endpoints authenticate with Worker credentials only:

```text
POST /v1/cluster/worker/heartbeat
POST /v1/cluster/worker/lease
POST /v1/cluster/worker/complete
```

A lease contains a one-time lease token. Completion requires the same Worker identity, valid lease token and non-expired lease.

## Controlled evolution

Admin control API:

```text
GET  /v1/evolution/control/proposals
POST /v1/evolution/control/proposals
GET  /v1/evolution/control/proposals/{id}
POST /v1/evolution/control/proposals/{id}/evaluations
POST /v1/evolution/control/proposals/{id}/decisions
```

Lifecycle:

```text
proposed
  -> evaluation_failed | review_required
  -> approved | rejected
  -> staged
  -> released
  -> rolled_back
```

Release requires a staged SHA-256 artifact identity, signature-verification record and passed health status. The controller records trust state; it does not allow a model to edit/deploy production source directly.

## Extension tools visible to the agent

Depending on policy/configuration, the tool registry can expose:

```text
platform_agents_list
platform_skills_list
platform_nodes_list
platform_schedule_create
platform_mission_dispatch
platform_plugin_call
platform_channel_send
platform_node_action

knowledge_search
knowledge_neighbors
knowledge_propose

cluster_workers_list
cluster_jobs_list
cluster_job_submit

evolution_proposals_list
evolution_propose_improvement
```

All extension tools use the same `allow / ask / deny`, exact approval and hash-chained audit boundary as core tools.

## Error and safety behavior

- Unknown or malformed JSON is rejected.
- Request/response sizes are bounded.
- Trust-changing operations are fail-closed around audit persistence.
- Remote credentials are not returned in normal list APIs.
- Raw platform and Worker tokens are intended to be returned only at issuance.
- Ambiguous external side effects are not blindly replayed by default.
