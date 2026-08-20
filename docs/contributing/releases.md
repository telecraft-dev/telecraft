---
title: Releases
description: What a Telecraft version number means, what a release contains, how to cut one, and how the public demo follows it.
order: 8
---

# Releases

A release is a tag on `main`, the artefacts attached to it, and a promise
that the code at that tag is the code other repositories may build from.
Telecraft has two consumers outside this repository, and both of them are the
reason releases exist:

- **The public demo** at `demo.telecraft.dev` is built in
  `telecraft-dev/estate-demo`, which checks out this repository and runs its
  evaluators over a curated estate. It builds from a release, so a bug on
  `main` cannot reach it.
- **The documentation and marketing sites** consume the design tokens and
  the self-hosted typefaces, which live here and are published as a release
  artefact rather than copied (ADR-0047 §1).

The decision behind all of this is
[ADR-0049](https://github.com/telecraft-dev/telecraft/blob/main/docs/adr/0049-releases-tags-on-main-and-the-demo-pin.md).
Read it before you change the scheme; this page tells you how to work it.

## What a version number means

Tags are `vMAJOR.MINOR.PATCH`, and Telecraft is pre-1.0. That is not
modesty about quality. It is a statement about the platform API documents,
the authored file formats, and the CLI flags, none of which are ones the
project is willing to freeze yet.

While the major version is zero:

| Part | Increments when |
|---|---|
| Minor (`v0.3.0`) | Anything a consumer can notice moves: a CLI flag, a platform API document, a token name, a file format, or a behaviour a reader could have relied on. Removals live here too. |
| Patch (`v0.3.1`) | A fix that changes no documented surface. |

A tag never moves once pushed. If a release is wrong, the fix is the next
number, not a new commit under the old name. The one exception is `release`,
which is a pointer rather than a version, and is described below.

`v1.0.0` is not a date. It is the release where the platform API documents
and the authored file formats are ones the project intends to keep, which
`docs/plan.md` puts at the far end of the build phases. Until then, read a
version number as "this is what changed", not as a compatibility contract.

A tag may carry a pre-release suffix: `v0.3.0-rc.1`. A pre-release publishes
its artefacts and never reaches the public demo, so it is how you rehearse a
release, or hand someone a build to test, without moving what the world sees.

## What a release contains

| Artefact | What it is |
|---|---|
| The source at the tag | The ref `estate-demo` builds the demo from, and the ref to build the CLI from with `go build -o telecraft ./cmd/telecraft`. GitHub attaches the source archives itself. |
| `telecraft-design-<version>.tar.gz` | `tokens.css`, `base.css` once it exists, `fonts/fonts.css`, the `.woff2` faces, and the two OFL licence texts, laid out the way a site serves them. |
| `SHA256SUMS` | The checksum of the archive above. |

Three things are deliberately absent:

- **Prebuilt binaries.** The documented way to get the CLI is to build it
  from source, which the [quickstart](../guides/quickstart.md) does in one
  command. Publishing binaries buys a platform matrix and an upgrade path
  that nothing has asked for yet.
- **The console bundle.** The console is built per deployment: demo mode
  reads a snapshot beside it, instance mode calls a server (ADR-0045). A
  single prebuilt bundle would be the wrong one for at least one consumer.
- **A Catalogue.** ADR-0020 §5 says a release embeds the Catalogue for its
  pinned collector version. There is no installable instance yet for it to
  be embedded in, so that clause has nothing to attach to. It is the first
  thing this release scheme grows when there is.

## How to cut a release

1. **Check that `main` is green.** The release workflow refuses a tag that
   does not sit on `main`, but it does not re-run the test suite: the pull
   request that merged is the gate.
2. **Choose the number** using the table above. To see what exists, run
   `git tag --sort=-v:refname --list 'v*'`.
3. **Tag the commit**, annotated, from an up to date `main`:

   ```bash
   git checkout main && git pull
   git tag -a v0.3.0 -m "v0.3.0"
   ```

4. **Push the tag**, which is the act that publishes:

   ```bash
   git push origin v0.3.0
   ```

5. **Watch the two workflows the push starts.** `release.yml` validates the
   tag, packs the design artefacts, checks the palette floors over the
   `tokens.css` it is about to ship, and creates the release.
   `demo-dispatch.yml` moves the `release` pointer and asks `estate-demo` to
   rebuild.
6. **Verify.** The release page lists the archive and `SHA256SUMS`,
   `git ls-remote --tags origin release` resolves to the commit you tagged,
   and the demo run in `estate-demo` finishes green.

## The release pointer, and how the demo follows it

`estate-demo` checks out a ref named `release`. That ref is a tag this
repository moves to each new stable version, so the demo follows releases
without anyone editing a second repository.

The pointer exists because the demo builds on more than one event. A push to
the estate itself, a manual run, and the dispatch from this repository all
build the same site, and only the manual run can carry a ref. If the pinned
version travelled in the dispatch, an estate content change would build
against something else, and which version of the platform the public site
runs would depend on which event fired last. One name that always means "the
current release" removes that question.

Between releases the demo lags `main`, on purpose. A change that has merged
but not been released is not on the demo, and a fix for something visible on
the demo reaches it when you cut a release. There is no staging demo: a
second site is a second deployment and a second public claim to keep honest,
and what it would catch, CI catches first. The demo job in `ci.yml` builds
the snapshot and the demo bundle on every pull request that touches them.

To put the demo back on an earlier release, move the pointer by hand and run
the Demo workflow:

```bash
git tag --force release v0.2.4
git push --force origin refs/tags/release
gh workflow run demo-dispatch.yml
```

The version tags are untouched by any of this. `release` is the only ref in
this repository that moves.

## Consuming the design artefacts

Another repository pins a version and fetches the archive:

```bash
curl -fsSLO https://github.com/telecraft-dev/telecraft/releases/download/v0.3.0/telecraft-design-v0.3.0.tar.gz
curl -fsSLO https://github.com/telecraft-dev/telecraft/releases/download/v0.3.0/SHA256SUMS
sha256sum --check SHA256SUMS
tar -xzf telecraft-design-v0.3.0.tar.gz
```

The archive extracts to `telecraft-design-v0.3.0/`, holding `tokens.css`,
`fonts/`, and a `README.md` describing each file. `fonts/fonts.css` reaches
its faces by relative URL, so keep the directory whole and serve all of it
from your own host. Nothing in the archive reaches another origin, which is
the rule the console is held to (ADR-0019) and which a site inherits by using
these sheets.

The sheets change rarely, so a pin can sit across several releases. Compare
the checksum of the archive you hold against the one in the newer release to
see whether they moved at all: the archive is packed with sorted names and a
fixed timestamp, so identical contents give an identical checksum. Re-pin
deliberately, and read the release notes when you do, because a token that is
renamed is a breaking change for whoever reads it by name.
