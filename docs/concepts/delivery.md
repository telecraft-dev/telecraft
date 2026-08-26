---
title: Delivery
description: Served and foreign collectors, the stateless OpAMP server, rendered artefacts and the commit stamp, the Unmatched artefact, and staged rollouts.
order: 6
---

# Delivery

Rendering puts an artefact in git. Delivery is how it reaches a collector.
Telecraft treats delivery as a choice you make per collector, not a condition
of using anything else.

## Two delivery paths, both legitimate

A collector's **delivery path** is a visible property of the collector, and
there are two.

**Served** means the collector receives its configuration from Telecraft's
OpAMP server. Serving requires the upstream OpAMP Supervisor beside every
served collector, and the Supervisor takes over the collector process. So
adopting serving is a migration, not an addition.

**Foreign** means anything else delivers the configuration: a GitOps
controller, configuration management, a cloud agent, or a person with an
editor. Foreign is legitimate, not lesser. Telecraft serves everything, and
GitOps is an alternative rather than a fallback. Mixed substrates are the
default mode.

Both paths get identical verdicts. Continuous evaluation judges every
Effective configuration by the same rules, so a foreign configuration using a
non-allowed component receives the same finding a composed one would, routed
to the same owner. Telecraft cannot block a foreign change, but it still
reports it.

The two paths differ in remedies, which is why Telecraft shows the path. A
drifted served collector and a drifted foreign collector need different
people.

## Rendered artefacts and the commit stamp

The renderer exports exactly one artefact per Tier: plain otelcol YAML at a
stable repository path, `rendered/<team>/<tier>.yaml`. Where a Tier is served,
a small supervisor configuration renders beside it at
`rendered/<team>/<tier>.supervisor.yaml`.

Rendering is deterministic, so CI can recompute the whole `rendered/` tree and
fail on any mismatch. Nobody commits into it by hand.

**Every artefact carries its own identity.** The renderer stamps the commit SHA
into the collector's own telemetry resource as `telecraft.commit`, next to
`telecraft.tier`, which names the Tier that produced it. The collector reports
both back, so Telecraft reads "which commit is this running" *from* the
collector rather than remembering it *about* the collector.

The stamp works the same on both delivery paths: a foreign collector reports
the same stamp, so it gets the same identity and the same claim evaluation.

The stamp rides on the collector's self-telemetry resource only. Telecraft
never writes anything into your telemetry data.

### Renderer hard rules

The renderer generates three things automatically rather than leaving them to
guidance, because each one fails silently when a person forgets it:

- Additional OpAMP extensions render as `opamp/<name>`, never a bare `opamp`
  block, which would silently override the Supervisor's injected endpoint.
- A node-unique identifying attribute arrives through Downward API
  indirection, so one daemonset manifest yields per-node identity.
- Data crossing an untrusted Hop has Telecraft's attribute namespace stripped,
  and identity is re-derived from the receiving Tier's own stamps rather than
  trusted from inbound data.

## The stateless OpAMP server

A collector connects and reports its identifying attributes. The server
matches those attributes against the Tier selectors held in git at head,
serves the rendered artefact at that path, and remembers nothing.

The serving path holds at most three things, all rebuildable and none
durable:

1. The repository snapshot: the fetched estate at last-known head, plus the
   selector index compiled from it, refreshed by poll. The fetch interval is
   the bounded staleness, and it is the only freshness setting.
2. A per-connection digest of each connected collector's last-reported
   effective configuration, in process memory, which dies with the
   connection.
3. Nothing else.

Artefact choice is a pure function of head and the reported attributes. That
is why several replicas behind a load balancer are independent read-only
clones that need no coordination, and why a restart is a non-event.

Removing the server loses delivery, never the record.

Two rules cover first boot:

- **The server never serves an empty configuration map.** A Supervisor handed
  an empty map reports applied and healthy while running nothing, which is the
  worst possible failure: silent success.
- **A collector matching no selector still gets an artefact.** See the
  following section.

## The Unmatched artefact

A served collector that matches no Tier selector receives the **Unmatched
artefact**: a distinguished rendered configuration owned by the root Team,
commit stamped, with self-telemetry on, no data pipelines, and never empty.

This is the one case where Telecraft is talking to an ungoverned thing, and
it puts that to use. Instead of a silent `nop` pipeline, the collector becomes
as visible as possible: stamped, reporting health, and labelled as governed by
nobody. So the onboarding prompt can say something useful: alive, on this
node, running this version, connected since this morning.

Not knowing is a rendered, visible state rather than an absence.

