# Learning, Self-Healing and Controlled Evolution

KINGAIBOT separates three mechanisms so “self evolution” does not become uncontrolled self-modification.

## 1. Runtime self-healing

Automatic recovery is allowed where the outcome can be made conservative and deterministic:

- queued/running durable tasks are recovered after service restart,
- worker queues apply bounded backpressure instead of spawning unbounded recovery goroutines,
- cancellation propagates to the active task context and terminal canceled state cannot be overwritten,
- transient provider errors use bounded retry, provider fallback and a circuit breaker,
- audit integrity is reverified periodically and unsafe side effects fail closed if integrity is lost,
- service managers restart a crashed runtime,
- signed updates activate only after a health check and roll back on failure.

An ambiguous external side effect is **not** “self-healed” by guessing/replaying it. The approval remains in a reconciliation-required execution state.

## 2. Learning

Successful task outputs can be retained as bounded episodic memory when enabled. Long-term raw task-input storage is disabled by default. Learning records are secret-redacted, size/count bounded, can expire and are treated as untrusted context when retrieved.

The runtime does not convert a single model output into trusted policy, code or permissions. Learning can influence future context but cannot raise its own authority.

## 3. Controlled evolution

Failures can produce durable review-only improvement proposals:

```text
Observed failure
  -> sanitized durable evidence
  -> improvement proposal
  -> operator/developer review
  -> branch/patch
  -> tests + security analysis
  -> signed/attested release
  -> staged update
  -> health check
  -> rollback if unhealthy
```

The production runtime never edits/replaces its own core just because a model requested it. Code, policy, release identity and deployment approval remain outside the model trust boundary.

Current proposals are stored in `data/evolution/` and exposed through `GET /v1/evolution/proposals`.

Future controlled-evolution layers can add evaluation suites, shadow execution, canary scoring, automated draft pull requests and rollback telemetry without removing these authority boundaries.
