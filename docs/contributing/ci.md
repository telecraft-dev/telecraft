---
title: Continuous integration
description: The four workflows, what decides which jobs run, how the live-backend suites are gated, and how a change reaches the public demo and the documentation site.
order: 3
---

# Continuous integration

Four workflows live in `.github/workflows/`. One guards changes, one
publishes releases, and two tell downstream repositories that something
they build from has moved.

| Workflow | Fires on | Does |
|---|---|---|
| `ci.yml` | every pull request, and every push to `main` | Builds, tests and lints — the checks review waits for |
| `release.yml` | a `v*` tag | Publishes the release and the design artefacts |
| `demo-dispatch.yml` | a `v*` tag, or a manual run | Moves the `release` pointer and asks the demo to rebuild |
| `docs-dispatch.yml` | a push to `main` touching `docs/**` | Asks telecraft.dev to rebuild the documentation |

Nothing here deploys the product. Telecraft has no hosted instance to
deploy to: an adopter runs it themselves, and the two public surfaces —
[demo.telecraft.dev](https://demo.telecraft.dev) and telecraft.dev — are
built by the repositories that own them, from a ref this one publishes.

## What decides which jobs run

`ci.yml` opens with a `changes` job that diffs the pull request against its
base and sets two outputs. Every other job carries an `if:` naming one of
them.

| Output | True when the change touches |
|---|---|
| `code` | anything that is not `docs/**`, `README.md`, or `docs-dispatch.yml` |
| `console` | `console/**`, `internal/console/**`, or `ci.yml` itself — and otherwise inherits `code` |

The reason is cost rather than tidiness. The live suites stand up a real
Elasticsearch and open real pull requests against a fixture repository;
running them because someone fixed a typo in a guide spends a service
container and leaves external side effects behind for no reading. When the
runners are slow, that wait is the whole review latency.

**Two lints are the deliberate exception.** Both run on everything,
always, because both scan `docs/**`, which makes a documentation-only
change exactly the change that can break them. The vendor-word lint reads
code and prose alike, and a vendor word arrives as easily through a guide
as through a Go file (ADR-0001). The front-matter check reads the
published pages, and it exists because **the site is built in a different
repository** (issue #74): the front matter and `docs/nav.yaml` are the
whole contract between here and there, so a block that does not parse
fails over there, after merge, in a build nobody here is watching. That is
not hypothetical — `docs/reference/estate-layout.md` carried an unquoted
colon in its description and took the whole documentation build down.

Two details worth knowing before you rely on this:

- **A skipped job reports success.** That is what makes the gating safe to
  put behind branch protection: a required check that is skipped is not a
  check that is missing. A workflow that never *ran* would leave the check
  pending forever, which is why the filtering is at job level and not on
  the workflow's own trigger.
- **An unusable diff base runs everything.** A first push, or a force-push,
  can leave `before` naming a commit the runner cannot resolve. Rather than
  guess at a range, `changes` sets both outputs true. Over-running is a
  cost; under-running is a hole.

## The checks

| Job | Runs | Guards |
|---|---|---|
| What changed | a diff against the base | Nothing. It decides what the rest of the table does |
| Vendor-word lint (ADR-0001) | `go run ./tools/vendorlint` | The neutral core holds: no vendor word in `cmd/`, `internal/`, `console/` or the normative docs, and provider implementations stay product-qualified |
| Documentation front matter | `go run ./tools/docslint` | Every published page carries front matter the site can read — the contract between this repository and the site built from it |
| Build and test | `go build ./...`, `go vet ./...`, `go test ./...` | The core compiles, vets and passes its unit tests — with no Docker and no network |
| TelemetryProvider live | the `Live` suite against a single-node Elasticsearch service container | The telemetry queries work against a real backend, not only a test double |
| Forge adapter live | the `Live` suite against the GitHub App and `estate-fixture` | The pull-request flow works against the real forge API |
| Console (ADR-0045) | `typecheck`, `test`, `check:palette`, `build`, `check:zero-cdn`, `e2e` | The console typechecks, its unit and Playwright suites pass, the palette clears its floors, and the built bundle reaches no external host |
| Demo snapshot and bundle | `build:demo`, `check:zero-cdn`, `telecraft snapshot`, the entry-document check | The public demo's two halves keep working: the snapshot the real evaluators produce, and the console bundle that reads it |

Four of those guard rules that are otherwise unenforceable in review.

**The vendor-word lint** exists because ADR-0001's neutral core is a
property of the whole tree, and no reviewer reads the whole tree.

**The front-matter check** exists because its failure lands in someone
else's repository. A reviewer here sees a sentence changed in a guide and
has no reason to think about YAML.

**`check:zero-cdn`** runs over the *built bundle*, not the source, because
the air-gap rule is about what the browser fetches and a bundler can
introduce a request no import statement shows (ADR-0019, ADR-0045 §5). In
the demo job it deliberately runs *before* the snapshot is placed beside
the bundle: a snapshot carries endpoints and module paths the console
prints and never fetches, and it is estate data rather than a bundled
artefact.

**`check:palette`** exists because ADR-0047's accessibility floors are
numbers, and a number can be regressed. It resolves both themes, checks
every token against the ground it sits on, measures the severity triad
under simulated deuteranopia and protanopia, and enforces the rule that
every colour is defined in exactly two blocks and never inside a media
query. Before it existed, `docs/branding/design-system.md` recorded the
method and the expected values — and the documented palette turned out not
to clear its own floors, which is precisely the failure a document cannot
catch.

## The live suites, and why they can be green without credentials

Both live jobs follow the conformance-kit discipline of ADR-0036: **absent
credentials never fail a suite.** The Forge suite skips loudly — printing
what it would have needed — when the `FORGE_*` secrets are not exposed, so
a fork's pull request stays green rather than failing on something the
contributor cannot provide.

This is a deliberate trade. It means a credential that quietly stops
working reads as a pass, so the log is the only place that says the suite
skipped. When you change provider code, read it rather than trusting the
tick.

The same suites run locally; the [development page](development.md) has
the environment variables, and `internal/provider/telemetry/demo.sh` stands up the backend
the Elasticsearch suite wants.

## Reproducing a failure locally

Every check is a command you can run yourself — there is nothing CI does
that the repository cannot.

```sh
go build ./... && go vet ./... && go test ./...
go run ./tools/vendorlint
go run ./tools/docslint

cd console
npm ci
npm run typecheck && npm test && npm run check:palette
npm run build && npm run check:zero-cdn
npm run e2e
```

If `npm run e2e` fails on a missing browser, `npx playwright install
chromium` fetches it. **Do not add `--with-deps`.** CI leaves it off on
purpose: it shells out to `apt-get` against the Ubuntu mirrors, and
measured over three consecutive runs it cost 138s, 362s, and then hung
past 25 minutes — against 14s for the suite it exists to enable. The
GitHub-hosted image already ships the shared libraries Chromium needs, and
a missing library fails the next step immediately with its name, which is
a better failure than a hang.

## How a change reaches the public surfaces

Neither public site is deployed from this repository. Both build from a ref
here, and both are told when that ref moves.

**The documentation site.** A push to `main` touching `docs/**` makes
`docs-dispatch.yml` send a repository dispatch to
`telecraft-dev/telecraft.dev`, which rebuilds. The navigation comes from
`docs/nav.yaml`, so a new page is published by adding it here and nothing
else (see [writing documentation](documentation.md)).

**The demo.** `demo-dispatch.yml` fires on a `v*` tag, not on a push, so
the demo builds from a release and a bug on `main` cannot reach it
(ADR-0049 §4, issue #86). Between releases the demo lags, deliberately —
there is no staging site, because `ci.yml`'s demo job already builds the
snapshot and the demo bundle on every pull request that touches them, and
a staging site would be a second deployment and a second public claim to
catch what CI catches first.

The ref the demo checks out is a **moving `release` tag** rather than a
version. That is not the obvious design and it is worth understanding
before changing it: `estate-demo` builds on three events — a push to its
own estate, a manual run, and the dispatch — and only the manual run can
carry a ref. Whatever its fallback names is therefore the pin for *all
three*, so a ref travelling in the dispatch payload would mean an estate
content push building against something else, and which platform version
the public site ran would depend on which event fired last.

So `demo-dispatch.yml` reads a tag and reaches one of three conclusions:

| Tag | Pointer | Demo |
|---|---|---|
| the newest stable version | moves to it | rebuilds |
| an older stable version | unmoved | unchanged — a fix on an older line must not drag the demo backwards |
| a pre-release (`v0.1.0-rc.1`) | unmoved | unchanged |

That third row is what makes the release path rehearsable: a pre-release
exercises `release.yml` end to end and publishes its artefacts while the
public demo stays exactly where it is (ADR-0049 §6). The version tags stay
immutable; `release` is the only ref that moves, and `git rev-parse
release` answers what the demo is built from.

Cutting a release is [its own page](releases.md).

## Changing a workflow

A change to `ci.yml` sets `console` true, so the console and demo jobs run
against their own new definition rather than the previous one.

Workflows in this repository carry their reasoning in comments, at more
length than is usual. That is deliberate: a CI file is read most often by
someone under time pressure trying to understand why it is failing, and
the decisions in these files — the missing `--with-deps`, the skipped-is-
success gating, the moving pointer — all look like mistakes until the
reason is at hand. Keep that up in anything you add.
