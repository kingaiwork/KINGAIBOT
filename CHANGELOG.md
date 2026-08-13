# Changelog

## 1.3.0 - 2026-08-13

### English

Cross-platform client and device-identity expansion:

- Added the first shared KINGAIBOT Control Center for Windows, macOS and Android using Tauri 2 + React + TypeScript + Rust.
- Added a Rust-side secure transport boundary: remote HTTPS enforcement, loopback-only plaintext development mode, no authenticated redirects, bounded responses and validated resource identifiers.
- Added Control Center views for runtime readiness, durable tasks, exact-action approvals, controlled-evolution proposals and trusted-device administration.
- Added Android biometric confirmation before approving sensitive actions.
- Added Stronghold-backed encrypted local server/device profile storage.
- Split Tauri desktop/mobile capabilities and intentionally omitted frontend filesystem, shell and process permissions.
- Added one-time short-lived Device Pairing so ordinary clients do not need to retain the server Admin Token.
- Added 256-bit Device Tokens with explicit task/approval/evolution scopes; only SHA-256 hashes of pairing/device secrets are persisted server-side.
- Added per-device revocation without rotating the global Admin Token.
- Added administrator APIs/UI to create pairings, list trusted devices and revoke individual devices.
- Kept the Admin Token as a trusted superuser/bootstrap credential rather than the normal long-lived mobile/desktop identity.
- Added locked npm/Cargo dependency graphs and dedicated client CI.
- Validated Linux security-core, Windows native compile, macOS native compile and a real Android Debug package build on GitHub-hosted runners.
- Revalidated the integrated Go Runtime with Go 1.26.5 format, unit, vet and race gates; Device Identity also passed formal CI and CodeQL before integration.
- Expanded the bilingual product/client documentation to describe KINGAIBOT as the future terminal execution layer for the KING AI intelligent-lifeform system.

### 中文

跨平台客户端与设备身份扩展版本：

- 新增第一套统一 KINGAIBOT Control Center，基于 Tauri 2 + React + TypeScript + Rust，面向 Windows、macOS 与 Android。
- 新增 Rust 安全传输边界：远程强制 HTTPS、明文仅允许本机 loopback 开发、携带凭据禁止自动重定向、响应大小限制与资源 ID 校验。
- Control Center 新增 Runtime readiness、持久任务、精确动作审批、受控进化提案和可信设备管理界面。
- Android 敏感审批前加入系统生物识别确认。
- 使用 Stronghold 加密保存本机服务器/设备 Profile。
- Windows/macOS 与 Android 使用独立 Tauri Capability，并故意不授予前端文件系统、Shell 和进程权限。
- 新增短时一次性 Device Pairing，普通客户端不再需要长期保存服务器 Admin Token。
- 新增 256-bit Device Token 与任务/审批/进化 Scope；服务器磁盘只保存 Pairing/Device Secret 的 SHA-256 哈希。
- 支持单设备撤销，无需轮换全局 Admin Token。
- 新增管理员创建 Pairing、查看可信设备和撤销单设备的 API/UI。
- Admin Token 继续作为可信管理员/首次引导超级权限凭据，而不是普通手机/桌面端长期身份。
- npm/Cargo 依赖图锁定，并新增独立 Client CI。
- GitHub 原生 Runner 已验证 Linux 安全核心、Windows 原生编译、macOS 原生编译以及真实 Android Debug Package 构建。
- 整合后的 Go Runtime 再次通过 Go 1.26.5 格式、单元、vet、race；Device Identity 在整合前也已通过正式 CI 与 CodeQL。
- 扩展中英文产品/客户端文档，进一步明确 KINGAIBOT 将作为 KING AI 智慧生命体未来的终端执行层。

## 1.2.0 - 2026-08-12

### English

Security-first execution-layer expansion:

