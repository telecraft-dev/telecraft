---
title: Environments
description: The Environment axis, Service Class and Sensitivity, stability floors per class and environment, and why every verdict is per environment.
order: 5
---

# Environments

Nothing distinguishes a Service in staging from the same Service in
production unless the model has a place to put the difference. Environment is
that place, and it is a separate axis from how much the Service matters and
from how sensitive its data is.

## The Environment axis

**Environment** is the test, staging, or production dimension of a Service's
deployment, aligned to the semantic-convention attribute
`deployment.environment.name`.

One Service has one identity and one owner across many Environments. The
alternative, a separate Service per environment, doubles the inventory, splits
ownership of one logical thing, and fights the convention.

The vocabulary is adopter-defined and open. Use `production`, `staging`,
`dev`, or whatever your estate already says. `production` is the one
distinguished value that policy defaults attach to.

The word "path" is never used for this axis. A [Path](authoring.md#the-objects)
is a topology object.

### Tiers declare an Environment

Every Tier declares exactly one Environment, and the Tier's Environment is an
attribute of the infrastructure: "this gateway is production plumbing".

That is how per-environment configuration works. Run parallel infrastructure
per environment and you declare sibling Tiers, a `gateway` in production and a
`gateway` in staging, each binding its own Blueprint version. The production
gateway can stay on version 4 while the staging gateway trials version 5. If
you run one shared gateway, you declare it `production` and it is judged as
production plumbing.

Because the Tier is the rendering and binding unit (see
[the objects](authoring.md#the-objects)), there is exactly one artefact per
Tier and no ambiguity about which Environment a configuration belongs to.

### Derived strictness

A Tier's configuration is judged at the Tier's declared Environment crossed
with the strictest Service Class among the Services whose Paths traverse it.
Topology answers which those are, so strictness is derived rather than
hand-maintained: routing a C1 Service's Path through a Tier tightens that
Tier's judgement automatically.

Non-production data flowing through a production Tier relaxes nothing.
Over-governed is harmless; under-governed is the failure mode.

## Service Class and Sensitivity

Two orthogonal axes, both carrying your own names and values, and never
conflated.

**Service Class** is how much a Service matters: `C1` > `C2` > `C3`, and the
values are renamable. It drives the required floor and it is cumulative, so C1
requires everything C2 does and more.

Service Class is never rendered as "Tier N". "Tier 1 app" is common industry
speak for criticality, and "Tier" here means a topology position and nothing
else. Nothing in the console, the docs, or the code writes a Service Class
that way.

**Sensitivity** is the orthogonal axis: what the data is, such as personal
data or financial data. It drives routing and redaction, never completeness.
A Service can be C3 and highly sensitive, or C1 and entirely public.

## Stability floors

A **stability floor** is the minimum upstream stability a Service's components
must meet. Floors exist because "a C1 service should not run its telemetry
through alpha components" needs a rule, and because stability is genuinely
per-signal: one component can be beta for logs and alpha for another signal.

Floors are configured per `(Service Class, Environment)`. The shipped defaults
are:

| Environment | C1 | C2 | C3 |
|---|---|---|---|
| `production` | beta or better | beta or better | alpha or better |
| everything else | no floor | no floor | no floor |

Non-production carries no floor because staging is where alpha and development
components are supposed to be exercised. C3 keeps a floor in production
because upstream defines `development` as not for production use; a team that
needs one anyway takes an [Exemption](governance.md#exemptions-and-grace),
which is what Exemptions are for.

Floors are validated as cumulative: a stricter class may never carry a lower
floor than a weaker one, because that would make adding a C1 Path *relax* a
Tier's judgement.

Two rules keep floors honest:

- **Floors evaluate per component and signal, and only for signals a
  Blueprint actually routes through the component.** An OTLP receiver
  carrying traces for a C1 production Service passes. Nothing is judged on
  capability it is not using.
- **A breach is a finding, never a block.** It carries remediation text naming
  the level, the floor, and the alternatives, and it routes to an owner.
  Breaches have legitimate temporary states, and blocking would let a routine
  catalogue import freeze everyone's configuration work.

Lifecycle is a separate axis, not a rung on the same ladder. Deprecated and
unmaintained are end-states judged by their own findings at any class and
environment, carrying upstream's machine-readable deprecation note as
remediation. They are never compared against a floor.

## Per-environment evaluation

The unit of conformance evaluation is `(Service, Environment, Requirement)`. A
Service's verdict is always per environment, and nothing blends across them.

Two reasons, both concrete. Staging's Observed telemetry would otherwise mask
a production outage of the same signal. And because sibling Tiers may bind
different Blueprint versions, one blended verdict would be judging two
different configurations at once.

Consequences worth knowing:

- Roll-ups count `(Service, Environment)` pairs as rows, so a Service deployed
  to three environments contributes three rows.
- A Service has no row in an environment where it runs nothing. Absence of an
  environment is not a finding.
- Estate views default to the `production` lens, which keeps headline numbers
  comparable. The lens is emphasis and evaluation context, not a hard filter:
  multi-environment surfaces keep every row visible.

### Requirements scope by environment

A Requirement stays one environment-neutral assertion with an optional
`environments` applicability list. Absent means every environment.
"Completeness in production only" is one authored line, `environments:
[production]`, rather than a per-environment variant file that would drift
from its sibling.

Explicit narrowing beats implicit non-coverage, so the default is all
environments rather than production only. An `environments` entry naming an
environment that appears nowhere in the estate raises an authoring finding:
visible, not fatal, because the vocabulary is open and a typo is the likely
cause.

Delivery gets no separate per-environment mechanism, and needs none: a
delivery finding attaches to a collector, which matches one Tier, which
declares one Environment. The facet is inherited by construction.

Reference: [ADR-0015](../adr/0015-vocabulary-alignment.md),
[ADR-0023](../adr/0023-environment-axis-stability-floors.md),
[ADR-0033](../adr/0033-per-environment-evaluation.md).
