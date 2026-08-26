---
title: Explore the demo
description: Tour the live read-only console and learn what the demo estate deliberately contains.
order: 8
---

# Explore the demo

<https://demo.telecraft.dev> is a read-only Telecraft console over a curated
synthetic estate. Nothing on the site is hand-written: the cards, drawers,
findings, and rendered artefacts are all output of Telecraft's own evaluators
over the YAML in `telecraft-dev/estate-demo`. Push to that repository and the
demo rebuilds. The demo pipeline is the product pipeline.

The estate is a mid-sized retailer's observability setup, authored to show the
governance states that are interesting rather than the ones that are common.

## What read-only means

There is no server behind the site. The console is built in demo mode and
reads a `demo-snapshot.json` beside it, holding the documents the Telecraft
API would serve. With no server to write to, the site is read-only.

The forms still work, and the refusal tells you where the boundary is. Fill in
a Grant, click **Propose as a pull request**, and you get:

> This is a read-only demo, so proposing this policy change stops here. On a
> real instance the console never writes to the estate. Instead it opens a
> pull request, rendered and attributed to you, and the review decides.
> Everything up to this point (the evaluation, the refusals, and the rendered
> preview) is the same code that runs on a real instance.

The header carries the same message permanently, beside the commit the site
was built from and the moment the estate was evaluated.

## The chrome

Three controls sit above every Workspace:

- **Lens** picks the leading Environment, `production` by default. It sets
  emphasis and evaluation context rather than filtering: surfaces that show
  several Environments keep every row visible.
- **Jump to object** (`⌘K`) is how you reach a specific Tier, Service,
  Blueprint, or Catalogue entry. There is no object-first navigation tree.
- The user chip shows whose team scopes the shelf. The demo signs you in as
  the root Team, so the first view is the whole estate.

## Estate

The landing Workspace has three views: **Shelf**, **Roll-up**, and **Flat
list**.

The Shelf is a grid of card faces, grouped by team subtree and split into
aligned Environment rows, with production first. Each card shows three bands:
Delivery, Expectation, and Conformance. Cards sort worst severity first, from
the face alone.

A banner sits above the grid:

> **4 ungoverned collectors**: 2 served the Unmatched artefact, 2 foreign.
> They don't match any Tier.

Click a card to open its drawer. The `data-flow/gateway` drawer names the
Tier, its owning team, its Environment, its Service Class, and its population
(`6 matched, floor 6 (declared)`), then lists every finding with its
remediation and a link to whoever acts on it:

- `trace-identity: not delivered in production (storefront/catalogue-web)`
- `traces-delivered: broken pipeline in production (storefront/catalogue-web)`
- `metrics-delivered: broken pipeline in production (storefront/search)`,
  marked **waived**
- `pins infosec/pii-redaction@2, but the owning team's head is version 3`

Claims carry a **why?** control that opens their provenance: the file, the
line, and the commit the claim was computed from.

The Roll-up view shows the same verdicts as ratios per team, with waived
counts at every level:

```
Team                   Tiers   Delivery  Expectation  Conformance      All Environments
Engineering            5 / 6   4/5 ✗     3/4 ✗        1/4 ✗ 1 exempt   14 findings, 1 exempt
Platform Engineering   4 / 5   3/4 ✗     2/3 ✗        1/3 ✗ 1 exempt   10 findings, 1 exempt
Data Flow              2 / 3   2/2       1/2 ✗        0/2 ✗ 1 exempt   9 findings, 1 exempt
Edge Operations        2 / 2   1/2 ✗     1/1          1/1              1 finding, 0 exempt
Storefront             1 / 1   1/1       1/1          0/1 ✗            4 findings, 0 exempt
```

Teams with no Tiers read `no verdicts` rather than a misleading 100%.

The Flat list is the collector table: one row per collector, with filters for
Tier, team, and Environment, and an **Ungoverned only** checkbox. Select it to
show the four collectors the banner counts:

```
Collector     Tier                                       Environment  State      Version
legacy-agg-1  ungoverned · served the Unmatched artefact  production   reporting  0.152.0
legacy-agg-2  ungoverned · served the Unmatched artefact  production   reporting  0.152.0
mktg-otel-1   ungoverned · foreign                        production   reporting  0.152.0
mktg-otel-2   ungoverned · foreign                        production   reporting  0.152.0
```

Each row has a checkbox, which starts the claim flow: select the collectors,
and the console suggests a selector generalised over the identity attributes
they share.

## Topology

Two views: **Flow canvas** and **Rollouts**.

The canvas draws the Tier graph in bands: ungoverned arrivals at the top, then
production, then staging. Sources (`internet`, `workloads`) sit beside the
Tiers they feed. Every Hop is drawn once per signal, so a Hop carrying three
signals is three routed edges.

Each Tier node carries its Service Class, its matched count, and the split by
delivery path:

```
gateway      C1   6 matched    6 served · 0 git
kafka-bridge C2   5 matched    5 served · 0 git
edge         C1   24 matched   0 served · 24 git
edge-arm          0 matched    0 served · 0 git
mobile-edge  C2   3 matched    3 served · 0 git
```

`edge` is the git-delivered Tier, and it shows up as legitimately as the
served ones.

A row of buttons above the canvas traces one Service's Paths through the
graph, and **Simulate flow** animates them.

The Rollouts view currently reports:

> No Rollout is active.

The demo estate does carry an active Rollout, `data-flow/bridge-canary`, but
its effect is visible in the estate repository rather than in this view: the
Kafka bridge Tier is dual-bound, and `rendered/data-flow/kafka-bridge@next.yaml`
renders beside the base artefact. The snapshot the demo console reads does not
carry Rollout state. To see how a Rollout behaves, read [stage a
Rollout](stage-a-rollout.md) and run the renders yourself.

## Compose

The Blueprint authoring Workspace. A list on the left holds the estate's five
Blueprints. Picking one opens three surfaces, **Composer**, **Requirement-first**,
and **Node canvas**, plus a **YAML** toggle.

The Composer shows the palette and the lanes side by side. The palette is what
this team can use, and it says why each entry is offered:

- `data-flow/gateway-exporter` and `infosec/pii-redaction` appear as shared
  Components, named by their owning team.
- `kafka` under exporters is annotated `via Grant kafka-egress-for-data-flow
  (platform → data-flow)`.
- Entries below the floor carry the reason inline, for example `alpha on
  profiles: below this Service's C1 floor in production (beta)`. They are
  greyed, not hidden, so you can see the cost of the choice.
- A line at the foot reads `258 components hidden by your allow-list`.

The lanes show the pipeline in order. Each Component links to its Catalogue
entry and is tagged with its stability for that signal, under a header naming
the floor in force (`floor beta (C1 · production)`). Above them, the
`satisfies` claims link to the verdict on the Requirement-first surface.

A **Save: propose v5 as a pull request** button sits at the top, with the line
`You can edit this Blueprint: data-flow is one of your teams.` A Blueprint
another team owns is read-only, and the same line says so.

## Catalogue & Governance

Three views: **Browse**, **Effective palette**, and **Governance**.

Browse is the Catalogue: 268 entries in the active `v0.158.0` artefact, with
`v0.156.0` kept beside it in the selector. Installed Catalogues are kept, never
replaced, because a collector is judged against the Catalogue for the version
it runs. Filters narrow by stability and by signal, and each row shows
per-signal stability rather than one overall rating. Below the table, a
**Governed Components** section lists the estate's configured instances at
their pinned versions.

Effective palette is `telecraft palette` as a page: pick a team and see every
entry it can use, with its origin (`default-allow`, `allow-list`, or a named
Grant) and a **why?** control.

Governance is where you edit Allow-lists and Grants. Each declared list is a
text area of `class/type-pattern` shapes, with a note that an emptied list is
refused: to inherit unchanged, a team declares no list at all. Below that sit
the existing Grant and a form for a new one, gated on the rule that a Grant's
owner's team must sit above its target.

## What the estate deliberately contains

Every state in this table is authored on purpose. It is the list worth
checking your own estate against.