The Unmatched artefact is not the quarantine destination. Quarantine is a
data-level routing pattern and lives in [governance](governance.md#ungoverned).

## Population and `never_seen`

The mirror image of an unmatched collector is a Tier whose selector has
matched nothing.

`never_seen` is a finding class attached to the Tier, never an eighth
conformance outcome. Outcomes are per-requirement crosses that need an
Effective or Observed reading, and a Tier with no collectors has neither. The
selector *is* the expectation, so the finding is the selector's.

Without a floor it is neutral. A freshly authored Tier waiting for its
workload is an ordinary state: visible, counted in no compliance ratio,
and never red. Its age doubles as the stale-configuration
signal: "never matched in 90 days".

A **Population floor** gives it teeth. The floor is the minimum instance count
a Tier's selector should match, either derived live from the substrate through
the `InventoryProvider` seam or declared in the Tier file. Absent means no
teeth at all. With a floor above zero and a zero that persists past the grace
window, `never_seen` escalates to violation grade.

`under_populated` is its sibling, not a degree of it: collectors matched, but
fewer than the floor, persisting past the grace window. "Expected at least 40,
seen 12" and "nothing exists" are different situations with different fixes.

A floor is always a floor. Surplus is never a finding, and Telecraft never
invents a count: a provider that cannot answer says so.

## Staged rollouts

**The default is the flat rebind.** Rebinding a Tier from Blueprint version 4
to version 5 is one change proposal, and every collector in the Tier picks up
the new artefact on its next poll. A Rollout is the optional instrument for
staging that change.

A **Rollout** is an authored, owned object that targets exactly one Tier and
is owned by the Tier's owner. It carries a *from* binding, a *to* binding, and
ordered stages. Each stage is a Cohort specification plus exit criteria such
as a minimum soak and health conditions.

While a Rollout is active:

- The Tier is dual-bound, and `rendered/` holds both artefacts at head:
  `<tier>.yaml` from the *from* binding and `<tier>@next.yaml` from the *to*
  binding. Everything the server needs is at head, so there is no commit
  pinning and no rollout branch.
- There is one active Rollout per Tier, and the Rollout file is the only way
  to change the binding: a direct rebind fails render validation until the
  Rollout completes or aborts.

### Cohorts

A **Cohort** is a subset of one Tier's collector population, never a Tier
itself: a Tier is a policy position, not a rollout wave. Three specification
forms can mix within a stage:

- **Enumerated hosts**, naming identifying-attribute values: the three hosts
  you trust.
- **Attribute selector**, such as a region.
- **Fraction**, resolved by a stable hash over the same identifying attributes
  the Tier matches on. Widening from 5% to 50% is a strict superset, so no
  collector flaps backwards. A fractional Cohort is statistically 5%, not
  exactly 5%.

Membership is a pure function the server evaluates on each connect, never
stored. The same function ships as a library, so CI and the console can
preview membership against a snapshot of the estate. A preview is information
for the reviewer, never the authoritative decision.

### Advancing, halting, aborting

Stages advance by change proposals that Telecraft proposes and a person
merges. When a stage's exit criteria are met, Telecraft opens the advance
proposal with the evidence in the body: soaked for this long, this many
applied, this many failed. A person merges it. The final advance completes the
Rollout: the Tier flips to single-bound *to*, the Rollout closes, and the
`@next` artefact retires.

**Halting is passive.** If criteria fail, the advance is never proposed. There
is no control loop to race, and collectors that individually broke have
already reverted themselves and report `FAILED`. Two halt conditions ship:

- A Cohort member reporting `FAILED` for the *to* artefact's hash.
- Went dark after apply: the collector was reporting, took the new
  configuration, then fell silent within the soak window. That is the
  crash-loop signature, and it never reports `FAILED` at all.

The condition set is extensible, so further signals plug in as conditions
rather than as changes to the model.

**Aborting is a proposal too.** By default, any Cohort failure blocks the
advance, and 10% failed-or-dark proposes an abort: Telecraft opens a proposal
that reverts the Tier to single-bound *from*. Nothing is reverted behind your
back.

### Foreign collectors during a rollout

The foreign population reads everything and blocks nothing. Both artefacts are
addressable at head, and your own tooling maps them to hosts however it likes.
Cohort membership is computed the same way and crossed with the commit stamp,
then rendered as **lag, never failure**: foreign delivery timing was never
Telecraft's to promise.

Advance evidence counts collectors actually running the *to* artefact,
whichever path delivered it. Foreign members still on *from* are displayed and
never block. Where a delivery path cannot report status at all, the `FAILED`
signal is unavailable, while the went-dark and Observed-side signals work
identically.
