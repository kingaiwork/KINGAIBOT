# KINGAIBOT

**A secure, durable, multi-agent execution platform and terminal execution layer for the KING AI intelligent-lifeform system from [kingai.work](https://kingai.work).**

Official website: **https://kingai.work**  
Contact: **vip@kingai.work**

> KINGAIBOT is a long-term R&D project of kingai.work. It is developed as an independent, security-first agent platform today and is intended to become the controlled terminal execution layer of the wider KING AI system.

## English

### Project Positioning

KINGAIBOT is not a model wrapper and is not a rename of KING AI. It is the execution platform that turns high-level intelligence into durable, policy-controlled digital work across local tools, remote services, cooperating agents, devices, workers and open agent protocols.

```text
KING AI Intelligent Lifeform
        |
Reasoning / Memory / Governance / Mission Intelligence
        |
Controlled Integration Boundary
        |
KINGAIBOT Platform + Execution Layer
        |
+----------------+----------------+----------------+
|                |                |                |
Local Tools   Workflows        Cluster          Federation
& Sandbox     Schedules        Workers          MCP / A2A
                 |                |
              Missions       Devices / Browser / Edge adapters
                 |
        Channels / Plugins / APIs
```

The central invariant is unchanged as the platform grows:

```text
intent -> policy -> exact approval -> audit -> execution -> evidence
```

New platform features do not automatically receive higher privilege.

### Current v1.3.0 Platform

#### Hardened execution core

- Linux / macOS / Windows Go runtime
- Go **1.26.6** production and release baseline
- Durable tasks with restart recovery and bounded worker queues
- `allow / ask / deny` tool policy
- Exact approvals bound to task + tool + canonical argument hash
- Go `os.Root` traversal-resistant filesystem sandbox
- Safe read/stat/list/atomic-write/mkdir/single-item-delete file capabilities
- No agent-exposed recursive delete in the trusted core
- HTTPS allowlists, SSRF/DNS-rebinding defenses and redirect downgrade controls
- Shell disabled by default and restricted to explicit bare executable allowlists when enabled
- Hash-chained audit/event log with startup and periodic integrity verification
- Fail-closed side-effect execution when audit integrity is unhealthy
- Signed/checksummed update pipeline with health verification and rollback

#### Multi-provider reasoning fabric

- OpenAI-compatible provider API
- Native Anthropic Messages adapter
- Native Gemini function-calling adapter
- Provider priority, bounded retry, fallback and circuit breaker
- Provider type validation at startup
- Credentials referenced by environment variable rather than persisted in configuration
- Guarded outbound networking shared by providers

Models remain replaceable reasoning resources. They are not the operating system and do not define execution authority.

#### Platform Control Plane

Durable platform objects are stored independently from the model context:

- Agent profiles
- Sessions
- Recurring schedules
- Sequential workflows
- Workflow runs
- Parallel missions / multi-agent fan-out
- Device / browser / edge node registrations
- Remote plugin manifests
- Channel adapters
- Integrity-hashed skills

Agent-triggered platform actions are registered through the same extension-tool boundary as core tools, so they still pass policy, approval and audit controls.

#### Identity and scoped access

- Durable platform identities
- `viewer`, `operator`, `automation`, `admin` roles
- Fine-grained platform permissions
- One-time-issued API keys
- Only SHA-256 token verifiers persisted
- Expiration and revocation
- Legacy environment Admin Token remains supported
- Identity/key creation fails closed when trust-changing audit records cannot be written

#### Inbound channel gateway

- Per-channel bearer credentials
- Sender allowlists
- Durable channel-to-session mapping
- `event_id` idempotency receipts
- Webhook retry deduplication so the same inbound event is not blindly executed twice
- Generic outbound channel adapter boundary

Vendor-specific channel transports can be implemented outside the trusted core and connected through the channel/plugin/MCP/worker interfaces.

#### Long-term knowledge and memory

KINGAIBOT separates episodic runtime memory from reviewed long-term knowledge.

Episodic memory:

- bounded record count and context size
- optional expiry
- secret redaction
- raw task-input retention disabled by default
- retrieved memory treated as untrusted context

Reviewed knowledge graph:

- notes, facts, entities, relations, procedures and preferences
- subject / predicate / object relationships
- source, confidence, tags and expiry metadata
- `proposed -> approved / rejected` trust states
- only approved knowledge is returned by trusted search
- agent can propose knowledge but cannot self-approve it
- admin review API separated from read API

#### Capability Envelope + WorkGraph Orchestration

KINGAIBOT now has a durable execution-authority and work-state layer that remains outside model control:

- Capability Envelopes can bound capabilities, data scopes, tools, budgets, expiry and delegation depth.
- Delegation may only narrow authority; parent revocation or expiry invalidates descendants.
- Trusted platform-created tasks resolve authority from trusted Agent identity. The model cannot select an `authority_id` in tool arguments.
- Durable WorkGraphs represent typed DAG work with approval gates, replay policy and evidence requirements.
- High/critical risk WorkGraph nodes cannot complete without evidence.
- WorkGraph execute/delegate nodes can be handed to Cluster through the Admin-only orchestration bridge.
- Cluster uses a native `held` Job state so a remote Worker cannot lease work before the corresponding WorkGraph node is durably running and activation is audited.
- Authority is revalidated at job submission, before lease delivery and before completion commit.
- Authority loss during remote execution retains the Worker result and moves the Job/WorkGraph into reconciliation instead of accepting false completion.
- Admin reconciliation can record already-observed reality, while `requeue` still requires effective execution authority.
- Cluster completion is propagated back to WorkGraph with Job identity and result SHA-256 evidence.
- Persistent orchestration bindings support restart recovery and fail-closed orphan-held-job cleanup.

The detailed state machine and API contract are documented in [`docs/ORCHESTRATION.md`](docs/ORCHESTRATION.md).

#### Multi-node worker runtime

The cluster coordinator provides durable remote execution primitives:

- one-time-issued Worker credentials with only verifier hashes persisted
- declared capabilities
- capability-matched durable jobs
- priorities
- lease tokens and bounded lease duration
- conservative replay policy
- ambiguous lease expiry defaults to `reconciliation`, not blind replay
- explicitly replay-safe jobs may be requeued
- duplicate completion protection
- operator reconciliation API

`kingworker` is the reference cross-platform worker. Its built-in capability set is intentionally small:

- `system.info`
- sandboxed `file.read`
- sandboxed `file.write`
- allowlisted HTTPS `http.get`

It does **not** expose shell execution by default. Browser, mobile and other higher-privilege device capabilities should run as dedicated adapters/workers with their own bounded permissions.

#### Controlled evolution

Production code still cannot rewrite or deploy itself just because a model asks it to.

The v1.3 controller adds a durable trust lifecycle:

```text
Proposal
  -> Evaluation
  -> Review Required
  -> Approved
  -> Staged Artifact
  -> Released
  -> Rollback when required
```

Trust gates include:

- evaluation result and evidence
- operator review
- SHA-256 staged artifact identity
- signature verification record
- health-check record
- audited release/rollback decisions

Agent tools can list and propose improvements. Approval, staging, release and rollback remain administrative operations.

#### Open interoperability

- MCP server
- remote MCP bridge
- A2A Agent Card and remote agent bridge
- generic extension-tool interface
- plugin adapters
- channel adapters
- cluster workers

The architecture is protocol-first so third-party integrations do not need unrestricted code execution inside the trusted daemon.

#### Operations and observability

- `/healthz` and `/readyz`
- platform status snapshot
- Prometheus-style platform/runtime counters
- durable audit timeline
- GitHub CI, CodeQL, govulncheck, race tests
- CycloneDX SBOM, provenance and Sigstore-oriented release flow

#### Four command-line programs

- `kingagentd` — core daemon / platform server
- `kingagent` — operator CLI
- `kingworker` — remote cluster worker reference implementation
- `kingconsole` — local zero-dependency web Control Center and restricted API proxy

The Control Center keeps its entered token in page memory only; it does not intentionally persist the token to browser storage.

### Important Scope Boundary

KINGAIBOT v1.3 provides the platform primitives required for broad agent-system coverage: sessions, schedules, workflows, multi-agent missions, skills, plugins, channels, long-term knowledge, device/worker coordination, provider diversity, MCP/A2A, authority-bound WorkGraph orchestration and controlled evolution.

That is different from claiming that every vendor-specific transport or device driver is bundled in the trusted core. For example, Telegram, WhatsApp, Slack, Chromium/CDP, Android and iOS can be connected through adapters, MCP, plugins or capability-scoped workers. Keeping those integrations outside the trust root is intentional and reduces supply-chain and privilege risk.

### Quick Start

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

Review an exact-action approval:

```bash
go run ./cmd/kingagent approvals
go run ./cmd/kingagent approve appr_xxx
```

Start the local Control Center:

```bash
go run ./cmd/kingconsole -api http://127.0.0.1:18888
```

Then open the local console address shown by the process (default port `18889`) and enter an Admin or scoped platform token.

### Remote Worker

1. Register a Worker through the admin cluster API and securely save the returned one-time token.
2. Start the worker:

```bash
export KINGAIBOT_WORKER_TOKEN="kaw_worker_..."
go run ./cmd/kingworker \
  -server http://127.0.0.1:18888 \
  -workspace ./worker-data \
  -allow-host api.github.com
```

For a non-loopback coordinator, `kingworker` requires HTTPS.

### Provider Examples

OpenAI-compatible:

```json
{
  "name": "openai-primary",
  "type": "openai-compatible",
  "base_url": "https://api.openai.com/v1",
  "api_key_env": "OPENAI_API_KEY",
  "model": "gpt-5.6",
  "priority": 10,
  "enabled": true
}
```

Native Anthropic and Gemini templates are included in `configs/config.example.json` but disabled until you provide the desired current model name and API key environment variable.

### Installation Repository

Repository: **kingaiwork/KINGAIBOT**

Linux bootstrap:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

macOS bootstrap:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

Windows PowerShell bootstrap:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'; $env:KINGAGENT_REQUIRE_SIGNATURE='1'; irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

For production, prefer a reviewed immutable signed release/tag rather than installing directly from a moving `main` branch.

### Documentation

- [Documentation Index](docs/README.md)
- [V1.3 Platform](docs/PLATFORM.md)
- [Authority-Bound Orchestration](docs/ORCHESTRATION.md)
- [Clean-Room Originality & IP Policy](docs/ORIGINALITY_IP_POLICY.md)
- [Product Definition](docs/PRODUCT.md)
- [User & Operations Guide](docs/USAGE.md)
- [Long-Term Roadmap](docs/ROADMAP.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](docs/SECURITY.md)
- [Controlled Evolution](docs/EVOLUTION.md)
- [Protocols](docs/PROTOCOLS.md)
- [Deployment](docs/DEPLOYMENT.md)
- [API](docs/API.md)
- [Validation](docs/VALIDATION.md)
- [Residual Risks](docs/RESIDUAL-RISKS.md)

---

# 中文

## 项目定位

**KINGAIBOT 是 kingai.work 面向长期发展的安全智能体执行平台，也是 KING AI 智慧生命体未来的终端执行层。**

它不是一个“套模型接口的聊天机器人”，也不是把 KING AI 改个名字。它负责把上层目标、推理、记忆与治理要求，转化为对工具、工作流、服务、多智能体、设备和远程节点的**持久、可控、可审计执行**。

核心执行原则始终保持：

```text
意图 -> 策略 -> 精确审批 -> 审计 -> 执行 -> 证据
```

系统能力越多，不代表模型自动获得越高权限。

## 当前 v1.3.0 全系统基础

### 1. 安全执行内核

- Linux / macOS / Windows 跨平台 Go Runtime
- Go **1.26.6** 生产与发布基线
- Durable Task 持久任务与重启恢复
- `allow / ask / deny` 权限策略
- 审批精确绑定“任务 + 工具 + 规范化参数哈希”
- 基于 Go `os.Root` 的抗路径穿越文件沙箱
- 安全文件读取、元数据、列表、原子写入、建目录和单项删除
- 核心不向 Agent 暴露递归删除
- HTTPS 白名单、SSRF / DNS Rebinding 防护和重定向降级阻断
- Shell 默认关闭
- SHA-256 前向哈希链审计与周期完整性验证
- 审计异常时危险副作用 Fail-Closed
- 签名、校验、健康检查与回滚式升级链

### 2. 真正的多模型 Provider Fabric

v1.3 不再把 `provider.type` 当成摆设：

- OpenAI-compatible
- Anthropic Messages 原生适配
- Gemini Function Calling 原生适配
- Provider 优先级
- 有界重试
- 自动 fallback
- Circuit Breaker
- 启动时 Provider 类型校验

模型是可替换的推理资源，不是操作系统，也不能决定自己的执行权限。

### 3. Platform Control Plane

新增持久化平台对象：

- Agent Profile
- Session
- Schedule
- Workflow
- Workflow Run
- Parallel Mission
- Device / Browser / Edge Node
- Plugin
- Channel
- Skill

所有 Agent 触发的平台扩展工具，仍然经过同一个 Policy / Approval / Audit 安全边界。

### 4. 身份与 RBAC

- `viewer / operator / automation / admin`
- 细粒度平台权限
- 一次性显示 API Key
- 只持久化 SHA-256 校验值
- Key 过期与吊销
- 兼容原有 Admin Token
- 关键身份/密钥变更无法写入审计时 Fail-Closed

### 5. 入站 Channel Gateway

- 每个 Channel 独立 Token
- Sender 白名单
- Channel → Session 持久映射
- `event_id` 幂等回执
- Webhook 重试去重，避免同一消息重复执行
- 通用出站 Channel Adapter

Telegram、WhatsApp、Slack、Discord、Email 等具体传输层可以放在可信核心之外，通过 Channel / Plugin / MCP / Worker 连接。

### 6. 长期记忆 + 可审核知识图谱

Runtime Memory：

- 有界容量
- 可过期
- Secret Redaction
- 默认不保存原始任务输入
- 检索结果始终当作“不可信历史上下文”

Knowledge Graph：

- Note / Fact / Entity / Relation / Procedure / Preference
- Subject / Predicate / Object
- Source / Confidence / Tags / Expiry
- `proposed -> approved / rejected`
- 只有 approved 数据进入可信搜索
- Agent 可以提议知识，但不能自己批准
- 普通读取 API 与 Admin 审核 API 完全分离

### 7. Capability Envelope + WorkGraph 原创建模与编排

KINGAIBOT 已加入独立于模型的持久执行权限和工作状态层：

- Capability Envelope 可以限制 Capability、数据范围、工具范围、预算、过期时间和委派深度。
- 子授权只能缩小父授权；父授权吊销或过期后，所有后代授权立即失效。
- 平台可信创建的 Task 从 Agent 身份解析 Authority；模型工具参数不能自行选择 `authority_id`。
- WorkGraph 使用 Typed DAG 表达持久工作，支持审批、Replay Policy 和 Evidence Requirement。
- High / Critical 风险节点没有 Evidence 不能完成。
- `execute / delegate` Node 可以通过 Admin-only Orchestration Bridge 派发给 Cluster。
- Cluster 新增 `held` Job：在 WorkGraph Node 没有持久进入 Running 且激活审计没有完成前，任何 Worker 都租不到该 Job。
- Job 提交、Lease 发放、Result 提交都会重新校验 Authority。
- 执行途中撤权时，Worker Result 会保留，但 Job 与 WorkGraph 进入 reconciliation，不能假完成。
- Admin 可以核验并记录已经发生的真实结果，但 `requeue` 仍要求当前 Authority 有效。
- Cluster 完成结果会把 Job ID 与 Result SHA-256 作为 Evidence 回写 WorkGraph。
- 持久 Orchestration Binding 支持重启恢复，并 Fail-Closed 清理无法证明有合法 Binding 的 orphan held Job。

完整状态机与接口见 [`docs/ORCHESTRATION.md`](docs/ORCHESTRATION.md)。

### 8. 多节点 Cluster Worker

- Worker 独立一次性凭据
- 只保存凭据哈希
- Capability Matching
- Durable Job Queue
- Priority
- Lease Token
- Lease Timeout
- Conservative Replay
- Ambiguous Side Effect 默认进入 reconciliation
- 只有显式 replay-safe Job 才能自动重新入队
- 防止重复完成
- Admin 人工 reconciliation

参考 Worker：`kingworker`。

为了不把设备节点变成“远程 Root Shell”，参考 Worker 默认仅提供：

- `system.info`
- 沙箱 `file.read`
- 沙箱 `file.write`
- 白名单 HTTPS `http.get`

不默认开放 Shell。

### 9. 多智能体 Mission

- 多 Agent 并行派发
- 有界 Fan-out
- 每个子任务仍是独立 Durable Task
- 每个子任务仍然独立经过权限与审批
- 聚合完成/部分失败状态

这提供 Swarm 风格协作能力，但不允许递归权限升级。

### 10. 受控进化状态机

生产 Runtime 仍然不能因为模型一句话就修改和部署自己。

完整状态链：

```text
Proposal
  -> Evaluation
  -> Review Required
  -> Approved
  -> Staged
  -> Released
  -> Rolled Back
```

Release 必须记录：

- 评估结果
- 人工审核
- 制品 SHA-256
- 签名验证
- 健康检查
- 审计事件

Agent 只有“查看 / 提议”的工具；Approve / Stage / Release / Rollback 只属于 Admin 控制面。

### 11. MCP / A2A / Plugin / Worker 开放协议

- MCP Server
- Remote MCP Bridge
- A2A Agent Card
- Remote A2A Bridge
- 通用 Extension Tool
- Plugin Adapter
- Channel Adapter
- Cluster Worker

扩展能力不需要把第三方代码直接装进可信 daemon 进程。

### 12. Control Center

新增 `kingconsole`：

- 默认仅监听 `127.0.0.1:18889`
- 零第三方前端依赖
- 管理 Agent / Session / Schedule / Mission / Node / Knowledge / Cluster / Evolution / Identity
- 通过受限同源代理访问 `kingagentd`
- 页面输入 Token 仅保存在当前页面内存，不主动写入浏览器持久存储

### 13. 四个正式程序

- `kingagentd`：核心 daemon + 平台 API
- `kingagent`：命令行操作端
- `kingworker`：远程节点执行端
- `kingconsole`：本地 Web 控制台

## 关于“覆盖 OpenClaw 类平台功能”的准确边界

v1.3 已经建立会话、自动化、工作流、Skill、Plugin、Channel、多智能体、设备/Worker、长期知识、Provider Fabric、MCP/A2A、Capability Envelope、WorkGraph、Authority-bound Orchestration、控制台和受控进化等**平台级基础能力**。

但“平台具备适配能力”不等于“把全球所有第三方厂商驱动都硬编码进核心”。例如 Telegram / WhatsApp / Slack / Chromium/CDP / Android / iOS 的具体实现应继续通过受限 Adapter、Plugin、MCP 或 Worker 扩展。这样做不是少功能，而是避免第三方 SDK 和高权限设备控制直接进入 KINGAIBOT 信任根。

另外，当前 Cluster 是**单协调器 + 多 Worker**的 Durable Worker 基线；真正多控制器 HA、分布式数据库、共识/租约后端仍属于后续企业级扩展，不应把它冒充成已经完成的多主高可用。

## 快速启动

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

运行任务：

```bash
export KINGAGENT_ADMIN_TOKEN="...与服务端相同..."
go run ./cmd/kingagent run "在 workspace 创建 hello.txt，写一句问候"
```

打开控制台：

```bash
go run ./cmd/kingconsole -api http://127.0.0.1:18888
```

默认控制台监听本机 `18889` 端口。

## Worker 节点

管理员先通过 Cluster API 注册 Worker，并安全保存只显示一次的 Worker Token，然后运行：

```bash
export KINGAIBOT_WORKER_TOKEN="kaw_worker_..."
go run ./cmd/kingworker \
  -server http://127.0.0.1:18888 \
  -workspace ./worker-data \
  -allow-host api.github.com
```

非 loopback Coordinator 强制要求 HTTPS。

## 官方信息

官网：**https://kingai.work**  
邮箱：**vip@kingai.work**  
GitHub：**kingaiwork/KINGAIBOT**

> KINGAIBOT 是长期工程。仓库中的版本代表当前经过实现、测试和安全验证的阶段能力，不代表“永远终极”或“绝对零 Bug”。高权限能力始终应遵循最小权限、可审计、可回滚和人工接管原则。
