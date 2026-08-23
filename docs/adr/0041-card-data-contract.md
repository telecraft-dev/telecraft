# ADR-0041: The card data-contract: face and drawer, states never colours

- Status: accepted
- Date: 2026-08-17 (session G6)

## Context

P4's verdict fixed the visual shape: hybrid card D ships. Three reading
bands over a per-signal matrix, findings demoted to a click-through drawer
with who-acts routing chips, equal card heights, mono-red legible (hue is
enhancement, never load-bearing). P3 fixed the grain (tier-level
aggregation; per-collector detail lives in list surfaces) and proved
derived values don't self-explain: the "why?" affordance must be fed, not
reconstructed. G7 builds every shelf and surface against this contract.

## Decision

1. **The card unit is the Tier.** Environment is the Tier's declared
   attribute (ADR-0025); P4's "Tier × Environment cards" were sibling
   Tiers. The contract keys on Tier id, never on a pair.
2. **The face payload** (cheap, bulk-fetchable for a whole shelf):
   - **Three bands in fixed order: Delivery, Expectation, Conformance**,
     each an *enum state* plus an optional worst-finding label. States are
     the contract; glyphs (▼◌✗) map from states; **hue appears nowhere in
     the contract**: P4's mono-red rule enforced structurally, by making
     colour underivable from required fields.
   - Band states include the honest neutrals, each distinct:
     `not_applicable`, `unknown`, `pending_settle` (ADR-0038's window),
     `stale_demoted` (ADR-0036's demotion).
   - **Per-signal matrix rows**: per signal lane (volume (in/out +
     reduction), freshness, shape summary), every reading carrying `as_of`
     and `Known`, so last-known-plus-age renders from the contract, never
     from client guessing (readings per ADR-0040).
   - **Population line**: matched count, floor, floor source
     (derived/declared/absent), `never_seen`/`under_populated` state
     (ADR-0035's outputs verbatim, including neutral age).
   - **Shelf summary fields**: owning team, Environment, per-band worst
     severity, finding counts per kind (exactly what G7's grouping and
     severity-ordering shelf needs, present so the shelf never fetches
     drawers to sort).
3. **The drawer payload** (fetched per card on demand): the findings list
   (kind, severity, dampening state, who-acts routing target, mandatory
   remediation text: a finding without remediation is a complaint) plus
   **"why?" derivations as structured provenance**: every derived value on
   the face (strictness, floor, each claim's verdict) carries a provenance
   object (claim → the config lines that implied it → the SHA judged
   against). P3 proved the need; P4 proved the popover shape; the contract
   feeds it.
4. **The contract is integer-versioned** like every other artefact, served
   by the platform API, and is the *only* thing card surfaces may consume:
   P3's canvas Tier cards and P4's observability cards read the same face
   payload, one model, many representations (the P2/P3 rule).

## Consequences

- The face payload is wide (bands + N signals × 3 readings + population +
  summary); bulk-fetching hundreds of Tiers is the shelf's load cost,
  bounded by authored-object cardinality, never per-collector.
- G7 inherits a shelf that can group and sort from face fields alone; the
  drawer's collapse behaviour (P4 rule 1) needs no extra endpoint shape.
- Contract version bumps are visible, reviewable events, not silent field
  drift.

## Sources

- Session G6; ADR-0017, 0025, 0033, 0035, 0036, 0038, 0039, 0040; P2, P3,
  P4 verdicts; OQ-14 (G7 consumes this contract).
