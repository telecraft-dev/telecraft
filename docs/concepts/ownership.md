---
title: Ownership
description: Owners, Teams, the ownership tree, how findings route to an accountable party, and how compliance rolls up without a blended number.
order: 3
---

# Ownership

Compliance is only useful when it is attributable. "A team running these
servers must run these modules" means nothing until every finding lands on
exactly one accountable party. So ownership is part of the model, not a
reporting dimension added afterwards.

## Owners and Teams

An **Owner** is the lowest unit of management and belongs to exactly one Team.
A **Team** is a group of Owners, child Teams, or both.

Teams form a strict tree: a Team has at most one parent. Telecraft rejects a
Team with two parents, because every roll-up would then count it twice.

The hierarchy arrives through a seam, and Telecraft never owns it. The
first-party shape is a `teams.yaml` file in the estate repository, committed
and reviewed like everything else, with a `users.yaml` beside it that maps
signed-in identities to Owners. Group-claim mapping from an identity provider
fits behind the same resolution step.

Loading is strict and fails closed. An unknown field, a malformed document, a
duplicate id, or an authored object with no owner is a load error that names
the file. A finding that routes to nobody would mean the problem exists and
nobody is paged, so Telecraft refuses to load that state.

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

Ownership is an attribute of each object, not a parallel hierarchy. That is
finer-grained than "a team owns a collector", on purpose. A gateway Tier (a
position in the collection topology) run by the data-flow team can contain a
redaction processor that the security team governs, and an exporter whose
endpoint is the gateway team's concern even when it renders into a downstream
team's artefact. The rendered artefact for any collector has many owners.

### Collectors are not ownable

A **Collector** is derived and read-only. You never draw it on a canvas, never
author it, and never own it directly. It connects, Telecraft matches it into a
Tier by selector, and it inherits that Tier's owner, policy, and obligations.

When one subset of a Tier's collectors needs a different owner, split the
Tier: the selector already expresses the difference. That keeps the authored
graph at a handful of nodes however large the estate grows, and keeps
Telecraft free of per-collector state.

## Who acts: routing

A finding routes to the owner of the object the finding is **about**, never
the owner of the file it renders into. A broken redaction processor pages the
team that owns the processor. A dead exporter pages the team that owns the
exporter. An unmet Service floor pages the Service's owner. A finding about a
collector routes through the Tier the collector matched into.

Every failure to resolve is an error, never a silent drop. A finding about a
collector that names no Tier, or about an object no estate authored, stops
the roll-up rather than quietly falling out of the compliance ratios.

## Roll-up

A Team's roll-up is the set of findings routed to owners anywhere in its
subtree. That set is larger than the sum of its Services: it includes the
delivery findings on Tiers the team owns and the component findings on
Components it owns. A parent Team's view is bigger than its children's
combined service verdicts, and that is the point.

Aggregation is **ratio plus worst, per finding kind, never blended**. For each
kind you get:

- a passing-over-counted ratio, kept as an integer pair;
- a worst-grade badge;
- the waived count, always alongside.

There is no single blended number at any level of the tree, including the
root. An exemption-heavy 100% cannot hide, because the waived total sits next
to it and every waived finding keeps its diagnosis in the list.

### Finding kinds

Telecraft scores each kind separately and never collapses one into another:

| Kind | What it carries |
|---|---|
| `service_conformance` | Requirement verdicts on a Service in an Environment |
| `delivery` | Delivery and population findings attached to a Tier |
| `component_health` | Findings about a Component |
| `expectation` | Claim failures from the Expectation engine |

### Grades

Every finding carries a grade: `pass`, `advisory`, or `violation`. A breach is
graded, never blocked. `advisory` is worth surfacing; `violation` means the
floor is unmet. Population findings add a fourth weight of their own,
`neutral`, which is not a pass: no compliance ratio counts a neutral
finding, where a pass would count in one. See
[Population floors](delivery.md#population-and-never_seen).

A kind whose findings are all waived keeps a pass badge. The waived count
beside it is what stops that from reading as clean.

## Authorisation follows ownership

What a signed-in person can author is derived from the ownership tree, not
from a separate role store. There is one source of truth: the ownership
metadata in the estate repository.

Where the forge (your git host) supports review routing, Telecraft
**generates** the forge's code-ownership file from that metadata, and merge
rights stay with the forge. The generated file is a cache, never the source:
people edit `teams.yaml` and the `owner` field on objects, and the renderer
emits the file in the configured forge's dialect. Every line carries the
owners of the team *plus its ancestors*, so review reach comes from the tree
rather than from the directory layout, and the ancestors who govern a team
keep their say.

A forge declares its capabilities up front rather than failing at merge time,
so if you run on plain git transport you can see exactly which forge-enforced
review you have given up. The render gate (see
[enforcement points](governance.md#enforcement-points)) still holds either
way.
