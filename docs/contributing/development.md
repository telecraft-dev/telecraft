---
title: Development setup
description: Prerequisites, and every build, test and lint command that CI runs, including the gated live-backend suites.
order: 2
---

# Development setup

Everything CI runs, you can run locally. Nothing in the default test path
needs Docker, a network, or a credential.

This page is the build, test and lint commands. To watch the product work
against real collectors and a real backend, see the
[local development environment](devenv.md), which is the only thing here that
wants Docker.

## Prerequisites

- **Go 1.26 or later.** `go.mod` declares `go 1.26.1`; CI uses the current
  stable release.
- **Node.js 24 or later**, with the `npm` that ships with it. Only the
  console needs it.
- **Git.** The renderer, the serving path, and the estate loaders all treat
  git as the source of truth.

Optional, and only for the suites they enable:

- **Chromium for Playwright**, installed with
  `npx playwright install chromium` from `console/`.
- **A running Elasticsearch, Elastic Fleet, Kubernetes API server, or forge
  app credential** for the live provider suites. Each suite skips when its
  variables are absent, so none of this is needed to work on the core.

## Get the source

```sh
git clone https://github.com/telecraft-dev/telecraft.git
cd telecraft
```

## Build and test the core

The three commands the **Build and test** job runs, in order:

```sh
go build ./...
go vet ./...
go test ./...
```

`go test ./...` covers every package, including the lint's own self-test and
the provider conformance kits. The live provider suites are compiled and run
by this command too, and skip themselves when their environment variables are
absent, so a clean checkout gives you a green run with no setup.

## Run the vendor-word lint

```sh
go run ./tools/vendorlint
```

The lint reads `vendorlint.yaml` at the repository root and prints one line
per finding, exiting 1 when it finds any and 0 when it does not. A clean run
prints how many files it scanned and exits 0.

Two flags exist: `-config` names the config file, relative to `-root`, and
`-root` names the tree to scan. The defaults are `vendorlint.yaml` and `.`,
which is what CI uses.

