---
title: Estate layout
description: The estate repository layout: root files, team directories, the generated rendered tree and CODEOWNERS.
order: 3
---

# Estate layout

An estate repository holds the authored objects that describe your collection
topology, and the artefacts the renderer generates from them. This page is the
map: which paths humans write, which paths the platform writes, and how a
file's place in the tree derives the id every reference uses.

The design decisions behind the layout are ADR-0018 (one estate repository,
path per team) and ADR-0027 (the directory design and satellite repositories).

## The tree

```
teams.yaml                            # the Team-tree seam
telemetry.yaml                        # the self-telemetry destination
allow-lists.yaml                      # optional: every team's declared list
grants.yaml                           # optional: every Grant
users.yaml                            # optional: the sign-in seam
teams/
  <team-id>/
    components/<name>.yaml            # shared Components
    blueprints/<name>.yaml            # Blueprints, local Components inline
    tiers/<name>.yaml                 # Tiers
    services/<name>.yaml              # Services
    rollouts/<name>.yaml              # Rollouts
rendered/
  <team-id>/<tier>.yaml               # generated: one artefact per Tier
  <team-id>/<tier>.supervisor.yaml    # generated: served Tiers only
  <team-id>/<tier>@next.yaml          # generated: Tiers under an active Rollout
  _estate/unmatched.yaml              # generated: the Unmatched artefact
CODEOWNERS                            # generated: the code-ownership projection
```

Team directories are flat. The hierarchy lives only in `teams.yaml`, so
reparenting a team rewrites no path and breaks no id.

## Root files

`teams.yaml`
: The Team tree. Required by every command that resolves ownership. Teams
  nest under a `teams:` key; each node carries `id`, an optional `name`, an
  optional `owners` list, and an optional nested `teams` list. A team id
  appearing twice is a load error, because a Team has at most one parent. An
  Owner named under two teams is a load error, because an Owner belongs to
  exactly one Team.

`telemetry.yaml`
: The estate-level self-telemetry destination, under a `self_telemetry:` key.
  Required: the renderer refuses an estate that declares nowhere for mandatory
  self-telemetry to go. Fields are `endpoint` (required), `protocol`
  (`grpc` or `http/protobuf`, default `http/protobuf`), `environments`
  (a map from Environment name to an overriding endpoint), and
  `new_pipeline_telemetry` (boolean, default `false`).

`allow-lists.yaml`, `grants.yaml`
: The Allow-list policy. Both optional; an absent file is the default posture.
  See [Allow-lists and Grants](allow-lists.md).

`users.yaml`
: The sign-in seam: the mapping from authenticated identities to the Owner
  each acts as. Each entry carries `email`, `name`, `owner` and an optional
  `password` hash produced by `telecraft passwd`.

```yaml
# teams.yaml
teams:
  - id: engineering
    name: Engineering
    teams:
      - id: platform
        name: Platform Engineering
        owners: [platform-observability]
        teams:
          - id: data-flow
            name: Data Flow
            owners: [gateway-owners]
```

```yaml
# telemetry.yaml
self_telemetry:
  endpoint: https://otlp.observability.internal:4318
  environments:
    production: https://otlp-prod.observability.internal:4318
```

## Team directories

Each subdirectory of `teams/<team-id>/` holds one kind of authored object, one
object per file:

| Directory | Object | Id it derives | Documented in |
|---|---|---|---|
| `components/` | shared Component | `<team-id>/<name>` | [Blueprints](blueprints.md) |
| `blueprints/` | Blueprint | `<team-id>/<name>` | [Blueprints](blueprints.md) |
| `tiers/` | Tier | `<team-id>/<name>` | [Tiers](tiers.md) |
| `services/` | Service | `<team-id>/<name>` | [Tiers](tiers.md) |
| `rollouts/` | Rollout | `<team-id>/<name>` | [Tiers](tiers.md) |

Rules the loaders apply to every one of them:

- The file's place in the layout derives the id. `teams/infosec/components/pii-redaction.yaml`
  is the shared Component `infosec/pii-redaction`.
- Only `*.yaml` and `*.yml` files directly inside the directory are read.
  Subdirectories are ignored.
- A file holds exactly one object, as a YAML mapping. A list, a second YAML
  document or an empty file is a load error.
