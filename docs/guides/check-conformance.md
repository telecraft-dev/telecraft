---
title: Check conformance
description: Read your telemetry backend, judge every Service against its requirements, and gate CI on the result.
order: 3
---

# Check conformance

Conformance is the rung that needs nothing but a connection string. You point
`telecraft` at your telemetry backend and at a file describing what each
Service is running, and it tells you which Services are delivering what they
are configured to deliver, and whose problem each gap is.

This guide assumes you have built the CLI and cloned the demo estate as the
[quickstart](quickstart.md) describes. All paths below are relative to your
`telecraft` checkout.

## Read one Service with `observe`

`telecraft observe` is the printer for the Observed reading: what landed in
the backend for one Service over a trailing window. Use it to confirm your
connection settings before you gate anything on them.

```sh
./telecraft observe \
  -service storefront/catalogue-web \
  -environment production \
  -window 24h \
  -attributes service.namespace,deployment.environment.name
```

```
service   storefront/catalogue-web
env       production
provider  elasticsearch
window    24h0m0s
as_of     2026-08-19T10:08:50Z

logs     known=true   present=true  volume=1
         coverage service.namespace            1.00
         coverage deployment.environment.name  1.00
metrics  known=true   present=true  volume=1
         coverage service.namespace            1.00
         coverage deployment.environment.name  1.00
traces   known=true   present=false volume=0

logs attribute names (sampled 1 of 1 records): deployment.environment.name, service.name
metrics attribute names (sampled 1 of 1 records): deployment.environment.name, service.name
traces attribute names (sampled 0 of 0 records):
```

Read the `known` column first. `known=true present=false` means the backend
answered and nothing arrived. That is a reading. When the backend cannot
answer, the same line says so and names the cause:

```
logs     known=false  cause="backend unreachable: Post \"http://localhost:9200/_msearch\": dial tcp [::1]:9200: connect: connection refused"
```

`observe` is a printer, not a gate. It exits 0 for every reading including a
degraded one. Scripting against presence belongs to `check`.

Connection settings come from flags or from the environment:
`TELECRAFT_TELEMETRY_ENDPOINT` and `TELECRAFT_TELEMETRY_API_KEY`. Which
backend answers is wiring inside the provider tree; the command itself holds
only neutral settings. The full flag list is in the
[reference section](../reference/index.md).

## Judge the estate with `check`

`check` is the CI mode. It evaluates every row of the estate once, writes one
machine-readable report to stdout, and exits non-zero exactly when counting
failures exist.

```sh
./telecraft check \
  -library ../estate-demo/requirements \
  -estate ../estate-demo/demo/rows.yaml \
  -exemptions ../estate-demo/exemptions \
  -source ../estate-demo \
  -catalogue ../estate-demo/catalogues/catalogue-v0.158.0.json \
  > report.json
```

The four inputs:

- `-library` is the requirements directory. A Requirement is a versioned
  assertion about configuration, about signal, or about both, and every one
  carries remediation text.
- `-estate` is the file holding each Service's Effective reading per
  Environment: the running configuration a collector reports, pipelines with
  component order preserved. One Service in two Environments is two rows,
  judged independently.
- `-exemptions` is optional, and covered in [write an
  Exemption](exemptions.md).
- `-source` and `-catalogue` go together and are optional. They add
  `library_drift` detection over the authored estate: configuration in git
  that passes the version it claims or pins while failing the current bar.

## Read the report

The summary is the top-level answer:

```json
{
  "rows": 5,
  "failing_rows": 1,
  "counting_failures": 4,
  "waived": 1,
  "library_drift": 2
}
```

`counting_failures` greater than zero is exactly the non-zero exit.
`library_drift` rides `counting_failures` and is broken out beside it, so a
gate red on drift alone is visibly red on drift. `waived` stays visible at
every level, so a green built on Exemptions cannot look like a clean green.

Each row carries its score and its findings:

```json
{
  "service": "storefront/catalogue-web",
  "environment": "production",
  "worst": "broken_pipeline",
  "score": {
    "total": 4,
    "passing": 2,
    "waived": 0,
    "failing": 2,
    "ratio": 0.5
  }
}
```

### The outcomes

Each finding carries an `outcome` and its `severity` rung. The ordering is
worst first, and it is the same ordering every badge and every roll-up sorts
on.

| Outcome | Severity | What it means |
|---|---|---|
| `broken_pipeline` | 7 | Configured yes, observed no. Somebody meant this to work and it is not working. |
| `not_configured` | 6 | Configured no, observed no. The owner needs to instrument. |
| `not_delivered` | 5 | Observed no, with no configuration evidence to explain why. |
| `misconfigured` | 4 | A configuration assertion failed with no signal reading to cross it against. |
| `library_drift` | 3 | Passes the version it claims or pins, fails the current one. The goalposts moved. |
| `unknown` | 2 | No evidence from any reading. |
| `ungoverned` | 1 | Observed yes, configured no. Passes, but surfaced: telemetry is arriving from something nobody configured. |
| `compliant` | 0 | Met. |

`broken_pipeline` leads because it is the finding no configuration-only tool
can produce:

```json
{
  "requirement": "traces-delivered",
  "requirement_level": "required",
  "owner": "platform-observability",
  "outcome": "broken_pipeline",
  "severity": 7,
  "detail": [
    "no traces received in the last 24h0m0s"
  ],
  "remediation": "Instrument the Service with an OpenTelemetry SDK or auto-instrumentation agent and point it at the collector's OTLP receiver. Spans arriving with no receiver configured means something is bypassing the managed collector.\n"
}
```

### The repo's own section

`library_drift` findings are owned by authored configuration, never by a row,
so they land in their own section with the team that owns them:

```json
{
  "facet": "component",
  "team": "data-flow",
  "owner": "gateway-owners",
  "blueprint": "data-flow/gateway-standard",
  "lane": "traces, logs",
  "outcome": "library_drift",
  "severity": 3,
  "message": "pins infosec/pii-redaction@2, but the owning team's head is version 3 — the reference passes the version it pins while the world has moved; a component update is available (ADR-0026 §2, §7)",
  "remediation": "review the infosec/pii-redaction v2→v3 config diff and bump the pin in a PR — git is the source of truth, there is no live mutation (ADR-0026 §2)"
}
```

An `authoring_findings` section carries problems with the authored inputs
themselves, such as an Exemption naming a Requirement that is not in the
library. Those are reported in every run and never enter the exit code.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Every counting finding passes. |
| 1 | Counting failures exist. |
| 2 | The check could not run: usage, a load error, or wiring. |

Exit 2 is the important one. A library that fails to load has judged nothing,
so it is never a lenient 0:

```sh
./telecraft check -library ../estate-demo/nope -estate ../estate-demo/demo/rows.yaml
```

```
check: requirements library directory ../estate-demo/nope does not exist
```

The same fail-closed rule covers the inputs that loosen the exit code. An
exemptions directory that will not load is exit 2, never a run that silently
counted findings somebody believes are waived:

```
check: invalid exemptions:
  - broken-exemptions/bad.yaml: exemption "search-trace-identity" has no expiry — mandatory (REQ-014): an open-ended waiver is a deleted requirement
```

### Unknown counts as a failure

An `unknown` outcome does not pass. It is not rounded up to green and it is
not rounded down to a specific diagnosis: it is reported as itself, and it
counts. Point the check at a backend that is not there and every production
row goes red:

```json
{
  "rows": 5,
  "failing_rows": 4,
  "counting_failures": 4,
  "waived": 0,
  "library_drift": 0
}
```

```json
{
  "requirement": "trace-identity",
  "outcome": "unknown",
  "severity": 2,
  "detail": [
    "traces reading unavailable: backend unreachable: Post \"http://localhost:9200/_msearch\": dial tcp [::1]:9200: connect: connection refused"
  ]
}
```

This is what stops a broken credential from reading as an estate that got
better overnight.

## Narrow to one Environment

Every row is judged by default. A gate that silently checked only production
would pass estates failing everywhere else, and the report already leads with
production rows under any lens.

`-environment` narrows a run when you want one lens:

```sh
./telecraft check \
  -library ../estate-demo/requirements \
  -estate ../estate-demo/demo/rows.yaml \
  -environment staging
```

```json
{
  "rows": 1,
  "failing_rows": 0,
  "counting_failures": 0,
  "waived": 0,
  "library_drift": 0
}
```

An Environment with no rows is exit 2, not a vacuous pass:

```sh
./telecraft check -library ../estate-demo/requirements \
  -estate ../estate-demo/demo/rows.yaml -environment qa
```

```
check: the estate has no row in environment "qa" — a gate judging nothing would pass vacuously
```

## Wire it into CI

The gate is the exit code, so the CI step is the command. Conformance that can
only be seen in a browser is conformance that regresses between people
remembering to look.

```yaml
name: Conformance

on:
  pull_request:
  schedule:
    - cron: "0 * * * *"

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Judge the estate
        env:
          TELECRAFT_TELEMETRY_ENDPOINT: ${{ vars.TELECRAFT_TELEMETRY_ENDPOINT }}
          TELECRAFT_TELEMETRY_API_KEY: ${{ secrets.TELECRAFT_TELEMETRY_API_KEY }}
        run: |
          go run ./cmd/telecraft check \
            -library requirements \
            -estate demo/rows.yaml \
            -exemptions exemptions \
            -source . \
            -catalogue catalogues/catalogue-v0.158.0.json \
            | tee report.json

      - name: Keep the report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: conformance-report
          path: report.json
```

Three things make this behave:

1. Upload the report with `if: always()`, so a red run still leaves the
   evidence behind.
2. Give the job the backend credentials it needs. Without them the run exits 1
   on `unknown`, which is correct but tells you nothing about the estate.
3. Do not add `|| true`. The exit code is the whole gate.

A scheduled run matters as much as the pull-request run: `broken_pipeline`
appears when a Service stops delivering, which is rarely the moment somebody
opens a pull request.

## What next

- [Write an Exemption](exemptions.md) when a finding is agreed, owned and
  time-boxed.
- [Author and render](author-and-render.md) puts the configuration those rows
  report under version control.
