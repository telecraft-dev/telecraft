# ADR-0052: A local development environment that reads the runtime seams for real

- Status: accepted
- Date: 2026-08-21

## Context

Nothing in local development has ever read a live seam.

The console's data comes from one of two places. `npm run backend` serves
`console/tools/fixture-backend.mjs`, a hand-written mock of the documented
platform API; `telecraft snapshot` computes the same documents with the real
evaluators, over an estate whose two runtime readings are authored YAML.
`estate-demo/demo/readings.yaml` states the position in its own header: in
production the collector estate arrives through the EstateProvider seam and
the arrivals through the TelemetryProvider seam, both from live systems, and a
demo estate has neither, so it declares them.

That is the right answer for a demo (ADR-0049) and the wrong one for
development. A contributor (or an agent working on the contributor's behalf)
can see every surface and has never seen the product. No collector has
connected to `telecraft serve` on a laptop. No span has crossed a rendered
pipeline into a backend. `broken_pipeline`, the one verdict the product exists
to produce (README, ADR-0004), has only ever been asserted by a fixture.

The parts to close the loop are already built and have never been wired
together outside a test: `internal/serving` is a real opamp-go server;
`internal/provider/estate/opampdirect.go` answers the EstateProvider seam from
that server's own wire (ADR-0008, ADR-0032); the renderer emits a supervisor
artefact beside every collector artefact (ADR-0010, REQ-032); and the
Elasticsearch TelemetryProvider's default index patterns and OTel-native field
layout are exactly what an OTLP intake writes.

Two constraints bound whatever gets built. ADR-0001 scopes `cmd/**` and
`internal/**` as neutral core, where no vendor word appears at all, and a
development environment must name a concrete backend to start one. And the
platform has no HTTP API server: the documented API is served by the fixture
backend or not at all, so a live console has no server to call.

## Decision

### 1. The devenv is a vendor-bound tree with its own lint scope

`devenv/` holds the whole environment, and `vendorlint.yaml` gains a `devenv`
scope carrying the provider scope's rules: a product may be named, and only
fully qualified (`Elasticsearch`, never `Elastic`; no bare `Fleet`).

The alternative was to leave the tree unscanned beside `.proto/` and `.teach/`.
Rejected: an unscanned tree is the one place a vendor word spreads without
anyone noticing, and the boundary ADR-0001 draws is worth more than the one
line it would have saved. The devenv is vendor-bound for the same reason
`internal/provider/` is, so it lives under the same discipline.

The consequence is placement. The devenv's binary lives at
`devenv/cmd/telecraft-devenv/`, outside `cmd/**` rather than smuggled through
it, and is a development tool rather than a fourth product binary.

### 2. The two runtime readings are read, and everything else is authored

The devenv is the first place `readings.yaml` is a *product of* the seams
rather than a stand-in for them. The devenv binary runs the OpAMP server with
`estate.OpAMPDirect` as its Tap, reads arrivals through the TelemetryProvider,
and writes the collector estate and the arrivals into the readings file that
`internal/console` already loads.

`rows.yaml` stays authored. The Effective reading per Service and Environment
is derivable (a Service's Paths name its Tiers, and the serving matcher
already maps a Tier to its collectors), but deriving it needs judgements the
product has not made: which collector represents a row, and what a
disagreement between replicas of one Tier means. The devenv does not get to
invent product semantics ahead of the product. Rows are authored in the
platform today, and they are authored here.

So the devenv makes real: populations and their floors, matching and the
Unmatched artefact (ADR-0030), delivery status and drift (ADR-0004, ADR-0005),
and every verdict that crosses a configured pipeline against what arrived. It
leaves declared exactly the one thing the product also declares.

### 3. Nothing in the devenv judges anything

The binary loads, wires, and projects. It turns two live seams into the two
files `console.Build` already reads, and every band, finding, population and
verdict downstream is the return value of the package that owns it. The rule
is `internal/console`'s own (`console.go`), and it applies here for the same
reason: a development environment that computed its own verdicts would be a
second implementation, and the first thing it would hide is a bug in the
first.

### 4. The estate is rendered at a fixed synthetic commit

`devenv/estate/` is not its own git repository. It lives inside this one, so
stamping its rendered artefacts with a real SHA would make the tree stale on
every commit to the platform, and `console.Build`'s recompute invariant
(ADR-0028 §2) would fail perpetually and stop meaning anything.

The estate renders at a fixed 40-hex constant that reads as obviously
synthetic. The recompute check then does its actual job: it catches an estate
whose sources have moved and whose `rendered/` has not, which is the failure it
exists to catch. A Go test runs the same build with no Docker and no backend,
so CI holds the devenv estate to it on every pull request.

### 5. The live console is read-only, and says so

The console reads the devenv's snapshot through demo mode (ADR-0049, issue
#50), because demo mode is the shape that already exists for a console with no
server to call. Read-only falls out by construction, and the write surfaces
still render in full: the composer still validates on every keystroke, the
claim flow still previews impact, and each terminates at the notice explaining
that a real instance opens a pull request instead (ADR-0003, ADR-0028).

Authoring's write paths therefore stay on the fixture backend, exactly as they
are today. That is a real limit and it is the honest one: making them live
means building the platform API server, which is its own decision and not a
development environment's to take.

## Consequences

- The fixture backend's drift from the documented API becomes more visible,
  not less: the devenv shows the same surfaces over the real evaluators beside
  a mock that has to be kept in step by hand. That pressure is intended, and
  the resolution is a server, not a better mock.
- The rendered supervisor artefact is revealed to be un-runnable on its own.
  It carries no identifying attributes, because matching is on what a collector
  reports and the operator supplies that at install
  (`docs/guides/serve-configs.md`). The devenv merges an identity overlay over
  it per collector. Whether the renderer should emit an identity stanza is a
  product question this work surfaces and does not answer.
- Docker becomes a prerequisite for one optional workflow. It stays absent from
  the default path: `go build ./...`, `go vet ./...`, `go test ./...` and both
  lints run on a clean checkout with no daemon, and the devenv's own tests are
  pure.
- The devenv pins image versions, so it ages. A pin that has drifted from the
  catalogue the estate is judged against is a real finding about the product's
  version handling, and it should be read as one rather than silently bumped.
- The devenv is offline after one image pull and one committed Catalogue
  import. Anything added later that reaches the network at run time breaks the
  air-gap-first principle the platform claims (ADR-0032), in the one place a
  contributor would notice.

## Sources

- ADR-0001 (the neutral core boundary), ADR-0004 and ADR-0005 (Intended ×
  Effective), ADR-0008 (the EstateProvider seam and not knowing), ADR-0010
  (the supervisor), ADR-0013 (the stateless server), ADR-0028 §2 (the
  recompute invariant), ADR-0030 (the Unmatched artefact), ADR-0032
  (statelessness and air-gap), ADR-0049 and issue #50 (the demo and its
  snapshot).
- `estate-demo/demo/readings.yaml`, whose header states the problem this
  decision answers.
