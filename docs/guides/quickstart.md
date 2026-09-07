---
title: Quickstart
description: See a console over a real estate in a few minutes, hosted or on your own machine, and get a verdict from the command line when you want one in CI.
order: 2
---

# Quickstart

Three ways in, and they are in the order most people want them. Take the
first one that fits and stop; none of them is a step towards another.

| | | |
|---|---|---|
| [We run it](#1-we-run-it) | Nothing to install | A console we keep running |
| [You run it](#2-you-run-it) | One file and `git` | A console on your machine |
| [The command line](#3-the-command-line) | One file | A verdict in CI |

## 1. We run it

Ask for an Organisation at <https://cloud.telecraft.dev> with a Google or
Microsoft Entra ID account. You get one Instance at an address of its own,
`<your-name>.cloud.telecraft.dev`, with its own estate, its own people, and
nothing shared with anybody else's.

Signing up is a request rather than a form that provisions. A person reads it
and merges it, so you wait for us rather than for a machine. It is the same
release you can run yourself, and there is no capability here that a
deployment on your own hardware does not have: what you are buying is the
running of it.

[Use the hosted service](hosted.md) covers signing up, connecting a
repository, signing your people in, and what is promised about keeping it.

Want to look before you ask? <https://demo.telecraft.dev> is the real console
over a public estate, read only and with no sign-in.

## 2. You run it

One downloaded file and `git`. No toolchain, no compiling, and nothing to
configure before you can look at it.

Download the build for your machine from the [latest
release](https://github.com/telecraft-dev/telecraft/releases/latest). It is
one static file with no runtime dependencies and no installer.

```sh
version=v0.9.0
os=$(uname -s | tr '[:upper:]' '[:lower:]')   # linux, or darwin on a Mac
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
base=https://github.com/telecraft-dev/telecraft/releases/download/$version

curl -fsSLO "$base/telecraft-$version-$os-$arch"
curl -fsSLO "$base/SHA256SUMS"
sha256sum --ignore-missing --check SHA256SUMS

chmod +x "telecraft-$version-$os-$arch"
mv "telecraft-$version-$os-$arch" telecraft
```

Keep the release's own name until the checksum has been checked. `SHA256SUMS`
lists every binary on the page by that name, and `--ignore-missing` skips the
ones you did not download, so renaming first leaves it with nothing to check
and nothing checked.

On Windows, download `telecraft-<version>-windows-amd64.exe` from the same
page. It is attached and it is not otherwise exercised: nothing in the
project's CI runs a Windows build, so tell us if it misbehaves.

Prefer not to place a binary? The container image is the same artefact:
`docker run --rm -v "$PWD:/w" -w /w ghcr.io/telecraft-dev/telecraft:release`
wherever this guide says `./telecraft`.

### Serve an estate

An estate is a git repository, not a database. Clone the public demo one and
serve it:

```sh
git clone https://github.com/telecraft-dev/estate-demo.git
./telecraft serve -estate estate-demo
```

It prints a sign-in it made up for itself:

```
serve: no users.yaml in this estate, so this process minted one sign-in for itself.
serve: It is not written anywhere and it dies with the process.
serve:   email     bootstrap@localhost
serve:   password  1338e8c8e5bcb67c2c093785e286ba73
serve: Add users.yaml to the estate to replace it.
console and API on http://127.0.0.1:4321
```

Open <http://127.0.0.1:4321> and sign in with those two. The password is
drawn fresh each time the process starts, it is written nowhere, and it only
exists because the console is bound to an address only your machine can
reach. An Instance anybody else can reach refuses to start without a
`users.yaml`, and [Run an Instance](run-an-instance.md) is where you write
one.

You are now looking at four Services across six Tiers, with real findings on
them. [Explore the demo](explore-the-demo.md) walks the surfaces.

## 3. The command line

The console is one way to read a verdict. The other is a JSON report and an
exit code, which is what belongs in CI, in a cron job, or on a laptop. The
same file does both.

### Get an estate

If you skipped the section above, clone the public demo estate beside the
binary:

```sh
git clone https://github.com/telecraft-dev/estate-demo.git
```

It holds a synthetic retailer's observability estate: four Services, six
Tiers, a requirements library, one Exemption, and the two declared readings a
repository can't hold for you (which collectors reported, and the running
configuration each one reports).

### Get a verdict

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

### Add a backend and get the real cross

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
