---
title: Quickstart
description: Download the CLI, point it at an estate, and read your first verdict.
order: 2
---

# Quickstart

This guide takes you from nothing to a real conformance verdict over a real
estate, in about five minutes. You need `git` and nothing else. There is no
toolchain to install and nothing to compile.

## 1. Get the CLI

Download the build for your machine from the [latest
release](https://github.com/telecraft-dev/telecraft/releases/latest). It is one
file: Telecraft is a single static binary with no runtime dependencies and no
installer.

```sh
version=v0.7.1
os=$(uname -s | tr '[:upper:]' '[:lower:]')   # linux, or darwin on a Mac
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

curl -fsSLo telecraft \
  "https://github.com/telecraft-dev/telecraft/releases/download/$version/telecraft-$version-$os-$arch"
chmod +x telecraft
```

On Windows, download `telecraft-<version>-windows-amd64.exe` from the same
page. It is attached and it is not otherwise exercised: nothing in the
project's CI runs a Windows build, so tell us if it misbehaves.

Check what you downloaded against the release's `SHA256SUMS`, which covers
every binary on the page:

```sh
curl -fsSLO "https://github.com/telecraft-dev/telecraft/releases/download/$version/SHA256SUMS"
sha256sum --ignore-missing --check SHA256SUMS
```

Prefer not to place a binary at all? The container image runs the same command,
and the rest of this guide works with `docker run --rm -v "$PWD:/w" -w /w
ghcr.io/telecraft-dev/telecraft:$version` wherever it says `./telecraft`.

Run it with no arguments to see the subcommands:

```
usage: telecraft observe -service <service.name> [-environment env] [-window 15m] [-endpoint URL] [-api-key KEY] [-attributes a,b,c]
       telecraft check -library <dir> -estate <file> [-source <dir> -catalogue <artefact>] [-exemptions dir] [-ownership dir] [-environment env] [-endpoint URL] [-api-key KEY]
       telecraft palette -team <team-id> -estate <dir> -catalogue <artefact>
       telecraft render -estate <dir> -catalogue <artefact> -commit <sha> [-out <dir>]
       telecraft serve (-estate <dir> | -repo <url> [-cache dir]) [-listen host:port] [-fetch-interval 30s]
       telecraft snapshot -estate <dir> -catalogue <artefact> -library <dir> -rows <file> -readings <file> -commit <sha> -team <team-id> [-catalogues dir] [-exemptions dir] [-out file]
       telecraft delivery -intended <file> -effective <file> -path (served|git)
       telecraft passwd   (reads the secret from stdin, prints the users.yaml hash)
```

The command exits 2 on a usage error, so a script that runs it with no
arguments fails instead of doing nothing.

## 2. Get an estate

An estate is a git repository, not a database. Clone the public demo estate
beside the binary you just downloaded:

```sh
git clone https://github.com/telecraft-dev/estate-demo.git
```

It holds a synthetic retailer's observability estate: four Services, six
Tiers, a requirements library, one Exemption, and the two declared readings a
repository can't hold for you (which collectors reported, and the running
configuration each one reports).

## 3. Get a verdict

To get a verdict, run `telecraft check`. It loads the requirements library,
judges every row of the estate, writes one JSON report to stdout, and sets its
exit code from the result:

```sh
./telecraft check \
  -library estate-demo/requirements \
  -estate estate-demo/demo/rows.yaml \
  -exemptions estate-demo/exemptions \
  > report.json
```

No telemetry backend is reachable yet, so the report says what the check
could not see. The summary at the end of `report.json`:

```json
{
  "rows": 5,
  "failing_rows": 4,
  "counting_failures": 4,
  "waived": 0,
  "library_drift": 0
}
```

Four production rows come back `unknown` rather than green, and each one
carries the reason:

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

Not knowing is a normal state, and the report shows it as itself. It never
rounds `unknown` up to a pass. The command exits 1, so a CI job that can't see
the backend goes red instead of green.

## 4. Add a backend and get the real cross

A verdict built on configuration alone can only tell you what somebody
intended. Point the check at a telemetry backend and it crosses that intent
against what arrived.

Start a throwaway single-node Elasticsearch:

```sh
docker run -d --name telecraft-quickstart \
  -p 127.0.0.1:9200:9200 \
  -e discovery.type=single-node \
  -e xpack.security.enabled=false \
  -e ES_JAVA_OPTS="-Xms512m -Xmx512m" \
  docker.elastic.co/elasticsearch/elasticsearch:9.1.0
```

