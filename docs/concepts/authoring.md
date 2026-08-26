---
title: Authoring
description: Services, Tiers, Blueprints and Components, the Catalogue they draw from, and the Allow-list policy that decides what each team may use.
order: 4
---

# Authoring

Authoring is the second rung. Teams compose collector configuration from
owned, versioned building blocks, and the renderer compiles the result to
plain otelcol YAML in git. Nothing in your delivery path has to change,
because the output is an ordinary file that whatever applies your
configuration today can keep applying.

## The objects

**Service** is the governed unit, identified by `service.name`. It carries a
[Service Class](environments.md#service-class-and-sensitivity) (how much it
matters), an owner, and the Requirements it is judged against. One Service
has one identity and one owner across many Environments.

**Tier** is a position in the collection topology: edge, gateway, or any
layer your design needs. It is an authored, ownable object that carries the
policy for everything at that position. It is also the only rendering and
binding unit: one rendered artefact per Tier, and one Blueprint version bound
per Tier. A collector is matched into a Tier by selector and inherits the
Tier's policy and owner. When one subset of a Tier's collectors needs
something different, split the Tier.

**Hop** is the directed edge between two Tiers, or between a Tier and a
destination. It is a first-class, ownable object because trust belongs to the
edge, not the node: one gateway commonly receives both trusted and untrusted
traffic, and the renderer generates attribute stripping across an untrusted
Hop automatically.

