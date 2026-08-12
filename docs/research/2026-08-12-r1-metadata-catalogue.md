# R-1: `metadata.yaml` as the Catalogue substrate

- Date: 2026-08-12 (session G1, feeding G2)
- Method: shallow clones of `opentelemetry-collector` (core),
  `-contrib`, `-releases` and `-ecosystem-explorer` at `main` (core HEAD
  `ce04ab8`, contrib HEAD `6c14655`, 2026-08-12), plus tags `v0.60.0`,
  `v0.99.0`, `v0.100.0`, `v0.106.0`, `v0.158.0`. All counts reproducible from
  those trees. Latest release: **v0.158.0** (2026-08-04).

## TL;DR verdict

**Yes — with caveats.** `metadata.yaml` is a sound substrate for the Catalogue's
**identity + stability** scope, which is all G2 needs.

- Coverage is **100%**: **267/267** real pipeline components across both repos
  carry `status.class` and a non-empty `status.stability`. No gaps.
- The `status:` block — the only part we depend on — has been **additive-only
  for two years**; zero breaking changes since v0.99.0 (2024).
- Trivially **vendorable**: ~960 KB raw YAML → ~110 KB JSON → **~8 KB gzipped**.
  A pinned sparse checkout is **3.9 MB**. All repos **Apache-2.0**.

**Biggest finding: we may not have to build this from scratch.** An official
aggregated, versioned, nightly-regenerated catalogue already exists —
`open-telemetry/opentelemetry-ecosystem-explorer` — derived from these same
`metadata.yaml` files (§3). Consume it as a cross-check, not as the source.

Four caveats, two of them genuine traps:

1. **A depth-1 directory walk silently misses 20 real components** — contrib
   nests `extension/{encoding,observer,storage}/*` a level deeper, including
   `file_storage`, `k8s_observer`, `docker_observer`. This bit this research
   mid-flight (§2).
2. **`type` alone is not a unique key.** 26 type strings are reused across
   classes — `datadog` is a connector, exporter, extension *and* receiver. The
   primary key must be **`(class, type)`** (§2).
3. **`status.distributions` drifts** from the real OCB manifests — 10 mismatches
   at v0.158.0. Derive distro membership from the manifests (§4).
4. **Stability is per-signal.** 36 components carry >1 level at once, so the
   allow-list must key on *(component, signal)*; `class: pkg` must also be
   filtered out (§7).

Config field schemas are **not** in `metadata.yaml` for any real component and
won't be for several quarters (§5) — scope to identity + stability now, as
planned.

---

## 1. The schema today

Documented in-repo at `cmd/mdatagen/metadata-schema.yaml` (a commented
exemplar, not a JSON Schema) with prose in `cmd/mdatagen/README.md`. There is
**no schema version number** — it is versioned implicitly by the release tag.

Top-level keys on `main`: `type`, `deprecated_type`, `display_name`,
`description`, `parent`, `scope_name`, `generated_package_name`,
`override_value_enabled`, `status`, `sem_conv_version`, `sem_conv_url`,
`config`, `resource_attributes`, `entities`, `attributes`, `metrics`, `events`,
`tests`, `telemetry`, `feature_gates`. Only `type`, `deprecated_type` and
`status` matter to us:

```yaml
status:
  class: <receiver|processor|exporter|connector|extension|cmd|pkg|scraper|converter|provider>
  stability:                # same signal vocabulary at every level
    <development|alpha|beta|stable|deprecated|unmaintained>: [<metrics|traces|logs|traces_to_metrics|…|extension>]
  deprecation:              # required when deprecated
    <signal>: {date: string, migration: string}
  distributions: [string]   # core | contrib | k8s | otlp
  warnings: [string]
  codeowners: {active: [string], emeritus: [string]}
  unsupported_platforms: [<linux|windows>]
```

### Real samples

