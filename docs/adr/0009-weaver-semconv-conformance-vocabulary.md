# ADR-0009: Conformance adopts semconv YAML and Weaver; nothing is built on schema URLs

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

Two different things are called "schema" in OpenTelemetry, and conflating them
is a trap. OTel schema files / schema URLs are a version-*migration* mechanism:
every expressible operation is a rename or a split, and the format is
structurally incapable of expressing a constraint. The semantic-conventions
YAML registry, tooled by Weaver, is a real shape declaration: typed attributes,
enum members, stability markers, and a four-level `requirement_level`.

## Decision

- **Adopt the semconv YAML model**: typed attributes, enum `members`,
  `stability`, and the four-level `requirement_level` (`required` /
  `conditionally_required` / `recommended` / `opt_in`) — strictly richer than
  a binary required-attributes list; `recommended` is the principled home for
  sub-1.0 attribute coverage.
- **Adopt Weaver custom registries** for adopter-specific requirements: import
  the OTel registry, tighten `requirement_level` locally, add adopter
  namespaced attributes (e.g. a criticality-tier enum) as standard model
  constructs. `weaver registry infer` turns onboarding from "write a schema"
  into "prune a generated one".
- **Adopt Weaver's finding taxonomy verbatim**, including the three-way
  severity split `violation` / `improvement` / `information`.
- **Build nothing on schema URLs** — except reading `schema_url` off the
  Resource as an observable claim of which convention version an application
  emits.
- **The query-language constraint is permanent**: requirements express signal
  presence, volume, attribute coverage and cardinality — never a backend's
  query language. Type, unit and instrument conformance are permanently out of
  backend-side checking, because most backends destroy the declared type at
  ingest. One new `TelemetryProvider` primitive — `AttributeNames` (the set of
  attribute names in use for an app/signal/window) — unlocks nine Weaver
  finding types as pure string logic.
- Profiles are out of scope: the signal is Alpha and a requirement written
  against it could not be evaluated honestly. Logs, metrics, traces only.

## Consequences

- The gap the product fills, stated precisely: Weaver checks a stream; nothing
  evaluates semconv conformance against telemetry that has already *landed*,
  per application, per tier, across an estate.
- Whether a collection-time `live-check` tap is in-product, recommended, or
  refused is open (OQ-4) — it collides with "nothing in the telemetry path".

## Sources

- Tickets 05, 16; research dossier `2026-08-04-05-findings.md`.
