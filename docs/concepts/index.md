---
title: Concepts
description: What Telecraft is, the problem it solves, the three rungs you can adopt separately, and the principles that hold across all of them.
order: 1
---

# Concepts

Telecraft models your OpenTelemetry collection topology, composes collector
configuration from owned, versioned building blocks, and checks whether the
telemetry that configuration promises actually arrives.

This section explains the model. For step-by-step tasks, see the
[guides](../guides/index.md). For flags, file schemas, and API shapes, see the
[reference](../reference/index.md). Every capitalised term on these pages is
defined in the [glossary](../glossary.md), which is the authoritative
vocabulary.

## The problem

An OpenTelemetry estate fails quietly. A receiver gets wired into the wrong
pipeline, a gateway drops an attribute, a team ships a service with no
instrumentation, and nothing announces any of it. Nobody builds the dashboard
that would show the gap, because nobody knows the gap exists.

Tools that read collector configuration tell you what was asked for. Tools
that read a telemetry backend tell you what turned up. Neither can tell you
which of those two facts is the problem, so neither can tell you whose
problem it is.

Telecraft reads the estate three ways and crosses the readings. A configured
pipeline that delivers nothing is a defect for the platform team. An
unconfigured Service (a workload Telecraft governs, identified by its
`service.name`) that delivers nothing is a governance gap for the workload
owner. Both score zero, but they need different people and different fixes,
so Telecraft keeps them apart.

In short: green means "the configuration worked", never only "the
configuration applied". See [readings and verdicts](readings-and-verdicts.md)
for how the crossing works, and
[pipeline observability](pipeline-observability.md) for how Telecraft works
out what "worked" should mean.

## Three rungs

Telecraft is three products that share one model. You can adopt any one of
them without the others, and using a lower rung never requires a higher one.

| Rung | What it does | What it costs you |
|---|---|---|
| **Conformance** | Reads your telemetry backend and the configuration your collectors report, judges every Service against the Requirements (the rules it must meet) that apply to it, and routes each finding to an owner | A connection string |
| **Authoring** | Composes Blueprints (versioned collector designs) from governed Components (owned, versioned building blocks) and renders plain otelcol YAML into git as change proposals | Nothing in your delivery path changes |
| **Serving** | A stateless OpAMP server that delivers rendered configuration from git to collectors, with git delivery as an equal alternative chosen per collector | An OpAMP Supervisor beside each served collector |

Conformance needs no Blueprints and no rendering. Point it at your
requirements library, a reading of your collectors, and a telemetry backend,
and it produces verdicts. Authoring renders artefacts into git and stops
there, so whatever applies your configuration today keeps applying it.
Serving replaces that delivery step for the collectors you choose, one at a
time.

## Principles

These five hold everywhere in the design. Where a feature would break one,
the feature loses.

**Nothing sits in the telemetry path.** If Telecraft is down, no telemetry
stops flowing. Telecraft is never a collector, a gateway, or a hop. Where a
capability looks like it needs to be inline, Telecraft uses a *rendered
pattern* instead: configuration the renderer emits into your own collectors,
plus a reading Telecraft takes afterwards through the same pluggable seam it
reads your telemetry through. Collector health works exactly that way. A
pattern that stops working costs you findings, never data.

**Git is the source of truth.** History, rollback, approval, and the audit
trail already exist in git, so Telecraft builds none of them. The console
opens change proposals; it never writes to a cluster. A configuration you
committed by hand is legitimate, not drift: Intended (the configuration in
git) is whatever git says at that commit. Deleting Telecraft loses delivery,
never the record.

**Configurations, never binaries.** Telecraft ships no collector
distribution, no container image, and no chart. The renderer exports one
artefact per Tier (a position in your collection topology, such as edge or
gateway): plain otelcol YAML at a stable path, plus a small supervisor
configuration where a Tier is served. The renderer never knows what applies
the result.

**The core is neutral.** Interface names are domain terms. Implementations
name the vendor's product, never the company. A lint fails on any vendor word
inside the core, so backend independence is something you can grep for.

**Air gaps come first.** Nothing depends on a SaaS or on a particular git
host. The component Catalogue (the inventory of collector component types) is
built once and carried, so an air-gapped instance runs the same import
pipeline without the download step. Authentication is a seam, so you never
have to delegate identity to a git host that is not there.

## Where to go next

- [Readings and verdicts](readings-and-verdicts.md): Intended, Effective, and
  Observed; the outcome cross and what each outcome diagnoses; delivery
  status.
- [Ownership](ownership.md): who is accountable for what, how findings route,
  and how compliance rolls up a team tree without a single blended number.
- [Authoring](authoring.md): Services, Tiers, Blueprints, Components, the
  Catalogue, and the Allow-list policy that decides what a team can use.
- [Environments](environments.md): the Environment axis, Service Class,
  Sensitivity, Stability floors, and why every verdict is per environment.
- [Delivery](delivery.md): served and foreign collectors, the stateless OpAMP
  server, the commit stamp, and staged rollouts.
- [Pipeline observability](pipeline-observability.md): Expectations and
  Claims, self-telemetry, metering, and the card contract.
- [Governance](governance.md): Requirements, exemptions, findings, and the one
  rule that hard-blocks.
