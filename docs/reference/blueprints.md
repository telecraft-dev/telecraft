---
title: Blueprint file format
description: Every field of a Blueprint and a shared Component, with reference syntax, pinning, lanes and the findings the loader raises.
order: 4
---

# Blueprint file format

A Blueprint is a named, integer-versioned composition of Components. It isn't
annotated collector config. It lists per-signal lanes under the upstream
signal names, each an explicitly ordered list of Component references, plus
one collector-wide `extensions` block.

Blueprints live at `teams/<team>/blueprints/<name>.yaml` and shared Components
at `teams/<team>/components/<name>.yaml`. See
[Estate layout](estate-layout.md) for the rules every authored file follows.

To load a tree, run `blueprint-check`. It prints what loaded and every
finding. `telecraft render` performs the same load.

## Blueprint fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | yes | none | Must equal the filename without its extension. |
| `version` | integer | yes | none | An integer, 1 or higher. The owner raises it in the same change that alters the Blueprint. |
| `owner` | string | yes | none | The accountable party. |
| `satisfies` | list of strings | no | empty | Version-stamped Requirement claims. See [Satisfies claims](#satisfies-claims). |
| `components` | list of Components | no | empty | The Blueprint's local Components, declared inline. |
| `pipelines` | mapping | no | empty | The four per-signal lanes. See [Lanes](#lanes). |
| `extensions` | list of entries | no | empty | The collector-wide extensions block. |

A Blueprint with no lane entries and no extensions is a load error, because
it would render an empty collector.

The Blueprint's team-qualified id is `<team>/<name>`, taken from the file's
place in the layout.

```yaml
# teams/data-flow/blueprints/gateway-standard.yaml
name: gateway-standard
version: 4
owner: gateway-owners
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
    config:
      protocols:
        grpc: {}
  - name: guard
    class: processor
    type: memory_limiter
    version: 2
    config:
      check_interval: 1s
      limit_mib: 512
  - name: batcher
    class: processor
    type: batch
    version: 1
  - name: health
    class: extension
    type: health_check
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: guard
    - component: infosec/pii-redaction@3
    - component: batcher
    - component: data-flow/gateway-exporter@2
  logs:
    - component: otlp-in
    - component: guard
    - component: batcher
    - component: data-flow/gateway-exporter@2
extensions:
  - component: health
```

## Component fields

One schema serves both kinds of Component. A shared Component is a standalone
file with a required `owner` and the team-qualified id `<team>/<name>`. A
local Component sits inline in a Blueprint's `components:` list, belongs to
the Blueprint's owner, and can't be referenced from outside that Blueprint.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | yes | none | On a shared Component, must equal the filename without its extension. On a local Component, the name that lane entries reference. |
| `class` | string | yes | none | One of `receiver`, `processor`, `exporter`, `connector`, `extension`. |
| `type` | string | yes | none | The Catalogue type this Component configures. |
| `version` | integer | yes | none | An integer, 1 or higher. |
| `owner` | string | shared only | none | Required on a shared Component, forbidden on a local one. |
| `config` | mapping | no | empty | The otelcol configuration body, written as is under the rendered id. |

A Component says what it is. The Tier that uses it decides which Catalogue
version judges it.

```yaml
# teams/data-flow/components/gateway-exporter.yaml
name: gateway-exporter
class: exporter
type: otlphttp
version: 2
owner: gateway-owners
config:
  endpoint: https://gateway.internal:4318
```

### Rendered ids

Each placed Component renders under a `type/name` id: a shared Component as
`<type>/<team>.<name>`, a local one as `<type>/<name>`. The id carries the
provenance. If two rendered ids collide, the render refuses.

## Lanes

`pipelines` accepts exactly four keys, named after the upstream signal names.
Each is optional and defaults to empty. Any other lane name is an unknown
field and fails the load.

| Key | Contains |
|---|---|
| `traces` | Ordered entries for the traces pipeline. |
| `metrics` | Ordered entries for the metrics pipeline. |
| `logs` | Ordered entries for the logs pipeline. |
| `profiles` | Ordered entries for the profiles pipeline. |

`extensions` sits beside `pipelines`, not inside it, and holds the
collector-wide extensions block. Findings report the extensions block under
the lane name `extensions`.

The renderer never re-sorts a lane. What you write is what renders. If an
order contradicts a rule, the renderer reports a finding instead. See
[Ordering findings](#ordering-findings).

## Lane entries

Every lane entry has exactly two fields. An entry can only point at a
Component, so you can't write a raw inline otelcol block.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `component` | string | yes | none | The Component reference. |
| `track` | string | no | empty | The only accepted value is `head`. Empty means pinned. |

### Reference syntax

A reference is an id, never a path:

| Form | Means |
|---|---|
| `<name>` | A local Component of this Blueprint. |
| `<team>/<name>@<version>` | A shared Component pinned to that version. |
| `<team>/<name>` with `track: head` | A shared Component tracking the owning team's head. |

The pin after `@` is a positive integer. Anything else is a load error.

The loader enforces these rules on each reference:

- A shared reference either pins a version or sets `track: head`. Doing
  neither is a load error: shared references pin by default, so write the pin.
- Doing both is a load error.
- A local reference must not carry a pin. A local Component travels with its
  Blueprint, so there's nothing for a pin to point at.
- A local reference must not set `track: head`. A local Component has no head
  apart from the Blueprint it lives in.
- A local reference must name a Component this Blueprint declares.
- The same id appearing twice in one lane is a load error.
- A declared local Component that no lane references is a load error.

## Satisfies claims

Each `satisfies` entry is the string `<requirement-id>@<version>`: the
Requirement id the Blueprint claims to satisfy, stamped with the Requirement
version the claim was made against. An unversioned claim is a load error,
because drift couldn't be detected. Claiming the same Requirement twice is a
load error.

A claim states intent. The evaluator always judges against the Requirement's
current version, so a claim can't freeze the rule at an older version. A
claim stamped behind the current version is what `library_drift` detects. See
[Requirements](requirements.md).

```yaml
satisfies:
  - logs-delivered@1
  - service-name-on-logs@1
```

## Load errors

The load fails closed and returns nothing. Every message names the file, and
the loader reports every problem it found rather than stopping at the first.
The load refuses on:

- an unknown or misspelled field, anywhere in the document
- a malformed document, an empty file, a top-level list, or more than one YAML
  document in the file
- a missing `owner`, or a `version` below 1
- a `name` that contradicts the filename
- a `class` outside the five pipeline classes, or an empty `type`
- an `owner` on a local Component
- a duplicate local Component name, or a duplicate object id across the source
  roots
- any of the reference rules above
- a source root with no `teams/` tree, or a tree with nothing authored at all

## Findings

Problems that cross object boundaries don't block the load. They surface as
findings, route to the owner of the Blueprint they're about, and name the
lane, so the fix is an edit to one list. One team's retraction never stops
another team's render.

Each finding carries a kind, the Blueprint id, the lane, and a message.

### Reference findings

Kind `reference`. Raised when a structurally valid reference can't deliver
what it promises:

- The referenced shared Component doesn't exist: it was never authored, or it
  has since been retracted.
- The pin is ahead of the owning team's current version, so the pinned version
  doesn't exist at head.
- An entry in the `extensions` block references something that isn't an
  extension.
- An entry in a signal lane references an extension. Extensions are
  collector-wide and never live in a signal lane.

A pin behind head is not a reference finding. That's `library_drift`, which
has its own detection.

### Ordering findings

Kind `ordering`. Raised when a lane's explicit order contradicts an ordering
rule keyed on a Catalogue type. Position is judged among the same-class
entries of that lane, because that's how the rendered pipeline executes. The
`extensions` block is never judged: extensions have no pipeline order.

The rules that ship:

| Class | Type | Slot | Why |
|---|---|---|---|
| `processor` | `memory_limiter` | `first` | Back-pressure must engage before any other processor buffers or fans out. |
| `processor` | `batch` | `last` | Batching belongs after every shaping processor, so the exporter sees the final shape. |

A lane entry that doesn't resolve is skipped: its problem is already a
reference finding.
