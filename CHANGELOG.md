# Changelog

## 1.4.0 - 2026-08-14

### English

Governance, crash-safety and replay-safety release over the v1.3 durable platform:

- Added directional, audit-gated Capability Envelope lifecycle and fail-closed revocation semantics.
- Enforced hierarchical `MaxConcurrentWork` and `MaxCostUnits` budgets across parent/child authority trees, including sibling sharing, durable reservations, idempotent cost charging and advisory preflight visibility.
- Hardened Cluster Worker identity and Job lifecycle with audit-before-enable, audit-before-queue, `held` orchestration handoff, lease-time authority rechecks and reconciliation when authority changes during execution.
- Hardened WorkGraph transitions so trust-expanding/success transitions are audit-first while fail-closed/reconciliation transitions persist the safer state first.
- Added crash-safe platform identities and scoped access keys with staged activation, fail-closed disable/revoke and path-level `platform.read`, `platform.write`, `platform.automation` and `platform.admin` enforcement.
- Switched production Platform entry points to the V14 safe control surface and V14 model extension.
- Added durable Session submission semantics so a successfully-created Runtime Task is never misreported as safe to retry merely because derived Session state could not be synchronized.
- Added durable inbound webhook receipts (`processing`, `task_created`, `accepted`, `reconciliation`, `failed`), `event_id` deduplication, conservative ambiguous-window handling and Admin-only reconciliation.
- Added safe long-term Knowledge proposal/review flow using `pending_audit -> proposed -> approved/rejected`; model tools may propose but cannot approve trusted knowledge.
- Added Runtime `pending_audit` creation state and audit-before-queue behavior.
- Added deterministic/idempotent Runtime Task creation using hashed caller-owned idempotency keys and input conflict detection.
- Changed restart recovery so only `queued` Tasks are automatically re-enqueued; `pending_audit`, `running` and `completing` Tasks move to `reconciliation` instead of being blindly replayed.
- Added two-phase task completion: durable `completing` output -> `task.completed` audit with output SHA-256 -> `completed`; completion-audit failure preserves output and requires reconciliation.
- Added Admin-only Runtime reconciliation decisions: `accept_completed`, `mark_failed`, and explicit `retry` with `allow_replay=true`; replay is blocked when durable output exists.
- Added crash-safe staged approval lifecycle: `pending -> approving/denying -> audit -> approved/denied -> Task transition`.
- Routed both the compatibility approval endpoint and the explicit `/decision` endpoint through the staged V14 approval path in production.
- Made repeated final approval decisions idempotent so the same Task is not queued twice.
- Added Workflow V14 with stable idempotent Task identity per step and crash recovery that reattaches to an already-created Task rather than creating a duplicate.
- Added Mission V14 with persist-before-dispatch behavior, stable child Task identities, dedicated V14 running state/synchronizer and child-reconciliation propagation to the whole Mission.
- Added v1.4 operational attention metrics for Runtime, Workflow, Mission and inbound reconciliation while keeping normal inbound `processing` out of reconciliation alerts.
- Kept Node trust directional: registration starts offline, only an audited heartbeat may promote online state, and status/list operations can demote stale nodes but never promote unaudited nodes.
- Fixed a CodeQL-reported allocation-size overflow pattern in Runtime metadata copying by removing unnecessary `len(meta)+2` capacity arithmetic.
- Expanded failure-injection and concurrency regression coverage for authority, budgets, approvals, Runtime creation/completion/restart, Sessions, inbound receipts, Workflow/Mission recovery, Knowledge review and reconciliation.
- Unified public/runtime identity to **KINGAIBOT 1.4.0** across server, worker, console, example configurations, containers and provider/worker/console User-Agent strings.
- Added `scripts/build-release-v14.sh` with reproducible six-target packages, four binaries, CycloneDX SBOM, SHA-256 checksums and Release Manifest under `dist-v14/`.
- Added V1.4 Full Validation, Container Smoke, Container Validation and Full Release Assets workflows using Go **1.26.6**.
- Removed superseded v1.3 current validation/release workflows and full-release builder from the active branch; historical versions remain available through Git history/tags.
- Preserved the Clean-Room originality/provenance policy: KING-owned internal architecture and data models, open standards at compatibility boundaries, dependency provenance inventory, SBOM and release attestations.

