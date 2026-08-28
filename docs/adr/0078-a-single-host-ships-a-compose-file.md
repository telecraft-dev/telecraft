# ADR-0078: A single host is no longer one service, so it ships a Compose file

- Status: accepted (amends ADR-0068 §5)
- Date: 2026-08-28

## Context

ADR-0068 §5 closes with the shape below the chart: "A single host is the
image and the documented flags. No Compose file: one service needs no file
to describe how it composes with the others."

The refusal followed from its premise, and two later decisions removed the
premise.

ADR-0074 §4 put the estate fetch outside the container. The image carries no
git, so `-repo` fails inside it and the supported shape is `serve -estate`
over a directory mounted read only, advanced by something else: a timer on
the host, a runner that publishes a tree, or a container beside the server.
The process no longer reaches its own estate. It reads a path another thing
owns, at a bounded staleness the two of them set between them (ADR-0032 §1).

ADR-0067 §5 keeps a certificate out of the process in every deployment
shape. A host reached at anything but a loopback address runs a terminator
in front of it, which on one host is a second container with its own
addresses, its own configuration and its own certificate directory. It is
not optional either: the server refuses to start on a non-loopback external
URL over plain HTTP.

ADR-0071 §3 adds the third piece. Every secret is a file the deployment
places, in the directory the command is told to read.

What is left is not one service. It is a server, an estate directory
something else keeps current, a secrets directory, two addresses published
on different interfaces, and a terminator that carries the OpAMP endpoint's
WebSocket upgrade through. ADR-0074 amended ADR-0068 §5 in the shape of what
Kubernetes runs and left this sentence standing, because there the same
arrangement is one pod and the chart already held it. On one host nothing
held it, so it was prose an operator retyped.

`deploy/compose/`, `docs/guides/deploy-with-compose.md` and
`tools/composelint` shipped against the sentence as written (issue #201).

## Decision

### 1. A single host is the image, a Compose file, and the guide around it

`deploy/compose/` holds three files: the compose file, the environment file
every value it reads comes from, and the terminator's configuration. It is
ADR-0074 §4's shape written down rather than described: the estate mounted
read only, the secrets presented as files where `-secrets-dir` looks for
them, each address published where it belongs, and the terminator behind a
profile so a first run answers before there is a certificate to give it.

The documented flags are still the whole configuration surface (ADR-0067
§4), and nothing here is a second way to configure the process. What the
file carries is the arrangement around it, which is the thing the struck
sentence said did not exist.

### 2. It is a file an operator copies, not an artefact a release publishes

It travels in the source at the tag, which is published already, and it is
copied out of the tree to wherever the deployment lives. ADR-0068 §6's count
is unchanged: four artefacts, two mechanisms, and nothing new to mirror
except the terminator's image, which is not this project's and is the same
extra reference ADR-0074 §5 already accepted for the sidecar.

The terminator the file names is an example, and its image is a value. A
file that started nothing would demonstrate nothing, and what this one
demonstrates is the two things the Instance needs from whatever sits in
front: the console on one address, and the OpAMP endpoint with its upgrade
carried through. Any terminator an operator already runs replaces it, and
nothing in the deployment depends on which one answers.

### 3. What it is not allowed to become

It deploys the Instance server and what terminates in front of it. No
collector, no Supervisor, no rendered configuration: ADR-0012's line holds
exactly where ADR-0068 §5 drew it for the chart, and the file starts nothing
in the telemetry path (ADR-0002). It is not a route onto a cluster, and the
chart stays the only Kubernetes answer. It is not a new deployment shape
either: it is the single host ADR-0068 §5 already decided, with its
arrangement in a file instead of in a reader's head.

## Consequences

- ADR-0068 §5's last two sentences are struck and replaced by §1 above.
  Everything else in §5 stands as written: the chart, the single replica and
  the Recreate strategy, the probes, the rebuildable cache, the argument for
  a chart over a manifest set, and the versioning.
- The file is checked rather than asserted. `tools/composelint` reads the
  address defaults out of `cmd/telecraft/serve.go` and fails on a writable
  estate mount, a secret carried as a value, or a variable the environment
  file never names, so a flag that moves fails here rather than in an
  adopter's deployment. That is ADR-0068 §5's "in this repository, not a
  sibling" applied to the shape below the chart.
- An adopter between `go build` and a cluster now has a documented
  deployment, which is the gap ADR-0068 §5 left open by answering the
  question with a flag list.
- The guide is where the two halves meet: the fetch that keeps the checkout
  current runs on the host, not in the file, and a guide that disagreed with
  the file beside it would be worse than either.
- No new capitalised domain terms. A compose file, a profile, a mount, a
  secret and a terminator are the industry's words for the industry's
  things, and the glossary is unchanged.

## Sources

- ADR-0068 §5 (the chart, and the sentence amended here), §2 (the image and
  the base that carries no git) and §6 (the artefacts and the two
  mechanisms).
- ADR-0074 §4 (the mounted checkout and the fetch outside the container) and
  §5 (one more image to mirror, and no default blessed for it), ADR-0067 §4
  and §5 (the flag surface, TLS in front, and the external URL that fails
  closed), ADR-0071 §3 (secrets are files a deployment places), ADR-0032 §1
  and §3 (bounded staleness, git-the-tool, and the local bare repository),
  ADR-0012 (Kubernetes as the control plane's substrate), ADR-0002 (nothing
  in the telemetry path).
- `deploy/compose/compose.yaml`, `deploy/compose/.env.example`,
  `deploy/compose/proxy/telecraft.conf`,
  `docs/guides/deploy-with-compose.md`, and `tools/composelint` as they
  landed.
- Issue #201.
