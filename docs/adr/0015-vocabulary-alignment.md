# ADR-0015: Vocabulary aligned to industry and upstream usage

- Status: accepted
- Date: 2026-08-12 (session G1)
- Supersedes: the *naming* halves of ADR-0004 and ADR-0007 (their models are
  unchanged)

## Context

The terminology lesson (`docs/terminology.html`) surfaced real misalignments
between the inherited vocabulary and what industry and upstream actually say.
The inherited words were chosen inside a single enterprise's context; as an
open-source project the calculus flips — matching the reader's existing
vocabulary beats internal continuity, and our own rule (ADR-0001: adopt
upstream words verbatim) was being violated by two of our core terms.

## Decision

| Old | New | Why |
|---|---|---|
| **Stage** | **Tier** | Industry universally says "edge tier / gateway tier" for a topology position. |
| **Criticality Tier** | **Service Class** | "Tier" is now taken by topology. Values C1/C2/C3, adopter-renamable. "Priority"/"Severity" remain rejected (incident words). |
| **Classification** | **Sensitivity** | Completes the Service Class move: "Class" next to "Classification" was a collision. "Sensitivity" is the established enterprise term (sensitivity labels) and reads instantly. |
| **Declared** | **Effective** | Genuine collision: GitOps calls the *git side* "declared configuration" — our old usage read backwards to a GitOps reader. OpAMP's own field is `EffectiveConfig`; ADR-0001 requires the upstream word. |
| **Application** | **Service** | OTel's identity attribute is `service.name`; the governed unit in every OTel-native backend is a service. |
| **Estate** | **Estate** (kept) | Industry says "fleet", but our flagship first-party integration is `ElasticFleet` — a bare "fleet" would be permanently ambiguous. Deliberate, documented deviation. |
| **Hop, Path, Blueprint** | kept | No industry-standard word exists for the first two (that gap is the product); Bindplane's copy-and-edit "Blueprints" collide only flatteringly. |

The axes rule survives verbatim under new names: **Service Class**
(completeness: how much telemetry a service must emit) ⊥ **Sensitivity**
(access: routing and redaction). Never conflated.

## Consequences

- **The trap inverts and must be policed**: "Tier 1 app" is *also* universal
  industry speak for criticality. Nothing in the UI, docs or code may ever
  render a Service Class as "Tier N" — the values are C1/C2/C3.
- The three readings are **Intended / Effective / Observed**. The conformance
  cross is Effective × Observed; delivery status is Intended × Effective.
- The code port renames accordingly (`model.Tier` → service class,
  `Application` → `Service`, `Declared` → `Effective`).
- Seeded ADRs 0001–0014 keep their original wording as historical records;
  the affected ones carry a banner pointing here. All living documents
  (glossary, requirements, plan, open questions) are swept.
- `docs/terminology.html` is the teaching companion to the glossary and is
  maintained alongside it.
