---
title: Pipeline observability
description: Expectations and Claims, Settle windows, self-telemetry ingestion, metering at two grains, and the card contract that surfaces them.
order: 7
---

# Pipeline observability

This is the part that makes green mean "the configuration worked" rather than
"the configuration applied". From the Intended configuration at a commit SHA,
Telecraft derives what telemetry should arrive, then checks whether it did.

## Expectations

An **Expectation** is a checkable statement about what should be observable.
Telecraft derives it from the Intended configuration at a commit SHA; nobody
authors it.

Three separate reds live side by side, and Telecraft never blends them:

- **Delivery-red**: the configuration never applied.
- **Expectation-red**: the configuration applied and did not work.
- **Conformance-red**: telemetry arrived and is wrong.

An Expectation is not a Requirement either. A Requirement is what a person
demands. An Expectation is what the configuration implies. They meet at the
verdict, not in the model.

The Expectation engine is machinery, not vocabulary. It adds no eighth outcome
and no fourth reading. Data claims are the computation that decides the
Observed leg of the [outcome cross](readings-and-verdicts.md#the-outcome-cross):
a Service's "expected traces never landed" *is* `not_delivered`, and the
Expectation is why the evaluator knew to look for traces at all. Pipeline
claims join the Tier-attached finding family instead.

## Claims

A **Claim** is the unit of an Expectation: one checkable assertion derived
from a rendered artefact at a commit SHA. There are three kinds.

**Arrival**, keyed per Service, Environment, and signal. The signal should
land, derived from the Service's [Paths](authoring.md#the-objects) through the
rendered pipelines. This feeds `not_delivered`.

**Enrichment**, keyed per Service and Environment. Attributes the rendered
configuration explicitly and literally inserts or upserts should be present on
landed telemetry. Only static actions with constant values derive a claim.

**Self-telemetry**, keyed per Tier. Each instantiated component should emit its
own telemetry.

### Literal-only

The engine claims only what it can read off the artefact. It never claims what
it believes about a component's semantics.

So a `transform` processor that sets a constant attribute produces an
enrichment claim, while a component whose output depends on runtime discovery
of its surroundings produces **no claim at all**, and therefore reads
`unknown`, never red.

### Derived at evaluation time, never committed

No expectations file exists. Derivation runs at evaluation time as a pure
function of the artefact, memoised in memory by SHA, and safe to lose.

The change-proposal check *displays* the expectation diff, in the style of an
impact report: "this change adds an arrival claim for traces". It is computed
twice and stored never.

### Judged at the stamped SHA

Claims are judged against the artefact the collector reports running, the
stamped SHA, never head.

This means a delivery failure cannot cascade into expectation-red: an
unapplied configuration's claims are never in force, and the running
artefact's claims still are.

One consequence is worth naming. A collector pinned to an ancient artefact is
judged green against ancient claims. That is correct: the staleness is
delivery's red, and blending the two would hide it.

### Settle windows

A **Settle window** is the period after a configuration goes `APPLIED` at a new
SHA during which its claims read neutral-pending: never red, never green.

Self-telemetry settles in seconds; arrival and enrichment take longer. The
shipped defaults are 30 seconds for self-telemetry claims and 30 minutes for
arrival and enrichment claims.

A Settle window is not the observation window. The observation window is how
far back a claim looks, defaulting to 24 hours, long enough to survive an
overnight quiet period. Persistence dampens shortfalls, using the same
mechanism Population floors use, so one blip never paints an estate red.

A claim reads one of four statuses: green, red, `pending` inside its Settle
window, or `unknown` when the reading behind it is not available.

### Where claim failures land

- **Requirement-backed data claims** raise no finding of their own. They are
  the machinery behind the cross, and the Requirement already owns their
  severity and routing.
- **Unbacked data claims**, where the configuration implies a signal no
  Requirement demands and it never arrives, raise a Service-attached finding
  of kind `expectation`. It is advisory grade and never a violation: no person
  demanded the signal, so it cannot fail compliance. The remediation text
  names both options: fix the pipeline, or delete the dead lane. That doubles
  as dead-configuration detection.
- **Pipeline claims** raise Tier-attached findings routed to the Tier's owner.
  These *can* reach violation grade once dampened. An instantiated exporter at
  100% send failure is "the configuration didn't work" in its sharpest form.

A dead lane might be intentional, whereas a failing instantiated component
never is.

`expectation` is its own [roll-up column](ownership.md#finding-kinds), never
blended, and Exemptions apply to it unmodified.

## Self-telemetry

Claims about pipelines need the collector's own health data, and Telecraft
reads it the same way it reads everything else: through the
`TelemetryProvider` seam, from your backend, with no privileged side channel.

Getting it there takes a rendered pattern, because the collector's defaults
will not do it. The default surface is a localhost Prometheus endpoint that
nobody scrapes.

**Every rendered artefact configures OTLP push of internal telemetry** to the
self-telemetry destination: metrics at level `normal` through a periodic
reader, and internal logs through a batch processor. Internal traces stay off.
The localhost Prometheus default is left untouched, so local debugging keeps
working.

Self-telemetry is mandatory in every rendered artefact. The
[Unmatched artefact](delivery.md#the-unmatched-artefact) is already
self-telemetry only, and a governed Tier should never report less than the
ungoverned fallback does. The ingest volume this creates is a real cost, and a
named one.

You declare the destination once, at estate level, and the renderer resolves
it per Tier. One routing rule governs it: **a Tier's self-telemetry never
depends on that Tier's own data pipelines.** A gateway does not export its own
health through itself. An edge Tier whose only network path runs through a
gateway can transit it as ordinary data. The shared fate is topological
reality rather than a design flaw: transport loss reads as `Known: false`,
never red.

The renderer owns the telemetry block. Blueprints do not author it.

### Joining readings back to configuration

One normalisation layer, owned by Telecraft, maps the identity attributes a
reading carries back to the component in the rendered YAML, keyed by kind and
id. Both generations of join key meet there and nowhere else:

- **Metrics** join on the legacy datapoint attributes, each holding the full
  `type/name` id exactly as rendered. These are the primary keys, on by
  default, and what every dashboard in the wild already joins on.
- **Logs** join on the `otelcol.component.*` instrumentation scope attributes.
  The same scope attributes on metrics sit behind an upstream alpha gate that
  ships off, so they are the alternate key.

Upstream's awkward corners are modelled as expected shapes, never as join
failures: synthetic graph nodes, singleton components that drop identity
attributes by design, two-level scraper identity, and a prefix trap between
two similarly named attributes. Receivers and exporters carry no pipeline id
on any surface, so pipeline membership is derived from the rendered
configuration topology rather than from telemetry.

One collector process start is an **Incarnation**, identified by
`service.instance.id`. It rotates on every restart, and Telecraft keeps that
rotation, because incarnation churn per Tier is the restart-rate reading.

## Metering

**Metering** is the family of derived flow readings, computed on read and
stored nowhere. No metering pipeline exists, and a derived value lives exactly
as long as the request that asked for it. History is a range query against
your backend at your retention.

### Two grains, never blended

**Pipeline grain** comes from self-telemetry, per Tier and signal. In is
receiver-accepted, out is per-exporter sent, summed across instances. A Hop's
throughput is its feeding exporter's out-rate.

**Service grain** comes from the Observed data itself, per Service,
Environment, and signal.

They are separate types with no arithmetic between them. Per-service flow
through a Tier does not exist in self-telemetry, and dividing one grain by the
other would invent it.

Items are the unit: spans, metric points, and log records, which is what the
helper metrics emit. There is no byte field, because no byte reading exists on
those surfaces and an estimate would be an invented number.

### Reduction, never loss

In minus out is **reduction**, and reduction is presented, never judged. A
filter processor dropping 90% of records is doing exactly the job it was
authored to do. "Loss" is not vocabulary here, because a word that implies
fault would make every correctly authored filter look like one.

Negative reductions are real and stay negative: a connector, or a fan-out to
two exporters, sends more items than the receivers accepted.

The only reds the meter itself sources are the error-rate readings: refused,
send-failed, and enqueue-failed. Those feed pipeline claims. The reduction
figure feeds nothing that grades anyone.

### Freshness and honesty

Freshness exists at both grains: the age of the newest landed record per
Service, Environment, and signal, and the age of the newest self-telemetry
reading per Tier, with incarnation churn beside it.

Metering never invents. A reading the provider could not take is `Known:
false` with a cause, and every derived value carries the instant of the
reading behind it, so surfaces show last-known-plus-age rather than a
confident zero.

On-read computation scales because query cardinality follows *authored*
objects, Tiers by components by signals, not collector count. An estate of
22,000 collectors collapses into server-side sums. The cost is console latency
as a function of your backend's query speed.

## Observability cards

Every card surface reads one contract, so the canvas Tier cards and the
observability cards cannot drift apart. The card unit is the Tier, and the
contract is integer-versioned.

The **face** is cheap and fetchable in bulk for a whole shelf (the console's
grid of cards). It carries:

- **Three bands in fixed order**: Delivery, Expectation, Conformance. Each
  band is an *enum state*, plus an optional worst-finding label. The states
  include the neutrals, each distinct: not applicable, unknown, pending
  settle, and stale demoted.
- **Per-signal matrix rows**: volume in and out with the reduction between
  them, freshness, and a shape summary, every reading carrying its timestamp
  and `Known` flag.
- **A population line**: matched count, floor, floor source, and the
  `never_seen` or `under_populated` state.
- **Summary fields**: owning team, Environment, per-band worst severity, and
  finding counts per kind, so a shelf can group and sort without fetching
  anything else.

Colour appears nowhere in the contract. States are the contract, and glyphs
map from states, so colour is an enhancement rather than load-bearing: a
renderer that cannot show colour still shows three distinct readings.

The **drawer** is fetched per card on demand: the findings list with kind,
severity, dampening state, routing target, and mandatory remediation text,
plus "why?" derivations as structured provenance. Every derived value on the
face carries a provenance object that traces it back to the configuration
lines that implied it and the SHA judged against. Every finding tells you what
to do, and every derived value explains itself.
