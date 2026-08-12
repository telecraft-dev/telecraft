# ADR-0017: Team hierarchy is a tree; roll-up is ratio-plus-worst per kind

- Status: accepted
- Date: 2026-08-12 (session G1)

## Context

Ownership attaches to every authored object (ADR-0016); compliance must
aggregate up a management structure so "team running these servers must run
these modules" is reportable at any level.

## Decision

1. **Owner → Team → parent Team, as a strict tree.** An Owner is the lowest
   unit of management and belongs to exactly one Team; a Team has at most one
   parent. Multi-parent membership is rejected because every roll-up would
   double-count. The hierarchy is supplied through a seam, not owned by the
   platform: first-party is a reviewable `teams.yaml` in the estate repo;
   group-claim mapping from OIDC/SAML (ADR-0019) can come later without core
   change.
2. **A team's roll-up is the set of findings routed to owners in its
   subtree** — which includes kinds beyond service verdicts: delivery
   findings on Tiers it owns, component findings on Components it owns.
3. **Aggregation is ratio-plus-worst, per kind, never blended.** For each
   finding kind (service conformance / delivery / component health): a
   passing-over-counted ratio, a worst-outcome badge, and the waived count
   always alongside — an exemption-heavy 100% cannot hide. No single blended
   number exists at any level; "never a single estate-wide number" (prior
   shaping) generalises to every node of the tree.

## Consequences

- A parent team's view is bigger than the sum of its services — it includes
  the delivery and component findings its platform-ish children own. That is
  the point.
- Prototype P2 (estate & team roll-up view) tests whether this reads clearly.
- Exemption authority within the tree is a G5 question; expected-count
  ("running N servers") needs the cardinality source, deferred to G5 (OQ-5).

## Sources

- Session G1; prior evaluator's `Score` design (ratio + worst-first + waived
  alongside) in the shaping-era codebase.
