# ADR-0025: The Tier is the rendering and binding unit; Tiers declare an Environment

- Status: accepted
- Date: 2026-08-14 (session G3)

## Context

ADR-0023 §2 said "a Service binds Blueprint versions per Environment" — but
collectors are not per-Service: an edge daemonset or gateway serves many
Services, and a collector inherits everything from the Tier it matches into
(ADR-0007, ADR-0016 §5). The rendered artefact needed an unambiguous unit,
and "environment" turned out to name two different facts: where the
*infrastructure* runs, and what *traffic* flows through it.

## Decision

1. **The Tier is the sole binding site and rendering unit.** One rendered
   artefact per Tier; a Tier binds exactly one Blueprint version. A Service
   with genuinely dedicated pipeline config (sidecar, per-service agent
   pool) gets its own Tier — already the established mechanism ("split the
   Tier", ADR-0016 §5). Rejected: Services as co-equal binding sites with
   renderer-side weaving (a merge engine with cross-team collision
   semantics — addable later as a new contribution kind if a real adopter
   needs it), and Tiers assembled purely from traversing Services (gateway
   infrastructure config belongs to no Service).
2. **Every Tier declares exactly one Environment.** The Tier's Environment
   is an attribute of the infrastructure ("this gateway is production
   plumbing"). Adopters running parallel infra per environment declare
   sibling Tiers (`gateway` in production, `gateway` in staging); an adopter
   with one shared gateway declares it `production`.
3. **ADR-0023 §2 is refined, not contradicted**: per-Environment Blueprint
   version binding is realised through sibling Tiers — the prod gateway
   binds v4 while the staging gateway trials v5. The promotion flow G4
   builds on is unchanged, anchored to the object that actually has a
   config.
4. **Evaluation strictness is derived, never hand-maintained**: a Tier's
   config is judged at **the Tier's declared Environment × the strictest
   Service Class among Services whose Paths traverse it** (topology answers
   which). Adding a C1 Service's Path through a Tier automatically tightens
   that Tier's judgement. Non-production data flowing through a production
   Tier relaxes nothing — over-governed is harmless, under-governed is the
   failure mode.

## Consequences

- The artefact inventory is enumerable — `Tier → rendered config` — which is
  the shape the stateless OpAMP server serves (ADR-0013) and a rollout
  Cohort can point at (OQ-1).
- Production data traversing a non-production Tier is a topology smell worth
  a future finding kind; deliberately not designed now.
- Per-collector artefacts never exist (ADR-0007, ADR-0012 hold).

## Sources

- Session G3; ADR-0007, ADR-0016, ADR-0023.
