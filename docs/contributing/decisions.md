---
title: Decisions and the design corpus
description: The ADR process, why an accepted ADR is amended rather than edited, the traceability matrix, and a themed map of the 51 ADRs.
order: 7
---

# Decisions and the design corpus

Telecraft is designed in documents before it is written in code. The corpus is
not background reading: it is where the answer to "why is it like this?"
lives, and it is the thing a pull request is measured against.

## The ADR process

Decisions are recorded as architecture decision records in `docs/adr/`,
numbered sequentially, one decision per file. ADR-0000 defines the process
and is the first thing to read.

Every ADR has the same sections: **Status**, **Context**, **Decision**,
**Consequences**, and **Sources**. A status is one of `proposed`, `accepted`,
`superseded by ADR-nnnn`, or `rejected`.

Every capitalised domain term an ADR uses has an entry in
`docs/glossary.md`. If your decision needs a new term, the glossary entry is
part of the change.

ADRs 0001 to 0014 are **seeded**: retroactive records of decisions made during
an earlier shaping effort, written down here so the corpus reads as one
uniform body. Each cites the shaping tickets and research that produced it.
They arrived `accepted` because the arguing had already happened. ADR-0015
onward are produced by design sessions in this repository.

Some seeded ADRs carry a vocabulary note, because they were written before
ADR-0015 renamed five core terms. Read Stage as Tier, Criticality Tier as
Service Class, Classification as Sensitivity, Declared as Effective, and
Application as Service.

## Amend, never edit

**A seeded or accepted ADR is amended by a new ADR, not edited.** Re-opening
a decision requires a superseding record that argues the change, so the
history of what was believed and when stays readable. The corpus is meant to
be an audit trail, and an edited decision destroys the trail.

The pattern is already visible in the corpus:

- ADR-0015 supersedes the *naming* halves of ADR-0004 and ADR-0007, leaving
  their models unchanged.
- ADR-0046 amends ADR-0005's hashing rules rather than rewriting them.
- ADR-0028 generalises ADR-0014, restating a forge-specific decision in
  forge-neutral terms.

Correcting a typo in an ADR is fine. Changing what it decides is a new ADR.

## The traceability matrix

`docs/requirements/traceability.md` is the verification artefact: every
requirement maps to at least one ADR and one build phase. Zero unmapped rows
is the gate the backlog was generated behind, and an unmapped requirement is
a gap rather than an option.

If your change adds a requirement, add its row. If your ADR decides an
existing requirement, update the row to cite it.

## The corpora, and what each is for

| Path | What it is |
|---|---|
| `docs/adr/` | The decision corpus: 52 records, ADR-0000 to ADR-0051. Normative. This is what code is measured against. |
| `docs/requirements/` | `product-requirements.md`, the numbered requirements the product exists to satisfy, and `traceability.md`, mapping each to the ADRs and build phase that deliver it. Normative. |
| `docs/research/` | Dated findings dossiers and the archived shaping tickets: the evidence ADRs cite. Not normative, and never edited to match a later decision. It records what was found, when. |
| `docs/prototypes/` | Verdict one-pagers from throwaway prototypes, plus the normaliser spike's verdict. Prototype code is never merged; the verdict is what survives, and several ADRs rest on one. |
| `docs/plan.md` | How the repository went from seeded documents to a build backlog: the design sessions in dependency order, the prototype and research tracks, and the build phases. |
| `docs/open-questions.md` | The register of everything unresolved, each with a status: owned by a scheduled session, `carried` with its reason, or `resolved-by-ADR-nnnn`. A question may not be silently dropped. Read the `carried` rows before proposing a feature; several are refused-for-now rather than undecided. |
| `docs/glossary.md` | Every capitalised domain term, grouped by area. |
| `docs/terminology.html` | The visual terminology guide, which is what surfaced the ADR-0015 renames. |
| `docs/branding/` | The naming decision and its collision sweep. |

The vendor-word lint runs over `docs/**` but excludes the research, prototype
and branding trees, because those record external reality verbatim: survey
quotes, product names, collision sweeps.

## A map of the 51 ADRs

Read ADR-0000 first, then ADR-0001, ADR-0002, ADR-0003 and ADR-0004. Those
five carry the principles everything else assumes. After that, read the group
that covers what you are working on.

### Foundations

The rules that hold everywhere.

- **ADR-0000** Architecture decision records, and how the seeded ones differ
- **ADR-0001** Neutral core; vendor words are a lint error
- **ADR-0002** Ships configurations, never binaries; exactly one artefact
- **ADR-0003** Git is the source of truth; the UI opens pull requests
- **ADR-0015** Vocabulary aligned to industry and upstream usage
- **ADR-0019** Pluggable authentication; ownership-derived authorization; air-gap first-class
- **ADR-0049** Releases are tags on main; the public demo follows a moving pointer
- **ADR-0050** Elastic License 2.0; the project is source-available, not open source

