# Telecraft

**Craft, govern, and verify OpenTelemetry across your whole estate.**

Telecraft is an open-source fleet and policy management platform for
OpenTelemetry. It models your collection topology, composes collector
configurations from owned, versioned building blocks, and then does the thing
no other tool does: derives from the configuration an *expectation* of what
telemetry should arrive — and checks it. Green means "the config worked",
never merely "the config applied".

Three separately-adoptable rungs, in any order:

| Rung | What it does | What it costs you |
|---|---|---|
| **Conformance** | Reads your telemetry backend and your collectors' reported configs, judges every service against its Service Class floor, and tells you *whose* problem each finding is | A connection string |
| **Authoring** | A console where teams compose Blueprints from governed Components and render plain otelcol YAML into git as pull requests | Nothing in your delivery path changes |
| **Serving** | A stateless OpAMP server that delivers rendered config from git to collectors — with GitOps as a co-equal alternative, chosen per collector | An OpAMP Supervisor beside each served collector |

Principles that hold everywhere: **nothing sits in the telemetry path** (if
Telecraft is down, no telemetry stops flowing); **git is the source of
truth** (history, rollback, approval and audit are git's, not ours);
**configurations, never binaries**; **vendor-neutral core** (Elastic,
Prometheus and friends are plugins); **air-gap first-class** (no hard
dependency on any SaaS).

## Status

Pre-alpha: the design corpus is complete and the build has started. Decisions
live in [`docs/`](docs/) — start with the [glossary](docs/glossary.md), the
visual [terminology guide](docs/terminology.html), the
[requirements](docs/requirements/product-requirements.md) and the
[ADRs](docs/adr/). The build plan is [`docs/plan.md`](docs/plan.md); the
backlog is the [issue tracker](https://github.com/telecraft-dev/telecraft/issues).

## Repository layout

| Path | What lives there |
|---|---|
| `cmd/` | Binaries (arrive with the first ported code) |
| `internal/` | The neutral core — no vendor word appears here (ADR-0001) |
| `internal/provider/` | Vendor implementations behind the core's seams, always product-qualified: `Elasticsearch`, `ElasticFleet` |
| `console/` | The TypeScript/React console (arrives in Phase 4, ADR-0045) |
| `tools/vendorlint/` | The ADR-0001 vendor-word lint; its scope globs in [`vendorlint.yaml`](vendorlint.yaml) are the core/provider boundary |
| `docs/` | The decision corpus: glossary, requirements, ADRs, research, prototype verdicts |

## Development

Go 1.26+. Both CI checks run locally with:

```sh
go test ./...             # unit tests, including the lint's self-test
go run ./tools/vendorlint # the vendor-word lint over code and docs
```

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
