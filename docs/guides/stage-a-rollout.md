---
title: Stage a Rollout
description: Move one Tier's collector population onto a new Blueprint version in Cohorts, advancing and aborting by pull request.
order: 6
---

# Stage a Rollout

A Rollout moves one Tier from one Blueprint version to another in Cohorts,
instead of flipping the whole population at once. It is optional: the default
is still the flat rebind, which means editing the Tier's `blueprint:` field in
a pull request.

Reach for a Rollout when the population is large enough, or the change risky
enough, that you want the first stage to be two machines you trust.

This guide edits a copy of the demo estate, which already carries one active
Rollout:

```sh
cp -R ../estate-demo ../my-estate
rm -rf ../my-estate/.git
```

## Anatomy of a Rollout

`teams/data-flow/rollouts/bridge-canary.yaml` in the demo estate:

```yaml
owner: gateway-owners
tier: data-flow/kafka-bridge
from: data-flow/bridge-standard@2
to: data-flow/bridge-next@1
stage: 1
hash_attributes: [service.instance.id]
stages:
  - cohort:
      hosts:
        attribute: host.name
        values: [bridge-a1, bridge-a2]
    soak: 2h
  - cohort:
      percent: 25
    soak: 24h
  - cohort:
      percent: 100
    soak: 24h
```

- `owner` must be the target Tier's owner. A Rollout is the Tier owner's tool,
  never a cross-team lever.
- `tier` is the one Tier this Rollout stages. Cohorts subdivide that Tier's
  population; a Cohort is never a Tier itself.
- `from` must equal the Tier's current binding. `to` names the candidate. The
  two must bind distinct Blueprints, so you author the candidate as a sibling
  Blueprint.
- `stage` is the index of the active stage, counting from zero. Advancing is a
  reviewed edit of this one field.
- `hash_attributes` pins the attributes that fractional membership hashes
  over. It is pinned with the object, because changing it mid-Rollout would
  reshuffle every fractional Cohort.
- `soak` is the minimum time a stage must have been active before its advance
  can be proposed.

## Cohorts

A stage's Cohort has three forms, and you can mix them. Membership is their
union, so "the three machines I trust, plus 5%" is one stage.

```yaml
cohort:
  hosts:                       # enumerated attribute values
    attribute: host.name
    values: [bridge-a1, bridge-a2]
  match:                       # an attribute selector, equality over all pairs
    k8s.cluster.name: eu-west-1
  percent: 5                   # a stable hash over hash_attributes
```

Two properties are worth knowing before you rely on them:

- The fractional form is statistically 5%, never exactly 5%.
- Membership is the union of every stage up to and including the active one,
  so advancing only ever widens. No collector flaps backwards out of a Cohort.

Membership is a pure function of the Rollout and the collector's reported
attributes, computed at serve time and never stored. Every replica computes
the same answer, which is why the server needs no coordination.

## The Tier is dual-bound while it runs

While the Rollout file exists, the Tier binds both Blueprints, and the render
emits both artefacts at head:

```sh
./telecraft render \
  -estate ../my-estate \
  -catalogue ../my-estate/catalogues/catalogue-v0.158.0.json \
  -commit 0000000000000000000000000000000000000000 \
  -out /tmp/rollout
```

```
kafka-bridge.supervisor.yaml
kafka-bridge.yaml
kafka-bridge@next.yaml
```

The base artefact is the *from* binding, and it goes to everybody outside the
Cohort:

```
# Tier data-flow/kafka-bridge (production), Blueprint data-flow/bridge-standard@2, commit 0000000000000000000000000000000000000000.
```

`kafka-bridge@next.yaml` is the *to* binding, and it goes to Cohort members:

```
# Tier data-flow/kafka-bridge (production), Blueprint data-flow/bridge-next@1, commit 0000000000000000000000000000000000000000.
```

Both are in git, both are reviewable, and the server chooses between them on
each connect from the collector's own attributes.

While a Rollout is active, the Rollout file is the only way to rebind the
Tier. A direct rebind fails render validation:

