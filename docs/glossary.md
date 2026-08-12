# Glossary

The ubiquitous language. Binding on code, docs, and UI — these are not
synonyms to be varied for readability. Terms marked ⚠ are placeholders whose
definitions will be pinned in the named grill session.

## Topology (ADR-0007)

| Term | Meaning |
|---|---|
| **Stage** | A position in the pipeline: edge, gateway, or any tier a design needs. Carries the policy applying to everything at that position. There will never be many. |
| **Hop** | The directed edge between two Stages (or Stage → destination). First-class. Trust is a property of the Hop, not the Stage. |
| **Path** | One application's route through the Stage graph. An application may have several; this is normal. A Path generates the delivery expectation. |
| **Collector** | A running otelcol process. Derived and read-only: never drawn, matched into a Stage by selector, inherits that Stage's policy. |
| **Estate** | The population of collectors, across all substrates. Never "the fleet". |
| **Fleet** (capital F) | The Elastic product: Fleet Server plus the Fleet UI in Kibana. Never the estate. Appears only as the qualified implementation name `ElasticFleet`. |

## Governance

| Term | Meaning |
|---|---|
| **Criticality Tier** | A service's centrally-assigned importance classification, driving its required floor. "Tier" means this and nothing else. Tiers are cumulative: Tier 1 is Tier 2 plus more. |
| **Classification** | The orthogonal axis to Criticality: marks data sensitivity, drives routing and redaction. Adopter-named values. Never conflated with Criticality. |
| **Application** | The governed unit: a service judged against its tier's requirements. |
| **Requirement** | A versioned assertion (config and/or signal) with mandatory remediation text. A finding with no suggested fix is a complaint. |
| **Blueprint** | A named, versioned bundle of otelcol components with a phase, the signals it produces, and the requirement ids it claims to satisfy. `satisfies` is a claim of intent, never of fact. |
| **Exemption** | A waiver for one requirement with mandatory owner and expiry. Waives the count, never the diagnosis. |
| **Grace Period** | Tier-scoped onboarding window during which findings are waived. Shrinks as criticality rises. |
| ⚠ **Owner** (G1) | The lowest unit of management. Belongs to a Team. |
| ⚠ **Team** (G1) | A group of Owners and/or child Teams. Compliance rolls up the tree. |
| ⚠ **Catalogue** (G2) | The machine-generated inventory of otelcol components (from collector-contrib `metadata.yaml`) from which allow-lists select. |
| ⚠ **Allow-list** (G2) | The subset of the Catalogue a Team may use. Scoping and inheritance to be pinned. |
| ⚠ **Palette** (G2/G7) | What the composer UI offers a given user: the Catalogue filtered by their Allow-list. |

## Readings and verdicts (ADR-0004)

| Term | Meaning |
|---|---|
| **Intended** | The config in git, pinned to a commit SHA. Hand-committed configs included. |
| **Declared** | The collector's own reported effective config — never what an applier holds. |
| **Observed** | Telemetry that landed in a backend over a window. |
| **Known** | The per-reading flag keeping "we cannot see" distinct from "it is absent". Not knowing is a normal state. |
| **Outcome** | One of: `compliant`, `not_configured`, `broken_pipeline`, `not_delivered`, `ungoverned`, `misconfigured`, `unknown`, plus `library_drift`. |
| **Delivery status** | OpAMP `RemoteConfigStatus`, verbatim: `UNSET` / `APPLYING` / `APPLIED` / `FAILED`. Per collector, sits beside the conformance verdict. |
| ⚠ **Expectation** (G5/G6) | What telemetry should arrive, derived from the Intended config at a SHA — the differentiator's object. |
| ⚠ **Cohort** (G4) | The set of collectors a staged rollout step applies to. Whether it is git state is the open hypothesis. |

## Serving (ADR-0010, ADR-0013)

| Term | Meaning |
|---|---|
| **Supervisor** | The upstream OpAMP Supervisor (`opampsupervisor`), mandatory beside every served collector. |
| **Served** | A collector receiving config from the platform's OpAMP server. |
| **Foreign** | A collector whose config is delivered by anything else (GitOps, config management, a person). Legitimate, not lesser. |
| **Delivery path** | Served or git-delivered — a visible property of each collector. |

## Rules of use

- Upstream vocabulary is adopted verbatim; local synonyms are a lint error
  (ADR-0001).
- Seam names are domain terms; implementations are vendor-product-qualified:
  `ElasticFleet`, `Elasticsearch`, `GrafanaFleetManagement`.
- Every capitalised domain term in an ADR must appear here.
