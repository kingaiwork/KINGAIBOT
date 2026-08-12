# Protocol Interoperability

## MCP 2026-07-28 profile

The server exposes a stateless JSON-RPC HTTP endpoint at `/mcp` with:
- `server/discover`
- `tools/list`
- `tools/call`

Non-discovery requests require `MCP-Protocol-Version: 2026-07-28`. Method/name routing headers are validated when present. Results use explicit result types. A tool that needs human approval returns an input-required result carrying the durable approval identifier and an elicitation-shaped request; the external protocol caller cannot silently self-approve the local security decision in this release.

`/mcp` uses `KINGAGENT_MCP_TOKEN`, independent from the Admin API credential.

Remote MCP servers are explicitly configured in `protocols.mcp_servers`. Remote calls use the same guarded networking layer and do not grant arbitrary URL access.

This is a focused tools profile, not a claim that every optional MCP extension is implemented.

## A2A 1.0 profile

The public Agent Card is at:

```text
/.well-known/agent-card.json
```

For remote deployments, configure `server.base_url` to the canonical HTTPS origin. If it is omitted, the Agent Card intentionally advertises loopback instead of reflecting an untrusted Host header.

The JSON-RPC endpoint is `/a2a`; requests require `A2A-Version: 1.0` plus the independent `KINGAGENT_A2A_TOKEN` bearer credential.

Supported A2A v1 operations:
- `SendMessage`
- `GetTask`
- `ListTasks`
- `CancelTask`

`SendMessage` is async-first and returns the durable task state rather than waiting for an unbounded synchronous mission. Internal task lifecycle values map to A2A `TASK_STATE_*` values and incoming text uses `ROLE_USER` semantics.

Streaming, push notifications, extended cards and other optional bindings are deliberately not advertised until implemented.

Remote A2A peers are explicitly configured in `protocols.a2a_peers` and called through `a2a_send`.
