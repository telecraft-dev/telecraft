---
title: Requirement file format
description: Every field of a Requirement, with levels, environments, assertion kinds, schema conformance and version-stamped satisfies claims.
order: 6
---

# Requirement file format

A Requirement is one named, versioned rule with required remediation text. It
can assert on Effective state, on Observed state, or on both. It can instead
assert schema conformance, which is judged against the Schema Registry alone.

Requirements live in a library directory: a flat directory of `*.yaml` or
`*.yml` files, one concern per file, so a change to one Requirement is a
one-file diff in review. Each file holds one Requirement or a list of them.
`telecraft check -library` and `telecraft snapshot -library` name that
directory. Subdirectories are not read.

## Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `id` | string | yes | none | Unique across the library. |
| `title` | string | no | empty | Short human-readable name. |
| `description` | string | no | empty | What the Requirement asks for, and why. |
| `version` | integer | yes | none | 1 or higher. Raise it when you tighten the Requirement, so the change is dated and visible. |
| `requirement_level` | string | no | `recommended` | One of `required`, `conditionally_required`, `recommended`, `opt_in`. |
| `owner` | string | yes | none | The accountable party. An Exemption from this Requirement is valid only with this owner's review. |
| `environments` | list of strings | no | empty | Narrows where the Requirement applies. Empty applies everywhere. |
| `config` | mapping | no | absent | The Effective-state assertion. |
| `signal` | mapping | no | absent | The Observed-state assertion. |
| `schema_conformance` | mapping | no | absent | The Schema Registry reference. See [Schema conformance assertions](#schema-conformance-assertions). |
| `placement` | string | no | `landed` | Which reading a schema assertion is judged against. Only on a `schema_conformance` Requirement. |
| `remediation` | string | yes | none | The concrete change that closes the gap. |

At least one of `config`, `signal` and `schema_conformance` must be present.
`schema_conformance` doesn't combine with the other two: see
[Schema conformance assertions](#schema-conformance-assertions).

```yaml
# requirements/delivery.yaml
- id: logs-delivered
  title: Logs are collected and delivered
  description: >
    The Service's logs reach a backend. Collection may be by file tail or by
    OTLP: how the logs are gathered is the owner's choice, that they arrive
    is not.
  version: 1
  requirement_level: required
  owner: platform-observability
  config:
    has_receiver: [filelog, otlp]
  signal:
    kind: logs
    present: true
    window: 24h
    min_volume: 1
  remediation: >
    Add a filelog or otlp receiver to the collector serving this Service and
    wire it into a logs pipeline.
```

## Requirement levels

`requirement_level` uses the four-level vocabulary of the OpenTelemetry
semantic conventions, unchanged. It's richer than a binary required list:
`recommended` is the natural level for attribute coverage that isn't yet
stable upstream.

| Value | Meaning |
|---|---|
| `required` | The assertion must hold. |
| `conditionally_required` | The assertion must hold where its condition applies. |
| `recommended` | The default when `requirement_level` is absent. |
| `opt_in` | The assertion holds only where you opt in. |

A value outside these four is a load error.

## Environments

`environments` narrows a Requirement to the named Environments. An absent or
empty list applies everywhere. Write one environment-neutral assertion with a
narrowing list, rather than one file per Environment.

You define the Environment vocabulary, so the loader has no authority over
which environments exist. A name that matches nothing the estate knows is
not a load error. It surfaces as an
[authoring finding](#authoring-findings) instead.

An empty string in the list, or a name listed twice, is a load error.

## Assertion kinds

The kind of a Requirement follows from the assertions present. You don't
write it, so it can never disagree with them.

| Kind | Present |
|---|---|
| `config` | `config` only. |
| `signal` | `signal` only. |
| `config_and_signal` | Both. |
| `schema_conformance` | `schema_conformance`, and neither of the other two. |

Asserting on both readings lets the evaluator cross them. A config-only
Requirement can pass for a collector that delivers nothing, and a signal-only
one can fail without naming a cause.

## Config assertions

`config` evaluates against the Effective reading: the running config the
collector reports. Each list is satisfied if any of its entries is present:
"collect logs somehow" is the real requirement, and `filelog` versus `otlp`
is an implementation detail.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `has_receiver` | list of strings | no | empty | Receiver types, any of which satisfies the list. |
| `has_processor` | list of strings | no | empty | Processor types, any of which satisfies the list. |
| `has_exporter` | list of strings | no | empty | Exporter types, any of which satisfies the list. |

A `config` block with all three lists empty is a load error.

Matching is on the type prefix, so a component the collector reports as
`otlp/onramp` satisfies an assertion asking for `otlp`. Config assertions
carry no pipeline scope, so an assertion matches any pipeline.

## Signal assertions

`signal` evaluates against the Observed reading: telemetry that landed in a
backend over a window. Every field is expressed in terms of signal presence,
volume, and attribute coverage. A Requirement can't carry a backend query,
so every Requirement works against every backend.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `kind` | string | yes | none | One of `logs`, `metrics`, `traces`. |
| `present` | boolean | no | `false` | Whether the signal must be present. |
| `window` | duration string | yes | none | The trailing window, such as `24h` or `90m`. Must be positive. |
| `min_volume` | integer | no | `0` | Minimum record count in the window. Must not be negative. |
| `required_attributes` | list of strings | no | empty | Attribute names that must appear on essentially every record. |
| `attribute_coverage` | number | no | `1.0` | The fraction of records that must carry each required attribute, within `(0, 1]`. |

`profiles` isn't a signal kind here: the signal is alpha upstream, so the
evaluator can't judge it reliably. Blueprint lanes do carry a `profiles`
lane, because a Blueprint can route a signal the evaluator can't yet judge.

`min_volume` catches a pipeline that's alive but delivering almost nothing,
which a presence check alone would call healthy. `attribute_coverage` tells a
partially instrumented estate from an uninstrumented one. The default
requires total coverage unless you relax it.

## Schema conformance assertions

`schema_conformance` asks whether the telemetry that arrived is the shape the
Schema Registry says it should be. It's a reference into a registry version,
never a copy of one: you name a version and a scope within it, and the
registry says which attributes that scope demands, at which requirement level,
with which types and which enum members.

There's no attribute list here, and adding one is a load error. A list in a
requirement file is a second copy of something the registry already states,
and the copy drifts the first time somebody edits one and not the other.

A Requirement of this kind runs against an estate. The evaluator reads the
attribute names in use for each signal and window the reference covers,
resolves what the pinned registry version demands of the scope, and judges one
against the other. Where the reading can't be taken, or the reference resolved
to no version, the verdict is `unknown` with the cause: no reading, so no
verdict, and never a silent pass.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `registry_version` | string | yes, unless tracking | none | The Schema Registry version, by the ref it was imported at. |
| `track` | string | no | empty | `head` opts this reference into judging against whichever version is active. The only value is `head`. |
| `scope` | mapping | yes | none | What conformance is demanded of. |
| `scope.groups` | list of strings | no | empty | Registry group ids, such as `span.db.client`. |
| `scope.namespaces` | list of strings | no | empty | Attribute-name prefixes, such as `db`: every attribute under the prefix is demanded. |
| `signals` | list of strings | yes | none | The signals the scope is judged on: `logs`, `metrics`, `traces`. |
| `window` | duration string | yes | none | The trailing window, such as `24h`. Must be positive. |

At least one of `scope.groups` and `scope.namespaces` must be present. An
empty scope would demand the whole registry of every Service by omission.

The window is read twice: once for the attribute names in use, and once for
whether the covered signals arrived at all. That second reading is what tells
`not_delivered` from `misconfigured`. Nothing arrived is `not_delivered`;
telemetry arrived and is missing an attribute the registry demands at
`required` is `misconfigured`; every `required` attribute in use is
`compliant`.

```yaml
# requirements/schema.yaml
- id: db-spans-conform
  title: Database spans carry what the registry demands
  version: 1
  requirement_level: required
  owner: platform-observability
  environments: [production]
  schema_conformance:
    registry_version: v1.4.0
    scope:
      groups: [span.db.client]
    signals: [traces]
    window: 24h
  remediation: >
    Add the missing attributes to the database instrumentation, or upgrade
    the instrumentation library to a version that sets them.
```

A `schema_conformance` Requirement doesn't also carry `config` or `signal`.
Collector config can't see instrumentation, so there's no Effective reading to
cross a schema verdict against, and no outcome for the combination. Two
Requirements say the two things honestly.

### Findings

A schema-conformance finding belongs to the Service it was judged on, in the
Environment it was judged in, and routes to that Service's owner. That's the
party who can act on it. The fix is a change to the service's own
instrumentation, so a Tier or a collector as the target would send the work to
somebody who can't make it.

The finding writes its own remediation out of the registry rather than
repeating the line you authored. It names the group that demanded the
attribute, the attribute, the type the registry declares it at, and the level.
Where the registry carries a deprecation notice, the finding hands over
upstream's own migration note, including what the attribute was renamed to.
The `remediation` you author stays on the Requirement, and is what a finding
falls back to when it has nothing better to say.

Where a missing attribute sits on a `resource` or `entity` group, the
remediation also mentions collection-time enrichment: a processor such as
`k8sattributes` can add it when the service can't. That's a suggestion and
nothing more. The finding doesn't split in two and doesn't reroute, so the
Service's owner still owns it.

### Pinned and tracking references

A reference pins a version by default. An adopter's registry tightening a
requirement level is a change every Service's score feels, so it's adopted
deliberately rather than overnight.

`track: head` is the opt-in for a scope that should follow the registry as it
moves, which is usually your own namespaced attributes rather than upstream's.
A reference either pins or tracks. Doing both is a load error, and doing
neither is too.

### Resolving the reference

The reference is resolved when the library loads, against the Schema Registry
versions the platform has imported. A pinned version that isn't installed is a
load error, as is a group id or a namespace the pinned version carries
nothing under. A reference that resolved to nothing would demand nothing, and
a Requirement that demands nothing passes every Service silently, which is the
one failure this loader exists to prevent.

A tracking reference names no version, so there's nothing to resolve a scope
against; what must exist is an imported registry for there to be a head at
all. Which installed version is the active one is an activation decision, and
nothing makes it yet, so a tracking reference loads and then reads `unknown`
at evaluation. Pin a version to get a verdict today.

Where the installed versions live is the command's to say. Each command that
loads a library takes a `-schema-registries` directory, holding one
`schema-registry-<ref>.json` artefact per imported version, which is what
`schema-registry-import` writes:

```sh
go run ./cmd/schema-registry-import -repo https://git.example/registry -ref v1.4.0
telecraft check -library requirements/ -estate estate.yaml \
  -schema-registries schema-registries/
```

Schema Registry versions are instance-side artefacts, not estate content, the
same as Catalogue versions and for the same reason: they're imported, retained
version by version, and shared by every estate the instance judges. A library
that references one and is loaded without the directory doesn't load at all,
which is the fail-closed direction: the alternative is a Requirement that
evaluates nothing and scores every Service clean.

### Placement

`placement` says which reading a schema assertion is judged against.

| Value | Meaning |
|---|---|
| `landed` | Telemetry that has already landed in a backend. The default. |
| `live` | Findings a collection-time tap emitted. |

`placement: live` is a load error today: the tap it reads findings from isn't
built, so a `live` Requirement would evaluate nothing and every Service would
read clean against it. The field is carried now so that the Requirement shape
doesn't change again when the tap lands.

`placement` on a Requirement with no `schema_conformance` block is a load
error. Nothing else has a placement.

## Load errors

The load fails closed and returns nothing. Every message names the file, and
for field errors the field. The load refuses on:

- an unknown or misspelled field, anywhere in the document
- a malformed document, an empty file, or more than one YAML document in the
  file
- a file that is neither one mapping nor a list of mappings
- a library directory that doesn't exist, or holds no `*.yaml` or `*.yml`
  files at all: an empty library would judge everything compliant with
  nothing checked
- a missing `id`, or an `id` defined in two files
- a missing `owner`, a missing `remediation`, or a `version` below 1
- a `requirement_level` outside the four values
- no `config`, no `signal` and no `schema_conformance`, or a `config` block
  with no entries
- a `signal` block with an unknown `kind`, a non-positive `window`, a negative
  `min_volume`, an `attribute_coverage` outside `(0, 1]`, or an empty required
  attribute name
- a `schema_conformance` block alongside a `config` or `signal` block
- an `attributes` or `required_attributes` list inside a `schema_conformance`
  block: the registry says which attributes a scope carries, and a copy drifts
- a `schema_conformance` block that both pins a `registry_version` and sets
  `track: head`, or that does neither, or that sets a `track` other than `head`
- a `schema_conformance` block with an empty `scope`, no `signals`, an unknown
  signal kind, a signal listed twice, or a non-positive `window`
- a `registry_version` that isn't installed, or that doesn't load
- a `scope.groups` entry the pinned registry version doesn't declare, or a
  `scope.namespaces` entry it carries no attribute under
- a `track: head` reference when no Schema Registry version is installed
- a `schema_conformance` block when the load was given no Schema Registry
  directory to resolve it against
- `placement: live`, which isn't implemented; a `placement` outside `landed`
  and `live`; or a `placement` on a Requirement with no `schema_conformance`
  block
- an empty or duplicated entry in `environments`

## Authoring findings

An authoring finding means the library is valid and loads, but something you
wrote can never take effect. Authoring findings appear in the `check`
report's `authoring_findings` array and are never part of the exit code.

The library raises one kind: an `environments` entry that matches no
Environment the estate declares. If every entry is unknown, one finding says
the Requirement never applies. If some are known, one finding per unknown
entry says that entry never matches.

[Exemptions](exemptions.md) raise authoring findings of their own into the
same array.

## Satisfies claims and versioning

`version` makes tightening a Requirement a dated, visible event rather than a
silent change in everyone's score.

A Blueprint claims Requirements through version-stamped `satisfies` entries of
the form `<requirement-id>@<version>`, where the version is the Requirement
version the claim was made against. See
[Satisfies claims](blueprints.md#satisfies-claims) for the authoring rules.

A claim states intent. The evaluator always judges against the Requirement's
current version, so a claim can't freeze the rule at an older version. When
config passes the version it claims but fails the current one, the outcome is
`library_drift`: the Requirement moved and the config hasn't caught up. That's
distinct from never having complied. Its remediation is the version diff
rather than re-instrumenting, and the repository owns it rather than a row.
`telecraft check -source -catalogue` detects it and reports it in the
report's `library_drift` section.

Claims that pass at both the claimed and the current version, but are stamped
behind, are housekeeping nudges: visible in the report's `housekeeping`
array, never counted.

## Outcomes

The eight outcomes, in severity order, worst first:

| Outcome | Meaning |
|---|---|
| `broken_pipeline` | Effective says yes, Observed says no. Someone configured this and it isn't working. |
| `not_configured` | Effective says no, Observed says no. The requirement is unmet, and the owner needs to instrument. |
| `not_delivered` | Observed says no, with no Effective evidence to explain why. |
| `misconfigured` | A config assertion failed with no signal reading to cross it against. |
| `library_drift` | The config passes the version it claims or pins while failing the current one. |
| `unknown` | No reading gave any evidence. Never silently a pass or a failure. |
| `ungoverned` | Observed says yes, Effective says no. The requirement is met, so this passes, but the report shows it. |
| `compliant` | The requirement is met. |

`compliant` and `ungoverned` are the passing outcomes. `library_drift` is the
one outcome the Effective by Observed cross never produces: it's judged from
the Intended reading, the config in git.
