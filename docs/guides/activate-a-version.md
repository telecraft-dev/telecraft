---
title: Activate a version
description: Read what changes before your estate is judged against a new Catalogue or Schema Registry version, then designate it.
order: 11
---

# Activate a version

Importing a Catalogue or a Schema Registry version installs it. It does not
put it to work. Which version your estate is judged against is a separate,
explicit decision, and this guide is that decision: read the impact report,
then designate the version.

Versions are kept side by side, so activating is reversible and the report is
cheap to compute: both versions are already on disk.

Activating changes judgement only. It moves what Blueprints are validated
against, what the Palette offers, which Stability floors bite, and what
`schema_conformance` Requirements that track head demand. It never reaches a
running collector, and it never rewrites a rendered configuration.

## What you need

An estate directory with at least one imported version. If you have none,
import one first with
[`catalogue-import`](../reference/cli.md#catalogue-import) or
[`schema-registry-import`](../reference/cli.md#schema-registry-import).

## Read the report

Run `telecraft activate` without `-confirm`. It computes the report and
changes nothing:

```sh
telecraft activate -estate ESTATE_DIR -substrate catalogue -version v0.159.0
```

```text
Catalogue v0.155.0 to v0.159.0: 1 component in use is removed and 1 entry is newly deprecated.
  processor/transform is removed. 1 Blueprint uses it: data-flow/gateway-standard (Data flow).
  processor/batch is deprecated for logs in this version. The upstream migration note is: use processor/batchprocessor. 1 Blueprint uses it: data-flow/gateway-standard (Data flow).

Nothing has changed. Run the same command with -confirm and -by <owner> to activate Catalogue v0.159.0.
```

Every line names the Blueprint the change lands on and the Team accountable
for it, so you know who to talk to before you activate rather than after.

A Catalogue report covers three things: components your Blueprints use that
the new version removes, components it newly marks deprecated, and stability
changes that take a component in use under the floor its Tier is held to. A
component nobody configures never appears: it is not your estate's news.

A Schema Registry report covers the version diff: attributes added and
removed, requirement levels tightened, and groups and attributes newly
deprecated. Which Services stop passing needs a reading of landed telemetry,
which this command takes none of, so it says so:

```text
No estate reading was taken, so this report does not say which Services stop passing.
```

The console shows both halves, because it has the readings. See
[Read it in the console](#read-it-in-the-console).

## Designate the version

When you are content with what the report says, run the same command again
with `-confirm` and the Owner deciding it:

```sh
telecraft activate -estate ESTATE_DIR -substrate catalogue -version v0.159.0 \
  -confirm -by platform-observability
```

That writes `activations.yaml` in the estate directory, carrying the active
version, who decided it, when, and the report the decision was taken on:

```yaml
catalogue:
  active: v0.159.0
  activations:
    - version: v0.155.0
      at: 2026-06-02T09:15:00Z
      by: platform-observability
      impact:
        summary: 'Catalogue v0.155.0: nothing in this estate is affected.'
    - version: v0.159.0
      previous: v0.155.0
      at: 2026-07-14T11:30:00Z
      by: platform-observability
      impact:
        summary: 'Catalogue v0.155.0 to v0.159.0: 1 component in use is removed and 1 entry is newly deprecated.'
        lines:
          - 'processor/transform is removed. 1 Blueprint uses it: data-flow/gateway-standard (Data flow).'
```

Commit it. The pull request is the audit: it carries what changed, who decided
it, and what they read before deciding.

## Check it took

Render or check the estate and watch the new version bite:

```sh
telecraft render -estate ESTATE_DIR -commit $(git -C ESTATE_DIR rev-parse HEAD)
```

A component the new version removed now fails to resolve, and a floor the new
stability crosses now raises a finding. Both are what the report told you to
expect, which is the point of reading it first.

To go back, activate the older version the same way. It is still installed:
versions are kept, never replaced.

## Read it in the console

The Catalogue &amp; Governance Workspace has an Activation view. It shows, for
both the Catalogue and the Schema Registry, the version your estate is judged
against, every installed version with the report activating it would be
decided on, and every activation so far.

The Schema Registry report there carries the estate half as well, because the
console has already read the telemetry: it names the Services that stop
passing and the attribute they fail on.

The control to activate is offered to operators, which means somebody in a
Team at the top of your team tree. Activating changes judgement for the whole
Estate, and no Team below the top answers for all of it. Everyone else sees
the same reports and no button.

## Where the active version is not used

Evaluating a collector does not consult the active version. A collector is
judged against the Catalogue for the version it actually runs, because
Telecraft does not control collector binaries. Where the version a collector
runs has no imported Catalogue, the nearest older one judges it and says the
judgement is degraded; where there is nothing older, nothing is known and the
fix is to import that version. See
[the Catalogue reference](../reference/catalogue.md#versions-and-activation).

Requirements that pin a Schema Registry version are the same story from the
other side. Activating a registry version moves nothing for a requirement that
pinned a different one: pinning exists so the bar does not move under a
Service overnight. Only a requirement with `track: head` follows the version
you activate.
