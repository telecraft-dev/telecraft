---
title: Implementing a provider
description: How to implement each seam, pass its shipped conformance kit, and name the result so the vendor-word lint stays clean.
order: 6
---

# Implementing a provider

A seam lets a vendor's system answer a question the core asks. The core
defines each seam in `internal/`, and every implementation lives under
`internal/provider/`. Nothing vendor-shaped crosses back: no query language,
no index name, no API type, no authentication scheme.

This page covers the four seams you implement against a vendor product:
TelemetryProvider, EstateProvider, InventoryProvider, and the forge adapter.
`internal/auth` is a seam on the same terms, and its first-party providers
follow the same rules.

Two rules apply to all of them, and both come from ADR-0008.

**Not knowing is a normal state.** An implementation that cannot answer, for
whatever reason (an unreachable backend, a missing index, a malformed
response), reports the affected reading with `Known` false and a `Cause`, and
returns no error. Degradation is data in the reading, never a fabricated value
and never a crash.

**Every reading carries the instant it was taken.** `AsOf` is set whether the
reading is known or not: "we could not see, as of when" is still a statement
with a timestamp.

## Naming, and the neutral door

Implementations are fully qualified with the vendor's **product**, never the
company. `Elasticsearch`, not `Elastic`. `ElasticFleet`, not `Fleet`.
`GrafanaFleetManagement`, not `Grafana`. The vendor-word lint enforces this
over `internal/provider/**`, and enforces the absence of the same words
everywhere else.

That means the caller cannot name your implementation. Each provider tree
exposes a neutral `New` taking a neutral `Config`, and that is the only door
a binary goes through:

```go
// internal/provider/telemetry/provider.go
func New(cfg Config) (seam.Provider, error)
```

When a second implementation lands, `Config` grows a selector and `New`
becomes a dispatch. Callers do not change. That is the test ADR-0008 sets for
a seam: a new implementation needs no core change.

Put your implementation in the existing package for its seam, one file per
implementation, and name the constructor after the product:
`NewElasticsearch`, `NewElasticFleet`, `NewKubernetes`, `NewGitHubApp`.
`Name()` returns the same qualified name as runtime data, for logs and stamps.

## TelemetryProvider

**Seam:** `internal/telemetry`. **Decisions:** ADR-0008, ADR-0009, ADR-0034,
ADR-0039, ADR-0040. **Shipped implementation:** `Elasticsearch`.

The seam answers one question in six shapes: did signal X arrive for Service
Y in window W.

```go
type Provider interface {
	Name() string
	Observe(ctx context.Context, service Service, window time.Duration, attributes []string) Observed
	AttributeNames(ctx context.Context, service Service, kind requirements.SignalKind, window time.Duration) AttributeNames
	DistinctValues(ctx context.Context, service Service, kind requirements.SignalKind, attribute string, window time.Duration) DistinctValues
	GroupNames(ctx context.Context, service Service, kind requirements.SignalKind, window time.Duration) GroupNames
	ObserveSelf(ctx context.Context, tier string, window time.Duration) SelfObserved
	Meter(ctx context.Context, tier string, window time.Duration) Metered
}
```

- `Observe` reads presence and volume per signal, plus the fraction of
  records carrying each requested attribute name, measured in the same round
  trip.
- `AttributeNames` reads the set of attribute names in use. It is the first
  of the three primitives that make schema-conformance checking pure string
  logic instead of a widened seam. An implementation that can only
  approximate, by sampling records for instance, says so through `Truncated`
  rather than silently.
- `DistinctValues` reads the values one attribute carries. It is hard-capped
  at `telemetry.MaxDistinctValues`, and a set that reached the cap, or a
  backend that held values outside the ones it returned, reads `Truncated`.
  Callers offer it only for the attributes the Schema Registry declares as
  enums; that constraint is caller-side, because a seam holding a registry
  would be a seam holding a vocabulary.
- `GroupNames` reads the values of the grouping key a signal is grouped by:
  span names for traces, metric names for metrics, event names for logs.
  semconv states its required-sets per group, so a conformance check cannot
  ask what is required until it knows which groups arrived.
- `ObserveSelf` reads collector self-telemetry for one Tier, matched on the
  `telecraft.tier` resource stamp every rendered artefact bakes into its own
  telemetry. This is the only door self-telemetry comes through: the platform
  reads it from the adopter's backend like any other telemetry, never over a
  privileged side channel.
