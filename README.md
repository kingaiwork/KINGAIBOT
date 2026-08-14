# KINGAIBOT

**A secure, durable and extensible intelligent-agent execution system from [kingai.work](https://kingai.work).**

Official website: **https://kingai.work**  
Contact: **vip@kingai.work**

> KINGAIBOT is the controlled execution layer for the KING AI intelligent-lifeform system. In v1.3 it also acts as the customer-local private runtime for **KING AI Enterprise Workforce OS**, so enterprise digital employees can receive approved business tasks without turning the cloud control plane into a privileged remote shell.

## English

### Product role

```text
KING AI / Enterprise Workforce control plane
Accounts / Billing / License / Organization / Digital Employees
                 ↓
      Governed task + policy boundary
                 ↓
KINGAIBOT customer-local execution layer
Durable tasks / local approvals / tool policy / audit / MCP / A2A
                 ↓
Customer files / APIs / CRM / ERP / browsers / approved tools
```

KINGAIBOT converts high-level goals into controlled digital execution. The model is replaceable; local policy remains authoritative.

### Current v1.3 foundation

- Linux / macOS / Windows cross-platform Go runtime
- **Go 1.26.6** production source, CI, container and release security baseline
- API-key-based model providers with model-vendor independence
- Durable task persistence and restart recovery
- Approval-aware tool execution with `allow / ask / deny`
- Exact approvals bound to task + tool + canonical argument hash
- Go `os.Root` traversal-resistant filesystem sandbox
- Safe file read/stat/list/atomic-write/mkdir/single-delete capabilities
- HTTPS tooling with target allowlists, SSRF/DNS-rebinding defenses and redirect downgrade prevention
- Authenticated Provider / MCP / A2A / Enterprise Workforce requests do not automatically follow redirects
- Shell disabled by default and restricted to explicit bare executable allowlists when enabled
- Separate Admin / MCP / A2A identities
- MCP server + remote MCP bridge
- A2A Agent Card + remote agent bridge
- Hash-chained audit/event log with integrity verification
- Controlled evolution proposals instead of uncontrolled self-modification
- Safe update pipeline with checksums, signature policy, health verification and rollback
- GitHub CI, CodeQL, govulncheck, race tests, SBOM, provenance and Sigstore-oriented release flow
- Optional **KING AI Enterprise Workforce private-node bridge**

### Enterprise Workforce private runtime

When `KINGAI_WORKFORCE_NODE_TOKEN` is present, `kingagentd` automatically becomes a private digital-employee node:

1. Heartbeat to the KING AI control plane.
2. Sync active digital-employee and workflow policy.
3. Pull only cloud tasks that have passed cloud-side governance.
4. Convert each cloud task into a normal local durable KINGAIBOT task.
5. Re-apply local tool policy, sandbox and local approval requirements.
6. Execute permitted work through the normal agent runtime.
7. Report terminal status back to the control plane.

Cloud policy **cannot** grant arbitrary shell permission and **cannot** bypass local approval. Cloud employee skill lists describe business intent; they do not grant local operating-system capabilities.

By default, local AI output is **not uploaded** to the cloud. Set `KINGAI_WORKFORCE_REPORT_OUTPUT=true` only when the enterprise data policy explicitly permits bounded result reporting.

### Design principles

1. **Model-independent** — models are replaceable reasoning resources, not the operating system.
2. **Local execution policy wins** — cloud intent never implies unrestricted privilege.
3. **Fail closed for dangerous operations** — missing audit, invalid approval or integrity failure blocks side effects.
4. **Durability before autonomy** — work survives restart and ambiguous actions are not blindly replayed.
5. **Private by default** — customer credentials, files, memory and normal task output stay on the customer runtime unless an approved integration requires transmission.
6. **Learning without uncontrolled self-modification** — evolution is proposed, tested, reviewed, staged and reversible.
7. **Open interoperability** — MCP, A2A and future standards are integration layers rather than vendor lock-in.

### Documentation

- [Enterprise Workforce Private Node](docs/ENTERPRISE-WORKFORCE-NODE.md)
- [Documentation Index](docs/README.md)
- [Product Definition](docs/PRODUCT.md)
- [User & Operations Guide](docs/USAGE.md)
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

### Standalone quick start

```bash
cp configs/config.example.json config.json
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="YOUR_KEY"
go run ./cmd/kingagentd -config ./config.json
```

Run a task from another terminal:

```bash
export KINGAGENT_ADMIN_TOKEN="...same admin token..."
go run ./cmd/kingagent run "Create hello.txt in the workspace with a short greeting"
```

For a private Enterprise Workforce node, additionally set the one-time node token issued in the KING AI Enterprise console:

```bash
export KINGAI_WORKFORCE_NODE_TOKEN='knode_...'
export KINGAI_WORKFORCE_URL='https://api.kingai.work'
go run ./cmd/kingagentd -config ./config.json
```

If a requested local tool is configured as `ask`, the normal local approval flow still applies:

```bash
go run ./cmd/kingagent approvals
go run ./cmd/kingagent approve appr_xxx
```

### Installation repository

Repository: **kingaiwork/KINGAIBOT**

Linux bootstrap:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

macOS bootstrap:

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

Windows PowerShell bootstrap:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'; $env:KINGAGENT_REQUIRE_SIGNATURE='1'; irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

For commercial production, use a reviewed immutable release/tag rather than a moving `main` branch.

---

# 中文

## 项目定位

**KINGAIBOT 是 KING AI 的安全终端执行层，也是 KING AI Enterprise Workforce OS 的企业私有数字员工运行节点。**

它负责把 KING AI 上层的目标、任务和治理要求，转化为对设备、API、文件、工具、工作流、MCP、A2A 与企业系统的安全、可控、可审计执行。

```text
KING AI / 企业数字员工云端控制面
账号 / 购买 / License / 企业 / 数字员工 / 审批
                 ↓
          受控任务与策略边界
                 ↓
KINGAIBOT 企业本地私有运行层
持久任务 / 本地审批 / 工具权限 / 审计 / MCP / A2A
                 ↓
企业文件 / CRM / ERP / API / 浏览器 / 已批准工具
```

### 当前 v1.3 核心能力

- Linux / macOS / Windows 跨平台 Go Runtime
- **Go 1.26.6** 生产源码、CI、Docker 与正式 Release 安全基线
- 基于 API Key 的多模型 Provider，模型厂商可替换
- Durable Task 持久任务与重启恢复
- `allow / ask / deny` 本地权限与审批机制
- 审批绑定“任务 + 工具 + 规范化参数哈希”
- 基于 Go `os.Root` 的抗路径穿越文件系统沙箱
- HTTPS 目标白名单、SSRF/DNS Rebinding 与重定向防护
- Shell 默认关闭；启用时也只能使用管理员明确配置的命令
- Admin / MCP / A2A 独立身份
- MCP Server / MCP Bridge / A2A Bridge
- SHA-256 前向哈希链审计日志
- 受控自进化提案，而不是生产环境无限自修改
- 校验、签名、健康检查与回滚式安全升级
- GitHub CI、CodeQL、govulncheck、Race、SBOM、构建溯源、Sigstore 发布链
- **企业数字员工私有节点桥接能力**

### 企业数字员工自动运行

企业在 `kingai.work` 创建数字员工并注册私有节点后，只需在客户机器保存一次性节点令牌和 AI Provider API Key。`kingagentd` 启动后会自动：

1. 上报节点健康状态；
2. 同步企业数字员工政策；
3. 拉取已经通过云端治理的任务；
4. 将任务转换成普通 KINGAIBOT 本地持久任务；
5. 再次执行本地 `allow / ask / deny`、沙箱和审批；
6. 通过已批准工具完成业务；
7. 向云端回传终态。

**云端永远不能绕过本地审批，也不能给自己获得任意 Shell 权限。**

默认 `KINGAI_WORKFORCE_REPORT_OUTPUT=false`：云端只获取任务成功/失败等运营状态，不上传本地 AI 完整输出。只有企业明确允许时才开启受长度限制的结果回传。

### 开发原则

1. **模型不是系统本身**：可以随时替换模型。
2. **本地安全策略优先**：云端任务不等于系统权限。
3. **危险操作 Fail-Closed**：审批、审计或完整性异常时阻止副作用。
4. **先保证持久可靠，再扩大自治范围**。
5. **企业数据默认留在企业私有环境**。
6. **允许学习和进化，但不允许不可控自我修改**。
7. **持续融合 MCP、A2A 与未来开放智能体协议**。

### 文档

- [企业数字员工私有运行节点](docs/ENTERPRISE-WORKFORCE-NODE.md)
- [文档总入口](docs/README.md)
- [产品定义](docs/PRODUCT.md)
- [使用与运维手册](docs/USAGE.md)
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

> KINGAIBOT 是长期工程。所有高权限能力持续遵循最小权限、可审计、可回滚和人工接管原则。
