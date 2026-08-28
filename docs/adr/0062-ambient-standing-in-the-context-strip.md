# ADR-0062: Ambient standing readings in the context strip

- Status: accepted (extends ADR-0058 §3)
- Date: 2026-08-28

## Context

ADR-0058 moved the lens into a context strip on every Workspace and made
the strip the place where surface-level context controls accumulate. As
built, the strip holds one select and a row of empty space, while the
readings a reader carries between Workspaces each sit one navigation away:
the estate's standing on Home, the designated Catalogue version in
Catalogue & Governance, the ungoverned population behind the flat list's
filter. A reader who wants to know whether anything changed while they
worked elsewhere has to leave the surface they are on to find out, and the
strip is the one band that is already on every surface, already seated
beside the lens whose Environment gives those numbers their meaning.

Readings are a new content class for the strip. ADR-0058 §3 speaks of
controls, and a reading is not a control; letting content accumulate in a
persistent band without a decision is how an instrument becomes a
dashboard, which is the failure the visual identity names. So the addition
is decided here, and bounded here.

## Decision

1. **The strip carries ambient standing readings beside the lens.**
   Three, and only these three:
   - the estate's finding standing under the lens: the worst mark, the
     finding count, and the exempt count alongside so an exemption-heavy
     clean reading cannot hide (ADR-0017's discipline at strip grain);
   - the active Catalogue version, with the version on offer beside it
     when one is;
   - the ungoverned collector count, in ungoverned styling and never a
     severity hue (ADR-0031: concern, never failure).

   A quiet per-Environment summary for the non-lens Environments may sit
   at the right edge, because the payload the strip already reads carries
   it. Nothing else joins the strip without a new decision: the strip
   never becomes a findings list, and a fourth reading class is a
   decision, not a convenience.

2. **The strip derives; it never judges.** This is Home's posture
   (ADR-0056 §2) applied to the band above it. Every number comes through
   the module that already owns the judgement: `estate/rollup.ts` for the
   root row's worst and waived counts, `estate/order.ts` for a card's
   finding total, `home/summary.ts` for the roll-up's worst-across-kinds
   and the ungoverned total, and the activations payload for its own
   designation. The derivation lives in `chrome/ambient.ts` and is
   tested there. A strip-only verdict could disagree with the surface its
   door opens, and a reader would have no way to tell which was right.

3. **Every reading is a door, and every door carries its filter**
   (ADR-0056 §4, generalised to the strip). The standing opens Home, the
   triage surface; the active version opens the Catalogue browse view;
   the version on offer opens the Activation view where the impact report
   waits; the ungoverned count opens the Estate flat list filtered to the
   ungoverned population, the same door Home offers. The edge summary is
   the one element that is not a door, and it is the first to give way.

4. **One row, on the chrome's own argument.** The strip never wraps: at
   narrow widths the edge summary is dropped, and past the point where
   nothing else fits the strip scrolls, exactly as the chrome bar does. A
   reading that is off the edge can be reached; two printed over each
   other cannot be read.

## Consequences

- The strip reads `/api/v1/estate` and `/api/v1/activations` on every
  Workspace. The estate read is free: the lens select already makes it
  everywhere, and TanStack Query serves both consumers from one fetch,
  as it does for the Workspaces that read the same key. The activations
  read is the strip's one addition, and it is deduped with the
  Activation view's own query the same way.
- The lens control is unmoved: it keeps its `lens-control` test id and
  its behaviour, so ADR-0058's consequences stand, and the Tour anchor
  on the strip is unaffected.
- `e2e/chrome.spec.ts` gains the strip's own measurements: the readings
  and their doors, and the one-row rule at the widths the chrome is
  already measured at.
- A reading whose data has not arrived is absent rather than a
  placeholder: the strip is ambient, and a band of skeletons above every
  surface would be louder than the readings themselves.
- The risk is the one named in context: the strip is now the easiest
  place to put a number nobody wants to give a surface to. Decision 1's
  bound is the budget, and a proposal to widen it re-opens this ADR.

## Sources

- ADR-0017; ADR-0031; ADR-0056 §2, §4; ADR-0058 §3; design session of
  2026-08-28.
