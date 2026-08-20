---
title: Contributing to Telecraft
description: How to propose a change, what review expects of it, and what continuous integration guards.
order: 1
---

# Contributing to Telecraft

Telecraft is a Go core with a TypeScript and React console, built against a
design corpus that settles most questions before code is written. This
section is the developer's half of the documentation: everything about
building the product, rather than using it.

Read the page that matches what you are doing:

- [Development](development.md) covers prerequisites and every build, test
  and lint command that CI runs.
- [Continuous integration](ci.md) covers the four workflows, what decides
  which jobs run, and how a change reaches the public demo and the
  documentation site.
- [Architecture](architecture.md) maps the packages, draws the neutral core
  boundary, and follows a verdict from authored files to output.
- [Providers](providers.md) explains how to implement each seam and pass its
  conformance kit.
- [Console](console.md) covers the four Workspaces, the card data contract,
  the canvas engine, and the zero-CDN rule.
- [Decisions](decisions.md) explains the ADR process and maps the 51 ADRs by
  theme.
- [Documentation](documentation.md) explains how this documentation system
  works and how to add a page.
- [Releases](releases.md) explains what a version number means, what a
  release contains, and how to cut one.

## Before you write code

Most questions have an answer in the corpus already. Find the decision that
governs the area you are touching, and read it before you change anything:
the [decisions page](decisions.md) groups the ADRs by theme, and
`docs/requirements/traceability.md` maps each requirement to the ADRs that
decide it.

If you disagree with a decision, that is a new ADR, not a code change. An
accepted ADR is amended by a superseding ADR, never edited in place.

If your change needs a term the corpus does not have, add it to
`docs/glossary.md`. Every capitalised domain term used in an ADR has a
glossary entry, and ADR-0015 fixes the current vocabulary: Tier, Service
Class, Sensitivity, Effective, and Service. The words those replaced are
errors, not synonyms.

## Issues

Work is tracked in the
[issue tracker](https://github.com/telecraft-dev/telecraft/issues). Open an
issue before a substantial change, so the design conversation happens before
the diff exists.

A good issue states what the change is, which requirement or ADR it serves,
and what "done" looks like as acceptance criteria a reviewer can check. Small
fixes (a typo, a broken link, a failing edge case with an obvious cause) need
no issue: open the pull request.

## Branches and pull requests

Branch off `main`. Nothing lands on `main` except through a pull request, and
`main` stays green.

Name the branch after the work, with the build phase or the documentation
section as its prefix: `p6/demo-deep-links`, `docs/contributing`. If you do
not have write access to the repository, push the branch to a fork and open
the pull request from there.

Write commit subjects as an area, a colon, and what the commit does:

```text
renderer: OTLP-push self-telemetry in every artefact with the Tier stamp
console: an entry document per Workspace URL, so deep links answer 200 (Refs #50)
```

The area is the package or component the change belongs to (`renderer`,
`console`, `card`, `ci`). Reference the issue as `(issue #34)` or
`(Refs #50)`, and the decision as `(ADR-0041 §4)`, when either explains why
the change looks the way it does. Commit in logical increments: one commit
per idea, not one commit per file and not one commit per branch.

What review expects:

- **A decision behind the change.** Anything that establishes a rule, a
  vocabulary, or a seam cites the ADR it follows, or ships a new ADR.
- **Tests that fail without the change.** New behaviour arrives with the test
  that proves it, and a bug fix arrives with the test that reproduced it.
- **No partial work.** A merged change works end to end. Placeholder
  functions, stubs, and unimplemented branches do not merge.
- **Green CI.** Every check that ran passes before review, not after it.
- **House style in user-visible prose.** Documentation, error messages, and
  UI text follow the rules on the
  [documentation page](documentation.md#house-style).

## What CI checks

Every pull request runs `.github/workflows/ci.yml`. A first job diffs the
change and decides what the rest of them do, so a documentation-only pull
request does not stand up Elasticsearch or install a browser — and a
skipped job reports success, which is what makes that safe. The
[continuous integration page](ci.md) explains the gating, the live suites,
and the workflows beyond this one.

Each remaining job guards something specific:

| Check | What it runs | What it guards |
|---|---|---|
| Build and test | `go build ./...`, `go vet ./...`, `go test ./...` | The core compiles, passes vet, and passes its unit tests, with no Docker and no network. |
| Vendor-word lint (ADR-0001) | `go run ./tools/vendorlint` | The neutral core holds. No vendor word appears in `cmd/`, `internal/`, `console/` or the normative docs, and provider implementations stay product-qualified. |
| Documentation front matter | `go run ./tools/docslint` | Every published page carries front matter the documentation site can read. The site is built in another repository, so a malformed block fails there rather than here. |
| Console (ADR-0045) | `npm ci`, `npm run typecheck`, `npm test`, `npm run check:palette`, `npm run build`, `npm run check:zero-cdn`, `npm run e2e` | The console typechecks, its unit tests and Playwright suite pass, the design tokens clear their contrast and colour-vision floors (ADR-0047), and the built bundle reaches no external host. |
| TelemetryProvider live (Elasticsearch) | `go test ./internal/provider/telemetry/ -run Live -v -count=1` against a single-node Elasticsearch service container | The telemetry queries work against a real backend, not only against a test double. |
| Forge adapter live (GitHub App) | `go test ./internal/provider/forge/ -run Live -v -count=1` | The pull-request flow works against the real forge API. The suite skips loudly when the credentials are absent, so the job stays green without them. |
| Demo snapshot and bundle (issue #50) | `npm run build:demo`, `npm run check:zero-cdn`, `go run ./cmd/telecraft snapshot`, and the entry-document check | The public demo's two halves keep working: the snapshot the real evaluators produce, and the console bundle that reads it. |

Run all of them locally before you push. The
[development page](development.md) lists each command and what it needs.

## Where to ask questions

Ask in the [issue tracker](https://github.com/telecraft-dev/telecraft/issues):
open an issue for a question about the product or the design, or comment on
the issue or pull request the question belongs to. The repository has no
discussion forum, so the tracker is the one place a question and its answer
stay findable.

If the answer turns out to be a decision, it becomes an ADR. If it turns out
to be missing documentation, it becomes a page in this section or in the
user-facing sections.
