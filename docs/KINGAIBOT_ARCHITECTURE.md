# KINGAIBOT Original Architecture

Status: Architecture baseline for KINGAIBOT v1.3+
Owner: USDX TECH LLC / KING AI

## 1. Positioning

KINGAIBOT is a security-first, durable, multi-agent execution platform and controlled terminal execution layer for the wider KING AI system.

It is not a clone of OpenClaw or any other agent framework. External products may inform capability discovery, but KINGAIBOT requirements, data models, state machines, security invariants, code, tests and product language are independently designed.

The central execution invariant is:

```text
intent -> policy -> exact approval -> audit -> execution -> evidence -> reconciliation
```

Models are replaceable reasoning resources. They do not own identity, permissions, durable work state or governance.

## 2. KINGAIBOT-native layers

### Layer A — Agent Identity

Defines stable platform identities for agents, operators, automations, services and workers.

Identity is platform data and is never inferred from model output.

### Layer B — Capability Envelope

Defines the maximum authority available to an agent, mission, workflow or worker:
- tool scopes
- data scopes
- capability scopes
- concurrency limits
- cost/budget ceilings
- expiration
- delegation depth

Delegation may only narrow authority. A child capability envelope can never widen itself beyond its parent.

### Layer C — KING Cognition Kernel

Model-agnostic bounded reasoning loop:

1. Observe
2. Frame
3. Retrieve
4. Plan
5. Verify
6. Act
7. Reconcile
8. Learn

Verify and Reconcile are platform-owned gates and cannot be skipped by model output.

### Layer D — WorkGraph Engine

Represents durable work as a typed dependency graph rather than a transient prompt chain.

Node types may include:
- Think
- Read
- Transform
- Decide
- Approve
- Execute
- Wait
- Delegate
- Verify
- Reconcile
- Report

Each node owns replay policy, risk, dependencies, evidence requirements and lifecycle state.

Unknown or ambiguous side effects default to reconciliation rather than blind retry.

### Layer E — Memory Fabric

Separates different trust classes:
- working context
- episodic runtime memory
- reviewed long-term knowledge
- relationships/entity links

Generated text is not automatically trusted knowledge.

Trust progression follows explicit policy-controlled states instead of model self-approval.

### Layer F — KING Skill Runtime

A KING skill is a bounded capability package with:
- manifest
- semantic version
- declared permissions
- input/output schema
- integrity hash
- compatibility metadata
- tests
- publisher/source metadata

Skills execute through policy boundaries; descriptions cannot grant privilege.

### Layer G — Agent Mesh

Coordinates agents and heterogeneous workers while keeping internal authority and memory private.

KING-native protocol objects include:
- AgentCard
- WorkEnvelope
- CapabilityClaim
- LeaseGrant
- CompletionEvidence

External A2A compatibility maps onto this boundary without replacing the KING internal model.

### Layer H — Execution Plane

Execution is isolated from cognition.

Worker classes can include:
- server worker
- browser worker
- desktop worker
- mobile worker
- cloud worker
- data worker
- code sandbox worker

Workers receive the minimum required payload and short-lived authority. They do not receive unrestricted platform credentials or unrelated memory.

### Layer I — Reconciliation and Evidence

A tool returning success is not always sufficient proof that real-world state is correct.

Important side effects use:
1. intended state
2. execution result
3. evidence
4. observed state
5. reconciliation decision

Evidence may contain IDs, hashes, API receipts, timestamps or other verifiable artifacts.

### Layer J — Governance Plane

Governance stays outside model control.

Controls include:
- deny-by-default policy
- scoped identities
- exact approvals
- capability envelopes
- budget ceilings
- data classification
- egress controls
- append-only audit
- replay rules
- emergency disable
- signed releases
- SBOM
- vulnerability and dependency scanning

### Layer K — Evolution Lab

KINGAIBOT may propose improvements but may not silently rewrite production authority or deploy itself.

Controlled evolution flow:
1. detect failure/opportunity
2. create proposal
3. generate candidate in isolated branch/sandbox
4. run tests/security/evaluations
5. compare with baseline
6. require policy-defined approval
7. release through signed deployment
8. observe health and rollback signals

## 3. Compatibility boundaries

### MCP

MCP is an external tool/context compatibility adapter. External calls are mapped into KING tool identities and policy before execution.

### A2A

A2A is an external agent interoperability adapter. External agents remain opaque peers and do not gain implicit access to KING memory, tools or authority.

### Model providers

OpenAI-compatible, Anthropic, Gemini, local and future providers connect through a stable provider interface. Replacing a model must not rewrite durable platform state.

## 4. Existing v1.3 implementation mapping

Current KINGAIBOT components already map to this architecture:
- platform identities/API keys -> Agent Identity
- tool policy/approval/audit -> Governance Plane
- durable sessions/schedules/workflows/missions -> WorkGraph foundations
- knowledge review -> Memory Fabric trust boundary
- skills/plugins/channels -> Skill Runtime and adapters
- cluster workers/jobs/leases -> Agent Mesh + Execution Plane
- lease reconciliation -> Reconciliation Engine foundation
- proposal-only evolution -> Evolution Lab
- MCP/A2A/provider APIs -> compatibility adapters

## 5. Originality requirements

For each new core subsystem:
- create a neutral KING requirement
- document security invariants
- design a KING-native contract/state machine
- implement from requirements and approved public standards
- write original tests from invariants/abuse cases
- record dependencies and licenses
- preserve commit history and provenance

Third-party source code, prompts, UI, documentation text or distinctive implementation structure must not be copied into KINGAIBOT.

## 6. Near-term implementation priorities

1. Capability Envelope enforcement throughout agent/workflow/worker execution
2. durable typed WorkGraph
3. Evidence Ledger and reconciliation receipts
4. internal WorkEnvelope/LeaseGrant protocol consolidation
5. model-independent Cognition Kernel boundaries
6. policy-driven skill manifest/signature verification
7. current MCP/A2A adapters without leaking their architecture into KING internals
8. private/local deployment hardening and operator console

## 7. Core rule

**KINGAIBOT owns identity, authority, work, memory, evidence and governance. Models and external protocols are replaceable adapters.**
