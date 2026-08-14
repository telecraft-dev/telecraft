# Telecraft

**Craft, govern, and verify OpenTelemetry across your whole estate.**

Telecraft is an open-source fleet and policy management platform for
OpenTelemetry. It models your collection topology, composes collector
configurations from owned, versioned building blocks, and then does the thing
no other tool does: derives from the configuration an *expectation* of what
telemetry should arrive — and checks it. Green means "the config worked",
never merely "the config applied".

Three separately-adoptable rungs, in any order:

| Rung | What it does | What it costs you |
|---|---|---|
| **Conformance** | Reads your telemetry backend and your collectors' reported configs, judges every service against its Service Class floor, and tells you *whose* problem each finding is | A connection string |
| **Authoring** | A console where teams compose Blueprints from governed Components and render plain otelcol YAML into git as pull requests | Nothing in your delivery path changes |
| **Serving** | A stateless OpAMP server that delivers rendered config from git to collectors — with GitOps as a co-equal alternative, chosen per collector | An OpAMP Supervisor beside each served collector |

Principles that hold everywhere: **nothing sits in the telemetry path** (if
Telecraft is down, no telemetry stops flowing); **git is the source of
truth** (history, rollback, approval and audit are git's, not ours);
**configurations, never binaries**; **vendor-neutral core** (Elastic,
Prometheus and friends are plugins); **air-gap first-class** (no hard
dependency on any SaaS).

## Status

Pre-alpha: design phase. The decision corpus lives in [`docs/`](docs/) —
start with the [glossary](docs/glossary.md), the visual
[terminology guide](docs/terminology.html), the
[requirements](docs/requirements/product-requirements.md) and the
[ADRs](docs/adr/). The build plan is [`docs/plan.md`](docs/plan.md).

## Licence

Apache-2.0.
