# KING AI Clean-Room Originality & IP Policy

Status: Mandatory engineering policy for KINGAIBOT and KING AI Digital Employee OS
Owner: USDX TECH LLC / KING AI

## 1. Objective

KING AI is an original digital-employee system. We may learn from public technology trends, public standards, public product capabilities, academic papers, security research and documented operational lessons, but KING AI must not be implemented as a clone of OpenClaw or any other agent framework.

The goal is independent creation: understand the problem, write our own requirements, design our own architecture, implement our own code, and maintain evidence showing how each important subsystem was created.

## 2. OpenClaw reference boundary

OpenClaw may be studied only at the capability level, for example:
- long-running agents
- tools
- skills
- channels
- memory
- automation
- multi-device execution
- sandboxed execution
- agent collaboration

These are problem categories, not implementation instructions.

KING developers MUST NOT copy or substantially imitate:
- OpenClaw source code
- file/folder layout
- internal class or function structure
- private or proprietary prompts
- configuration schemas unless they are an open external standard
- UI layouts, artwork, icons, branding or wording
- documentation text
- example workflows in a way that reproduces their expressive implementation
- undocumented/private behavior obtained from leaks or reverse engineering

When studying another product, developers should first write a neutral capability requirement in KING terminology, then design the implementation without looking at third-party source code.

## 3. Clean-room development workflow

For every major KING-native subsystem:

1. **Capability statement**
   - Describe the user/business problem without third-party implementation terminology.

2. **KING requirements**
   - Define inputs, outputs, invariants, security requirements, failure modes, durability and audit semantics.

3. **Independent design**
   - Create a KING-native data model, API contract, state machine and threat model.

4. **Original implementation**
   - Write code from the KING requirements and approved public standards only.

5. **Original tests**
   - Build tests from KING invariants and abuse cases, not by copying another project's test suite.

6. **Provenance record**
   - Record author/agent, requirement or ADR, commit, third-party dependencies and any standards used.

7. **IP review gate**
   - Run license, dependency, secret and provenance checks before release.

## 4. Standards are compatibility boundaries

Open standards may be implemented for interoperability, but they do not define KING internals.

Examples:
- MCP is an external Tool/Context adapter.
- A2A is an external Agent-to-Agent adapter.
- OpenAI-compatible APIs are model-provider adapters.
- OAuth/OIDC, HTTP, JSON Schema and similar standards are infrastructure interfaces.

All external protocol messages must be mapped into KING-owned internal objects before they can affect employee identity, authority, work, memory, execution or governance.

## 5. KING-owned core concepts

The following concepts are part of KING's original architecture and should remain independent of external frameworks:

- Employee Identity Plane
- Responsibility Contract
- Authority Envelope
- KING Cognition Kernel
- WorkGraph Engine
- Memory Fabric
- Trust Ladder
- KING Skill Runtime
- Employee Mesh
- WorkEnvelope
- LeaseGrant
- Reconciliation Engine
- Evidence Ledger
- Evolution Lab
- Governance Plane

External frameworks must never become the system of record for these objects.

## 6. Code provenance requirements

Each production change should be attributable to one or more of:
- a KING requirement
- an Architecture Decision Record (ADR)
- a public standard/specification
- a documented bug or security finding
- an internally designed feature

Do not paste code from blogs, repositories, Q&A sites, generated snippets with uncertain provenance, or other products unless the license and intended use are explicitly reviewed and the code is intentionally accepted as a third-party dependency/component.

Whenever practical, prefer writing a small original implementation rather than copying an implementation example.

## 7. Third-party dependency policy

Third-party libraries are allowed when intentionally selected and license-compatible.

Requirements:
- maintain an SBOM
- record package name, version and license/SPDX identifier
- retain required notices and attribution
- pin or lock production dependency versions
- scan dependencies for vulnerabilities
- review transitive dependencies
- avoid unknown/custom licenses without review
- do not remove copyright or license notices from third-party code

Permissive licensing does not make third-party code 'KING original'. Such code remains a dependency and must be identified as such.

## 8. UI, branding and documentation originality

KING AI must have its own visual identity and interaction model.

Do not reproduce another product pixel-for-pixel or reuse its:
- logos
- icons without a valid license
- illustrations
- screenshots
- page copy
- onboarding wording
- distinctive navigation structure
- proprietary naming

Product wording should use KING-native concepts and explain the system in our own language.

## 9. AI-generated code/content policy

AI assistance is permitted, but generated output is not automatically assumed safe or original.

Before production use:
- review generated code for suspiciously recognizable third-party fragments
- do not ask a model to reproduce a named project's implementation
- prompt from KING requirements rather than "copy X"
- require tests and security review
- keep human/company-authored architecture decisions as the source of truth

For critical modules, maintain design notes that show independent creation from requirements.

## 10. Copyright discipline

KING's software should be independently written and fixed in source-control history.

Keep:
- Git commit history
- authorship records
- architecture documents
- ADRs
- test history
- release artifacts
- dependency notices

These records help demonstrate independent development and distinguish KING's expression from third-party code.

Ideas, general capabilities and methods may inspire product requirements, but their particular third-party expression must not be copied.

## 11. Patent discipline

Copyright clearance and patent clearance are different activities.

Before commercial release of a materially novel or high-value mechanism:
- describe the mechanism in neutral technical terms
- search relevant published patents/applications
- record potentially relevant claims
- change the design where appropriate
- obtain qualified patent counsel for a formal freedom-to-operate review when business risk justifies it

A clean copyright history does not guarantee freedom from patent claims.

## 12. Feature intake rule

When someone requests "make KING work like Product X", convert it to:

> What capability or user outcome is needed?

Then create a KING-native requirement and implementation.

Bad requirement:
- "Copy OpenClaw skill system."

Good requirement:
- "KING employees need versioned, signed capabilities with declared permissions, deterministic schemas, policy gating, integrity verification and rollback."

## 13. Release checklist

A KING release is not IP-ready unless:
- [ ] new core code has an identifiable KING requirement/ADR
- [ ] no copied third-party UI/text/source is known to be present
- [ ] dependencies and licenses are inventoried
- [ ] required notices are retained
- [ ] SBOM is generated
- [ ] security and dependency scans pass
- [ ] generated code has been reviewed
- [ ] high-risk novel mechanisms have an IP/patent review note
- [ ] release artifacts correspond to reviewed source commits

## 14. Engineering rule of thumb

**Learn the problem. Learn the standard. Design the KING solution. Never clone the implementation.**

OpenClaw and other systems can show that a capability is useful. They do not define how KING AI should build it.
