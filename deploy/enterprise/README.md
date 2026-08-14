# KING AI Enterprise Private Runtime

This package runs a private KINGAIBOT node for KING AI Enterprise Workforce OS.

## What stays local

- AI provider API keys
- local files and workspace data
- local memory
- tool allow/ask/deny policy
- local approvals and audit logs
- full task output by default

The cloud control plane receives task state. Full local output is returned only when `KINGAI_WORKFORCE_REPORT_OUTPUT=true` is explicitly enabled.

## Required values

1. A KING AI account with a plan that includes private runtime.
2. A private runtime node created in the Enterprise console.
3. The one-time `knode_...` token from that node registration.
4. At least one AI provider API key configured in `config.json` / environment.

## Linux / macOS

Copy `kingagent.env.example` to a protected environment file and fill in the values. Export the variables before starting the daemon, or install them through your service manager.

```bash
cp config.example.json config.json
export KINGAGENT_ADMIN_TOKEN='replace-with-random-token'
export KINGAGENT_MCP_TOKEN='replace-with-random-token'
export KINGAGENT_A2A_TOKEN='replace-with-random-token'
export OPENAI_API_KEY='replace-with-provider-key'
export KINGAI_WORKFORCE_NODE_TOKEN='knode_...'
./kingagentd -config ./config.json
```

## Windows PowerShell

```powershell
$env:KINGAGENT_ADMIN_TOKEN='replace-with-random-token'
$env:KINGAGENT_MCP_TOKEN='replace-with-random-token'
$env:KINGAGENT_A2A_TOKEN='replace-with-random-token'
$env:OPENAI_API_KEY='replace-with-provider-key'
$env:KINGAI_WORKFORCE_NODE_TOKEN='knode_...'
.\kingagentd.exe -config .\config.json
```

## Safety

Do not put filled secrets back into GitHub or upload them to the KING AI control plane. The node token is a machine credential. If it is exposed, revoke that runtime node and register a replacement.
