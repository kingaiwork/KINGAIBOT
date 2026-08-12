# Architecture

## Runtime layers

```text
Client / CLI / A2A / MCP
        |
Separated Auth + Rate Limits
        |
Durable Task Runtime ---- Hash-Chained Audit Log
        |                         |
        |                  Periodic Integrity Verify
        |
Agent Loop ----------- Bounded / Redacted Memory
        |
Provider Fabric        Policy + Exact Approval Kernel
        |                  |
Model APIs          Tool Registry
                         |  |  |
                    Local MCP A2A
```

Core invariants:

1. **Model independence.** Provider failure must not corrupt durable task state.
2. **Durability before autonomy.** A task is persisted and creation is audited before it enters a worker queue.
3. **Policy before side effects.** Every tool call passes policy, exact approval binding and audit-health checks before execution.
4. **Conservative crash recovery.** Queued/running tasks can resume after process restart; an ambiguous approved side effect is not blindly replayed.
5. **Evidence before success.** Tool success comes from actual tool output; the model is not trusted to fabricate execution.
6. **Bounded autonomy.** Queue capacity, worker count, steps, request size, task duration, output size, memory size and retries are bounded.
7. **Evolution outside the trust root.** Failures may generate proposals, but production code cannot rewrite/deploy itself directly.

## Persistence

The single-node commercial baseline uses local files/JSONL without an external database. Task, approval and evolution snapshots use same-filesystem atomic replacement. On Unix the parent directory is fsynced; Windows uses replace-existing/write-through replacement semantics. Audit and memory appends are fsynced; memory compaction uses atomic replacement.

The audit chain is verified at startup and periodically. A detected integrity/persistence failure becomes a runtime safety signal and blocks side-effect execution.

For HA/multi-node deployments, replace these stores behind stable interfaces with transactional database/object/stream infrastructure and distributed leases/idempotency.

## Provider fabric

Providers are ordered by priority. Transient failures are retried with a small bound and contribute to a circuit breaker; provider failure falls through to the next enabled provider. Credentials are referenced only by environment-variable name and never embedded in model prompts/config values.

Outbound provider connections use guarded DNS resolution and pinned allowed IP dialing.

## Tool fabric

Local tools:
- `system_info`
- `file_read`
- `file_write`
- `http_get`
- `shell_exec`

Federation tools:
- `mcp_tools_list`
- `mcp_tools_call`
- `a2a_send`

The extension boundary is protocol-first: many new remote capabilities can be added without modifying the core binary. A future WASI component host can add local sandboxed plugin execution while keeping policy/approval outside the plugin trust boundary.
