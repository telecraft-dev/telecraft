---
title: Command line reference
description: Every Telecraft binary and subcommand, with its flags, defaults, exit codes and a minimal example.
order: 2
---

# Command line reference

Telecraft ships three binaries: `telecraft`, the platform command line;
`catalogue-import`, the Catalogue import pipeline; and `blueprint-check`, the
strict Blueprint and Component loader. All three build from this repository
with the Go toolchain:

```sh
go build ./cmd/telecraft
go build ./cmd/catalogue-import
go build ./cmd/blueprint-check
```

You can also run them without installing:

```sh
go run ./cmd/telecraft render -estate ESTATE_DIR -catalogue ARTEFACT -commit SHA
```

Throughout this page, `ESTATE_DIR` is an estate checkout (see
[Estate layout](estate-layout.md)), `ARTEFACT` is the path of a Catalogue
artefact such as `catalogues/catalogue-v0.158.0.json` (see
[Catalogue](catalogue.md)), and `SHA` is the commit the estate is read at.

## telecraft

`telecraft` dispatches on its first argument. With no arguments, or with an
argument that isn't a known subcommand, it prints the usage summary to stderr
and exits `2`.

| Subcommand | What it does |
|---|---|
| `observe` | Print the Observed readings for one Service over a trailing window. |
| `check` | Evaluate the estate once and write one machine-readable report. |
| `palette` | Print one team's effective palette with provenance. |
| `render` | Compile every Tier's bound Blueprint to the rendered artefact tree. |
| `serve` | Run the stateless OpAMP server. |
| `snapshot` | Write the console API snapshot for one estate checkout. |
| `delivery` | Print one collector's delivery status from two files. |
| `passwd` | Hash one basic-auth secret for `users.yaml`. |

Every subcommand accepts `-h`, which prints that subcommand's flags to stderr
and exits `2`.

### Environment variables

`TELECRAFT_TELEMETRY_ENDPOINT`
: Default for `-endpoint` on `observe` and `check`. When unset, the default is
  `http://localhost:9200`.

`TELECRAFT_TELEMETRY_API_KEY`
: Default for `-api-key` on `observe` and `check`. When unset, the default is
  empty.

An explicit flag always beats the environment variable.

## telecraft observe

Reads one Service's Observed state through the telemetry provider seam and
prints it: per signal, whether the reading is known, whether the signal is
present, its volume, and the coverage of each attribute you asked about. It
then prints the attribute names each signal carries, with the sample size.

`observe` is a printer, not a gate. A degraded reading prints with its cause
and the command still exits `0`. Script against outcomes with `check`.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-service` | string | none, required | `service.name` of the Service to read. |
| `-environment` | string | empty | Narrow the reading to one Environment. |
| `-window` | duration | `15m` | Trailing window the reading covers. |
| `-attributes` | string | empty | Comma-separated attribute names to measure coverage for. |
| `-endpoint` | string | `TELECRAFT_TELEMETRY_ENDPOINT`, else `http://localhost:9200` | Telemetry backend base URL. |
| `-api-key` | string | `TELECRAFT_TELEMETRY_API_KEY`, else empty | Telemetry backend API key. |
| `-timeout` | duration | `30s` | Overall deadline for the readings. |

| Exit code | Meaning |
|---|---|
| `0` | The readings were printed, degraded readings included. |
| `2` | Usage error, `-service` missing, or the provider could not be wired. |

```sh
telecraft observe -service checkout -environment production -window 1h
```

## telecraft check

The CI mode. It loads the requirements library and the estate file, reads
Observed state once per row and window, judges every row, writes one JSON
report to stdout, and exits non-zero exactly when counting failures exist.

Every row is judged by default. `-environment` narrows the run to one
Environment; the report always orders `production` rows first. If the estate
has no row in the named Environment, the run fails with exit `2` rather than
passing vacuously.