**Path** is one Service's route through the Tier graph. Several Paths per
Service is normal: browser traffic arriving at a gateway on-ramp and pod logs
arriving through an edge daemonset are two Paths for one Service. A Path
generates the delivery expectation, so it is also how the
[Expectation engine](pipeline-observability.md#claims) knows which signals to
look for.

A workload that runs no collector at all is still representable: a Path
straight to a gateway Tier.

## Blueprints

A **Blueprint** is a named, integer-versioned composition of Components. It is
a document in its own schema, never annotated collector configuration.

The document holds per-signal lanes under the upstream signal names
(`traces`, `metrics`, `logs`, `profiles`), each an explicitly ordered list of
Component references, plus one collector-wide `extensions` block. The renderer
never re-sorts a lane: what you see is what renders.

A Blueprint can route `profiles`, but a Requirement cannot assert on it. The
signal is alpha upstream, so a requirement written against it could not be
judged reliably. Conformance covers logs, metrics, and traces.

There is no phase concept. Ordering advice, such as
putting `memory_limiter` first and `batch` last, ships as evaluator rules
keyed on Catalogue types. Those rules raise ordering findings that advise;
they never reorder.

A Blueprint also carries the Requirement ids it claims to satisfy. That claim
is intent, never fact, and the two never blur: the `satisfies` list says what
the author meant, and the verdict says what is true.

## Components

A **Component** is a configured instance of a Catalogue type: a receiver,
processor, exporter, connector, or extension. It is named, integer-versioned,
and ownable. Every lane entry is a Component. Raw inline otelcol blocks are
not a second kind of lane entry.

A Component lives in one of two places:

- **Shared**: a standalone file with an explicit owner and the team-qualified
  id `<team>/<name>`, for example `infosec/pii-redaction`. The id is the
  reference, never the path, so the repository layout is not frozen into
  every file that uses it.
- **Local**: declared inline in a Blueprint, owned by the Blueprint's owner,
  and not referenceable from outside it.

Promoting a local Component to shared means moving it to a file, which raises
the ownership question at the moment it becomes real. In the rendered YAML,
instances become standard otelcol ids that carry their origin: a shared
Component renders as `type/team.name`, a local one as `type/name`. A
collision between rendered ids is a mechanical render error.

**Inheritance is by reference, never by copy.** The model has no way to embed
another team's configuration, only to reference it.

### Pins and tracking

A shared Component reference pins a version by default, written
`infosec/pii-redaction@3`. To follow the owning team's latest version
instead, opt in per reference with `track: head`.

A pinned reference behind the owner's head raises a `library_drift` finding
with the Component facet: "you reference version 3, the world is at 5". It
routes to the consuming Blueprint's owner and it never blocks. The remedy is
a change proposal that bumps the pin, because nothing in the model mutates
live.

One consequence follows: a team shipping a critical fix cannot force it to
propagate. The pressure is findings, plus the conversation between teams.

Versions are explicit, increasing integers on Components and Blueprints
alike, bumped by the owner in the same change as the edit. Changing content
without bumping the version is a mechanical render refusal, in the same
category as invalid YAML rather than policy.

`library_drift` has a second facet for the same shape one level up: a
Requirement whose bar moved, or a Service Class floor raised over standing
configuration. Both mean "you meet the version you claim and fail the current
one". See the [outcome cross](readings-and-verdicts.md#the-outcome-cross).

## The Catalogue

The **Catalogue** is the versioned inventory of otelcol component types:
identity, per-signal stability, and lifecycle, keyed by `(class, type)`.

Telecraft generates it from the `metadata.yaml` files of the upstream
collector distributions at a pinned release tag. Nobody curates the component
list by hand, and nothing anywhere in Telecraft holds an authored list of
components.

Three details matter when you use it:

- **The key is `(class, type)`, not `type`.** The same type string is reused
  across classes: `kafka` is both a receiver and an exporter. A
  `deprecated_type` alias resolves on every lookup, so an entry written
  against a historical name keeps selecting the component it always did.
- **Stability is per signal.** One component can be beta for logs and alpha
  for another signal, so a [floor](environments.md#stability-floors) is judged
  per component and signal, never per component.
- **Catalogue versions are atomic and retained.** Importing the same tag twice
  yields byte-identical artefacts. A new tag yields a new artefact beside the
  old one rather than replacing it. Telecraft does not control collector
  binaries, so the version a collector runs is a discovered fact, and the
  matching Catalogue has to still be there.

The Catalogue states what exists. It never states what you can use.

## Allow-lists and Grants

The **Allow-list** is the subset of the Catalogue a Team can use, keyed on the
same `(class, type)` unit.

Entries are patterns rather than literals, written as `class/type-pattern`.
The class side is exact; the type side takes `*` and `?`. So `receiver/otlp`
names one component, `exporter/kafka*` names a family, and `processor/*`
names a class. The pattern vocabulary stops there: what loads, matches.

**Inheritance narrows only.** A Team's effective list is its parent's
effective list intersected with its own declared list. Descendants can
subtract and nothing else. To widen, talk to the ancestor who owns the wider
list.

A **Grant** is the outcome of that conversation as an object: an exception,
authored and owned by an ancestor, that adds named Catalogue entries to a
descendant Team's effective list. It applies to the target Team's subtree and
can be narrowed back out below, like anything else. Its authority is its
owner's team, which must be a proper ancestor of the target.

Everything a team can use therefore traces either to the root list surviving
intersection or to a named Grant, so git history alone answers "can team T
use component X, and on whose authority?".

**The default is allow.** With no authored list, the effective list is the
whole active Catalogue. Governance pressure comes from floors, lifecycle
findings, and conformance, not from an empty shop on day one. Deprecated
components stay in the default allow and produce findings, so a newly
imported Catalogue never breaks a re-render.

Allow-lists and Grants are authored files in the estate repository beside
`teams.yaml`, reviewed and versioned like everything else. They are never
rows edited live in a database.

### The Palette

The **Palette** is what the composer (the console's Blueprint editor) offers
one user: the active Catalogue intersected with their team's effective
Allow-list, judged live by the shared evaluator. Allowed components are
shown. Components that breach the floor for the current context are visible
but greyed, with the reason, because seeing the reason teaches the policy.
Non-allowed components are hidden, because a list of banned entries is noise.

The Palette is presentation. It enforces nothing: enforcement happens at
render, and only [one rule](governance.md#enforcement-points) hard-blocks
there.

## From authored objects to artefacts

Rendering is a deterministic function of the authored trees. Identical inputs
produce byte-identical artefacts, which lets CI recompute the `rendered/`
tree and fail on any mismatch. Every artefact is stamped with the commit SHA
so it carries its own identity, and each Tier's artefact lands at a stable
path. See [delivery](delivery.md) for what happens to it next, and the
[reference](../reference/index.md) for the file schemas.
