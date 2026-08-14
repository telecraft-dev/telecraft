# ADR-0034: Schema conformance — registry substrate, two placements, Service-owned findings

- Status: accepted
- Date: 2026-08-14 (session G5)

## Context

ADR-0009 adopted semconv YAML + Weaver as the conformance vocabulary and
left OQ-4 open: how the vocabulary lands in the requirement model, the
four-level `requirement_level` adoption, `AttributeNames` fidelity on
index-scoped backends, and whether a collection-time `live-check` tap is
in-product, recommended-but-external, or refused — it collides with
REQ-002 (nothing in the telemetry path). Also registered: a schema
violation's fix is an instrumentation change; routing it to whoever edits
collector config sends work to the wrong person. The gap the product fills
(ADR-0009): nothing in the ecosystem evaluates semconv conformance against
telemetry that has already landed.

## Decision

1. **The Schema Registry is a second Catalogue-pattern substrate**
   (ADR-0020 applied again): the adopter maintains a custom Weaver registry
   (importing OTel semconv, tightening `requirement_level` locally, adding
   namespaced attributes) as ordinary git content buildable by `weaver`
   alone; the platform imports registry versions through the one import
   pipeline — explicit activation with impact report, retained versions.
   No second schema syntax exists. Because `RegistryProvider` already names
   an unrelated seam, the substrate is always written **Schema Registry**;
   bare "registry" is ambiguous and avoided.
2. **A new requirement kind, `schema_conformance`, is a reference into the
   Schema Registry, never a copy**: pinned registry version by default with
   opt-in `track: head` (ADR-0026 semantics), a scope (groups/namespaces
   demanded), covered signals, and the `environments` list (ADR-0033).
   Inline attribute lists in requirement files are rejected — they
   duplicate the registry and drift. Passing the pinned registry version
   while failing the current one is `library_drift`, one more facet on the
   existing finding kind (ADR-0026).
3. **Outcome mapping — no eighth outcome** (discipline per ADR-0030). The
   Effective half of ADR-0004's cross is not applicable (instrumentation is
   invisible to collector config). All violation-grade checks in scope pass
   → `compliant`; any fails → `misconfigured` (arrived, but wrong shape);
   signal never arrived → `not_delivered`; provider cannot produce the
   reading → `unknown` with `Known: false`. Level mapping: `required` →
   violation, flips the outcome. `conditionally_required` → demoted to
   improvement-grade at evaluation: the condition is prose, not
   machine-evaluable — hard-failing on an unevaluable condition
   manufactures false reds; adopters who mean "always" tighten to
   `required` in their registry, which is what local override is for.
   `recommended` → improvement finding with a coverage ratio
   (`AttributeCoverage`); `opt_in` → information. Improvement/information
   findings ride alongside — visible on the Service and counted in
   roll-ups — but the binary feeding ratio-plus-worst is decided by
   violations alone. Promoting improvements to violations is deferred
   escalation (ADR-0022 §4); the supported lever is editing the registry
   level.
4. **`TelemetryProvider` widens by three primitives, service-scoped by
   contract**: `AttributeNames` (per service, signal, window),
   `DistinctValues` (hard-capped, truncation always reported, offered only
   for attributes the registry declares as enums), and a grouping key
   (presence per span/metric/event name — semconv required-sets are
   per-group). Fidelity is contractual: a provider that cannot produce
   service-scoped names reports the affected checks `Known: false` — never
   silent approximation, never misattribution (ADR-0008's discipline, both
   directions). The index-scoped union is sanctioned as a screening fast
   path only: a clean union proves every service in it clean; a dirty
   union triggers the costly service-scoped aggregation to attribute the
   violation. Evaluation cost stays governed by OQ-12's mitigations; OQ-12
   remains carried.
5. **The live-check tap ruling: in-product as pattern-and-reading, external
   as runtime, refused as platform component** (ADR-0031's move applied
   again). The platform renders opt-in tap wiring — a *teed*, sampled
   fan-out branch off a gateway Tier to an adopter-deployed
   `weaver registry live-check` service — never an inline hop: the tap
   dying costs findings, not data (REQ-002 intact); the adopter deploys
   upstream Weaver (REQ-003: configurations, never binaries). A
   probabilistic sampler is part of the rendered pattern by default —
   shape violations are systematic, sampling finds them at a fraction of
   the cost. Findings come home via `--emit-otlp-logs` as ordinary log
   records, read back through `TelemetryProvider` — no privileged side
   channel (REQ-053's rule).
6. **Placement is a property of the requirement**: `placement: landed`
   (default — backend-side checks per §4) or `placement: live` (evaluates
   the tap's emitted findings, unlocking type/unit/instrument/structural
   checks impossible backend-side). Same registry reference, same outcome
   mapping. A `live` requirement whose tap emitted nothing in the window is
   `unknown`, never passing — a dead tap must not read as clean.
7. **Findings are Service-owned, always.** A schema-conformance finding
   attaches to `(Service, Environment)` and routes to the Service's owner
   (REQ-015) — the party who can fix instrumentation. Never the Tier or a
   collector. Remediation text is registry-derived and concrete: group,
   attribute, declared type/level, upstream's machine-readable migration
   note for deprecations. Where a gap is enrichable at collection
   (`k8sattributes` injecting a resource attribute), remediation may
   *suggest* it — the finding never splits or reroutes; one finding, one
   owner.

## Consequences

- REQ-022/REQ-023 gain their mechanism; the sanctioned seam extension grows
  from one primitive to three plus a grouping key.
- The import pipeline (ADR-0020) generalises to two substrate types;
  activation impact reports must cover registry activations ("N services
  newly fail `required` on `db.namespace`").
- The evaluator needs Schema Registry versions as context alongside
  catalogue versions and Environment (ADR-0022, ADR-0033).
- P4 (per-node cards) should show conformance-red from a schema violation
  beside delivery-red; G6's Expectation engine may consume tap findings as
  ordinary telemetry.

## Sources

- Session G5; OQ-4; ADR-0004, ADR-0008, ADR-0009, ADR-0016, ADR-0020,
  ADR-0022, ADR-0026, ADR-0030, ADR-0031, ADR-0033; research dossier
  `2026-08-04-05-findings.md` (Q3–Q6).
