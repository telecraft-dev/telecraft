# ADR-0020: Catalogue sourcing, construction and versioning

- Status: accepted
- Date: 2026-08-13 (session G2)

## Context

Blueprints compose Components; Allow-lists select from an inventory of what
exists. That inventory, the Catalogue, needs a substrate, a build process,
an update story, and an answer for air-gapped instances (ADR-0019) and for
estates running mixed collector versions. Research task R-1
(`docs/research/2026-08-12-r1-metadata-catalogue.md`) established the facts;
this ADR records the decisions taken on them.

## Decision

1. **Substrate: upstream `metadata.yaml`** (collector core + contrib), scoped
   to **identity + per-signal stability + lifecycle**. Config field schemas
   are explicitly out of scope: they do not exist upstream and are not
   expected before mid-2027 (R-1 §5); revisit at G7 for form generation.
2. **We build the walker ourselves** (recursive discovery by sibling
   `go.mod`, keep only the five pipeline classes, dropping `class: pkg`)
   and **diff the output against `opentelemetry-ecosystem-explorer` in CI**
   as a drift alarm. The upstream registry is never consumed directly (known
   `class: pkg` false positives, no layout guarantee).
3. **Primary key is `(class, type)`**; `deprecated_type` aliases resolve on
   every lookup. `type` alone collapses 29 real components.
4. **Distribution membership derives from OCB release manifests**, never
   from `status.distributions` (10 known mismatches at v0.158.0); divergence
   is logged as a drift signal.
5. **Build-time baseline, one import pipeline, three transports.** Every
   release embeds the catalogue for its pinned collector version, with
   source commits recorded. Updates arrive as the identical artefact through
   any of: (a) bundled in a release, (b) an online check that discovers and
   downloads newer catalogues, (c) operator upload across an air gap. All
   three feed one validator and one activation step. There is no separate
   runtime-fetch code path to drift; air-gapped instances run the same
   pipeline minus the convenience transport.
6. **Activation is explicit and audited.** The instance prompts operators
   (not general console users) when a new catalogue is available, presenting
   an **impact report computed before activation**: newly deprecated or
   removed components in use, by which Blueprints and Teams; stability
   changes crossing floors. No silent auto-apply.
7. **Catalogue versions are atomic.** Components release only in lockstep
   with tagged collector releases, so there is nothing item-level to poll
   for; cherry-picking parts of a release would fabricate a catalogue that
   matches no upstream reality. The itemised list is what the prompt shows,
   not what the operator chooses from.
8. **Activating a catalogue can never break a running collector.** The
   Catalogue is metadata about components; it is not part of rendered
   configs and is never shipped to collectors. Activation changes
   judgement (findings, palettes, floors), never pipelines.
9. **Installed catalogues are retained, never replaced.** Authoring and the
   Palette judge against the designated *active* catalogue; **evaluation of
   a collector consults the catalogue for the version it actually runs**.
   The platform does not control collector binaries (ADR-0002), so version
   is a discovered fact; a collector whose version has no installed
   catalogue is judged against the nearest older one with the judgement
   flagged degraded (the Known rule: say "cannot fully know", never assert
   falsehoods), and the operator is prompted to import the missing version.
   Retained pairs make upgrade-impact diffs ("v0.155 → v0.158 deprecates two
   components you run") a cheap first-class feature.
10. **Adopter-authored entries** (private/OCB-built components) are
    first-class: same shape as upstream entries (`(class, type)`, display
    name, per-signal stability (mandatory, self-declared), lifecycle),
    marked `source: adopter`, imported item-level, and judged identically
    downstream. **Collisions with upstream keys are rejected**: upstream
    keys are reserved; a fork is a different component and carries its own
    type name. Shadowing would let an overlay rewrite upstream facts and
    corrupt impact reports.

## Consequences

- The catalogue artefact format is a public, versioned contract: it must be
  validated on import and evolved compatibly.
- The online check requires outbound reachability and is strictly optional;
  no platform feature may depend on it.
- Catalogue-version-aware lookup appears everywhere the Catalogue is
  consulted; evaluation carries (collector version → catalogue) resolution.
- The walker and the artefact generator ship as a published tool so adopters
  can regenerate catalogues themselves and carry them across air gaps.

## Sources

- R-1 research (`docs/research/2026-08-12-r1-metadata-catalogue.md`);
  session G2.
