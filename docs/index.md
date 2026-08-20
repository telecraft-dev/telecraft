---
title: Telecraft documentation
description: Start here to learn what Telecraft does, how to use it, and how to contribute to it.
order: 0
---

# Telecraft documentation

Telecraft is a source-available fleet and policy management platform for
OpenTelemetry. It models your collection topology, composes collector
configuration from owned and versioned building blocks, and checks that
the telemetry you expected actually arrived.

If you want to see it before you read about it, the
[live demo](https://demo.telecraft.dev) runs the real console over a
public demo estate.

## Where to start

Pick the entry point that matches what you came for.

- **New to Telecraft?** Read [what Telecraft is](concepts/index.md). It
  covers the problem, the three rungs you can adopt separately, and the
  principles that hold everywhere.
- **Ready to run something?** Start with the
  [quickstart](guides/quickstart.md), which takes you from nothing to a
  first verdict.
- **Looking up a flag or a file format?** Go to the
  [reference](reference/index.md).
- **Want to change the code?** Go to
  [contributing](contributing/index.md).

## How this documentation is organised

| Section | What it holds | Read it when |
|---|---|---|
| [Concepts](concepts/index.md) | The model and the vocabulary: readings and verdicts, ownership, authoring, environments, delivery, pipeline observability, governance | You want to understand what Telecraft is doing and why |
| [Guides](guides/index.md) | Task-oriented instructions, each one ending in a result you can see | You have a job to do |
| [Reference](reference/index.md) | Every command, flag, and authored file format, plus the [glossary](glossary.md) | You know what you want and need the exact spelling |
| [Contributing](contributing/index.md) | Local development, package architecture, the provider seams, and how decisions are recorded | You are working on Telecraft itself |

Concepts explain, guides instruct, and reference lists. When a page
starts doing another section's job, it links instead.

## The three rungs

Telecraft is three capabilities that you can adopt separately, in any
order. Each one is useful without the others.

Conformance
: Telecraft reads your telemetry backend and your collectors' reported
  configuration, judges every service against the floor set by its
  Service Class, and tells you whose problem each finding is. It costs
  you a connection string.

Authoring
: Teams compose Blueprints from governed Components and render plain
  OpenTelemetry Collector YAML into git as pull requests. Nothing in
  your delivery path changes.

Serving
: A stateless OpAMP server delivers rendered configuration from git to
  collectors, with GitOps as a co-equal alternative chosen per
  collector. It costs you an OpAMP Supervisor beside each served
  collector.

## Principles

These hold across every part of the product.

- **Nothing sits in the telemetry path.** If Telecraft stops, no
  telemetry stops flowing.
- **Git is the source of truth.** History, rollback, approval, and audit
  are git's, not ours.
- **Configurations, never binaries.** Telecraft ships configuration to
  collectors you already run.
- **The core is vendor-neutral.** Backends and fleet managers sit behind
  seams as plugins, and a lint keeps vendor names out of the core.
- **Air-gapped deployment is first-class.** Nothing depends on a hosted
  service.
