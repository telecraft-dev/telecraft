---
title: Environments
description: The Environment axis, Service Class and Sensitivity, stability floors per class and environment, and why every verdict is per environment.
order: 5
---

# Environments

Nothing distinguishes a Service in staging from the same Service in
production unless the model has a place to put the difference. Environment is
that place. It is a separate axis from how much the Service matters and from
how sensitive its data is.

## The Environment axis

**Environment** is the test, staging, or production dimension of a Service's
deployment, aligned to the semantic-convention attribute
`deployment.environment.name`.

One Service has one identity and one owner across many Environments. A
separate Service per environment would double the inventory, split ownership
of one logical thing, and fight the convention.

The vocabulary is yours and open. Use `production`, `staging`, `dev`, or
whatever your estate already says. `production` is the one distinguished
value, and policy defaults attach to it.

The word "path" never refers to this axis. A
[Path](authoring.md#the-objects) is a topology object.

### Tiers declare an Environment

Every Tier declares exactly one Environment, and the Tier's Environment is an
attribute of the infrastructure: "this gateway is production plumbing".

That is how per-environment configuration works. If you run parallel
infrastructure per environment, you declare sibling Tiers, a `gateway` in
production and a `gateway` in staging, each binding its own Blueprint
version. The production gateway can stay on version 4 while the staging
gateway trials version 5. If you run one shared gateway, you declare it
`production` and Telecraft judges it as production plumbing.

Because the Tier is the rendering and binding unit (see
[the objects](authoring.md#the-objects)), there is exactly one artefact per
Tier and no doubt about which Environment a configuration belongs to.

### Derived strictness

Telecraft judges a Tier's configuration at the Tier's declared Environment
crossed with the strictest Service Class among the Services whose Paths pass
through it. Topology says which Services those are, so you never maintain
strictness by hand: routing a C1 Service's Path through a Tier tightens that
Tier's judgement automatically.

Non-production data flowing through a production Tier relaxes nothing.
Over-governed is harmless; under-governed is the failure mode.

## Service Class and Sensitivity

These are two independent axes. Both carry your own names and values, and
Telecraft never conflates them.

**Service Class** is how much a Service matters: `C1` > `C2` > `C3`, and you
can rename the values. It drives the required floor, and it is cumulative, so
C1 requires everything C2 does and more.

Service Class is never written as "Tier N". "Tier 1 app" is common industry
shorthand for criticality, but "Tier" in Telecraft means a topology position
and nothing else. Nothing in the console, the docs, or the code writes a
Service Class that way.

**Sensitivity** is the other axis: what the data is, such as personal data or
financial data. It drives routing and redaction, never completeness. A
Service can be C3 and highly sensitive, or C1 and entirely public.

## Stability floors

A **stability floor** is the minimum upstream stability a Service's components
must meet. Floors exist because "a C1 service should not run its telemetry
through alpha components" needs a rule, and because stability is per signal:
one component can be beta for logs and alpha for another signal.

Floors are configured per `(Service Class, Environment)`. The shipped defaults
are:

| Environment | C1 | C2 | C3 |
|---|---|---|---|
| `production` | beta or better | beta or better | alpha or better |
| everything else | no floor | no floor | no floor |

Non-production carries no floor, because staging is where you exercise alpha
and development components. C3 keeps a floor in production because upstream
defines `development` as not for production use. A team that needs one anyway
takes an [Exemption](governance.md#exemptions-and-grace).

Floors are validated as cumulative: a stricter class can never carry a lower
floor than a weaker one, because adding a C1 Path would then *relax* a Tier's
judgement.

Two rules govern how floors apply:

- **Floors evaluate per component and signal, and only for signals a
  Blueprint routes through the component.** An OTLP receiver carrying traces
  for a C1 production Service passes. Nothing is judged on capability it is
  not using.
- **A breach is a finding, never a block.** It carries remediation text naming
  the level, the floor, and the alternatives, and it routes to an owner.
  Breaches have legitimate temporary states, and blocking would let a routine
  catalogue import freeze everyone's configuration work.

Lifecycle is a separate axis, not a rung on the same ladder. Deprecated and
unmaintained are end states, judged by their own findings at any class and
environment, carrying upstream's machine-readable deprecation note as
remediation. They are never compared against a floor.

## Per-environment evaluation

The unit of conformance evaluation is `(Service, Environment, Requirement)`. A
Service's verdict is always per environment, and nothing blends across them.

There are two concrete reasons. Staging's Observed telemetry would otherwise
mask a production outage of the same signal. And because sibling Tiers can
bind different Blueprint versions, one blended verdict would judge two
different configurations at once.

Consequences worth knowing:

- Roll-ups count `(Service, Environment)` pairs as rows, so a Service deployed
  to three environments contributes three rows.
- A Service has no row in an environment where it runs nothing. Absence from
  an environment is not a finding.
- Estate views default to the `production` lens, which keeps headline numbers
  comparable. The lens sets emphasis and evaluation context; it is not a hard
  filter, so multi-environment surfaces keep every row visible.

### Requirements scope by environment

A Requirement stays one environment-neutral assertion with an optional
`environments` applicability list. Absent means every environment.
"Completeness in production only" is one authored line, `environments:
[production]`, rather than a per-environment variant file that would drift
from its sibling.

The default is all environments rather than production only, because explicit
narrowing beats implicit non-coverage. An `environments` entry naming an
environment that appears nowhere in the estate raises an authoring finding:
visible, not fatal, because the vocabulary is open and a typo is the likely
cause.

Delivery needs no separate per-environment mechanism. A delivery finding
attaches to a collector, which matches one Tier, which declares one
Environment, so the finding inherits its Environment from the Tier.
