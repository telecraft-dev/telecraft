# ADR-0003: Git is the source of truth; the UI opens pull requests

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

The platform needs history, rollback, approval, and an audit trail. All four
already exist in git; building any of them would duplicate infrastructure every
adopter already runs.

## Decision

- Git holds the rendered otelcol YAML, the history, the rollback
  (`git revert`) and the approval (the pull request). None of it is built.
- The graphical surface **opens pull requests; it never writes to a cluster**.
  The audit trail is git history.
- **A hand-committed config is legitimate, not drift.** Git is authoritative,
  so a human editing the YAML directly is a supported path: `Intended` is
  whatever git says at that SHA. No model-versus-git cross is built, because
  going "around" the UI is not a thing to catch.
- The platform holds no per-collector state (see ADR-0013). Deleting the
  platform loses delivery, never the record.

## Consequences

- Authoring is adoptable with no agent change: render into git, apply with
  whatever already applies config.
- Staged rollout must be expressible as git state or carried by the server
  statelessly — an open question (see `docs/open-questions.md`, OQ-1).

## Sources

- Shaping premises 9, 10; tickets 09, 11, 21.
