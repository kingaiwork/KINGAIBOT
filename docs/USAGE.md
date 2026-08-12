# KINGAIBOT User & Operations Guide / 使用与运维手册

## English

### 1. Core components

- `kingagentd` — persistent runtime service
- `kingagent` — command-line control client
- `config.json` — runtime configuration
- durable stores — tasks, approvals, memory, evolution proposals and audit state

### 2. First local start

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

### 3. Submit and inspect work

```bash
go run ./cmd/kingagent run "Your authorized task"
go run ./cmd/kingagent tasks
go run ./cmd/kingagent approvals
```

### 4. Security rules

- Keep shell execution disabled unless explicitly required.
- Prefer `ask` for file writes, external HTTP, MCP calls and A2A delegation.
- Keep Admin, MCP and A2A tokens separate.
- Do not embed model API keys in source code or prompts.
- Run the service as a dedicated low-privilege account.
- Put remote deployments behind TLS and an authenticated reverse proxy or private network.
- Back up durable state before upgrades.

### 5. Upgrade principle

Commercial upgrades should come from immutable signed releases and be verified before activation. The updater should keep rollback material until the replacement runtime passes readiness checks.

### 6. Troubleshooting

Check `/healthz`, `/readyz`, runtime logs, task state, approval state and audit integrity first. Do not repeatedly replay a side-effecting task when execution state is ambiguous.

---

## 中文

### 1. 核心组件

- `kingagentd`：长期运行的 Runtime 服务
- `kingagent`：命令行控制客户端
- `config.json`：运行配置
- 持久状态：任务、审批、记忆、进化提案和审计状态

### 2. 首次本地启动

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

### 3. 提交与查看任务

```bash
go run ./cmd/kingagent run "经过授权的任务"
go run ./cmd/kingagent tasks
go run ./cmd/kingagent approvals
```

### 4. 安全使用原则

- 没有明确需要时保持 Shell 关闭。
- 文件写入、外部 HTTP、MCP 调用、A2A 委派优先使用 `ask`。
- Admin、MCP、A2A Token 必须分离。
- 模型 API Key 不写进源码和 Prompt。
- Runtime 使用专用低权限账户运行。
- 远程部署必须使用 TLS，并放在认证网关或私有网络之后。
- 升级前备份 Durable State。

### 5. 升级原则

商用升级应来自不可变、已签名的 Release，并在激活前完成校验。新版没有通过 readiness 检查前，应保留可回滚的旧版本。

### 6. 故障排查

优先检查 `/healthz`、`/readyz`、Runtime 日志、任务状态、审批状态和审计链完整性。如果一个有副作用的操作执行状态不明确，不应盲目重复执行。
