---
title: Releases
description: What a Telecraft version number means, what a release contains, how to cut one, and how the public demo follows it.
order: 10
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
[ADR-0049](https://github.com/telecraft-dev/telecraft/blob/main/docs/adr/0049-releases-tags-on-main-and-the-demo-pin.md),
and the sizing rule is
[ADR-0066](https://github.com/telecraft-dev/telecraft/blob/main/docs/adr/0066-a-version-number-is-sized-by-what-it-asks.md)'s.
Read them before you change the scheme; this page tells you how to work it.

## What a version number means

Tags are `vMAJOR.MINOR.PATCH`, and Telecraft is pre-1.0. That is not
modesty about quality. It is a statement about the platform API documents,
the authored file formats, and the CLI flags, none of which are ones the
project is willing to freeze yet.

While the major version is zero, size the number by what the release asks
of a consumer, never by what it shows them (ADR-0066). Ask two questions,
in order; the first yes decides:

1. **Must a consumer change anything to take it?** Migrating an authored
   file, changing a CLI invocation, re-pinning a renamed token,
   re-rendering a committed estate, adjusting to a changed platform API
   document. Yes: **minor** (`v0.3.0`), and the release notes lead with
   what must change and how.
2. **Can a consumer do something they could not do before?** A new CLI
   command or flag, a new platform API document, a new field in an
   authored format, a new Workspace or view, a new release artefact.
   Yes: **minor**.

Neither: **patch** (`v0.3.1`), however visible. Fixes, redesigns, copy
and polish all live here; a patch is safe to take on sight, and that
promise is the point of the split.

A consumer is anyone who takes this repository at a ref: `estate-demo`,
the sites consuming the design artefact, an operator building the CLI and
authoring an estate. A reader of the demo is an audience, not a consumer.

Every release's notes open with `Breaking:` and `New:`, each possibly
`none`, so the number can be audited against its notes.

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
| The source at the tag | The ref `estate-demo` builds the demo from, and the ref a contributor builds the CLI from. GitHub attaches the source archives itself. |
| `ghcr.io/telecraft-dev/telecraft:<version>` | The container image (ADR-0068): the `telecraft` binary with the console inside it, the Catalogue baseline and `LICENSE`, as one index over `linux/amd64` and `linux/arm64`. A stable release also moves the `release` tag. |
| `telecraft-<version>-<os>-<arch>` | The CLI, for every workstation and pipeline the product expects to meet (ADR-0081): `linux-amd64`, `linux-arm64`, `darwin-arm64`, `darwin-amd64` and `windows-amd64.exe`. The two Linux ones are the binaries the image holds; the other three are cross-compiled in the same job. This is what the [quickstart](../guides/quickstart.md) downloads. |
| `telecraft-design-<version>.tar.gz` | The design system: `tokens.css`, `base.css` once it exists, `fonts/` holding `fonts.css`, the `.woff2` faces and the two OFL licence texts, and `icons/` holding the brand mark in the five formats a browser is offered. `LICENSE`, `VERSION` and a `README.md` describing each file travel with them. |
| `SHA256SUMS` | The checksums of every file attached above. |
| The chart, at `oci://ghcr.io/telecraft-dev/charts/telecraft` | The Helm chart in `charts/telecraft`, packaged and pushed on the same tag push. Its `version` is the release version without the leading `v`, because Helm requires SemVer without one, and its `appVersion` is the tag verbatim, so the chart and the image it deploys carry one number. |

The image is addressed by digest rather than by a checksum file, which is the
same discipline in the form a registry takes: the release notes carry the
digest, and a deployment pins it.

What is still absent, and why:

- **A package manager, and an installer script.** No Homebrew tap, no Scoop
  manifest, no `curl | sh` (ADR-0081 §3). Each is a second published surface
  with its own staleness, and the first two are a claim to keep current in a
  repository this project does not own. A download and a checksum are two
  commands and no third party.
- **Any architecture but amd64 and arm64.** Nothing has asked, and each one
  added is a build nobody runs.
- **An SBOM.** The binary carries its own module inventory, which `go version
  -m` prints from the file the release attaches, and the base image
  contributes no package manager to enumerate. Raised as OQ-26 rather than
  dropped.

Two earlier absences have closed. The console bundle is inside the binary
(ADR-0067 §3), so there is no separate artefact left to refuse; and the
Catalogue baseline of ADR-0020 §5 now has an installable instance to be
embedded in, so the image carries it.

## How to cut a release

1. **Check that `main` is green.** The release workflow refuses a tag that
   does not sit on `main`, but it does not re-run the test suite: the pull
   request that merged is the gate.
2. **Choose the number** with the two questions above. To see what
   exists, run
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
   tag, builds the console and both binaries, assembles and pushes the image
   index, starts the image it has just built with networking disabled and
   requires it to serve, packs the design artefacts, checks the palette
   floors over the `tokens.css` it is about to ship, and creates the release.
   `demo-dispatch.yml` moves the `release` pointer and asks `estate-demo` to
   rebuild.
6. **Verify.** The release page lists the binaries, the archive and
   `SHA256SUMS`; `docker pull ghcr.io/telecraft-dev/telecraft:<version>`
   resolves to the digest the notes name;
   `git ls-remote --tags origin release` resolves to the commit you tagged;
   and the demo run in `estate-demo` finishes green.
7. **Verify the chart resolves and renders**, from a directory with no
   checkout of this repository in it:

   ```bash
   helm pull oci://ghcr.io/telecraft-dev/charts/telecraft --version 0.3.0
   helm show chart oci://ghcr.io/telecraft-dev/charts/telecraft --version 0.3.0
   ```

   The `appVersion` it prints is the tag you pushed. An adopter's first
   install is the first thing that reads either number, and a chart pushed
   with the wrong one installs an image from another release.
8. **Verify that a known-good licence still verifies**, against the binary
   the release built:

   ```bash
   ./telecraft licence -licence-file KNOWN_GOOD_LICENCE
   ```

   It must print the Enterprise Edition line. A build shipping the wrong
   public keys denies every Enterprise Instance its Entitlements while
   looking entirely healthy, and this is the step that catches it. The
   licence to test with is `licences/tc-2026-0000.licence` in the private
   `telecraft-dev/licensing` repository, which is where the keys live too; a
   signing key never enters this repository, its CI, an image, or a release
   artefact.

   The emptiest version of that failure is not left to this step.
   `TestThisBuildShipsAKey` in `pkg/licence` fails when the compiled-in list
   holds no key, and `go test ./...` runs on every pull request, so a build
   that would accept no licence at all cannot reach a tag unnoticed. What the
   test cannot say is whether the keys are the right ones, which is what the
   command above is for.
9. **Trust the checks over an open tab.** The demo serves behind a cache
   that can hold the previous build for some minutes after the deployment
   finishes, so a page that still looks old is not evidence the release
   failed. Hard-refresh, and compare the version the console names in its
   profile section against the tag you pushed.

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

The image carries a tag of the same name, for the same reason, and it moves
at the same moment: `ghcr.io/telecraft-dev/telecraft:release` is whatever the
current stable version is. There is no `latest`, and no rolling `v0` or
`v0.7`: a floating minor tag promises a compatibility surface, and being
pre-1.0 is the statement that there is not one yet. A pre-release publishes
its version tag and moves neither pointer.

## Consuming the design artefacts

Another repository pins a version and fetches the archive:

```bash
curl -fsSLO https://github.com/telecraft-dev/telecraft/releases/download/v0.3.0/telecraft-design-v0.3.0.tar.gz
curl -fsSLO https://github.com/telecraft-dev/telecraft/releases/download/v0.3.0/SHA256SUMS
sha256sum --check SHA256SUMS
tar -xzf telecraft-design-v0.3.0.tar.gz
```

The archive extracts to `telecraft-design-v0.3.0/`, holding `tokens.css`,
`fonts/`, `icons/`, and a `README.md` describing each file. `fonts/fonts.css`
reaches its faces by relative URL, so keep the directory whole and serve all
of it from your own host. `icons/` is the mark as a browser sees it, and
nothing in it points at anything else, so move those five files to wherever
your markup names them; `favicon.ico` belongs at the site root, because a
browser probes for it there whether or not a page links it. Nothing in the
archive reaches another origin, which is the rule the console is held to
(ADR-0019) and which a site inherits by using these sheets.

The sheets change rarely, so a pin can sit across several releases. Compare
the checksum of the archive you hold against the one in the newer release to
see whether they moved at all: the archive is packed with sorted names and a
fixed timestamp, so identical contents give an identical checksum. Re-pin
deliberately, and read the release notes when you do, because a token that is
renamed is a breaking change for whoever reads it by name.
