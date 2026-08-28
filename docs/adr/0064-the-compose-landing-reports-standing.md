# ADR-0064: The Compose landing reports its Blueprints' standing

- Status: accepted (extends ADR-0043; complements ADR-0061 §3)
- Date: 2026-08-28

## Context

ADR-0043 fixed the Compose Workspace's shape as three surfaces over the
one open Blueprint, and left its landing undecided by omission. As built,
the landing was three name-only cards in the top-left of an otherwise
empty screen: a reader arriving in Compose saw which Blueprints exist and
nothing of what governance already knows about them, though every fact a
row could carry (the lanes, the `satisfies` claims, the Tier binding, the
effective Allow-list, the Catalogue designation) is served or derivable
from data other surfaces already read. The Blueprints view (ADR-0061 §3)
does not fill the gap and must not: it answers discovery across the
estate, with fits facets and endorsement filters, and a landing that grew
the same controls would be the same view twice.

The temptation the landing invites is a verdict column. The engine's
verdicts (findings, floors, requirement coverage, the save gate) are
computed per open draft (ADR-0022 §2), so a full standing per Blueprint
would mean an engine run per row on every visit, or a new endpoint
caching one; either way a reading that can go stale against the composer
it opens.

## Decision

1. **The landing is the reader's Blueprints with their
   governance-relevant facts.** One table row per Blueprint: name and
   version, owning team, the signal lanes the doc declares, the
   `satisfies` claims as stamped, and the Tier binding as a door to that
   card on the Estate shelf (a doc bound to no Tier says so plainly).
   The row's name opens the workspace exactly as the cards did.
   Discovery stays in the Blueprints view (ADR-0061 §3), one door away:
   the landing answers "my Blueprints and their standing", the browse
   view "which Blueprint fits", and neither grows the other's filters or
   controls. A Requirements section beneath lists every claim the
   landing's Blueprints stamp, one row per claiming Blueprint, each a
   door to the engine's verdict on the Requirement-first surface: the
   claim is intent, the verdict is fact, and the landing shows only the
   intent (REQ-031).

2. **A right rail carries the two readings checked before composing,
   derived through the modules that own them.** The reader's team's
   effective palette summary (allowed count, the version judged against,
   the Grants in force) comes through `governance/effective.ts`, the
   module the Effective palette view reads; the Catalogue designation
   and the impact report behind any version on offer come from the
   activations payload the context strip and the Activation view already
   read, repeated verbatim, never re-derived (ADR-0020 §6). Every
   element is a door: the palette summary to the Effective palette view,
   the offer to Activation, exactly as ADR-0062 §3 doors the strip's
   readings. A section whose data is absent is absent, so a read-only
   reader or an estate with nothing on offer sees a shorter rail, not a
   placeholder.

3. **The one verdict-shaped column is the Allow-list standing, and it
   speaks only of the save gate.** Only an Allow-list violation blocks a
   save (ADR-0022 §3), and that judgement is membership alone:
   `governance/effective.ts` already mirrors it for the Effective
   palette view, so a row's standing derives from data the landing
   fetches anyway, with no engine run. A blocked row reads "save
   disabled" with the offending reference named beside it, in the
   engine's own words; an unblocked row reads "clear" under a column
   headed Allow-list, which claims nothing about floors, lifecycle,
   ordering, or requirement coverage. A full standing column was
   rejected: those verdicts are the engine's per open draft, and a
   landing that computed them its own way, cached them, or grew an
   endpoint for them could disagree with the composer its row opens,
   and a wrong or stale standing is worse than none. The derivation
   lives in `surfaces/compose/standing.ts` and is tested against the
   engine's blocking rule.

4. **The landing's data cost is bounded to reads other surfaces make.**
   It adds no endpoint. Beyond the Blueprint list the old cards read, it
   reads the estate payload (the lens select already fetches it
   everywhere), governance, the catalogue versions and the active
   version's entries (the Effective palette view's reads), the shared
   Component list, and activations (the strip's read); every query uses
   the same key as its existing consumer, so TanStack Query serves each
   from one fetch. The effective palette is computed once per owning
   team and shared between the table rows and the rail.

## Consequences

- The composer, the Requirement-first surface, the node canvas, the YAML
  flyout, the claim flow, and the save flow are untouched; once a
  Blueprint is open, the compact list returns beside the workspace so
  switching Blueprints stays one click, and the old `blueprint-` doors
  keep their ids in both states.
- The Allow-list standing and the composer's save gate share one
  judgement path, so the chip cannot say "clear" while Save is disabled.
  The cost of the bound is honesty about scope: a Blueprint with floor
  or lifecycle findings still reads "clear" in the Allow-list column,
  and the composer remains where those findings are read.
- The Requirements section repeats claims the composer also shows. That
  is the point: the landing gathers what the reader would otherwise
  collect by opening each Blueprint, and the verdict stays where the
  engine gives it.
- A demo snapshot serves every read the landing makes, so demo parity
  holds without projection changes.

## Sources

- ADR-0022; ADR-0043; ADR-0061; ADR-0062; design session of 2026-08-28.
