---
title: Governance
description: Requirements and conformance levels, exemptions and grace, the kinds of finding, where policy is enforced, and how ungoverned things surface.
order: 8
---

# Governance

Governance in Telecraft is a library of authored assertions, one evaluator
that judges everything against them, and exactly one rule anywhere in the
system that stops you doing something. Everything else is a finding routed to
somebody who can act on it.

## Requirements

A **Requirement** is a named, versioned rule a Service must meet, and it
carries its own remediation text. The remediation is mandatory, so every
finding tells you what to do.

A Requirement can assert on the Effective reading, the Observed reading, or
both. Telecraft derives the kind from the assertions present rather than
asking you to write it, so the two can never disagree:

| Kind | Asserts on |
|---|---|
| `config` | Effective only |
| `signal` | Observed only |
| `config_and_signal` | Both |

Asserting on both readings is what makes the
[outcome cross](readings-and-verdicts.md#the-outcome-cross) possible. A
configuration-only Requirement can be satisfied by a collector that delivers
nothing, and a signal-only one can fail without naming a cause.

Configuration assertions name receiver, processor, or exporter types, and
each list is satisfied if *any* of its entries is present. "Collect logs
somehow" is the real requirement; the choice between two receivers is an
implementation detail.

Signal assertions are expressed in terms of presence, volume over a window,
and attribute coverage. A minimum volume guards against a pipeline that is
technically alive and delivering almost nothing, which reads as healthy to any
presence check. Attribute coverage defaults to total coverage unless an author
relaxes it, because a partially instrumented estate is worth distinguishing
from an entirely uninstrumented one.

**Requirements never embed a backend query language.** No such field exists in
the model. If a requirement could carry a query string, the
`TelemetryProvider` seam would stop being an abstraction and only one backend
would ever really be supported. The sanctioned extension is `AttributeNames`,
the set of attribute names in use for a Service, signal, and window. It
enables attribute-shape checking as pure string logic without widening the
seam towards any vendor's API.

### The library

The requirements library is a directory of YAML files, one concern per file,
so a change to one Requirement is a one-file diff in review.

Loading is strict and fails closed. An unknown field, a malformed document, or
a missing mandatory field is a load error that names the file and the field,
never a quietly lenient verdict. A library that fails to load has judged
nothing, and the CI check reports that as a failure to run rather than as a
pass.

Version is part of the model: raising the bar is a dated, visible event rather
than a silent overnight change in everyone's score. The evaluator always
judges against a Requirement's current version, so a
[`satisfies` claim](authoring.md#pins-and-tracking) never freezes the
goalposts. Passing the version you claim while failing the current one is
[`library_drift`](readings-and-verdicts.md#the-outcome-cross).

## Conformance levels

Telecraft uses the OpenTelemetry semantic-conventions vocabulary rather than
inventing one. Every Requirement carries a `requirement_level`, one of four:

- `required`
- `conditionally_required`
- `recommended`
- `opt_in`

Absent defaults to `recommended`, matching the upstream default. The
four-level scale is richer than a binary required list: `recommended` is the
home for attribute coverage that is not yet universal, and tightening a level
is an authored, dated event you can point at.

The level is validated at load and carried with every finding it produces, so
a report can distinguish what your organisation demands from what it
suggests.

## Exemptions and grace

Two things waive a finding's count. They stay distinct because one is
authored and one is computed.

An **Exemption** is an ordinary authored object that lives in git. It waives
exactly one Requirement, and it carries:

- a mandatory owner, because a waiver nobody answers for is not a waiver;
- a mandatory expiry, because an open-ended waiver is a deleted requirement;
- exactly one subject, either one Service or one Team subtree.

Subtree scope exists for onboarding: "everything under this team is waived
from the completeness requirement until March" is one reviewable file rather
than 40 copies. There are no narrowing semantics, because an Exemption waives
a count and never forbids complying.

**Authority is a review rule, not a workflow.** An Exemption is valid only
when the change introducing it is approved by the owner of the Requirement
being waived, or by that owner's ancestor team. This makes self-forgiveness
impossible: an Exemption is a loosening, and every loosening mechanism in the
model runs in one direction, the same way only an ancestor can widen an
[Allow-list](authoring.md#allow-lists-and-grants).

Telecraft builds no approval workflow for that. Review routing belongs to the
forge (your git host), driven by the
[generated code-ownership projection](ownership.md#authorisation-follows-ownership).
What Telecraft enforces itself is structural: the loader refuses an Exemption
that names no Requirement, no owner, or no expiry, or that carries both a
Service and a Team.

Renewal is a fresh change proposal. When an Exemption expires, it stops
counting on the next run with no manual step, and an expired Exemption left in
the tree is an authoring finding: dead configuration, in the same spirit as an
aged `never_seen`.

A **Grace Period** is the other mechanism: an onboarding window, scoped by
Service Class, that Telecraft computes from the Service's class and onboarding
date. Findings are waived during it. Windows shrink as class rises, and the
loader enforces that shape, so a table that gave the most critical class the
longest forgiveness cannot load.

Where both cover the same finding, the Exemption wins, because it names the
party answering for the waiver.

**Neither replaces the diagnosis.** A waived finding keeps its outcome and its
detail, gives up only its count, and its waived total appears at every
[roll-up](ownership.md#roll-up) level. Authority controls who can loosen;
visibility makes sure loosening never hides.

## Findings and their kinds

A finding is data. It carries a subject, a grade, a diagnosis, and remediation
text, and it routes to the owner of the object it is about.

Compliance findings roll up under four
[kinds](ownership.md#finding-kinds), scored separately and never blended:
`service_conformance`, `delivery`, `component_health`, and `expectation`. Each
carries a grade of `pass`, `advisory`, or `violation`.

Several families feed those kinds:

- **Conformance findings**, one per Requirement per row, carrying one of the
  [outcomes](readings-and-verdicts.md#the-outcome-cross).
- **Population findings** on Tiers: `never_seen`, `under_populated`, and a
  floor conflict where a declared floor sits above the live derived count.
  These add a `neutral` weight of their own, which is not a pass: no
  compliance ratio counts a neutral finding. See
  [population and `never_seen`](delivery.md#population-and-never_seen).
- **Stability floor and lifecycle findings** on components a Blueprint routes.
  See [Stability floors](environments.md#stability-floors).
- **Expectation findings** from claim failures. See
  [where claim failures land](pipeline-observability.md#where-claim-failures-land).
- **Authoring findings**, which are about the authored files rather than the
  estate: a Component reference that cannot deliver what it promises, a lane
  whose explicit order contradicts the ordering rules, a Requirement naming an
  environment the estate has never seen, or an expired Exemption still
  present. These route to an owner and never block anyone else's render.

## Enforcement points

There is **one evaluator**, and every caller goes through it: the composer as
you edit, the render step, the CI check, and the continuous evaluation over
Effective configurations. CI never vendors a copy of the rules, because the
policy state they need (the active Catalogue, the Allow-lists, the Grants, the
floors) lives with the instance, and a vendored copy would judge with stale
policy. The composer's live findings, the save gate, CI annotations, and the
estate view can never disagree, because the same code judges all four over
the same policy.

Judging is stateless, draft in and findings out, so validation is continuous
rather than triggered by saving: the composer shows findings and palette
states as you edit, and saving asks the same question with enforcement on.

**Exactly one policy rule hard-blocks: an Allow-list violation, at render.** A
Blueprint that references a component outside its team's effective list does
not render into the estate repository. The rule has a complete authority chain
and a fast, auditable escape hatch: request a
[Grant](authoring.md#allow-lists-and-grants). Because that escape hatch
exists, there is no override mechanism.

Everything else that is policy produces findings. Stability floors and
lifecycle never block: breaches have legitimate temporary states, such as a
newly imported Catalogue downgrading a long-running component, and blocking
would let a routine Catalogue import freeze everyone's configuration work.
Tightening can be added later without breaking the model, while loosening
cannot, so findings stay findings.

Separately from policy, **mechanical invalidity always refuses a render**: a
dangling reference, a collision between rendered component ids, content edited
without a version bump, or a lane that would compile to a pipeline no
collector accepts. That is the same category as invalid YAML, not policy. An
artefact nobody reviewed must not exist, and a partial artefact is one nobody
reviewed.

To state the distinction once more: *mechanical validity* can always refuse.
*Policy* hard-blocks on Allow-list violations and nothing else.

### The CI check

A check mode evaluates the estate once, writes one machine-readable report,
and exits non-zero exactly when counting failures exist. Conformance you can
only see in a browser regresses between people remembering to look.

Every row is judged by default, because a gate that checked only one
environment would pass estates failing everywhere else. A load error exits
with a distinct code rather than a lenient zero. Waived findings stay in the
report with their diagnosis and count towards the waived totals, so a green
built on exemptions cannot look like a clean green. See the
[reference](../reference/index.md) for exit codes and flags.

## Ungoverned

"Ungoverned" names two different facts, and the remedy depends on which one
you have. Both drain the same way: somebody authors or widens a selector, or
registers a Service, which is exactly what the onboarding prompt proposes.

**An ungoverned collector** is population-level: a discovered collector that
matches no Tier selector. It might be served, in which case it runs the
[Unmatched artefact](delivery.md#the-unmatched-artefact) and is stamped and
health-visible, or foreign, read through the `EstateProvider` seam.

Ungoverned collectors appear in the estate view with an explicit onboarding
prompt, and no compliance ratio counts them: a concern, never a
failure. No stigma attaches to the delivery path itself. Foreign but governed
is fully legitimate; only matching no selector is the problem.

**Ungoverned data** is signal-level: telemetry from unrecognised
`service.name` values arriving through *governed* pipelines. It is also the
`ungoverned` [outcome](readings-and-verdicts.md#the-outcome-cross), which
passes the requirement and is surfaced anyway.

### Quarantine

The **quarantine pattern** handles ungoverned data. A gateway Blueprint can
carry an authored routing rule that sends telemetry from unrecognised
`service.name` values to a short-retention quarantine destination. Telecraft
reads what lands there through the same seam it reads everything else, and
tells you: unknown sources are arriving, so onboard them.

That is a governance pattern you author out of ordinary Components, never a
Telecraft runtime capability, because Telecraft is
[not in the data path](index.md#principles). It drains by onboarding, never by
retention growth.

The quarantine destination is not the Unmatched artefact. One is data-level
routing you author; the other is a configuration the server hands to a
collector nobody has claimed.
