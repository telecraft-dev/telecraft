# Glossary

The ubiquitous language. Binding on code, docs, and UI — these are not
synonyms to be varied for readability. Vocabulary aligned to industry/upstream
usage by ADR-0015; the visual companion is `docs/terminology.html`. Terms
marked ⚠ are placeholders whose definitions will be pinned in the named grill
session.

## Topology (ADR-0007, names per ADR-0015)

| Term | Meaning |
|---|---|
| **Tier** | A position in the collection topology: edge, gateway, or any layer a design needs. An authored, ownable object carrying the policy for everything at that position. Declares exactly one Environment and binds exactly one Blueprint version; the rendering unit — one rendered artefact per Tier (ADR-0025). Matches industry usage ("gateway tier"). Never means criticality. |
| **Hop** | The directed edge between two Tiers (or Tier → destination). First-class and ownable. Trust is a property of the Hop, not the Tier. |
| **Path** | One Service's route through the Tier graph. A Service may have several; this is normal. A Path generates the delivery expectation. |
| **Collector** | A running otelcol process. Derived and read-only: never drawn, never authored, never owned directly — matched into a Tier by selector, inheriting that Tier's policy and owner. Exceptions are expressed by splitting the Tier. |
| **Estate** | The population of collectors, across all substrates. Deliberate deviation from industry "fleet" (ADR-0015): a bare "fleet" is permanently ambiguous beside the `ElasticFleet` integration. |
| **Fleet** (capital F) | The Elastic product: Fleet Server plus the Fleet UI in Kibana. Never the estate. Appears only as the qualified implementation name `ElasticFleet`. |

## Governance (ADR-0015, ADR-0016)

| Term | Meaning |
|---|---|
| **Service** | The governed unit, identified by `service.name`. Assigned a Service Class; judged against its requirements. (Formerly "Application".) |
| **Service Class** | How much a Service matters: C1 > C2 > C3, adopter-renamable values. Drives the required floor. Cumulative: C1 = C2 plus more. Never rendered as "Tier N". |
| **Sensitivity** | The orthogonal axis: what the data is (PII, finance…). Drives routing and redaction, never completeness. Service Class ⊥ Sensitivity, never conflated. (Formerly "Classification".) |
| **Requirement** | A versioned assertion (config and/or signal) with mandatory remediation text. A finding with no suggested fix is a complaint. |
| **Component** | A configured instance of a catalogue type (receiver, processor, exporter, connector, extension): named, integer-versioned, ownable. Two residences (ADR-0024): **shared** — a standalone file, id `<team>/<name>` — or **local** — declared inline in a Blueprint, owned by its owner, not referenceable outside it. Consumers inherit shared Components by reference, never by copy; references pin a version by default, `track: head` is opt-in (ADR-0026). |
| **Blueprint** | A named, integer-versioned composition of Components, serialised as per-signal lanes (upstream signal names, explicitly ordered) plus collector-wide extensions (ADR-0024). Bound by exactly one Tier per version binding. Carries the requirement ids it claims to satisfy, version-stamped; `satisfies` is a claim of intent, never of fact. No phase concept — ordering is lane order, advised by evaluator findings. |
| **Owner** | The accountable party attached to every authored object (ADR-0016). The lowest unit of management; belongs to exactly one Team. |
| **Team** | A group of Owners and/or child Teams, forming a strict tree (single parent). Compliance rolls up the subtree as ratio-plus-worst per finding kind, waivers always visible (ADR-0017). Supplied through a seam (`teams.yaml` first-party), never owned by the platform. |
| **Catalogue** | The versioned inventory of otelcol component types — identity, per-signal stability, lifecycle — keyed `(class, type)`, one catalogue per collector release, machine-generated from upstream `metadata.yaml`. Installed catalogues are retained; a collector is judged against the catalogue for the version it runs. Adopter-authored entries layer on top. States what exists, never what may be used (that is the Allow-list). |
| **Allow-list** | The subset of the Catalogue a Team may use, keyed `(class, type)`. Effective list = parent's effective list ∩ own, plus Grants; narrowing-only down the tree. Absent any list: the whole Catalogue. Authored in git. The only rule that hard-blocks (at render). |
| **Grant** | An ancestor-authored, owned exception adding named Catalogue entries to a descendant Team's effective Allow-list. Applies to that subtree; narrowable below. Everything usable traces to the root list or a Grant. |
| **Stability floor** | The minimum upstream stability a Service's components must meet, configured per (Service Class, Environment), evaluated per (component, signal actually used). Breach is a finding, never a block. |
| **Palette** | What the composer offers a given user: Catalogue ∩ effective Allow-list, judged live by the shared evaluator. Allowed shown; floor-breaching greyed with the reason; non-allowed hidden. Pure presentation — enforces nothing. |
| **Environment** | The test/staging/production dimension of a Service's deployment, aligned to `deployment.environment.name`. One Service, one owner, many Environments; per-Environment Blueprint bindings are realised through sibling Tiers, each Tier declaring one Environment (ADR-0025). Adopter-defined open vocabulary; `production` is the distinguished value policy defaults attach to. Never called "path" (a Path is topology). |
| **Satellite repo** | An optional repo holding one Team subtree's authored content and rendered artefacts outside the primary estate repo (ADR-0027). The exception, not the path. Governance never moves: the team stays in the primary `teams.yaml`, the mapping is declared centrally, references run satellite→primary only. Verdicts are estate-public; content may be subtree-private. |
| **Exemption** | A waiver for one requirement with mandatory owner and expiry. Waives the count, never the diagnosis. |
| **Grace Period** | Service-Class-scoped onboarding window during which findings are waived. Shrinks as class rises. |

