# KINGAIBOT User & Operations Guide / 使用与运维手册

## English

### 1. Core components

- `kingagentd` — persistent runtime service
- `kingagent` — command-line control client
- `config.json` — runtime configuration
- durable stores — tasks, approvals, memory, evolution proposals, device identities and audit state
- `workspace_dir` — default bounded local execution workspace
- MCP / A2A bridges — controlled external tool and agent interoperability
- `clients/control-center` — Windows/macOS/Android Control Center source

### 2. First local start

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

Check runtime readiness:

```bash
curl -fsS http://127.0.0.1:18888/healthz
curl -fsS http://127.0.0.1:18888/readyz
```

### 3. Submit and inspect work

```bash
go run ./cmd/kingagent run "Your authorized task"
go run ./cmd/kingagent tasks
go run ./cmd/kingagent approvals
```

When an operation is waiting for approval:

```bash
go run ./cmd/kingagent approvals
go run ./cmd/kingagent approve appr_xxx
```

Approvals are bound to the task, tool and canonical argument hash. Changing the requested path, command, destination or content requires a new approval.

### 4. Local execution capabilities

Recommended baseline policies are shown in `configs/config.example.json`:

| Tool | Function | Recommended default |
|---|---|---|
| `system_info` | bounded local system information | `allow` |
| `file_read` | read a bounded regular file under approved read roots | `allow` |
| `file_stat` | inspect metadata under approved read roots | `allow` |
| `file_list` | list up to 1024 entries in an approved directory | `allow` |
| `file_write` | atomically write a bounded file under write roots | `ask` |
| `file_mkdir` | create a directory tree under write roots | `ask` |
| `file_delete` | delete one file or one empty directory | `ask` |
| `http_get` | HTTPS request to an allowlisted destination | `ask` |
| `mcp_tools_list` | inspect configured MCP tools | `allow` |
| `mcp_tools_call` | invoke a configured remote MCP tool | `ask` |
| `a2a_send` | delegate to a configured A2A peer | `ask` |
| `shell_exec` | execute a directly allowlisted binary without a shell | `deny` |

KINGAIBOT intentionally does **not** expose recursive deletion as a general agent tool in this baseline.

### 5. File sandbox

`file_read_roots` and `file_write_roots` define the filesystem capability boundary. File tools execute through Go `os.Root`; the path is not merely checked and then opened later. Symlink and `..` traversal that would escape the configured root are rejected by the rooted filesystem operation.

Example:

```json
{
  "file_read_roots": ["./data/workspace"],
  "file_write_roots": ["./data/workspace"]
}
```

Do not configure `/`, an entire user home, system configuration directories or secret stores as broad write roots for a general-purpose agent.

### 6. Network boundaries

`http_get` requires HTTPS and an explicit host allowlist. The global wildcard `*` is rejected. DNS/IP validation blocks unsafe address classes and the request is pinned to validated resolution to reduce DNS-rebinding exposure. Redirects may not downgrade to HTTP, escape the host allowlist or move to a non-standard HTTPS port.

Model Provider, MCP and A2A requests use administrator-configured endpoints and do not automatically follow redirects while carrying credentials.

### 7. Shell execution

Keep `shell_exec` set to `deny` unless the deployment genuinely requires it. When enabled:

- only bare executable names explicitly listed in `shell_allowlist` are accepted;
- executable paths are rejected;
- commands are executed directly with argv rather than through `sh -c`, `cmd /c` or another shell parser;
- dangerous or broad interpreters should not be allowlisted merely for convenience;
- prefer purpose-built MCP/WASM/native capabilities over broad shell access.

### 8. Identity, device pairing and secrets

Keep these trust domains separate:

- Admin API token
- Device token
- MCP token
- A2A token
- model-provider API keys

A normal Windows/macOS/Android Control Center should use a **Device Token**, not a permanently copied Admin Token.

The V1.3 pairing flow is:

1. A trusted administrator connects with the Admin Token.
2. In **Devices**, create a short-lived pairing. The default lifetime is five minutes and the maximum is fifteen minutes.
3. On the new device, choose **Pair this device** and enter the server origin, Pairing ID, one-time secret and a recognizable device name.
4. The pairing can be consumed only once.
5. The server issues a 256-bit scoped Device Token. Only its SHA-256 hash is persisted server-side.
6. Optionally store the credential in the local Stronghold encrypted vault.
7. If the device is lost or retired, revoke only that device from the administrator workstation.

Current device scopes:

```text
tasks:read
tasks:create
tasks:cancel
approvals:read
approvals:decide
evolution:read
```

Do not reuse one token between control planes. Model API keys should remain in environment/credential storage and must not be embedded into prompts or source code.

### 9. Control Center: Windows and macOS

Development:

```bash
cd clients/control-center
npm ci
npm run tauri:dev
```

The desktop client can connect to a remote Runtime over HTTPS or a local loopback Runtime during development. The WebView has no direct filesystem, shell or process capability. HTTP transport and resource validation are performed by the Rust backend.

