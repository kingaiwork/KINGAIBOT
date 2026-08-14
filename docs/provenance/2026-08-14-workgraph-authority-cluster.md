# KINGAIBOT Provenance Record — WorkGraph, Capability Envelope and Authority-Bound Cluster

Date: 2026-08-14
Product: KINGAIBOT
Owner: USDX TECH LLC / KING AI
Development method: Clean-room original implementation

## 1. KING requirement

KINGAIBOT requires durable autonomous work that remains controlled when model providers, processes, machines or workers change. The execution layer must not infer authority from model text, and a remote side effect must not become trusted completion merely because a worker reports success.

The internal requirements for this change are:

- represent multi-step work as durable typed state rather than transient prompt context;
- require explicit approval for nodes configured to need approval;
- require evidence for high-risk/critical completion;
- place ambiguous side effects into reconciliation instead of blind replay;
- represent execution authority as an explicit durable envelope outside the model;
- delegation may narrow authority but must never widen it;
- revocation or expiry of a parent authority invalidates descendants;
- remote jobs must be authority-checked before submission, before lease delivery, and before result commit;
- authority loss during a remote side effect must move the result to reconciliation rather than completed;
- every trust-changing transition must remain auditable and fail closed when audit persistence fails.

## 2. Independent KING design

KING-native components created from the above requirements:

- `internal/authority/Envelope`
- durable `authority/Grant` registry
- parent-linked authority delegation and transitive effectiveness checks
- `internal/workgraph/Graph` and typed work-node state machine
- durable audited WorkGraph store and admin API
- `cluster/JobAuthorityBinding`
- authority-aware submit / lease / completion boundaries
- reconciliation on mid-flight authority loss

The design deliberately keeps model reasoning, durable work state, execution authority and remote worker leasing as separate trust domains.

## 3. External reference boundary

No third-party agent-framework source code, proprietary prompt, UI, internal class hierarchy, directory layout, configuration schema or test suite was used as an implementation source for these modules.

General public engineering concepts used are non-product-specific concepts such as:

- directed dependency graphs;
- capability-based authorization;
- least privilege;
- expiring/revocable grants;
- leases;
- idempotency/replay safety;
- two-phase outcome verification/reconciliation;
- append-only audit evidence.

MCP and A2A remain external interoperability adapters and do not define these internal KINGAIBOT objects.

## 4. Original security invariants

The tests are derived from KINGAIBOT security requirements rather than third-party test cases. Covered invariants include:

- dependency cycles are rejected;
- critical WorkGraph nodes cannot complete without evidence;
- approval-gated nodes cannot start without approval;
- ambiguous side effects enter reconciliation;
- delegated authority cannot exceed parent capabilities, data scopes, tools, budget or delegation depth;
- revoked parent authority invalidates descendants;
- expired authority is ineffective;
- an authority-bound cluster job cannot be submitted without a trusted authority identifier;
- authority revoked before leasing prevents work disclosure to the worker;
- authority revoked during execution prevents direct completion and retains the result for reconciliation.

## 5. Repository implementation evidence

Primary files introduced or materially changed during this development sequence include:

- `internal/authority/envelope.go`
- `internal/authority/store.go`
- `internal/authority/http.go`
- `internal/authority/envelope_test.go`
- `internal/authority/store_test.go`
- `internal/workgraph/graph.go`
- `internal/workgraph/store.go`
- `internal/workgraph/http.go`
- `internal/workgraph/graph_test.go`
- `internal/workgraph/store_test.go`
- `internal/cluster/authority_guard.go`
- `internal/cluster/authority_guard_test.go`
- `internal/cluster/http.go`
- `internal/cluster/extension.go`
- `cmd/kingagentd/main.go`

The Git history on `agent/v1.3-platform` is the authoritative authorship and chronology record.

## 6. IP classification

- KING-native source and documentation: proprietary KINGAIBOT material subject to the repository commercial license.
- Go standard library: external platform dependency under its own license.
- Open protocols/standards: compatibility interfaces only; not claimed as KING-owned.
- Third-party actions, tools and build dependencies: governed by their own licenses and tracked separately from KING-original code.

## 7. Review rule

Future changes to these modules must continue to satisfy `docs/ORIGINALITY_IP_POLICY.md`. Any external implementation code intentionally introduced later must be treated as a third-party component, license-reviewed, attributed where required, and must not be relabeled as KING-original source.
