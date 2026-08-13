# KINGAIBOT Client Platform Architecture

> English first / 中文在后

## English

### 1. Product role

KINGAIBOT is the future terminal execution layer for the KING AI intelligent-life system. The server runtime remains the durable, policy-controlled execution authority; Windows, macOS and Android clients become trusted control surfaces for people and devices.

The client platform is intentionally separated from the core runtime:

- **KINGAIBOT Server Runtime** — durable tasks, model routing, memory, MCP/A2A, tools, policy, approvals, audit and evolution proposals.
- **KINGAIBOT Control Center** — one Tauri 2 application codebase for Windows and macOS, with Android sharing the same UI and protocol SDK.
- **Android Companion Mode** — task dispatch, approval, monitoring, notification and biometric confirmation. Android does not receive desktop shell/root capabilities.
- **Future KING AI integration** — KING AI can delegate terminal execution to one or more KINGAIBOT runtimes through controlled interfaces.

### 2. Technology direction

The Control Center uses:

- Tauri 2 for Windows/macOS/Android packaging and native bridges.
- React + TypeScript for the shared interface.
- Rust inside the Tauri process for network transport and local security boundaries.
- IOTA Stronghold through the official Tauri plugin for encrypted local credentials.
- Official Tauri biometric integration on Android for high-risk approval confirmation.
- Official Tauri notifications for approval and task-state notifications.
- The existing Go runtime for server-side durable execution.

The frontend never receives direct filesystem, shell or process permissions. Network calls to KINGAIBOT are made by a narrow Rust command layer. The Rust transport rejects credential-bearing URLs, redirect following, non-HTTPS remote servers and non-loopback plaintext HTTP.

### 3. Modes

#### Remote Server mode

Windows, macOS or Android connects to an existing KINGAIBOT server over HTTPS. This is the default mode for VPS, office and home-server deployments.

#### Local Runtime mode (desktop roadmap)

Windows/macOS installers will bundle `kingagentd` as a Tauri sidecar. The desktop app will start a low-privilege local runtime, generate local credentials, monitor readiness and stop/recover the process cleanly. This mode is intentionally not enabled until sidecar signing and lifecycle tests are complete.

#### Mobile Companion mode

Android is a controller, not a privileged workstation. It can:

- submit and inspect tasks;
- approve/deny exact pending actions;
- require biometric confirmation before approval;
- view evolution proposals;
- receive notifications;
- later scan a QR enrollment code and store a device-scoped token.

### 4. Security model

1. Remote production servers require HTTPS.
2. Plain HTTP is accepted only for loopback development/local runtime endpoints.
3. The client transport does not follow redirects when an authorization token is attached.
4. Tokens are never written to logs.
5. Persisted secrets use an encrypted local vault rather than browser localStorage.
6. Android approval can be gated by system biometric authentication.
7. Tauri capabilities are split by platform; Android is not granted desktop-only capabilities.
8. The application loads only bundled UI assets; remote web pages do not get Tauri command access.
9. Future device enrollment will issue device-scoped revocable credentials instead of distributing the server admin token.
10. Signed desktop auto-updates and Android store signing are required before production distribution.

### 5. V1.3 functional scope

The first Control Center implements:

- server connection and authentication verification;
- health/readiness status;
- create/list/cancel tasks;
- list and approve/deny pending approvals;
- list controlled-evolution proposals;
- responsive desktop/mobile UI;
- Android biometric gate before approval;
- local notification hooks;
- encrypted profile-vault primitives;
- strict Rust-side server URL validation.

### 6. Next protocol upgrade: device enrollment

A later server increment will add device-native identity:

1. An administrator creates a short-lived pairing session.
2. The server returns a QR code containing only a pairing identifier/nonce — never a reusable admin token.
3. The Android/desktop client presents an ephemeral device public key.
4. The server records device identity, requested role and capability set.
5. An existing trusted operator approves enrollment.
6. The server issues a revocable, scoped device credential.
7. High-risk approval signatures bind device ID, task ID, action hash and timestamp.

This lets a phone approve an action without becoming a copy of the server administrator identity.

### 7. Long-term platform plan

- V1.3: shared Windows/macOS/Android Control Center foundation.
- V1.4: QR device enrollment, per-device tokens, revocation and role scopes.
- V1.5: WebSocket event stream, push notifications and offline-safe command queue.
- V1.6: signed `kingagentd` desktop sidecar and one-click local mode.
- V1.7: local device connectors, file handoff and consent-scoped desktop automation.
- V1.8: multi-runtime fleet view, health aggregation and remote diagnostics.
- V2.x: enterprise OIDC/passkeys, hardware-backed keys, policy federation and KING AI main-system execution fabric.

---

## 中文

### 1. 产品定位

