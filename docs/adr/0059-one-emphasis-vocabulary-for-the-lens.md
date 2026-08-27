# ADR-0059: One emphasis vocabulary for the lens

- Status: accepted (amends ADR-0042 §4's presentation of lens emphasis)
- Date: 2026-08-26

## Context

ADR-0042 §4 made the lens emphasis and never a filter, and required
multi-Environment surfaces to keep every row visible. It did not say what
emphasis looks like, and three surfaces answered differently. The shelf
drew non-lens Environment rows at reduced opacity, which readers took for
disabled rather than for judged-under-another-Environment. The flat list
dimmed its non-lens rows the same way while also offering an explicit
Environment filter, two environment controls with different semantics and
no visible distinction. The topology canvas gave the lens no treatment at
all. Reviewed together on the demo, the lens looked like it did a
different thing on every surface, and on none of them did it look like
what ADR-0042 §4 says it is.

The move to the context strip (ADR-0058) sharpened this: a control seated
with the filters is expected to visibly do something, and 40% less
opacity is not legible as something.

## Decision

1. **Emphasis is lead position plus collapse, never opacity.** The lens's
   Environment goes first, drawn in full. On the shelf, every other
   Environment row collapses to one line carrying the Environment's name,
   its Tier count, its finding count and its worst mark, with the cards
   one press away. Nothing leaves the page: the line is the row, so a
   staging violation still shows its count and its red mark while the
   lens is on production.
2. **Expanding a collapsed row is transient presentation.** It is local
   to the session and fully loseable, like the within-row arrangement
   (ADR-0042 §7); it is not URL state. The lens in the URL already
   reproduces what the sender saw at the grain the URL promises.
3. **The flat list's rows carry no lens styling.** The list is the home
   of explicit filters (ADR-0042 §4), and its Environment filter is the
   tool there. Dimming rows a visible filter did not remove gave one
   table two environment semantics at once.
4. **The topology canvas orders its Environment bands lens-first.** The
   same lead-position vocabulary, at band grain. Every band stays drawn
   in full: canvas nodes carry populations rather than verdicts, so there
   is nothing for a collapse to summarise.
5. **The opacity rules leave the stylesheet.** Reduced opacity on the
   canvas continues to mean exactly one thing: outside the traced Paths
   (ADR-0042 rule 3.3). It no longer also means outside the lens
   anywhere.

## Consequences

- The shelf's row count is unchanged by the lens: switching the lens
  re-orders and re-collapses, it never removes. Existing tests that count
  rows across a lens switch hold.
- A reader who wants two Environments side by side expands the second
  row and has them; the expansion does not survive a reload, which is the
  cost of keeping the URL honest about what it reproduces.
- The glossary's Environment lens entry follows this wording.

## Sources

- ADR-0042 §4, ADR-0044 §3, ADR-0058; the 2026-08-26 UI review.