Eight components sampled across all five kinds and both repos.
`core/receiver/otlpreceiver` shows per-signal divergence;
`contrib/receiver/signalfxreceiver` shows the deprecation shape; and
`contrib/connector/spanmetricsconnector` shows the alias plus a `seeking_new`
codeowners field **that is not in the documented schema**:

```yaml
type: otlp                       | type: span_metrics              | type: signalfx
status:                          | deprecated_type: spanmetrics    | status:
  class: receiver                | status:                         |   stability:
  stability:                     |   class: connector              |     deprecated: [metrics, logs]
    stable: [traces,metrics,logs]|   stability:                    |   deprecation:
    alpha: [profiles]            |     alpha: [traces_to_metrics]  |     metrics:
  distributions:                 |   codeowners:                   |       date: "2026-02-13"
    [core, contrib, k8s, otlp]   |     active: [portertech, …]     |       migration: "Use OTLP …"
                                 |     seeking_new: true           |     logs: {…}
```

Also verified: `core/processor/batchprocessor` (beta, 3 signals, large
`telemetry:` block), `core/exporter/otlpexporter`, `core/extension/zpagesextension`,
`contrib/receiver/simpleprometheusreceiver` (the sole `unmaintained`),
`contrib/exporter/mezmoexporter` (deprecated 2026-07-30).

### Stability over ~2 years

