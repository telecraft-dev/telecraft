---
title: Local development environment
description: "Run real collectors under real Supervisors against a real backend, and drive the estate into the states worth looking at."
order: 3
---

# Local development environment

Everywhere else in local development, the two runtime readings are declared.
The fixture backend serves a hand-written mock of the platform API, and the
demo snapshot is computed over an estate that authors its collectors and its
arrivals by hand. Both are the right answer for what they are, and neither
lets you see the product work.

The development environment closes the loop on one machine. Real collectors
run under real OpAMP Supervisors, are served the artefacts this repository
renders, emit into a real telemetry backend, and report what they are running
back over the wire. The collector estate and the arrivals are then read from
those live systems and handed to the same evaluators that produce every
verdict in production.

It exists so you can see a `broken_pipeline`: a lane that is configured,
wired, and delivering nothing. No tool that reads configuration alone can
produce that verdict, it is the one the product exists for, and until now
nothing outside a fixture had ever produced it here.

The decisions behind the environment are in
[ADR-0052](https://github.com/telecraft-dev/telecraft/blob/main/docs/adr/0052-local-development-environment.md).

## What you need

- **Docker**, running. It is the only prerequisite the rest of development
  does not already have, and nothing in the default test path needs it.
- **Go and Node.js**, as [development setup](development.md) describes.
- **One pull.** The images are fetched once and then the environment runs
  offline. Nothing reaches the network at run time.

## Start it

From the repository root:

```sh
devenv/devenv up
```

That renders the estate, composes each collector's Supervisor configuration,
starts the containers, and then runs the OpAMP server and the snapshot loop
in the foreground. Leave it running and work in another terminal.

Three things are listening:

| Address | What |
|---|---|
| `127.0.0.1:4320` | The OpAMP endpoint, at `/v1/opamp`. The collectors connect here. |
| `127.0.0.1:4321` | The snapshot, and the console bundle when one is built. |
| `127.0.0.1:9200` | The telemetry backend. |

To stop it and delete its volumes:

```sh
devenv/devenv down
```

## See it

The console reads the environment through demo mode, which is the shape that
already exists for a console with no server to call. Build the bundle once:

```sh
cd console
npm run build:demo
```

Then open `http://127.0.0.1:4321`. Every card, band, population and finding on
the page was computed by the real evaluators over readings taken from the
collectors and the backend a moment earlier. The loop rebuilds the snapshot
every 10 seconds, so a reload shows the estate as it is now.

To work on the console itself, with hot reload against the same live data:

```sh
cd console
npm run dev:demo
```

The dev server proxies the snapshot from the loop.

Demo mode is read-only, and says so. The write surfaces still render in full:
the composer validates on every keystroke, the claim flow previews impact, and
each ends at a notice explaining that a real instance opens a pull request
instead. To work on those paths, use the fixture backend as
[development setup](development.md) describes.

Two pieces of demo copy are wrong here, because the console cannot tell this
environment from the public demo: the banner reads "Read-only demo", and the
welcome tour's first step says you are looking at a public estate rebuilt from
git on every push. Read-only is true. The rest is the demo's sentence, not
this environment's.

## Check on it

```sh
devenv/devenv status
```

That prints the containers, the collectors as the OpAMP server sees them, what
has arrived in the last five minutes, and which reported configurations are on
disk. `devenv/devenv logs collector-gateway-1` follows one container.

## Drive it

Six scenarios put the estate into a state worth looking at:

```sh
devenv/devenv scenario broken-pipeline
```

| Scenario | What it does | What to look for |
|---|---|---|
| `healthy` | Every sim running, every collector on the artefact the estate describes | The resting state. Compliant, with one waived finding on search's metrics. |
| `broken-pipeline` | Stops checkout's traces sim | The traces lane stays configured and nothing arrives. After one window, `broken_pipeline`, never `not_configured`. |
| `drift` | Merges a local configuration file into `gateway-1`'s Supervisor | What the collector reports running is no longer what the server sent. |
| `shrink` | Stops one of the gateway Tier's two collectors | The population drops below the Tier's declared floor of two. |
| `unmatched` | Starts a collector whose attributes satisfy no selector | It is served the Unmatched artefact: self-telemetry on, no data pipelines, never an empty config map. |
| `reset` | Everything back to healthy | |

The windows in `devenv/estate/requirements/` are five minutes rather than the
day a real library would use, so a scenario is visible while you are still
looking at it. Give each one a window to settle.

The `drift` scenario is checkable with the product's own command, because the
loop writes each collector's reported configuration to disk:

```sh
go run ./cmd/telecraft delivery \
  -intended devenv/estate/rendered/platform/gateway.yaml \
  -effective devenv/run/effective/gateway-1.yaml \
  -path served
```

The same estate answers `telecraft check`, against the live backend:

```sh
go run ./cmd/telecraft check \
  -library devenv/estate/requirements \
  -estate devenv/estate/rows.yaml \
  -exemptions devenv/estate/exemptions
```

## What is in the tree

| Path | What lives there |
|---|---|
| `devenv/estate/` | The authored estate: two teams, two Tiers, two Services, a small requirements library, and one Exemption |
| `devenv/estate/rendered/` | Renderer-written, and committed. Re-render after any authored change |
| `devenv/identity/` | One file per collector: which Tier's Supervisor artefact it starts from, and the identity it reports |
| `devenv/drift/` | The local configuration the `drift` scenario merges |
| `devenv/collector/` | The collector image: the Supervisor, with the collector it runs |
| `devenv/cmd/telecraft-devenv/` | The tool that composes the Supervisor configurations and runs the loop |
| `devenv/run/` | Generated, and gitignored: composed configurations, the readings, the snapshot, the reported configs |

## Change the estate

The estate renders at a fixed commit, `d0d0d0…`, which is deliberately not a
plausible SHA. The estate lives inside this repository rather than its own, so
stamping it with a real commit would make the rendered tree stale on every
platform commit, and the check that the tree matches its sources would fail
perpetually and stop meaning anything.

After changing anything under `devenv/estate/teams/`, re-render:

```sh
go run ./cmd/telecraft render \
  -estate devenv/estate \
  -catalogue devenv/estate/catalogues/catalogue-v0.159.0.json \
  -commit d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0
```

`devenv/devenv up` does this for you. `go test ./devenv/...` fails if you
forget, so a stale tree never reaches a pull request.

Two joins have to be kept by hand, and a test holds each one:

- **Selectors and identities.** A Tier selector is equality over reported
  attributes, and `devenv/identity/` is what reports them. Change one and
  change the other, or the Tier sits empty and the collector reads as
  ungoverned.
- **`rows.yaml` and the rendered pipelines.** The conformance estate is the
  one reading the environment leaves authored, and it names rendered component
  ids exactly as the artefact spells them.

## Update the pins

`devenv/.env` names every version the environment runs, once. The Catalogue in
`devenv/estate/catalogues/` is imported at the collector version, so moving one
means moving both:

```sh
go run ./cmd/catalogue-import -tag v0.159.0 -out devenv/estate/catalogues
```

That import needs the core collector components as well as the contrib ones,
so point it at a checkout holding both repositories with `-source`.

The pins age deliberately visibly. A pin that has drifted from the Catalogue
the estate is judged against is a real finding about the product's version
handling, and it is worth reading as one before bumping it.

## What it does not cover

- **Authoring's write paths.** Demo mode has nothing to POST to. They stay on
  the fixture backend.
- **The Foreign path.** Every collector here is served over OpAMP. A
  git-delivered collector reporting through the collector's own `opamp`
  extension is a second shape and not this one.
- **Live rows.** The Effective reading per Service is authored, as it is in the
  platform today.
