# ADR-0049: Releases are tags on main; the public demo follows a moving pointer

- Status: accepted
- Date: 2026-08-20

## Context

Nothing in this repository has ever been released, and two pieces of work
stop at that same absence.

Issue #86 wants the public demo built from a release rather than from `main`,
because `demo.telecraft.dev` is the first thing most people see of the
product and a bug that reaches `main` reaches them. The wiring is already
there: `estate-demo` takes a `telecraft_ref` and falls back to `main`, so the
pin is a one-line change once there is something to pin to.

ADR-0047 §1 distributes the token and base sheets "as a versioned artefact
both repositories consume", and its consequences say the release of those
sheets is "a real task, not a copy". The documentation site is built in a
second repository and cannot import a React component, but it can consume a
stylesheet — provided the stylesheet has a version.

Three facts constrain the answer. The product is mid-build: `docs/plan.md`
runs to P5 and the platform API documents, the authored file formats and the
CLI flags are all still moving. Both consumers build this repository from a
git ref rather than consuming a binary — `estate-demo` checks out the
platform and runs `go run ./cmd/telecraft`, and the quickstart builds the CLI
with `go build`. And `estate-demo` builds on three events (a push to its own
estate, a manual run, and a repository dispatch from here), only one of which
can carry a ref, which turns out to decide the shape of the pin.

## Decision

1. **Tags are `vMAJOR.MINOR.PATCH`, and the major version is zero until it
   has earned not being.** While it is zero, minor covers anything a
   consumer can notice — a flag, a platform API document, a token name, a
   file format, a removal — and patch covers a fix behind no documented
   surface. `v1.0.0` is the release where the platform API documents and the
   authored file formats are ones we intend to keep, which is a state and
   not a date. Claiming a stable major now would be a promise about
   documents that P5 has not finished arguing about. A tag never moves once
   pushed; a wrong release is corrected by the next number.

2. **A release is cut by pushing an annotated tag on a commit that is on
   `main`.** `release.yml` does the rest, and refuses two things: a tag that
   is not `vMAJOR.MINOR.PATCH` with an optional pre-release suffix, and a tag
   that does not sit on `main`. Both guards exist because a tag push has no
   review behind it. A workflow that created the tag from a button was
   considered and rejected: it would either decide the number itself, which
   nothing can do correctly, or take it as an input, which is the same typing
   behind more machinery.

3. **A release carries the source at the tag and one design artefact, and
   nothing else yet.** `telecraft-design-<version>.tar.gz` holds
   `tokens.css`, `base.css` when it exists, `fonts/fonts.css` and the faces
   with their OFL texts, laid out as a site serves them, with `SHA256SUMS`
   beside it. It is packed reproducibly so the checksum is a fact about the
   tag, and the palette floors (ADR-0047 §4) are checked over the bytes being
   shipped rather than over the copy in the tree. Absent, deliberately:
   prebuilt binaries, because the documented way to get the CLI is to build
   it and publishing binaries buys a platform matrix nothing has asked for;
   the console bundle, because the console is built per deployment mode
   (ADR-0045) and one prebuilt bundle would be wrong for at least one
   consumer; and a Catalogue, because ADR-0020 §5's embedded catalogue needs
   an installable instance to be embedded in and there is not one yet. That
   last is recorded rather than dropped: it is the first thing this scheme
   grows when P3 lands.

4. **The demo builds from a release, and between releases it lags.** No
   staging demo. A second site is a second deployment, a second domain and a
   second published claim to keep honest, and what it would catch is already
   caught earlier: `ci.yml`'s demo job builds the snapshot and the demo
   bundle on every pull request that touches them, so a change is exercised
   on a real surface before it merges. Issue #86 called lagging "the simpler
   answer and probably the right one to start with", and nothing found while
   wiring this argues otherwise.

5. **The pin is a moving `release` tag, and it is the only ref here that
   moves.** `demo-dispatch.yml` moves it to each new stable version and then
   asks `estate-demo` to rebuild; `estate-demo`'s fallback names `release`
   once and never needs editing again. The alternatives both fail on the same
   fact. A literal version in `estate-demo` costs an edit in a second
   repository on every release, and leaves the dispatch asking for a rebuild
   at a ref that has not moved. Carrying the version in the dispatch payload
   fails harder: a repository dispatch is one of three events over there and
   the other two carry nothing, so an estate content change would build
   against the fallback and the platform version on the public site would
   depend on which event fired last. Whatever the fallback names is the pin,
   so the fallback has to name something that stays current.

6. **A pre-release tag publishes artefacts and never moves the pointer.**
   `v0.3.0-rc.1` exercises the whole release path without the public demo
   moving, which is what makes the path rehearsable at all. The suffix is the
   entire signal, in both workflows.

## Consequences

- `estate-demo`'s `demo.yml` changes one line, from `|| 'main'` to
  `|| 'release'`. It must not land before the pointer exists, or every demo
  build fails on a ref that is not there.
- A fix that is visible on the demo now reaches it when someone cuts a
  release. That is the price of the demo being a claim about the product
  rather than a preview of it, and it makes the release cadence a public
  thing rather than an internal one.
- `demo-dispatch.yml` no longer fires on pushes to `main`. The gap it was
  written to close — a console change lands, the demo goes on serving the
  build before it, and nothing anywhere says so — is closed differently now:
  the demo is not meant to match `main`, so it cannot silently fail to.
- Nothing on the demo says which release it is built from. Its chrome carries
  the estate's provenance, not the platform's version, and `estate-demo` is
  the only place that could show the lag to a reader. Worth doing there;
  not decided here.
- `release` is mutable, and nothing records where it pointed last week.
  "Which release was the demo built from" is answered by `git rev-parse
  release` for now and by the `estate-demo` run log for then, not by a file
  in a repository's history. That is the cost paid for not editing a second
  repository on every release, and it is only acceptable because the version
  tags stay immutable and the pointer is never an input to anything kept.
- The documentation site pins a version and re-pins deliberately. The sheets
  change rarely, so a pin can sit across several releases; the archive
  checksum is how that repository tells whether they moved at all.
- `release.yml` trusts `main` to be green rather than re-running the suite.
  Nothing stops a tag on a commit whose CI failed, and nothing would catch
  it. Querying the tagged commit's check runs before publishing is the
  tightening available if that ever bites.
- No new capitalised domain terms, so the glossary is unchanged. A release,
  a tag and a pointer are the industry's words for the industry's things.

## Sources

- Issue #86, which named the sub-decisions and carried the staging question.
- ADR-0002, ADR-0013, ADR-0019, ADR-0020 §5, ADR-0045 §4, ADR-0047 §1 and its
  consequences.
- `docs/branding/design-system.md`, "Distribution".
- `telecraft-dev/estate-demo`, `.github/workflows/demo.yml` as it stood on
  2026-08-20: three triggers, one of which can carry a ref.
- `docs/plan.md`, build phases P0 to P5.
