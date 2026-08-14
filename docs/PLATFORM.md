# KINGAIBOT Platform Control Plane v1.3

## Purpose

KINGAIBOT v1.3 expands the hardened v1.2 execution core into a durable agent platform without moving high-risk authority into plugins, channels, schedules or child agents.

The design rule is simple:

```text
User / Channel / Schedule / Workflow / Mission
                 |
         Platform Control Plane
                 |
        Agent + Tool Registry
                 |
     Policy -> Exact Approval -> Audit
                 |
 Built-in / Plugin / Channel / Node / MCP / A2A
```

The platform layer coordinates work. The core execution layer remains the authority boundary.

## Durable platform objects

All platform state is stored under `data/platform/` with atomic file replacement and restrictive file permissions.

- `agents/` — operator-defined agent profiles and role prompts
- `sessions/` — durable conversation turns linked to runtime task IDs
- `schedules/` — recurring schedules with persisted next-run state
- `workflows/` — bounded sequential workflow definitions
- `workflow-runs/` — restart-resumable workflow executions
- `missions/` — bounded parallel multi-agent mission dispatch and aggregation
- `nodes/` — registered desktop/server/mobile/browser-capable execution nodes
- `plugins/` — remote plugin manifests with integrity hashes and env-referenced credentials
- `channels/` — outbound channel adapters
- `skills/` — operator-installed instructions with SHA-256 content identity

## Security model

Platform objects do not grant authority by themselves.

Agent-triggered extension tools are registered through `tool.Extension` and executed by `Registry.ExecuteAny`. Every extension call therefore passes the same controls as built-in tools:

1. tool policy evaluation (`allow / ask / deny`),
2. canonical argument hashing,
3. exact approval binding for `ask`,
4. durable execution-state recording,
5. hash-chained audit events,
6. fail-closed behavior when audit persistence is unhealthy.

A plugin, channel or node cannot silently gain shell, filesystem or network authority merely because it is registered.

## Extension tools

The v1.3 platform exports these model-visible tools:

- `platform_agents_list`
- `platform_skills_list`
- `platform_nodes_list`
- `platform_schedule_create`
- `platform_mission_dispatch`
- `platform_plugin_call`
- `platform_channel_send`
- `platform_node_action`

Recommended production policy:

```json
{
  "security": {
    "default_tool_policy": "deny",
    "tool_policies": {
      "platform_agents_list": "allow",
      "platform_skills_list": "allow",
      "platform_nodes_list": "allow",
      "platform_schedule_create": "ask",
      "platform_mission_dispatch": "ask",
      "platform_plugin_call": "ask",
      "platform_channel_send": "ask",
      "platform_node_action": "ask"
    }
  }
}
```

## Sessions

A session is a durable logical conversation. Each user turn becomes a normal KINGAIBOT task and keeps the runtime task ID. Reading the session synchronizes completed task output back into the turn.

Agent profiles can be attached to sessions. Their role prompt is operator-defined data and is composed into the task input; it does not bypass the immutable runtime system safety prompt.

## Schedules

Schedules persist their `next_run_at` before dispatching the task. This prevents the scheduler loop from repeatedly firing the same due instance during a single process lifetime.

Intervals are bounded to 60 seconds through 31 days. Scheduled tasks carry metadata identifying the schedule and optional agent profile.

Agent-created schedules should normally use an `ask` policy so persistent future execution requires explicit approval.

## Workflows

Workflows contain up to 64 sequential steps. A workflow run stores:

- current step,
- current task ID,
- all task IDs,
- prior outputs,
- terminal state and error.

Running workflow records are recovered after daemon restart. If a step task already exists, the workflow resumes by observing that task rather than blindly creating a duplicate.

Previous step output is explicitly labeled as untrusted context when passed to the next step.

## Multi-agent missions

A mission can dispatch the same objective to up to 32 operator-defined agent profiles in parallel. Each child task remains an ordinary durable KINGAIBOT task with its own policy and approval boundary.

Mission status aggregates child task state into `running`, `completed` or `partial_failure`.

This provides a safe baseline for swarm-style delegation without recursive privilege amplification.

## Nodes

Nodes describe external execution endpoints such as:

- desktop automation hosts,
- browser-control hosts,
- mobile device bridges,
- edge workers,
- specialized hardware agents.

Node calls use `platform_node_action`. Public endpoints require HTTPS. Explicit insecure HTTP is accepted only for loopback addresses. Credentials are referenced by environment-variable name rather than persisted as secrets.

## Plugins

Plugins are remote capability adapters. A plugin manifest receives a canonical SHA-256 identity derived from name, version, endpoint and declared capabilities.

`platform_plugin_call` performs a bounded POST request through the guarded network client. Redirects are disabled and response size is bounded. Secrets remain in environment variables.

This remote-plugin design deliberately avoids loading arbitrary third-party code into the trusted KINGAIBOT daemon process.

## Channels

Channels use the same guarded remote adapter model. A channel implementation can bridge Telegram, Discord, Slack, WhatsApp, email, WebChat or another transport outside the trusted core.

Agent-triggered outbound sends use `platform_channel_send` and therefore remain policy/approval mediated.

## Skills

Skills are operator-installed instruction records with a SHA-256 content identity. They are discoverable by the agent through `platform_skills_list`.

Skills are data, not executable authority. Installing a skill does not alter policy or automatically grant tools.

## Admin API

All platform HTTP endpoints are mounted below `/v1/platform/` and are protected by the existing admin bearer secret.

Important endpoints:

```text
GET  /v1/platform/capabilities
GET  /v1/platform/agents
POST /v1/platform/agents
GET  /v1/platform/sessions
POST /v1/platform/sessions
GET  /v1/platform/sessions/{id}
POST /v1/platform/sessions/{id}/messages
GET  /v1/platform/schedules
POST /v1/platform/schedules
POST /v1/platform/schedules/{id}/enabled
GET  /v1/platform/workflows
POST /v1/platform/workflows
POST /v1/platform/workflows/{id}/run
GET  /v1/platform/workflow-runs
GET  /v1/platform/nodes
POST /v1/platform/nodes
POST /v1/platform/nodes/{id}/heartbeat
GET  /v1/platform/plugins
POST /v1/platform/plugins
GET  /v1/platform/channels
POST /v1/platform/channels
GET  /v1/platform/skills
POST /v1/platform/skills
GET  /v1/platform/missions
POST /v1/platform/missions
GET  /v1/platform/missions/{id}
```

## Relationship to OpenClaw-style capabilities

The control plane provides native primitives corresponding to the major agent-platform categories: sessions, persistent automation, skills, plugins, channels, multi-agent delegation and device/browser nodes. Channel-specific and device-specific adapters remain external modules rather than privileged code inside the daemon.

This is intentional: feature coverage is expanded without collapsing all authority into one always-on gateway process.

## Next hardening layers

The v1.3 control plane establishes stable interfaces for later additions including OIDC/SSO/RBAC, transactional multi-node leases, semantic/vector memory backends, OpenTelemetry export, browser-node reference implementations, signed plugin catalogs and staged autonomous repair proposals.

Those additions should extend these boundaries rather than bypass them.