| State | Where to look |
|---|---|
| A healthy Tier, all three bands green | `edge-ops/edge`, 24 node agents at their declared floor |
| A requirement violation | `storefront/catalogue-web` stopped delivering traces: configured and not working |
| A waived finding | `storefront/search` misses `metrics-delivered` under an authored Exemption. The count is waived, the diagnosis is not |
| `library_drift` | `data-flow/gateway-standard` pins `infosec/pii-redaction@2` while its owner is at v3 |
| A stability-floor breach | `storefront/mobile-collector` routes metrics through a processor upstream rates alpha, below the C2 production floor |
| Ungoverned collectors | Four collectors match no Tier selector: two served the Unmatched artefact, two read through the estate provider |
| A never-seen Tier | `edge-ops/edge-arm` was authored ahead of a migration and nothing has ever matched it |
| A silent component | The Kafka bridge's batch processor emits no self-telemetry past the Settle window |
| Delivery divergence | One staging collector reports an artefact other than the one git holds |
| An active Rollout | `data-flow/bridge-canary` is mid-stage across the Kafka bridge |

Two drawers repay a visit. `edge-ops/edge-arm` shows a Tier with a floor and
no population:

> no collector matches this Tier's selector
>
> Check the Tier's selector against what the collectors actually report, or
> delete the Tier if the workload it was authored for never arrived.

Its population line has a **why?** control, and opening it gives the whole
chain:

> population floor 4, declared on the Tier as min_expected: a minimum, not an
> exact count
>
> `teams/edge-ops/tiers/edge-arm.yaml:12` &nbsp; `min_expected: 4`
>
> judged at `870c9b8a26458402c1982359bcdea90fdb7ef73d`

`data-flow/gateway-staging` shows all three bands finding at once:

```
[conformance/violation] pins infosec/pii-redaction@2, but the owning team's head is version 3
[expectation/advisory]  unbacked arrival claim on the logs lane for checkout/payments
[delivery/advisory]     1 of 2 collectors report an artefact other than head
```

Three different questions, three different owners, on one card.

## Flow is declared, and shape still is not

Every card's flow table carries figures: what each signal lane accepted, what
it sent, the difference between them, and how long ago the counters were
read. `storefront/mobile-edge` sheds four fifths of the metrics it accepts
through a filter. The table reports that as a reduction, never as a loss,
because a filter dropping most of what it sees is doing the job it was
authored to do.

A repository can't hold a running collector estate or the telemetry that
arrived from it. The demo doesn't invent either: it *declares* them, and
`telecraft snapshot` plays them back through the same seams a live instance
reads (`demo/readings.yaml` in the estate repository). Everything judged from
them is judged by the product's own evaluators.

Three things are still rendered as something other than a figure, and all
three are worth a look, because they are the point:

- **`edge-ops/edge-arm` reads unknown on every lane.** Nothing has ever
  matched that Tier, so no counters exist to read. A zero would be a claim
  about a Tier Telecraft can't see, and the contract keeps "we can't see" and
  "nothing arrived" apart.
- **Some lanes read "no lane on this Tier".** `data-flow/kafka-bridge` wires
  no metrics pipeline and `storefront/mobile-edge` wires no traces pipeline,
  so there is nothing on those lanes to meter. Their counters would truthfully
  read `in 0 / out 0`, which is identical to `storefront/catalogue-web`'s
  traces lane: a lane that is wired, broken, and a genuine finding. Two
  opposite meanings can't share one rendering, so a lane with no pipeline
  behind it carries no numbers at all.
- **Shape reads unknown on every card**, and the drawer says why:

  > no shape reading exists at pipeline grain: self-telemetry counts items,
  > not what is inside them, and service-grain conformance is never blended
  > into pipeline grain

  Self-telemetry counts items passing through a pipeline; it doesn't open
  them. The service-grain reading that *would* answer the question is a
  different grain, and metering never blends the two.

## Run the same estate locally

Everything the site shows is reproducible from the two public repositories:

```sh
git clone https://github.com/telecraft-dev/telecraft.git
git clone https://github.com/telecraft-dev/estate-demo.git
cd telecraft
go build -o telecraft ./cmd/telecraft
```

The [quickstart](quickstart.md) takes it from there.
