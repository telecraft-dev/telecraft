# ADR-0033: Evaluation is per (Service, Environment); Requirements scope by environment list

- Status: accepted
- Date: 2026-08-14 (session G5)

## Context

Since the Environment axis landed (ADR-0023) everything around the verdict
went per-environment — sibling Tiers bind different Blueprint versions per
env (ADR-0025), floors key on (Class, Env), and `deployment.environment.name`
splits the Observed reading — while the seven-outcome cross (ADR-0004)
remained per (Service, Requirement). OQ-16 asked whether Requirements and
delivery expectations vary per Environment, e.g. completeness demanded in
production only.

## Decision

1. **The unit of conformance evaluation is `(Service, Environment,
   Requirement)`.** A Service's verdict is always per-Environment; nothing
   blends across environments. The alternative — verdict per Service with
   env as a display facet — is rejected: staging's Observed telemetry could
   mask a prod outage of the same signal, and under ADR-0025 the two
   environments may be running different config versions, so one verdict
   would judge two configs. Roll-ups (ADR-0017) count `(Service, Env)` pairs
   as rows; estate views default to the `production` lens. A Service simply
   has no row in an environment where it has no Tier and no telemetry —
   absence of an environment is not a finding.
2. **A Requirement stays one env-neutral assertion with an optional
   `environments` applicability list; absent means all environments.**
   "Completeness in production only" is one authored line
   (`environments: [production]`). Per-env requirement variant files are
   rejected (duplicated assertions drift apart); a separate
   `(Class, Env) → requirement set` mapping table is rejected for authored
   content (floors earn theirs by being platform-shipped policy;
   requirements are adopter files, one concern per file, REQ-021).
   Defaulting to production-only is rejected: explicit narrowing beats
   implicit non-coverage.
3. **Guards on the open env vocabulary**: the strict loader (REQ-021)
   rejects unknown keys; an `environments` entry naming an environment never
   seen or declared anywhere in the estate raises an authoring finding —
   visible, not fatal.
4. **Delivery gets no separate per-env mechanism.** A delivery finding
   attaches to a collector, which matches one Tier, which declares one
   Environment — the env facet is inherited by construction. Per-env
   delivery severity ("FAILED in staging is only a warning") is rejected in
   v1: the reading is identical in every env, urgency is the lens's job, and
   escalation policy was already deliberately deferred (ADR-0022 §4) because
   tightening can be added later without breaking the model.

## Consequences

- The evaluator's context (ADR-0022) already carries Environment; the
  verdict store and every surface key on `(Service, Env)`.
- Ratio denominators change: a Service deployed to three environments
  contributes three rows where it contributed one. The `production` default
  lens keeps headline numbers comparable to before.
- G6's Expectation engine inherits the same unit — expectations derive per
  `(Service, Env)` from that environment's bound config version.

## Sources

- Session G5; OQ-16; ADR-0004, ADR-0017, ADR-0022, ADR-0023, ADR-0025;
  REQ-021.
