# Planning & build plan

How this repository goes from seeded docs to a build backlog. The workflow:
grill-with-docs sessions produce ADRs and grow the glossary; throwaway
prototypes stress-test feel; `/to-issues` consumes the phased skeleton once
the exit gate passes.

## Grill sessions (dependency-ordered)

| # | Topic | Must produce | Ordered here because |
|---|---|---|---|
| G0 | Naming & branding (parallel track) | `branding/naming.md` decision, identity direction | The name lands in module paths, the commit-stamp key, and every doc; must precede G3. |
| G1 | Tenancy, teams & ownership | Owner/Team hierarchy ADR; compliance roll-up ADR (incl. cardinality source, OQ-5); authn/authz + GitHub App attribution ADR; tenancy-to-git ADR (OQ-7/OQ-8) | Highest-leverage new constraint: reshapes allow-list scoping, repo layout, cohorts, exemption authority. |
| G2 | Component catalogue & governance | Catalogue sourcing/versioning ADR; allow-list policy ADR; enforcement-points ADR (OQ-9) | Allow-lists are team-scoped (G1); the palette constrains G3. |
| G3 | Blueprint schema & rendering | Blueprint schema v1 ADR; satisfies/`library_drift` ADR; rendered-repo layout ADR; PR workflow ADR (OQ-10) | Needs G1's repo layout and G2's palette. |
| G4 | Delivery, rollout & estate status | Staged-rollout ADR (cohort-as-git-state tested hard, OQ-1); bootstrap ADR (OQ-2); ungoverned-in-view ADR (OQ-3); server cache ADR (OQ-15) | Largest undesigned piece; operates on G3's artefacts. |
| G5 | Conformance extensions | Schema-conformance requirement-kind ADR incl. live-check-tap ruling (OQ-4); expected-but-never-seen ADR; exemption-inheritance ADR; EstateProvider contract ADR (OQ-6) | Needs G1 (authority) and G4 (delivery status as input). Defines Expectation before G6 builds on it. |
| G6 | Pipeline observability & the Expectation engine | Expectation model ADR; self-telemetry ingestion ADR; metering ADR; card data-contract ADR | Most novel ground; inherits settled models and P3/P4 verdicts. |
| G7 | UI information architecture | Surface inventory + navigation ADR; winning composer variant ADR; canvas interaction ADR; tech stack ADR (OQ-14) | Last: consumes every prototype verdict. |
| n/a | Closing consistency grill | Amendments only | Danger pairs: tenancy↔repo-layout↔attribution; cohort↔GitOps path; expectation-findings↔"can't-report ≠ failing". |

## Prototype track (`/prototype`, throwaway; verdict one-pagers in `docs/prototypes/`)

| # | Prototype | Question | After | Feeds |
|---|---|---|---|---|
| P1 | Blueprint composer, 3 variants rebuilt fresh (Catalogue / Requirement-first / Signal lanes) | Which mental model survives phase ordering + allow-lists? | G2 | G3, G7 |
| P2 | Estate & team roll-up view | Does hierarchy roll-up read clearly? Where do ungoverned/can't-report sit without looking like failure? | G1 | G4, G5, G7 |
| P3 | Topology canvas (Tiers/Hops/Paths, selector-matched counts) | Does never-draw-collectors hold at realistic scale? Multiple Paths per service? | G3 | G6, G7 |
| P4 | Per-node observability cards (volume/freshness/shape + expectation states) | Is expectation-red legible next to delivery-red and conformance-red? | G5 | G6, G7 |
| P5 (opt) | Rollout cohort progress | Can cohort-as-git-state be visualised across both paths? | G4 | G7 |

Prototype code is never merged. The prior repo's three-variant prototype is
mined for verdicts, not code.

## Research tasks

| # | Task | Blocks |
|---|---|---|
| R-1 | collector-contrib `metadata.yaml` catalogue feasibility (schema stability, coverage, machine-consumability; watch collector schema-roadmap issues #14543/#15010) | G2 |
| R-2 | Naming collision sweep (trademarks, GitHub/package/domain availability; scope of the Sourcegraph Amp collision) | G0 |
| R-3 | Ownership-hierarchy prior art (GitHub Teams, Backstage Groups, commercial fleet products; conditional roll-up requirements) | G1 |
| R-4 | Collector self-telemetry attribute spelling vs current release | G6 |

The 2026-08-04 research corpus in `docs/research/` is otherwise current;
re-verify only what a session forks on.

## Build phases (what `/to-issues` consumes)

- **P0 Scaffolding**: repo layout, CI, the vendor-word lint (before the code
  it lints), docs published under the final name.
- **P1 Neutral core + Conformance**: port the prior evaluator under neutral
  terminology; real seams; `Elasticsearch` as first `TelemetryProvider`; CI
  check mode; early **normaliser spike** (riskiest component, ADR-0005).
- **P2 Catalogue + Authoring backend (headless)**: catalogue ingestion;
  blueprint schema + allow-list validation; renderer with hard rules; forge
  adapter PR flow (GitHub App first, ADR-0028). Testable entirely via git
  diffs. The estate source-set abstraction (primary + satellite repos,
  ADR-0027) is modelled here from the start; satellite support itself lands
  no earlier than P4 (its content gating needs the console).
- **P3 Serving**: stateless OpAMP server, commit stamping, delivery status
  via the normaliser, both `EstateProvider` implementations, empty-config-map
  guard, bootstrap behaviour per G4.
- **P4 Console v1**: estate/team views, winning composer variant,
  conformance + delivery reporting.
- **P5 Rollout + Observability**: staged rollout per G4; Expectation engine
  and cards per G6. Last because the differentiator composes all three
  readings live.

## Exit gate for `/to-issues`

Traceability matrix has zero unmapped rows; name decided; all sessions closed
(ADRs merged, glossary updated, open-questions rows flipped); P1 to P4 verdict
one-pagers exist and G7 cites them.
