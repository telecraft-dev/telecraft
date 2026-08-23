---
title: Allow-list and Grant file formats
description: The allow-lists.yaml and grants.yaml formats, entry patterns, and the rules that resolve a team's effective palette.
order: 7
---

# Allow-list and Grant file formats

The Catalogue lists what exists. An Allow-list lists what a Team can use.
Together with Grants, the two files answer one question: can team `T` use
component `X`?

Both files sit at the estate root beside `teams.yaml`, and both are optional.
Without them, every team can use the whole active Catalogue.

```
allow-lists.yaml    # every team's declared list
grants.yaml         # every Grant
```

To print a team's answer, run `telecraft palette`:

```sh
telecraft palette -team data-flow -estate ESTATE_DIR -catalogue ARTEFACT
```

## allow-lists.yaml

One document with a single `allow_lists` key holding a list.

| Field | Type | Required | Description |
|---|---|---|---|
| `team` | string | yes | The Team this list applies to. Must be in the team tree. |
| `owner` | string | yes | The accountable party. Must be in the team tree. |
| `allow` | list of strings | yes | Entries, at least one. |

```yaml
allow_lists:
  - team: platform
    owner: platform-observability
    allow:
      - receiver/otlp
      - processor/*
      - exporter/otlphttp
      - extension/health_check
  - team: data-flow
    owner: gateway-owners
    allow:
      - receiver/otlp
      - processor/memory_limiter
      - processor/batch
      - exporter/otlphttp
```

Each Team can declare at most one list. A second list for the same Team is a
load error.

## grants.yaml

One document with a single `grants` key holding a list.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique. `telecraft palette` reports this id as the reason a granted component is allowed. |
| `owner` | string | yes | The author. Their Team must be a proper ancestor of `team`. |
| `team` | string | yes | The target: the Team, and its subtree, that this Grant widens. |
| `adds` | list of strings | yes | Entries, at least one. |

```yaml
grants:
  - id: gateway-kafka-2026-q3
    owner: platform-observability
    team: data-flow
    adds:
      - exporter/kafka
```

A parent team writes a Grant. The author is the owner's Team, and it must sit
strictly above the target. A Team can't grant to itself, because a Team can
only ever narrow its own palette.

## Entry syntax

Each entry is a pattern of the form `class/type-pattern`. It selects Catalogue
components by their `(class, type)` key.

The class side is exact and must be one of `receiver`, `processor`,
`exporter`, `connector`, or `extension`.

The type side is a pattern with a small vocabulary:

| Token | Matches |
|---|---|
| `*` | Any run of characters, including none. |
| `?` | Exactly one character. |
| anything else | Itself. |

| Entry | Selects |
|---|---|
| `receiver/otlp` | One component. |
| `exporter/kafka*` | A family. |
| `processor/*` | A whole class. |

Character classes and escapes are rejected: an entry containing `[`, `]`, `\`,
or a second `/` is a load error.

Telecraft tries a pattern against a component's canonical type and against its
`deprecated_type` alias, so an entry written against a historical name keeps
selecting the same component.

## Resolution

Telecraft computes a team's effective palette by walking the team chain from
the root down to the team. At each team in the chain, in this order:

1. If that team declares a list, intersect: a component the list doesn't match
   is removed.
2. Then apply every Grant targeting that team: a component a Grant matches is
   added back.

Two consequences follow from that order.

A Grant overrides its own target's declared list
: The Grant applies after the intersection at the same team, so a Grant
  targeting team `T` re-admits a component that `T`'s own list excludes.

Lists below the target still narrow it away
: The walk continues downward, and a descendant's list runs after the Grant.
  A Grant widens the palette from its target downward, and a list lower in the
  chain can narrow it again.

If no team on the chain declares a list, the effective palette is the whole
active Catalogue.

## Provenance

Every entry in an effective palette carries the reason it's there. `telecraft
palette` prints it in the last column.

| Origin | Means |
|---|---|
| `default-allow` | No team on the chain declares an Allow-list. |
| `allow-list` | The component survived every declared list on the chain. |
| `grant` | A named Grant admitted it; the lists alone would exclude it. |

A `grant` entry also names the Grant id, the granting team (the Grant owner's
team), and the target team. Every component a team can use traces back to the
declared lists, the default, or a named Grant.

## Enforcement

The allow-list check is the one policy rule that blocks a render. If a
Blueprint uses a catalogue type outside the owning team's effective palette,
`telecraft render` refuses. There's no override: to add the component, ask
for a Grant.

Everything else the render notices, such as a stability floor breach or a
binding pinned off head, is a finding routed to an owner. Findings never
block.

## Load errors

Loading fails closed and returns nothing. Each message names the file. The
load refuses on:

- an unknown or misspelled field in either file
- a malformed document, an empty file, or more than one YAML document in the
  file
- a present `allow-lists.yaml` holding no `allow_lists`, or a present
  `grants.yaml` holding no `grants`: declare one or delete the file
- an allow-list with an empty `allow` list. To inherit the parent's effective
  list unchanged, declare no list at all. An empty list isn't supported as a
  way to ban everything.
- a Grant with an empty `adds` list: a Grant exists to widen a palette
- a missing `team` or `owner`, or one the team tree doesn't know
- a Grant with no `id`, or an `id` defined twice
- one Team declaring two allow-lists
- a Grant whose owner's team isn't a proper ancestor of the target team
- a duplicated entry in one list
- a malformed entry: not `class/type-pattern`, an unknown class, or a pattern
  containing a rejected character
- an entry that selects nothing in the active Catalogue. This usually means an
  unknown component type or a typo in the pattern, so it fails the load rather
  than silently allowing nothing.

The last rule ties a loaded policy to the Catalogue version it was validated
against. If you activate a different Catalogue, Telecraft validates the policy
again, so an entry can't silently match nothing.
