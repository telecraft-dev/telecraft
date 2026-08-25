---
title: Author and render
description: Compose a Blueprint from governed Components, check a team's palette, and render otelcol YAML into git.
order: 4
---

# Author and render

Authoring is the rung where teams stop copying collector configuration from
each other. You compose Blueprints from owned, versioned Components, bind one
to a Tier, and render plain otelcol YAML into git. Nothing in your delivery
path changes: the rendered artefacts are ordinary files, and how they reach
collectors is a separate decision.

This guide edits a copy of the demo estate. Make one, so you can throw it
away:

```sh
cp -R ../estate-demo ../my-estate
rm -rf ../my-estate/.git
```

## The estate layout

The layout is the id convention: a file's place gives it its name, and one
file holds one authored object.

```
teams.yaml                     the team tree
allow-lists.yaml               declared Allow-lists
grants.yaml                    ancestor-authored exceptions
telemetry.yaml                 the self-telemetry destination
teams/<team>/tiers/*.yaml      Tiers: the rendering and binding unit
teams/<team>/blueprints/*.yaml Blueprints: per-signal lanes of Components
teams/<team>/components/*.yaml shared Components, referenced never copied
teams/<team>/services/*.yaml   Services and the Paths that set strictness
teams/<team>/rollouts/*.yaml   the staged rebind instrument
requirements/*.yaml            the requirements library
exemptions/*.yaml              authored waivers
catalogues/catalogue-*.json    installed Catalogues, retained side by side
rendered/                      generated, and humans never commit here by hand
CODEOWNERS                     generated from the team tree
```

## Check what a team may use

A Blueprint can only reference Components whose Catalogue entry is inside the
authoring team's effective palette. That palette is the Catalogue, narrowed by
every Allow-list from the root of the team tree down, plus any Grant an
ancestor authored. `telecraft palette` prints it, with the provenance of every
entry.

```sh
./telecraft palette \
  -team storefront \
  -estate ../my-estate \
  -catalogue ../my-estate/catalogues/catalogue-v0.158.0.json
```

```
team       storefront
catalogue  v0.158.0
allowed    7 of 268 components

exporter/otlp_http        allow-list
extension/health_check    allow-list
extension/opamp           allow-list
processor/batch           allow-list
processor/filter          allow-list
processor/memory_limiter  allow-list
receiver/otlp             allow-list
```

Narrowing only runs down the tree. Widening happens through a Grant, and the
palette says which Grant admitted an entry and who authored it:

```sh
./telecraft palette -team data-flow -estate ../my-estate \
  -catalogue ../my-estate/catalogues/catalogue-v0.158.0.json
```

```
team       data-flow
catalogue  v0.158.0
allowed    10 of 268 components

exporter/kafka            grant kafka-egress-for-data-flow (granted by platform to data-flow)
exporter/otlp_http        allow-list
extension/health_check    allow-list
extension/opamp           allow-list
processor/batch           allow-list
processor/memory_limiter  allow-list
processor/tail_sampling   allow-list
processor/transform       allow-list
receiver/kafka            allow-list
receiver/otlp             allow-list
```

A team with no declared list inherits its parent's unchanged. The root Team
sees the whole Catalogue, marked `default-allow`.

## Author a Blueprint

A Blueprint is a named, integer-versioned composition of Components, written
as per-signal lanes plus collector-wide extensions. Lane order is pipeline
order: there is no separate phase concept.

Write `../my-estate/teams/storefront/blueprints/web-edge.yaml`:

```yaml
name: web-edge
version: 1
owner: storefront-team
satisfies:
  - traces-delivered@2
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
    config:
      protocols:
        http:
          endpoint: 0.0.0.0:4318
  - name: guard
    class: processor
    type: memory_limiter
    version: 1
    config:
      check_interval: 1s
      limit_percentage: 70
      spike_limit_percentage: 20
  - name: batcher
    class: processor
    type: batch
    version: 1
    config:
      send_batch_size: 2048
      timeout: 5s
  - name: health
    class: extension
    type: health_check
    version: 1
    config:
      endpoint: 0.0.0.0:13133
pipelines:
  traces:
    - component: otlp-in
    - component: guard
    - component: batcher
    - component: data-flow/gateway-exporter@3
extensions:
  - component: health
```