Wait for it to start, then seed telemetry for the demo estate's Services.
Every document carries the Service name, the Environment, and the two identity
attributes the demo's `trace-identity` requirement asks about:

```sh
ES=http://localhost:9200
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

for index in logs-demo metrics-demo traces-demo; do
  curl -fsS -XPUT "$ES/$index" -H 'Content-Type: application/json' -d '{
    "mappings": {
      "dynamic_templates": [
        {"strings_as_keyword": {"match_mapping_type": "string",
                                "mapping": {"type": "keyword"}}}
      ],
      "properties": {"@timestamp": {"type": "date"}}
    }
  }' > /dev/null
done

doc() {
  echo '{"index":{}}'
  printf '{"@timestamp":"%s","resource":{"attributes":{"service.name":"%s","deployment.environment.name":"%s"}},"service":{"namespace":"%s"},"deployment":{"environment":{"name":"%s"}}}\n' \
    "$NOW" "$1" "$2" "$3" "$2"
}

# storefront/catalogue-web deliberately gets no traces, and
# storefront/search deliberately gets no metrics.
{ doc checkout/payments production checkout
  doc checkout/basket production checkout
  doc storefront/catalogue-web production storefront
  doc storefront/search production storefront
  doc checkout/payments staging checkout
} | curl -fsS -XPOST "$ES/logs-demo/_bulk?refresh=true" \
      -H 'Content-Type: application/x-ndjson' --data-binary @- > /dev/null

{ doc checkout/payments production checkout
  doc checkout/basket production checkout
  doc storefront/catalogue-web production storefront
  doc checkout/payments staging checkout
} | curl -fsS -XPOST "$ES/metrics-demo/_bulk?refresh=true" \
      -H 'Content-Type: application/x-ndjson' --data-binary @- > /dev/null

{ doc checkout/payments production checkout
  doc checkout/basket production checkout
  doc storefront/search production storefront
  doc checkout/payments staging checkout
} | curl -fsS -XPOST "$ES/traces-demo/_bulk?refresh=true" \
      -H 'Content-Type: application/x-ndjson' --data-binary @- > /dev/null
```

Run the same check again. `-endpoint` defaults to `http://localhost:9200`, so
the command doesn't change:

```sh
./telecraft check \
  -library estate-demo/requirements \
  -estate estate-demo/demo/rows.yaml \
  -exemptions estate-demo/exemptions \
  > report.json
```

```
checkout/basket           production  compliant
checkout/payments         production  compliant
storefront/catalogue-web  production  broken_pipeline
storefront/search         production  compliant
checkout/payments         staging     compliant
```

```json
{
  "rows": 5,
  "failing_rows": 1,
  "counting_failures": 2,
  "waived": 1,
  "library_drift": 0
}
```

`storefront/catalogue-web`
has an OTLP receiver wired into a traces pipeline, and no spans arrived, so
the finding is `broken_pipeline`, not `not_configured`:

```json
{
  "requirement": "traces-delivered",
  "title": "Distributed traces are delivered",
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

Somebody configured that pipeline on purpose, and it is silently not working.

`storefront/search` scores `compliant` while carrying one waived finding. An
authored Exemption covers its missing metrics, and the summary keeps
`"waived": 1` visible, so a green built on waivers never passes for a clean
green.

When you are done, remove the container:

```sh
docker rm -f telecraft-quickstart
```

## Building it yourself

You do not need this to use Telecraft, and the download above is the supported
way to get the CLI. Build from source when you are changing it: fixing
something, adding a subcommand, or running an unreleased commit.

```sh
git clone https://github.com/telecraft-dev/telecraft.git
cd telecraft
go build -o telecraft ./cmd/telecraft
```

You need Go 1.26 or later. The binary this produces is the same one the
release attaches, minus the version stamp the release build sets.
[Contributing](../contributing/index.md) covers the rest of the development
setup.

## What next

- [Check conformance](check-conformance.md) explains how to read the report
  and puts the check in CI.
- [Author and render](author-and-render.md) starts the Authoring rung: compose
  a Blueprint and render otelcol YAML into git.
- [Explore the demo](explore-the-demo.md) shows the same estate through the
  console.
