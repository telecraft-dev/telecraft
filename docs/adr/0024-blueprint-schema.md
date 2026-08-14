# ADR-0024: Blueprint schema v1 — a domain document where everything is a Component

- Status: accepted
- Date: 2026-08-14 (session G3)

## Context

P1's verdict fixed the shape the schema must serialise: per-signal lanes plus
collector-wide extensions (variant A), with phase ordering handled naturally
by findings, not UI. OQ-10 asked for the schema itself: format, component
references, ordering rules, versioning units.

## Decision

1. **The Blueprint is a domain-shaped document, never annotated otelcol
   config.** Its own schema; the renderer resolves references and compiles to
   otelcol YAML, which is strictly an *output* format. Annotating collector
   config was rejected: otelcol YAML has no reference mechanism, so
   inheritance-by-reference (ADR-0016) would force copy semantics or
   non-standard syntax inside a file that looks standard.
2. **Lanes mirror upstream verbatim.** Pipelines serialise per-signal under
   upstream signal names (`traces`, `metrics`, `logs`, `profiles` —
   ADR-0001/0015), each an **explicitly ordered list** of Component
   references, plus a collector-wide `extensions` block. The renderer never
   re-sorts; what you see is what renders.
3. **Every lane entry is a Component; two residences.** A Blueprint may
   declare **local Components inline** — same schema as standalone ones,
   implicitly owned by the Blueprint's owner, not referenceable from outside
   the Blueprint. Shared Components are standalone files with explicit
   owners. Promotion local→shared is a mechanical move-to-file that forces
   the ownership conversation at the right moment. Raw inline otelcol blocks
   (a second kind of lane entry) were rejected: invisible to ownership,
   findings routing and the evaluator.
4. **Identity: team-qualified logical ids.** Shared Components are
   `<team>/<name>` (e.g. `infosec/pii-redaction`); the team segment matches
   the owning team's directory, and the layout convention (ADR-0027)
   resolves id → path — the id, never the path, is the reference. Local
   Components are a bare `name`, unique within their Blueprint. Flat global
   names (collisions, no ownership signal) and raw repo paths (freeze the
   layout into every consumer file) were rejected.
5. **Rendered component ids carry provenance.** In rendered otelcol YAML,
   instances become standard `type/name` ids: shared → `type/team.name`
   (`transform/infosec.pii-redaction`), local → `type/name`. Collisions are
   a mechanical render error.
6. **The phase concept is dropped.** With one Blueprint bound per Tier
   (ADR-0025), multi-blueprint stacking — the thing phases arbitrated — no
   longer exists; OQ-10's "phase-collision rules" resolve to *nothing to
   collide*. Ordering wisdom (`memory_limiter` first, `batch` last) ships as
   **evaluator rules keyed on catalogue types**, raising ordering findings
   only (ADR-0022) — exactly what P1 validated. Per-Component phase metadata
   was rejected as a self-maintained taxonomy upstream doesn't provide.
7. **Versions are explicit monotonic integers**, on Components and
   Blueprints alike, bumped by the owner in the same PR as the change. Pins
   (`@4`) are legible, diffs are version-to-version, "behind by N" is
   countable. Content change without a bump is a **mechanical render
   refusal** — like invalid YAML, not a policy block. The distinction is
   explicit: *mechanical validity* may always refuse a render; *policy*
   hard-blocks only on allow-list violations (ADR-0022 §3 intact). Git-SHA
   versions (noise, unreadable pins) and semver (compatibility promises we
   cannot check) were rejected.

## Consequences

- The glossary's Blueprint entry loses "with a phase"; `satisfies` mechanics
  are pinned in ADR-0026.
- The schema is a public, versioned contract (like the catalogue artefact,
  ADR-0020): validated on load, evolved compatibly.
- The composer's palette-drag creates a local Component by default; "share
  this" is a deliberate act.
- REQ-030 ("phase-ordered blueprints") is satisfied by ordering findings,
  not a phase field.

## Sources

- Session G3; P1 verdict (`docs/prototypes/p1-blueprint-composer.md`);
  ADR-0001, ADR-0016, ADR-0022.
