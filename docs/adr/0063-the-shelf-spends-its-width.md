# ADR-0063: The shelf spends its width

- Status: accepted (amends ADR-0059 §1's geometry; extends ADR-0042 §2)
- Date: 2026-08-28

## Context

ADR-0059 fixed the lens's emphasis vocabulary on the shelf: the lens
Environment leads and draws in full, and every other Environment collapses
to one line carrying its name, its Tier count, its finding count and its
worst mark. As built, each Environment took a full-width row of the page.
A three-card lens row left the right third of a wide window empty, the
collapsed lines under it left a band of empty height, and the view and
scope switchers spent two rows of the header saying two short things.
Reviewed on the demo at desktop widths, the shelf reads as a narrow column
on a wide instrument.

The band under the cards has a natural occupant. ADR-0042 §3.4 makes every
collector count a door to the flat list, which is the only home of
per-collector detail; a reader inspecting a card who wants to see its
collectors always has to leave the shelf to look at them. The rows are
cheap to show and the shelf did not show them.

## Decision

1. **Environment sections flow in one row-major stream.** Within a team
   section, the lens Environment's cards lead, and every other
   Environment follows as a segment in the same flow: beside the lens's
   cards when it fits, wrapping when it does not, with a hairline between
   neighbours. The emphasis vocabulary is unchanged: lead position plus
   collapse, never opacity (ADR-0059 §1), a collapsed segment carries
   exactly the collapsed line's readings, expansion stays transient and
   out of the URL (ADR-0059 §2), and nothing leaves the page. What this
   decision changes is geometry alone: a collapsed Environment used to
   cost a full-width row and now costs a segment's width. The view and
   scope switchers join the title row for the same reason.
2. **A Collectors band rides under the shelf.** It shows the collector
   rows for the shelf's scope, the selected Tier's matched collectors
   first when a card is selected, bounded to a fixed number and naming
   how many it did not show. The band is a reflection, never a second
   filter home: it has no controls of its own, it follows the shelf's
   scope and selection, and its door lands on the flat list pre-filtered,
   where the explicit filters live (ADR-0042 §4).
3. **The shelf reads the collectors endpoint.** The band's rows come from
   the same endpoint the flat list reads, under the same query key, so
   TanStack Query serves both views from one fetch and the shelf's added
   load cost is one request that view-switching then reuses.

## Consequences

- The shelf's row count is still unchanged by the lens: switching
  re-orders and re-collapses the segments, it never removes one, and the
  tests that count rows across a lens switch hold.
- The collapsed line's readings are unchanged, so ADR-0059's consequences
  stand; a reader who wants two Environments side by side now often has
  them without expanding anything.
- The band repeats rows the flat list also shows. That is the point: the
  flat list remains the home of filters and of the claim flow's herd
  selection, and the band never grows either.
- A `never_seen` Tier's card carries a door to its setup guidance in the
  space its matrix would fill, beside its quiet line: ADR-0060 §3 made
  the card the waiting room, and the shelf now walks the reader to it.
- The card contract is untouched: card size, face fields and band order
  are as ADR-0041 fixed them; the cards moved, they did not change.

## Sources

- ADR-0041; ADR-0042 §2, §3.4, §4; ADR-0059; ADR-0060 §3; design session
  of 2026-08-28.
