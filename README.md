# KINGAIBOT

**A future-facing, secure, durable and extensible intelligent-agent execution system from [kingai.work](https://kingai.work).**

Official website: **https://kingai.work**  
Contact: **vip@kingai.work**

> KINGAIBOT is a long-term R&D project of kingai.work. It is being developed as the future **terminal execution layer of the existing KING AI intelligent-lifeform system**. It is independently developed and validated today, with controlled integration into the KING AI main system as its long-term destination.

## English

### Project Positioning

KINGAIBOT is not a rename or replacement of the existing KING AI intelligent-lifeform system. It is an execution-oriented system designed to convert high-level intelligence into durable, policy-controlled and auditable action across servers, computers, mobile devices, APIs, tools, workflows and cooperating agents.

```text
KING AI Intelligent Lifeform
        ↓
Reasoning / Memory / Governance / Mission Intelligence
        ↓
Controlled Integration Boundary
        ↓
KINGAIBOT Execution Layer
        ↓
Server Runtime + Windows/macOS/Android Control Center
        ↓
Devices / APIs / Tools / MCP / A2A / Workflows / Services
```

### Current V1.3 Foundation

#### Server Runtime

- Cross-platform Go runtime for Linux, macOS and Windows.
- Go 1.26.5 production baseline.
- API-key-based, model-vendor-independent provider layer.
- Durable tasks, restart recovery and explicit terminal states.
- Approval-aware tools with `allow / ask / deny` policy.
- Exact approvals bound to task + tool + canonical argument hash.
- Go `os.Root` traversal-resistant filesystem sandbox.
- HTTPS destination allowlists with SSRF/DNS-rebinding protection.
- Authenticated Provider / MCP / A2A calls do not follow redirects.
- Shell denied by default and restricted to explicit bare-command allowlists when enabled.
- Separate Admin / MCP / A2A identities.
- MCP server/bridge and A2A agent interoperability.
- Hash-chained audit/event log with runtime integrity verification.
- Controlled-evolution proposals rather than uncontrolled core self-modification.
- Safe update path with checksum/signature policy, readiness verification and rollback.

#### Windows / macOS / Android Control Center

V1.3 introduces a shared **Tauri 2 + React + TypeScript + Rust** client platform under `clients/control-center/`.

- Windows native client foundation.
- macOS native client foundation.
- Android companion client with real Debug package build validation.
- Shared responsive Control Center UI.
- Rust-side secure HTTPS transport; the WebView does not directly call the Runtime.
- Durable task creation, listing and cancellation.
- Exact-action approval review and approve/deny controls.
- Android biometric confirmation before approving sensitive actions.
- Controlled-evolution proposal visibility.
- Stronghold-backed encrypted local profile storage.
- Separate Tauri capabilities for desktop and Android.
- No frontend filesystem, shell or process permission.
- Remote servers require HTTPS; plaintext HTTP is restricted to loopback/local Runtime development.

#### V1.3 Device Identity

Routine clients no longer need to copy the server Admin Token.

1. A trusted administrator creates a short-lived pairing.
2. The server produces a one-time Pairing ID and 256-bit secret.
3. The new Windows/macOS/Android client consumes it once.
4. The server returns a 256-bit **Device Token** with explicit scopes.
5. Only SHA-256 hashes of pairing/device secrets are persisted by the server.
6. A device can be revoked independently without rotating the Admin Token.

Current scopes include:

```text
tasks:read
tasks:create
tasks:cancel
approvals:read
approvals:decide
evolution:read
```

Pairings default to five minutes, cannot exceed fifteen minutes and are single-use. The Admin Token remains the superuser credential for trusted administrative workstations, device provisioning and high-level operations.

### Security Model

1. **Model-independent** — models are replaceable reasoning resources, not the operating system.
2. **Least privilege** — intelligence never automatically implies unrestricted execution authority.
3. **Fail closed** — missing audit, invalid approval, integrity failure or insufficient scope blocks side effects.
4. **Device isolation** — one lost client can be revoked without changing every other credential.
5. **Credential separation** — Admin, MCP, A2A and Device identities serve different trust domains.
6. **Durability before autonomy** — ambiguous actions are not blindly replayed after failures.
7. **Controlled evolution** — learning and improvement flow through proposals, tests, review, staged activation and rollback.
8. **Open interoperability** — MCP, A2A and future standards are integration layers rather than lock-in.
9. **Long-term KING AI convergence** — KINGAIBOT evolves independently until its execution boundary is mature enough for controlled integration into the main intelligent-lifeform system.

### Validation Status

The current V1.3 development line has been validated with separate server and client gates:

```text
Go 1.26.5 gofmt / test / vet / race       PASS
GitHub CI + CodeQL for Device Identity     PASS
React/TypeScript production build          PASS
Rust fmt / check / test / clippy           PASS
Linux Control Center security core         PASS
Windows native Rust/Tauri compile          PASS
macOS native Rust/Tauri compile            PASS
Android Tauri init + Debug package build   PASS
Locked npm and Cargo dependency graphs     ENABLED
```

Production distribution is a separate trust step: Windows/macOS installers and Android release packages must be signed with production credentials before customer distribution.

### Documentation

- [Documentation Index](docs/README.md)
- [Product Definition](docs/PRODUCT.md)
- [User & Operations Guide](docs/USAGE.md)
- [Cross-Platform Client Architecture](docs/CLIENT-PLATFORM.md)
- [Control Center Guide](clients/control-center/README.md)
- [Long-Term Roadmap](docs/ROADMAP.md)
- [Future Technology Integration Plan](docs/FUTURE-TECHNOLOGY.md)
- [V1.2 Security & Execution-Layer Baseline](docs/V1.2-SECURITY-AND-EXECUTION.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](docs/SECURITY.md)
- [Controlled Evolution](docs/EVOLUTION.md)
- [Protocols](docs/PROTOCOLS.md)
- [Deployment](docs/DEPLOYMENT.md)
- [Validation](docs/VALIDATION.md)
- [Residual Risks](docs/RESIDUAL-RISKS.md)
- [Support](docs/SUPPORT.md)

### Server Quick Start

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

### Control Center Development

```bash
cd clients/control-center
npm ci
npm run tauri:dev
```

Android development:

```bash
npm run android:init
npm run android:dev
```

Repository: **https://github.com/kingaiwork/KINGAIBOT**

For commercial production, use reviewed immutable releases and signed client artifacts rather than treating a moving branch as a production release.

---

# 中文

## 项目定位

**KINGAIBOT 是 kingai.work 面向未来长期开发的智能体执行系统，并将作为现有 KING AI 智慧生命体未来的终端执行层（Execution Layer）。**

它不是 KING AI 主系统的改名，也不是替代主系统。KINGAIBOT 当前独立研发、测试和部署，负责把上层智慧、目标、任务和治理要求转化为对服务器、电脑、手机、API、工具、工作流和其他智能体的安全、持久、可控、可审计执行；未来成熟后再通过受控接口并入 KING AI 主系统。

```text
KING AI 智慧生命体
        ↓
推理 / 记忆 / 治理 / Mission 智慧
        ↓
受控整合边界
        ↓
KINGAIBOT 终端执行层
        ↓
Server Runtime + Windows/macOS/Android Control Center
        ↓
设备 / API / 工具 / MCP / A2A / 工作流 / 服务
```

### 当前 V1.3 基础能力

#### Server Runtime

- Linux / macOS / Windows 跨平台 Go Runtime。
- Go 1.26.5 生产基线。
- API Key 多模型 Provider，模型厂商可替换。
- Durable Task 持久任务、重启恢复和明确终态。
- `allow / ask / deny` 权限与审批机制。
- 审批精确绑定“任务 + 工具 + 规范化参数哈希”。
- 基于 Go `os.Root` 的抗路径穿越文件系统沙箱。
- HTTPS 白名单、SSRF/DNS Rebinding 防护。
- Provider / MCP / A2A 鉴权请求禁止自动跟随重定向。
- Shell 默认关闭，启用时只能使用管理员明确配置的裸命令名。
- Admin / MCP / A2A 独立身份。
- MCP Server/Bridge 与 A2A 智能体互操作。
- SHA-256 前向哈希链审计与运行期完整性验证。
- 受控进化 Proposal，而不是生产核心无限自改。
- 校验、签名策略、readiness 和自动回滚式升级路径。

#### Windows / macOS / Android Control Center

V1.3 新增统一的 **Tauri 2 + React + TypeScript + Rust** 客户端平台：

- Windows 原生客户端基础。
- macOS 原生客户端基础。
- Android Companion 客户端，并已经过真实 Debug Package 构建验证。
- 桌面与手机共享响应式 Control Center UI。
- 网络请求由 Rust 安全层发出，WebView 不直接调用服务器。
- 创建、查看、取消持久任务。
- 查看精确动作参数并批准/拒绝审批。
- Android 敏感审批前调用系统生物识别。
- 查看受控进化提案。
- Stronghold 加密保存本机 Profile。
- Windows/macOS 与 Android 使用独立 Tauri Capability。
- 前端不拥有文件系统、Shell、进程权限。
- 远程服务器强制 HTTPS；明文 HTTP 仅用于本机 loopback/local Runtime 开发。

#### V1.3 设备身份

普通 Windows/macOS/Android 客户端不再需要长期复制服务器 Admin Token。

1. 可信管理员创建短时 Pairing；
2. 服务器生成一次性 Pairing ID 和 256-bit Secret；
3. 新设备只能消费一次；
4. 服务器签发具有明确 Scope 的 256-bit Device Token；
5. 服务器磁盘只保存 Pairing/Device Secret 的 SHA-256 哈希，不保存明文；
6. 某一台手机或电脑丢失时，可只撤销该设备，不必重置整个服务器 Admin Token。

当前 Device Scope：

```text
tasks:read
tasks:create
tasks:cancel
approvals:read
approvals:decide
evolution:read
```

Pairing 默认 5 分钟、最长 15 分钟，并且只能使用一次。Admin Token 继续作为可信管理员工作站、设备注册和高级运维的超级权限身份。

### 安全原则

1. **模型不是系统本身**：任何模型都可以替换。
2. **越智能不等于越高权限**：执行必须最小权限化。
3. **危险操作 Fail-Closed**：审计、审批、完整性或 Scope 异常时阻止副作用。
4. **设备身份隔离**：一台终端失陷可以单独撤销。
5. **身份域分离**：Admin、MCP、A2A、Device 使用不同信任边界。
6. **先保证持久可靠，再扩大自治范围**。
7. **允许学习和进化，但必须经过 Proposal、测试、审查、阶段启用和回滚**。
8. **持续融合开放标准**：MCP、A2A、WASM/WASI 与未来智能体协议。
9. **最终与 KING AI 主系统融合**：当前独立演进，成熟后成为主系统受控终端执行层。

### 当前验证状态

```text
Go 1.26.5 gofmt / test / vet / race       PASS
Device Identity GitHub CI + CodeQL         PASS
React/TypeScript Production Build          PASS
Rust fmt / check / test / clippy           PASS
Linux Control Center Security Core         PASS
Windows Native Rust/Tauri Compile          PASS
macOS Native Rust/Tauri Compile            PASS
Android Tauri Init + Debug Package Build   PASS
npm / Cargo 锁定依赖                         ENABLED
```

正式面向客户发行仍需要独立的生产信任步骤：Windows/macOS 安装包和 Android Release 包必须使用正式签名凭据后再发布。

### 文档

- [文档总入口](docs/README.md)
- [产品定义](docs/PRODUCT.md)
- [使用与运维手册](docs/USAGE.md)
- [跨平台客户端架构](docs/CLIENT-PLATFORM.md)
- [Control Center 使用说明](clients/control-center/README.md)
- [长期开发路线图](docs/ROADMAP.md)
- [未来前沿技术融合计划](docs/FUTURE-TECHNOLOGY.md)
- [V1.2 安全与终端执行层基线](docs/V1.2-SECURITY-AND-EXECUTION.md)
- [架构](docs/ARCHITECTURE.md)
- [安全](docs/SECURITY.md)
- [受控进化](docs/EVOLUTION.md)
- [协议](docs/PROTOCOLS.md)
- [部署](docs/DEPLOYMENT.md)
- [验证记录](docs/VALIDATION.md)
- [剩余风险](docs/RESIDUAL-RISKS.md)
- [支持](docs/SUPPORT.md)

官网：**https://kingai.work**  
邮箱：**vip@kingai.work**  
GitHub：**kingaiwork/KINGAIBOT**

> KINGAIBOT 是长期工程。仓库中的版本代表当前经过验证的阶段能力，而不是“绝对零 Bug”或“永远终极”的承诺。高权限能力始终遵循最小权限、可审计、可撤销、可回滚和人工接管原则。
