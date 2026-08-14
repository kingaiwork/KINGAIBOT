# KINGAIBOT Enterprise Workforce Node

KINGAIBOT 1.3 adds an optional private-runtime bridge for KING AI Enterprise Workforce OS.

The bridge does **not** create a privileged cloud execution path. Every cloud-assigned business task enters the existing `Runtime.Create()` path and therefore remains subject to KINGAIBOT durable task storage, model-provider routing, local `allow / ask / deny` tool policy, local approvals, sandbox restrictions and hash-chained audit logging.

## Enable a node

1. In the KING AI Enterprise console, create an organization and register a private runtime node.
2. Save the one-time `knode_...` token on the customer machine.
3. Configure at least one normal KINGAIBOT AI provider in `config.json` and set its API-key environment variable.
4. Set:

```bash
export KINGAI_WORKFORCE_NODE_TOKEN='knode_...'
export KINGAI_WORKFORCE_URL='https://api.kingai.work'
```

5. Start `kingagentd` normally.

If `KINGAI_WORKFORCE_NODE_TOKEN` is absent, the enterprise bridge is completely disabled and KINGAIBOT behaves as before.

## Automatic lifecycle

When enabled, the daemon automatically:

1. sends a bounded heartbeat,
2. syncs active digital-employee policy,
3. reconciles previously created local durable tasks after restart,
4. atomically pulls one approved cloud task,
5. creates a normal local KINGAIBOT task,
6. lets the local runtime execute or request local approval,
7. reports only terminal task status back to the cloud.

A task that reaches local `waiting_approval` remains local and is **not** treated as completed. The normal KINGAIBOT approval flow must resume it.

## Privacy

`KINGAI_WORKFORCE_REPORT_OUTPUT=false` is the default. In this mode the cloud control plane receives task status but not the local model output.

To explicitly allow bounded result text to return to the cloud:

```bash
export KINGAI_WORKFORCE_REPORT_OUTPUT=true
export KINGAI_WORKFORCE_MAX_REPORT_BYTES=8192
```

Do this only when the organization's data policy permits it.

## Security rules

- Production control-plane URLs must use HTTPS.
- Insecure HTTP is accepted only for localhost/loopback development and only when explicitly enabled.
- Authenticated redirects are never automatically followed.
- The node token is read from the environment; it is never written to config or logs by the bridge.
- Sync rejects any cloud policy claiming arbitrary shell access or claiming it can bypass local approval.
- Cloud employee skill lists describe business intent. They do not grant local tool permissions.
- Local tool policy always wins.
- The bridge uses bounded request and response bodies.
- After a restart, cloud/local task relationships are reconstructed from local durable-task metadata when possible.
- Ambiguous work is not blindly replayed. A cloud task claimed immediately before a machine failure may require operator reconciliation instead of unsafe duplicate execution.

## Environment variables

| Variable | Default | Purpose |
|---|---:|---|
| `KINGAI_WORKFORCE_NODE_TOKEN` | disabled | One-time-issued node credential; presence enables bridge |
| `KINGAI_WORKFORCE_URL` | `https://api.kingai.work` | Commercial/control-plane API |
| `KINGAI_WORKFORCE_HEARTBEAT_SECONDS` | 60 | Node heartbeat cadence |
| `KINGAI_WORKFORCE_SYNC_SECONDS` | 120 | Employee/workflow policy sync cadence |
| `KINGAI_WORKFORCE_POLL_SECONDS` | 8 | Task polling cadence |
| `KINGAI_WORKFORCE_REQUEST_TIMEOUT_SECONDS` | 30 | Per-request timeout |
| `KINGAI_WORKFORCE_REPORT_OUTPUT` | false | Permit bounded local result upload |
| `KINGAI_WORKFORCE_MAX_REPORT_BYTES` | 8192 | Maximum returned result/error bytes |
| `KINGAI_WORKFORCE_ALLOW_INSECURE_HTTP` | false | Loopback development only |

## systemd

The existing service already uses:

```text
EnvironmentFile=/etc/kingagent/kingagent.env
```

Copy `deploy/systemd/kingagent.env.example`, fill the secrets locally, restrict it to the service account, and never commit the filled file.

## Separation of responsibilities

```text
KINGAIASE / Cloudflare
  commerce, account, license, organization,
  digital employee definitions, approvals,
  task state, signed downloads
             |
             v
KINGAIBOT / customer machine
  provider keys, local memory, local files,
  durable execution, tool permissions,
  local approval, MCP/A2A, audit chain
             |
             v
customer systems
```
