# Telecraft

**Craft, govern, and verify OpenTelemetry across your whole estate.**

Telecraft is an open-source fleet and policy management platform for
OpenTelemetry. It models your collection topology, composes collector
configuration from owned, versioned building blocks, and then does the thing
no other tool does: it derives from the configuration an *expectation* of what
telemetry should arrive, and checks it. Green means "the configuration
worked", never merely "the configuration applied".

See it running: **[demo.telecraft.dev](https://demo.telecraft.dev)** is the
real console over a public demo estate, read-only and rebuilt from git on
every push.

Three separately-adoptable rungs, in any order:

| Rung | What it does | What it costs you |
|---|---|---|
| **Conformance** | Reads your telemetry backend and your collectors' reported configuration, judges every service against its Service Class floor, and tells you *whose* problem each finding is | A connection string |
| **Authoring** | A console where teams compose Blueprints from governed Components and render plain otelcol YAML into git as pull requests | Nothing in your delivery path changes |
| **Serving** | A stateless OpAMP server that delivers rendered configuration from git to collectors, with GitOps as a co-equal alternative, chosen per collector | An OpAMP Supervisor beside each served collector |

Principles that hold everywhere: **nothing sits in the telemetry path** (if
Telecraft is down, no telemetry stops flowing); **git is the source of
truth** (history, rollback, approval and audit are git's, not ours);
**configurations, never binaries**; **vendor-neutral core** (Elasticsearch,
Prometheus and friends are plugins); **air-gap first-class** (no hard
dependency on any hosted service).

## Documentation

Full documentation lives in [`docs/`](docs/):

- [Concepts](docs/concepts/) explain the model, from readings and verdicts to
  governance.
- [Guides](docs/guides/) walk through tasks, starting with the
  [quickstart](docs/guides/quickstart.md).
- [Reference](docs/reference/) covers every command, flag, and authored file
  format, alongside the [glossary](docs/glossary.md).
- [Contributing](docs/contributing/) is the developer documentation: local
  development, package architecture, the provider seams, and how decisions
  are recorded.

## Status

The first build is complete: conformance, authoring, and serving all work
end to end, with a console over all four workspaces. Interfaces are still
free to change, so treat it as early software rather than a stable release.

The decision corpus behind the product lives in
[`docs/adr/`](docs/adr/) (47 architecture decision records), with
[requirements](docs/requirements/), [research](docs/research/), and
[prototype verdicts](docs/prototypes/) beside it.

## Repository layout

| Path | What lives there |
|---|---|
| `cmd/` | The `telecraft` CLI, plus the `catalogue-import` and `blueprint-check` tools |
| `internal/` | The neutral core: no vendor word appears here (ADR-0001) |
| `internal/provider/` | Vendor implementations behind the core's seams, always product-qualified: `Elasticsearch`, `ElasticFleet` |
| `console/` | The TypeScript and React console (ADR-0045) |
| `tools/vendorlint/` | The ADR-0001 vendor-word lint; its scope globs in [`vendorlint.yaml`](vendorlint.yaml) are the core and provider boundary |
| `docs/` | Documentation and the decision corpus |

## Development

Go 1.26 or later, and Node.js for the console. The full set of checks that CI
runs:

```sh
go build ./...            # the core and the CLI
go test ./...             # unit tests, including the lint's self-test
go run ./tools/vendorlint # the vendor-word lint over code and docs

cd console
npm ci && npm run typecheck && npm test && npm run build
```

CI runs six jobs: build and test, the console build, the vendor-word lint,
a live check against Elasticsearch, a live check of the forge adapter against
GitHub, and a build of the demo snapshot and bundle. The two live jobs skip
themselves, loudly, when their credentials are absent.

See [contributing](docs/contributing/) for the detail.

### Brand and design

Anything a user sees — console surfaces, documentation, `telecraft.dev`, and
CLI output — follows one system.

| Read this | For |
|---|---|
| [`docs/branding/identity.md`](docs/branding/identity.md) | What Telecraft looks and sounds like, including the voice the docs are written in |
| [`docs/branding/design-system.md`](docs/branding/design-system.md) | Token values, type scale, marks, and the accessibility floors |
| [ADR-0047](docs/adr/0047-visual-identity-and-design-tokens.md) | Why it is the way it is |

Four rules carry most of it:

- **Colour never carries meaning alone.** Every state ships a mark and a word;
  every signal colour ships its lane name. Hue reinforces, it never tells
  (ADR-0041 §2, ADR-0047 §5).
- **Every colour is defined in exactly two blocks**, never inside a media
  query, or it is stranded in the unresolved theme state.
- **No asset is fetched from another origin.** Fonts and icons are bundled;
  CI fails the build otherwise (ADR-0019).
- **British English**, in prose and in identifiers: `colour`, `licence`,
  `normalise`, `--colour-bg`.

## Licence

Apache-2.0.