- `Meter` reads one Tier's pipeline-grain flow counters. An implementation
  whose backend cannot aggregate that way returns `MeterUnknown` with a
  cause. Metering never invents, and a reading nobody can take is unknown,
  not a zero.

### Fidelity on the three primitives

All three are service-scoped by contract, and the rule runs both ways
(ADR-0034 §4). A reading you cannot narrow to one Service is `Known` false
with a cause, which `telemetry.NotServiceScoped` renders. There is no third
option, and the two that tempt are exactly the two the rule forbids.

The first is a silent approximation: answering with what the whole index
holds, because the index is easy to aggregate and the Service is not. The
second is a misattribution: reporting a value another Service put in that
index under this Service's name. Both read as knowledge downstream, and both
fail in the direction that turns a violation into a pass. A wrong answer
here is worse than no answer, because a wrong one is acted on.

The index-scoped union is sanctioned, but only as a screening fast path with
a service-scoped follow-up: a clean union proves every service in it clean, a
dirty union triggers the costly service-scoped aggregation to attribute the
violation. Nothing implements that today, and until something does, no
implementation returns a union from these methods.

The one thing that must not happen here is a backend's query syntax passing
through the seam. The moment it can, only one backend is ever really
supported.

The signal vocabulary belongs to `internal/requirements`. Adopt it rather
than minting a synonym.

## EstateProvider

**Seam:** `internal/estate`. **Decisions:** ADR-0008, ADR-0036. **Shipped
implementations:** `OpAMPDirect`, `ElasticFleet`.

```go
type Provider interface {
	Name() string
	Declaration() Declaration
	Estate(ctx context.Context) Estate
}
```

The seam is keyed on the collector and returns the whole estate in one call.

### Capability declaration

`Declaration()` is static: stated once, before any reading, and never varying
per collector or per call.

```go
type Declaration struct {
	Readings       map[ReadingKind]bool
	RefreshCadence time.Duration
}
```

`Readings` maps **every** kind `estate.Kinds()` defines (`effective`,
`health`, `delivery_status`) to whether this implementation can ever populate
it. Every kind must be present. Incapable is a declaration, never an
omission, because a silent gap and an honest "never" would otherwise be
indistinguishable.

That split is the point of the mechanism. A declared-incapable reading renders
as "not applicable" and is never a failure: `ElasticFleet` can never report
delivery status, and saying so up front stops a permanent absence from looking
like a fault. A reading declared capable but not delivered is **silent**,
which is a provider fault and is loud.

`RefreshCadence` is mandatory and positive. It is the input to the platform's
staleness arithmetic, and a declaration without one demotes every reading.

### The minimum populated set

For every collector returned:

- the **identity attributes** selectors match on. An empty identity is
  non-conforming: a reading nothing can match belongs to nobody.
- an **`AsOf` timestamp** on every reading carried, known or not.
- every capability-declared reading either populated with its timestamp or
  explicitly `Known` false with a cause.
- **pipeline component order preserved**, never a flat component list. A
  receiver wired only into traces is exactly the broken-pipeline case the
  product exists to catch.
- the **recursive component-health tree**, never the flattened roll-up.

A collector the implementation cannot find comes back as `Known: false`, never
an error.

### Staleness demotion

Freshness is the platform's arithmetic, never the provider's claim. You
declare a cadence; the platform computes the horizon as cadence multiplied by
`estate.StaleTolerance`, which is 3. Past the horizon, `ForEvaluation` demotes
the reading to `Known` false, so a stale Effective config can never feed a
fresh-looking verdict. Surfaces may still show last-known-plus-age. Stale data
may inform a human, never a verdict.

You do not implement any of this. You declare the cadence and take honest
timestamps.

## InventoryProvider

**Seam:** `internal/inventory`. **Decision:** ADR-0035. **Shipped
implementation:** `Kubernetes`.

```go
type Provider interface {
	Name() string
	Declaration() Declaration
	Expected(ctx context.Context, selector map[string]string) Count
}
```

One deliberately narrow question: given this Tier's selector, how many
instances should match right now. The seam is separate from EstateProvider by
design. The estate seam reads the population that exists; this one reads what
should exist, from the substrate. Different source, different auth, different
deployment shape.

