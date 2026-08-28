# ADR-0067: One Instance server: the console, the API and the OpAMP endpoint in one process; TLS terminates in front

- Status: accepted
- Date: 2026-08-28

## Context

Telecraft has no process that serves the product.

`telecraft serve` is the stateless OpAMP server of ADR-0013, and it serves
collectors only. The platform API documented in `console/README.md` exists
twice and ships neither time: `console/tools/fixture-backend.mjs` is a
hand-written mock the console's own tests run against, and the dev-only
server inside `devenv/cmd/telecraft-devenv` serves a snapshot to a
read-only console. ADR-0052 §5 named that limit and refused to lift it,
because lifting it means building the platform API server and that is its
own decision. `internal/auth` is written and tested against the documented
contract, down to `Require` and the whole route set, and nothing outside
`telecraft passwd` imports it.

So the product has a console with no server to call, an API with no
implementation, and an authentication package mounted by nothing.
Packaging, Kubernetes and hosting all wait on the same missing answer:
what a Telecraft deployment is.

Four decisions bound the answer. REQ-006 and ADR-0019: air-gapped
deployment is first class and nothing depends on a SaaS. ADR-0032:
git-the-tool and never git-the-service, the serving path holds a closed
list of rebuildable caches, and a single binary plus a directory is a
complete standalone instance. ADR-0045 §5: no asset is fetched from
another origin, checked in CI. ADR-0008 and ADR-0036: the reading of
Served collectors comes off the OpAMP wire through the EstateProvider
seam.

## Decision

### 1. The Instance server is `telecraft serve`, grown to serve the whole estate

One command, one process. Over one estate source it serves the console
bundle, the platform API under `/api/v1/`, the auth round trips under
`/api/v1/auth/`, and the OpAMP endpoint at `/v1/opamp`. Everything under
`/api/v1/` that is not an auth route is mounted behind `auth.Handler`'s
`Require`, which is the gate that package was written to be.

The command keeps its name because what it does is unchanged in kind: it
serves the estate, and now it serves all of it.

Humans and collectors arrive on separate addresses, so a deployment can
expose one and not the other: `-listen` stays the OpAMP address and
`-http` is the new one. The HTTP endpoint is always open, because it
carries the probes. The OpAMP endpoint closes on an empty address, which
is the shape of an Instance whose collectors are all Foreign and reached
through a vendor EstateProvider instead.

An unknown path under `/api/v1/` is a 404 with a JSON body. Every other
unknown path is the console's `index.html`, so a deep link survives a
reload and ADR-0042 §3.5's everything-in-the-URL rule holds outside the
dev server. The bundle itself is served signed out, because a 401 from
the API is what renders the sign-in surface.

The Instance server does not render and never merges. Rendering happens
in the pull request (ADR-0028); the write endpoints leave through the
forge adapter as proposals. An Instance with no forge credential answers
those endpoints with a refusal that names what is missing, never a 500.

### 2. One process, because the Served reading is this process's own wire

`estate.OpAMPDirect` answers the EstateProvider seam as a `serving.Tap`
over the connections the OpAMP server holds (ADR-0008). Splitting the
roles across two processes means the API process cannot see the Served
population without either an inter-process channel or shared storage.
Storage is refused by ADR-0032, and a channel is a distributed system
built for a problem no adopter has yet, which is the kind of building
REQ-060 exists to postpone.

So the two roles share one estate source, one fetch, one poll, one
snapshot pointer and one compiled selector index. The API projects its
documents from the same pointer the serving matcher reads, so the console
cannot report a head the server is not serving.

The cost is stated rather than hidden: the Served reading is scoped to
the connections this process holds. Two Instance servers behind one
address split it, and the console would report half an estate as though
it were whole. One Instance is therefore one Instance server. ADR-0032 §2
stands where it was argued, over the serving decision itself: N read-only
clones make the same choice from the same head. It was never a licence to
run two consoles.

### 3. The console travels inside the binary

The built bundle is embedded in the `telecraft` binary and served from
there. ADR-0032 §3 promises that a single binary plus a directory is a
complete instance, and an instance whose UI is a separate tree somebody
has to place beside it does not keep that promise. One artefact also
means one version, which is what ADR-0065 assumes when the console names
the release it came from, and the zero-CDN rule then holds at run time by
construction rather than only at build time.

A binary built with no bundle answers the console route with a plain page
saying the console was not built into it, and serves the API normally, so
`go build ./...`, `go vet ./...` and `go test ./...` still pass on a
checkout where npm has never run.

### 4. Flags are the configuration surface, and the estate carries everything shaped

Flags first. Every flag has an environment variable, `TELECRAFT_` plus
the flag name upper-cased with dashes as underscores, so a container
configures the process without an entrypoint that rewrites arguments.
Precedence is flag, then environment variable, then default.

There is no config file. Three ways to say one thing is two too many, and
the flag set stays small only because the shaped configuration is not in
it: what describes the estate is authored in the estate, under review.
The providers an Instance offers are authored in `auth.yaml`, beside
`teams.yaml` and `users.yaml` in the ownership directory and on the same
seam pattern: each entry names its kind, the name the sign-in surface
shows, and its issuer, and it names its secret rather than carrying it.
Changing who may sign in is then a pull request, exactly like changing
who may author.

