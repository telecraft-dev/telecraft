---
title: Tier file format
description: Every field of a Tier, Service and Rollout, with selectors, Blueprint binding, population floors and dual binding.
order: 5
---

# Tier file format

A Tier is one authored topology position and the rendering unit: one rendered
artefact per Tier. It declares exactly one Environment and binds exactly one
Blueprint version.

Tiers live at `teams/<team>/tiers/<name>.yaml`. Services and Rollouts load
from the same tree and are documented here, because a Service's Paths decide a
Tier's judgement strictness and a Rollout dual-binds a Tier. See
[Estate layout](estate-layout.md) for the rules every authored file obeys.

The decisions behind the format are ADR-0025 (the Tier as rendering and
binding unit), ADR-0035 (population floors) and ADR-0029 (staged rollouts).

## Tier fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | no | the filename | Must equal the filename without its extension when present. |
| `owner` | string | yes | none | The accountable party. |
| `environment` | string | yes | none | The one Environment this Tier declares. |
| `blueprint` | string | yes | none | The Blueprint binding, `<team>/<name>@<version>`. |
| `selector` | map of string to string | no | empty | Equality selector over reported identifying attributes. |
| `min_expected` | integer | no | `0` | The declared population floor. Zero means no declared floor. |
| `serving` | mapping | no | absent | Present marks the Tier served over OpAMP. Holds one field, `endpoint`. |
| `hops` | list of mappings | no | empty | Directed edges arriving at this Tier. Each has `from` and `trusted`. |

The Tier's team-qualified id is `<team>/<name>`, derived from the file's place
in the layout.

```yaml
# teams/data-flow/tiers/gateway.yaml
owner: gateway-owners
environment: production
blueprint: data-flow/gateway-standard@4
selector:
  telecraft.tier: gateway
  deployment.environment: production
min_expected: 2
serving:
  endpoint: wss://opamp.telecraft.internal/v1/opamp
hops:
  - from: internet
  - from: data-flow/edge
    trusted: true
```

## Environment

`environment` is a single adopter-defined value aligned to
`deployment.environment.name`. It's an attribute of the infrastructure, so a
Tier declares one and only one. Per-Environment binding is realised through
sibling Tiers: author `gateway.yaml` and `gateway-staging.yaml`, each with its
own Environment and its own binding.

`production` is the distinguished value that policy defaults attach to: it's
the default Environment lens, it leads every report, and it's the Environment
the shipped stability floors are defined for.

A Tier with no `environment` is a load error.

## Blueprint binding

`blueprint` is the string `<team>/<name>@<version>`. A binding always pins:
the version after `@` is a positive integer, and there's no track-head mode,
so rebinding is an authored, reviewed change.

| Problem | Result |
|---|---|
| No `@` and version | Load error. |
| A version that isn't a positive integer | Load error. |
| Not of the form `<team>/<name>` | Load error. |
| Pinned to a version other than the one at head | `binding` finding at render. |

The estate tree holds head content, so head is what renders. A pin off head is
the visible drift, reported as a `binding` finding, never a block.

## Selector

`selector` is equality over every authored pair, matched against the
identifying attributes a collector reports. A collector is never authored: it
connects, reports its attributes, and is matched into the Tier whose selector
its attributes satisfy. The most specific satisfied selector wins.

The selector doubles as the Tier's expectation: it says what shape should
match, never how many. A collector matching no Tier selector is served the
Unmatched artefact.

| Problem | Result |
|---|---|
| A pair with an empty key or value | Load error: an empty side can never match. |
| `serving` declared with no selector | Load error: every collector of the Tier would land on the Unmatched artefact. |
| `min_expected` above zero with no selector | Load error: the floor counts collectors matched by selector. |

## Population floors

`min_expected` is the Tier's declared population floor: at least this many
collectors should match its selector. It's reviewable in git, which is what
substrates with no queryable inventory need.

- It's a floor, never an equality. Surplus is never a finding.
- Zero, the default, means no declared floor.
- A negative value is a load error.
- A live count derived from the substrate always outranks the declaration:
  derived beats declared beats absent.

The findings a floor gives teeth to are `never_seen`, when the selector has
matched no collector in any reading, and `under_populated`, when collectors
matched but fewer than the floor. Both attach to the Tier and route to the
Tier's owner.

## Serving

The presence of a `serving` block marks the Tier served over OpAMP and makes
the renderer emit `rendered/<team>/<tier>.supervisor.yaml` beside the
collector artefact.

| Field | Type | Required | Description |
|---|---|---|---|
| `endpoint` | string | yes | The OpAMP server endpoint the Supervisor connects to. |

A `serving` block with no `endpoint` is a load error. A Tier with no `serving`
block is git-delivered, which is legitimate and not lesser.

## Hops

Each entry of `hops` is a directed edge arriving at this Tier.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `from` | string | yes | none | The source of the edge: another Tier's id, or a name for the world outside the graph. |
| `trusted` | boolean | no | `false` | Whether data arriving over this edge is trusted. |

Trust is a property of the Hop, never of the Tier: one gateway receives both
trusted and untrusted traffic. An undeclared `trusted` fails safe to
untrusted.

One Tier renders one artefact for all its collectors, so intake can't be split
per Hop at render: any untrusted arrival makes the whole intake untrusted. The
renderer then emits a processor that strips the platform's attribute namespace
from arriving data, so identity is re-derived from the receiving Tier's own
config stamps rather than from inbound data.

A Hop with no `from` is a load error.

## Service fields