KINGAIBOT 是 KING AI 智慧生命体未来的终端执行层。服务器 Runtime 继续作为持久任务、权限、安全策略和审计的最终执行权威；Windows、macOS 与 Android 客户端成为人与设备使用 KINGAIBOT 的可信控制终端。

客户端与核心 Runtime 明确分层：

- **KINGAIBOT Server Runtime**：负责持久任务、模型路由、记忆、MCP/A2A、工具、安全策略、审批、审计与受控进化提案。
- **KINGAIBOT Control Center**：Windows/macOS 采用 Tauri 2，Android 共用同一套 UI 和协议 SDK。
- **Android Companion Mode**：负责下发任务、审批、监控、通知和生物识别确认，不授予桌面 Shell/root 能力。
- **未来 KING AI 整合**：KING AI 主系统通过受控接口，把终端执行任务委托给一个或多个 KINGAIBOT Runtime。

### 2. 技术方向

Control Center 采用：

- Tauri 2：Windows/macOS/Android 原生打包与系统桥接；
- React + TypeScript：共享 UI；
- Rust：运行在 Tauri 后端，负责安全网络传输和本地边界；
- IOTA Stronghold：用于本地加密凭据；
- Tauri 官方 Android Biometric：高风险审批前的系统生物识别；
- Tauri Notification：审批和任务状态通知；
- 现有 Go Runtime：继续承担服务器端持久执行。

前端不直接获得文件系统、Shell 或系统进程权限。所有 KINGAIBOT 网络调用通过狭窄的 Rust 命令层完成。Rust 传输层拒绝 URL 内嵌账号密码、拒绝携带授权信息的重定向、拒绝远程明文 HTTP；只有本机 loopback 可用于开发和本地 Runtime。

### 3. 三种运行模式

#### 远程服务器模式

Windows、macOS、Android 通过 HTTPS 连接 VPS、办公室服务器或家庭服务器中的 KINGAIBOT。

#### 本机 Runtime 模式（桌面路线）

未来 Windows/macOS 安装包会把 `kingagentd` 作为 Tauri sidecar 一起签名打包。桌面程序以低权限启动本机 Runtime，自动生成本机凭据、检查 readiness、负责异常恢复。侧车签名和生命周期测试完成前，不把这一模式默认打开。

#### Android Companion 模式

Android 是安全控制终端，而不是高权限工作站，可执行：

- 创建、查看任务；
- 审批/拒绝精确动作；
- 审批前进行生物识别；
- 查看进化提案；
- 接收通知；
- 后续扫描 QR 完成设备注册并保存设备级凭据。

### 4. 安全模型

1. 远程生产服务器只允许 HTTPS；
2. 明文 HTTP 仅允许 loopback；
3. 携带授权 Token 的请求禁止自动跟随重定向；
4. Token 永不写入日志；
5. 持久凭据进入加密 Vault，不使用浏览器 localStorage；
6. Android 高风险审批可强制系统生物识别；
7. Tauri capability 按平台拆分，手机不会继承桌面权限；
8. App 只加载随安装包发布的 UI，不允许远程网页获得 Tauri 命令权限；
9. 未来设备注册使用可撤销、有限权限的 Device Credential，而不是复制 Admin Token；
10. 正式发行必须使用签名桌面升级和 Android 商店/安装包签名。

### 5. V1.3 第一阶段完整功能

- 服务器连接与鉴权验证；
- health/readiness；
- 创建/查看/取消任务；
- 查看并批准/拒绝审批；
- 查看受控进化提案；
- 桌面/手机响应式 UI；
- Android 生物识别审批门；
- 本地通知接口；
- 加密 Profile Vault 基础；
- Rust 端严格服务器 URL 校验。

### 6. 下一阶段：设备注册协议

设备注册不会把服务器管理员 Token 放进二维码。流程为：管理员创建短期 pairing session → QR 只携带一次性 ID/nonce → 客户端提交临时设备公钥 → 可信管理员确认设备身份和权限 → 服务器签发可撤销的设备级凭据 → 高风险审批把设备 ID、任务、动作哈希和时间绑定在一起。

### 7. 长期路线

- V1.3：Windows/macOS/Android 共用 Control Center 基础；
- V1.4：QR 配对、设备级 Token、撤销、角色权限；
- V1.5：WebSocket 实时事件、Push 与离线安全队列；
- V1.6：签名 `kingagentd` 桌面 sidecar、一键本机模式；
- V1.7：设备连接器、文件安全交接、用户同意范围内的桌面自动化；
- V1.8：多 Runtime Fleet、健康聚合、远程诊断；
- V2.x：企业 OIDC/Passkey、硬件密钥、策略联邦，并正式形成 KING AI 主系统的分布式终端执行网络。
