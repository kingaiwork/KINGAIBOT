# KINGAIBOT v1.4.0

**Secure, durable and model-agnostic digital-employee execution platform for the KING AI intelligent-lifeform system.**

Official website: **https://kingai.work**  
Owner: **USDX TECH LLC / KING AI**  
Contact: **vip@kingai.work**

KINGAIBOT is developed as an independent KING implementation. It is not a model wrapper and it is not a copy of another agent framework. Models are replaceable reasoning resources; durable identity, authority, budgets, work state, memory, evidence, approvals and reconciliation remain owned by KINGAIBOT.

> **Learn the problem. Learn the standard. Design the KING solution. Never clone the implementation.**

---

## English

### Product position

KINGAIBOT is the controlled execution layer of the wider KING AI system. It turns durable digital-employee responsibilities and operator intent into policy-bound work across local tools, workflows, remote workers, devices, services and open agent protocols.

```text
KING AI / Digital Employee
        |
Identity + Responsibility + Memory + Goals
        |
Authority Envelope + Budget + Policy
        |
Approval + Audit
        |
KINGAIBOT Runtime / WorkGraph / Mission
        |
Tools / Workers / Nodes / Channels / MCP / A2A
        |
Evidence -> Completion or Reconciliation
```

The v1.4 security invariant is:

```text
intent
  -> trusted identity
  -> authority envelope
  -> hierarchical budget
  -> policy / approval
  -> audit
  -> execution
  -> evidence
  -> completion or reconciliation
```

The model cannot select its own authority, approve its own high-risk actions, silently promote knowledge to trusted truth, or bypass reconciliation.

### v1.4.0 highlights

#### Crash-safe Runtime

Durable task states are explicit:

```text
pending_audit -> queued -> running -> waiting_approval -> completing -> completed
        \             ambiguous execution/completion             /
         +--------------------> reconciliation <-----------------+
```

- New work is persisted as `pending_audit` before it can become executable.
- `task.created` must be durably audited before `queued` is reached.
- Only `queued` work is automatically re-enqueued after restart.
- Restart during `pending_audit`, `running` or `completing` moves the task to `reconciliation` instead of replaying it blindly.
- Stable idempotent task creation lets durable Workflow/Mission recovery reattach to existing work.
- Same idempotency key + same input returns the same Task; conflicting input is rejected.

#### Two-phase completion

A successful result is not immediately trusted as completed:

```text
running
  -> completing (durable output)
  -> task.completed audit + output SHA-256
  -> completed
```

If completion audit fails, output is retained for inspection and the task enters `reconciliation`.

#### Operator reconciliation

Runtime reconciliation is not an automatic retry queue. Admin-only decisions are:

- `accept_completed` — accept durable output after reconciliation audit.
- `mark_failed` — record a fail-closed terminal decision.
- `retry` — requires an operator note and `allow_replay=true`.
- Retry is blocked when durable output already exists.

```text
POST /v1/tasks/{id}/reconcile
```

This power is not exposed to ordinary model tools, MCP peers or A2A peers.

#### Crash-safe staged approval

Trust-expanding approvals use:

```text
pending -> approving / denying -> audit -> approved / denied -> Task transition
```

Approval cannot become executable before its decision audit is durable. Repeating the same final approval is idempotent and does not enqueue the same Task twice.

```text
POST /v1/approvals/{id}
POST /v1/approvals/{id}/decision
```

Both production routes use the v1.4 staged decision path.

#### Capability Envelopes and hierarchical budgets

Authority is durable data outside the model. Envelopes can constrain capabilities, data scopes, tools, expiry, delegation depth, concurrent work and cost units.

Delegation can only narrow a parent grant. Revoked/expired parents invalidate descendants. Child work counts against ancestor budgets, so sibling delegates share parent ceilings. Preflight APIs explain bottlenecks while execution still rechecks authority and budget atomically.

Caller-supplied `authority_id` is stripped by the trusted task-binding layer; effective authority is derived from trusted Agent identity.

#### WorkGraph

WorkGraph stores durable execution intent and evidence, not hidden model chain-of-thought. Node kinds can represent Think, Read, Transform, Decide, Approve, Execute, Wait, Delegate, Verify, Reconcile and Report work.

High/critical-risk completion can require evidence. Unknown real-world side effects prefer reconciliation over automatic replay.

#### Workflow V14

Each Workflow step receives a stable idempotent Runtime identity. If Runtime task creation succeeds but the process crashes before `CurrentTaskID` is persisted, recovery reattaches to the existing Task instead of creating another one.

Runtime reconciliation propagates to the Workflow.

