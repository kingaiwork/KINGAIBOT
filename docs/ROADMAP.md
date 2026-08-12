# KINGAIBOT Long-Term Roadmap / 长期开发路线图

## English

KINGAIBOT is designed as a multi-year platform, not a single release. Development is capability-gated: new autonomy is introduced only after security, observability and rollback controls are mature enough.

### Phase 1 — Hardened execution core
Durable tasks, policy, approvals, audit integrity, safe networking, model abstraction, MCP/A2A baseline and signed upgrade path.

### Phase 2 — Plugin isolation
WASM/WASI component runtime, capability-scoped plugins, signed plugin manifests, resource budgets and deterministic permission boundaries.

### Phase 3 — Enterprise identity and secrets
OIDC, SSO, RBAC/ABAC, KMS/HSM integration, tenant isolation, scoped service identities and short-lived credentials.

### Phase 4 — Distributed durable runtime
Multiple nodes, leases, task ownership, durable queues, replay-safe workflows, recovery coordination and optional edge runtimes.

### Phase 5 — Advanced long-term memory
Hybrid semantic + graph memory, provenance, confidence, retention policy, user/org boundaries, memory lifecycle and poisoning defenses.

### Phase 6 — Multi-agent coordination
Agent roles, capability discovery, delegation policies, cost/risk routing, A2A streaming/push evolution and bounded recursive coordination.

### Phase 7 — MCP platform
MCP client/server extensions, resource subscriptions, richer auth, tool catalogs, enterprise connectors and policy-aware tool federation.

### Phase 8 — Deep observability
OpenTelemetry traces, metrics, structured events, cost/token accounting, SIEM export, forensic timelines and reliability SLOs.

### Phase 9 — Self-healing and controlled evolution
Failure clustering, automated diagnosis, regression generation, sandbox repair proposals, shadow tests, canary activation and automatic rollback.

### Phase 10 — Device and edge execution
Desktop, server, mobile-control and edge runtimes with scoped local capabilities and hardware-aware execution.

### Phase 11 — Supply-chain maturity
Reproducible builds, SBOM policy, provenance verification, signed plugins, dependency admission gates and independent security testing.

### Phase 12 — KING AI integration
Introduce a controlled contract between KING AI and KINGAIBOT so the main intelligent-lifeform system can dispatch missions to the execution layer without surrendering governance or safety boundaries.

---

## 中文

KINGAIBOT 是多年持续研发的平台，而不是一次发布就结束的产品。路线采用“能力闸门”原则：只有安全、可观测和回滚机制成熟后，才扩大自治权限。

### 第一阶段：加固执行核心
Durable Task、权限策略、审批、审计完整性、安全网络、多模型抽象、MCP/A2A 基线、签名升级。

### 第二阶段：插件强隔离
WASM/WASI Component Runtime、能力级插件授权、插件签名、资源预算和确定性权限边界。

### 第三阶段：企业身份和密钥
OIDC、SSO、RBAC/ABAC、KMS/HSM、多租户隔离、服务身份和短期凭据。

### 第四阶段：分布式 Durable Runtime
多节点、Lease、任务归属、Durable Queue、可安全重放工作流、故障协调和边缘节点。

### 第五阶段：高级长期记忆
语义 + 图谱混合记忆、来源、可信度、保留策略、用户/企业边界、生命周期和记忆投毒防护。

### 第六阶段：多智能体协作
角色、能力发现、委派策略、成本/风险路由、A2A Streaming/Push 和受限递归协作。

### 第七阶段：MCP 平台化
MCP Client/Server 扩展、资源订阅、更完善认证、工具目录、企业连接器和策略化工具联邦。

### 第八阶段：深度可观测性
OpenTelemetry Trace/Metrics/Event、Token 与成本、SIEM、取证时间线和可靠性 SLO。

### 第九阶段：自我修复和受控进化
故障聚类、自动诊断、回归测试生成、沙箱修复提案、Shadow Test、Canary 和自动回滚。

### 第十阶段：设备和边缘执行
桌面、服务器、移动控制端和边缘 Runtime，按设备能力与硬件约束安全执行。

### 第十一阶段：软件供应链成熟
可复现构建、SBOM 策略、Provenance 验证、插件签名、依赖准入和第三方独立安全测试。

### 第十二阶段：与 KING AI 主系统整合
建立 KING AI ↔ KINGAIBOT 的受控契约，使主智慧生命体可以向终端执行层下发 Mission，同时不牺牲治理、安全和人工接管边界。
