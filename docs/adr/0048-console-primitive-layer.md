# ADR-0048: A console primitive layer, and panel width as a reader's preference

- Status: accepted
- Date: 2026-08-19

## Context

ADR-0047 predicted that the branding pass would land "by rewriting
`console/src/tokens.css` and adding a theme resolver plus one chrome
control. No surface markup changes." Rewriting the tokens proved that
prediction wrong, because swapping the values exposed what the values were
hiding.

Counted over `app.css` before the pass: nine button rules, eleven chip
rules, and three side-panel rules. No two buttons agreed on padding, no two
chips agreed on radius, and the three panels were fixed at 320px, 380px and
380px. A token swap recolours all of that faithfully and leaves it exactly
as inconsistent as it was, because the inconsistency was never in the
colours. ADR-0045 §4's contract says the branding pass need not touch
markup; it does not say the markup is beyond touching.

Two other things surfaced with it. The card grid's fixed height clipped its
own footer once the type grew, and the footer carries the collector count
that ADR-0042 §3.4 makes a door to the flat list. And the fixed panel widths
were never right for anyone: a Tier's findings and a rendered otelcol
document want very different amounts of room, and so do a 13-inch laptop and
a 32-inch display.

## Decision

1. **A primitive layer in `console/src/ui/`, of four components and no
   more.** `Button` (three tones), `Chip` (five tones), `Panel`, and the
   `Mark`/`Icon` pair ADR-0047 §6 already required. Each exports a class
   helper beside the component, because roughly half of these are links
   rather than buttons — a who-acts chip routes, it does not act — and a
   link must stay an anchor. This is not a component library, which
   ADR-0047 §1 ruled out; it is the small set of things `app.css` was
   already describing several times each.

2. **Tones are structural, not chromatic.** A primary button is a solid
   fill of the ink colour and a quiet one is underlined text, because
   ADR-0047 §4 spends the console's accent on contrast rather than hue.
   A chip's tone reinforces the words inside it and never replaces them
   (ADR-0041 §2).

3. **Panel width is the reader's, and it is a device preference.** Drag the
   handle, or focus it and use the arrow keys; the handle is a `separator`
   to a screen reader. The chosen width follows the theme's rule (ADR-0047
   §2): it lives in `localStorage`, not the URL, because it says nothing
   about what is on screen and a shared link should not carry it. That is
   the second documented exception to ADR-0042 §3.5, and the last one we
   expect: view state is citable, device preferences are not.

4. **The per-signal matrix scrolls inside the card; the card does not
   grow.** Equal heights at card grain are not negotiable (ADR-0042 §2, P4
   rule 3). Measured in-browser, the common card is roughly 250px and the
   contract's worst case — four signal lanes each carrying a reduction and
   an error reading — is 410px. Sizing every card for the worst case wastes
   most of the shelf; clipping loses the foot. So the matrix is bounded and
   scrolls, and the bands and the foot are on screen whatever the payload
   carries.

## Consequences

- ADR-0047's "no surface markup changes" consequence is superseded by this
  ADR, and only that consequence. Every decision in ADR-0047 stands.
- `app.css` loses roughly 5.8 KB of duplicated rules. What remains beside a
  shared class is only what makes that use different, and a rule that
  re-decides padding, border, radius or size is now a review comment.
- The primitive layer is a place things can be added to carelessly. Four
  components is the budget; a fifth is a decision, not a convenience.
- `--card-width` and `--card-height` were re-measured in the browser rather
  than chosen, at 308px and 288px.
- The router stamps `active` on any `Link` whose route matches, so the
  pressed state of a toggle is `selected`. A shared class named `active`
  would silently fill any who-acts link that happened to point at the
  surface the reader was already on, which it did until it was caught.

## Sources

- ADR-0041, ADR-0042, ADR-0044, ADR-0045, ADR-0047.
- `docs/branding/design-system.md`, `docs/branding/identity.md`.
- Card grid and matrix measurements taken in-browser over the fixture
  estate, session 2026-08-19.
