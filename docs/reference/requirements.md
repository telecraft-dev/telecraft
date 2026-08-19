---
title: Requirement file format
description: Every field of a Requirement, with levels, environments, assertion kinds and version-stamped satisfies claims.
order: 6
---

# Requirement file format

A Requirement is one named, versioned assertion with mandatory remediation
text. It may assert on Effective state, on Observed state, or on both.

Requirements live in a library directory: a flat directory of `*.yaml` or
`*.yml` files, one concern per file, so a change to one Requirement is a
one-file diff in review. Each file holds one Requirement or a list of them.
`telecraft check -library` and `telecraft snapshot -library` name that
directory. Subdirectories are not read.

The decisions behind the format are ADR-0009 (the semantic-conventions
vocabulary), ADR-0033 (per-environment evaluation) and ADR-0026 (version
stamping).

## Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `id` | string | yes | none | Unique across the library. |
| `title` | string | no | empty | Short human-readable name. |
| `description` | string | no | empty | What the Requirement asks for, and why. |
| `version` | integer | yes | none | 1 or higher. Raising the bar is a dated, visible event. |
| `requirement_level` | string | no | `recommended` | One of `required`, `conditionally_required`, `recommended`, `opt_in`. |
| `owner` | string | yes | none | The accountable party. An Exemption from this Requirement is valid only with this owner's review. |
| `environments` | list of strings | no | empty | Narrows applicability. Empty applies everywhere. |
| `config` | mapping | no | absent | The Effective-state assertion. |
| `signal` | mapping | no | absent | The Observed-state assertion. |
| `remediation` | string | yes | none | The concrete change that would close the gap. |

At least one of `config` and `signal` must be present.

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

`requirement_level` is the four-level vocabulary adopted verbatim from the
semantic-conventions model. It's strictly richer than a binary required list:
`recommended` is the principled home for sub-1.0 attribute coverage.

| Value | Meaning |
|---|---|
| `required` | The assertion must hold. |
| `conditionally_required` | The assertion must hold where its condition applies. |
| `recommended` | The default when `requirement_level` is absent. |
| `opt_in` | The assertion holds only where an adopter opts in. |

A value outside these four is a load error.

## Environments

`environments` narrows a Requirement to the named Environments. An absent or
empty list applies everywhere: explicit narrowing beats implicit non-coverage.
One environment-neutral assertion with a narrowing list is the model, never a
per-environment variant file, which would drift.

