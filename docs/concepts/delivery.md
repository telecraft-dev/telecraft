---
title: Delivery
description: Served and foreign collectors, the stateless OpAMP server, rendered artefacts and the commit stamp, the Unmatched artefact, and staged rollouts.
order: 6
---

# Delivery

Rendering puts an artefact in git. Delivery is how it reaches a collector, and
Telecraft treats that as a choice you make per collector rather than a
condition of using anything else.

## Two delivery paths, both legitimate

A collector's **delivery path** is a visible property of the collector, and
there are two.

**Served** means the collector receives its configuration from the platform's
OpAMP server. Serving requires the upstream OpAMP Supervisor beside every
served collector, which makes adopting it a migration rather than an addition:
the Supervisor takes over the collector process.

**Foreign** means anything else delivers the configuration: a GitOps
controller, configuration management, a cloud agent, or a person with an
editor. Foreign is legitimate, not lesser. The platform serves everything and
GitOps is an alternative, not a fallback; mixed substrates are the default
mode, because half of large estates span both container orchestration and
virtual machines.

Both paths get identical verdicts. Continuous evaluation judges every
Effective configuration by the same rulebook, so a foreign configuration using
a non-allowed component receives the same finding a composed one would, routed
to the same owner. Not being able to block is not a reason not to report.

The two differ in remedies, which is why the path is surfaced. A drifted
served collector and a drifted foreign collector need different people.

## Rendered artefacts and the commit stamp

The renderer exports exactly one artefact per Tier: plain otelcol YAML at a
stable repository path, `rendered/<team>/<tier>.yaml`. Where a Tier is served,
a small supervisor configuration renders beside it at
`rendered/<team>/<tier>.supervisor.yaml`.

Rendering is deterministic, so CI can recompute the whole `rendered/` tree and
fail on any mismatch. Humans never commit into it.

**Every artefact carries its own identity.** The renderer stamps the commit SHA
into the collector's own telemetry resource as `telecraft.commit`, alongside
`telecraft.tier` naming the Tier that produced it. The collector reports both
back, so "which commit is this running" is read *from* the collector rather
than remembered *about* it.

That single decision is what makes the server stateless, and it works
identically on both delivery paths: a foreign collector reports the same stamp,
so it gets the same identity and the same claim evaluation for free.

The stamp rides on the collector's self-telemetry resource only. Nothing is
ever written into your telemetry data.

### Renderer hard rules

Three rules are generated automatically rather than left to guidance, because
each one fails silently when a human forgets it:

- Additional OpAMP extensions render as `opamp/<name>`, never a bare `opamp`
  block, which would silently override the Supervisor's injected endpoint.
- A node-unique identifying attribute arrives through Downward API
  indirection, so one daemonset manifest yields per-node identity.
- Data crossing an untrusted Hop has the platform's attribute namespace
  stripped, and identity is re-derived from the receiving Tier's own stamps
  rather than trusted from inbound data.

## The stateless OpAMP server

A collector connects and reports its identifying attributes. The server
matches those attributes against the Tier selectors held in git at head,
serves the rendered artefact at that path, and remembers nothing.

The serving path may hold exactly three things, all rebuildable and none
durable:

1. The repository snapshot: the fetched estate at last-known head, plus the
   selector index compiled from it, refreshed by poll. The fetch interval is
   the bounded staleness, and it is the only freshness knob.
2. A per-connection digest of each connected collector's last-reported
   effective configuration, in process memory, dying with the connection.
3. Nothing else.

Artefact choice is a pure function of head and the reported attributes.
That is why several replicas behind a load balancer are independent read-only
clones needing no coordination, and why a restart is a non-event.

Removing the server loses delivery, never the record.

Two rules keep first boot honest:

- **The server never serves an empty configuration map.** A Supervisor handed
  an empty map reports applied and healthy while running nothing, which is the
  worst possible failure: silent success.
- **A collector matching no selector still gets an artefact.** See below.

## The Unmatched artefact

A served collector matching no Tier selector receives the **Unmatched
artefact**: a distinguished, root-team-owned rendered configuration, commit
stamped, self-telemetry on, no data pipelines, and non-empty by construction.

The one case where the platform is genuinely talking to an ungoverned thing is
not wasted. Instead of a silent `nop` pipeline, the collector becomes
maximally visible: stamped, health-reporting, and explicitly labelled as
governed by nobody, so the onboarding prompt can say something useful: alive,
on this node, running this version, connected since this morning.

Not knowing is a rendered, visible state rather than an absence.