Security invariant for v1.4:

```text
intent -> trusted identity -> authority envelope -> hierarchical budget
       -> policy/approval -> audit -> execution -> evidence
       -> completion or reconciliation
```

### 中文

在 v1.3 持久化平台基础上的治理、崩溃安全与防重复执行版本：

- Capability Envelope 权限生命周期改为方向性可信状态，启用/扩权必须先完成审计，撤权/失效 Fail-Closed。
- `MaxConcurrentWork` 与 `MaxCostUnits` 正式成为运行时分层预算，子任务消耗同时计入祖先额度，兄弟委派共享父级上限，并支持持久 Reservation、幂等 Cost Charge 与只读 Preflight。
- Cluster Worker 身份与 Job 状态全面加固：审计后启用、审计后入队、`held` 无竞态交接、Lease 前重新校验权限、执行中撤权后进入 reconciliation。
- WorkGraph 可信扩张/成功状态采用 Audit-First，失败与 reconciliation 状态优先持久更安全的结果，避免审计失败反向恢复执行权限。
- Platform Identity / Access Key 使用分阶段激活、Fail-Closed 禁用/吊销，并正式执行 `platform.read / write / automation / admin` 路径级权限。
- 生产 daemon 切换到 V14 Safe Platform Handler 与 V14 model extension。
- Session 采用 Durable Submission：Runtime Task 已创建即视为执行事实，后续 Session 派生状态同步失败不会误导调用方安全重试。
- Inbound Gateway 使用持久 Receipt：`processing / task_created / accepted / reconciliation / failed`，支持 `event_id` 去重、模糊窗口 Fail-Closed 和 Admin-only reconciliation。
- 长期 Knowledge 使用 `pending_audit -> proposed -> approved/rejected` 信任链；模型只能 propose，不能自我批准可信知识。
- Runtime 新增 `pending_audit` 与 Audit-Before-Queue。
- 新增基于稳定 Key 的幂等 Runtime Task 创建，同 Key 同输入返回同一个 Task，不同输入直接冲突。
- 重启恢复改为：只有 `queued` 自动重新入队；`pending_audit / running / completing` 统一进入 reconciliation，绝不盲重放。
- 新增两阶段完成：输出先持久为 `completing`，再写 `task.completed` + 输出 SHA-256，最后才进入 `completed`；完成审计失败保留输出并进入 reconciliation。
- 新增 Runtime Admin reconciliation：`accept_completed / mark_failed / retry`；retry 必须显式 `allow_replay=true`，存在持久输出时禁止直接重放。
- 新增 Staged Approval：`pending -> approving/denying -> audit -> approved/denied -> Task transition`。
- 兼容审批 URL 和 `/decision` URL 在生产环境全部统一走 V14 staged approval，不再落回旧 persist-first 路径。
- 重复提交同一个最终 approved 决定不会重复把同一个 Task 放入执行队列。
- Workflow V14 为每一步提供稳定幂等 Task Identity，崩溃恢复时重新关联已创建 Task，不再产生重复 Step Task。
- Mission V14 先持久 Mission 再派发子任务，每个 Slot 使用稳定 Task Identity，并拥有独立 V14 running/synchronizer；子 Task reconciliation 会上卷到整个 Mission。
- Status / Metrics 新增 Runtime、Workflow、Mission、Inbound reconciliation 与 `attention_required`，普通 inbound `processing` 不再误触发告警。
- Node 注册默认 Offline，只有成功写入审计的 Heartbeat 才能上线；状态读取只能降级过期节点，不能提升未经审计的节点。
- 修复 CodeQL 指出的 Runtime metadata map 分配大小潜在整数溢出模式，移除不必要的 `len(meta)+2` 算术。
- 大幅扩展故障注入、并发与恢复回归测试，覆盖 Authority、Budget、Approval、Runtime 创建/完成/重启、Session、Inbound、Workflow/Mission、Knowledge 与 Reconciliation。
- Server、Worker、Console、示例配置、Docker 与 User-Agent 对外版本统一为 **1.4.0**。
- 新增 `scripts/build-release-v14.sh`，在 `dist-v14/` 生成四个二进制、六个平台目标、CycloneDX SBOM、SHA-256 与 Release Manifest。
- 新增 V1.4 Full Validation / Container Smoke / Container Validation / Full Release Assets 四条正式流水线，统一使用 Go **1.26.6**。
- 删除当前分支上已被 v1.4 替代的 v1.3 验证/发布入口；历史版本继续保留在 Git History/Tag。
- 继续执行 Clean-Room 原创与来源治理：KING 自有内部架构和数据模型，开放标准只用于边界兼容，第三方依赖有来源记录，发布带 SBOM/Provenance。

