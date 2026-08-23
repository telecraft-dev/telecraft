---
title: Exemption file format
description: The Exemption file format, its mandatory owner and expiry, subject scoping, the grace table, and load-time validation.
order: 8
---

# Exemption file format

An Exemption is one authored waiver: exactly one Requirement, waived for one
Service or one Team subtree, with a required owner and a required expiry.

An Exemption waives the count, not the diagnosis. A waived finding keeps its
outcome and its detail in the report and gives up only its contribution to
the exit code, so a pass built on exemptions still shows every waiver. An
Exemption never stops a Service from complying.

Exemptions live in a directory of `*.yaml` or `*.yml` files, each holding one
Exemption or a list of them. `telecraft check -exemptions` and `telecraft
snapshot -exemptions` name that directory. Subdirectories are not read.

## Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `id` | string | yes | none | Unique across the directory. |
| `requirement` | string | yes | none | The one Requirement this Exemption waives. |
| `owner` | string | yes | none | The party responsible for the waiver. |
| `expires` | date | yes | none | Written as `2026-09-01`. The waiver stops counting at UTC midnight at the start of that day. |
| `service` | string | one of two | none | The subject: one Service. |
| `team` | string | one of two | none | The subject: one Team subtree. |
| `reason` | string | no | empty | Why the waiver exists. The report appends it to the waiver reason. |

Set exactly one of `service` and `team`. Setting neither or both is a load
error.

```yaml
# exemptions/checkout-traces.yaml
id: checkout-traces-2026-q3
requirement: traces-delivered
owner: checkout-team
expires: 2026-10-01
service: checkout
reason: >
  Tracing lands with the November SDK upgrade; the rollout is scheduled and
  tracked in the platform backlog.
```

## Subject scoping

`service` names one Service by the same name the estate rows carry.

`team` names a Team and covers its whole subtree. Use it when onboarding a
team: one reviewable file rather than a copy per Service.

To resolve a team-scoped Exemption, Telecraft needs the ownership model, so
it can tell whether a Service belongs to the named Team's subtree. `telecraft
check` reads that from `-ownership`. Without it, a team-scoped Exemption is an
error on the run rather than a waiver that silently never applies. A Service
the ownership model doesn't know is in no subtree, so the waiver stays
unapplied and the finding stays visible.

## Expiry

Expiry depends on the clock alone. There's no manual step: an expired
Exemption stops matching, so the raw finding is back on the next run.

To renew, change the file. An expired Exemption still in the tree is dead
config, and raises an [authoring finding](#authoring-findings).

## Owner review

The change that adds an Exemption must be approved by the waived
Requirement's owner, or by that owner's ancestor team. The generated
`CODEOWNERS` file enforces this, not the loader, so a team can't waive its
own Requirement.

## Precedence with Grace

Two things can waive a finding's count on one run: an authored Exemption, and
the Grace Period Telecraft computes from a Service's Class and onboarding
date. They stay distinct. Where both cover the same finding, the Exemption
wins, because it names the party responsible for the waiver.

Passing findings are never waived: there's nothing to forgive, and waiving
them would inflate the waived count that roll-ups keep visible.

The report records which applied. The waiver reason for an Exemption names the
Exemption id, its owner, its expiry date, and its reason. The waiver reason
for Grace names the Service Class and when the onboarding window ends.

## The grace table

Grace Periods are declared in the estate file, the same file `telecraft check`
takes as `-estate` and `telecraft snapshot` takes as `-rows`. Its `grace` key
maps each Service Class to its onboarding window. Its `services` key declares
the rows to judge, each carrying the Effective reading and the two inputs the
Grace computation needs.

```yaml
grace:
  - class: C1
    window: 240h
  - class: C2
    window: 720h
services:
  - name: checkout
    class: C1
    onboarded: 2026-08-01
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [otlp]
            processors: [batch]
            exporters: [otlphttp]
      - name: staging
        pipelines: []
```

### Grace fields

| Field | Type | Required | Description |
|---|---|---|---|
| `grace[].class` | string | yes | The Service Class this window applies to. |
| `grace[].window` | duration string | yes | The onboarding window, such as `240h`. Must be positive. |

Write the table highest class first. Grace shrinks as class rises, so a
window must never be shorter than the one above it. A table that gives the
most critical class the longest window can't load.

An absent or empty `grace` key means no Grace Periods apply.

### Service fields

| Field | Type | Required | Description |
|---|---|---|---|
| `services[].name` | string | yes | The Service name, matched by exemptions and by the telemetry provider. |
| `services[].class` | string | no | The Service Class. Must be one the grace table defines. |
| `services[].onboarded` | date | no | Written as `2026-08-01`. |
| `services[].environments[].name` | string | yes | The Environment. One row per (Service, Environment). |
| `services[].environments[].pipelines` | list | no | The Effective reading: each pipeline's `name`, `receivers`, `processors`, and `exporters`. |

A Service with no `class` or no `onboarded` date never gets a Grace Period.
The window runs from the onboarding date for the class's duration. Outside
it, including before onboarding, nothing is waived.

An authored row is a known reading, including one that reports no pipelines
at all: that's a collector reporting an empty config, not a blind spot.

## Load errors

Loading fails closed and returns nothing. Each message names the file. Unlike
the requirements library, a directory with no exemption files loads as none:
zero exemptions is the strictest state there is.

The exemptions load refuses on:

- an unknown or misspelled field
- a malformed document, an empty file, or more than one YAML document in the
  file
- a file that is neither one mapping nor a list of mappings
- an `expires` or `onboarded` value that isn't a `2006-01-02` date
- a missing `id`, or an `id` defined in two files
- a missing `requirement`, `owner`, or `expires`
- a subject that isn't exactly one `service` or one `team`
- an exemptions directory that doesn't exist

The estate file load refuses on:

- an unknown or misspelled field
- no `services` at all: an empty estate would pass every check with nothing
  judged
- a grace entry with no class, a class listed twice, or a non-positive window
- a grace window shorter than a weaker class's window
- a Service whose `class` the grace table doesn't define
- a Service with an `onboarded` date but no `class`
- a Service with no name, or deployed to no environment
- an environment with no name, or the same (Service, Environment) row twice
- a pipeline with no name, or the same pipeline name twice in one row

## Authoring findings

An authoring finding means the file is valid and loads, but it can never take
effect. These appear in the `check` report's `authoring_findings` array,
carry both the Exemption id and the Requirement it names, and are never part
of the exit code.

Two are raised:

- The Exemption waives a Requirement the library doesn't hold. It waives
  nothing, and is almost always a typo.
- The Exemption has expired and is still in the tree. Renew it by changing
  the file, or delete the file.
