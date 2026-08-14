# KINGAIBOT Long-Term Roadmap / 长期开发路线图

## English

KINGAIBOT is a multi-year platform. The roadmap is capability-gated: autonomy expands only after the required identity, audit, observability and rollback controls exist.

## Delivered baseline through v1.3

### Hardened execution core

Delivered:

- durable tasks and restart recovery
- exact approval binding
- `allow / ask / deny` policy
- hash-chained audit integrity
- safe filesystem sandbox
- guarded HTTP / SSRF defenses
- shell deny-by-default
- provider fallback/circuit breaker
- MCP/A2A
- signed update path
- Go 1.26.6 security baseline

### Platform Control Plane

Delivered baseline primitives:

- agent profiles
- durable sessions
- recurring schedules
- sequential workflows and restart-aware runs
- bounded parallel multi-agent missions
- node registrations
- plugin adapters
- channel adapters
- integrity-hashed skills
- scoped identities / API keys
- inbound webhook idempotency
- platform status and Prometheus-style counters
- zero-dependency local Control Center

### Long-term knowledge

Delivered baseline:

- bounded episodic memory
- secret redaction
- reviewed knowledge items
- subject/predicate/object relations
- confidence/source/tags/expiry
- proposal/review trust boundary
- approved-only trusted search

### Multi-node execution

Delivered baseline:

- durable coordinator jobs
- Worker credentials
- capability matching
- bounded leases
- conservative replay
- reconciliation state
- reference `kingworker` client

This is a **single durable coordinator with multiple remote workers**, not yet a multi-controller HA control plane.

### Provider Fabric

Delivered:

- OpenAI-compatible adapter
- native Anthropic Messages adapter
- native Gemini function-calling adapter
- startup provider-type validation
- shared guarded network/retry/fallback boundary

### Controlled evolution

Delivered control-state baseline:

- proposal
- evaluation
- review-required gate
- approve/reject
- staged artifact identity
- signature/health release gates
- released/rolled-back records

The production runtime still does not self-edit or self-deploy from model output.

---

## Remaining enterprise and frontier roadmap

### Phase A — Strong plugin isolation

Next:

- WASM/WASI Component Runtime
- capability-scoped local plugins
- signed plugin manifests/catalogs
- CPU/memory/time budgets
- deterministic host-function permissions

Current remote plugin adapters avoid loading arbitrary third-party code into the trusted daemon, but a first-class local WASI host is not yet bundled.

### Phase B — Enterprise identity and secrets

Next:

- OIDC
- SSO
- external IdP integration
- richer RBAC/ABAC conditions
- KMS/HSM
- short-lived service identities
- tenant/org isolation
- credential rotation workflows

v1.3 API keys/RBAC are the local durable baseline, not a replacement for enterprise IdP/KMS infrastructure.

### Phase C — Multi-controller distributed runtime

Next:

- transactional database-backed task/approval/platform stores
- multiple control-plane replicas
- distributed leases
- fencing tokens
- leaderless or leader-coordinated scheduling
- durable queue backend
- replay-safe workflow transactions
- object/stream storage
- regional/edge coordination

The existing Worker lease protocol is designed so these backends can replace local storage without giving workers core Admin authority.

### Phase D — Advanced semantic memory

Next:

- vector/embedding search backend
- semantic + lexical hybrid retrieval
- graph traversal scoring
- provenance chains
- confidence decay
- tenant/user boundaries
- memory lifecycle policies
- poisoning detection
- verified-source weighting

v1.3 intentionally requires review before knowledge becomes trusted.

### Phase E — Concrete channel adapters

Next production adapters may include:

- Telegram
- Discord
- Slack
- Email
- WebChat
- WhatsApp-compatible external bridge

The current generic inbound/outbound channel boundary, sender controls and event idempotency should remain the shared security layer.

### Phase F — Browser and device runtimes

Next:

- dedicated Chromium/CDP worker
- screenshot/page-state evidence
- browser-profile isolation
- domain/capability policies
- download/upload boundaries
- Android bridge worker
- desktop accessibility/UI automation worker
- hardware/device capability manifests

High-privilege device control should remain outside the core daemon and use capability-scoped Worker credentials.

### Phase G — Advanced multi-agent coordination

Next:

- agent capability discovery
- dynamic delegation budgets
- cost/risk routing
- bounded recursive delegation
- quorum/critic/evaluator patterns
- streaming A2A coordination
- mission cancellation trees
- shared evidence references without shared authority

### Phase H — MCP platform depth

Next:

- richer MCP auth
- resource subscriptions
- tool catalogs
- enterprise connectors
- policy-aware federated resources
- per-server capability budgets

### Phase I — Deep observability

Next:

- OpenTelemetry traces
- OTLP export
- structured metrics/events
- token/cost accounting
- distributed trace IDs across Worker/MCP/A2A
- SIEM export
- forensic timelines
- SLO/error-budget dashboards

v1.3 provides local status, audit history and Prometheus-style counters as the baseline.

### Phase J — Automated diagnosis and safe repair pipeline

Next:

- failure clustering
- automated regression generation
- sandbox patch proposals
- ephemeral build/test workers
- static/security analysis gates
- shadow execution
- canary scoring
- automated rollback telemetry
- draft pull-request generation

