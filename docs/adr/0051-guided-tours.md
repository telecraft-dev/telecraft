# ADR-0051: Guided Tours — authored Steps over the running console, addressable, and never driving

- Status: accepted
- Date: 2026-08-21

## Context

The console is eleven surfaces over a vocabulary a reader has to have been
taught. A card carries three bands whose reds are deliberately distinct
(ADR-0041 §2); the shelf's resting scope is a team subtree rather than the
estate (ADR-0042 §2); the environment lens is emphasis and never a filter
(§4); every authoring exit is a pull request and not a write (ADR-0003,
ADR-0028). None of that is guessable from the pixels, and all of it is the
product's argument.

Two populations arrive without it. A new adopter's first engineer, signed in
against their own estate; and every visitor to `demo.telecraft.dev`, which
ADR-0049 makes the product's front door and which is, deliberately, the real
console over a real estate rather than a screenshot. Today both land on the
shelf with nothing to read.

The documentation answers all of it, and lives in another repository. Sending
a first-time reader there costs them the thing they came to look at.

Nothing in the corpus covers teaching inside the product, and the two words
that look free are not. "Onboard" is already the ungoverned collector's route
into governance (ADR-0031, the claim flow, OQ-3); a **Grace Period** is a
Service's onboarding window (ADR-0015). Neither is a human learning the
console, and the glossary's terms are binding rather than reusable.

Two budgets also bind. ADR-0042 §1 closes the v1 surface inventory, so
whatever teaches must not become a twelfth surface. ADR-0048 sets the
console's primitive layer at four components and says a fifth is a decision,
not a convenience.

## Decision

1. **A Tour is authored data, not a surface.** A **Tour** is an ordered list
   of **Steps**; a Step carries its prose, an optional anchor naming an
   element on a surface, and an optional destination — a route and its search
   params. Tours live in `console/src/tours/` and are registered in one list.
   Writing a Tour is authoring a file; it builds nothing. One runner in the
   chrome renders every Tour, assembled from the ADR-0048 primitives and the
   dialog the jump-to-object search already uses, so the surface inventory is
   untouched and the primitive budget is unspent.

2. **A Tour narrates the product; it never drives it.** A Step may navigate,
   because a destination is a URL and this console already treats a URL as
   state. It may point at an element. It may not click a control, fill a
   field, propose anything, or show invented data: everything a Tour points at
   is the running console over the estate the reader is signed in to. The
   overlay follows from that rule rather than decorating it — an anchored Step
   takes no pointer events, so the product underneath stays usable and a
   reader may simply ignore the Tour and carry on.

3. **A Tour's position is in the URL, like every other console state.**
   `tour` and `step` are root search params (ADR-0042 §3.5). "Step four of the
   welcome Tour" is citable in a pull-request comment exactly like a traced
   Path or a filtered list, which is how a confusing surface gets reported.
   They survive a Workspace switch, because a Tour belongs to the console and
   not to a Workspace: the environment lens's rule (§4), not the object
   selection's (§1).

4. **A missing anchor degrades; it never breaks.** An unknown Tour id, a step
   index past the end, an anchor no surface carries, a surface still loading:
   the Tour runs, the Step renders centred, and nothing throws. Teaching is
   never load-bearing — a console that cannot explain itself is worse, and a
   console that fails because it tried to is unacceptable. Degradation is not
   the same as permission: the Playwright suite walks every Step of every
   registered Tour and fails on an anchor that resolves nowhere, so a lost
   anchor is a red build rather than a quiet hole in production.

5. **An anchor is a declared contract, `data-tour`, and never a test id.** A
   `data-testid` is a test's grip on an element and may be renamed whenever
   the test is; a `data-tour` is authored content a reader is shown. Sharing
   one attribute would make a test-only rename a silent product regression.

6. **What a reader has already seen is presentation state.** This amends
   ADR-0042 §7 and nothing else: the closed key list grows by one,
   `toursSeen`. It qualifies on that section's own terms — per user,
   presentation only, and fully loseable, because losing it offers the welcome
   again, which is the correct way for this to fail.

7. **The welcome is the first Step of a Tour, not a second mechanism.** A Step
   with no anchor renders centred over a scrim, which is exactly the welcome
   modal that would otherwise have been built beside this. One mechanism, two
   placements. It opens once per user and only on a bare landing URL: a link
   someone shared carries their context, and a Tour never lands on top of it.

8. **The welcome Tour is demo-aware in exactly one Step.** The demo needs its
   first paragraph to say what the reader is looking at — a snapshot of a
   public estate, read-only by construction (ADR-0049, issue #50) — and an
   instance needs it to say the opposite. Every other Step is identical,
   because the demo is not a different console.

## Consequences

- The documentation keeps its job and the Tour keeps a smaller one: a Tour
  teaches *this console*, and the documentation teaches the product. A Step
  that starts explaining what an Expectation is has become documentation in
  the wrong repository. The word budget — roughly sixty words to a Step — is
  the check on that, and it is enforced by review rather than by a lint.
- A surface that renames or drops a `data-tour` anchor breaks a Tour, and
  §4's walk says so in CI.
- `console/src/tours/` is somewhere Tours can accumulate carelessly, in the
  way ADR-0048 warned the primitive layer could. A Tour is a decision about
  what a reader most needs next, not a changelog for a release.
- Because the overlay is non-blocking, a reader can act mid-Tour and change
  what a later Step points at. That Step degrades to centred under §4, which
  is the right outcome: the Tour was never the authority on what is on screen.
- Two exceptions to §3 already exist for device preferences (ADR-0047 §2,
  ADR-0048 §3). `toursSeen` is not a third: it records what a reader has been
  shown, never what is on screen, and the position itself stays in the URL.

## Sources

- ADR-0003, ADR-0028, ADR-0031, ADR-0041, ADR-0042, ADR-0043, ADR-0045,
  ADR-0047, ADR-0048, ADR-0049.
- OQ-3 and the claim flow, for the vocabulary collision "onboarding" would
  have caused.
- `console/README.md` (the search-param contract), `docs/glossary.md`.
