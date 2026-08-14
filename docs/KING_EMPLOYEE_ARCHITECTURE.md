# KING AI Digital Employee Architecture

Status: Original architecture baseline for KINGAIBOT v1.3+
Owner: USDX TECH LLC / KING AI

## 1. Originality rule

KINGAIBOT is not a clone of any existing agent product or framework. We may study public papers, standards, protocol specifications, security research, and operational lessons, but we do not copy third-party source code, proprietary prompts, private workflows, product UI, branding, or internal architecture.

Open standards such as MCP and A2A are compatibility boundaries only. They do not define KINGAIBOT's internal cognition, memory, scheduling, execution, governance, or employee model.

Every internal subsystem should have a KING-owned contract, data model, tests, threat model, and audit semantics.

## 2. Product model: a digital employee, not a chatbot

A KING digital employee is a durable software identity that owns responsibilities, context, work history, permissions, tools, schedules, collaborators, budgets, escalation rules, and measurable outcomes.

The primary unit is an Employee Runtime, not a prompt session.

Each employee has:
- Employee Identity: stable ID, role, team, authority, credentials and lifecycle.
- Responsibility Contract: goals, allowed scope, forbidden scope, service-level expectations and escalation rules.
- Work Ledger: append-only record of assignments, decisions, approvals, actions, outputs and reconciliation.
- Memory Fabric: episodic memory, reviewed knowledge, working context and retention policy.
- Skill Portfolio: signed and policy-gated capabilities, versioned independently from the employee.
- Relationship Graph: people, systems, customers, projects, agents and dependencies relevant to the employee.
- Operating Rhythm: schedules, triggers, recurring duties and follow-up obligations.
- Evaluation Profile: quality, latency, cost, reliability and human-feedback signals.

## 3. KING-native architecture

### Layer A — Employee Identity Plane

Purpose: define who the employee is and what authority it possesses.

KING-native concepts:
- EmployeeID: immutable runtime identity.
- ResponsibilityContract: machine-readable job scope.
- AuthorityEnvelope: explicit permissions, budgets and data boundaries.
- DelegationGrant: time-bounded authority that can be delegated to another employee or worker.
- EscalationRoute: deterministic path to a human or higher-authority employee.

Security invariant: identity and authority are never inferred from model output.

### Layer B — Cognition Kernel

Purpose: convert goals and observations into bounded decisions.

The Cognition Kernel is model-agnostic. Models are replaceable reasoning engines, not the system of record.

Stages:
1. Observe — normalize messages, events, schedules and system state.
2. Frame — bind the observation to employee responsibility and current work.
3. Retrieve — pull only policy-allowed memory and reviewed knowledge.
4. Plan — produce a typed WorkGraph proposal.
5. Verify — check policy, authority, budgets, preconditions and idempotency.
6. Act — dispatch local or remote execution through the Execution Plane.
7. Reconcile — compare intended and observed state before finalizing side effects.
8. Learn — create memory or knowledge proposals; never silently promote them to trusted truth.

Security invariant: the model cannot bypass Verify or Reconcile.

### Layer C — WorkGraph Engine

Purpose: represent real work as durable state instead of transient chain-of-thought.

A WorkGraph contains typed nodes such as:
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

Every node has inputs, outputs, owner, prerequisites, timeout, retry policy, replay safety and evidence.

Original design principle: retry semantics are attached to each work node. Unknown side effects default to manual reconciliation rather than automatic replay.

### Layer D — Memory Fabric

Four memory classes are intentionally separated:
- Working Context: short-lived task state.
- Episodic Memory: durable events and outcomes.
- Reviewed Knowledge: facts that passed trust review.
- Relationship Graph: entity links and operating context.

New knowledge follows a Trust Ladder:
1. observed
2. proposed
3. corroborated
4. approved
5. trusted
6. expired/revoked

A model may propose knowledge, but only policy or authorized review can move it upward.

### Layer E — Skill Runtime

A Skill is a signed capability package with:
- manifest
- semantic version
- declared permissions
- input/output schema
- deterministic policy name
- test vectors
- integrity hash
- publisher identity
- compatibility metadata

Skills are loaded through a policy boundary. A skill is never trusted because its description says it is safe.

### Layer F — Employee Mesh

Purpose: coordinate many KING employees and heterogeneous workers without sharing proprietary internals.

