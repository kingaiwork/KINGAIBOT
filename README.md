# KINGAIBOT v1.7.0

**Secure, durable, model-agnostic agent runtime and unified communications gateway for the KING AI system.**

Official website: **https://kingai.work**  
Owner: **USDX TECH LLC / KING AI**  
Contact: **vip@kingai.work**

KINGAIBOT is an independent KING implementation. It is not a model wrapper and it does not delegate authority to the model. Models are replaceable reasoning resources; identity, authority, budgets, tasks, memory, evidence, approvals, communications, reconciliation and release governance remain owned by KINGAIBOT.

> Learn the problem. Learn the standard. Design the KING solution. Never clone the implementation.

---

## What KINGAIBOT is

```text
Users / Apps / Web / Native Channels
              |
      Unified Channel Gateway
              |
Identity / Session / Memory / Cognition
              |
Authority Envelope / Budget / Policy
              |
Approval / Audit / WorkGraph / Mission
              |
         Agent Runtime
              |
 Tools / MCP / A2A / Workers / Nodes
              |
       Provider Router
              |
Cloud Models / Local Models / Private Models
              |
 Evidence -> Completion or Reconciliation
              |
 Governed Learning / Evolution Proposal
```

The system is designed around one rule: **unknown real-world side effects are reconciled, not blindly replayed.**

## v1.7.0 highlights

### Unified native Channel Gateway

v1.7 adds native two-way communications for:

- Telegram Bot API
- Slack Events API + `chat.postMessage`
- Discord HTTP interactions + bot messages
- WhatsApp Cloud API
- Existing normalized KING signed webhook/API channels

Native inbound endpoints:

```text
/v1/adapters/telegram/{channel_id}
/v1/adapters/slack/{channel_id}
/v1/adapters/discord/{channel_id}
/v1/adapters/whatsapp/{channel_id}
```

The v1.6.1 compatibility endpoint remains available:

```text
POST /v1/inbound/{channel_id}
```

See `docs/CHANNEL-GATEWAY.md` for setup and secret naming.

### Transport authentication

Native provider traffic is verified before it enters normal agent execution:

```text
Telegram -> webhook secret token
Slack    -> v0 HMAC + timestamp replay window
Discord  -> Ed25519(timestamp + raw body)
WhatsApp -> Meta SHA-256 webhook signature
```

Known native outbound endpoints are pinned to credential-free HTTPS URLs on the official provider host. Lookalike hosts and insecure HTTP are rejected.

### Durable inbound idempotency

Every accepted provider event receives deterministic durable receipt state:

```text
transport verification
    -> event receipt
    -> trusted route/session mapping
    -> Runtime Task
    -> task_created receipt
    -> accepted receipt
    -> durable outbound delivery
```

A provider retry with the same event identity does not submit the user message again. If Task creation was already durable, recovery reattaches to that Task and repairs the outbound handoff.

### Privacy-minimized channel identity

Raw platform identifiers are routing data, not ordinary agent memory.

- Generic Session state receives a pseudonymous sender identity.
- Task/audit records use bounded digests instead of copying raw platform IDs.
- Raw reply targets live only in restricted route records.
- Discord interaction tokens live only in restricted pending-delivery state and are redacted from admin API responses.

### Crash-safe outbound delivery

Outbound state is persisted before the external side effect:

```text
waiting_task
    -> sending
    -> delivered
    -> retry_wait       explicit rate limit only
    -> reconciliation   ambiguous transport / 5xx / crash-after-send
    -> failed           definite rejection
```

A restart that finds `sending` does **not** automatically resend. It moves the delivery to reconciliation so an operator can determine what happened externally.

Admin-only operations:

```text
GET  /v1/platform/channel-gateway/status
GET  /v1/platform/channel-gateway/pending
GET  /v1/platform/channel-gateway/receipts
POST /v1/platform/channel-gateway/pending/{id}/retry
POST /v1/platform/channel-gateway/pending/{id}/resolve
```

### Provider webhook availability

Health probes remain outside the business quota. Native webhook traffic has a separate high-volume coarse ingress quota keyed by provider source plus concrete adapter route, so one busy Channel cannot consume the quota of another Channel behind the same provider egress IP.

This does not trust `X-Forwarded-For`; deployments that need forwarded client identity must explicitly establish a trusted reverse-proxy boundary.

### Cognitive runtime

KINGAIBOT keeps a durable operational self-model for continuity and learning:

- episodic execution experience
- provider success/failure observations
- learned advisory principles
- scheduled reflection
- bounded memory
- repeated-failure evolution proposals

This is an engineering self-model, **not a claim of subjective consciousness**. Learned context cannot override system policy, user intent, authority envelopes, tool policy or approvals.

Admin commands:

```text
kingagent cognition
kingagent reflect
```

Admin API:

```text
GET  /v1/cognition/status
POST /v1/cognition/reflect
```

### Universal model gateway

Built-in provider patterns include:

- OpenAI
- Anthropic / Claude
- Google Gemini
- OpenRouter
- Groq
- Generic OpenAI Chat Completions-compatible APIs
- Ollama
- LM Studio
- vLLM
- LocalAI