Two kinds of Component appear here. `otlp-in`, `guard`, `batcher`, and
`health` are local: declared inline, owned by this Blueprint's owner, and not
referenceable from anywhere else. `data-flow/gateway-exporter@3` is shared: it
lives in another team's `components/` directory, and the Blueprint references
it rather than copying it. The `@3` pins a version, which is the default; a
reference that tracks head is opt-in.

`satisfies` is a claim of intent, never of fact. It says which Requirement
versions this Blueprint means to meet, and the evaluators judge whether it
does.

## Author a Tier

A Tier is a position in the collection topology and the rendering unit: one
Tier renders one artefact. It declares exactly one Environment and binds
exactly one Blueprint version.

Write `../my-estate/teams/storefront/tiers/web-edge.yaml`:

```yaml
owner: storefront-team
environment: production
blueprint: storefront/web-edge@1
selector:
  telecraft.tier: web-edge
  deployment.environment: production
min_expected: 2
hops:
  - from: internet
```

The parts that matter:

- `selector` matches collectors into the Tier. You never author a collector:
  one connects, reports its identifying attributes, and Telecraft matches it
  into the Tier whose selector its attributes satisfy. Matching is equality
  over every authored pair, and the most specific satisfied selector wins.
- `min_expected` is the Population floor: at least this many collectors should
  match. It is a floor, never an equality, so a surplus is never a finding.
- `hops` are the arrivals into this Tier. Trust belongs to the Hop, not the
  Tier, and an undeclared trust level fails safe.

## Render

To render every Tier in the estate, run:

```sh
./telecraft render \
  -estate ../my-estate \
  -catalogue ../my-estate/catalogues/catalogue-v0.158.0.json \
  -commit 0000000000000000000000000000000000000000 \
  -out /tmp/rendered
```

`-commit` is the SHA every artefact stamps itself with. On an instance, the
pull-request bot and the CI recompute both pass the commit being rendered; the
demo estate's own pipeline passes `$GITHUB_SHA`. Omit `-out` to write in
place, over the estate's own `rendered/` tree.

Every Tier renders, not only the one you added:

```
wrote /tmp/rendered/CODEOWNERS
wrote /tmp/rendered/rendered/_estate/unmatched.yaml
wrote /tmp/rendered/rendered/data-flow/gateway-staging.supervisor.yaml
wrote /tmp/rendered/rendered/data-flow/gateway-staging.yaml
wrote /tmp/rendered/rendered/data-flow/gateway.supervisor.yaml
wrote /tmp/rendered/rendered/data-flow/gateway.yaml
wrote /tmp/rendered/rendered/data-flow/kafka-bridge.supervisor.yaml
wrote /tmp/rendered/rendered/data-flow/kafka-bridge.yaml
wrote /tmp/rendered/rendered/data-flow/kafka-bridge@next.yaml
wrote /tmp/rendered/rendered/edge-ops/edge-arm.yaml
wrote /tmp/rendered/rendered/edge-ops/edge.yaml
wrote /tmp/rendered/rendered/storefront/mobile-edge.supervisor.yaml
wrote /tmp/rendered/rendered/storefront/mobile-edge.yaml
wrote /tmp/rendered/rendered/storefront/web-edge.yaml
```

The rendered artefact is plain otelcol YAML:

```yaml
# Generated by the telecraft renderer. Do not edit by hand: change the source in git and render again.
# Tier storefront/web-edge (production), Blueprint storefront/web-edge@1, commit 0000000000000000000000000000000000000000.
receivers:
  otlp/otlp-in:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
processors:
  # Generated: this Tier receives data over an untrusted Hop, so this
  # processor drops every telecraft.* attribute on arrival. Identity
  # comes from this Tier's own stamps, never from inbound data.
  attributes/telecraft.untrusted-hop:
    actions:
      - action: delete
        pattern: ^telecraft\.
  batch/batcher:
    send_batch_size: 2048
    timeout: 5s
  memory_limiter/guard:
    check_interval: 1s
    limit_percentage: 70
    spike_limit_percentage: 20
exporters:
  otlp_http/data-flow.gateway-exporter:
    compression: gzip
    endpoint: https://ingest.observability.internal:4318
    retry_on_failure:
      enabled: true
      max_elapsed_time: 300s
    sending_queue:
      enabled: true
      queue_size: 10000
extensions:
  health_check/health:
    endpoint: 0.0.0.0:13133
service:
  extensions:
    - health_check/health
  pipelines:
    traces:
      receivers:
        - otlp/otlp-in
      processors:
        - attributes/telecraft.untrusted-hop
        - memory_limiter/guard
        - batch/batcher
      exporters:
        - otlp_http/data-flow.gateway-exporter
  telemetry:
    resource:
      k8s.node.name: ${env:TELECRAFT_NODE_NAME}
      telecraft.commit: "0000000000000000000000000000000000000000"
      telecraft.tier: storefront/web-edge
    metrics:
      level: normal
      readers:
        - periodic:
            exporter:
              otlp:
                endpoint: https://otlp-prod.observability.internal:4318
                protocol: http/protobuf
```