37 commits touched `metadata-schema.yaml` since 2024-08, but diffing the
`status:` block at `v0.99.0` against `main` shows **only additive change**: the
`deprecation:` sub-block was added (PR #12464, 2025-03-04); the `class` enum
gained `scraper, converter, provider`; the signal enum gained `converter,
provider` (and `profiles` in practice); and `stability` became **required**
(PR #14070, 2025-10-29) — a tightening, not a break. Every other change landed
in the telemetry/metrics/attributes/entities sections we don't read.
**The identity+stability core is stable.**

---

## 2. Coverage

Walking `<repo>/<kind>/*/` **and** `<repo>/<kind>/*/*/` for dirs with a
`go.mod`, keeping `status.class ∈ {receiver,processor,exporter,connector,extension}`:

| Kind | core | contrib | total |
|---|---|---|---|
| receiver | 2 | 114 | 116 |
| processor | 3 | 35 | 38 |
| exporter | 4 | 47 | 51 |
| connector | 1 | 14 | 15 |
| extension | 2 | 45 | 47 |
| **TOTAL** | **12** | **255** | **267** |

**Every component dir with a `go.mod` has a `metadata.yaml`; all 267 carry
class and stability.** Coverage is not the risk — correct *enumeration* is.

### Three enumeration traps

**(a) Nested components — the trap that caught this research.** A depth-1 glob
returns 246 and looks plausible, but omits **20 contrib extensions nested one
level deeper**: `extension/encoding/*` (11), `observer/*` (5), `storage/*` (3),
`dbauth/*`, `tailstorage/*`. Not obscure — they include **`file_storage`,
`db_storage`, `k8s_observer`, `docker_observer`, `ecs_observer`,
`host_observer`** and the whole `*_encoding` family. Discover by `go.mod`, not
fixed depth.

**(b) `class: pkg` must be excluded.** 16 core files declare `class: pkg`
(`receiverhelper`, `exporterhelper`, `extensionauth`, …), sit under
`core/receiver/` etc. and look like components; 8 have no `stability` at all.
Upstream's own registry **fails to filter these** and ships 5 as components (§3).

**(c) Over-broad `find`.** `find -name metadata.yaml` returns 99 core / 375
contrib — the excess is subcomponents (`hostmetricsreceiver/internal/scraper/*`),
fixtures, `internal/` packages and mdatagen samples.

### `(class, type)` is the primary key

267 components yield only **238 distinct `type` strings** but **267 distinct
`(class, type)` pairs**. 26 types are reused across classes: `datadog`
(connector + exporter + extension + receiver), `sumologic` (3), `memory_limiter`
and `remotetap` (extension + processor), plus `kafka`, `prometheus`, `zipkin`,
`signalfx`, `elasticsearch`, `awss3`, `nop`, … (exporter + receiver). This
mirrors the collector's own config addressing, so `(class, type)` is correct and
natural. Getting it wrong collapses 29 components.

---

## 3. Machine-consumability

**What mdatagen generates is not a catalogue.** It is strictly
**per-component**, driven by `//go:generate mdatagen metadata.yaml`:
`generated_status.go`, `documentation.md`, metrics/resource/telemetry builders
in `internal/metadata/`, generated tests, and recently per-component
`config.schema.yaml`. **No aggregated artefact, no JSON index.** Aggregation is
on us or on someone upstream.

### ⚠ `opentelemetry-ecosystem-explorer` — the upstream aggregated catalogue

The significant discovery of R-1. `open-telemetry/opentelemetry-ecosystem-explorer`
(created 2026-01-21, live at `explorer.opentelemetry.io`, **Apache-2.0**)
publishes what we were about to build:

```
ecosystem-registry/collector/{core,contrib}/v0.158.0/{receiver,...,extension}.yaml
```

Envelope `distribution, version, repository, component_type, schema_hash,
components[]`; each entry nests the component's `metadata` verbatim (`type`,
`display_name`, `description`, `status.{class,stability,distributions,codeowners}`).
It is **versioned per release and retained** (`v0.154.0` … `v0.158.0` plus
`v0.158.1-SNAPSHOT` — directly satisfying "pin to a collector version") and
**regenerated nightly** (`nightly-registry-update.yml`, cron `0 2 * * *`) by
`ecosystem-automation/collector-watcher/`, which clones core+contrib and parses
their `metadata.yaml` — same substrate, same semantics. It also ships
`component_readmes/*.md`, `meta/schemas/`, `deprecations.yaml` and a
`schema_hash` per file. **Verified count at v0.158.0: 271** (core 17, contrib 254).

**But not authoritative enough to consume blindly:** **5 false positives** —
`xreceiver`, `xprocessor`, `xexporter`, `xconnector`, `xextension` are
`class: pkg` experimental interface packages, so the real count is 266. (The
other 2-component delta is expected tag-vs-`main` drift.) There is **no JSON
API** (`/api/components` 404s; static SPA) — raw GitHub YAML is the machine
interface — and the repo is six months old with no layout stability guarantee.

### The opentelemetry.io registry is **not** usable

`opentelemetry.io/ecosystem/registry/` is hand-written YAML in
`data/registry/*.yml` submitted by PR (`content/en/ecosystem/registry/adding.md`),
schema `data/registry-schema.json`. It has the only true JSON endpoint
(`/ecosystem/registry/index.json`, 1165 entries) but is **disqualified twice**:
**no stability field at all**, and only 297 `language: collector` entries — not
generated from `metadata.yaml`. Separately the site's `data/collector/*.yml`
(266 components, per-signal stability) powers the docs' component tables, but
its generator `scripts/collector-sync/` **reads the ecosystem-explorer
registry** — confirming the explorer as the upstream aggregation point.

### Tools that already walk every `metadata.yaml`

`collector-watcher` (ecosystem-explorer → the versioned registry above),
`collector-sync` (opentelemetry.io → `data/collector/*.yml`), `githubgen`
(`opentelemetry-go-build-tools` → `.github/CODEOWNERS`, stamped *"generated by
githubgen"*), and `checkapi` (API-conformance lint). Contrib's `cmd/githubgen/`
is now a stub pointing at `opentelemetry-go-build-tools` (contrib #37294).
**Walking every `metadata.yaml` is a well-trodden, upstream-sanctioned
pattern** — our aggregation is neither novel nor fragile.

---

## 4. Versioning and distribution manifests

Components release in lockstep; every release is a tag `vX.Y.Z` on both repos,
and `metadata.yaml` files exist at old tags — but only back to a point. Core
carried **0** at v0.60.0 (2022-09-14), 15 at v0.100.0 (2024-05-06) and 99 at
v0.158.0. Contrib adopted earlier and was already near-complete two years ago
(v0.106.0, 2024-07-29: 93 receivers / 24 processors / 45 exporters / 11
connectors / 23 extensions). **Practical floor: don't build a catalogue for
anything older than ~v0.100.0 (mid-2024).** Modern versions are complete.

### OCB manifests

`opentelemetry-collector-releases/distributions/<name>/manifest.yaml` is fully
machine-readable, one `gomod:` per component with the version pinned:

```yaml
dist: {name: otelcol, version: 0.158.0}
receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.158.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver v0.158.0
```

Five distros at v0.158.0: `otelcol` (32 modules), `otelcol-contrib` (252),
`otelcol-k8s` (74), `otelcol-otlp` (5), `otelcol-ebpf-profiler` (21). So
**"what is in distro X at version Y" is directly derivable** — join manifest
`gomod` paths to each component's `go.mod` `module` line.

### ⚠ `status.distributions` drifts from reality

Cross-checking the field against the manifests at v0.158.0 gives **10 mismatches**:
claims `contrib` but absent from the manifest — `spanpruningprocessor`,
`activedirectoryinvreceiver`, `googlecloudpubsubpushreceiver`, `signalfxreceiver`
(contrib HEAD is literally "remove signalfxreceiver from otelcontribcol");
shipped in `otelcol-contrib` but metadata omits it — `sumologicextension`,
`coralogixprocessor`, `icmpcheckreceiver`; shipped in `otelcol-k8s` but metadata
omits `k8s` — `spanmetricsconnector`, `cgroupruntimeextension`,
`metricsgenerationprocessor`.

Also `otelcol-ebpf-profiler` has **no token at all** — the canonical set in
`cmd/mdatagen/internal/status.go` is only `core | contrib | k8s | otlp`, so that
distro is invisible to `metadata.yaml`. **Treat `status.distributions` as a hint;
derive membership from manifests.**

---

## 5. Config-schema status

**`metadata.yaml` carries no config field schema for any real component, and
won't for several quarters.** Identity+stability scope is the right call.

`docs/rfcs/component-configuration-schema-roadmap.md` (merged 2026-01-23, PR
#14433; detail added 2026-06-09, #14486) defines four phases: **1** bootstrap
schemas via `schemagen`, **2** build the generation tool, **3** migrate all
components, **4** extend OCB to a whole-collector schema.

- **Phase 1 — effectively done.** `cmd/schemagen` (moved into core 2026-06-15)
  reverse-extracts JSON Schema from Go structs; **419 `config.schema.yaml` files
  committed** (27 core / 392 contrib). But its README says *"In Development"*,
  *"only for temporary use"*, it **cannot extract defaults or validation
  constraints**, and CI freshness covers only 9 dirs.
- **Phase 2 — tooling built, pilot unfinished.** mdatagen gained config JSON
  Schema generation (#14548, 2026-03-03), Go struct generation (#14693),
  validators, default setters and README generation (Feb–May 2026). The last
  open sub-issue **#14566 "Pilot migration of a handful of components"** has
  **zero comments and is untouched since 2026-06-10**.
- **Phase 3 — not started for components.** Migrated so far: shared config
  *packages* only (`configtls`, `configretry`, `configcompression`,
  `configmiddleware`, `configauth`, `scraperhelper`, `filter`) via an
  `exported_configs:` key. **Not one receiver/processor/exporter/connector/
  extension has been migrated.**
- **Phase 4 — blocked.** PR #15366 (combined schema via OCB) open since
  2026-05-27, `mergeable_state: blocked`; the author pinged the OCB codeowner
  2026-06-10, 06-29 and 08-05 with no response. Issue #15010
  (`otelcol print-schema`) untouched since 2026-04-27.

Verified on `main` today: `metadata.yaml` with top-level `config:` = **3, all
mdatagen samples**; with `exported_configs:` = **10, all shared core packages**;
generated `config.schema.json` = **11 core, 0 contrib**. Tracking issue **#14543**
is open, created 2026-02-06, **last comment 2026-03-05 — five months stale**, no
labels, no milestone, its Phase-2 checklist never updated for the March–May work.

**Timeline signal: pessimistic.** Tool-building moved fast; adoption stalled on
human review. A published whole-collector schema is gated on a PR blocked two
months plus bug #15728 (the combined schema rejects the very common empty-body
`receivers:\n  otlp:` form). **Do not plan around config schemas landing in
metadata.yaml before mid-2027.** For G7 form generation, the `config.schema.yaml`
bootstrap files are the pragmatic interim source — broad coverage, but no
defaults or validation.

---

## 6. Vendorability

All four repos are **Apache-2.0** (confirmed from each `LICENSE`). Apache-2.0
permits redistribution of derived data; retain the licence text and a NOTICE
attributing sources. No copyleft, no field-of-use restriction. Component names
and stability are facts, not creative content — low risk regardless.

Footprint: full shallow clone of core + contrib is 266 MB, but a **pinned sparse
checkout** (`**/metadata.yaml`, `**/go.mod`) of core @ v0.158.0 is **3.9 MB**.
All raw `metadata.yaml` bytes ≈ 960 KB; the emitted `catalogue.json` ≈ 110 KB,
**~8 KB gzipped**.

### Recommended offline pipeline

Build-time only; the air-gapped runtime never fetches anything.

1. Pin refs: `collector@vX.Y.Z`, `contrib@vX.Y.Z`, `releases@vX.Y.Z`.
2. Sparse shallow fetch of `**/metadata.yaml`, `**/go.mod`,
   `distributions/*/manifest.yaml`. Verified working:
   `git init && git sparse-checkout init --no-cone && git fetch --depth 1 origin refs/tags/vX.Y.Z`
   → 3.9 MB, 99 files for core.
3. Walk **recursively** (§2a): require a sibling `go.mod`, parse
   `metadata.yaml`, keep only the five pipeline classes (drops 16 `pkg`), key by
   **`(class, type)`**.
4. Resolve each Go module path from `go.mod`; parse the five distro manifests;
   compute real distro membership by module-path join. Log
   `status.distributions` divergence rather than trusting it.
5. Emit `catalogue.json`: `{catalogue_version, collector_version, generated_at,
   source_commits{core,contrib,releases}, components[]}` with per component
   `{class, type, deprecated_type, repo, module, display_name,
   stability{signal→level}, distributions[], deprecation{}, warnings[],
   codeowners[], unsupported_platforms[]}`.
6. Embed in the release artefact (no sidecar service). Record source SHAs so a
   catalogue is reproducible and auditable.

A proof-of-concept for steps 3–5 ran in this session against v0.158.0 and
`main`, producing all 267 components with correct stability.

**Cross-check upstream rather than replacing it.** Consuming ecosystem-explorer
directly is tempting but it ships 5 `class: pkg` false positives with no
compatibility guarantee. Pragmatic split: **build from `metadata.yaml` ourselves**
(~40 lines of walker, we control correctness) and **diff against the explorer
registry in CI** as a free upstream sanity check and drift alarm.

---

## 7. Stability vocabulary

Six levels, defined in `docs/component-stability.md` (core), paraphrased:

| Level | Definition | Count* |
|---|---|---|
| `development` | Not all pieces in place; may not be in any distribution; config may break often. **Not for production.** | 19 |
| `alpha` | Limited non-critical workloads. Config can change with minimal notice. | 126 |
| `beta` | As alpha, but **config options deemed stable**; breaking changes minimised. Expected prior non-critical production exposure. | 115 |
| `stable` | Generally available. No breaking changes without prior notice. | 3 |
| `deprecated` | Planned for removal, no further support. Exists **at least two minor releases** after entering. | 3 |
| `unmaintained` | **No active code owner.** Removed from official distribution after **3 months**. | 1 |

\* Components whose *highest* level is that value, over 267. Raw per-level
tallies (a component can appear in several): alpha 139, beta 115, development
42, deprecated 4, stable 3, unmaintained 1.

Note how thin the top is: **only 3 components are `stable`** (`otlp`
receiver/exporter, `otlphttp` exporter — and only for traces/metrics/logs). A
"stable only" policy would make the platform unusable. Realistic allow-list
floors are `beta` for production Service Classes, `alpha` for the long tail.

Policy-relevant observations:

- **Stability is per-signal.** 36 components carry ≥2 levels — `otlpreceiver` is
  `stable` for traces/metrics/logs but `alpha` for profiles. **A "beta or
  better" rule must evaluate per (component, signal)**, or it wrongly admits
  alpha profiles support or wrongly excludes a stable receiver.
- `deprecated` and `unmaintained` are **not** rungs on the maturity ladder but
  orthogonal end-states. Model them as a separate `lifecycle` axis. Deprecated
  components carry machine-readable `deprecation.<signal>.{date,migration}` —
  ready-made remediation advice for the console.
- **62 components have a `deprecated_type` alias** (`spanmetrics` →
  `span_metrics`). Allow-list matching and config linting must resolve aliases
  or teams hit false "not in Catalogue" failures on working configs.
- 15 carry free-text `warnings`; 9 have no `codeowners.active` — a leading
  indicator of drift toward `unmaintained`.
- 15 distinct signal tokens exist (connectors use `X_to_Y`, extensions the
  literal `extension`). Treat the vocabulary as **open** — pass unknown tokens
  through rather than validating against a closed enum; it grew twice in 2 years.

---

## Recommendations for session G2

1. **Adopt `metadata.yaml` as the Catalogue substrate** — coverage is complete
   (267/267), the schema stable, and it is trivially vendorable.
2. **Build the walker ourselves; diff against ecosystem-explorer in CI.** Don't
   consume the upstream registry directly (5 `class: pkg` false positives, no
   compatibility guarantee) — but don't ignore it either (§3).
3. **Key on `(class, type)`, and enumerate recursively** by `go.mod` discovery,
   filtering on `status.class`. `type` alone collapses 29 components; a depth-1
   walk drops `file_storage`, `k8s_observer` and 18 others (§2).
4. **Scope to identity + stability + lifecycle.** Don't wait for or design
   around config schemas (§5); revisit at G7 for form generation.
5. **Derive distribution membership from OCB manifests**, not
   `status.distributions` (§4). Log divergence — it's a useful drift signal.
6. **Model stability per (component, signal)**; retrofitting is painful. Keep
   `deprecated`/`unmaintained` on a separate lifecycle axis, and note a "stable
   only" policy is unusable — only 3 qualify (§7).
7. **Resolve `deprecated_type` aliases** in Catalogue lookups (62 components).
8. **Pin and record source commits** in every catalogue artefact; make the
   Catalogue a build-time output embedded in the release (~8 KB gzipped).
   **Do not build a runtime fetcher**, even behind a flag — ADR-0019's air-gap
   constraint is satisfied free by build-time generation, and a fetcher would
   create two code paths and a drift surface.
9. **Consider "catalogue diff" as a first-class feature.** Pinned reproducible
   catalogues make v0.157 → v0.158 upgrade-impact reports cheap (newly
   deprecated, removed, stability changes). Upstream's `schema_hash` and
   `deprecations.yaml` are prior art worth mirroring.
10. **Add an ADR** recording the substrate choice, the `(class, type)` key and
    the manifest-over-metadata decision — non-obvious calls future sessions will
    otherwise re-litigate.