A Service is the governed unit. Its Paths decide which Tiers its Service Class
judges, so it loads with the topology.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | no | the filename | Must equal the filename without its extension when present. |
| `owner` | string | yes | none | The accountable party. |
| `class` | string | yes | none | The Service Class, such as `C1`. |
| `paths` | list of mappings | no | empty | Each has a `through` list of team-qualified Tier ids. |

```yaml
# teams/product/services/checkout.yaml
owner: checkout-team
class: C1
paths:
  - through: [data-flow/edge, data-flow/gateway]
  - through: [data-flow/gateway-staging]
```

A Path through a Tier nobody authored is a load error, not a finding: a
silently dropped Path would relax the traversed Tier's floor judgement, and
under-governed is the failure mode. A Path through no Tiers at all is also a
load error.

### How Paths set a Tier's floor

Stability floors are judged at render, per (component, signal actually
routed), at the Tier's declared Environment crossed with the strictest Service
Class among Services whose Paths traverse that Tier. Adding a C1 Path is what
tightens both traversed Tiers to the C1 floor; strictness is derived from
traversal, never hand-maintained.

The floors that ship: in `production`, C1 and C2 require beta or better and C3
requires alpha or better. Environments absent from the table carry no floor at
all, which is where alpha and development components are meant to be
exercised. The maturity ladder is `development` < `alpha` < `beta` <
`stable`; `deprecated` and `unmaintained` are lifecycle end-states rather than
rungs, and are judged apart from floors.

A breach is a `floor` finding routed to an owner, never a block.

## Rollout fields

A Rollout is the opt-in staging instrument: an authored, owned object at
`teams/<team>/rollouts/<name>.yaml` targeting exactly one Tier. The default
remains the flat rebind, so a Rollout is never mandatory.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | no | the filename | Must equal the filename without its extension when present. |
| `owner` | string | yes | none | Must be the target Tier's owner. |
| `tier` | string | yes | none | Team-qualified id of the one Tier this Rollout stages. |
| `from` | string | yes | none | The current binding, `<team>/<name>@<version>`. Must equal the Tier's own binding. |
| `to` | string | yes | none | The candidate binding. Must name a different Blueprint from `from`. |
| `stage` | integer | no | `0` | Zero-based index of the active stage. |
| `hash_attributes` | list of strings | when a stage uses `percent` | empty | The identifying attributes fractional membership hashes over, in authored order. |
| `stages` | list of stages | yes | none | The ordered stage list. At least one. |

Each stage has:

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `cohort` | mapping | yes | none | The cohort spec. At least one form must be present. |
| `soak` | duration string | no | `0s` | Minimum time the stage must have been active before its advance can be proposed. Zero means no soak gate. |

A cohort spec has three mixable forms; membership is their union, so "the
three boxes I trust plus 5%" is one stage:

| Field | Type | Description |
|---|---|---|
| `hosts` | mapping | Enumerated identifying-attribute values. Holds `attribute` and `values`, both required when `hosts` is present. |
| `match` | map of string to string | An equality selector over reported identifying attributes, with the Tier selector's semantics. |
| `percent` | integer | A fraction of the population, 1 to 100, via a stable hash over `hash_attributes`. Statistically that share, never exactly it. |

```yaml
# teams/data-flow/rollouts/gateway-v5.yaml
owner: gateway-owners
tier: data-flow/gateway
from: data-flow/gateway-standard@4
to: data-flow/gateway-next@1
stage: 0
hash_attributes: [host.name]
stages:
  - cohort:
      hosts:
        attribute: host.name
        values: [gw-01, gw-02, gw-03]
    soak: 24h
  - cohort:
      percent: 25
    soak: 24h
  - cohort:
      percent: 100
```

### Dual binding

While a Rollout is active the target Tier is dual-bound. Both artefacts render
at head: the base artefact from `from` at `rendered/<team>/<tier>.yaml`, and
the candidate at `rendered/<team>/<tier>@next.yaml` from `to`. The `@next`
artefact exists exactly when the Rollout does, and is retired with it.

The candidate is judged like any bound Blueprint: floors, the allow-list hard
block and the stale-pin finding all apply, because that config is about to run
in this Tier.

Every step is a commit on this one small file. Starting a rollout is adding
it, advancing is bumping `stage`, completing is flipping the Tier to `to` and
deleting the file, aborting is deleting the file alone.

### Rollout load errors

Beyond the field rules above, the topology load refuses when:

- The Rollout targets a Tier nobody authored, or a Tier of another team.
- Its `owner` differs from the target Tier's owner.
- Two Rollouts target the same Tier: one active Rollout per Tier.
- `from` doesn't equal the Tier's authored binding. While a Rollout is active
  the Rollout file is the only door, so a direct rebind of the Tier fails
  render validation.
- `from` and `to` name the same Blueprint. The estate tree holds one content
  per Blueprint id at head, so both artefacts would render identically and the
  rollout would stage nothing. Author the candidate as a sibling Blueprint.
- `stage` is negative or not an index into `stages`. Completion deletes the
  file rather than counting past the end.
- A stage's cohort spec is empty, its `hosts` form lacks an attribute or
  values, its `match` form has an empty key or value, or its `percent` is
  outside 1 to 100.
- A stage uses `percent` but the Rollout pins no `hash_attributes`, or
  `hash_attributes` holds an empty or duplicated entry.

## Topology load errors

The topology load fails closed on every problem, structural or cross-object,
and returns nothing. Beyond the per-object rules above:

- An unknown or misspelled field anywhere in the document.
- A malformed document, an empty file, a top-level list, or more than one YAML
  document in the file.
- A duplicate object id across the source roots.
- A source root with no `teams/` tree.
- A set of roots holding no Tiers at all: the Tier is the rendering unit, so
  there'd be nothing to render.