Waivers loosen the exit code, so their inputs fail closed: an exemptions
directory or ownership directory that doesn't load is exit `2`. A waived
finding keeps its outcome and detail in the report and gives up only its
count.

`-source` and `-catalogue` go together and enable `library_drift` detection
over the authored estate. Supplying one without the other is a usage error.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-library` | string | none, required | Requirements library directory. |
| `-estate` | string | none, required | Estate file: services and their per-environment Effective reading. |
| `-exemptions` | string | empty | Exemptions directory. Empty means no authored waivers. |
| `-ownership` | string | empty | Ownership directory holding `teams.yaml` and the authored objects. Needed only to resolve team-scoped exemptions. |
| `-source` | string | empty | Authored estate root holding `teams/` and `rendered/`. Enables `library_drift` detection; needs `-catalogue`. |
| `-catalogue` | string | empty | Path to the active Catalogue artefact. Enables `library_drift` detection; needs `-source`. |
| `-endpoint` | string | `TELECRAFT_TELEMETRY_ENDPOINT`, else `http://localhost:9200` | Telemetry backend base URL. |
| `-api-key` | string | `TELECRAFT_TELEMETRY_API_KEY`, else empty | Telemetry backend API key. |
| `-environment` | string | empty | Narrow the check to one Environment. Empty judges every row. |
| `-timeout` | duration | `5m` | Overall deadline for the run. |

| Exit code | Meaning |
|---|---|
| `0` | Every counting finding passes. |
| `1` | Counting failures exist. |
| `2` | The check could not run: usage, load or wiring error. |

The report is one JSON document on stdout with these top-level fields:

| Field | Type | Description |
|---|---|---|
| `evaluated_at` | string | The instant the run judged at, in UTC. |
| `provider` | string | Name of the telemetry provider that answered. |
| `rows` | array | One entry per judged row, each with `service`, `environment`, `worst`, `score` and `findings`. |
| `authoring_findings` | array | Problems with authored content that can never take effect. Never part of the exit code. |
| `library_drift` | array | `library_drift` findings, owned by authored config rather than by a row. |
| `housekeeping` | array | Stale-but-passing claim nudges. Never part of the exit code. |
| `summary` | object | `rows`, `failing_rows`, `counting_failures`, `waived` and `library_drift` totals. |

`summary.counting_failures` greater than zero is exactly the non-zero exit.

```sh
telecraft check -library requirements/ -estate estate.yaml
```

## telecraft palette

Prints one team's effective palette: the components of the active Catalogue
the team may use, each with the provenance that admitted it. See
[Allow-lists and Grants](allow-lists.md) for the resolution rules and the
`default-allow`, `allow-list` and `grant` origins.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-team` | string | none, required | Team id to print the palette for. |
| `-estate` | string | none, required | Estate directory holding `teams.yaml` and the policy files. |
| `-catalogue` | string | none, required | Path to the active Catalogue artefact. |

| Exit code | Meaning |
|---|---|
| `0` | The palette was printed. |
| `2` | Usage error, or the team tree, Catalogue or policy failed to load. |

```sh
telecraft palette -team data-flow -estate ESTATE_DIR -catalogue ARTEFACT
```

## telecraft render

Compiles every Tier's bound Blueprint into the rendered artefact tree and the
generated code-ownership projection, writing them under `-out`. The output is
a pure function of the estate at `-commit`, which is what lets CI recompute
`rendered/` and diff it against what's committed.

Findings ride along without blocking: each is printed on stdout after the
written paths, prefixed `finding`. Exactly one policy rule refuses the render,
the allow-list hard block, alongside mechanical invalidity such as a reference
that resolves to nothing or a rendered-id collision.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | none, required | Estate root holding `teams.yaml` and the `teams/` tree. |
| `-catalogue` | string | none, required | Path to the active Catalogue artefact. |
| `-commit` | string | none, required | Commit SHA stamped into every artefact. |
| `-out` | string | the value of `-estate` | Directory to write `rendered/` and `CODEOWNERS` under. |

| Exit code | Meaning |
|---|---|
| `0` | Rendered. Policy findings, if any, are printed and don't block. |
| `1` | The render refused, or an artefact could not be written. |
| `2` | Usage error, or the team tree, Catalogue, policy, sources, topology or self-telemetry declaration failed to load. |

```sh
telecraft render -estate ESTATE_DIR -catalogue ARTEFACT -commit SHA
```

## telecraft serve

Runs the stateless OpAMP server: it serves the estate's rendered artefacts
from git, matching each collector's reported identifying attributes against
the Tier selectors at head, and stores nothing durable. The OpAMP endpoint
listens at `/v1/opamp` on the `-listen` address.

Exactly one of `-estate` or `-repo` names the source. `-estate` points at a
local checkout, the standalone and air-gapped shape. `-repo` names a git URL,
including a `file:///` URL, fetched on the `-fetch-interval` poll.