- Unknown fields are rejected. A misspelled key fails the load naming the file
  and the field.
- The layout, not the body, is the id convention. A `name:` in the body must
  match the filename when it's present; Blueprints and shared Components
  require it, while Tiers, Services and Rollouts take the filename when it's
  absent.
- A team directory name containing `/`, `@` or whitespace is a load error:
  those characters are reserved separators inside references.
- A missing directory is simply empty. A team need not author every kind.

A team directory may hold other files; the loaders read only the five
directories above.

## The rendered tree

`rendered/` is written by `telecraft render`. Humans never commit into it: CI
recomputes the tree from the authored sources and fails on a mismatch.

| Path | When it exists | What it holds |
|---|---|---|
| `rendered/<team>/<tier>.yaml` | one per Tier, always | The Tier's plain otelcol config, commit-stamped. |
| `rendered/<team>/<tier>.supervisor.yaml` | the Tier declares `serving` | The OpAMP Supervisor config for that Tier. |
| `rendered/<team>/<tier>@next.yaml` | a Rollout targets the Tier | The candidate render under the Rollout's `to` binding. |
| `rendered/_estate/unmatched.yaml` | always | The Unmatched artefact, served to a collector matching no Tier selector. |

The `@next.yaml` artefact exists exactly while the Rollout does, and is
retired with it. See [Tiers](tiers.md) for dual binding.

Every artefact opens with a generated header naming what it is and the commit
SHA it was rendered at, and stamps `telecraft.commit` onto the collector's
self-telemetry resource. A Tier's artefact also stamps `telecraft.tier` with
the Tier's team-qualified id; the Unmatched artefact stamps
`telecraft.unmatched: true` instead. These attributes ride the collector's
self-telemetry resource only, never customer data.

## Generated CODEOWNERS

`CODEOWNERS` at the repository root is a projection of `teams.yaml` and the
objects' `owner:` fields, written in the forge's dialect. It's a cache: losing
it loses nothing, because the tree is the source.

For each team with at least one owner on its chain, two lines are written:

```
/teams/<team-id>/ @owner @ancestor-owner
/rendered/<team-id>/ @owner @ancestor-owner
```

Handles are the team's own owners first, then each ancestor's owners, nearest
ancestor first, deduplicated. Review reach derives from the team tree, not
from the directory shape, so an ancestor keeps its say over a team nested
several levels below it. A team whose whole chain has no owners gets no line
at all, which leaves the path to the repository default.

When the root team or teams carry owners, one further line is written:

```
/rendered/_estate/ @root-owner
```

## Authored versus generated

| Path | Written by |
|---|---|
| `teams.yaml`, `telemetry.yaml`, `allow-lists.yaml`, `grants.yaml`, `users.yaml` | you |
| `teams/**` | you |
| `rendered/**` | `telecraft render` |
| `CODEOWNERS` | `telecraft render` |

Catalogue artefacts are never estate repository content. They're instance-side
artefacts with their own import pipeline; see [Catalogue](catalogue.md).

## The ownership directory

`telecraft check -ownership` takes a directory with a different shape from the
estate root: `teams.yaml` plus flat authored-object files beside it, each
holding one object or a list of objects with `kind`, `id` and `owner` fields.
`allow-lists.yaml`, `grants.yaml` and `users.yaml` are skipped, so one
directory can carry the whole authored set.

`kind` is one of `component`, `blueprint`, `tier`, `hop`, `path`, `service`,
`requirement` or `exemption`. `collector` is rejected: a collector is derived,
never authored, and inherits its owner from the Tier it matched into. An
object with no owner, or an owner the team tree doesn't know, is a load error,
because a finding routed to it would reach nobody.

## Source sets and satellite repositories

An estate is a set of repositories, each mapped to exactly one Team subtree,
with the single-repository case the ordinary one. Every loader that walks
`teams/` accepts several roots, and `blueprint-check` takes them as positional
arguments.

A satellite repository has an internal layout identical to a monorepo subtree,
so promotion back into the primary repository is a mechanical move.
Governance never moves out: a satellite team appears in the primary
`teams.yaml` like everyone else, and Grants targeting the subtree live with
their ancestors in the primary repository. References run satellite to primary
only, never primary to satellite and never satellite to satellite.
