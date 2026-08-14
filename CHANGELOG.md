# Changelog

## 1.3.0 - 2026-08-14

### English

Platform expansion over the hardened v1.2 execution core:

- Added a durable Platform Control Plane without weakening the existing policy, exact-approval, audit-integrity or fail-closed execution boundaries.
- Added operator-defined agent profiles and durable sessions linked to runtime task IDs.
- Added persistent recurring schedules with bounded intervals and persisted next-run state.
- Added bounded sequential workflows with restart recovery and task-aware resume behavior.
- Added parallel multi-agent missions with bounded fan-out and aggregated completion state.
- Added device/browser/edge node registration, heartbeat state and policy-gated remote actions.
- Added remote plugin manifests with SHA-256 identity and guarded network execution.
- Added outbound channel adapters so messaging transports can remain outside the trusted daemon process.
- Added integrity-hashed operator skills as instruction data without automatic authority escalation.
- Added a generic extension-tool boundary so platform tools pass the same `allow / ask / deny`, canonical argument hash, exact approval, execution-state and audit pipeline as core tools.
- Added authenticated `/v1/platform/*` administration APIs.
- Added platform and extension-policy unit tests and fixed root binary ignore patterns that previously shadowed `cmd/kingagent*` source paths during automation.
- Runtime version advanced to KINGAIBOT 1.3.0 while retaining the v1.2 hardened execution core as the trust foundation.

### 中文

在 V1.2 加固执行内核之上的完整平台扩展：

- 新增持久化 Platform Control Plane，同时保持原有策略、精确审批、审计完整性和 Fail-Closed 安全边界不变。
- 新增运营者定义的 Agent Profile 与绑定 Runtime Task ID 的持久会话。
- 新增持久定时任务，带执行间隔边界和 `next_run_at` 持久状态。
- 新增有界顺序工作流，支持服务重启后的任务感知恢复。
- 新增并行多智能体 Mission，限制扇出数量并聚合子任务状态。
- 新增桌面 / 浏览器 / 移动 / Edge 节点注册、心跳状态与受策略控制的远程动作。
- 新增带 SHA-256 身份的远程插件清单，并通过安全网络边界执行。
- 新增出站 Channel Adapter，使 Telegram / Discord / Slack / Email / WebChat 等适配器可以保持在可信核心进程之外。
- 新增内容完整性哈希的 Skill 数据；Skill 本身不会自动获得系统权限。
- 新增统一扩展工具边界，使平台工具与核心工具一样经过 `allow / ask / deny`、规范参数哈希、精确审批、执行状态和审计链。
- 新增受 Admin Token 保护的 `/v1/platform/*` 管理 API。
- 新增平台与扩展审批单元测试，并修复 `.gitignore` 根目录二进制规则误覆盖 `cmd/kingagent*` 源码目录的问题。
- Runtime 版本升级为 KINGAIBOT 1.3.0，V1.2 的加固执行核继续作为信任根。

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
