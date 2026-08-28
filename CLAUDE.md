# Working in this repository

Read this before writing anything a user will see, and before changing a
decision.

## Words a user sees

Console surfaces, CLI help and output, error messages, findings and their
remediation text, and every published page are held to one house style. It is
in [Writing documentation](docs/contributing/documentation.md#house-style),
and the extra rules for product surfaces are in
[Interface text](docs/contributing/documentation.md#interface-text). Read both
rather than guessing from the surrounding code, because the surrounding code
is not always right yet.

The four that are easiest to break without noticing:

- **British English, and no em dashes anywhere**, including code comments,
  test messages, and fixtures. `tools/vendorlint` fails the build on a dash.
- **No decision references on screen.** `ADR-0042`, `REQ-031`, `§3` and issue
  numbers live in code comments and in `docs/adr/`, never in text a user
  reads.
- **No rationale on screen either.** A surface reports; it does not defend
  itself. If a sentence argues for a design decision rather than stating a
  reading, delete it and put it in the code comment beside the thing it
  explains. Prose that argues reads as a prototype.
- **Plain words outside the glossary.** Governed domain nouns are exact and
  stay exactly as written (Tier, Environment, Rollout, Blueprint, Service
  Class, and the rest of [the glossary](docs/glossary.md)). Everything else is
  where jargon collects: prefer the plain word, and where the glossary already
  names the concept, use its name.

When you change a term, change it on every surface that shows it. Two surfaces
naming one thing two ways is a defect even when each reads well alone.

After writing a surface, re-read every string as somebody who has never opened
a decision record. Anything that argues, instructs, or names an internal
concept is a finding on your own work.

## Decisions

Decisions live in `docs/adr/`, one per file, and the process is in
[Decisions](docs/contributing/decisions.md). An accepted ADR is not edited to
change its mind: write a new one that supersedes or amends it, and say which
in its `Status` line. ADR-0054 and ADR-0056 are the worked examples of an
amendment.

If a task contradicts an accepted ADR, say so before building, then build what
was asked once the decision is confirmed. Silently contradicting the corpus is
worse than either.

Every capitalised domain term an ADR introduces needs a glossary entry.

## Before you say it works

Run what CI runs. [CI](docs/contributing/ci.md) is authoritative; the short
form for a console change is:

```sh
cd console
npm run typecheck && npm test && npm run build
npm run check:zero-cdn && npm run check:bundle-budget && npm run check:palette
npm run e2e
```

and from the repository root, for any change that touches `docs/` or adds
prose anywhere:

```sh
go run ./tools/docslint
go run ./tools/vendorlint
```

`npm run e2e` starts its own fixture backend on port 4700. Stop any backend or
dev server you left running first, or the run reuses the wrong one and the
failures make no sense.

Report what actually happened. A check that was not run is not a check that
passed.

## Releases

A release is an annotated `vMAJOR.MINOR.PATCH` tag pushed on a green
`main`; the workflows do the rest, and the public demo follows the moving
`release` pointer a few minutes later, behind a cache, so an open tab
showing the old build proves nothing. The sizing table, the cut steps and
the verification list are in
[Releases](docs/contributing/releases.md); while the major is zero,
anything a consumer can notice is a minor. Never cut a release unasked,
and never move a version tag: a wrong release is corrected by the next
number. `release` is the one ref that moves.
