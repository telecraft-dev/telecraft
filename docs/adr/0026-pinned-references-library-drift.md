# ADR-0026: Pinned references with opt-in tracking; `satisfies` mechanics; `library_drift` defined

- Status: accepted (amends ADR-0016 §3)
- Date: 2026-08-14 (session G3)

## Context

ADR-0016 §3 promised "when the owning team changes it, every consumer
re-renders": track-head semantics. ADR-0023 §2 pins Blueprint versions per
environment. Left unreconciled, shared-Component updates would propagate
into production configs the consumer never reviewed. Separately, `satisfies`
("a claim of intent, never of fact") and the `library_drift` outcome existed
without mechanics; OQ-1 had already noted library versioning "may be the same
problem".

## Decision

1. **Shared-Component references pin a version by default**
   (`infosec/pii-redaction@3`). Opt-in `track: head` per reference restores
   ADR-0016's auto-propagation for consumers who want it.
2. **A pinned reference behind the owner's head raises a finding**
   ("component update available"), counted in compliance roll-ups
   (ADR-0017), routed to the consuming Blueprint's owner, never a block
   (ADR-0022 §3 stands). The console shows the pinned-vs-head config diff
   with a guided update that produces a **PR bumping the pin**: git is the
   source of truth, there is no live mutation.
3. **This amends the letter of ADR-0016 §3**: automatic consumer re-render
   now holds only for tracking references. The spirit survives: pinning a
   *version* is still reference-not-copy. Accepted consequence, stated
   plainly: an owning team shipping a critical fix cannot force propagation;
   the pressure is findings plus the organisational conversation. If that
   proves too weak, an owner-marked "critical" version escalating finding
   severity is an additive later fix.
4. **`satisfies` claims are version-stamped** (`req-payment-completeness@3`),
   stamped by the composer at save. Unversioned claims cannot drift
   detectably.
5. **The evaluator always judges against the requirement's current
   version**: a claim is never a way to freeze the goalposts.
6. **`library_drift` finally means one thing**: the subject *passes the
   version it claims or pins but fails the current one* ("the goalposts
   moved and you haven't caught up"), distinct in diagnosis and remediation
   (the version diff) from "you never complied". Fails both → the ordinary
   failure outcome. Passes both with a stale claim → housekeeping nudge, not
   an outcome.
7. **One finding kind, two facets.** Requirement-behind and Component-behind
   are the same kind, `library_drift`, with the referenced-object kind
   (Requirement | Component) carried as a facet. One mental model ("you
   reference version N; the world is at M"), one roll-up column per
   ADR-0017; the flat estate list (OQ-14) slices by facet.

## Consequences

- The composer surfaces update-available state per reference; the update
  action is a PR, reviewable like any change (ADR-0028).
- REQ-025 and REQ-031 map here; ADR-0004's outcome list is unchanged in
  membership, sharpened in meaning.

## Sources

- Session G3; ADR-0016, ADR-0017, ADR-0022, ADR-0023; OQ-1's note.