The server stops on `SIGINT` or `SIGTERM` and is given 10 seconds to shut
down.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | empty | Local estate checkout to serve. |
| `-repo` | string | empty | Git URL of the estate repository to fetch and serve. |
| `-cache` | string | a fresh temporary directory, removed on exit | Directory the fetched clone lives in. |
| `-listen` | string | `127.0.0.1:4320` | `host:port` the OpAMP endpoint listens on. |
| `-fetch-interval` | duration | `30s` | Repository snapshot poll interval. |

| Exit code | Meaning |
|---|---|
| `0` | Clean signal-driven shutdown. |
| `1` | The server could not start, or could not stop cleanly. |
| `2` | Usage error, including naming neither or both sources, or a bad configuration. |

```sh
telecraft serve -estate ESTATE_DIR -listen 0.0.0.0:4320
```

## telecraft snapshot

Writes the console API snapshot: the JSON documents the platform API would
serve, computed by the real evaluators over one estate checkout. It's a pure
function of the estate at `-commit` and the readings the estate declares.

`-rows` names the conformance estate file, each Service's Effective reading
per Environment. It's the same file `check` takes as `-estate`, and
[Exemptions](exemptions.md) documents its fields. `-readings` names the
readings file, which declares the runtime readings a repository cannot
hold: the collector estate, the arrivals, and each Tier's flow.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | none, required | Estate root holding `teams.yaml`, the `teams/` tree and `rendered/`. |
| `-catalogue` | string | none, required | Path to the active Catalogue artefact. |
| `-library` | string | none, required | Requirements library directory. |
| `-rows` | string | none, required | Conformance estate file. |
| `-readings` | string | none, required | Readings file. |
| `-commit` | string | none, required | Commit SHA the snapshot is taken at. |
| `-team` | string | none, required | The presented user's team: the shelf's resting scope. |
| `-catalogues` | string | the directory holding `-catalogue` | Directory of installed Catalogue artefacts. |
| `-exemptions` | string | empty | Exemptions directory. |
| `-repository` | string | empty | Estate repository name, shown as the source link. |
| `-user` | string | `demo-user` | Id of the user the snapshot presents as signed in. |
| `-user-name` | string | `Demo user` | Display name of that user. |
| `-user-email` | string | `<user>@estate.internal` | Attribution address of that user. |
| `-out` | string | empty, meaning stdout | File the snapshot is written to. |

Only files named `catalogue-*.json` in the `-catalogues` directory are read as
installed artefacts.

| Exit code | Meaning |
|---|---|
| `0` | Written. |
| `1` | The snapshot could not be built or written, including a `rendered/` tree that no longer matches the sources. |
| `2` | Usage error, or an input failed to load. |

```sh
telecraft snapshot -estate ESTATE_DIR -catalogue ARTEFACT -library requirements/ \
  -rows estate.yaml -readings readings.yaml -commit SHA -team data-flow -out snapshot.json
```

## telecraft delivery