v1.4 安全不变量：

```text
意图 -> 可信身份 -> Capability Envelope -> 分层预算
     -> Policy/Approval -> Audit -> Execution -> Evidence
     -> Completion 或 Reconciliation
```

## 1.3.0 - 2026-08-14

### English

Platform expansion over the hardened v1.2 execution core:

- Added durable Agent Profiles, Sessions, schedules, sequential Workflows and parallel Missions.
- Added device/browser/edge Node registration, remote plugin manifests, Channel adapters and integrity-hashed Skills.
- Added a generic extension-tool boundary so platform actions pass the same policy, exact approval and audit pipeline as core tools.
- Added durable platform identities, scoped API keys and authenticated `/v1/platform/*` APIs.
- Added inbound channel authentication, sender allowlists and webhook deduplication.
- Added reviewed long-term Knowledge separate from episodic Runtime memory.
- Added Capability Envelopes, WorkGraph DAGs, authority-bound Cluster execution and Admin-only orchestration bindings.
- Added native Cluster `held` Jobs for race-free WorkGraph-to-Worker handoff and reconciliation on ambiguous side effects.
- Added controlled Evolution proposal/evaluation/review/stage/release/rollback primitives.
- Added Clean-Room originality/IP policy and third-party provenance checks.
- Updated production baseline to Go **1.26.6**.

### 中文

在 v1.2 安全执行核基础上完成平台化扩展：

- 新增 Agent Profile、Session、Schedule、Workflow、Mission 持久对象。
- 新增 Node、Plugin、Channel、Skill 与统一扩展工具边界。
- 新增 Platform Identity、Scoped API Key 与 `/v1/platform/*` 管理接口。
- 新增 Inbound Channel 鉴权、Sender 白名单与 Webhook 去重。
- 新增与 Episodic Memory 分离的 Reviewed Knowledge。
- 新增 Capability Envelope、WorkGraph、Authority-bound Cluster 与 Admin-only Orchestration。
- 新增 Cluster `held` Job，避免 WorkGraph 到 Worker 的派发竞态，并对模糊副作用使用 reconciliation。
- 新增受控 Evolution 生命周期与 Clean-Room 原创/来源治理。
- 生产 Go 基线升级到 **1.26.6**。

## 1.2.0 - 2026-08-12

Security-first execution-layer expansion:

- Replaced file tool execution paths with Go `os.Root` traversal-resistant operations.
- Added path/component bounds, regular-file-only reads and safe stat/list/write/mkdir/single-delete capabilities.
- Hardened HTTPS access, redirects, SSRF/DNS-rebinding controls and provider/MCP/A2A credential forwarding behavior.
- Kept shell execution denied by default and strengthened allowlist validation.
- Hardened signed/checksummed updates and cross-platform atomic storage.
- Added CodeQL, govulncheck, Race, SBOM, provenance and Sigstore-oriented release controls.

## 1.1.0 - 2026-08-12

Security/reliability hardening over the initial commercial baseline:

- Corrected MCP/A2A protocol behavior.
- Added exact argument-bound approvals and durable at-most-once tool execution state.
- Hardened cancellation, queue recovery, networking, shell allowlists, audit integrity, memory and updater behavior.
- Split Admin/MCP/A2A credentials and added immutable-action-pinned CI plus supply-chain verification.

## 1.0.0 - 2026-08-12

- Initial cross-platform commercial baseline.
