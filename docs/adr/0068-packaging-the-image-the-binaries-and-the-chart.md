# ADR-0068: A release ships one image, two Linux binaries and one chart, and all of it mirrors

- Status: accepted (amends ADR-0049 §3's artefact list)
- Date: 2026-08-28

## Context

Nothing ships.

`release.yml` publishes the source at the tag and one design archive, and
its header states the three absences ADR-0049 §3 argued for: no prebuilt
binaries, because the documented way to get the CLI is `go build
./cmd/telecraft`; no console bundle, because the console was built per
deployment mode and one prebuilt bundle would be wrong for a consumer; and
no Catalogue, because ADR-0020 §5's embedded baseline needs an installable
instance to be embedded in and there was not one.

There is no Dockerfile for the product. `devenv/collector/Dockerfile` is a
collector for the development environment (ADR-0052), not the thing this
repository builds. There is no chart, no manifest set, and no install
guide: `docs/guides/quickstart.md` is the whole story, and it compiles Go.

Every one of those absences rested on a premise ADR-0067 has now removed.
There is one long-running process, one command, and the console travels
inside the binary, so there is exactly one thing to package and its bundle
is no longer a consumer's choice. An adopter who wants to run a control
plane is currently asked to install a Go toolchain on the host that runs
it, and a hosted deployment would have to invent its own packaging in a
second repository and keep it honest by hand.

Five decisions bound the answer. ADR-0002: the platform ships
configurations, never binaries, and nothing it publishes sits in the
telemetry path. ADR-0011: no Helm or Kustomize rendering. ADR-0012:
Kubernetes is the control plane's substrate rather than its managed
population. REQ-006 and ADR-0019: air-gapped deployment is first class and
nothing depends on a SaaS, which ADR-0045 §5 already enforces for the
bundle in CI. ADR-0049 and ADR-0066: a release is an immutable tag on
`main`, `release` is the one ref that moves, and a version number is sized
by what it asks of a consumer.

## Decision

### 1. What ADR-0002 and ADR-0011 refuse, stated before somebody quotes them

ADR-0002's list, "no collector distribution, no gateway, no container
image, no Helm chart, no rendered DaemonSet", is a rule about the telemetry
path. What it refuses is Telecraft owning how a collector reaches a host,
so that nothing published here can stop telemetry flowing when it is down.
Packaging the control plane sits on the other side of that line: no
collector runs anything this decision publishes, and if the image below is
not running, every collector goes on running the configuration it last
fetched. The non-goal is untouched, and ADR-0002's consequence stands as
written: an adopter who wants the serving path still supplies their own
Supervisor-plus-collector image, because upstream ships none and this
project does not either.

ADR-0011 refuses to *render* Helm and Kustomize: reading an adopter's
collector configuration out of a chart, priced at a repo-server dependency
or a values engine of our own. It says nothing about how Telecraft itself
is installed. The chart in §5 adds no renderer, reads no adopter's chart,
and templates nothing that describes an estate.

Both rules are about the read and write paths. This decision is about the
box the product arrives in.

### 2. The container image is the release artefact

The image holds three files: the `telecraft` binary, the Catalogue
baseline below, and the licence. The console bundle is inside the binary
(ADR-0067 §3), so there is no asset tree beside it and no second version
to reconcile.
`LICENSE` travels at a documented path because Elastic License 2.0 asks
that whoever is given part of the software is given the terms with it
(ADR-0050), and an image is a copy handed to somebody.

**Base: `gcr.io/distroless/static-debian12:nonroot`, pinned by digest.**
It supplies the three things a static Go binary needs and cannot supply
itself: a certificate bundle for the git fetch and the provider round
trips, timezone data, and a non-root user. A scratch image with those
three copied in was considered and refused, because it makes this project
the maintainer of a certificate bundle, and a bundle we maintain goes
stale in a way no reviewer here would catch. The digest pin is what keeps
the image a function of the tag: refreshing the base is a commit, which is
a review, and it ships as a patch because neither of ADR-0066's questions
answers yes.

**Entrypoint is the binary, and the default command is `serve`.** The
image is the whole CLI, so a pipeline with no Go toolchain runs
`telecraft check` from the same artefact its control plane runs, which is
the shape an air-gapped adopter's CI is in. There is no entrypoint script:
every flag has an environment variable under it (ADR-0067 §4), so
configuration reaches the process without a shell rewriting arguments, and
what a container sets is what the command's own usage documents.

**The image binds every interface on both addresses**, keeping each flag's
default port, and sets nothing else. `serve` defaults to loopback because
loopback is the right default on a host; inside a network namespace it
means unreachable, and an image whose first run answers nothing is a bad
image. No external URL, no session key and no estate is baked in: those
are the operator's, and two of them are secret.

**The image is assembled, not compiled.** The Dockerfile copies a binary
from the build context and carries no build stage, so the binary attached
to the release and the binary inside the image are the same bytes rather
than two builds of one commit. The cost is that `docker build .` on a
checkout with nothing built fails on a missing file. That is the intended
failure: the alternative is an image that quietly holds a different binary
from the one whose checksum was published.

**Two architectures, `linux/amd64` and `linux/arm64`, published as one
index.** Go cross-compiles both from a single console build, so the second
architecture costs a `GOARCH` and no emulation. Nothing else is built:
Windows and macOS containers are not a deployment shape for a control
plane, and a 32-bit build is a platform matrix entry with no adopter
behind it.

**Registry: `ghcr.io/telecraft-dev/telecraft`.** The forge is a seam for
the estates Telecraft reads (ADR-0028); where this project publishes its
own artefacts is not that seam, and a second registry account is a second
credential and a second thing to own.

**The image carries the Catalogue for its pinned collector version**, at a
documented path. This is the baseline ADR-0020 §5 asks every release to
embed, and it is the clause ADR-0049 §3 recorded as waiting for something
installable to embed it in. It arrives through the same import and
activation pipeline as a Catalogue uploaded across an air gap, so there is
still one code path and the operator still activates deliberately
(ADR-0020 §5, §6). An air-gapped instance can therefore judge on the day
it starts, rather than after a transfer.

### 3. Image tags follow the release model, and a deployment pins the digest

Two tags, and no third.

`vMAJOR.MINOR.PATCH`, matching the git tag exactly and never moved. A
wrong image is corrected by the next number, which is ADR-0049 §1's rule
applied to the registry, and it is the rule that makes a version tag worth
reading at all.

`release`, which moves to each new stable version beside the git ref of
the same name. It is the only tag here that moves, for the same reason
that ref is (ADR-0049 §5): one name that always means "the current
release" removes the question of which version a thing without a pin got.

No `latest`. It means whichever build ran last, a rehearsal included, and
`release` already carries the word for the current stable version. Two
names for one idea is two names that can disagree.

No rolling `v0` or `v0.7`. A floating minor tag promises a compatibility
surface, and being pre-1.0 is the statement that there is not one yet
(ADR-0049 §1, ADR-0066 §4).

A pre-release publishes its version tag and does not move `release`, which
is ADR-0049 §6 in the registry: the whole publishing path is rehearsable
without anything the world follows moving.

A deployment pins the digest, and the release notes carry it. A tag is a
name a registry resolves; a digest is the bytes. The chart's default is
the version tag, because a default that a human cannot read is a default
nobody checks, and the value that replaces it with a digest is one line.

### 4. Two Linux binaries travel with the image; the matrix is exactly what the image forced

ADR-0049 §3's refusal of prebuilt binaries is amended here, in one
direction only. The release attaches `telecraft-<version>-linux-amd64` and
`telecraft-<version>-linux-arm64`, and `SHA256SUMS` grows to cover them
beside the design archive.

The refusal held because a binary bought a platform matrix nothing had
asked for. The image pays for that matrix already: those two binaries are
built whether or not anybody downloads them, and attaching them costs an
upload. What it buys is the adopter who runs `telecraft check` in a
pipeline that has neither a container runtime nor a Go toolchain.

Nothing else is attached. No macOS, no Windows, no other architecture:
none of those is built for anything else, and building them is the matrix
ADR-0049 declined. Every other platform builds from source in the one command the
quickstart documents, and that stays the documented way to get the CLI on
a workstation.

ADR-0049 §3's second absence lapses rather than being amended. It refused
to publish the console bundle because the bundle was built per deployment
mode; the bundle is now inside the binary, so there is no separate artefact
left to refuse.

### 5. Kubernetes is one chart, in this repository, published to the same registry

A chart, and one chart: a Deployment, a Service, an optional Ingress, a
ServiceAccount, and references to secrets the operator creates. It deploys
the Instance server and nothing else. No collector, no Supervisor, no
DaemonSet, no rendered configuration, no custom resource. ADR-0012 puts
the control plane on Kubernetes and keeps the managed population off it,
and this chart is the control plane's half of that sentence.

**`replicas` is 1, and the chart refuses more.** The Served reading is the
reading of the connections one process holds, so two servers behind one
address each report half an estate as though it were whole (ADR-0067 §2).
The update strategy is Recreate for the same reason: a rolling update puts
two of them on one address for the length of the rollout. The price is
that an upgrade is an outage of the control plane, which is not an outage
of telemetry: collectors go on running the configuration they already
hold, which is the non-goal ADR-0002 exists to protect.

**Probes are `/healthz` and `/readyz`** (ADR-0067 §6). The cache directory
is an `emptyDir`, because everything in it is rebuildable (ADR-0032 §1)
and a volume that outlived the pod would be the first durable thing this
product keeps.

**TLS is not the process's.** The Ingress or whatever sits in front
terminates it, and `-external-url` declares what the outside sees
(ADR-0067 §5). The chart sets that value from the same host it puts on the
Ingress, so the two cannot drift into a redirect that returns nowhere.

**A chart rather than a manifest set, for air gap rather than for taste.**
An install inside an air gap re-points every image reference at a mirror.
With a chart that is one value; with a manifest set it is an edit, and an
operator who edits a manifest owns a fork of it for as long as they run
it. Charts are OCI artefacts, so the chart and the image travel through
one mirroring mechanism and one credential, which is the difference
between one air-gap procedure and two. The chart has no dependencies, so
nothing resolves a second chart repository at install time. Whoever
applies YAML through GitOps runs `helm template` and commits the output,
so there is no second manifest set to keep honest.

**In this repository, not a sibling.** The chart's entire surface is the
flag set of one binary (ADR-0067 §4). A chart in another repository is a
copy of a contract it cannot watch change, and the failure is silent: the
flag lands here, the chart goes on setting the old one, and the first
person to notice is an adopter. Here, the pull request that adds a flag
adds its value, and CI installs the chart against the image built from the
same commit. The cost is that this repository grows a Kubernetes tree, and
that is smaller than a chart that lags.

**The chart versions with the platform.** Its `version` is the release
version without the leading `v`, because Helm requires SemVer without one,
and its `appVersion` is the tag verbatim. It is pushed to
`oci://ghcr.io/telecraft-dev/charts` on the same tag push that publishes
the image, so one number answers "what do I deploy".

A single host is the image and the documented flags. No Compose file: one
service needs no file to describe how it composes with the others.

### 6. Air-gap parity, checked rather than asserted

Four artefacts, two mechanisms. The image and the chart are OCI, copied
with whatever an operator already replicates registries with. The
binaries, the design archive and `SHA256SUMS` are files attached to the
release. Nothing else exists to mirror.

At run time the image reaches nothing beyond what the operator points it
at. The console is inside the binary and fetches no asset from another
origin (ADR-0045 §5, ADR-0067 §3). The Catalogue baseline is in the image.
The estate is a git remote the operator names, and ADR-0032 §3 already
allows that to be a local bare repository. No init container fetches
anything, no chart hook downloads anything, and there is no dependency to
resolve.

That rule is checked the way `check:zero-cdn` checks the bundle's half of
it. The release job starts the image it has just built with networking
disabled, over a bare estate repository inside the container, and requires
`/readyz` to go green and the console to load. An image that needed one
fetch fails there, before anything is published, rather than in somebody's
air gap.

What an air gap still needs supplied from inside: the secrets, the estate,
and any Catalogue newer than the image's, which crosses as an operator
upload on ADR-0020 §5's third transport.

### 7. Supply chain: checksums and provenance in v1, no key of our own and no SBOM

- **Checksums.** `SHA256SUMS` covers every file attached to the release.
  The image and the chart are addressed by digest, and the release notes
  carry the image digest, so the same discipline reaches the artefacts a
  checksum file cannot hold.
- **Provenance.** A build attestation for the image, the chart and the
  binaries, produced by the forge's own attestation service from the
  workflow run that built them. It ties an artefact to a commit, a
  workflow and a run, it is one step of workflow, and it needs no key this
  project has to hold.
- **No signing key of our own.** A key is custody, rotation, a revocation
  story and a ceremony that somebody has to be able to perform. A project
  key with none of those is a claim that reads stronger than it is, and a
  signature nobody can revoke is worse than no signature. Revisit when
  there is somebody to run the ceremony.
- **No SBOM in v1.** The binary already carries its module inventory, and
  `go version -m` prints it from the same file the release attaches. The
  base contributes no package manager and nothing to enumerate. The
  console's tree is in the source at the tag, in a lockfile. An SBOM would
  be a second copy of facts already shipped, in one of two competing
  formats neither of which anybody has asked for, and the first time it is
  generated from a build other than the published one it is a falsehood
  with a filename. Raised as OQ-26 rather than dropped.
- **Offline verification is the checksum and the digest.** Verifying an
  attestation reaches a transparency log, so it happens where the mirror
  is filled rather than inside the air gap. Raised as OQ-27.

## Consequences

- A release stops being a packing step and becomes a build. `release.yml`
  compiles the console and two binaries, assembles and pushes a
  multi-architecture index, packages and pushes the chart, runs the
  offline check, and attaches the files. Cutting a release is still one
  annotated tag pushed at `main`, and the guards on it are unchanged.
- `docs/contributing/releases.md` rewrites "What a release contains". All
  three of its deliberate absences close: the binaries are attached, the
  bundle is inside one of them, and the Catalogue is in the image. The
  page gains how to install, and how to mirror.
- The first release under this scheme is a minor. A consumer can do three
  things they could not before, which is ADR-0066's second question
  answering yes.
- `docs/guides/quickstart.md`'s `go build` stops being the whole install
  story. Building from source stays the way to get the CLI on a
  workstation; running Telecraft becomes pulling an image or installing a
  chart, and the quickstart has to say which question each answers.
- Publishing an image makes this project a distributor of something people
  run in production. A vulnerability in the base image or the Go toolchain
  is now a patch release to cut here, rather than an adopter's rebuild,
  and `SECURITY.md` has an artefact to speak about for the first time.
  `.github/dependabot.yml` gains a `docker` entry so the base image digest
  is watched the way the modules and the actions are, and the refresh
  arrives as a reviewable pull request like every other bump.
- One Instance server per Instance is now enforced in a second place. If
  OQ-19 is ever answered, the chart's replica ceiling changes with the
  code, not after it.
- The image being the whole CLI means `telecraft check` in a pipeline and
  `telecraft serve` in a cluster are the same bytes. A conformance verdict
  in CI and a verdict on the console cannot disagree because one of them
  is older.
- No new capitalised domain terms. An image, a chart, a tag, a digest and
  a registry are the industry's words for the industry's things, and the
  glossary is unchanged.

## Sources

- ADR-0002 (configurations never binaries, nothing in the telemetry path),
  ADR-0010 (the Supervisor, and never serving empty), ADR-0011 (no Helm or
  Kustomize rendering), ADR-0012 (Kubernetes as the control plane's
  substrate), ADR-0019 (air-gap first class), ADR-0020 §5 and §6 (the
  embedded Catalogue baseline, one import pipeline, explicit activation),
  ADR-0028 (the forge as a seam), ADR-0032 §1 and §3 (the closed list, and
  a single binary plus a directory), ADR-0045 §5 (zero CDN, checked in
  CI), ADR-0049 §1, §3, §5 and §6 (the tag, the artefact list, the moving
  pointer, the rehearsal), ADR-0050 (Elastic License 2.0 and its Notices
  clause), ADR-0052 (the development environment and its collector image),
  ADR-0066 (sizing a version number), ADR-0067 (the Instance server, the
  embedded console, the flag surface, TLS in front, the probes).
- `.github/workflows/release.yml` as it stood on 2026-08-28, and its
  header stating the three absences.
- `docs/guides/quickstart.md`, the whole install story; `cmd/telecraft/serve.go`,
  the loopback default this image overrides; `devenv/collector/Dockerfile`,
  the collector image that is not the product's.
- REQ-003, REQ-006, REQ-060.
- Issue #194.
