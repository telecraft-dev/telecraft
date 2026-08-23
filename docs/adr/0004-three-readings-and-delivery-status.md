# ADR-0004: Three readings; OpAMP delivery vocabulary; `library_drift`

- Status: accepted (seeded)
- Vocabulary note: written pre-ADR-0015: read Stage as Tier, Criticality Tier as Service Class, Classification as Sensitivity, Declared as Effective, Application as Service
- Date: 2026-08-12 (decided during prior shaping)

## Context

Every competitor reads a collector estate at most twice (what was pushed, what
is running). Reading it three ways, and crossing the readings, is what
produces findings with distinct owners, which is the product's differentiator:
not "we find the problem" but "we know whose problem it is".

## Decision

Three readings, each with its own definition:

| Reading | Definition |
|---|---|
| **Intended** | The config in git, pinned to a commit SHA, never a branch tip. A hand-committed config is Intended too. |
| **Declared** | The collector's own reported effective config: never what an applier holds, never what a ConfigMap contains. One definition for served and foreign collectors alike. |
| **Observed** | Telemetry that landed in a backend over a window. |

They compose; they do not multiply:

- **Declared × Observed is per requirement**, the conformance cross with
  seven outcomes: `compliant`, `not_configured`, `broken_pipeline`,
  `not_delivered`, `ungoverned`, `misconfigured`, `unknown`. Waivers
  (exemption, grace) are applied after the diagnosis, never instead of it.
- **Intended × Declared is per collector**: delivery status, using OpAMP's
  `RemoteConfigStatus` vocabulary verbatim (`UNSET`/`APPLYING`/`APPLIED`/
  `FAILED` plus hash and error). No invented delivery states. Delivery status
  sits beside the conformance verdict and qualifies it: `broken_pipeline` with
  `applied` is a pipeline fault; with `stale`/drifted it is a delivery fault
  wearing a pipeline fault's clothes; with `FAILED` it belongs to whoever
  wrote the config.
- One genuinely new per-requirement outcome: **`library_drift`**. The config
  in git no longer satisfies the tier, usually because the bar was raised and
  nothing was re-rendered. Same checker as the declared config assertion, run
  against the intended YAML. Owner is the repo; remedy is re-render and open a
  PR.

Absence discipline: every reading carries a `Known` flag. Not knowing is a
normal state, never collapsed into a failure: `unmanaged` (we see it, we do
not author it) never collapses into `unknown` (we cannot see it).

Both Intended and Declared carry **pipelines with component order preserved**,
not flat component lists: `has_receiver: [filelog]` wired only into traces is
exactly the `broken_pipeline` case the product exists to catch.

## Consequences

- The platform parses otelcol YAML properly (type/name conventions,
  normalisation): its problem now, see ADR-0005.
- Config assertions gain a pipeline scope; bare assertions default to
  any-pipeline as the migration path.

## Sources

- Tickets 11 (the dense one), 02, 05, 08; the working evaluator in the prior
  `ampup` repo (`internal/evaluate/`), whose tests read as this spec.