KING-native protocol objects:
- EmployeeCard: public capability summary.
- WorkEnvelope: delegatable unit of work.
- CapabilityClaim: worker-declared capability with server-side policy validation.
- LeaseGrant: short-lived right to process a WorkEnvelope.
- CompletionEvidence: result plus evidence and reconciliation metadata.

External A2A compatibility may map onto this layer, but Employee Mesh remains the authoritative internal model.

### Layer G — Execution Plane

Execution is isolated from cognition.

Worker classes can include:
- server worker
- browser worker
- desktop worker
- Android worker
- iOS-compatible remote worker
- cloud worker
- data worker
- code sandbox worker

Workers receive the minimum job payload and a short-lived lease. They never receive the employee's full memory, platform admin token or unrestricted credentials.

For side effects, completion is two-phase:
1. worker reports outcome/evidence
2. coordinator commits or moves the work into reconciliation

### Layer H — Governance Plane

Governance is always outside the model.

Controls:
- deny-by-default tool policy
- scoped identities and API keys
- exact approvals
- budget ceilings
- data classification
- egress control
- append-only audit
- replay policy
- human escalation
- emergency disable
- signed releases and SBOM
- vulnerability scanning

### Layer I — Evolution Lab

KING employees may improve the system, but production self-modification is prohibited.

Evolution pipeline:
1. detect recurring failure or inefficiency
2. create an Evolution Proposal
3. generate candidate code/config/prompt/skill in an isolated branch or sandbox
4. run unit, integration, security, regression and evaluation suites
5. compare against baseline
6. require policy-defined approval
7. release through normal signed deployment
8. observe rollback signals

The employee can invent; it cannot silently rewrite its own production authority.

## 4. KING differentiators

### Responsibility-first autonomy
Most agent systems begin with a conversation or task. KING begins with an employee responsibility contract and derives allowed work from it.

### Reconciliation-first side effects
Remote work is not considered complete merely because a tool returned success. High-impact work must reconcile desired state with observed state.

### Evidence-bearing work
Important WorkGraph nodes can carry evidence: API response hashes, file hashes, screenshots, IDs, timestamps or reviewer decisions. This creates a verifiable work history rather than a chat transcript.

### Trust-separated memory
Generated text, observed events and reviewed facts are different data classes. This reduces self-reinforcing hallucinated memory.

### Replaceable intelligence
Models, providers and external protocols are adapters. Employee identity, work state, memory, authority and governance remain KING-owned.

### Local/private operation
The same employee model should support a laptop, private server, VPS or distributed deployment. Cloud services are optional adapters rather than architectural dependencies.

## 5. Compatibility strategy

MCP: implement as an adapter at the Tool/Context boundary. Follow current protocol semantics for interoperability, but map every external tool to a KING policy identity before execution.

A2A: implement as an adapter at the Employee Mesh boundary. External agents remain opaque peers and never gain implicit access to KING internal memory or authority.

Provider APIs: expose a stable model adapter contract so OpenAI-compatible, local and future providers can be replaced without rewriting employee state.

## 6. Intellectual-property discipline

Development rules:
- Write original implementation from requirements and public specifications.
- Do not paste third-party implementation code into KINGAIBOT.
- Do not copy another product's UI pixel-for-pixel.
- Do not import proprietary prompts or leaked internal documents.
- Track third-party libraries and licenses explicitly.
- Prefer protocol conformance tests over borrowing reference implementation internals.
- Keep architecture decision records for major KING-native mechanisms.
- Keep public compatibility adapters thin; keep proprietary employee logic behind internal interfaces.

## 7. v1.3 implementation mapping

Existing v1.3 work maps into this architecture as follows:
- platform identities/API keys -> Employee Identity Plane
- durable profiles/sessions/schedules/workflows/missions -> Employee Runtime + WorkGraph
- reviewed knowledge store -> Memory Fabric Trust Ladder
- skills/plugins/channels -> Skill Runtime and adapters
- cluster workers/jobs/leases/reconciliation -> Execution Plane + Employee Mesh
- policy/approval/audit -> Governance Plane
- proposal-only evolution -> Evolution Lab

Next implementation priority:
1. ResponsibilityContract and AuthorityEnvelope data model
2. typed WorkGraph node engine
3. evidence objects and reconciliation receipts
4. EmployeeCard / WorkEnvelope internal protocol
5. delegation grants and budget envelopes
6. evaluation telemetry per employee and responsibility
7. compatibility adapters for current MCP and A2A releases
8. private deployment packaging and admin console
