# ADR-0036: The `EstateProvider` contract: capability declaration, `as_of`, staleness demotion, shipped test kit

- Status: accepted
- Date: 2026-08-14 (session G5)

## Context

ADR-0008 demanded a minimum-populated-set rule "stated so it can be
checked", covering freshness not only presence: not-knowing is normal
(`Known: false`, never an error); a provider that can never report a
reading (ElasticFleet's permanently-`UNSET` delivery status) must not look
like failure; a populated-but-stale field is worse than an absent one.
OQ-6 (ticket 12's surviving question) asked for the mechanics.

## Decision

1. **Static capability declaration.** An implementation declares once
   which readings it can ever populate (`ElasticFleet:
   delivery_status: never`). This splits `UNSET` into two honest states
   mechanically: **incapable** (declared, renders as "not applicable",
   never failure, ADR-0008's demand made structural) versus **silent**
   (declared capable but not delivering, which is a provider fault and is
   loud).
2. **The minimum populated set**, for every collector returned:
   - the identity attributes selectors match on;
   - an `as_of` timestamp on every reading carried;
   - every capability-declared reading either populated-with-timestamp or
     explicit `Known: false`;
   - the ADR-0008 structural invariants: pipeline order preserved,
     `ComponentHealth` recursive, never flattened.
   Absent identity or absent timestamps is non-conforming, full stop.
3. **Freshness is the platform's arithmetic, never the provider's claim.**
   The provider declares its refresh cadence; the platform computes
   staleness uniformly (`now − as_of` against cadence × tolerance). Past
   the horizon a reading is **demoted to `Known: false` for evaluation**
   (a stale Effective config never feeds a fresh-looking verdict), while
   surfaces may show last-known-plus-age ("as of 3h ago"). Stale data may
   inform a human, never a verdict.
4. **The rule ships as a conformance test kit, not prose**: a fixture
   suite every implementation must pass. Unknown collector →
   `Known: false` not error; capability honesty (declare it, populate it);
   timestamp presence; order and tree preservation; staleness demotion.
   ADR-0008's "verify the seam against a third implementation" stays true
   through the kit rather than re-litigation. The same kit pattern applies
   to `InventoryProvider` (ADR-0035).

## Consequences

- The contract-test requirement ADR-0008 attached to unstable upstream
  APIs (ElasticFleet) is subsumed: the kit is the contract test.
- Provider capability declarations become surface input: estate views
  render "not applicable" (incapable) distinctly from `unknown` (capable
  but currently unknowable): P2's neutral-state handling extends
  naturally.
- Staleness demotion means verdict counts can change with no estate
  change when a provider goes quiet: the honest behaviour; the finding
  is the provider's silence, not the Services'.

## Sources

- Session G5; OQ-6; ADR-0008, ADR-0013, ADR-0035; ticket 12.