A future signed sidecar mode will bundle `kingagentd` for one-click local desktop operation. Until the sidecar signing/lifecycle gate is complete, treat the Go service and the desktop Control Center as separate deployable components.

### 10. Android companion client

Initialize/develop Android:

```bash
cd clients/control-center
npm ci
npm run android:init
npm run android:dev
```

Build a development package:

```bash
npm run android:build
```

Android is a control/approval companion, not a privileged workstation. It shares tasks, approvals, evolution status and pairing flows with the desktop UI, while using a separate Tauri capability set. Sensitive approval can require system biometric authentication.

Do not distribute the Debug package as a production artifact. Production Android distribution requires a release signing key and the chosen store/direct-distribution signing process.

### 11. Durable tasks and ambiguous side effects

Tasks survive runtime restarts. Before a side-effecting tool runs, the execution state is persisted. If a crash leaves the result ambiguous, KINGAIBOT does not blindly replay the operation; reconcile the task/approval/audit state before retrying.

### 12. Memory and controlled learning

Long-term memory is bounded and secret-redacted. Raw task inputs are disabled by default in the example configuration. Controlled evolution remains `proposal-only`: the runtime may produce improvement proposals, but production core changes must go through source review, tests, security scanning, signed releases and rollback-capable deployment.

### 13. Commercial installation and upgrade

Production deployments should install immutable signed releases rather than a moving branch. Release assets use the `kingaibot_...` archive prefix. Keep signature verification enabled and retain rollback material until the new runtime passes `/readyz`.

Server release publication uses a version tag, security/race/vulnerability gates, reproducible server artifacts, SBOM, provenance attestations and Sigstore/Cosign signing. Client production signing is a separate platform trust step: Windows installers, macOS applications and Android release packages require their platform signing identities before customer distribution.

### 14. Troubleshooting order

1. `/healthz`
2. `/readyz`
3. runtime/service logs
4. task state
5. approval state
6. device scope/revocation state
7. audit-chain integrity
8. provider/MCP/A2A endpoint configuration
9. workspace/root permissions
10. release/update signature and checksum state

Do not repeatedly replay a side-effecting task when execution state is ambiguous.

---

## 中文

### 1. 核心组件

- `kingagentd`：长期运行的 Runtime 服务
- `kingagent`：命令行控制客户端
- `config.json`：运行配置
- 持久状态：任务、审批、记忆、进化提案、设备身份和审计状态
- `workspace_dir`：默认受控本地执行工作区
- MCP / A2A Bridge：受控接入外部工具和其他智能体
- `clients/control-center`：Windows/macOS/Android Control Center 源码

### 2. 首次本地启动

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

检查 Runtime：

```bash
curl -fsS http://127.0.0.1:18888/healthz
curl -fsS http://127.0.0.1:18888/readyz
```

### 3. 提交与查看任务

```bash
go run ./cmd/kingagent run "经过授权的任务"
go run ./cmd/kingagent tasks
go run ./cmd/kingagent approvals
```

等待审批时：

```bash
go run ./cmd/kingagent approvals
go run ./cmd/kingagent approve appr_xxx
```

审批绑定“任务 + 工具 + 规范化参数哈希”。路径、命令、目标或内容发生改变时必须重新审批。

### 4. 本地执行能力

`configs/config.example.json` 已给出建议默认策略：

| 工具 | 功能 | 建议默认策略 |
|---|---|---|
| `system_info` | 受限系统信息 | `allow` |
| `file_read` | 在允许根目录读取受限大小普通文件 | `allow` |
| `file_stat` | 查看允许根目录中的文件/目录元数据 | `allow` |
| `file_list` | 列出允许目录，最多 1024 项 | `allow` |
| `file_write` | 在写入根目录原子写文件 | `ask` |
| `file_mkdir` | 在写入根目录创建目录树 | `ask` |
| `file_delete` | 删除一个文件或一个空目录 | `ask` |
| `http_get` | 请求显式白名单中的 HTTPS 目标 | `ask` |
| `mcp_tools_list` | 查看已配置 MCP 工具 | `allow` |
| `mcp_tools_call` | 调用已配置远程 MCP 工具 | `ask` |
| `a2a_send` | 委派任务给已配置 A2A Agent | `ask` |
| `shell_exec` | 不经过 shell，执行管理员显式白名单程序 | `deny` |

当前基线**故意不向通用智能体开放递归删除功能**。

### 5. 文件系统沙箱

`file_read_roots` 和 `file_write_roots` 定义文件权限边界。文件工具通过 Go `os.Root` 执行，不再只是“先检查路径、随后重新打开”。符号链接和 `..` 如果会逃离配置根目录，将由 rooted 文件操作拒绝。

示例：

```json
{
  "file_read_roots": ["./data/workspace"],
  "file_write_roots": ["./data/workspace"]
}
```

