# ADR-0056: A Home surface, and triage before activity

- Status: accepted (amends ADR-0042 §1's closed surface inventory)
- Date: 2026-08-25

## Context

ADR-0042 §1 closed the console's surface inventory at four activity-first
Workspaces and assigned Estate the question "how are we doing?". That
assignment was right, and it is not the question this ADR is about.

The shelf rests on the signed-in user's team subtree and one click widens
it to the estate (§2). Widened, over an estate of many Teams and many
Tiers, the shelf answers "how are we doing?" by drawing every Tier: Team
subtree sections, crossed with Environment rows, of equal-height cards.
Ordering is worst-severity-first from face fields alone, so the card that
most deserves attention is genuinely first. What ordering cannot supply is
an ending. A reader who has seen the worst card still has to walk the whole
shelf to learn whether anything else is wrong, and the shelf is unbounded
in exactly the dimension an estate grows in.

Estate is also the Workspace with the least room left. It already carries
three view-switchers over one model (shelf, tree-table roll-up, flat
filter-first list), the universal card panel summoned in place beside any
of them, and the claim flow's panel in place of that. A summary band added
above them would make the densest Workspace denser, and it would compete
with the shelf for the same first glance, which is the one thing the shelf
is for.

The question that has no home is not "how are we doing?" but "where do I
look first?". The two differ in where they end. "How are we doing?" ends in
Estate, which is why Estate owns it. "Where do I look first?" almost never
does: an ungoverned population ends in the flat list (ADR-0031), a held
Rollout in Topology's ledger (ADR-0029), a blueprint-shaped finding in
Compose at the offending lane (ADR-0042 rule 3.3). Answering a departure
question inside one destination makes that destination the default answer,
and it usually is not.

## Decision

1. **A fifth top-level entry, Home, at `/`.** It replaces the redirect to
   `/estate` and is the console's landing. Its question is "where do I
   look first?", and it is the one entry named for a place rather than an
   activity, because the activity it serves is choosing which activity.
   ADR-0042 §1's inventory becomes five and stays closed: a sixth is a
   decision, not a convenience.

2. **Home derives; it never judges.** Every number on it comes from
   endpoints the four Workspaces already read (`/api/v1/estate`,
   `/api/v1/rollouts`), through the modules that already own each
   judgement: `estate/rollup.ts` for ratio-plus-worst, `estate/order.ts`
   for card standing and ordering. There is no `/api/v1/home`. A summary
   with a verdict of its own could disagree with the surface it points at,
   and a reader would have no way to tell which of the two was right.

3. **No blended number, at estate grain either.** ADR-0017's rule is not
   relaxed by aggregating further: ratio-plus-worst per finding kind, the
   waived count always alongside, neutrals counted and never hidden. Home
   shows the root row of the same roll-up the tree-table draws, so the two
   surfaces cannot drift. A single estate health score is the one thing
   this surface must not grow.

4. **Every element is a door, and every door carries its filter.**
   ADR-0042 rule 3.4 said collector counts open the flat list pre-filtered;
   Home generalises it to everything it draws. A worst Tier opens Estate
   with that card selected, a Team opens the shelf scoped to it, the
   ungoverned count opens the flat list filtered to the ungoverned
   population, a Rollout opens the ledger. An element that is not a door is
   a review comment.

5. **Home is bounded where the shelf is not.** It shows a fixed number of
   worst Tiers and names how many it did not show, with the door to the
   surface that shows all of them. Truncation that does not say it
   truncated reads as an all-clear, which is the sin the denominator rule
   already forbids (ADR-0017, ADR-0031).

6. **Home holds no state of its own.** The Environment lens is its
   evaluation context exactly as it is the roll-up's (ADR-0042 §4), with
   the all-Environments count beside every lens-judged one so the lens
   hides nothing. There is no selection, no filter, and no view-switcher,
   so ADR-0042 §3.5 is satisfied by having nothing to address.

## Consequences

- The glossary's **Workspace** entry names five areas, and the four-area
  wording in `docs/contributing/console.md` follows it.
- `/` stops redirecting. The static-site assembler special-cases it,
  because `index.html` is already the document that URL resolves to; the
  pre-render loop would otherwise write a file named `.html`.
- Home reads two endpoints where the four Workspaces read one each, so it
  is the first surface whose payload is two queries. Both are already
  cached by TanStack Query for the Workspaces that follow, so the cost is
  paid once and the departure is warm.
- Home is the one Workspace imported eagerly rather than through a dynamic
  import (issue #125). A landing page behind a dynamic import puts a round
  trip in front of the first paint on the most common entry, which is the
  one place a waterfall is least affordable. Measured: the entry chunk goes
  from 121.6 kB gzipped to 125.1 kB, and the demo bundle from 126.3 kB to
  129.5 kB, against the 140 kB ceiling. That spends 3.5 kB of headroom
  reserved for Tours (ADR-0051), and it is recorded here for the same
  reason raising the ceiling would be: it is a shared budget.
- The demo answers Home for free: it composes only endpoints
  `src/api/demo.ts` already projects from the snapshot, and a snapshot
  carrying no Rollouts renders the section empty rather than absent.
- The risk is that Home becomes the place things are added to when no
  Workspace wants them, which is what ADR-0048 §4 says about the primitive
  layer and is true of landing pages twice over. Decisions 4 and 5 are the
  budget: one question, every element a door, truncation always named.

## Sources

- ADR-0042 §1, §2, §4 and rule 3.4; ADR-0017; ADR-0041 §2; ADR-0029;
  ADR-0031; ADR-0035; ADR-0048 §4.
