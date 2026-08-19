---
title: Concepts
description: What Telecraft is, the problem it solves, the three rungs you can adopt separately, and the principles that hold across all of them.
order: 1
---

# Concepts

Telecraft models your OpenTelemetry collection topology, composes collector
configurations from owned, versioned building blocks, and checks whether the
telemetry those configurations promise actually arrives.

This section explains the model and the reasoning behind it. For step-by-step
tasks, see the [guides](../guides/index.md); for flags, file schemas, and API
shapes, see the [reference](../reference/index.md). Every term with a capital
letter here is defined in the [glossary](../glossary.md), which is the
authoritative vocabulary.

## The problem

An OpenTelemetry estate fails quietly. A receiver gets wired into the wrong
pipeline, a gateway drops an attribute, a team ships a service with no
instrumentation, and nothing announces any of it. The dashboard that should
have shown the gap is the dashboard nobody built, because nobody knew the gap
existed.

Tools that read collector configuration can tell you what was asked for. Tools
that read a telemetry backend can tell you what turned up. Neither can tell
you which of those two facts is the problem, so neither can tell you whose
problem it is.

Telecraft reads the estate three ways and crosses the readings. A configured
pipeline delivering nothing is a defect for the platform team. An
unconfigured Service delivering nothing is a governance gap for the workload
owner. Both score zero; they need different people and different fixes.
Collapsing them makes a tool honest and useless at the same time.

That is the whole idea: green means "the config worked", never merely "the
config applied". See [readings and verdicts](readings-and-verdicts.md) for how
the crossing works, and [pipeline observability](pipeline-observability.md)
for how the platform derives what "worked" should mean.

## Three rungs

Telecraft is three products that happen to share a model. You can adopt any
one of them without the others, and adopting a higher rung is never a
condition of using a lower one.

| Rung | What it does | What it costs you |
|---|---|---|
| **Conformance** | Reads your telemetry backend and your collectors' reported configs, judges every Service against the Requirements that apply to it, and routes each finding to an owner | A connection string |
| **Authoring** | Composes Blueprints from governed Components and renders plain otelcol YAML into git as change proposals | Nothing in your delivery path changes |
| **Serving** | A stateless OpAMP server that delivers rendered config from git to collectors, with git delivery as a co-equal alternative chosen per collector | An OpAMP Supervisor beside each served collector |

Conformance needs no Blueprints and no rendering: point it at your
requirements library, a reading of your collectors, and a telemetry backend,
and it produces verdicts. Authoring renders artefacts into git and stops
there, so whatever applies your configuration today keeps applying it.
Serving replaces that delivery step for the collectors you choose, one at a
time.

## Principles

These five hold everywhere in the design. Where a feature would break one,
the feature loses.

**Nothing sits in the telemetry path.** If Telecraft is down, no telemetry
stops flowing. The platform is never a collector, a gateway, or a hop. Every
governance mechanism that looks like it needs to be inline (schema checking at
collection time, quarantining unrecognised sources) ships instead as a
*rendered pattern*: configuration the renderer emits into your own collectors,
plus a reading the platform takes afterwards. A pattern that dies costs you
findings, never data.

**Git is the source of truth.** History, rollback, approval, and the audit
trail already exist in git, so none of them are built. The console opens
change proposals; it never writes to a cluster. A configuration you committed
by hand is legitimate, not drift: Intended is whatever git says at that
commit. Deleting the platform loses delivery, never the record.

**Configurations, never binaries.** No collector distribution, no container
image, no chart. The renderer exports one artefact per Tier, plain otelcol
YAML at a stable path, plus a small supervisor config where a Tier is served.
The renderer never knows what applies the result.

**The core is neutral.** Interface names are domain terms; implementations
name the vendor's product, never the company. A vendor word inside the core is
a lint failure, not a style preference, so backend independence stays
greppable rather than aspirational.

**Air gaps come first.** Nothing depends on a SaaS or on a particular git
host. The component Catalogue is built once and carried, so an air-gapped
instance runs the same import pipeline minus the convenience of downloading.
Authentication is a seam, so identity never has to be delegated to a forge
that is not there.

## Where to go next

- [Readings and verdicts](readings-and-verdicts.md): Intended, Effective, and
  Observed; the outcome cross and what each outcome diagnoses; delivery
  status.
- [Ownership](ownership.md): who is accountable for what, how findings route,
  and how compliance rolls up a team tree without a single blended number.
- [Authoring](authoring.md): Services, Tiers, Blueprints, Components, the
  Catalogue, and the allow-list policy that decides what a team may use.
- [Environments](environments.md): the Environment axis, Service Class,
  Sensitivity, stability floors, and why every verdict is per environment.
- [Delivery](delivery.md): served and foreign collectors, the stateless OpAMP
  server, the commit stamp, and staged rollouts.
- [Pipeline observability](pipeline-observability.md): Expectations and
  Claims, self-telemetry, metering, and the card contract.
- [Governance](governance.md): Requirements, exemptions, findings, and the one
  rule that hard-blocks.
