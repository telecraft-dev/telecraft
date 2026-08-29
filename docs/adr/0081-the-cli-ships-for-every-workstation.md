# ADR-0081: The CLI ships for every workstation, and building it is for contributors

- Status: accepted (amends ADR-0068 §4)
- Date: 2026-08-29

## Context

ADR-0068 §4 attached two Linux binaries to a release and refused the rest:

> Nothing else is attached. No macOS, no Windows, no other architecture:
> none of those is built for anything else, and building them is the matrix
> ADR-0049 declined. Every other platform builds from source in the one
> command the quickstart documents, and that stays the documented way to get
> the CLI on a workstation.

Both halves of that were reasoned from cost. The two Linux binaries were
free because the image builds them anyway, and the others were not, so the
line fell where the image happened to put it.

What the line ignores is who is standing on each side of it. The Linux
binaries serve a pipeline, which is somebody who has already adopted the
product. A workstation is where somebody decides whether to. The quickstart
promises a verdict in about five minutes and opens by requiring Go 1.26 and
a clone of the platform, so the first thing the product asks of an evaluator
is a toolchain it needs for nothing else, at the one moment it has to prove
itself. A reader on macOS, which is most of them, meets that before they
have any reason to want it.

The cost was also overstated. The build is `CGO_ENABLED=0` with `-trimpath`
already, so a further platform is one `GOOS` and `GOARCH` pair and one more
`go build` in the job that is already running. It buys no runners, no
toolchain, and no separate pipeline.

## Decision

### 1. A release attaches a binary for every workstation the product expects to meet

Five, not two: `linux-amd64`, `linux-arm64`, `darwin-arm64`, `darwin-amd64`
and `windows-amd64.exe`, named `telecraft-<version>-<os>-<arch>` as the two
existing ones already are. `SHA256SUMS` covers all five, and the build
provenance attestation covers all five, because an artefact somebody
downloads and runs is exactly the one worth attesting.

The two Linux binaries keep coming from the image's staged context rather
than being built twice. The other three are cross-compiled in the same job.

### 2. Building from source is documented for contributors, not for readers

`go build ./cmd/telecraft` stays exactly as it is and stays correct. It stops
being the first thing a quickstart says. The quickstart leads with a download
and a checksum, the container image is offered beside it for a reader who
would rather not place a binary, and building from source moves to the end of
the page under a heading that says who it is for.

This is the part ADR-0068 §4 got round the wrong way. Building from source is
not a way to get the CLI on a workstation; it is what you do when you are
changing the CLI.

### 3. No package managers, and no installer script

No Homebrew tap, no Scoop manifest, no `curl | sh`. Each is a second
published surface with its own staleness, and the first two are a claim to
keep current in a repository this project does not own. A download and a
checksum are two commands and no third party. This is recorded so that the
next person to ask is answered rather than relitigating it.

## Consequences

The quickstart no longer requires a Go toolchain, so `docs/guides/quickstart.md`
and the first-verdict block on `telecraft.dev` both change with this.

The release notes stop saying that every platform but Linux builds from
source, because that is no longer true.

Windows is attached and is not otherwise exercised: nothing in CI runs a
Windows binary, and the CLI's file handling has never been read with Windows
paths in mind. It ships because refusing it costs a reader more than an
untested build does, and the honest statement of that belongs in the
quickstart rather than in a footnote nobody reaches.

`darwin-amd64` is attached for the Intel Macs still in use. When the release
notes stop mentioning it, that is the signal it can go.
