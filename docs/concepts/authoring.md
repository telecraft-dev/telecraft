---
title: Authoring
description: Services, Tiers, Blueprints and Components, the Catalogue they draw from, and the allow-list policy that decides what each team may use.
order: 4
---

# Authoring

Authoring is the second rung: teams compose collector configuration from
owned, versioned building blocks, and the renderer compiles the result to
plain otelcol YAML in git. Nothing in your delivery path has to change to
adopt it, because the output is an ordinary file that whatever applies your
configuration today can keep applying.

## The objects

**Service** is the governed unit, identified by `service.name`. It carries a
[Service Class](environments.md#service-class-and-sensitivity), an owner, and
the Requirements it is judged against. One Service has one identity and one
owner across many Environments.

**Tier** is a position in the collection topology: edge, gateway, or any layer
your design needs. It is an authored, ownable object carrying the policy for
everything at that position, and it is the sole rendering and binding unit:
one rendered artefact per Tier, and one Blueprint version bound per Tier.
A collector is matched into a Tier by selector and inherits its policy and
owner. When one subset of a Tier's collectors needs something different, you
split the Tier.

**Hop** is the directed edge between two Tiers, or between a Tier and a
destination. It is first-class and ownable, because trust is a property of the
edge rather than the node: one gateway commonly receives both trusted and
untrusted traffic, and the renderer generates attribute stripping across an
untrusted Hop automatically.

**Path** is one Service's route through the Tier graph. Several Paths per
Service is normal, not an anomaly: browser traffic arriving at a gateway
on-ramp and pod logs arriving through an edge daemonset are two Paths for one
Service. A Path is what generates the delivery expectation, so it is also how
the [Expectation engine](pipeline-observability.md#claims) knows which signals
to look for.

A workload that runs no collector at all is representable: a Path straight to
a gateway Tier.

## Blueprints

A **Blueprint** is a named, integer-versioned composition of Components. It is
a domain-shaped document in its own schema, never annotated collector
configuration: otelcol YAML has no reference mechanism, so annotating it would
force copy semantics into a file that looks standard.

The document serialises per-signal lanes under the upstream signal names
(`traces`, `metrics`, `logs`, `profiles`), each an explicitly ordered list of
Component references, plus one collector-wide `extensions` block. The renderer
never re-sorts a lane: what you see is what renders.

A Blueprint may route `profiles`, but a Requirement cannot assert on it: the
signal is alpha upstream, and a requirement written against it could not be
judged honestly. Logs, metrics, and traces are what conformance covers.

There is no phase concept. With one Blueprint bound per Tier, there is no
multi-blueprint stacking for phases to arbitrate. Ordering wisdom, such as
putting `memory_limiter` first and `batch` last, ships as evaluator rules
keyed on catalogue types and raises ordering findings, which advise rather
than reorder.

A Blueprint also carries the Requirement ids it claims to satisfy. That claim
is intent, never fact, and the surfaces never blur the two: a `satisfies` list
says what the author meant, and the verdict says what is true.

## Components

A **Component** is a configured instance of a catalogue type: a receiver,
processor, exporter, connector, or extension, named, integer-versioned, and
ownable. Every lane entry is a Component. Raw inline otelcol blocks are not a
second kind of lane entry, because they would be invisible to ownership,
routing, and the evaluator.

Components have two residences:

- **Shared**: a standalone file with an explicit owner and the team-qualified
  id `<team>/<name>`, for example `infosec/pii-redaction`. The id is the
  reference, never the path, so the repository layout is not frozen into every
  consumer file.
- **Local**: declared inline in a Blueprint, implicitly owned by the
  Blueprint's owner, and not referenceable from outside it.

Promotion from local to shared is a move to a file, which forces the ownership
conversation at the moment it becomes real. In the rendered YAML, instances
become standard otelcol ids that carry provenance: a shared Component renders
as `type/team.name`, a local one as `type/name`. A collision between rendered
ids is a mechanical render error.

**Inheritance is by reference, never by copy.** The model has no way to embed
another team's configuration, only to reference it.

### Pins and tracking

A shared Component reference pins a version by default, written
`infosec/pii-redaction@3`. Opt in per reference with `track: head` to follow
the owning team's latest instead.

A pinned reference sitting behind the owner's head raises a `library_drift`
finding carrying the Component facet: "you reference version 3, the world is
at 5". It is routed to the consuming Blueprint's owner and it never blocks.
The remedy is a change proposal bumping the pin, because there is no live
mutation anywhere in the model.

The accepted consequence, stated plainly: a team shipping a critical fix
cannot force propagation. The pressure is findings plus the organisational
conversation.

Versions are explicit monotonic integers on Components and Blueprints alike,
bumped by the owner in the same change as the edit. Changing content without
bumping the version is a mechanical render refusal, in the same category as
invalid YAML rather than in the category of policy.

`library_drift` has a second facet for the same shape one level up: a
Requirement whose bar moved, or a Service Class floor raised over standing
configuration. Both mean "you meet the version you claim and fail the current
one". See the [outcome cross](readings-and-verdicts.md#the-outcome-cross).

## The Catalogue

The **Catalogue** is the versioned inventory of otelcol component types:
identity, per-signal stability, and lifecycle, keyed by `(class, type)`.

It is machine-generated from the `metadata.yaml` files of the upstream
collector distributions at a pinned release tag. Hand-curation of the
component list is prohibited, because that maintenance burden is what kills
configuration libraries. Nothing anywhere in Telecraft holds an authored list
of components.

Three details matter when you use it:

- **The key is `(class, type)`, not `type`.** The same type string is reused
  across classes: `kafka` is both a receiver and an exporter. A
  `deprecated_type` alias resolves on every lookup, so an entry written
  against a historical name keeps selecting the component it always did.
- **Stability is per signal.** One component can be beta for logs and alpha
  for another signal, so a [floor](environments.md#stability-floors) is judged
  per component and signal, never per component.
- **Catalogue versions are atomic and retained.** Importing the same tag twice
  yields byte-identical artefacts; a new tag yields a new artefact beside the
  old one rather than replacing it. That matters because the platform does not
  control collector binaries, so the version a collector runs is a discovered
  fact.

The Catalogue states what exists. It never states what may be used.

## Allow-lists and Grants

The **Allow-list** is the subset of the Catalogue a Team may use, keyed on the
same `(class, type)` unit.

Entries are shapes rather than literals, authored as `class/type-pattern`. The
class side is exact; the type side takes `*` and `?`. So `receiver/otlp` names
one component, `exporter/kafka*` names a family, and `processor/*` names a
class. The pattern vocabulary stops there deliberately: what loads, matches.

**Inheritance narrows only.** A Team's effective list is its parent's effective
list intersected with its own declared list. Descendants can subtract and
nothing else. Widening is an organisational conversation with the ancestor who
owns the wider list, which is the point.

A **Grant** is that conversation's outcome as an object: an
ancestor-authored, owned exception adding named Catalogue entries to a
descendant Team's effective list. It applies to the target Team's subtree and
can be narrowed back out below like anything else. Its authority is its
owner's team, which must be a proper ancestor of the target.

Everything a team may use therefore traces either to the root list surviving
intersection or to a named Grant, so "may team T use component X, and on whose
authority?" is answerable entirely from git history.

**The default posture is allow.** Absent any authored list, the effective list
is the whole active Catalogue. Governance pressure comes from floors,
lifecycle findings, and conformance, not from an empty shop on day one.
Deprecated components stay in the default allow and produce findings, because
a newly imported catalogue must never break a re-render.

Allow-lists and Grants are authored files in the estate repository beside
`teams.yaml`, reviewed and versioned like everything else. They are never rows
edited live in a database.

### The Palette

The **Palette** is what the composer offers one user: the active Catalogue
intersected with their team's effective Allow-list, judged live by the shared
evaluator. Allowed components are shown. Components breaching the floor for
the current context are visible but greyed, with the reason, because greying
teaches the policy. Non-allowed components are hidden, because listing banned
entries is noise.

The Palette is presentation. It enforces nothing: enforcement happens at
render, and only [one rule](governance.md#enforcement-points) hard-blocks
there.

## From authored objects to artefacts

Rendering is a deterministic function of the authored trees. Identical inputs
produce byte-identical artefacts, which is what lets CI recompute the
`rendered/` tree and fail on any mismatch. Every artefact is stamped with the
commit SHA so it carries its own identity, and each Tier's artefact lands at a
stable path. See [delivery](delivery.md) for what happens to it next, and the
[reference](../reference/index.md) for the file schemas.

Reference: [ADR-0020](../adr/0020-catalogue-sourcing-versioning.md),
[ADR-0021](../adr/0021-allowlist-policy.md),
[ADR-0024](../adr/0024-blueprint-schema.md),
[ADR-0025](../adr/0025-tier-rendering-unit.md),
[ADR-0026](../adr/0026-pinned-references-library-drift.md).
