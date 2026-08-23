# ADR-0013: The OpAMP server is stateless transport; the artefact carries its own identity

- Status: accepted (seeded)
- Vocabulary note: written pre-ADR-0015: read Stage as Tier, Criticality Tier as Service Class, Classification as Sensitivity, Declared as Effective, Application as Service
- Date: 2026-08-12 (decided during prior shaping)

## Context

No OpAMP server anywhere reads from git or has a pluggable config source: the
spec's implementations list names two agent-management platforms, both
proprietary; the upstream example server is unimportable and in-memory; reading
config from a file is an open, unimplemented upstream request. The wire
protocol layer is a maintained Apache-2.0 library (`opamp-go`'s 755-line server
package); the measured serving loop over it is ~99 lines. The larger, less
glamorous half is auth, TLS, HA, and identity, none of which the baseline
includes.

## Decision

- The platform's OpAMP server is **stateless transport that reads git and
  stores nothing**. A collector connects and reports identifying attributes;
  the server matches them against selectors held in git and serves the config
  at that path, remembering nothing. Removing the server loses delivery,
  never the record.
- **The artefact carries its own identity**: every rendered config is stamped
  `service.telemetry.resource: {telecraft.commit: <sha>}` (key follows the
  project name, decided in `branding/naming.md`). The collector reports it
  back, so
  "which commit is this running" is read *from* the collector, not remembered
  *about* it. This works identically for foreign collectors delivered by
  anything, and the stamp survives ElasticFleet's redaction.
- Collector identity is derived from reported identifying attributes;
  OpAMP's `instance_uid` stays the connection key and is never surfaced as
  identity.
- Deliberately not done (opt-in if ever): stamping the SHA onto telemetry
  itself via a resource processor: it writes into customer data.

## Consequences

- Staged rollout is the hard residue: a stateless server serving one repo path
  cannot decide that this collector gets the new config and that one does not.
  Cohort membership is state. The cohort-as-git-state hypothesis is untested
  (OQ-1, the largest undesigned piece).
- Production serving needs auth/TLS/HA designed on top of the 99-line
  baseline; that design lands in session G4 and Phase 3.

## Sources

- Tickets 11, 21, 23; premise 8's build-defence (reuse enumeration recorded in
  `2026-08-04-23-findings.md`).
