---
title: Ownership
description: Owners, Teams, the ownership tree, how findings route to an accountable party, and how compliance rolls up without a blended number.
order: 3
---

# Ownership

Compliance is only useful if it is attributable. "A team running these servers
must run these modules" means nothing until every finding lands on exactly one
accountable party, so ownership is built into the model rather than bolted on
as a reporting dimension.

## Owners and Teams

An **Owner** is the lowest unit of management and belongs to exactly one Team.
A **Team** is a group of Owners, child Teams, or both.

Teams form a strict tree: a Team has at most one parent. Multi-parent
membership is rejected because every roll-up would then double-count.

The hierarchy arrives through a seam and is never owned by the platform. The
first-party shape is a reviewable `teams.yaml` in the estate repository,
committed and reviewed like everything else, with a `users.yaml` beside it
mapping signed-in identities to Owners. Group-claim mapping from an identity
provider fits behind the same resolution step.

Loading is strict and fails closed. An unknown field, a malformed document, a
duplicate id, or an authored object with no owner is a load error naming the
file. A finding that routes to nobody is this model's version of the lenient
verdict: the problem exists and nobody is paged.

## Universal ownership

Every authored object carries an owner. The authored set is:

- Component
- Blueprint
- Tier
- Hop
- Path
- Service
- Requirement
- Exemption

Ownership is an attribute of each object, not a parallel hierarchy. This is
finer-grained than "a team owns a collector", and deliberately so: a gateway
Tier run by the data-flow team can contain a redaction processor that the
security team governs, and an exporter whose endpoint is the gateway team's
concern even when it renders into a downstream team's artefact. The rendered
artefact for any collector is multi-owner by construction.

### Collectors are not ownable

A **Collector** is derived and read-only. It is never drawn on a canvas, never
authored, and never owned directly: it connects, is matched into a Tier by
selector, and inherits that Tier's owner, policy, and obligations.

Where one subset of a Tier's collectors needs a different owner, the selector
mechanism already expresses it: split the Tier. That keeps the authored graph
at a handful of nodes however large the estate grows, and it keeps the
platform free of per-collector state.

## Who acts: routing

A finding routes to the owner of the object the finding is **about**, never
the owner of the file it renders into. A broken redaction processor pages the
team that owns the processor. A dead exporter pages the team that owns the
exporter. An unmet Service floor pages the Service's owner. A collector
subject routes through the Tier it matched into.

Every failure to resolve is an error rather than a silent drop. A finding
about a collector that names no Tier, or about an object no estate authored,
stops the roll-up rather than quietly leaving a denominator.

## Roll-up

A Team's roll-up is the set of findings routed to owners anywhere in its
subtree. That set is larger than the sum of its Services: it includes the
delivery findings on Tiers the team owns and the component findings on
Components it owns. A parent team's view being bigger than its children's
combined service verdicts is the point, not an artefact.

Aggregation is **ratio plus worst, per finding kind, never blended**. For each
kind you get:

- a passing-over-counted ratio, kept as an integer pair;
- a worst-grade badge;
- the waived count, always alongside.

There is no single blended number at any level of the tree, including the
root. An exemption-heavy 100% cannot hide, because the waived total sits next
to it and every waived finding keeps its diagnosis in the list.

### Finding kinds

Kinds are scored separately and never collapsed into each other:

| Kind | What it carries |
|---|---|
| `service_conformance` | Requirement verdicts on a Service in an Environment |
| `delivery` | Delivery and population findings attached to a Tier |
| `component_health` | Findings about a Component |
| `expectation` | Claim failures from the Expectation engine |

### Grades

Every finding carries a grade: `pass`, `advisory`, or `violation`. A breach is
graded, never a block. `advisory` is worth surfacing; `violation` is the floor
unmet. Population findings add a fourth weight of their own, `neutral`, which
is not a pass: a neutral finding is excluded from every denominator, where a
pass would count in one. See
[population floors](delivery.md#population-and-never_seen).

A kind whose findings are all waived keeps a pass badge, and the waived count
beside it is what stops that from reading as clean.

## Authorisation follows ownership

What a signed-in human may author is derived from the ownership tree, not from
a parallel role store. There is one source of truth and two enforcement
shapes: where the forge supports review routing, the platform generates the
forge's code-ownership file from ownership metadata and merge rights stay the
forge's; where no review machinery exists, a platform merge gate enforces the
same rule.

That generated projection is what makes Exemption authority mechanical rather
than procedural: an exemption file touching a Requirement routes to that
Requirement's owning team for review, so self-forgiveness is impossible by
construction. See [governance](governance.md#exemptions-and-grace).

Reference: [ADR-0016](../adr/0016-ownership-and-components.md),
[ADR-0017](../adr/0017-team-hierarchy-rollup.md).