### Readings and verdicts

How the product decides whether something is right.

- **ADR-0004** Three readings; OpAMP delivery vocabulary; `library_drift`
- **ADR-0005** Drift is judged by a three-layer hash; the normaliser is allow-listed
- **ADR-0009** Conformance adopts semconv YAML and Weaver
- **ADR-0026** Pinned references with opt-in tracking; `satisfies` mechanics; `library_drift` defined
- **ADR-0033** Evaluation is per Service and Environment; Requirements scope by environment list
- **ADR-0034** Schema conformance: registry substrate, two placements, Service-owned findings
- **ADR-0037** Exemption authority via requirement-owner review; subtree scope
- **ADR-0038** The Expectation engine: machinery behind existing verdicts, never new vocabulary

### The estate model

What the world is shaped like, and where it lives in git.

- **ADR-0007** The topology model: Tier, Hop, Path; collectors are never drawn
- **ADR-0012** Kubernetes is the control plane's substrate; no custom resource per collector
- **ADR-0018** One estate monorepo, path-per-team
- **ADR-0023** The Environment axis; stability floors per Service Class and Environment
- **ADR-0025** The Tier is the rendering and binding unit; Tiers declare an Environment
- **ADR-0027** Estate repo layout; satellite repos for private subtrees

### Ownership and governance

Who owns what, and what they are allowed to use.

- **ADR-0016** Universal ownership on authored objects; the Component as a first-class unit
- **ADR-0017** Team hierarchy is a tree; roll-up is ratio-plus-worst per kind
- **ADR-0020** Catalogue sourcing, construction and versioning
- **ADR-0021** Allow-list policy: narrowing inheritance with owned Grants
- **ADR-0022** One evaluator as a service; one hard rule at render
- **ADR-0046** Layer-2 digests are a per-delivery-path family; allow-list entries are shapes

### Authoring and rendering

How a human's intent becomes a collector config.

- **ADR-0006** A small purpose-built console; Backstage rejected with the door held open
- **ADR-0011** No Helm or Kustomize rendering in v1
- **ADR-0014** GitHub is the v1 host; authentication is a GitHub App with attributable actions
- **ADR-0024** Blueprint schema v1: a domain document where everything is a Component
- **ADR-0028** Render-in-PR; the forge is a seam; repo onboarding by credential; CI fails closed

### Delivery and serving

How the config reaches collectors, and what the platform remembers.

- **ADR-0010** Serving uses the upstream OpAMP Supervisor; renderer hard rules
- **ADR-0013** The OpAMP server is stateless transport; the artefact carries its own identity
- **ADR-0029** Staged rollout as git state; the Rollout object and Cohorts
- **ADR-0030** First boot: the Unmatched artefact; `never_seen` is a Tier-attached finding class
- **ADR-0031** Ungoverned in the estate view: two referents, both drain by onboarding
- **ADR-0032** The server's cache inventory is a closed list; git-the-tool, never git-the-service

### Seams and readings

The contracts a vendor implementation answers. See
[Providers](providers.md).

- **ADR-0008** `EstateProvider`, keyed on the collector, with two implementations from day one
- **ADR-0035** `InventoryProvider` floors; `never_seen` teeth and `under_populated`
- **ADR-0036** The `EstateProvider` contract: capability declaration, `as_of`, staleness demotion, shipped test kit
- **ADR-0039** Self-telemetry ingestion: a rendered pattern over the `TelemetryProvider` seam
- **ADR-0040** Metering: derived flow readings, computed on read, stored nowhere

### The console

What the product looks like. See [Console](console.md).

- **ADR-0041** The card data contract: face and drawer, states never colours
- **ADR-0042** Surface inventory, activity-first navigation, and the shelf
- **ADR-0043** The composer: three surfaces, one Blueprint, one engine
- **ADR-0044** One canvas engine, two vocabularies; geometry that cannot lie
- **ADR-0045** Console tech stack: boring on purpose, sovereignty where it counts
- **ADR-0047** Visual identity: our own token layer, dark-first with a light twin, colour never load-bearing
- **ADR-0048** A console primitive layer, and panel width as a reader's preference
- **ADR-0051** Guided Tours: authored Steps over the running console, never driving it

## Proposing a decision

Open an issue describing the question and why the corpus does not answer it.
If the answer is a decision, write the ADR: take the next free number, follow
the section structure, cite what the decision rests on in **Sources**, and add
any new terms to the glossary. Then update
`docs/requirements/traceability.md` if a requirement now maps to it, and
`docs/open-questions.md` if the ADR resolves an open question.

An ADR arrives as a pull request like anything else, and the discussion
happens there.
