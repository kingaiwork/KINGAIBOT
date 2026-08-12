# Changelog

## 1.2.1 - 2026-08-12

### English

Supply-chain publication patch over V1.2.0:

- Replaced the hand-written CycloneDX JSON generator with the official `CycloneDX/cyclonedx-gomod` generator pinned to v1.10.0.
- Preserved reproducible-build behavior by normalizing release timestamp and KINGAIBOT version metadata after standards-compliant BOM generation.
- Fixed the controlled tag-first release dispatcher to pass the repository explicitly when invoking the tagged workflow.
- Kept `v1.2.0` immutable after its publication pipeline correctly stopped at SBOM validation; no failed or unsigned binary Release was published from that tag.
- Bumped runtime and example configuration identity to KINGAIBOT 1.2.1.
- V1.2.1 retains all V1.2 execution-layer and security hardening features unchanged.

### 中文

V1.2.0 之上的供应链发布修复版：

- 不再手写 CycloneDX JSON，改用 CycloneDX 官方 `CycloneDX/cyclonedx-gomod` 生成器，并固定到 v1.10.0。
- 在标准 SBOM 生成后仅规范化发布时间戳和 KINGAIBOT 版本元数据，继续保持可复现构建目标。
- 修复受控 Tag-first Release 调度器，触发 Tag 工作流时显式指定仓库。
- 保持 `v1.2.0` Tag 不可移动；该版本发布流程在 SBOM 验证处正确停止，没有发布失败或未签名的二进制 Release。
- Runtime 和示例配置版本更新为 KINGAIBOT 1.2.1。
- V1.2.1 完整保留 V1.2 已通过验证的执行层功能和安全加固。

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