```
render: invalid topology sources:
  - rollout "data-flow/bridge-canary" binds from data-flow/bridge-standard@2 but tier "data-flow/kafka-bridge" binds data-flow/bridge-next@1. While a Rollout is active, the Rollout file is the only way to rebind the Tier.
```

## Advance

Advancing is one line: bump `stage` in a pull request.

```yaml
stage: 2
```

Render again, and both artefacts are still there. What changed is who is in
the Cohort, which the server recomputes on the next connect. Both render
commands exit 0; a validation failure exits 2 and writes nothing.

The stage number is an index into the authored stages, so it can't run past
the end:

```
render: invalid topology sources:
  - ../my-estate/teams/data-flow/rollouts/bridge-canary.yaml: rollout "data-flow/bridge-canary" declares stage 5 of 3. The stage is a zero-based index into the stages list. To complete the rollout, delete the file instead of counting past the end.
```

On an instance, Telecraft proposes that edit for you when the stage's exit
criteria are met, on a deterministic branch, and a human merges it. Telecraft
only ever proposes: there is no control loop that merges its own change.
Halting is passive: it is the proposal that never arrives.

Two halt conditions ship, and they gate the advance:

- A Cohort member reports `FAILED` for the candidate artefact's hash. It took
  the offer, the apply failed, and the Supervisor has already reverted it. A
  `FAILED` for any other hash belongs to some other delivery.
- A Cohort member takes the candidate and then goes silent within the soak
  window. That is the crash-loop signature that never reports `FAILED`.

Any halted Cohort member blocks the advance. When 10% or more of the seen
Cohort has halted, Telecraft proposes the abort instead of the advance.

## Complete

Completing is one pull request that does two things: flip the Tier to the
candidate, and delete the Rollout file.

```yaml
# teams/data-flow/tiers/kafka-bridge.yaml
blueprint: data-flow/bridge-next@1
```

```sh
rm ../my-estate/teams/data-flow/rollouts/bridge-canary.yaml
```

The next render retires the `@next` artefact in the same change:

```
kafka-bridge.supervisor.yaml
kafka-bridge.yaml
```

```
# Tier data-flow/kafka-bridge (production), Blueprint data-flow/bridge-next@1, commit 0000000000000000000000000000000000000000.
```

## Abort

Aborting is deleting the Rollout file alone, which leaves the Tier bound to
*from*:

```sh
rm ../my-estate/teams/data-flow/rollouts/bridge-canary.yaml
```

```
gateway-staging.supervisor.yaml
gateway-staging.yaml
gateway.supervisor.yaml
gateway.yaml
kafka-bridge.supervisor.yaml
kafka-bridge.yaml
```

The `@next` artefact is gone, and every collector in the Tier is back on the
base artefact on its next connect. Nothing needs rolling back by hand, because
nothing was ever mutated: the whole Rollout was one small file in git.

## What the Foreign path sees

Telecraft doesn't serve a git-delivered collector, so it can't hand it the
candidate artefact and can't read its `RemoteConfigStatus`. Rollouts still
work there, but the reading is advisory rather than authoritative.

Telecraft decides which artefact a Foreign collector is running from what it
can see, in this order:

1. An `APPLIED` acknowledgement naming a config hash, where one exists. Only
   `APPLIED` names what is running: a `FAILED` reading's hash names what was
   refused, and `APPLYING` is not there yet.
2. Otherwise, the reported pipeline wiring compared against both artefacts,
   with component order preserved.

That gives four readings: `to`, `from`, `other` (a different commit entirely,
or a foreign config), and `unknown`. Wiring that both artefacts share
distinguishes nothing, so it reads `unknown` rather than a guess.

The important consequence: a Foreign Cohort member still on the *from*
artefact is lag, never failure. Telecraft never controlled Foreign delivery
timing, and the `FAILED` halt signal isn't available there. The Rollout reads
everything and blocks nothing on that path.

## What next

- [Serve configurations](serve-configs.md) covers how Cohort members receive
  the candidate artefact.
- [Check conformance](check-conformance.md) answers whether the new Blueprint
  version delivers.