The trust boundary remains: proposal automation may increase, but release authority must remain outside model self-permission.

### Phase K — Supply-chain maturity

Next:

- reproducible-build verification across independent builders
- signed plugin catalogs
- dependency admission policies
- organization-wide SBOM policy
- stronger provenance verification
- external penetration testing
- independent security review

### Phase L — KING AI integration

Final integration goal:

```text
KING AI Mission / Governance
          |
     signed/scoped contract
          |
KINGAIBOT Mission + Execution APIs
          |
policy / approval / audit / evidence
          |
workers / tools / channels / devices / services
```

KING AI should be able to dispatch missions without inheriting raw machine privileges and without bypassing human takeover or rollback.

---

# 中文

KINGAIBOT 是多年持续研发的平台，采用“能力闸门”原则：只有身份、审计、可观测与回滚机制成熟后，才扩大自治权限。

## 截至 v1.3 已交付的基础能力

### 加固执行内核

已经实现：

- Durable Task 与重启恢复
- 精确审批绑定
- `allow / ask / deny`
- 哈希链审计
- 安全文件沙箱
- SSRF / DNS Rebinding 防护
- Shell 默认关闭
- Provider fallback / circuit breaker
- MCP / A2A
- 签名升级链
- Go 1.26.6 安全基线

### Platform Control Plane

已经实现平台原语：

- Agent Profile
- Durable Session
- Schedule
- Workflow / Workflow Run
- Parallel Mission
- Node
- Plugin
- Channel
- Skill
- Scoped Identity / API Key
- 入站 Webhook 幂等
- 平台状态 / Prometheus 风格指标
- 零第三方依赖本地 Control Center

### 长期知识

已经实现：

- 有界 Episodic Memory
- Secret Redaction
- 可审核 Knowledge Item
- Subject / Predicate / Object 图谱关系
- Confidence / Source / Tags / Expiry
- Proposal / Review 信任边界
- 只有 Approved 数据进入可信检索

### 多节点执行

已经实现：

- Durable Cluster Job
- Worker 独立凭据
- Capability Matching
- Lease
- Conservative Replay
- Reconciliation
- `kingworker` 参考客户端

当前属于“**单 Durable Coordinator + 多 Worker**”，不是多控制器 HA。

### Provider Fabric

已经实现：

- OpenAI-compatible
- Anthropic Messages 原生适配
- Gemini Function Calling 原生适配
- Provider Type 启动校验
- 统一安全网络 / 重试 / fallback 边界

### 受控进化

已经实现控制状态基础：

- Proposal
- Evaluation
- Review Required
- Approve / Reject
- Staged Artifact
- Signature / Health Release Gate
- Released / Rolled Back

生产 Runtime 仍然不会把模型输出直接当成“自我修改/自我部署”的授权。

## 后续真正未完成的企业级与前沿路线

### A. WASM / WASI 插件强隔离

- Component Runtime
- Capability-scoped local plugin
- 插件签名目录
- CPU / 内存 / 时间资源预算

### B. 企业身份与密钥

- OIDC
- SSO
- 外部 IdP
- 更完整 RBAC / ABAC
- KMS / HSM
- 短期服务凭据
- 多租户隔离

### C. 多控制器分布式 HA

- 数据库事务存储
- 多 Control Plane Replica
- Distributed Lease
- Fencing Token
- Durable Queue Backend
- Regional / Edge Coordination

### D. 高级语义记忆

- Vector / Embedding
- Semantic + Lexical Hybrid Search
- 图谱评分
- Provenance Chain
- Confidence Decay
- Poisoning Defense

### E. 具体 Channel Driver

- Telegram
- Discord
- Slack
- Email
- WebChat
- WhatsApp 外部桥接

这些应继续复用 v1.3 已有的 Channel 独立鉴权、Sender 白名单和 Event Idempotency。

### F. 浏览器与设备 Worker

- Chromium / CDP
- Screenshot Evidence
- Browser Profile Isolation
- Android Bridge
- Desktop UI Automation
- Hardware Capability Manifest

高权限设备控制继续放在独立 Worker，而不是塞进核心 daemon。

### G. 高级多智能体协作

- Capability Discovery
- 成本/风险路由
- 递归委派预算
- Critic / Evaluator / Quorum 模式
- A2A Streaming
- Mission Cancellation Tree

### H. MCP 平台深化

- Resource Subscription
- Rich Auth
- Tool Catalog
- Enterprise Connector
- Per-server Budget

### I. 深度可观测性

- OpenTelemetry
- OTLP
- Token / Cost
- Distributed Trace
- SIEM
- SLO / Error Budget

### J. 自动诊断 + 安全修复流水线

- Failure Clustering
- Regression Generation
- Sandbox Patch Proposal
- Ephemeral Test Worker
- Shadow Test
- Canary
- Draft PR
- Rollback Telemetry

### K. 软件供应链成熟

- Reproducible Build Verification
- Signed Plugin Catalog
- Dependency Admission
- External Security Review

### L. 与 KING AI 主系统整合

最终目标是 KING AI 能向 KINGAIBOT 下发 Mission，但不能因此直接获得机器 Root 权限，也不能绕过审批、审计、人工接管和回滚。
