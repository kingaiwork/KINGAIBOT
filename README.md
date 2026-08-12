# KINGAIBOT

**A future-facing, secure, durable and extensible intelligent-agent execution system from [kingai.work](https://kingai.work).**

Official website: **https://kingai.work**  
Contact: **vip@kingai.work**

> KINGAIBOT is a long-term R&D project of kingai.work. It is being developed as the future **terminal execution layer of the existing KING AI intelligent-lifeform system**. KINGAIBOT is developed and validated as an independent system today, while its long-term destination is controlled integration into the KING AI main system.

## English

### Project Positioning

KINGAIBOT is not a rename of the existing KING AI intelligent-lifeform system and it does not replace the current main system. It is a separate execution-oriented agent runtime designed to become the trusted terminal execution layer for KING AI.

Its responsibility is to turn high-level intelligence into controlled real-world digital execution across devices, APIs, tools, workflows, services and cooperating agents.

Long-term architecture:

```text
KING AI Intelligent Lifeform
        ↓
Reasoning / Memory / Governance / Mission Intelligence
        ↓
Controlled Integration Boundary
        ↓
KINGAIBOT Execution Layer
        ↓
Devices / APIs / Tools / MCP / A2A / Workflows / Services
```

### Current v1.1.0 Foundation

- Cross-platform Go runtime for Linux, macOS and Windows
- API-key-based model providers with model-vendor independence
- Durable task persistence and restart recovery
- Approval-aware tool execution with `allow / ask / deny`
- Filesystem sandbox and symlink/path traversal defense
- HTTPS tool with SSRF and DNS-rebinding protections
- Shell disabled by default and restricted to explicit executable allowlists
- Separate Admin / MCP / A2A identities
- MCP server and remote MCP bridge
- A2A Agent Card and remote agent bridge
- Hash-chained audit/event log with integrity verification
- Controlled evolution proposals rather than uncontrolled self-modification
- Safe update pipeline with checksums, signature policy and rollback
- GitHub CI, CodeQL, vulnerability gates, SBOM, provenance and Sigstore-oriented release flow

### Design Principles

1. **Model-independent** — models are replaceable reasoning resources, not the operating system.
2. **Execution is policy-controlled** — intelligence never automatically implies unrestricted privilege.
3. **Fail closed for dangerous operations** — missing audit, invalid approval or integrity failure blocks side effects.
4. **Durability before autonomy** — work must survive restart and ambiguous actions must not be blindly replayed.
5. **Learning without uncontrolled self-modification** — evolution is proposed, tested, reviewed, staged and reversible.
6. **Open interoperability** — MCP, A2A and future standards are integration layers, not vendor lock-in.
7. **Long-term convergence with KING AI** — independent development now, controlled integration into the main intelligent-lifeform system later.

### Documentation

- [Documentation Index](docs/README.md)
- [Product Definition](docs/PRODUCT.md)
- [User & Operations Guide](docs/USAGE.md)
- [Long-Term Roadmap](docs/ROADMAP.md)
- [Future Technology Integration Plan](docs/FUTURE-TECHNOLOGY.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](docs/SECURITY.md)
- [Controlled Evolution](docs/EVOLUTION.md)
- [Protocols](docs/PROTOCOLS.md)
- [Deployment](docs/DEPLOYMENT.md)
- [Validation](docs/VALIDATION.md)
- [Residual Risks](docs/RESIDUAL-RISKS.md)
- [Support](docs/SUPPORT.md)

### Quick Start

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

If a requested tool is configured as `ask`, review and approve it explicitly:

```bash
go run ./cmd/kingagent approvals
go run ./cmd/kingagent approve appr_xxx
```

### Installation Repository

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

For commercial production, prefer a reviewed immutable release/tag rather than installing directly from a moving `main` branch.

---

# 中文

## 项目定位

**KINGAIBOT 是 kingai.work 面向未来长期开发的智能体执行系统。**

它不是现有 KING AI 智慧生命体系统的简单改名，也不是用来替代现有主系统。KINGAIBOT 将作为独立系统持续研发、测试、部署和演进，长期目标是成为 **KING AI 智慧生命体的终端执行层（Execution Layer）**。

它负责把 KING AI 上层的目标、任务、推理和治理要求，转化为对设备、API、工具、工作流、服务和其他智能体的安全、可控、可审计执行。

长期结构：

```text
KING AI 智慧生命体
        ↓
推理 / 记忆 / 治理 / Mission 智慧
        ↓
受控整合边界
        ↓
KINGAIBOT 终端执行层
        ↓
设备 / API / 工具 / MCP / A2A / 工作流 / 服务
```

### 当前 v1.1.0 基础能力

- Linux / macOS / Windows 跨平台 Go Runtime
- 基于 API Key 的多模型 Provider，模型厂商可替换
- Durable Task 持久任务与重启恢复
- `allow / ask / deny` 权限与审批机制
- 文件系统沙箱、路径穿越和符号链接防护
- HTTPS 工具、SSRF 与 DNS Rebinding 防护
- Shell 默认关闭，只允许显式白名单程序
- Admin / MCP / A2A 三套独立身份
- MCP Server 与远程 MCP Bridge
- A2A Agent Card 与远程 Agent Bridge
- SHA-256 前向哈希链审计日志与完整性验证
- 受控自进化提案机制，而不是生产代码无限自修改
- 校验、签名策略、健康检查和回滚式安全升级
- GitHub CI、CodeQL、漏洞检查、SBOM、构建溯源和 Sigstore 发布链

### 长期开发原则

1. **模型不是系统本身**：任何模型都可以替换。
2. **越智能不等于越高权限**：执行必须受策略控制。
3. **危险操作 Fail-Closed**：审计、审批、完整性异常时阻止副作用。
4. **先保证持久可靠，再扩大自治范围**。
5. **允许学习和进化，但不允许不可控自我修改**。
6. **持续融合开放标准**：MCP、A2A、WASM/WASI 与未来智能体协议。
7. **最终与 KING AI 主系统融合**：当前独立演进，成熟后通过受控接口成为主系统终端执行层。

### 官方信息

官网：**https://kingai.work**  
邮箱：**vip@kingai.work**  
GitHub：**kingaiwork/KINGAIBOT**

### 文档

- [文档总入口](docs/README.md)
- [产品定义](docs/PRODUCT.md)
- [使用与运维手册](docs/USAGE.md)
- [长期开发路线图](docs/ROADMAP.md)
- [未来前沿技术融合计划](docs/FUTURE-TECHNOLOGY.md)
- [架构](docs/ARCHITECTURE.md)
- [安全](docs/SECURITY.md)
- [受控进化](docs/EVOLUTION.md)
- [协议](docs/PROTOCOLS.md)
- [部署](docs/DEPLOYMENT.md)
- [验证记录](docs/VALIDATION.md)
- [剩余风险](docs/RESIDUAL-RISKS.md)
- [支持](docs/SUPPORT.md)

> KINGAIBOT 是长期工程。仓库中的版本代表当前经过验证的阶段能力，而不是“永远终极”或“绝对零 Bug”的声明。所有高权限能力都应遵循最小权限、可审计、可回滚和人工接管原则。