#### Mission V14

Parallel Missions are persisted before child work is created. Each Mission slot receives a stable Task identity. V14 uses dedicated dispatch/running states and its own synchronizer, so it does not race the compatibility synchronizer. Child reconciliation propagates to the whole Mission.

The model-facing `platform_mission_dispatch` tool uses the v1.4 idempotent dispatcher.

#### Durable Sessions and inbound channels

Session submission treats successful Runtime Task creation as execution truth even if derived Session synchronization later fails.

Inbound receipts are durable:

```text
processing -> task_created -> accepted
ambiguous  -> reconciliation
terminal   -> failed
```

Features include per-channel credentials, sender allowlists, durable channel-to-session mapping, `event_id` deduplication, duplicate Task identity return, conservative ambiguity handling and admin reconciliation.

Normal `processing` receipts do not trigger reconciliation alerts.

#### Reviewed long-term knowledge

Long-term knowledge is trust-separated from model output:

```text
pending_audit -> proposed -> approved / rejected
```

Models may propose knowledge but cannot approve it. Trusted search returns approved knowledge only. Opposing concurrent reviews cannot both commit.

#### Platform identities and scoped access

Roles:

- `viewer`
- `operator`
- `automation`
- `admin`

Path-level permissions:

- `platform.read`
- `platform.write`
- `platform.automation`
- `platform.admin`

Access keys are one-time-issued and only verifier hashes are persisted. Trust expansion is audit-gated; disable/revoke is fail-closed.

#### Multi-node worker runtime

`kingworker` is the reference capability-scoped remote worker. Its built-in capability set is intentionally small:

- `system.info`
- sandboxed `file.read`
- sandboxed `file.write`
- allowlisted HTTPS `http.get`

Workers receive worker credentials rather than admin credentials. Cluster uses bounded leases, replay policy, authority rechecks and reconciliation for ambiguous side effects.

#### Safe node heartbeat trust

Node registration starts offline. Only an audited heartbeat can promote a Node to online. Status/list reads may demote stale nodes but cannot promote an unaudited Node.

#### Model-provider fabric

Provider directions include OpenAI-compatible APIs plus native Anthropic and Gemini adapters, with priority/fallback, bounded retry and circuit breaking. Credentials are referenced through environment variables instead of persisted secrets.

Models are replaceable. KINGAIBOT authority and work identity do not depend on one vendor.

#### Open interoperability

Compatibility boundaries include:

- MCP 2026-07-28
- A2A 1.0
- plugin adapters
- channel adapters
- remote Workers

These protocols are boundary adapters, not the authoritative internal architecture, and they do not implicitly inherit KING authority or memory.

#### Operations and observability

- `/healthz`
- `/readyz`
- `/v1/platform/status`
- `/v1/platform/metrics`
- Runtime / Workflow / Mission / Inbound reconciliation counters
- `attention_required`

### Programs

- `kingagentd` — core daemon and control plane
- `kingagent` — operator CLI
- `kingworker` — remote capability-scoped worker
- `kingconsole` — local web Control Center and restricted API proxy

Current product/runtime version: **1.4.0**.

### Quick start

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

Run a task:

```bash
export KINGAGENT_ADMIN_TOKEN="...same admin token..."
go run ./cmd/kingagent run "Create hello.txt in the workspace with a short greeting"
```

Start the Control Center:

```bash
go run ./cmd/kingconsole -api http://127.0.0.1:18888
```

Remote Worker:

```bash
export KINGAIBOT_WORKER_TOKEN="kaw_worker_..."
go run ./cmd/kingworker \
  -server http://127.0.0.1:18888 \
  -workspace ./worker-data \
  -allow-host api.github.com
```

Non-loopback coordinators require HTTPS.

### Release engineering

The v1.4 release path includes Go **1.26.6**, format/vet, unit and Race tests, `govulncheck`, CodeQL, native macOS/Windows validation, three container builds, six-target archives, CycloneDX SBOM, SHA-256 checksums, release manifest, provenance/attestation and Sigstore-oriented signing.

```bash
./scripts/build-release-v14.sh
```

Output:

```text
dist-v14/
```

### Installation repository

Repository: **kingaiwork/KINGAIBOT**

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

Windows PowerShell:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'; $env:KINGAGENT_REQUIRE_SIGNATURE='1'; irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

For production, prefer a reviewed immutable signed release/tag rather than a moving branch.

### Scope boundary

KINGAIBOT provides secure primitives for digital employees, durable work, tools, workers, channels and open protocols. Vendor-specific transports and privileged device drivers should use bounded adapters, MCP, plugins or dedicated Workers rather than expanding the trusted daemon unnecessarily.

