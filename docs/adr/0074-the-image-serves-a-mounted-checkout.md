# ADR-0074: The image serves a mounted checkout; the fetch stays outside the container

- Status: accepted (amends ADR-0068 §2, §5 and §6)
- Date: 2026-08-28

## Context

ADR-0068 §2 pinned the image's base at
`gcr.io/distroless/static-debian12:nonroot`, by digest, and argued for it:
it supplies the three things a static Go binary cannot supply itself, and a
scratch image with those three copied in makes this project the maintainer
of a certificate bundle. Building the image against that base (issue #200)
found that the base and one of `serve`'s flags cannot both be right.

The base holds `ca-certificates`, `tzdata` and `passwd`, and its `usr/bin`
is empty. No git, no shell, no libc, which is the whole point of it.

`serve` reaches an estate two ways (ADR-0067 §4). `-estate` reads a
checkout already on disk. `-repo` clones a remote once and fetches it on
every poll, and it does that by running `git`, because ADR-0032 §3 made the
git dependency git-the-tool and never git-the-service, so that a local bare
repository satisfies ADR-0028's transport floor. There is no git in the
image to run. `serve -repo` therefore fails at the first snapshot with

```
initial repo snapshot: git clone --quiet …: exec: "git": executable file not found in $PATH
```

and the server refuses to start, because serving cannot begin on an estate
it could not read. `serve -estate` works: `git rev-parse` cannot run either,
so the head reads `(untracked directory)` in the server's own log, and
nothing else notices, because the API documents are computed at the commit
the rendered tree carries (ADR-0013) rather than at the head of a working
copy.

Two sentences of ADR-0068 were written on the other assumption. §2 justifies
the base's certificate bundle as one "for the git fetch and the provider
round trips". §6 has the release job start the image "over a bare estate
repository inside the container", which is ADR-0032 §3's air-gap floor
written as though the image were the host process.

The question is which of the two is wrong: the base, or the sentences.

## Decision

### 1. The base stays exactly as ADR-0068 §2 pinned it

`gcr.io/distroless/static-debian12:nonroot`, by digest, unchanged. The
digest pin, the Dependabot entry that watches it, and the patch-sized
refresh all stand.

Its three needs stand too, and only one clause of the reasoning is struck.
The certificate bundle is not there for a git fetch: it is there for the
round trips the process makes anyway, to a forge over the seam ADR-0028
defines, to an identity provider (ADR-0019), and to the backends the
`EstateProvider` and `TelemetryProvider` implementations read. Timezone data
and a non-root user are unaffected. The base is still the cheapest way to
have those without maintaining them.

**A base that carries git is refused.** No distroless variant ships one, so
this means a Debian or Alpine base, which means a package manager and a
shell in the image and an `apt-get` or `apk` at build time. Three costs, any
one of which is enough. The build gains a network dependency, and ADR-0068
§2 made the image assembled rather than compiled precisely so that what it
holds is the bytes the release published, not the result of a resolution
that ran once. The image gains the package surface the base was chosen to
avoid, which this project would then be the one cutting patch releases for.
And a long-running process that terminates nothing and renders no adopter's
configuration would be sharing its namespace with a shell and a package
manager, which is a worse posture for the sake of a fetch that something
outside the container can do.

### 2. A git binary copied into the image is refused as well

The narrower version of the same idea is copying a statically linked `git`
into the image, which is the move ADR-0068 §2 already refused for a
certificate bundle. It loses to that same argument, and to a fact about
git.

The argument: a binary we place in the image is a binary this project
maintains, watches for vulnerabilities, and rebuilds. It arrives under no
package manager, so nothing that watches the base digest sees it, and the
review that would catch it going stale is a review nobody here is equipped
to do.

The fact: git is not one program. `git clone` over HTTPS reaches for a
remote helper, and the helper wants a libc and a TLS stack; `git` runs
subprocesses for its own work and expects a shell for a good deal of it. A
static build that answered `clone` and `fetch` today would answer some
neighbouring command with an exec failure tomorrow, and that failure would
arrive in an adopter's cluster rather than here. Putting a program that
executes other programs into an image whose selling point is that it has no
programs to execute is the wrong trade.

A third answer exists and is not decided here: a git implementation inside
the binary, which needs no tool in any image and would hand `-repo` back to
every shape at once. It is a change to how the product fetches rather than
to how it is packaged, and it is raised as OQ-31 rather than dropped.

### 3. `-repo` is unsupported inside the image, and says so by failing closed

The flag is not removed, not stubbed, and nothing detects that the process
is in a container. On a host and in the development environment `-repo` is
what it has always been, and ADR-0032 §3 is unamended: a local bare
repository is still the air-gap floor for the process, and a single binary
plus a directory is still a complete standalone instance.

Inside the image `-repo` fails at the first snapshot with the message
above, and the server exits rather than serving an estate it never read.
That is the behaviour this decision wants, and it costs nothing to keep: an
operator who names the flag learns immediately, from a message that names
the missing tool, and no code has to be written that could tell a host
process it is a container. A check that refused the flag by inspecting the
image would be a second way to be wrong about the same thing.

What carries the answer instead is the documentation. The image's guide
states which shape the image offers, beside the shape a host offers, which
is where an operator meets the question.

### 4. The supported shape is a mounted checkout, kept current from outside

`serve -estate`, over a directory mounted into the container. Each poll
re-reads the directory, so a merged pull request arrives without a restart,
and the fetch interval is the bounded staleness ADR-0032 §1 already fixed.
The mount is read only: nothing in the estate is the server's to write
(ADR-0032 §1's closed list), and a read-only mount says so to the substrate
as well as to the reader.

Advancing that directory belongs to the deployment. It is git-the-tool
still, running somewhere that has git: a job on the host that pulls, a CI
runner that publishes a tree, or a container beside this one. This is not a
new rung on ADR-0032 §3's ladder. It is the rung that ladder already has,
with the fetch on the other side of a container boundary.

Two things follow that are worth stating rather than discovering. The
credential that reads the estate repository now belongs to whatever fetches,
not to the Instance server, so in this shape the server holds one secret
fewer and the Secret directory (ADR-0071) does not grow. And the server's
log names the head as `(untracked directory)`, because it cannot run
`git rev-parse` even over a checkout that has a history. Every artefact
still carries its own commit stamp (ADR-0013), every document is still
computed at the commit the rendered tree carries, and a collector receives
the same bytes whichever side of the boundary filled the directory.

### 5. On Kubernetes a sidecar fetches, and the chart has a place for it

ADR-0068 §5's Deployment gains an estate volume and a place for a second
container: an `emptyDir` that the Instance server mounts read only at the
path it is given as `-estate`, and an optional sidecar that syncs the estate
repository into it. One pod, one lifecycle, one address, and `replicas` is
still 1 with the same Recreate strategy for the same reason (ADR-0067 §2).

ADR-0068 §5's "it deploys the Instance server and nothing else" holds where
it was aimed. The refusal is of a managed population in the control plane's
chart: no collector, no Supervisor, no DaemonSet, no rendered
configuration, no custom resource (ADR-0012). A container that fetches the
estate is on the control plane's side of that line, and it is the same fetch
a host does with a cron job.

**The chart names no default image for it.** The volume, the mount and the
values are the chart's; which syncing container an operator runs is theirs.
Blessing one would make this project the thing that watches a third party's
digest for its adopters, and inside an air gap it would be an image the
operator has to mirror because our chart's default named it rather than
because they chose it. An operator who configures no sidecar supplies the
volume themselves, from whatever already has the checkout, and the mount is
unchanged.

### 6. The release job's offline check runs over an estate directory

ADR-0068 §6's release job still starts the image it has just built with
networking disabled and still requires `/readyz` to go green and the console
to load. What it serves is an estate checkout copied into a directory the
container mounts, with one user written into it so an Instance with nobody
able to sign in does not refuse to start, and the check also requires the
OpAMP endpoint to answer.

The bare repository in the original sentence was carrying an idea that does
not belong to the image. ADR-0068 §6 exists to prove that the image reaches
nothing
to become ready, and a directory proves that exactly as well as a repository
would; the repository was there because ADR-0032 §3's floor is written for
the process on a host. The check now runs the shape the image supports,
which is also the shape it would fail in.

Nothing else in ADR-0068 §6 changes. Four artefacts and two mechanisms, and
the list of what an air gap supplies from inside is the same list: the
secrets, the estate, and any Catalogue newer than the image's. The estate
was always on it.

## Consequences

- ADR-0068 §2 keeps its base, its digest pin and its three needs. One
  clause of its justification is struck: the certificate bundle is for the
  round trips, not for a git fetch that does not happen in this image.
- ADR-0068 §6's offline check is over an estate directory, and
  `tools/image/offline.sh` is that sentence. An image that needed one fetch
  to become ready still fails before anything is published.
- The image and the host process differ in one place, and only one: which
  flag reaches an estate. Everything else about `serve` is the same binary
  behaving the same way, which is what makes the guide's answer a short one.
- An air-gapped Kubernetes deployment mirrors one image more than ADR-0068
  §6 counted, and it is not this project's. It travels through the same
  registry mirroring the image and the chart already use, so it is one more
  reference in a procedure that exists, rather than a second procedure.
- The chart grows a volume, a mount and a sidecar block whose image is
  empty by default. A deployment that fills none of them serves nothing, so
  the chart's documentation has to make the estate a first-class step of
  installing rather than a detail after it.
- Whoever runs the sync holds the estate credential. That is a security
  improvement in the image shape and a change of ownership in the
  deployment, and an operator who was expecting to hand the server a repo
  URL and a key hands them to something else instead.
- `serve -repo` on a host, in the development environment (ADR-0052) and
  over a local bare repository is untouched, and so is ADR-0032 §3. This
  decision is about one deployment shape, not about the product's sources.
- No new capitalised domain terms. An image, a base, a volume, a mount and
  a sidecar are the industry's words for the industry's things, and the
  glossary is unchanged.

## Sources

- ADR-0068 §2 (the base and what it supplies), §5 (the chart and what it
  deploys) and §6 (the offline check): §2 and §6 amended in the two
  sentences named above, §5 in the shape of its Deployment.
- ADR-0032 §1 and §3 (the closed list, bounded staleness, git-the-tool and
  the standalone rung), ADR-0028 (the transport floor and the forge seam),
  ADR-0067 §2 and §4 (one process per Instance, and the flag surface),
  ADR-0013 (the artefact carries its own commit stamp), ADR-0012
  (Kubernetes as the control plane's substrate), ADR-0019 (air gap first
  class), ADR-0071 (secrets are files a deployment places), ADR-0052 (the
  development environment).
- `internal/serving/source.go`, the two sources and the `git` helper whose
  error the image reports; `internal/instance/instance.go`, the refusal to
  start without a first snapshot; `internal/instance/estate.go`, the
  documents computed at the rendered tree's commit stamp.
- `tools/image/offline.sh` and `docs/guides/run-the-container-image.md` as
  they landed with the image.
- REQ-006.
- Issues #194 and #200.