## Readings and verdicts (ADR-0004, names per ADR-0015)

| Term | Meaning |
|---|---|
| **Intended** | The config in git, pinned to a commit SHA. Hand-committed configs included. (This is what GitOps calls "declared" — we say Intended.) |
| **Effective** | The collector's own reported running config — OpAMP's `EffectiveConfig`, adopted verbatim. Never what an applier holds. (Formerly "Declared".) |
| **Observed** | Telemetry that landed in a backend over a window. |
| **Known** | The per-reading flag keeping "we cannot see" distinct from "it is absent". Not knowing is a normal state. |
| **Outcome** | One of: `compliant`, `not_configured`, `broken_pipeline`, `not_delivered`, `ungoverned`, `misconfigured`, `unknown`, plus `library_drift`. The cross is Effective × Observed, per requirement. |
| **library_drift** | Passing the version you claim or pin while failing the current one — "the goalposts moved" (ADR-0026). One finding kind with a facet for what drifted: a Requirement (stale `satisfies`) or a Component (pin behind head). Distinct in diagnosis and remediation from "you never complied". |
| **Delivery status** | OpAMP `RemoteConfigStatus`, verbatim: `UNSET` / `APPLYING` / `APPLIED` / `FAILED`. Intended × Effective, per collector, beside the conformance verdict. |
| ⚠ **Expectation** (G5/G6) | What telemetry should arrive, derived from the Intended config at a SHA — the differentiator's object. |
| ⚠ **Cohort** (G4) | The set of collectors a staged rollout step applies to. Whether it is git state is the open hypothesis. |

## Serving (ADR-0010, ADR-0013)

| Term | Meaning |
|---|---|
| **Supervisor** | The upstream OpAMP Supervisor (`opampsupervisor`), mandatory beside every served collector. |
| **Served** | A collector receiving config from the platform's OpAMP server. |
| **Foreign** | A collector whose config is delivered by anything else (GitOps, config management, a person). Legitimate, not lesser. |
| **Delivery path** | Served or git-delivered — a visible property of each collector. |
| **Forge adapter** | The seam over the git host's API (change proposals, review routing, attribution). Implementations vendor-qualified — GitHub App first (ADR-0014/0028). The mandatory floor beneath it is plain git transport (deploy key / token); governance never depends on a forge feature. |

## Rules of use

- Upstream vocabulary is adopted verbatim; local synonyms are a lint error
  (ADR-0001).
- Seam names are domain terms; implementations are vendor-product-qualified:
  `ElasticFleet`, `Elasticsearch`, `GrafanaFleetManagement`.
- A Service Class is never written "Tier N" (ADR-0015).
- Every capitalised domain term in an ADR must appear here.
