---
title: Catalogue
description: The Catalogue artefact format, its (class, type) key, how it is imported from upstream metadata.yaml, and how a version is made active.
order: 9
---

# Catalogue

The Catalogue is the versioned inventory of otelcol component types: each
component's identity, per-signal stability, and lifecycle. Telecraft
generates it from the `metadata.yaml` files of
`opentelemetry-collector-contrib` at a pinned release tag.

You can't curate the component list by hand. The only way a component enters
a Catalogue is through the import pipeline, which walks an upstream source
tree.

A Catalogue states what exists. What a Team can use is the
[Allow-list](allow-lists.md).

## The (class, type) key

The primary key is the pair `(class, type)`. `type` alone isn't unique,
because the same type string appears in more than one class: `kafka` is both
a receiver and an exporter.

`class` is one of five pipeline classes:

| Class |
|---|
| `receiver` |
| `processor` |
| `exporter` |
| `connector` |
| `extension` |

The import excludes upstream's helper classes, such as `pkg`, `cmd`,
`scraper`, `converter`, and `provider`, and records them in its coverage
report.

`deprecated_type` aliases resolve on every lookup, so a config that says
`spanmetrics` still finds `span_metrics`.

## Stability

Stability is per signal: one component can be beta for logs and alpha for
profiles. A floor is judged per (component, signal), never per component.

| Level | Kind |
|---|---|
| `development` | maturity rung |
| `alpha` | maturity rung |
| `beta` | maturity rung |
| `stable` | maturity rung |
| `deprecated` | lifecycle end-state |
| `unmaintained` | lifecycle end-state |