The Unmatched artefact is not the quarantine destination. Quarantine is a
data-level routing pattern and lives in [governance](governance.md#ungoverned).

## Population and `never_seen`

The mirror image of an unmatched collector is a Tier whose selector has
matched nothing.

`never_seen` is a finding class attached to the Tier, never an eighth
conformance outcome: the outcomes are per-requirement crosses needing an
Effective or Observed reading, and a Tier with no collectors has neither.
The selector *is* the expectation, so the finding is the selector's.

It is neutral without a floor. A freshly authored Tier awaiting its workload
is an ordinary state: visible, excluded from every compliance denominator, and
never red. Its age doubles as the stale-configuration signal, "never matched in
90 days".

A **population floor** gives it teeth. The floor is the minimum instance count
a Tier's selector should match, either derived live from the substrate through
the `InventoryProvider` seam or declared in the Tier file. Absent means no
teeth at all. With a floor above zero and a persistent zero past the grace
window, `never_seen` escalates to violation grade.

`under_populated` is its sibling, not a degree of it: collectors matched, but
fewer than the floor, persisting past the grace window. "Expected at least 40,
seen 12" and "nothing exists" are different situations with different fixes.

A floor is always a floor. Surplus is never a finding, and the platform never
invents a count: a provider that cannot answer says so.

## Staged rollouts

**The default is the flat rebind.** Rebinding a Tier from Blueprint version 4
to version 5 is one change proposal, and every collector in the Tier picks the
new artefact up on its next poll. A Rollout is the opt-in instrument for
staging that change, never mandatory ceremony.

A **Rollout** is an authored, owned object targeting exactly one Tier, owned by
the Tier's owner. It carries a *from* binding, a *to* binding, and ordered
stages, each stage a Cohort specification plus exit criteria such as a minimum
soak and health conditions.

While a Rollout is active:

- The Tier is dual-bound, and `rendered/` holds both artefacts at head:
  `<tier>.yaml` from the *from* binding and `<tier>@next.yaml` from the *to*
  binding. Everything the server needs is at head, so no commit pinning and no
  rollout branches are involved.
- There is one active Rollout per Tier, and the Rollout file is the only door:
  a direct rebind fails render validation until the Rollout completes or
  aborts.

### Cohorts

A **Cohort** is a subset of one Tier's collector population, never a Tier
itself: a Tier is a policy position, not a rollout wave. Three specification
forms mix per stage:

- **Enumerated hosts**, naming identifying-attribute values: the three hosts
  you trust.
- **Attribute selector**, such as a region.
- **Fraction**, resolved by a stable hash over the same identifying attributes
  the Tier matches on. Widening from 5% to 50% is a strict superset, so no
  collector flaps backwards. Accepted openly: a fractional cohort is
  statistically 5%, not exactly 5%.

Membership is a pure function evaluated by the server on each connect, never
stored. The same function ships as a library so CI and the console can preview
membership against a snapshot of the estate. A preview is information for the
reviewer, never the authoritative decision.

### Advancing, halting, aborting

Stages advance by platform-proposed, human-merged change proposals. When a
stage's exit criteria are met, the platform opens the advance proposal with the
evidence in the body: soaked for this long, this many applied, this many
failed. A human merges it. The final advance completes the Rollout: the Tier
flips to single-bound *to*, the Rollout closes, and the `@next` artefact
retires.

**Halting is passive.** If criteria fail, the advance is never proposed. There
is no control loop to race, and collectors that individually broke have
already reverted themselves and report `FAILED`. Two halt conditions ship:

- A cohort member reporting `FAILED` for the *to* artefact's hash.
- Went dark after apply: reporting, took the new configuration, then silent
  within the soak window. That is the crash-loop signature that never reports
  `FAILED` at all.

The condition set is deliberately extensible, so further signals plug in as
conditions rather than as amendments to the model.

**Aborting is a proposal too.** Past the threshold, by default any cohort
failure blocks the advance and 10% failed-or-dark proposes an abort, the
platform opens a proposal reverting the Tier to single-bound *from*. Nothing
is reverted behind your back.

### Foreign collectors during a rollout

The foreign population reads everything and blocks nothing. Both artefacts are
addressable at head, and your own tooling maps them to hosts however it likes.
Cohort membership is computed the same way and crossed with the commit stamp,
then rendered as **lag, never failure**: foreign delivery timing was never the
platform's to promise.

Advance evidence counts collectors actually running the *to* artefact,
whichever path delivered it. Foreign members still on *from* are displayed and
never block. Where a delivery path cannot report status at all, the `FAILED`
signal is honestly unavailable, while the went-dark and Observed-side signals
work identically.

Reference: [ADR-0010](../adr/0010-opamp-supervisor-serving-risk.md),
[ADR-0013](../adr/0013-stateless-opamp-server-commit-stamp.md),
[ADR-0029](../adr/0029-staged-rollout-cohorts.md),
[ADR-0030](../adr/0030-bootstrap-unmatched-never-seen.md).