Process configuration carries only what git must not: the addresses, the
external URL, the fetch interval, the cache directory, the session key,
and the values of the secrets the estate names. How a secret value
reaches the process is a separate decision.

The session key is `-session-key-file`, and its absence draws a random
key at start, which is what `auth.NewSessions` already does: sessions
then live as long as the process. The start-up line says so, because a
restart signing everybody out is worth one line of warning.

### 5. TLS terminates in front, and the external URL says what the outside sees

The process holds no certificate, in any deployment shape. Both endpoints
speak plain HTTP, and TLS terminates in an ingress, a load balancer, a
reverse proxy, or nowhere at all on a loopback address where nothing sits
between the browser and the process. Issue, renewal and rotation are
disciplines the surrounding platform already runs, and running a second
copy of them inside the binary would make them worse rather than
optional.

`-external-url` declares the URL the Instance is reached at. Its scheme
decides whether session cookies are marked Secure, which is the
`HandlerConfig.Secure` switch `internal/auth` was written around, and it
is the base the OIDC redirect is built from, so a provider round trip
returns to the address the browser used.

It fails closed. The server refuses to start when the external URL names
a non-loopback host over `http`, unless `-insecure-http` says the
operator means it. A password crossing a network in clear text is not a
default.

### 6. Two unauthenticated probe paths, and one process to answer for

`/healthz` and `/readyz` sit on the HTTP endpoint, outside `/api/v1/`,
unauthenticated, and answer a status word and nothing else: no estate
content passes an unauthenticated route.

`/healthz` is liveness and answers 200 while the process runs. `/readyz`
is readiness: 503 until the first snapshot is held, 200 after it. A
refresh that fails later keeps the last snapshot and readiness stays
green, because a stale head serves correct configuration for the commit
it names, and delivery must not stop because a fetch failed (ADR-0010
never serves empty). The head and the snapshot's age are readable on the
API, behind a session.

One process means one probe. The OpAMP endpoint gets none of its own: it
is up when the process is, and a second probe would report the same fact
twice.

### 7. The closed list is unchanged, and the storage audit widens to cover this process

The Instance server adds nothing durable. What it holds is the repo
snapshot and its selector index (ADR-0032 §1.1), the per-connection
layer-1 digest (§1.2), and the per-connection reading `OpAMPDirect`
already keeps, which is derivable from live connections and dies with
them. Sessions are signed and verified, never looked up. Users, teams,
ownership, governance, the Catalogue and the activations all live in
git. The per-user Presentation store stays in the browser (ADR-0042 §7).
The API documents are a pure function of the snapshot and the live
readings, computed per request or memoised loseably, the ADR-0038
discipline.

So ADR-0032 stands unamended, and its audit widens:
`TestStorageInventoryIsTheClosedList` covers the Instance server's own
type the way it covers `serving.Server`. A field added there fails the
audit until it is classified, and a field holding collector data or a
human's work fails it until ADR-0032 is amended.

The posture has a price, and it is small: sign-in throttling and any
other rate limit is per process and in memory, so it resets on restart.
A counter that survived a restart would be the first durable record this
product keeps, and it is not worth being the first.

## Consequences

- The documented contract gains an implementation adopters run. The
  fixture backend's job narrows to the console's own tests, and the drift
  ADR-0052 named becomes a difference between two implementations rather
  than between an implementation and a document.
- The devenv's read-only console can become a live one, and the write
  paths that terminate at a notice today can terminate in a proposal.
  That is a follow-on, not part of this decision.
- Packaging has one thing to package: one binary, one command, two
  addresses, no sidecar and no separate asset tree.
- Multi-tenancy now has a shape to argue against. ADR-0019 gives one
  Instance per isolation domain and this gives one process per Instance,
  so a tenancy unit is either a process or a partition inside one, and
  that is the next decision's to take.
- One Instance server per Instance is a ceiling on the read path while
  the Served reading lives in process memory. Raised as OQ-19.
- Exposing the OpAMP endpoint through a terminator alongside the console
  makes "what does a collector present, and what refuses it" a live
  question the corpus does not answer. Raised as OQ-20.
- `telecraft serve` stops being a delivery command and becomes the
  product's one long-running process, so `docs/guides/serve-configs.md`
  and `docs/reference/cli.md` both describe something narrower than what
  will exist, and the build work carries their update.

## Sources

- ADR-0008 (the EstateProvider seam), ADR-0010 (never serve empty),
  ADR-0013 (the stateless server), ADR-0019 (pluggable authentication,
  ownership-derived authorization, air-gap first-class), ADR-0028 (render
  in the pull request), ADR-0032 (the closed list, coordination-free
  replicas, git-the-tool), ADR-0036 (the seam's minimum populated set),
  ADR-0038 (loseable memoisation), ADR-0042 §3.5 and §7, ADR-0045 §5 and
  §6, ADR-0052 §5 (the read-only console and the server it refused to
  build), ADR-0065 (the console names its version).
- `console/README.md` (the platform API contract), `internal/auth/http.go`
  (the route set and `Require`), `internal/serving/server.go` and its
  storage audit, `internal/provider/estate/opampdirect.go`.
- REQ-006, REQ-017, REQ-040, REQ-060.