`Count` carries `Known`, `Cause`, `AsOf` and `Instances`. A selector matching
nothing is a count of zero, which is a real reading and not a blind spot. An
empty selector, an unreachable substrate, or an ask the substrate cannot map
comes back `Known` false with a cause. The platform never invents a count.

Expectations built on the answer are **floors, never equalities**. The only
finding is a shortfall; surplus is never a finding. Source ranking per Tier is
derived, then declared, then absent: a live provider floats with the
autoscaler, a static `min_expected` covers substrates with no API, and no
provider plus no declaration means no teeth at all rather than a guess.

Staleness works exactly as it does on the estate seam: declare a positive
`RefreshCadence`, and `Count.ForEvaluation` demotes a count past
cadence multiplied by 3 so it cannot float a fresh-looking floor.

## The forge adapter

**Seam:** `internal/forge`. **Decisions:** ADR-0014, ADR-0028. **Shipped
implementation:** `GitHubApp`.

```go
type Forge interface {
	Name() string
	Capabilities() Capabilities
	Propose(ctx context.Context, change Change) (Proposal, error)
}
```

The seam is deliberately small. A change is a branch, a message, an acting
human and a set of file contents. A proposal is an opaque identifier and a
URL. No forge's API types, review vocabulary or authentication scheme crosses
it in either direction.

`Propose` opens the change proposal, or refreshes it when the branch already
carries one, moving the branch to a new commit authored by the acting human
and updating the title and body. It is idempotent per branch and file set:
re-proposing the same content is not an error. That matters because the
platform re-renders on every push to the branch.

`Capabilities()` is the static ladder declaration from ADR-0028: `Proposals`,
`ReviewRouting`, `Annotations`, `VerifiedAttribution`. Capability is what the
forge is, not how it feels right now, so the declaration is constant per
implementation. Bare git sits at the bottom of the ladder: branch push over
SSH, manual merge, unverified git-author attribution. Validation and the
render gate still hold there; forge-enforced human review is what that adopter
forfeits.

Onboarding is a repository URL plus a credential, per repository. The neutral
`internal/provider/forge.New` splits the URL and dispatches on its host, so
further rungs of the ladder land there without changing a caller.

## The conformance kits

Two seams ship their contract as a fixture suite rather than as prose. The
rule is not a paragraph you are asked to honour: it is a test you pass or fail.

```go
// internal/provider/estate/opampdirect_test.go
estatetest.Run(t, estatetest.Kit{Provider: p, Seeded: seeds})

// internal/provider/inventory/kubernetes_test.go
inventorytest.Run(t, inventorytest.Kit{Provider: p, Seeded: seeds})
```

`internal/estate/estatetest` takes the implementation already reading a
fixture estate, the `Seeded` collectors the harness arranged with the exact
readings the provider must reproduce, and optionally an `Absent` identity
guaranteed to match nothing. `Seeded` must be non-empty: a run over an empty
estate would pass vacuously. Nil or zero expectation fields go unchecked, so a
harness states only what it actually controls.

The kit checks capability honesty, the minimum populated set, structural
preservation of pipeline order and the health tree, the unknown-collector
discipline, and staleness demotion.

`internal/inventory/inventorytest` takes the implementation pointed at a
substrate the harness controls, `Seeded` selectors with the exact count for
each, and optionally an `Unanswerable` selector the provider genuinely cannot
answer. Seed at least one selector with a positive count, so staleness
demotion has a real reading to exercise. It checks the positive declared
cadence, known counts with their timestamps, honest unknowns, and demotion.

Both kits expose `Run(t, kit)`, which fails the test with one actionable line
per violation, and `Violations(ctx, kit)`, which returns the same judgement as
data. The second form is how the kits' own tests prove that a deliberately
broken provider gets caught.

ADR-0008 asked for the seam to be verified against a third implementation.
The kits are how that stays true: a new implementation passes `Run`, or it
does not conform.

## Live tests

Every provider gets a gated live suite beside its unit tests, named
`*_live_test.go`, with test names containing `Live` so `-run Live` selects
them. The suite reads its configuration from the environment and calls
`t.Skip` with a message naming the variables when they are absent. An absent
credential is a skip, never a red suite.

The [development page](development.md#the-live-backend-suites) lists the
variables each suite reads. Two of the suites run in CI, and the rest run
where someone has the system to point them at.