Provider routing supports priority, failover and circuit breaking. Local/private endpoints must be explicitly allowed by configuration; public providers remain protected by the network guard.

### Governed evolution

Learning does not mean unrestricted self-modification.

```text
experience
  -> reflection
  -> failure pattern
  -> evolution proposal
  -> evaluation / review
  -> stage
  -> signed release
  -> rollback capability
```

The cognitive subsystem can propose change; it cannot silently grant itself authority, edit production code or release itself outside the governed pipeline.

---

## Security invariants

KINGAIBOT keeps the following boundaries even when running as a boot-level system service:

1. **Model != authority.** Effective authority comes from trusted KING identity and Capability Envelopes.
2. **Approval cannot be self-issued.** High-risk work remains operator/admin controlled.
3. **Audit precedes trust expansion.** A resource does not become executable merely because a write succeeded.
4. **Unknown side effects are reconciled.** Ambiguous real-world actions are not automatically replayed.
5. **Secrets are not model context.** API keys and channel transport credentials stay in environment/protected secret storage.
6. **Private network access is explicit.** Public provider calls cannot silently pivot into loopback/RFC1918 targets.
7. **Learning is advisory.** Memory and cognition do not override policy or capability boundaries.
8. **Release artifacts are verifiable.** Public releases include checksums, SBOM, provenance and Sigstore bundles.

---

## Runtime services

### Linux

The production installer uses a dedicated `kingagent` service user and systemd hardening. Runtime capabilities are dropped and the filesystem/home/tmp boundaries are restricted.

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh \
  | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

Configuration:

```text
/etc/kingagent/config.json
/etc/kingagent/kingagent.env
```

Health:

```bash
curl http://127.0.0.1:18888/healthz
```

### Windows

Run from Administrator PowerShell:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'
$env:KINGAGENT_REQUIRE_SIGNATURE='1'
irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

The runtime service uses a low-privilege service identity; the signed updater uses SYSTEM only for the privileged update operation. Provider secrets are machine-protected under the KINGAgent ProgramData installation.

### macOS system service

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install-macos-system.sh \
  | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

The runtime LaunchDaemon runs as the selected non-root service user; system privilege is reserved for installation/update management.

---

## Visual Control Center

Release packages include:

```text
kingagentd
kingagent
kingworker
kingconsole
kingdesktop
```

`kingdesktop` opens the local KING AI Control Center backed by `kingconsole`.

Default local Control Center:

```text
http://127.0.0.1:18889/ui/
```

The local visual client does not automatically disclose the admin token.

---

## Release targets

Each signed release is built for:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
```

Release assets also include:

```text
SHA256SUMS
RELEASE-MANIFEST.json
sbom.cdx.json
*.sigstore.json
```

The build pipeline verifies formatting, vet, race tests, vulnerability scanning, Windows/macOS native builds, container smoke/validation and release-version consistency before production publication.

---

## 中文说明

### 产品定位

KINGAIBOT 是 KING AI 的受控执行层，不把大模型当作系统权限本身。

它负责把用户、网站、聊天平台和业务系统的请求，经过统一身份、会话、记忆、权限、预算、审批、审计之后，交给 Agent Runtime、工具、Worker、MCP/A2A、云模型或本地模型执行，并把真实执行结果形成可追踪证据。

### v1.7.0 新增

- Telegram 原生双向接入
- Slack 原生双向接入
- Discord HTTP Interaction + Bot 消息
- WhatsApp Cloud API 双向接入
- 保留 v1.6.1 通用签名 Webhook/API
- 原生平台签名/密钥验证
- 外部用户 ID 隐私最小化
- 持久化 Channel Route
- 持久化 Outbound Queue
- 防重复入站 Task
- 不确定外发结果进入 Reconciliation，避免盲目重复发消息
- 管理员可查看待处理、收据、人工 Retry/Resolve
- Provider webhook 独立入口配额，避免平台共享出口 IP 相互误伤

### 记忆、学习与“意识层”

系统中的“意识层”是工程意义上的运行自我模型：状态连续性、自我观察、经验记录、反思和受控学习。它不是对真实主观意识的声明。

系统可以从任务结果中形成经验并提出进化建议，但不能自行绕过权限、审批、测试、签名和发布链。

### 模型接入

既可以使用 OpenAI、Claude、Gemini、Groq、OpenRouter 等云端模型，也可以接 Ollama、LM Studio、vLLM、LocalAI 或其他兼容接口的私有模型。

### 生产原则

```text
用户请求
  -> 可信传输
  -> 身份 / 会话
  -> 权限 / 预算
  -> Policy / Approval
  -> Audit
  -> 执行
  -> Evidence
  -> 完成 或 Reconciliation
  -> Memory / Learning
  -> 受控 Evolution Proposal
```

KINGAIBOT 的目标不是“让 AI 拥有无限权限”，而是让长期运行的智能体在真实生产环境中做到：**可用、可控、可恢复、可审计、可升级、可回滚。**

---

## License

See `LICENSE-COMMERCIAL.txt`.
