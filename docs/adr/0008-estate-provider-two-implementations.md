# ADR-0008: `EstateProvider`, keyed on the collector, with two implementations from day one

- Status: accepted (seeded)
- Vocabulary note: written pre-ADR-0015 — read Stage as Tier, Criticality Tier as Service Class, Classification as Sensitivity, Declared as Effective, Application as Service
- Date: 2026-08-12 (decided during prior shaping)

## Context

The prior codebase's seam was wrong twice: named `FleetProvider` (a vendor
product name inside the neutrality-promising package — see ADR-0001), and
shaped per-application returning only declared config, while delivery status is
per collector. Serving everything (ADR-0013) does not remove the foreign
population: collectors delivered by GitOps, config management, or a person
still need reading.

## Decision

The seam is **`EstateProvider`**: keyed on the collector, returning the estate
in one call — `Declared` (effective config, pipelines with order preserved,
and the full recursive `ComponentHealth` tree, never the flattened roll-up)
**plus** delivery status (`RemoteConfigStatus`, ADR-0004).

**Two implementations ship from day one:**

1. **OpAMP direct** — collectors served by the platform's own server.
2. **`ElasticFleet`** — the foreign-population reading via Elastic Fleet's
   agent APIs (structured effective config, pipeline wiring preserved,
   estate-wide fingerprint in one call).

Contract disciplines:

- An implementation that cannot find a collector returns `Declared{Known:
  false}`, never an error — not knowing is a normal state.
- An implementation that can never report delivery status (ElasticFleet is
  permanently `UNSET` — Elastic Fleet is monitoring-only, with no GA commitment and no
  "enforcement later") must be expressible **without that reading looking like
  a failure**.
- The minimum-populated-set rule must cover freshness, not only presence: a
  populated-but-stale field is worse than an absent one. (Open: OQ-6.)
- An integration against an API that can change unannounced needs a contract
  test, not just a client.

The other two seams survive on their original terms: `RegistryProvider` (what
applications exist, what tier is each — an input to the platform, never an
output) and `TelemetryProvider` (did signal X arrive for application Y in
window W — no query language, no index name, no product concept may leak
through it).

## Consequences

- Enforcement via Elastic Fleet is permanently unavailable, not deferred:
  Elastic Fleet redacts on key names, freezes agent identity at enrol time, and is a
  console, not a source. Any roadmap implying "read-only now, enforcement
  later" is unfounded.
- The seam design is verified against a third implementation (Prometheus,
  Bindplane, or Grafana Fleet Management) needing no core change.

## Sources

- Tickets 11, 12, 02, 03, 13, 21.