Prints one collector's delivery status from two files: the Intended config,
which is the rendered artefact in git, and the collector's reported Effective
config. `-path` names the collector's delivery path and selects the Mutation
profile the comparison runs under.

A file comparison carries no `RemoteConfigStatus` reading, so the remote axis
always prints `known=false` with its cause. Like `observe`, this is a printer:
it exits `0` for every computed status, drift included.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-intended` | string | none, required | Path to the Intended config. |
| `-effective` | string | none, required | Path to the reported Effective config. |
| `-path` | string | none, required | The collector's delivery path: `served` or `git`. |

| Exit code | Meaning |
|---|---|
| `0` | A status was computed and printed, including a drifted one. |
| `2` | Usage error, an unknown `-path` value, an unreadable file, or the status could not be computed. |

```sh
telecraft delivery -intended rendered/data-flow/gateway.yaml \
  -effective reported.yaml -path served
```

## telecraft passwd

Hashes one basic-auth secret for the `password` field of a `users.yaml` entry.
The secret is read from stdin, never from an argument, so it doesn't land in
shell history. `passwd` takes no flags and rejects positional arguments.

The printed hash has four `$`-separated parts: the algorithm, the iteration
count, the base64 salt and the base64 derived key.

| Exit code | Meaning |
|---|---|
| `0` | The hash was printed. |
| `1` | Reading the secret from stdin failed. |
| `2` | A positional argument was given, or the secret was empty. |

```sh
printf 'SECRET' | telecraft passwd
```

```
pbkdf2-sha256$600000$9WjDMLabnekbxwNlJaUxlg$qmNB8MhgWk4UiXZt1v13di5Ko4W8ElkBic/SgfKrq24
```

## catalogue-import

Runs the Catalogue import pipeline: it fetches
`opentelemetry-collector-contrib` at a pinned release tag, walks every
`metadata.yaml`, and writes one atomic, versioned Catalogue artefact plus a
coverage report of what was found, excluded and missing.

The fetch is a sparse, depth-1 checkout of only `metadata.yaml` and `go.mod`
into a temporary directory, removed afterwards. Re-running against the same
tag is idempotent: the artefact is byte-identical and left untouched. A
different tag writes a new artefact beside the old one.

Use `-source` to import an already-fetched tree offline. A tree copied without
its `.git` still imports; the artefact then records no source commit.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-tag` | string | none, required | Collector release tag to import, such as `v0.158.0`. |
| `-out` | string | `catalogues` | Directory the versioned artefact is written to. |
| `-repo` | string | `https://github.com/open-telemetry/opentelemetry-collector-contrib` | Source repository URL. |
| `-source` | string | empty | Import an existing checkout instead of fetching. |

| Exit code | Meaning |
|---|---|
| `0` | The artefact was written, or already held this import byte for byte. |
| `1` | `-tag` was missing, or the fetch, import or write failed. |
| `2` | An unknown flag was given. |

```sh
catalogue-import -tag v0.158.0
```

## blueprint-check

Strict-loads every Blueprint and shared Component in an estate's source roots
and prints the findings: references to missing or retracted Components or
versions, misplaced extensions, and lane orderings that contradict the shipped
ordering rules.

Each root holds the `teams/<team>/{components,blueprints}` layout. Several
roots are the primary-plus-satellites source set. With no argument the current
directory is checked.

Mechanical invalidity refuses the load and exits `1`. Findings are printed and
exit `0`: they route to owners and advise, and never block.

```sh
blueprint-check ESTATE_DIR
```

```
loaded 2 shared Components, 2 Blueprints
  blueprint data-flow/edge-standard@1 (owner gateway-owners)
  blueprint data-flow/gateway-standard@4 (owner gateway-owners)
no findings
```

| Exit code | Meaning |
|---|---|
| `0` | The sources loaded, with or without findings. |
| `1` | The load refused. |
| `2` | An unknown flag was given. |