The Environment vocabulary is adopter-defined and open, so the loader has no
authority over what environments exist. A name that matches nothing known to
the estate is therefore not a load error. It surfaces as an
[authoring finding](#authoring-findings) instead.

An empty string in the list, or a name listed twice, is a load error.

## Assertion kinds

The kind of a Requirement is derived from the assertions present, never
authored, so it can never disagree with them.

| Kind | Present |
|---|---|
| `config` | `config` only. |
| `signal` | `signal` only. |
| `config_and_signal` | Both. |

Asserting on both readings is what makes the outcome cross possible. A
config-only Requirement can be satisfied by a collector that delivers nothing,
and a signal-only one can fail without naming a cause.

## Config assertions

`config` evaluates against the Effective reading: the collector's own reported
running config. Each list is satisfied if any of its entries is present,
because "collect logs somehow" is the real requirement and `filelog` versus
`otlp` is an implementation detail.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `has_receiver` | list of strings | no | empty | Receiver types, any of which satisfies the list. |
| `has_processor` | list of strings | no | empty | Processor types, any of which satisfies the list. |
| `has_exporter` | list of strings | no | empty | Exporter types, any of which satisfies the list. |

A `config` block with all three lists empty is a load error.

Matching is on the type prefix, so a component the collector reports as
`otlp/onramp` satisfies an assertion asking for `otlp`. Config assertions
carry no pipeline scope, so a bare assertion matches any pipeline.

## Signal assertions

`signal` evaluates against the Observed reading: telemetry that landed in a
backend over a window. Every field is expressed in terms of signal presence,
volume and attribute coverage. Nothing here can carry a backend query, by
design: the moment a Requirement could embed a query string, only one backend
would ever really be supported.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `kind` | string | yes | none | One of `logs`, `metrics`, `traces`. |
| `present` | boolean | no | `false` | Whether the signal must be present. |
| `window` | duration string | yes | none | The trailing window, such as `24h` or `90m`. Must be positive. |
| `min_volume` | integer | no | `0` | Minimum record count in the window. Must not be negative. |
| `required_attributes` | list of strings | no | empty | Attribute names that must appear on essentially every record. |
| `attribute_coverage` | number | no | `1.0` | The fraction of records that must carry each required attribute, within `(0, 1]`. |

`profiles` is deliberately not a signal kind here: the signal is alpha
upstream, so a Requirement written against it couldn't be evaluated honestly.
Blueprint lanes do carry a `profiles` lane, because a Blueprint may route a
signal the evaluator can't yet judge.

`min_volume` guards against a pipeline that's technically alive and delivering
almost nothing, which reads as healthy to any presence check.
`attribute_coverage` distinguishes a partially instrumented estate from an
entirely uninstrumented one: the default of total coverage holds unless you
relax it explicitly.

## Load errors

The load fails closed and returns nothing. Every message names the file, and
for field errors the field. The load refuses on:

- an unknown or misspelled field, anywhere in the document
- a malformed document, an empty file, or more than one YAML document in the
  file
- a file that is neither one mapping nor a list of mappings
- a library directory that doesn't exist, or holds no `*.yaml` or `*.yml`
  files at all: an empty library would judge everything compliant vacuously
- a missing `id`, or an `id` defined in two files
- a missing `owner`, a missing `remediation`, or a `version` below 1
- a `requirement_level` outside the four values
- no `config` and no `signal`, or a `config` block with no entries
- a `signal` block with an unknown `kind`, a non-positive `window`, a negative
  `min_volume`, an `attribute_coverage` outside `(0, 1]`, or an empty required
  attribute name
- an empty or duplicated entry in `environments`

## Authoring findings

An authoring finding means the library is valid and loads, but something an
author wrote can never take effect. Surfacing that beats silently never
applying it. Authoring findings appear in the `check` report's
`authoring_findings` array and are never part of the exit code.

The library raises one kind: an `environments` entry that matches no
Environment the estate declares. If every entry is unknown, one finding says
the Requirement will never apply. If some are known, one finding is raised per
unknown entry saying that entry never matches.

[Exemptions](exemptions.md) raise authoring findings of their own into the
same array.

## Satisfies claims and versioning

`version` makes raising the bar a dated, visible event rather than a silent
overnight change in everyone's score.

A Blueprint claims Requirements through version-stamped `satisfies` entries of
the form `<requirement-id>@<version>`, where the version is the Requirement
version the claim was made against. See
[Satisfies claims](blueprints.md#satisfies-claims) for the authoring rules.

A claim is intent, never fact. The evaluator always judges against the
Requirement's current version, so a claim can never freeze the goalposts. When
config passes the version it claims while failing the current one, the
diagnosis is `library_drift`: the goalposts moved and the subject hasn't
caught up. That's a distinct outcome from never having complied, its
remediation is the version diff rather than re-instrumenting, and it's owned
by the repository rather than by a row. `telecraft check -source -catalogue`
detects it and reports it in the report's own `library_drift` section.

Claims that pass at both the claimed and the current version, but are stamped
behind, are housekeeping nudges: visible in the report's `housekeeping`
array, never counted.

## Outcomes

The eight outcomes, in severity order, worst first:

| Outcome | Meaning |
|---|---|
| `broken_pipeline` | Effective says yes, Observed says no. Someone configured this and it isn't working. |
| `not_configured` | Effective says no, Observed says no. An unmet requirement, and the owner needs to instrument. |
| `not_delivered` | Observed says no, with no Effective evidence to explain why. |
| `misconfigured` | A config assertion failed with no signal reading to cross it against. |
| `library_drift` | The config passes the version it claims or pins while failing the current one. |
| `unknown` | No evidence available from any reading. Never silently a pass or a failure. |
| `ungoverned` | Observed says yes, Effective says no. The requirement is met, so this passes, but it's surfaced. |
| `compliant` | The requirement is met. |

`compliant` and `ungoverned` are the passing outcomes. `library_drift` is the
one outcome the Effective by Observed cross never produces: it's judged from
the Intended reading, the config in git.