- Replaced file tool execution paths with Go `os.Root` traversal-resistant operations to reduce path traversal, symlink-escape and check-then-use risk.
- Added path length/component bounds and regular-file-only reads.
- Added safe `file_stat`, `file_list`, `file_mkdir` and single-item `file_delete` capabilities; recursive delete remains intentionally unavailable to agents.
- Added cross-platform atomic overwrite regression coverage.
- Hardened generic HTTPS access against protocol downgrade, non-standard-port redirect escape and global `*` destination allowlists.
- Disabled automatic redirects for authenticated model-provider, MCP and A2A requests so credentials are not forwarded across redirect targets.
- Strengthened shell allowlist startup validation while keeping shell execution denied by default.
- Moved the production source baseline to Go 1.26.5.
- Updated runtime identity/version to KINGAIBOT 1.2.0.
- Renamed release archives to the `kingaibot_...` product prefix while retaining `kingagent` / `kingagentd` binary names for compatibility.
- Updated signed-release attestations and Sigstore paths for the new package identity.
- Added the bilingual V1.2 security/execution-layer baseline documentation.
- Changes are gated by formatting, vet, unit/race tests, govulncheck, macOS/Windows native validation, PowerShell parsing, Docker build and CodeQL before merge/release.

### 中文

安全优先的终端执行层扩展版本：

- 文件工具真实执行路径改用 Go `os.Root` 抗路径穿越 API，降低路径穿越、符号链接逃逸和 check-then-use / TOCTOU 风险。
- 加入路径长度、目录层级限制，并将文件读取限制为普通文件。
- 新增安全 `file_stat`、`file_list`、`file_mkdir` 和单项 `file_delete`；V1.2 仍故意不向智能体开放递归删除。
- 新增跨平台原子覆盖写入回归测试。
- HTTP 能力进一步阻止 HTTPS 降级、非标准端口重定向逃逸和全局 `*` 目标白名单。
- Provider、MCP、A2A 鉴权请求禁止自动跟随重定向，防止凭据被带到重定向目标。
- 强化 Shell 白名单启动校验，同时继续默认 DENY Shell。
- 生产源码基线升级到 Go 1.26.5。
- Runtime 身份与版本升级到 KINGAIBOT 1.2.0。
- 正式发行归档统一使用 `kingaibot_...` 前缀，同时为了兼容继续保留 `kingagent` / `kingagentd` 二进制名称。
- GitHub provenance / SBOM / Sigstore 正式发布路径同步到新包名。
- 新增英文在前、中文在后的 V1.2 安全与终端执行层基线文档。
- 合并与发布前继续强制通过格式、vet、单元/Race、govulncheck、macOS/Windows 原生验证、PowerShell 解析、Docker 和 CodeQL。

## 1.1.0 - 2026-08-12

Security/reliability hardening release over the initial v1.0 commercial baseline:

- Correct A2A v1 method/state/role profile and MCP 2026-07-28 result/header behavior.
- Exact argument-bound approvals with durable at-most-once execution state and crash reconciliation.
- Fixed in-flight cancellation overwrite and queue-recovery goroutine amplification.
- Added DNS-to-dial pinning, permanent link-local denial, HTTPS-only generic HTTP and stricter redirect/size controls.
- Fixed shell allowlist path masquerading; bare command names only.
- Added SHA-256 hash-chained audit events, periodic verification, readiness integration and fail-closed side effects.
- Split Admin/MCP/A2A credentials and hardened Agent Card/base URL handling.
- Added bounded/expiring secret-redacted memory with raw-input learning disabled by default.
- Hardened atomic storage replacement across Unix/Windows.
- Hardened remote CLI HTTPS/redirect/response behavior.
- Hardened update verification parity across Linux/macOS/Windows.
- Pinned GitHub Actions to immutable commit SHAs and added govulncheck, CycloneDX SBOM, provenance/SBOM attestations and Sigstore release signing.
- Added explicit residual-risk and engineering-audit documentation.

## 1.0.0 - 2026-08-12

- Initial cross-platform commercial baseline.
