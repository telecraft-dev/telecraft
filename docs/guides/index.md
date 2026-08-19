---
title: Guides
description: Find the task you need and the adoption path it belongs to.
order: 1
---

# Guides

These pages are tasks. Each one starts from a state you can reproduce, gives
you commands you can run, and ends with a result you can check.

They deliberately explain very little. For the model behind the vocabulary
(Tier, Blueprint, Service Class, Intended, Effective, Observed), read the
[concepts section](../concepts/index.md). For every flag, field and file
schema, read the [reference section](../reference/index.md).

## Before you start

You need Go 1.26 or later and `git`. Every guide here runs against the public
demo estate, `telecraft-dev/estate-demo`, so you can follow along before you
have an estate of your own. The [quickstart](quickstart.md) sets both up.

## Choose a starting point

Telecraft is three rungs, and they are separately adoptable in any order. Pick
the one that matches the problem you have today, and ignore the other two
until you want them.

| Rung | The problem it solves | What it costs you | Start here |
|---|---|---|---|
| Conformance | You cannot tell which Services are actually delivering the telemetry they are configured to deliver | A connection string to your telemetry backend | [Check conformance](check-conformance.md) |
| Authoring | Collector configuration is copied between teams, and nobody can say who owns a given processor | Nothing in your delivery path changes | [Author and render](author-and-render.md) |
| Serving | Rendered configuration has to reach collectors, and you want that path to be one you can audit | An OpAMP Supervisor beside each served collector | [Serve configurations](serve-configs.md) |

Nothing sits in your telemetry path on any rung. If Telecraft stops, no
telemetry stops flowing.

### The Conformance path

1. [Quickstart](quickstart.md) builds the CLI and gets you a first verdict.
2. [Check conformance](check-conformance.md) reads a backend, judges every
   Service against its requirements, and wires the check into CI.
3. [Write an Exemption](exemptions.md) waives a finding's count, with an owner
   and an expiry, without hiding the diagnosis.

### The Authoring path

1. [Quickstart](quickstart.md) builds the CLI.
2. [Author and render](author-and-render.md) composes Blueprints from governed
   Components, inspects a team's effective palette, and renders plain otelcol
   YAML into git.

### The Serving path

1. [Author and render](author-and-render.md) first: the server serves what the
   renderer wrote, so there is nothing to serve until you have rendered.
2. [Serve configurations](serve-configs.md) runs the stateless OpAMP server
   from a local estate or a git URL.
3. [Stage a Rollout](stage-a-rollout.md) moves one Tier's population onto a new
   Blueprint version in cohorts, advancing and aborting by pull request.

## See it running first

If you would rather look before you build, [explore the
demo](explore-the-demo.md) is a guided tour of <https://demo.telecraft.dev>, a
read-only console rebuilt from the demo estate on every push.