### Originality and provenance

KINGAIBOT follows a clean-room development policy: requirements come from KING product needs; internal concepts/data models are KING-owned; public standards are used only at compatibility boundaries; dependency provenance is inventoried; release assets carry SBOM/provenance controls.

See [`docs/ORIGINALITY_IP_POLICY.md`](docs/ORIGINALITY_IP_POLICY.md).

### Documentation

- [Documentation Index](docs/README.md)
- [v1.4 Governance Status](docs/V14_GOVERNANCE_STATUS.md)
- [Platform](docs/PLATFORM.md)
- [Authority-Bound Orchestration](docs/ORCHESTRATION.md)
- [Originality & IP Policy](docs/ORIGINALITY_IP_POLICY.md)
- [Product](docs/PRODUCT.md)
- [Usage](docs/USAGE.md)
- [Roadmap](docs/ROADMAP.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](docs/SECURITY.md)
- [Evolution](docs/EVOLUTION.md)
- [Protocols](docs/PROTOCOLS.md)
- [Deployment](docs/DEPLOYMENT.md)
- [API](docs/API.md)
- [Validation](docs/VALIDATION.md)
- [Residual Risks](docs/RESIDUAL-RISKS.md)

---

# 中文

## 产品定位

**KINGAIBOT v1.4.0 是 KING AI 面向数字员工、私有部署和企业自动运营的安全执行平台。**

它不是“给大模型套一个聊天界面”，也不是复制其他 Agent Framework。模型只是可替换的推理资源；数字员工的身份、职责、权限、预算、任务、长期记忆、技能、客户上下文、工作历史、证据和治理状态属于 KINGAIBOT。

```text
意图
 -> 可信身份
 -> Capability Envelope
 -> 分层预算
 -> Policy / Approval
 -> Audit
 -> Execution
 -> Evidence
 -> Completion 或 Reconciliation
```

模型不能自行扩大权限，不能自己批准高风险动作，也不能把自己生成的内容直接升级为可信知识。

## v1.4.0 关键升级

### 崩溃安全 Runtime

```text
pending_audit -> queued -> running -> waiting_approval -> completing -> completed
不确定创建/执行/完成 ---------------------------> reconciliation
```

新任务先进入 `pending_audit`，只有创建审计成功后才能 `queued`。重启时只有 `queued` 可以自动重新入队；`pending_audit / running / completing` 全部进入 reconciliation，防止不确定副作用被重复执行。

Workflow 与 Mission 使用稳定幂等 Task Identity，崩溃恢复时找回原 Task，而不是再建一份。

### 两阶段完成

成功输出先持久为 `Completing`，再写 `task.completed` 审计和输出 SHA-256，最后才允许 `Completed`。完成审计失败时保留输出证据并进入 reconciliation。

### 管理员 Reconciliation

管理员可以：

- `accept_completed`
- `mark_failed`
- `retry`

`retry` 必须显式 `allow_replay=true` 并带操作说明；已有持久输出时禁止直接 retry。

```text
POST /v1/tasks/{id}/reconcile
```

模型、普通 MCP 和 A2A 对端没有该权限。

### Staged Approval

```text
pending -> approving / denying -> audit -> approved / denied
```

审批审计成功之前不会获得执行权限。重复 approved 决定不会把同一个 Task 重复放入执行队列。

### Capability Envelope + 分层预算

可限制能力、数据范围、工具、过期时间、委派深度、最大并发和 Cost Units。子授权只能缩小父授权；父授权失效后后代同步失效；子工作同时消耗祖先预算。

调用方夹带的 `authority_id` 会被可信绑定层剥离，真正权限来自持久 Agent Identity。

### WorkGraph

WorkGraph 保存可审计工作状态和 Evidence，不保存模型隐藏思维链。高风险节点可以强制证据门，真实世界副作用不确定时优先进入 reconciliation。

### Workflow V14 / Mission V14

Workflow 每一步、Mission 每个子任务都有稳定幂等 Task Identity。Task 已经创建但上层状态尚未落盘时发生崩溃，恢复会重新关联原 Task。

子 Task 进入 reconciliation 时，会向 Workflow / Mission 上卷，不再永久假装 running。

### Session + Inbound Gateway

- 每个 Channel 独立 Token
- Sender 白名单
- Channel → Session 持久映射
- `event_id` 去重
- 重复事件返回原 Task ID
- 不确定 processing 窗口不盲重放
- Admin 可关联已存在且元数据匹配的 Task，或明确标记失败

普通 `processing` 属于正常状态，不会误触发 reconciliation 告警。

