# Normaliser spike verdict — three-layer drift hashing (issue #13, ADR-0005)

- Status: **awaiting human review** — merging the PR that carries this file
  records the ruling; ADR-0005 amendments land separately after it
- Date: 2026-08-17
- Spike code: this directory (nested Go module, never built by root CI)

## What was spiked

ADR-0005's three layers, against a corpus of three realistic collector
configs (edge-on-Kubernetes shaped after the ticket 06 live capture, a
gateway tier, a GitOps-delivered host-metrics node) with cosmetic variants,
real delivery-path mutations, and seeded semantic changes:

- **Layer 1** — digest of raw bytes (`Layer1`).
- **Layer 2** — digest of the canonical form after applying a mutation
  allow-list (`Layer2`): parse (YAML or JSON), transform, deterministic
  type-tagged re-encoding, SHA-256.
- **Layer 3** — structural diff of the normalised trees (`Layer3`), computed
  only when layer 2 disagrees.

The allow-listed mutations are exactly the catalogued ones: the OpAMP
Supervisor's injected `extensions.opamp` block (ephemeral localhost port) and
`opamp` appended to `service.extensions` (tickets 01/06); Elastic Fleet's
key-substring redaction, opamp-extension body stripping, and YAML→JSON
re-marshalling (tickets 02/06/12).

## What held

- **H-1 One canonicalisation move kills four cosmetic axes.** Parse +
  sorted-key, type-tagged re-encode neutralises key order, quoting/flow
  style, anchors/aliases, and YAML-vs-JSON in one mechanism, with no
  per-case handling. `TestCosmeticVariantsAgreeAtLayer2` — every variant
  agrees at layer 2 under every profile while layer 1 differs.
- **H-2 Supervisor mutations allow-list as *shapes*.** The injected endpoint
  port is ephemeral, so the allow-list entry is a pattern
  (`^ws://127\.0\.0\.1:\d+/v1/opamp$`) plus list-entry removal and
  empty-container cleanup. Rendered and Supervisor-reported configs agree at
  layer 2; the exact profile still flags them, proving the allow-list is
  load-bearing and not decorative. `TestSupervisorReportAgrees`.
- **H-3 Fleet comparability = damaging the rendered side identically.**
  Applying Fleet's redaction rule and opamp-body strip to *both* sides makes
  the rendered config and Fleet's redacted JSON report agree at layer 2.
  `TestElasticFleetReportAgrees`.
- **H-4 Semantic changes cannot hide.** Processor reorder, processor
  removal, exporter endpoint change, and interval change each flip layer 2
  under **every** profile, and layer 3 localises each to exactly the changed
  paths with zero noise. Pipeline list order is preserved as semantic
  (ticket 11 §4). `TestSemanticChangesFlipLayer2AndLocalise`.

## What broke

- **F-1 There is no single "normalised digest".** A layer-2 digest is only
  meaningful relative to a delivery path's mutation profile (exact /
  supervisor / elastic-fleet). Comparing digests across profiles is a
  category error the spike prevents by mixing the profile name into the hash
  domain. ADR-0005 describes layer 2 as one thing; it is a family.
- **F-2 "Explicit defaults" are not normalisable and should be struck from
  the cosmetic list.** Equating `batch: {}` with
  `batch: {send_batch_size: 8192, timeout: 200ms}` requires every
  component's default table, which lives in component Go code
  (`createDefaultConfig`), not in `metadata.yaml` — so the Catalogue (#14)
  cannot supply it either. It is also *unnecessary* for the drift check:
  both delivery paths report the merged **input** config, not a
  defaults-expanded form, so rendered-vs-reported never disagrees on
  defaults alone. The only source of explicit-default variance is a human
  editing the file, which is a visible edit, not drift.
  `TestExplicitDefaultsDoNotAgree` pins the behaviour.
- **F-3 The Fleet profile is structurally blind inside redacted values.** A
  rotated exporter credential yields byte-identical layer-2 digests under
  the elastic-fleet profile — silent no-drift on a real change, the exact
  failure ADR-0005 fears. It is not a bug; it is the price of comparing
  through a lossy reporter, bounded to the redaction list.
  `TestElasticFleetProfileIsBlindToRedactedValues` pins it so the cost stays
  named.
- **F-4 The redaction rule is Fleet's and version-coupled.** The substring
  list (`auth|certificate|passphrase|password|token|key|secret`) was
  observed live on ticket 06's run. If a Fleet release changes it, layer 2
  reports false drift estate-wide until the profile follows. This needs the
  ADR-0008 discipline: a contract test against the live API and the
  redaction list versioned with the provider, not hard-coded in core.
- **F-5 The opamp extension body is wholly unverifiable via Fleet.** The
  server block arrives absent (not redacted) and the one surviving field
  (`polling_interval`) surfaces at a different position than authored. The
  spike therefore empties `opamp*` extension bodies on both sides — entry
  *presence* still compares, contents do not. Consequence: where the
  extension points cannot be drift-checked through Fleet, only through the
  platform's own delivery path.

## Proposed ADR-0005 amendments

1. Layer 2 is **parameterised by delivery-path profile**; the profile is
   part of digest identity (domain separation), and digests from different
   profiles are never comparable.
2. **Strike "explicit defaults"** from the cosmetic/normalisable list
   (rationale in F-2).
3. **Name the Fleet-path blindness** (redacted values, opamp extension
   bodies) as an accepted, bounded cost, with the redaction list pinned to
   observed Fleet behaviour and contract-tested (F-3/F-4/F-5).
4. State that allow-list entries are **shapes/patterns, never literals**
   (the Supervisor port is ephemeral).

## Not covered (known edges for the production build)

- Duplicate map keys and YAML merge keys (`<<`) — untested; production
  should parse via `yaml.Node` and fail closed on duplicates.
- The corpus is realistic but synthetic; it should grow from real estate
  captures as they appear.
- Layer-3 list diffing is positional; a single mid-list insertion reports
  the tail as changed. Fine for "drifted where", coarse for "fix what".

## Acceptance criteria → evidence

| AC | Evidence |
| --- | --- |
| Cosmetic variants hash equal at the semantic layer | `TestCosmeticVariantsAgreeAtLayer2`, `TestSupervisorReportAgrees`, `TestElasticFleetReportAgrees` — except explicit defaults, deliberately: F-2 |
| Semantic changes flip the expected layer only | `TestSemanticChangesFlipLayer2AndLocalise` (layer 2 flips under every profile; layer 3 localises; layer 1 trivially differs) |
| Written spike verdict | this document |
| Human review recorded before delivery-status work (#21) | pending — merging this PR records the ruling |

Run: `go test ./...` in this directory (6 tests).