The lint scans `cmd/`, `internal/`, `console/`, `README.md` and `docs/`. It
is the mechanical form of the neutral core boundary, so read
[Architecture](architecture.md#the-neutral-core-boundary) before you change
`vendorlint.yaml`.

## Golden files

Two suites compare output against checked-in golden files. Regenerate them
deliberately, and read the resulting diff before you commit it:

```sh
TELECRAFT_UPDATE_GOLDEN=1 go test ./internal/renderer ./internal/expectation
go test ./internal/card -update
```

`go test ./internal/card -update` rewrites `console/fixtures/card-contract.json`,
the one artefact both sides of the card data contract are held to. The Go
engine writes it and the console's `tests/card-contract.test.ts` reads it, so
a field added on one side without the other following is a failing test. If
the shape changed rather than the fixture data, bump the contract version as
well.

## Run the CLI binaries

Three binaries live under `cmd/`:

```sh
go run ./cmd/telecraft            # the platform CLI; prints its usage with no arguments
go run ./cmd/blueprint-check .    # strict-load every Blueprint and Component in an estate
go run ./cmd/catalogue-import -tag v0.158.0
```

`telecraft observe` and `telecraft check` read their backend settings from
flags, defaulting to the `TELECRAFT_TELEMETRY_ENDPOINT` and
`TELECRAFT_TELEMETRY_API_KEY` environment variables. The endpoint defaults to
`http://localhost:9200` when neither the flag nor the variable is set.

## Work on the console

All console commands run from `console/`:

```sh
cd console
npm ci
```

`npm ci` installs exactly what `package-lock.json` pins, which is what CI
does. Use it rather than `npm install` so your tree matches the one CI tests.

To run the console against the fixture backend, in two terminals:

```sh
npm run backend    # the fixture backend on http://127.0.0.1:4700
npm run dev        # the console on http://localhost:5173, proxying /api
```

The fixture backend prints its sign-in credentials at start-up. The platform
binary verifies PBKDF2 hashes from the estate's `users.yaml` instead, and
`telecraft passwd` authors them.

The checks the **Console** job runs, in the order it runs them:

```sh
npm run typecheck           # tsc --noEmit
npm test                    # Vitest: the engine, the presentation store, shelf ordering, the card contract
npm run check:palette       # the design tokens against their contrast and colour-vision floors
npm run build               # tsc --noEmit, then vite build into dist/
npm run check:zero-cdn      # no external host in any built artefact
npm run check:bundle-budget # the entry chunk within its gzipped ceiling
npm run e2e                 # Playwright against dist/ and the fixture backend
```

`npm run e2e` needs a browser first:

```sh
npx playwright install chromium
```

Playwright starts the fixture backend itself, serving both the documented API
and the built bundle from `dist/`, so run `npm run build` before `npm run e2e`.

`npm run check:zero-cdn` runs over `dist/`, so it also needs a build first.
It fails on any external URL in a built artefact: HTML, CSS and SVG tolerate
none, and JavaScript tolerates only the allowlisted never-fetched string
literals the script documents. The Playwright suite enforces the same rule at
runtime by intercepting every network request and failing on any host beyond
the console's own origin.

`npm run check:bundle-budget` needs a build first for the same reason. It
measures the gzipped size of the entry chunk, the module `dist/index.html`
loads, and fails when that exceeds the ceiling the script states and argues
for. The [console page](console.md#the-bundle-budget) explains what the
ceiling is holding.

To build the demo bundle and its snapshot, as the **Demo snapshot and bundle**
job does:

```sh
npm run build:demo       # VITE_DEMO=1: the console reads a snapshot, not /api
```

then generate the snapshot beside it with `go run ./cmd/telecraft snapshot`.
The [console page](console.md#demo-mode) covers what demo mode changes.

## The live-backend suites

Four suites talk to a real system. Each one reads its configuration from the
environment and calls `t.Skip` when it is absent, so the suite is a skip and
never a failure on a machine without credentials. That discipline is
ADR-0036's: an absent credential is not a contract violation.

Because they skip rather than fail, a green `go test ./...` does not mean the
live suites ran. Read the test output when you expect them to.

| Variables | Suite | Enables |
|---|---|---|
| `TELECRAFT_TELEMETRY_LIVE_ENDPOINT` | `go test ./internal/provider/telemetry/ -run Live -v -count=1` | The Elasticsearch TelemetryProvider against a real cluster: reads, attribute names, self-telemetry, and metering. The suite writes to `telecraft-live-*` indices only. This is the one live suite CI runs on every pull request, against a service container. |
| `TELECRAFT_ELASTICFLEET_LIVE_ENDPOINT`, `TELECRAFT_ELASTICFLEET_LIVE_APIKEY` | `go test ./internal/provider/estate/ -run Live -v -count=1` | The `ElasticFleet` EstateProvider against a real Elastic Fleet API: the estate read and the redaction contract. Not run in CI. |
| `TELECRAFT_INVENTORY_LIVE_ENDPOINT`, `TELECRAFT_INVENTORY_LIVE_TOKEN` | `go test ./internal/provider/inventory/ -run Live -v -count=1` | The `Kubernetes` InventoryProvider against a real API server, for example `kubectl proxy` at `http://127.0.0.1:8001`. Not run in CI. |
| `FORGE_APP_ID`, `FORGE_INSTALLATION_ID`, `FORGE_APP_PRIVATE_KEY`, optionally `FORGE_LIVE_REPO` | `go test ./internal/provider/forge/ -run Live -v -count=1` | The forge adapter's pull-request flow against a real repository. `FORGE_LIVE_REPO` overrides the default fixture repository. The suite skips twice over: once when the three credentials are not all set, and again when the app installation cannot see the repository. |

Each skip message names the variables it wants and why, so a skipped run tells
you what to provide.
