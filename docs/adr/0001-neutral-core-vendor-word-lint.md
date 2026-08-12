# ADR-0001: Neutral core; vendor words are a lint error

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

The platform must be backend-neutral to be a credible open-source project. The
prior shaping work found this discipline decays unless it is mechanically
checkable: the original codebase named a seam `FleetProvider` — a vendor
product name inside the one package whose contract promises vendor neutrality.

## Decision

The core is vendor-neutral. Elastic is the first plugin, never privileged; any
vendor-specific behaviour in the core is a design error.

Naming rules, enforced by a lint over both code and docs:

- **Seam (interface) names are domain terms only** — no vendor word appears in
  any interface definition: `EstateProvider`, `RegistryProvider`,
  `TelemetryProvider`.
- **Implementations are fully qualified with the vendor's product**, never the
  company: `ElasticFleet` (never `Fleet`), `Elasticsearch` (never `Elastic`),
  `GrafanaFleetManagement` (never `Grafana`). A bare `Fleet` appears nowhere.
- Where a concept exists upstream (OpenTelemetry, OpAMP, Kubernetes), adopt its
  name and semantics verbatim rather than inventing a dialect; where something
  is genuinely absent upstream, propose it there rather than shipping a local
  synonym.

## Consequences

- Backend-agnosticism becomes greppable — a lint, not a habit. The lint ships
  in Phase 0, before the code it checks.
- Porting the prior conformance code requires renames (e.g. the
  `internal/provider/telemetry/elastic.go` implementation talks to
  Elasticsearch and must say so).

## Sources

- Shaping premises 5, 12, 13, 14; ticket 11 (`shaping-tickets/11-the-three-readings.md`),
  where the naming rule was set and generalised.
