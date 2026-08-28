---
title: Write an Exemption
description: Waive a finding's count with a named owner and an expiry, without hiding the diagnosis.
order: 9
---

# Write an Exemption

An Exemption waives the count of one Requirement for one Service or one Team
subtree. It never waives the diagnosis: the finding stays visible in the
report, in the drawer, and in every roll-up, marked waived. The count comes
back on its own the day the Exemption expires.

Use one when a gap is agreed, owned, and time-boxed. Don't use one to make a
red build green: everything an Exemption touches stays in the `waived` totals
that every summary carries.

## Write the file

Exemptions live in the estate's `exemptions/` directory, one object per file.
This is the demo estate's `exemptions/search-metrics.yaml`:

```yaml
id: search-metrics-onboarding
requirement: metrics-delivered
owner: platform-observability
service: storefront/search
expires: 2027-01-31
reason: >
  Search is mid-migration to the shared metrics SDK; the platform team owns
  the cutover and has agreed the gap until the end of January.
```

Every field except `reason` has a job:

- `requirement` names exactly one Requirement. One Exemption waives one
  Requirement, always.
- `owner` is the party who answers for the waiver. A waiver nobody answers for
  is not a waiver.
- `expires` is a calendar day, written as `2027-01-31`. The waiver stops
  counting at the UTC midnight that starts that day.
- `service` or `team`, exactly one. `service` waives for one Service. `team`
  waives for a whole subtree, which suits onboarding: one reviewable file
  instead of a copy per Service.

`owner` and `expires` are both mandatory, and loading fails closed. Run this
from the estate root, with a file that omits the expiry:

```sh
./telecraft check -library requirements -estate demo/rows.yaml \
  -exemptions broken-exemptions
```

```
check: invalid exemptions:
  - broken-exemptions/bad.yaml: exemption "search-trace-identity" has no expiry. Every Exemption needs an expiry date, because an open-ended waiver would delete the Requirement
```

The command exits 2. An exemptions directory that won't load never becomes a
run that silently counted findings somebody believes are waived.

## Who can approve it

Nothing in the Exemption file grants itself authority. Validity comes from the
generated `CODEOWNERS` file: the waived Requirement's owner, or that owner's
ancestor team, is a required reviewer on the pull request that adds the file.
Nobody can approve their own Exemption, because the generated review rules
never let them, and no policy has to be remembered.

Renewal is a fresh pull request. There is no extend field, and no way to move
an expiry without the same review that created it.

## What a waived finding looks like

Run the check with `-exemptions` pointing at the directory. The cross produces
nothing but `unknown` unless a backend is reachable, so this run uses the
quickstart's seeded Elasticsearch:

```sh
./telecraft check \
  -library ../estate-demo/requirements \
  -estate ../estate-demo/demo/rows.yaml \
  -exemptions ../estate-demo/exemptions \
  > report.json
```

The finding keeps its outcome, its severity, and its detail. Only `waived` and
`waiver_reason` are added:

```json
{
  "requirement": "metrics-delivered",
  "title": "Service metrics are delivered",
  "requirement_level": "conditionally_required",
  "owner": "platform-observability",
  "outcome": "broken_pipeline",
  "severity": 7,
  "waived": "exempt",
  "waiver_reason": "exemption search-metrics-onboarding: waived by platform-observability until 2027-01-31: Search is mid-migration to the shared metrics SDK; the platform team owns the cutover and has agreed the gap until the end of January.\n",
  "detail": [
    "no metrics received in the last 24h0m0s"
  ],
  "remediation": "Enable metric export in the Service's SDK, or add a host_metrics or prometheus receiver to the collector serving it.\n"
}
```

That is still a broken pipeline, and Telecraft keeps saying so. What changed
is the row's arithmetic: the finding leaves `failing` and joins `waived`, so
the row scores clean while carrying its waiver in the open.

```json
{
  "total": 4,
  "passing": 3,
  "waived": 1,
  "failing": 0,
  "ratio": 1
}
```

The estate summary carries the same total, so a green built on Exemptions is
visibly green on Exemptions:

```json
{
  "rows": 5,
  "failing_rows": 1,
  "counting_failures": 2,
  "waived": 1,
  "library_drift": 0
}
```

Passing findings are never marked waived. There is nothing to forgive, and
waiving them would inflate the number every roll-up keeps visible.

## Waive a whole subtree

A team-scoped Exemption names a subtree instead of a Service:

```yaml
id: storefront-onboarding
requirement: traces-delivered
owner: platform-observability
team: storefront
expires: 2026-12-31
reason: >
  Storefront is onboarding to the shared tracing SDK; the platform team owns
  the cutover for the whole subtree.
```

Resolving a subtree needs the ownership model, so pass `-ownership`:

```sh
./telecraft check \
  -library ../estate-demo/requirements \
  -estate ../estate-demo/demo/rows.yaml \
  -exemptions ../my-exemptions \
  -ownership ../my-ownership
```

```json
{
  "rows": 5,
  "failing_rows": 2,
  "counting_failures": 2,
  "waived": 1,
  "library_drift": 0
}
```

`storefront/catalogue-web` now waives `traces-delivered` and nothing else. Its
`trace-identity` finding still counts, because an Exemption waives exactly one
Requirement:

```
logs-delivered     compliant       waived=null
metrics-delivered  compliant       waived=null
trace-identity     not_delivered   waived=null
traces-delivered   broken_pipeline waived=exempt
```

`-ownership` names a directory holding `teams.yaml` and the authored-object
files beside it. The loader reads every `.yaml` file in that directory other
than `teams.yaml`, `allow-lists.yaml`, `grants.yaml`, and `users.yaml` as an
authored object, so point it at a directory that holds only those. Without
`-ownership`, a team-scoped Exemption is an error, not a waiver that silently
never applies.

## Grace by Service Class

Grace is the other way a finding stops counting, and you don't author it per
Service. The estate declares one table, and Telecraft applies it during a
Service's onboarding window:

```yaml
grace:
  - class: C1
    window: 168h
  - class: C2
    window: 336h
  - class: C3
    window: 720h
```

The window shrinks as the class rises: the most critical Services get the
least forgiveness. The loader enforces that shape, so a table that quietly
gives C1 the longest window can't load.

A Grace Period runs from the Service's onboarding date for its class's
duration. A Service with no class or no onboarding date never enters a window,
and nothing is waived. Grace findings are marked `"waived": "grace"`, with the
end of the window as the reason.

Where both cover the same finding, the Exemption wins, because it names the
party who answers for the waiver.

## Expiry is a property of the clock

An expired Exemption stops matching with no manual step, and the raw finding
is back on the next run. The file still sitting in the tree becomes an
authoring finding:

```json
{
  "requirement": "trace-identity",
  "exemption": "search-trace-identity",
  "message": "expired 2026-06-30 and is still in the tree. To renew it, open a new PR. Otherwise delete the file"
}
```

Every run reports authoring findings, and they never enter the exit code: they
tell you the tree needs tidying, not that the estate is failing. An Exemption
naming a Requirement that is not in the library raises the same kind of
finding, because it waives nothing.

## What next

- [Check conformance](check-conformance.md) covers the report the waived
  finding appears in, and the CI gate it feeds.
- The [reference section](../reference/index.md) has the full Exemption
  schema.