通用智能体不要把 `/`、整个用户 Home、系统配置目录或秘密存储目录配置成宽泛写入根目录。

### 6. 网络边界

`http_get` 强制 HTTPS 和显式主机白名单，并拒绝全局 `*`。DNS/IP 校验会阻止危险地址类别，并将连接绑定到经过验证的解析结果以降低 DNS Rebinding 风险。重定向不能降级到 HTTP、不能逃出主机白名单，也不能切换到非标准 HTTPS 端口。

模型 Provider、MCP、A2A 属于管理员配置端点；携带鉴权信息的请求不会自动跟随重定向。

### 7. Shell 执行

没有真实需要时保持 `shell_exec=deny`。启用时：

- 只接受 `shell_allowlist` 中管理员显式列出的裸程序名；
- 拒绝可执行文件路径；
- 使用 argv 直接执行，不经过 `sh -c`、`cmd /c` 等 shell 解析器；
- 不要为了方便把宽泛解释器加入白名单；
- 优先使用专门的 MCP / WASM / Native Capability，而不是扩大 Shell 权限。

### 8. 身份、设备配对和秘密

以下信任域必须分离：

- Admin API Token
- Device Token
- MCP Token
- A2A Token
- 模型 Provider API Key

普通 Windows/macOS/Android Control Center 日常应使用 **Device Token**，而不是永久复制 Admin Token。

V1.3 配对流程：

1. 可信管理员使用 Admin Token 登录；
2. 在 **Devices** 页面创建短时 Pairing，默认 5 分钟、最长 15 分钟；
3. 新设备选择 **Pair this device**，输入服务器地址、Pairing ID、一次性 Secret 和设备名称；
4. Pairing 只能消费一次；
5. 服务器签发具有 Scope 的 256-bit Device Token，服务器磁盘只保存 SHA-256 哈希；
6. 可选择把凭据保存到本机 Stronghold 加密 Vault；
7. 手机/电脑丢失或停用时，只撤销该设备即可。

当前 Device Scope：

```text
tasks:read
tasks:create
tasks:cancel
approvals:read
approvals:decide
evolution:read
```

不同控制面不要复用同一个 Token。模型 API Key 应保留在环境变量/凭据存储，不写入 Prompt 和源码。

### 9. Windows 与 macOS Control Center

开发：

```bash
cd clients/control-center
npm ci
npm run tauri:dev
```

桌面客户端可通过 HTTPS 连接远程 Runtime，也可在开发时连接本机 loopback Runtime。WebView 不拥有直接文件系统、Shell、进程权限；网络传输和资源校验由 Rust 后端完成。

未来签名 Sidecar 模式会把 `kingagentd` 内置进桌面安装包，实现一键本机运行。在 Sidecar 签名和生命周期测试完成前，Go Runtime 与桌面 Control Center 仍按两个独立可部署组件管理。

### 10. Android Companion

初始化/开发 Android：

```bash
cd clients/control-center
npm ci
npm run android:init
npm run android:dev
```

开发构建：

```bash
npm run android:build
```

Android 是控制/审批 Companion，而不是高权限工作站。它与桌面端共享任务、审批、进化状态和配对流程，但使用独立 Tauri Capability；敏感审批可要求系统生物识别。

Debug 包不能当正式商用安装包分发。正式 Android 发行必须使用 Release 签名密钥和对应商店/直接分发签名流程。

### 11. Durable Task 与副作用不确定状态

任务可以在 Runtime 重启后恢复。执行有副作用的工具前会先持久化执行状态。如果崩溃导致结果不确定，KINGAIBOT 不会盲目自动重放；应先核对任务、审批和审计状态再决定重试。

### 12. 记忆与受控学习

长期记忆有数量/大小限制并执行秘密清洗；示例配置默认不保存原始任务输入。自进化继续采用 `proposal-only`：Runtime 可以提出改进方案，但生产核心修改必须经过源码审查、测试、安全扫描、签名 Release 和可回滚部署。

### 13. 商用安装与升级

生产环境优先使用不可变、已签名 Release，不直接依赖移动中的分支。服务器发行包统一使用 `kingaibot_...` 前缀。保持签名验证开启，并在新版通过 `/readyz` 前保留回滚材料。

服务器 Release 通过版本 Tag、漏洞/安全/Race 门禁、可复现构建、SBOM、Provenance Attestation 和 Sigstore/Cosign 完成。客户端正式签名属于独立平台信任步骤：Windows 安装包、macOS App、Android Release Package 都必须在面向客户分发前使用正式平台签名身份。

### 14. 故障排查顺序

1. `/healthz`
2. `/readyz`
3. Runtime / Service 日志
4. Task 状态
5. Approval 状态
6. Device Scope / 撤销状态
7. Audit 哈希链完整性
8. Provider / MCP / A2A 端点配置
9. Workspace / Root 权限
10. Release / Update 签名与校验状态

如果有副作用的操作执行状态不确定，不要反复盲目重放。