### 长期知识信任分层

```text
pending_audit -> proposed -> approved / rejected
```

模型可以提出知识，但不能自我批准。可信搜索只返回 approved 内容，并发相反 Review 只能有一个最终提交。

### 身份与细粒度权限

角色：`viewer / operator / automation / admin`。

平台权限：`platform.read / platform.write / platform.automation / platform.admin`。

API Key 一次性显示，只持久化校验值；启用/提权必须经过审计，禁用/吊销 Fail-Closed。

### Worker / 多节点执行

`kingworker` 默认只提供：

- `system.info`
- 沙箱 `file.read`
- 沙箱 `file.write`
- 白名单 HTTPS `http.get`

Worker 不拿 Admin Token。Lease、Replay Policy、Authority Recheck 和 Reconciliation 都由 Coordinator 治理。

### Node 心跳信任

Node 注册后默认 Offline，只有成功写入审计的 heartbeat 才能提升为 Online。状态查询只能降级过期节点，不能偷偷提升节点信任。

### 多模型与开放协议

Provider 可替换：OpenAI-compatible、Anthropic、Gemini，以及后续本地/其他模型。

开放边界：MCP 2026-07-28、A2A 1.0、Plugin、Channel Adapter、Worker。开放协议只是兼容层，不自动继承 KING 内部 Memory 或 Authority。

### 运维监控

- `/healthz`
- `/readyz`
- `/v1/platform/status`
- `/v1/platform/metrics`
- `attention_required`
- Runtime / Workflow / Mission / Inbound reconciliation 计数

### 四个程序

- `kingagentd`：核心 Daemon / Control Plane
- `kingagent`：管理员 CLI
- `kingworker`：远程 Worker
- `kingconsole`：本地 Web Control Center

当前正式代码身份统一为 **1.4.0**。

## 快速启动

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

```bash
export KINGAGENT_ADMIN_TOKEN="...same admin token..."
go run ./cmd/kingagent run "Create hello.txt in the workspace with a short greeting"
```

```bash
go run ./cmd/kingconsole -api http://127.0.0.1:18888
```

远程 Worker：

```bash
export KINGAIBOT_WORKER_TOKEN="kaw_worker_..."
go run ./cmd/kingworker \
  -server http://127.0.0.1:18888 \
  -workspace ./worker-data \
  -allow-host api.github.com
```

非 Loopback Coordinator 必须使用 HTTPS。

## v1.4 发布与验证

正式流程包括 Go 1.26.6、gofmt、go vet、Unit/Race、govulncheck、CodeQL、macOS/Windows 原生测试、Server/Worker/Console 容器验证、六目标归档、CycloneDX SBOM、SHA-256、Release Manifest、Provenance/Attestation 和 Sigstore-oriented Signing。

```bash
./scripts/build-release-v14.sh
```

产物：

```text
dist-v14/
```

## 私有部署边界

KINGAIBOT 可以作为企业数字员工系统的执行底座部署在 PC、服务器、VPS 或私有环境。高权限浏览器、移动设备、消息平台和行业系统适配器建议运行在独立 Worker/Adapter 中，而不是把全部第三方代码塞进核心 Daemon，以缩小供应链和权限风险。

## 原创开发原则

KINGAIBOT 使用 Clean-Room 原创开发：从 KING 自身产品需求定义问题，自主设计内部概念/数据模型/状态机/安全边界，开放标准只用于互操作接口，第三方依赖保留来源与许可证记录，发布过程保留 SBOM / Provenance。

详见 [`docs/ORIGINALITY_IP_POLICY.md`](docs/ORIGINALITY_IP_POLICY.md)。

## 文档

- [文档索引](docs/README.md)
- [v1.4 Governance Status](docs/V14_GOVERNANCE_STATUS.md)
- [Platform](docs/PLATFORM.md)
- [Authority-Bound Orchestration](docs/ORCHESTRATION.md)
- [Originality & IP Policy](docs/ORIGINALITY_IP_POLICY.md)
- [Product](docs/PRODUCT.md)
- [Usage](docs/USAGE.md)
- [Roadmap](docs/ROADMAP.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](docs/SECURITY.md)
- [Evolution](docs/EVOLUTION.md)
- [Protocols](docs/PROTOCOLS.md)
- [Deployment](docs/DEPLOYMENT.md)
- [API](docs/API.md)
- [Validation](docs/VALIDATION.md)
- [Residual Risks](docs/RESIDUAL-RISKS.md)

---

Copyright © USDX TECH LLC / KING AI. All rights reserved except where a file or dependency states otherwise.