The maturity ladder is `development` < `alpha` < `beta` < `stable`. A
[stability floor](tiers.md#how-paths-set-a-tiers-floor) compares against
that ladder. Lifecycle end-states have no rung; lifecycle findings judge them
separately.

The six-level vocabulary is closed: an unknown level fails the load, so a new
upstream level is noticed rather than passed through. The signal vocabulary is
open: unknown signal names pass through, because upstream adds signals over
time.

## Artefact format

One Catalogue version is one JSON file. The encoding is deterministic:
components in `(class, type)` order, object keys sorted, no timestamps.
Importing the same tag twice produces byte-identical artefacts, and you can
diff artefacts across an air gap.

| Field | Type | Description |
|---|---|---|
| `format_version` | integer | The artefact format. The supported value is `1`. |
| `source` | object | Where this Catalogue came from. |
| `source.repository` | string | The upstream repository identity, such as `github.com/open-telemetry/opentelemetry-collector-contrib`. |
| `source.ref` | string | The pinned release tag. This is the Catalogue's version. |
| `source.commit` | string | The commit that tag resolved to. Omitted when the import ran against a tree with no `.git`. |
| `components` | array | Every pipeline component type found at that tag. |

Each entry of `components`:

| Field | Type | Description |
|---|---|---|
| `class` | string | One of the five pipeline classes. |
| `type` | string | The component type. |
| `deprecated_type` | string | The historical alias, when upstream declares one. Omitted otherwise. |
| `module` | string | The Go module path from the component's sibling `go.mod`. |
| `display_name` | string | Upstream's display name. Omitted when absent. |
| `description` | string | Upstream's description. Omitted when absent. |
| `stability` | object | Map from signal name to level. |
| `deprecation` | object | Map from signal name to `{ "date", "migration" }`, for deprecated signals. Omitted when empty. |

```json
{
  "format_version": 1,
  "source": {
    "repository": "github.com/open-telemetry/opentelemetry-collector-contrib",
    "ref": "v0.158.0",
    "commit": "9f0c1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b"
  },
  "components": [
    {
      "class": "exporter",
      "type": "otlphttp",
      "module": "go.opentelemetry.io/collector/exporter/otlphttpexporter",
      "stability": {
        "logs": "beta",
        "metrics": "stable",
        "traces": "stable"
      }
    }
  ]
}
```

### Artefact naming

An artefact is written as `catalogue-<ref>.json`, so versions sit side by
side. A release tag containing `/` or `\` can't name an artefact file and is
rejected.

Writes are atomic: the bytes land in a temporary file, which is then renamed
into place, so a reader never sees a half-written Catalogue. If the file
already holds exactly those bytes, the write is skipped.

### Load errors

Loading fails closed and returns nothing. An artefact travels: bundled in a
release, downloaded, or carried across an air gap. A tampered or truncated
artefact would corrupt every judgement made against it, so the load refuses
on:

- an unknown field, or trailing data after the document
- a `format_version` other than `1`
- an empty `source.repository` or `source.ref`
- no components at all
- a component with a non-pipeline class, an empty `type`, or an empty `module`
- a `deprecated_type` equal to the component's own `type`
- a component with no `stability`, an empty signal name, or an unknown level
- a signal marked `deprecated` with no deprecation notice, a notice for a
  signal that isn't deprecated, or a notice with no `migration` text
- two components sharing the `(class, type)` key
- a `deprecated_type` that's another component's real type, or claimed by two
  components

## Importing with catalogue-import

```sh
catalogue-import -tag v0.158.0
```

writes `catalogues/catalogue-v0.158.0.json` and prints the coverage report.
See the [command line reference](cli.md#catalogue-import) for every flag and
exit code.

The fetch is a sparse, depth-1 git checkout of only the `metadata.yaml` and
`go.mod` files, a few megabytes rather than the whole repository, into a
temporary directory that's removed afterwards. The upstream tree is never
vendored. Fetching happens at import time only, on your machine, and the
artefact is what travels onward. Telecraft never fetches at runtime.

Import an already-fetched tree offline with `-source`:

```sh
catalogue-import -tag v0.158.0 -source /path/to/contrib
```

A tree copied without its `.git`, which is what an air-gap transfer usually
produces, still imports. The artefact then records the tag alone, with no
source commit.

### Discovery

Discovery is by sibling `go.mod`, recursively, never by directory depth.
Depth-based discovery would miss the contrib extensions nested a level
deeper, such as `extension/storage/filestorage`.

A directory is a component candidate when it holds a `go.mod`. It enters the
Catalogue when it also holds a `metadata.yaml` whose `status.class` is one of
the five pipeline classes.

The import reads only the fields of upstream's `metadata.yaml` it needs and
ignores the rest: `type`, `deprecated_type`, `display_name`, `description`,
and the `status` block's `class`, `stability`, and `deprecation`. Upstream's
stability map runs from level to signals; the import inverts it to signal to
level. A signal listed under two levels fails the import.

### Coverage report

The import prints an account of the whole tree, so nothing the walker saw goes
unrecorded:

| Section | What it holds |
|---|---|
| found | The count of components that entered the Catalogue, broken down by class. |
| excluded by class | Parsed components whose class keeps them out, with the class recorded. |
| missing | Directories under a component root that hold a Go module but no `metadata.yaml`. |

A directory outside the component roots with no `metadata.yaml` is ordinary Go
layout, not a gap, and isn't listed.

The import fails closed on anything malformed: a `metadata.yaml` that doesn't
parse, a pipeline component without stability, or a duplicate `(class, type)`
key. A gap is reported, never silently dropped.

## Versions and activation

A Catalogue is versioned against one collector release tag. There's no
partial upgrade: a Catalogue is the whole tag.

Installed catalogues are kept, never replaced. Re-importing the same tag is
idempotent and leaves the existing artefact untouched. A different tag writes
a new artefact beside the old one.

The active Catalogue is the artefact you pass as `-catalogue`. Every command
that judges authoring against the Catalogue takes it explicitly:

| Command | Flag | Used for |
|---|---|---|
| `telecraft palette` | `-catalogue` | Validating entries and materialising the palette. |
| `telecraft render` | `-catalogue` | The allow-list block and stability floors. |
| `telecraft check` | `-catalogue` | `library_drift` detection, with `-source`. |
| `telecraft snapshot` | `-catalogue` | The active version, marked as such among the installed set. |

`telecraft snapshot` also takes `-catalogues`, a directory of installed
artefacts. It reads every `catalogue-*.json` in that directory, marks the one
given by `-catalogue` as active, and includes the active artefact even when it
lives elsewhere. The default is the directory holding `-catalogue`.

An Allow-list policy is bound to the Catalogue version it was validated
against, because every entry must select at least one component in it.
Activating a different Catalogue validates the policy again.
