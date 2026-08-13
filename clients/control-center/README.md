# KINGAIBOT Control Center

> Windows · macOS · Android  
> English first / 中文在后

## English

KINGAIBOT Control Center is the human-facing cross-platform client for the KINGAIBOT execution layer. It connects to the existing Go server runtime and deliberately keeps privileged execution on the server side.

### Current V1.3 capabilities

- Connect to a local or remote KINGAIBOT Runtime.
- HTTPS required for remote servers; loopback HTTP allowed for local development/runtime mode.
- Verify authentication before retaining an in-memory session.
- List/create/cancel durable tasks.
- Review exact pending tool arguments before approval.
- Approve or deny actions.
- Android/iOS biometric confirmation before approval.
- List controlled-evolution proposals and evidence.
- Poll runtime state and send local approval notifications.
- Save/load an optional encrypted server profile using Stronghold.
- Responsive desktop/mobile interface with light/dark appearance.

### Security boundary

The React WebView never sends KINGAIBOT HTTP requests itself. It invokes a small Rust command surface. The Rust backend:

- validates the server origin;
- rejects remote plaintext HTTP;
- rejects embedded URL credentials, query strings and fragments;
- never follows redirects while holding an authorization token;
- limits response sizes;
- validates path/resource identifiers before encoding them into API URLs;
- keeps the active server token in Rust memory after connection.

The frontend has **no Tauri filesystem, shell or process permission**. Desktop and mobile capabilities are separate. Android receives biometric permission but not desktop privileges.

### Development

Requirements:

- Node.js LTS
- Rust stable
- Tauri 2 platform prerequisites
- Android Studio/JDK/Android SDK/NDK for Android development

```bash
cd clients/control-center
npm install
npm run tauri:dev
```

Android project initialization:

```bash
npm run android:init
npm run android:dev
```

Production desktop build:

```bash
npm run tauri:build
```

Production Android build:

```bash
npm run android:build
```

Do not distribute unsigned production installers. macOS direct distribution requires signing/notarization; Windows production installers should be code-signed; Android release packages must be signed before store/direct distribution.

### Connection model

For development on the same machine:

```text
http://127.0.0.1:18888
```

For remote production deployments:

```text
https://agent.example.com
```

The current V1.3 client can use the Runtime admin token. This is an interim bootstrap model. V1.4 replaces routine remote use with short-lived QR pairing and revocable device-scoped credentials.

### Local desktop Runtime

The UI and architecture are ready for local mode, but V1.3 does not silently launch a privileged binary. The planned V1.6 desktop mode will bundle a signed `kingagentd` sidecar, run it under the logged-in user's low-privilege identity, provision loopback credentials and supervise readiness/recovery.

---

## 中文

KINGAIBOT Control Center 是 KINGAIBOT 终端执行层的人机控制客户端，统一服务于 Windows、macOS 与 Android。高权限执行继续留在 Go Server Runtime，客户端只承担可信交互、任务控制、审批和监控，不把手机或桌面 UI 变成另一个高权限执行引擎。

### V1.3 当前能力

- 连接本机或远程 KINGAIBOT Runtime；
- 远程强制 HTTPS，本机 loopback 开发模式允许 HTTP；
- 鉴权成功后才在 Rust 内存中保留会话；
- 创建/查看/取消持久任务；
- 审批前查看完整精确工具参数；
- 批准/拒绝动作；
- Android/iOS 审批前生物识别确认；
- 查看受控进化提案及证据；
- 自动轮询状态并发送本机审批通知；
- Stronghold 加密保存可选服务器 Profile；
- 桌面/手机响应式 UI 与明暗模式。

### 安全边界

React WebView 不直接向 KINGAIBOT Server 发 HTTP 请求，而是调用受限 Rust 命令。Rust 层负责：

- 校验服务器 Origin；
- 拒绝远程明文 HTTP；
- 拒绝 URL 内嵌账号密码、Query、Fragment；
- 请求携带授权信息时绝不自动跟随 Redirect；
- 限制响应大小；
- API 资源 ID 进入 URL 前先验证；
- 连接后把当前 Token 留在 Rust 内存，不继续放在 React state。

前端 **没有 Tauri 文件系统、Shell、进程权限**。桌面和手机使用独立 Capability；Android 只有额外生物识别权限，不继承桌面能力。

### 开发

```bash
cd clients/control-center
npm install
npm run tauri:dev
```

Android：

```bash
npm run android:init
npm run android:dev
```

桌面生产构建：

```bash
npm run tauri:build
```

Android 生产构建：

```bash
npm run android:build
```

正式安装包不得以未签名状态对客户发布。macOS 直接分发需要签名/公证，Windows 正式安装包应代码签名，Android Release 必须使用正式签名密钥。

### 当前连接模式

本机开发：

```text
http://127.0.0.1:18888
```

远程生产：

```text
https://agent.example.com
```

V1.3 暂时允许使用 Runtime Admin Token 完成早期联调。这只是过渡方案；V1.4 将改为 QR 短时配对 + 可撤销设备级凭据，让手机和桌面不再长期持有服务器管理员身份。

### 桌面本机 Runtime

V1.3 已为本机模式预留架构，但不会为了“一键体验”偷偷启动高权限程序。V1.6 计划把签名后的 `kingagentd` 作为 Tauri sidecar 内置，由当前登录用户的低权限身份运行，自动配置 loopback 凭据，并负责 readiness、故障恢复和生命周期管理。
