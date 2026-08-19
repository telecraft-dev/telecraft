---
title: Allow-list and Grant file formats
description: The allow-lists.yaml and grants.yaml formats, entry patterns, and the rules that resolve a team's effective palette.
order: 7
---

# Allow-list and Grant file formats

The Catalogue states what exists. An Allow-list states what a Team may use.
Together with Grants they answer one question: may team `T` use component `X`?

Both files sit at the estate root beside `teams.yaml`, and both are optional.
An absent file is the default posture: the whole active Catalogue.

```
allow-lists.yaml    # every team's declared list
grants.yaml         # every Grant
```

The decision behind the policy is ADR-0021.

Print a team's answer with `telecraft palette`:

```sh
telecraft palette -team data-flow -estate ESTATE_DIR -catalogue ARTEFACT
```

## allow-lists.yaml

One document with a single `allow_lists` key holding a list.

| Field | Type | Required | Description |
|---|---|---|---|
| `team` | string | yes | The Team this list is declared for. Must be in the team tree. |
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

At most one list per Team: a Team's declared list is one intersection term, so
declaring two is a load error.

## grants.yaml

One document with a single `grants` key holding a list.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique. Everything a team may use traces to the root list or to a named Grant, and this is that name. |
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

A Grant is parent-authored. The author is the owner's Team, and it must sit
strictly above the target: a Team granting to itself would be self-widening,
which is exactly what narrowing-only inheritance forbids.

## Entry syntax

Entries are shapes, never literals. Each selects Catalogue components as
`class/type-pattern`.

The class side is exact and must be one of `receiver`, `processor`,
`exporter`, `connector`, `extension`: the unit of allowing is the component
key `(class, type)`.

The type side is a pattern with a deliberately small vocabulary:

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

Character classes and escapes are rejected: an entry containing `[`, `]`, `\`
or a second `/` is a load error. What loads, matches.

A pattern is tried against a component's canonical type and against its
`deprecated_type` alias, so an entry written against a historical name keeps
selecting the component it always did.

## Resolution

A team's effective palette is computed by walking the team chain from the root
down to the team. At each team in the chain, in this order:

1. If that team declares a list, intersect: a component the list doesn't match
   is removed.
2. Then apply every Grant targeting that team: a component a Grant matches is
   added back.

Two consequences follow from that order, and both are deliberate.

A Grant overrides its own target's declared list
: The union is applied after the intersection at the same team, so a Grant
  targeting team `T` re-admits a component `T`'s own list excludes.

Lists below the target still narrow it away
: The walk continues downward, and a descendant's list runs after the Grant.
  A Grant widens from its target's subtree downward and is narrowed back out
  below like anything else.

Absent any declared list anywhere on the chain, the effective list is the
whole active Catalogue.

## Provenance

Every entry in an effective palette carries the reason it's there. `telecraft
palette` prints it in the last column.

| Origin | Means |
|---|---|
| `default-allow` | No Allow-list is declared anywhere on the team's chain. |
| `allow-list` | The component survived every declared list on the chain. |
| `grant` | A named Grant admitted it; the lists alone would exclude it. |

A `grant` entry also names the Grant id, the granting team (the Grant owner's
team, which is the authority) and the target team it was attached to. The
audit chain is total: everything a team may use traces to the lists surviving
intersection, the default posture, or a named Grant.

## Enforcement

The allow-list check is the one policy rule that hard-blocks. A Blueprint
using a catalogue type outside the owning team's effective palette refuses the
render. The escape hatch is a Grant, fast and auditable, so no break-glass
override exists.

Everything else the render notices, such as a stability floor breach or a
binding pinned off head, is a finding routed to an owner and never a block.

## Load errors

Loading fails closed and returns nothing. Each message names the file. The
load refuses on:

- an unknown or misspelled field in either file
- a malformed document, an empty file, or more than one YAML document in the
  file
- a present `allow-lists.yaml` holding no `allow_lists`, or a present
  `grants.yaml` holding no `grants`: declare one or delete the file
- an allow-list with an empty `allow` list. To inherit the parent's effective
  list unchanged, declare no list at all. An empty list would ban everything,
  and default-deny is deliberately not built in v1.
- a Grant with an empty `adds` list: a Grant exists to widen a palette
- a missing `team` or `owner`, or one the team tree doesn't know
- a Grant with no `id`, or an `id` defined twice
- one Team declaring two allow-lists
- a Grant whose owner's team isn't a proper ancestor of the target team
- a duplicated entry in one list
- a malformed entry: not `class/type-pattern`, an unknown class, or a pattern
  containing a rejected character
- an entry that selects nothing in the active Catalogue. An entry selecting
  nothing is an unknown component type or a typo'd pattern, and it fails the
  load rather than silently allowing nothing.

That last rule binds a loaded policy to the Catalogue version it was validated
against. Judging with a different Catalogue would let entries silently match
nothing.
