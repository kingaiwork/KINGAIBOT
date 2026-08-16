# KINGAIBOT v1.8.0

**Secure, durable, model-agnostic system Agent Runtime with governed cloud fleet, memory continuity and native communications.**

Official website: **https://kingai.work**  
Owner: **USDX TECH LLC / KING AI**  
Contact: **vip@kingai.work**

KINGAIBOT is the controlled execution layer of the KING AI system. It is not a model wrapper and it never treats a model response as system authority. Models are replaceable reasoning resources; durable identity, authority, budgets, tasks, memory, approvals, evidence, communications, reconciliation and release governance remain owned by KINGAIBOT.

> Learn the problem. Learn the standard. Design the KING solution. Never clone the implementation.

---

## System architecture

```text
Users / Apps / Telegram / Slack / Discord / WhatsApp / Webhook
                              |
                    Unified Channel Gateway
                              |
                 Identity / Session / Route
                              |
                     Memory / Cognition
                              |
KING AI Cloud ----> restriction-only policy
     |                        |
Fleet health                  v
E2EE ciphertext      Local Authority Envelope
     |                        |
     +--------------> Budget / Policy / Approval
                              |
                            Audit
                              |
                       Agent Runtime
                              |
            Tools / MCP / A2A / Worker / Node
                              |
                       Provider Router
                              |
        Cloud Models / Local Models / Private Models
                              |
                Evidence -> Complete / Reconcile
                              |
                 Memory / Reflection / Learning
                              |
                    Evolution Proposal
```

Two invariants define the system:

1. **Model != authority.**
2. **Unknown real-world side effects are reconciled, not blindly replayed.**

v1.8 adds a third:

3. **Cloud policy can contract local authority, never expand it.**

---

## v1.8.0 — Cloud Identity & Fleet

### Ed25519 device identity

Each cloud-managed KINGAIBOT node owns its own Ed25519 private key. The private key is generated locally and remains in the protected Runtime data directory. The cloud receives only public SPKI material.

Enrollment reuses the existing KINGAIASE OPS identity plane:

```text
one-time enrollment token
        -> local Ed25519 key
        -> signed KINGAI-OPS-ENROLL-V2 request
        -> organization / workspace / node identity
        -> token consumed
        -> future requests signed by device key
```

The raw one-time token is never written into KINGAIBOT durable state. The supplied cloud configurators remove it from the service environment after successful enrollment.

### Restriction-only policy

Cloud policy may:

- disable an already-configured Provider;
- disable an already-configured Channel;
- lower Runtime step/worker/timeout ceilings;
- tighten `allow -> ask -> deny` tool policy;
- disable encrypted continuity sync.

Cloud policy cannot:

- turn `deny` into `ask` or `allow`;
- create a Capability Envelope;
- create/enable a Provider or Channel;
- approve high-risk work;
- bypass local audit or reconciliation;
- obtain local model/provider credentials.

Channel contraction is safe to apply live and is audit-first. Provider, Runtime-ceiling and Tool-Policy changes affect components constructed at process start, so v1.8 marks them explicitly as `policy_restart_required` instead of falsely claiming they were hot-applied.

### Local-first availability

By default, KINGAIBOT remains operational under the last locally constructed security boundary if KING AI Cloud is unavailable.

Set:

```text
KINGAI_CLOUD_REQUIRE_POLICY=1
```

only for deployments that intentionally require cloud-policy availability during startup.

### AES-256-GCM encrypted continuity

Memory continuity is opt-in:

```text
KINGAI_MEMORY_SYNC=1
KINGAI_SYNC_KEY=<base64 of exactly 32 random bytes>
```

Encryption happens on the device before upload. The cloud does **not** receive the sync key or plaintext memory.

Authenticated encryption binds:

```text
workspace + stream + key_id
```

The cloud persists only:

```text
node_id
stream
monotonic sequence
key_id
nonce
ciphertext
ciphertext SHA-256
timestamps / tenant scope
```

Only the newest five encrypted envelopes per node/stream are retained. Peer Cognition snapshots can be preserved for recovery evidence, but one node's operational self-model is never silently merged into another node. Imported peer Memory is sanitized, deduplicated and confidence-capped.

### Crash-recoverable device-key rotation

v1.8 uses a two-phase Ed25519 key rotation rather than replacing the key in one unsafe step:

```text
generate replacement key locally
        -> persist pending private key (0600)
        -> PREPARE signed by old key + new key
        -> persist rotation ID / new key ID
        -> COMMIT signed by new key
        -> atomically replace active local key
        -> clear pending state
```

If the network drops after the server may have committed, KINGAIBOT keeps the pending key and rotation marker. The next retry or restart repeats the same COMMIT idempotently and completes the local transition. It does not generate a second identity.

---

## Cloud & Fleet Control Center

The visual client now includes:

```text
http://127.0.0.1:18889/ui/cloud/
```

It shows:

- enrollment / Local-First state;
- Node ID, Workspace and current Key ID;
- last heartbeat;
- current policy version;
- restart-required policy state;
- last E2EE sync;
- last key rotation;
- pending key-rotation reconciliation;
- latest cloud error.

Admin actions:

```text
Pull Cloud Policy
Sync Encrypted Continuity
Rotate Device Key
```

The page does not persist or reveal the local Admin Token.

Local admin API:

```text
GET  /v1/cloud/status
POST /v1/cloud/policy/pull
POST /v1/cloud/sync
POST /v1/cloud/key/rotate
```

See `docs/CLOUD-FLEET.md`.

---

## Native communications

The v1.7 unified Channel Gateway remains fully available in v1.8:

- Telegram Bot API
- Slack Events API + `chat.postMessage`
- Discord HTTP interactions + bot messages
- WhatsApp Cloud API
- normalized KING signed webhook/API

Native ingress:

```text
/v1/adapters/telegram/{channel_id}
/v1/adapters/slack/{channel_id}
/v1/adapters/discord/{channel_id}
/v1/adapters/whatsapp/{channel_id}
/v1/inbound/{channel_id}
```

Transport verification:

```text
Telegram -> webhook secret token
Slack    -> v0 HMAC + replay window
Discord  -> Ed25519(timestamp + raw body)
WhatsApp -> Meta SHA-256 webhook signature
```

Outbound external side effects use durable delivery state. Explicit rate limits can retry; ambiguous transport/5xx/crash-after-send enters reconciliation instead of blind replay.

See `docs/CHANNEL-GATEWAY.md`.

---

## Memory, cognition and governed evolution

KINGAIBOT keeps an engineering operational self-model for continuity and learning:

- episodic execution experience;
- provider success/failure observations;
- bounded long-term memory;
- learned advisory principles;
- scheduled reflection;
- repeated-failure evolution proposals.

This is **not a claim of subjective consciousness**. Cognition and learned context cannot override user intent, authority envelopes, tool policy, approvals or audit.

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

Admin commands:

```text
kingagent cognition
kingagent reflect
```

See `docs/COGNITIVE-RUNTIME.md`.

---

## Universal model gateway

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

Provider routing supports priority, failover and circuit breaking. Local/private endpoints require explicit network permission.

---

## Security invariants

1. **Model != authority.** Effective authority comes from trusted identity and Capability Envelopes.
2. **Cloud != authority expansion.** Remote policy is restriction-only.
3. **Approval cannot be self-issued.** High-risk work remains operator/admin controlled.
4. **Audit precedes trust expansion.** A resource does not become executable just because persistence succeeded.
5. **Ambiguous effects reconcile.** Unknown real-world side effects are not automatically replayed.
6. **Secrets are not model context.** Provider, channel, device and sync credentials remain protected data.
7. **Private-network access is explicit.** Public provider/cloud paths cannot silently pivot to loopback/RFC1918 destinations.
8. **Learning is advisory.** Memory/cognition cannot override authorization.
9. **Device rotation is recoverable.** Uncertain rotation commits keep durable pending state.
10. **Release artifacts are verifiable.** Releases include checksums, SBOM, provenance and Sigstore bundles.

---

## Install

### Linux system service

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh \
  | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

Runtime user: dedicated low-privilege `kingagent` system account.

Configuration:

```text
/etc/kingagent/config.json
/etc/kingagent/kingagent.env
```

### Windows

Administrator PowerShell:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'
$env:KINGAGENT_REQUIRE_SIGNATURE='1'
irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

Runtime identity is `NT AUTHORITY\LOCAL SERVICE`, not Administrator/SYSTEM. The signed updater alone uses SYSTEM.

### macOS boot-level service

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install-macos-system.sh \
  | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

The LaunchDaemon runs as the selected non-root service user.

---

## Enroll a device into KING AI Cloud

Create a one-time `kop_enroll_...` token from an authorized KING AI OPS organization/workspace, then run the platform configurator.

Linux / macOS system service:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/configure-cloud.sh | sudo bash
```

Windows Administrator PowerShell:

```powershell
irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/configure-cloud.ps1 | iex
```

The configurator prompts for the one-time token without echoing it, restarts the existing low-privilege Runtime, verifies durable enrollment and then clears the raw token from service configuration.

For encrypted continuity, set `KINGAI_MEMORY_SYNC=1` before running the configurator. If no sync key is supplied, the configurator generates a random 32-byte key locally. Provision that same key only to devices that are intentionally allowed to share continuity.

---

## Runtime and visual client

Release packages include:

```text
kingagentd
kingagent
kingworker
kingconsole
kingdesktop
configure-cloud.sh / configure-cloud.ps1
cloud.env.example
```

Control Center:

```text
http://127.0.0.1:18889/ui/
```

Cloud & Fleet:

```text
http://127.0.0.1:18889/ui/cloud/
```

Health:

```text
http://127.0.0.1:18888/healthz
```

---

## Release targets

Every formal signed release builds:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
```

Release assets include:

```text
SHA256SUMS
RELEASE-MANIFEST.json
sbom.cdx.json
*.sigstore.json
```

The release gate validates formatting, vet, race tests, vulnerability scanning, native Windows/macOS builds, containers, version consistency, SBOM, provenance and GitHub OIDC/Sigstore signing before publication.

---

## 中文定位

KINGAIBOT 1.8 已形成一条完整的长期运行链路：

```text
设备身份
  -> 云端 Fleet
  -> restriction-only 策略
  -> 本地 Authority / Approval / Audit
  -> Agent Runtime
  -> 云模型 / 本地模型 / Tools / Worker
  -> Evidence / Reconciliation
  -> Memory / Cognition / Reflection
  -> 端到端加密连续性
  -> Governed Evolution
```

“云管理”不等于“云端接管本机”。云端负责身份、Fleet 健康、策略收紧和密文连续性；真正执行权限仍由每台机器自己的 Authority、Policy、Approval 和 Audit 决定。

系统中的“意识层”仍然是工程意义上的运行自我模型——状态连续性、自我观察、经验、反思和受控学习——不是对真实主观意识的声明。

目标仍然是：**可用、可控、可恢复、可审计、可升级、可回滚。**

---

## License

See `LICENSE-COMMERCIAL.txt`.
