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

Authority, WorkGraph and orchestration hardening added during the v1.3 development cycle:

- Added durable KINGAIBOT Capability Envelopes with capability, data-scope, tool-scope, budget, expiry and delegation-depth boundaries.
- Delegated authority can only narrow a parent grant; revoked or expired parents invalidate descendant grants.
- Added trusted agent-to-task authority binding: platform-created tasks resolve authority from trusted `agent_id` metadata; model tool arguments never carry or select `authority_id`.
- Added durable typed WorkGraph DAGs with approval gates, replay policy, high-risk evidence requirements and conservative side-effect reconciliation.
- Added persistent WorkGraph administration APIs under `/v1/workgraphs`.
- Added authority-bound Cluster submission, lease and completion checks. Authority is revalidated before a Worker receives a lease and before a Worker result becomes terminal success.
- If authority changes while remote work is executing, the reported result is retained and the Job moves to `reconciliation` instead of being accepted as completed.
- Added authority-aware reconciliation: an administrator may record an already-observed external result, but `requeue` requires currently effective execution authority.
- Added native Cluster `held` Jobs for race-free orchestration. Held Jobs are durable but cannot be leased until the corresponding WorkGraph node is durably running and the activation transition is audited.
- Added durable `internal/orchestration` bindings linking WorkGraph node, Cluster Job and resolved Authority without accepting a client/model-selected authority identifier.
- Added restart recovery for held/active/reconciling bindings and fail-closed cancellation of orphaned held jobs.
- Added completion evidence propagation from Cluster Job results into WorkGraph nodes using Job identity and result SHA-256.
- Added Admin-only orchestration APIs under `/v1/orchestration/`; model-facing tools do not receive orchestration approval, dispatch, completion or reconciliation authority.
- Added Clean-Room originality/IP policy, change provenance records and third-party dependency inventory checks. The implementation is developed from KING requirements and public standards rather than copied agent-framework source or product internals.
- Updated the production baseline from Go 1.26.5 to Go **1.26.6** and fixed provider metadata persistence, approval denial/rollback semantics, hard-bounded root rate-limit state, release packaging nounset handling and Go 1.26 `ServeMux` console startup compatibility.
- Removed obsolete development-only patch/format/once workflows so formal CI, CodeQL and v1.3 validation/release workflows are the authoritative verification paths.

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

本轮 v1.3 继续新增 Authority / WorkGraph / Orchestration 安全执行链：

- 新增 KINGAIBOT 原创 Capability Envelope 持久授权体系，可限制 Capability、数据范围、工具范围、预算、过期时间与委派深度。
- 子授权只能缩小父授权；父授权吊销或过期后，所有后代授权立即失效。
- 新增可信 Agent → Task 授权绑定：平台创建任务时从可信 `agent_id` 解析授权，模型工具参数不能携带或自行选择 `authority_id`。
- 新增持久化 Typed WorkGraph DAG，支持审批门、Replay Policy、高风险证据要求与保守的 Side Effect Reconciliation。
- 新增 `/v1/workgraphs` Admin 管理 API。
- Cluster 在 Job 提交、Worker Lease 发放、Worker Result 提交三个阶段都会重新校验执行权限。
- 如果远程执行过程中权限被撤销或失效，Worker 返回结果会被保留为证据，Job 进入 `reconciliation`，不会直接记为完成。
- Admin 可以在核验真实外部状态后记录 reconciliation 结果，但 `requeue` 会再次检查原始执行授权，撤权后不能借人工 reconciliation 重新启动执行。
- 新增 Cluster `held` 状态实现无竞态派发：Held Job 已持久化但 Worker 永远租不到，直到对应 WorkGraph Node 已经持久进入 Running 且 activation 审计完成。
- 新增 `internal/orchestration` 持久 Binding，把 WorkGraph Node、Cluster Job 和已解析 Authority 绑定在一起，不接受客户端/模型自报权限。
- 新增服务重启后的 held / active / reconciling Binding 恢复，并对没有持久 Binding 的 orphan held Job Fail-Closed 取消。
- Cluster 完成后把 Job ID 与 Result SHA-256 作为 Evidence 回写 WorkGraph，高风险节点不能无证据完成。
- 新增 `/v1/orchestration/` Admin-only API；模型不获得编排审批、派发、完成或 reconciliation 权限。
- 新增 Clean-Room 原创/IP 规范、变更来源记录与第三方依赖清单门禁，核心实现来自 KING 自有需求与公开标准，不复制第三方 Agent Framework 源码或产品内部实现。
- 生产 Go 基线升级到 **1.26.6**，同时修复 Provider 元数据持久化、审批拒绝/审计失败回滚、Root Rate Limiter 硬上限、发布脚本 `set -u` 变量问题，以及 Go 1.26 `ServeMux` 导致的 Console 启动冲突。
- 清理开发期 `patch / fix / once / format` 临时工作流，正式 CI、CodeQL、v1.3 Validation 与 Release 流程成为唯一可信验证路径。

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