The generated explanatory comments and the matching `logs` self-telemetry
block are cut here for length. Three things in that output are not in your
Blueprint:

1. The `attributes/telecraft.untrusted-hop` processor. The Tier declares an
   arrival from `internet` with no trust level, so inbound data sheds
   Telecraft's attribute namespace before anything downstream reads it.
2. The commit stamp and the Tier id under `service.telemetry.resource`. The
   artefact carries its own identity, which is how a reading later joins back
   to the artefact that produced it.
3. The self-telemetry destination from the estate's `telemetry.yaml`. Internal
   metrics and logs go over their own exporter and connection, never through
   this Tier's data pipelines.

## What lands in `rendered/`

```
rendered/_estate/unmatched.yaml         the Unmatched artefact, root-team owned
rendered/<team>/<tier>.yaml             one collector artefact per Tier
rendered/<team>/<tier>.supervisor.yaml  only for Tiers with a serving block
rendered/<team>/<tier>@next.yaml        only while a Rollout is active
CODEOWNERS                              projected from the team tree
```

The new `storefront/web-edge` Tier declares no `serving:` block, so no
supervisor artefact renders beside it. That Tier is git-delivered;
[serve configurations](serve-configs.md) covers the other choice.

`CODEOWNERS` is a projection of the team tree, never a source. You edit
`teams.yaml` and the `owner` field on each object:

```
/teams/storefront/ @storefront-team @product-platform @engineering-lead
/rendered/storefront/ @storefront-team @product-platform @engineering-lead
```

That projection is what stops anyone granting themselves an Exemption: the
waived Requirement's owner is a required reviewer on the file that waives it.

## What the renderer refuses

The Allow-list is the only rule that hard-blocks, and it blocks at render.
Add a Kafka receiver to the Storefront Blueprint, which Storefront's list does
not include:

```
render: render refused:
  - tier "storefront/web-edge": Component bus-in uses receiver/kafka, which team "storefront"'s Allow-list does not include. Ask for a Grant to add it.
```

The command exits 1 and writes nothing. The fix is a Grant from an ancestor
team, which is one reviewable file.

`render` exits 0 when it rendered, 1 when it refused (a mechanical invalidity
or the Allow-list hard block), and 2 on a usage or load error.

A stability-floor breach is a finding, not a block. The render completes and
reports it on stdout, after the `wrote` lines:

```
finding floor tier=storefront/mobile-edge lane=metrics: routes metrics through debug-counters (processor/filter), which is alpha for metrics, below the beta floor for Service Class C2 in production. The floor comes from storefront/catalogue-web.
```

## Keep `rendered/` honest

Rendering is a deterministic function of the authored trees, so the committed
`rendered/` tree must equal a fresh render of its sources. Check that in CI:
recompute at the commit the committed artefacts already stamp themselves
with, then diff:

```sh
stamped=$(grep -oE '[0-9a-f]{40}' rendered/_estate/unmatched.yaml | head -1)
go run ./cmd/telecraft render \
  -estate . \
  -catalogue catalogues/catalogue-v0.158.0.json \
  -commit "$stamped" \
  -out "$RUNNER_TEMP/recompute"
diff -r "$RUNNER_TEMP/recompute/rendered" rendered
diff "$RUNNER_TEMP/recompute/CODEOWNERS" CODEOWNERS
```

The indirection is necessary: every artefact carries the SHA that rendered
it, and no commit can carry its own. Recomputing at the stamp the tree already
claims checks the exact invariant. Any difference means the sources moved and
the tree did not.

## What next

- [Serve configurations](serve-configs.md) delivers these artefacts to
  collectors over OpAMP.
- [Stage a Rollout](stage-a-rollout.md) moves a Tier onto a new Blueprint
  version in cohorts rather than all at once.
- [Check conformance](check-conformance.md) judges whether the configuration
  worked.
