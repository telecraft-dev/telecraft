# P4 — Per-node observability cards

- Date: 2026-08-15
- Status: verdict recorded
- Prototype: `.proto/p4-observability-cards/` (throwaway, gitignored, fixture data)

## Question

Is expectation-red ("the config didn't work") legible next to delivery-red
("the config never applied") and conformance-red ("the telemetry is
wrong")?

## What was built

One fixture of nine Tier × Environment cards covering the G5 state space:
the differentiator card (applied and conforming, expected traces never
landed), a pure delivery failure, a pure conformance failure (landed
required-attribute miss + live-tap `type_mismatch`), a double-red,
ElasticFleet's not-applicable / stale-demoted / unknown trio,
`under_populated` (seen 12 of ≥40), escalated `never_seen` (≥38, seen 0),
and the neutral 94-day never-matched Tier. Cards are Tier-grain per P3's
verdict. Four variants:

- **A · Reading bands** — Delivery/Expectation/Conformance as three fixed
  bands, same order on every card.
- **B · Signal matrix** — rows = signals, columns = readings; delivery
  spans rows (it is per-collector, not per-signal).
- **C · Triage headline** — findings-first with who-acts routing chips;
  healthy cards collapse to one line.
- **D · Bands + matrix hybrid** — built mid-session from the user's
  reaction: A's bands over B's matrix, equal card heights, C's finding
  rows demoted to a click-through drawer.

Plus: a failure-colour toggle (three hues vs one red + glyphs ▼◌✗), and
the P3-mandated "why?" popover on every derived value (strictness, floors,
expectation claims, staleness demotion).

## Verdict

**Yes — expectation-red is legible beside the other two reds, and variant
D ships.** A read fastest; B's per-signal matrix was worth keeping; the
hybrid was requested and preferred on sight ("big blocks of colour" for
the readings, signal rates as the skeleton). The applied-but-didn't-work
reading came through without coaching on the differentiator card, and the
double-red card read as two distinct problems, neither swallowing the
other.

Rules the session settled:

1. **D is the card face; C is the drill-in.** Clicking a troubled card is
   the natural gesture; the C-style finding list with who-acts routing
   chips (`→ checkout-team` for conformance, `→ tier owner` for delivery)
   lives in an expanding drawer, not on the card face.
2. **Mono-red survives.** With all three kinds in one red, band position +
   glyph + label still distinguish them — hue is enhancement, never
   crutch. Colour-blind users and small screens are safe.
3. **Equal card heights.** The uniform grid beats C's collapsing cards on
   the shelf; the collapse behaviour survives inside the drawer.
4. Neutral trio (not-applicable / stale-demoted / unknown) passes — three
   distinct readings, none alarming. Population states (escalated
   `never_seen` vs `under_populated` vs neutral-aged "consider deleting")
   read as different situations. The "why?" popover is the right shape
   for derivations. Vocabulary held — no label invited a misreading.

One open finding, deliberately deferred: **the card answers "what is
wrong with this Tier?" but not "which of these nine matter to me?"** —
with nine cards visible the relatedness/relevance question surfaced
immediately. That is the shelf, not the card: grouping by team subtree
and environment band (P3's aligned rows), severity ordering, and the
production-first lens (ADR-0033) are G7 navigation concerns (OQ-14), and
prerequisites for the card grid to be scannable at estate scale.

## What it changed

- G6's card data-contract ADR inherits D's shape: three reading states +
  per-signal volume/freshness/shape rows + a findings list carrying
  owner-routing — exactly the fields the card consumed.
- G7's surface inventory: the D card + drawer pattern joins the P1/P2/P3
  surfaces; new constraint — cards need a shelf (team/env grouping,
  severity ordering) before the grid scales.
- The three-hue palette is sanctioned but must never be load-bearing;
  glyphs and band order are the contract.
