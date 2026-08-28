---
title: Command line reference
description: Every Telecraft binary and subcommand, with its flags, defaults, exit codes and a minimal example.
order: 2
---

# Command line reference

Telecraft ships five binaries: `telecraft`, the main command line;
`catalogue-import` and `schema-registry-import`, the two substrates of the one
import pipeline; `blueprint-check`, the strict Blueprint and Component
loader; and `register-check`, the strict loader for the register a deployment
serving several Organisations reads. All five build from this repository with
the Go toolchain:

```sh
go build ./cmd/telecraft
go build ./cmd/catalogue-import
go build ./cmd/schema-registry-import
go build ./cmd/blueprint-check
go build ./cmd/register-check
```

You can also run them without installing:

```sh
go run ./cmd/telecraft render -estate ESTATE_DIR -catalogue ARTEFACT -commit SHA
```

On this page, `ESTATE_DIR` is an estate checkout (see
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
| `serve` | Run the Instance server: the console, the API and the OpAMP endpoint. |
| `snapshot` | Write the console API snapshot for one estate checkout. |
| `delivery` | Print one collector's delivery status from two files. |
| `activate` | Show what activating an imported version would change, and designate it. |
| `licence` | Print the Edition this build is running, and what a licence file says. |
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

Every flag of `serve` has one too: `TELECRAFT_` plus the flag name
upper-cased, with dashes as underscores, so `-external-url` reads
`TELECRAFT_EXTERNAL_URL`.

No environment variable on `serve` carries secret material. The estate names
what it needs and the deployment places a file of that name under
`-secrets-dir`; the process's own secrets take a file path. `check` and
`observe` are short-lived and keep `TELECRAFT_TELEMETRY_API_KEY`.

An explicit flag always wins over the environment variable.

## telecraft observe

Reads one Service's Observed state from the telemetry backend and prints it.
For each signal, it prints whether the reading is known, whether the signal is
present, its volume, and the coverage of each attribute you asked about. It
then prints the attribute names each signal carries, with the sample size.

`observe` prints; it doesn't gate. A degraded reading prints with its cause
and the command still exits `0`. To script against outcomes, use `check`.

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
| `2` | Usage error, `-service` missing, or the telemetry provider could not be set up. |

```sh
telecraft observe -service checkout -environment production -window 1h
```

## telecraft check

The CI mode. It loads the requirements library and the estate file, reads
Observed state once per row and window, judges every row, writes one JSON
report to stdout, and exits non-zero exactly when counting failures exist.

Every row is judged by default. `-environment` narrows the run to one
Environment; the report always lists `production` rows first. If the estate
has no row in the named Environment, the run fails with exit `2` rather than
passing with nothing judged.

Waivers loosen the exit code, so their inputs fail closed: an exemptions
directory or ownership directory that doesn't load is exit `2`. A waived
finding keeps its outcome and detail in the report and gives up only its
count.

`-source` and `-catalogue` go together and turn on `library_drift` detection
over the authored estate. Supplying one without the other is a usage error.

`-schema-registries` names the directory a `schema_conformance` reference
resolves against, holding one `schema-registry-<ref>.json` per imported
version. A library that references a version and is loaded without the
directory is a load error and exit `2`, never a run that scored every Service
against a floor nobody checked. See
[Schema conformance assertions](requirements.md#schema-conformance-assertions).

| Flag | Type | Default | Description |
|---|---|---|---|
| `-library` | string | none, required | Requirements library directory. |
| `-schema-registries` | string | empty | Directory of installed Schema Registry artefacts. A library holding a `schema_conformance` Requirement needs it; one that holds none is unaffected. |
| `-estate` | string | none, required | Estate file: services and their per-environment Effective reading. |
| `-exemptions` | string | empty | Exemptions directory. Empty means no authored waivers. |
| `-ownership` | string | empty | Ownership directory holding `teams.yaml` and the authored objects. Needed only to resolve team-scoped exemptions. |
| `-source` | string | empty | Authored estate root holding `teams/` and `rendered/`. Turns on `library_drift` detection; needs `-catalogue`. |
| `-catalogue` | string | empty | Path to the active Catalogue artefact. Turns on `library_drift` detection; needs `-source`. |
| `-endpoint` | string | `TELECRAFT_TELEMETRY_ENDPOINT`, else `http://localhost:9200` | Telemetry backend base URL. |
| `-api-key` | string | `TELECRAFT_TELEMETRY_API_KEY`, else empty | Telemetry backend API key. |
| `-environment` | string | empty | Narrow the check to one Environment. Empty judges every row. |
| `-timeout` | duration | `5m` | Overall deadline for the run. |

| Exit code | Meaning |
|---|---|
| `0` | Every counting finding passes. |
| `1` | Counting failures exist. |
| `2` | The check could not run: usage, load, or wiring error. |

The report is one JSON document on stdout with these top-level fields:

| Field | Type | Description |
|---|---|---|
| `evaluated_at` | string | The instant the run judged at, in UTC. |
| `provider` | string | Name of the telemetry provider that answered. |
| `rows` | array | One entry per judged row, each with `service`, `environment`, `worst`, `score`, and `findings`. |
| `authoring_findings` | array | Problems with authored content that can never take effect. Never part of the exit code. |
| `library_drift` | array | `library_drift` findings, owned by authored config rather than by a row. |
| `housekeeping` | array | Stale-but-passing claim nudges. Never part of the exit code. |
| `summary` | object | `rows`, `failing_rows`, `counting_failures`, `waived`, and `library_drift` totals. |

`summary.counting_failures` greater than zero is exactly what makes the exit
code non-zero.

```sh
telecraft check -library requirements/ -estate estate.yaml
```

## telecraft palette

Prints one team's effective palette: the components of the active Catalogue
the team can use, each with the provenance that admitted it. See
[Allow-lists and Grants](allow-lists.md) for the resolution rules and the
`default-allow`, `allow-list`, and `grant` origins.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-team` | string | none, required | Team id to print the palette for. |
| `-estate` | string | none, required | Estate directory holding `teams.yaml` and the policy files. |
| `-catalogue` | string | none, required | Path to the active Catalogue artefact. |

| Exit code | Meaning |
|---|---|
| `0` | The palette was printed. |
| `2` | Usage error, or the team tree, Catalogue, or policy failed to load. |

```sh
telecraft palette -team data-flow -estate ESTATE_DIR -catalogue ARTEFACT
```

## telecraft render

Compiles every Tier's bound Blueprint into the rendered artefact tree and the
generated `CODEOWNERS` file, and writes them under `-out`. The output depends
only on the estate at `-commit`, so CI can recompute `rendered/` and diff it
against what's committed.

Findings don't block. Each is printed on stdout after the written paths,
prefixed `finding`. Exactly one policy rule refuses the render: the
Allow-list check. Mechanical problems also refuse it, such as a reference
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
| `2` | Usage error, or the team tree, Catalogue, policy, sources, topology, or self-telemetry declaration failed to load. |

```sh
telecraft render -estate ESTATE_DIR -catalogue ARTEFACT -commit SHA
```

## telecraft serve

Runs the Instance server: one process over one estate. On the `-http` address
it serves the console, the platform API under `/api/v1/`, and the two probes
`/healthz` and `/readyz`. On the `-listen` address it serves the OpAMP
endpoint at `/v1/opamp`, matching each collector's reported identifying
attributes against the Tier selectors at head. It stores nothing durable.

Name the source with exactly one of `-estate` or `-repo`. `-estate` points at
a local checkout, which suits standalone and air-gapped use. `-repo` names a
git URL, including a `file:///` URL, which the server fetches every
`-fetch-interval`.

The HTTP address is always open, because it carries the probes. An empty
`-listen` closes the OpAMP endpoint.

Both endpoints speak plain HTTP, and the process holds no certificate: TLS
terminates in front. `-external-url` declares the URL the Instance is reached
at. Its scheme decides whether session cookies are marked Secure, and it is
the base a redirect sign-in returns to. A non-loopback host over plain HTTP is
refused unless `-insecure-http` is given.

Secret material is read from files, never from a flag or an environment
variable. `auth.yaml` names a secret; the deployment places a file of that
name under `-secrets-dir`. A named secret with no file stops the start. A
file path this command defaulted to, with nothing at it, is an absence, and
the capability that needed it declares itself unavailable.

`-licence-file` names a licence, and no licence is the ordinary case: the
Instance is then Standard Edition, which is the whole free product. The
licence is not a secret and has no default path, so nothing is read that the
flag did not name. It is verified against keys inside the binary, with no
network involved at any point, and a file that is not accepted is one line on
the terminal and changes nothing else: the server starts, the probes answer,
and collectors are served. See [Licensing](../guides/licensing.md).

The server stops on `SIGINT` or `SIGTERM` and has 10 seconds to shut down.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | empty | Local estate checkout to serve. |
| `-repo` | string | empty | Git URL of the estate repository to fetch and serve. |
| `-cache` | string | a fresh temporary directory, removed on exit | Directory the fetched clone lives in. |
| `-http` | string | `127.0.0.1:4321` | `host:port` the console, the API and the probes listen on. |
| `-listen` | string | `127.0.0.1:4320` | `host:port` the OpAMP endpoint listens on; empty closes it. |
| `-external-url` | string | `http://` and the `-http` address | The URL a browser reaches this Instance at. |
| `-insecure-http` | bool | `false` | Admit an external URL naming a non-loopback host over plain HTTP. |
| `-fetch-interval` | duration | `30s` | Repository snapshot poll interval. |
| `-window` | duration | `15m` | Trailing window the arrival readings cover. |
| `-secrets-dir` | string | empty | Directory the deployment placed the secrets the estate names in. |
| `-session-key-file` | string | `session-key` under `-secrets-dir` | File holding the session signing key, at least 32 bytes. Nothing placed draws one at start, so a restart signs everybody out. |
| `-telemetry-endpoint` | string | empty | Telemetry backend base URL. Empty takes no arrival reading. |
| `-telemetry-key-file` | string | `telemetry-key` under `-secrets-dir` | File holding the telemetry backend credential. |
| `-licence-file` | string | empty | File holding the Enterprise Edition licence. None named runs Standard Edition. |

| Exit code | Meaning |
|---|---|
| `0` | Clean signal-driven shutdown. |
| `1` | The server could not start, or could not stop cleanly. |
| `2` | Usage error, including naming neither or both sources, or a bad configuration. |

```sh
telecraft serve -estate ESTATE_DIR \
  -http 0.0.0.0:4321 -listen 0.0.0.0:4320 \
  -external-url https://telecraft.example
```

The console is served from inside the binary. A binary built without one
answers the console route with a page saying so and serves the API as usual;
[Run an Instance](../guides/run-an-instance.md) has the build steps that put
it there.

## telecraft snapshot

Writes the console API snapshot: the JSON documents the Telecraft API would
serve, computed by the real evaluators over one estate checkout. The output
depends only on the estate at `-commit` and the readings the estate declares.

`-rows` names the conformance estate file, which holds each Service's
Effective reading per Environment. It's the same file `check` takes as
`-estate`, and [Exemptions](exemptions.md) documents its fields. `-readings`
names the readings file, which declares the runtime readings a repository
can't hold: the collector estate, the arrivals, and each Tier's flow.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | none, required | Estate root holding `teams.yaml`, the `teams/` tree, and `rendered/`. |
| `-catalogue` | string | none, required | Path to the active Catalogue artefact. |
| `-library` | string | none, required | Requirements library directory. |
| `-rows` | string | none, required | Conformance estate file. |
| `-readings` | string | none, required | Readings file. |
| `-commit` | string | none, required | Commit SHA the snapshot is taken at. |
| `-team` | string | none, required | The team of the user the snapshot presents. The console's shelf starts on this team's Tiers. |
| `-catalogues` | string | the directory holding `-catalogue` | Directory of installed Catalogue artefacts. |
| `-schema-registries` | string | empty | Directory of installed Schema Registry artefacts, which a `schema_conformance` reference resolves against. |
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
which is the rendered artefact in git, and the Effective config the collector
reports. `-path` names the collector's delivery path and selects the Mutation
profile the comparison runs under.

A file comparison has no `RemoteConfigStatus` reading, so the remote axis
always prints `known=false` with its cause. Like `observe`, this command
prints rather than gates: it exits `0` for every computed status, drift
included.

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

## telecraft activate

Designates which imported version of the Catalogue or the Schema Registry
your estate is judged against, after showing you what activating it would
change. Nothing is designated without `-confirm`: the run that shows you the
report is not the run that changes the estate.

`-confirm` writes `activations.yaml` in the estate directory, carrying the
active version, who decided it, when, and the report the decision was taken
on. Commit that file: the review is the audit.

The Schema Registry report this command shows is the version diff. Which
Services stop passing needs a reading of landed telemetry, which this command
takes none of, and the report says so. The console shows both halves.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-estate` | string | none, required | Estate directory holding `teams.yaml` and the authored objects. |
| `-substrate` | string | none, required | `catalogue` or `schema-registry`. |
| `-version` | string | none, required | The imported version to activate. |
| `-artefacts` | string | the substrate's directory under `-estate` | Directory of installed artefacts to read the two versions from. |
| `-by` | string | none | The Owner deciding the activation. Required with `-confirm`. |
| `-confirm` | bool | `false` | Record the activation, after reading the report. |

| Exit code | Meaning |
|---|---|
| `0` | The report was computed, and the version activated with `-confirm`. |
| `1` | The activation was refused: the version is already active, is not imported, or the report is not a change from the active version. |
| `2` | Usage error, or the estate failed to load. |

```sh
telecraft activate -estate ESTATE_DIR -substrate catalogue -version v0.159.0
telecraft activate -estate ESTATE_DIR -substrate catalogue -version v0.159.0 \
  -confirm -by platform-observability
```

## telecraft licence

Prints the Edition this build is running and what a licence file says: the
licensee, the licence id, the dates, the Entitlements it grants, and the path
it was read from. With no file it prints the Standard Edition line and stops.

It reports rather than judges, so it exits `0` whatever it finds. A file that
was not accepted is a fact about the file, not a failure of the command.

Nothing here reaches a network. Verification is a function of the file, the
keys compiled into the binary and the host clock, so it prints the same answer
on a machine that has never had a route out.

| Flag | Type | Default | Description |
|---|---|---|---|
| `-licence-file` | string | empty | The licence file to read. None named is Standard Edition. |

| Exit code | Meaning |
|---|---|
| `0` | Always, whatever the file says. |
| `2` | Usage error. |

```sh
telecraft licence -licence-file /run/licence/acme.licence
```

```
Enterprise Edition, licensed to Acme Ltd, expires 3 March 2027
  licensee      Acme Ltd
  licence       tc-2026-0007
  issued        1 August 2026
  expires       3 March 2027
  entitlements  many-organisations
  file          /run/licence/acme.licence
```

## telecraft passwd

Hashes one basic-auth secret for the `password` field of a `users.yaml` entry.
It reads the secret from stdin, never from an argument, so the secret doesn't
land in shell history. `passwd` takes no flags and rejects positional
arguments.

The printed hash has four parts separated by `$`: the algorithm, the
iteration count, the base64 salt, and the base64 derived key.

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

Runs the Catalogue import pipeline. It fetches
`opentelemetry-collector-contrib` at a pinned release tag, walks every
`metadata.yaml`, and writes one atomic, versioned Catalogue artefact plus a
coverage report of what it found, excluded, and missed.

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
| `1` | `-tag` was missing, or the fetch, import, or write failed. |
| `2` | An unknown flag was given. |

```sh
catalogue-import -tag v0.158.0
```

## schema-registry-import

Runs the Schema Registry import pipeline, the Catalogue's sibling on the same
pipeline. It fetches an adopter's custom registry at a pinned ref, reads every
model file out of it, and writes one atomic, versioned Schema Registry
artefact plus a coverage report of what was found, what was left out, and
which references come from a dependency registry that isn't in the tree.

The fetch is a sparse, depth-1 checkout of only the YAML into a temporary
directory, removed afterwards. Re-running against the same ref is idempotent;
a different ref writes a new artefact beside the old one. The import reads
registry content out of git and runs no registry toolchain.

The directory this writes to is the one a `schema_conformance` Requirement's
reference resolves against: see
[Schema conformance assertions](requirements.md#schema-conformance-assertions).

| Flag | Type | Default | Description |
|---|---|---|---|
| `-ref` | string | none, required | Registry version to import: a tag or branch in the registry repository. |
| `-repo` | string | none, required | Registry repository URL, recorded as the artefact's provenance. |
| `-out` | string | `schema-registries` | Directory the versioned artefact is written to. |
| `-path` | string | empty | Registry root within the repository, where the registry manifest lives. |
| `-source` | string | empty | Import an existing checkout instead of fetching. |

| Exit code | Meaning |
|---|---|
| `0` | The artefact was written, or already held this import byte for byte. |
| `1` | A required flag was missing, or the fetch, import, or write failed. |
| `2` | An unknown flag was given. |

```sh
schema-registry-import -repo https://git.example/registry -ref v1.4.0
```

## blueprint-check

Strict-loads every Blueprint and shared Component in an estate's source roots
and prints the findings: references to missing or retracted Components or
versions, misplaced extensions, and lane orders that contradict the shipped
ordering rules.

Each root holds the `teams/<team>/{components,blueprints}` layout. Pass
several roots for a primary repository plus its satellite repositories. With
no argument, it checks the current directory.

A load problem refuses the load and exits `1`. Findings print and exit `0`:
they go to owners as advice and never block.

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

## register-check

Loads the register of Organisations and prints what it holds: every
Organisation, its state, the address its Instance is reached at, and where
its estate comes from. Run it on the pull request that changes a register.

The argument is the directory of records, one file per Organisation. With no
argument, it reads the current directory. Every field and every rule is in
[the register format](organisations.md).

A register that does not load exits `1` with every problem in every file on
stderr. An empty directory loads and reports nothing.

```sh
register-check REGISTER_DIR
```

```
loaded 3 Organisations, 2 active
  acme         active   https://acme.telecraft.example         hosted estate
  beacon-rail  active   https://beacon-rail.telecraft.example  https://git.example.com/beacon-rail/estate.git
  corvid       retired
```

| Exit code | Meaning |
|---|---|
| `0` | The register loaded. |
| `1` | The register did not load. |
| `2` | An unknown flag was given. |
