# ADR-0058: The lens moves into a context strip on the surface

- Status: accepted (amends ADR-0042 §4's placement of the lens control)
- Date: 2026-08-26

## Context

ADR-0042 §4 made the environment lens one global chrome control. The chrome
has since grown around it: five Workspace entries (ADR-0056), jump-to-object
search, the Tour control (ADR-0051), and the profile control that the chrome
compaction introduced (issue #182). Reviewed on the demo at desktop widths,
the bar reads as crowded while the surface below it does not, and the lens
select is the one control in the bar that is neither navigation nor a door
to a panel.

Placement also mis-states what the lens is. A select sitting between the
Workspace entries and the search reads as part of getting somewhere, and
readers looked for it among the surface's own controls instead, where
choosing an Environment feels like it belongs. The lens changes what every
number on the page means; that is an argument for keeping it one control
with one value, not an argument for seating it in the navigation bar.

## Decision

1. **One context strip, on every Workspace, holds the lens.** The strip
   sits at the top of the main area, beneath the chrome and above the
   surface, and holds its place while the surface scrolls. The lens leaves
   the chrome.
2. **Placement only; meaning unchanged.** The lens remains emphasis and
   evaluation context, never a hard filter (ADR-0042 §4). It defaults to
   `production` (ADR-0033), is persisted per user, and an explicit lens in
   the URL still beats the preference. There is still exactly one lens
   control, with one value, everywhere.
3. **The strip is where surface-level context controls accumulate.** As
   filter controls earn a place above a surface, they join the strip rather
   than the chrome. The chrome keeps identity, navigation, search, the Tour
   control, and the profile control: things about getting around and about
   the reader, not about what the numbers mean.

## Consequences

- The Tour step that introduces the lens anchors on the strip; the anchor
  exists on every Workspace, exactly as it did in the chrome.
- ADR-0056 §6 is unaffected: Home still holds no state of its own, and the
  strip above it is the same control every other Workspace shows.
- The chrome's width pressure drops. The one-row rule and its measurement
  in `e2e/chrome.spec.ts` stand.
- Tests keep addressing the select by its `lens-control` test id; nothing
  about the control's behaviour changes.

## Sources

- ADR-0033, ADR-0042 §4, ADR-0051, ADR-0056 §6; issues #182 and #183;
  design session of 2026-08-26.
